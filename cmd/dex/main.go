package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lirancohen/dex/internal/api"
	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/crypto"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/forgejo"
	"github.com/lirancohen/dex/internal/mesh"
	"github.com/lirancohen/dex/internal/toolbelt"
)

const version = "0.1.0-dev"

func main() {
	// Define flags
	dbPath := flag.String("db", "dex.db", "Path to SQLite database file")
	addr := flag.String("addr", ":8080", "Server address (e.g., :8080 or 0.0.0.0:8443)")
	certFile := flag.String("cert", "", "Path to TLS certificate (optional)")
	keyFile := flag.String("key", "", "Path to TLS key (optional)")
	staticDir := flag.String("static", "", "Path to frontend static files (e.g., ./frontend/dist)")
	toolbeltConfig := flag.String("toolbelt", "", "Path to toolbelt.yaml config file (optional)")
	baseDir := flag.String("base-dir", "", "Base Dex directory (default: /opt/dex). Repos at {base-dir}/repos/, worktrees at {base-dir}/worktrees/")
	showVersion := flag.Bool("version", false, "Show version and exit")

	// Mesh networking flags
	meshEnabled := flag.Bool("mesh", false, "Enable mesh networking")
	meshHostname := flag.String("mesh-hostname", "", "Hostname for this node on the mesh network")
	meshControlURL := flag.String("mesh-control-url", "https://central.enbox.id", "Central coordination service URL")
	meshAuthKey := flag.String("mesh-auth-key", "", "Pre-auth key for automatic mesh registration")
	meshStateDir := flag.String("mesh-state-dir", "", "Directory for mesh state (default: {base-dir}/mesh)")

	// Forgejo flags
	forgejoEnabled := flag.Bool("forgejo", false, "Enable embedded Forgejo git server")
	forgejoBinary := flag.String("forgejo-binary", "", "Path to Forgejo binary (default: auto-download)")
	forgejoPort := flag.Int("forgejo-port", 3000, "HTTP port for Forgejo")

	flag.Parse()

	if *showVersion {
		fmt.Printf("Poindexter (dex) v%s\n", version)
		os.Exit(0)
	}

	fmt.Println("Poindexter (dex) - AI Orchestration System")
	fmt.Printf("Version: %s\n", version)

	// Initialize database
	fmt.Printf("Opening database: %s\n", *dbPath)
	database, err := db.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	// Run migrations
	fmt.Println("Running database migrations...")
	if err := database.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running migrations: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Database initialized successfully")

	// Determine base directory - used for repos, worktrees, and secrets
	dataDir := *baseDir
	if dataDir == "" {
		dataDir = os.Getenv("DEX_DATA_DIR")
	}
	if dataDir == "" {
		dataDir = "/opt/dex"
	}

	// Initialize encryption
	fmt.Println("Initializing encryption...")
	encConfig, err := crypto.InitEncryption(dataDir, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing encryption: %v\n", err)
		os.Exit(1)
	}
	if encConfig.MasterKey != nil {
		fmt.Println("  - Master key: INITIALIZED (secrets encrypted at rest)")
	} else {
		fmt.Println("  - Master key: NOT CONFIGURED (secrets stored in plaintext)")
	}
	if encConfig.HQKeyPair != nil {
		fmt.Println("  - HQ keypair: INITIALIZED (worker payloads can be encrypted)")
	}

	// Create encrypted secrets store
	secretsStore := db.NewEncryptedSecretsStore(database, encConfig.MasterKey)

	// Migrate existing plaintext secrets to encrypted format
	if encConfig.MasterKey != nil {
		// Migrate file-based secrets first
		migrated, err := database.MigrateSecretsFromFile(dataDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to migrate secrets from file: %v\n", err)
		} else if migrated > 0 {
			fmt.Printf("  - Migrated %d secrets from secrets.json to database\n", migrated)
		}

		// Encrypt any plaintext secrets in database
		encrypted, err := secretsStore.MigrateToEncrypted()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to encrypt secrets: %v\n", err)
		} else if encrypted > 0 {
			fmt.Printf("  - Encrypted %d plaintext secrets\n", encrypted)
		}

		// Encrypt GitHub App config if present
		githubStore := db.NewEncryptedGitHubStore(database, encConfig.MasterKey)
		if err := githubStore.MigrateToEncrypted(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to encrypt GitHub App config: %v\n", err)
		}
	}

	// Load toolbelt configuration (optional)
	var tb *toolbelt.Toolbelt
	if *toolbeltConfig != "" {
		fmt.Printf("Loading toolbelt config: %s\n", *toolbeltConfig)
		var err error
		tb, err = toolbelt.NewFromFile(*toolbeltConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load toolbelt config: %v\n", err)
			// Continue without toolbelt - it's optional
		} else {
			status := tb.Status()
			configured := 0
			for _, s := range status {
				if s.HasToken {
					configured++
				}
			}
			fmt.Printf("Toolbelt loaded: %d/%d services configured\n", configured, len(status))
		}
	} else {
		// Try loading from encrypted database (primary storage after onboarding)
		secrets, err := secretsStore.GetAllSecrets()
		if err == nil && len(secrets) > 0 {
			fmt.Printf("Loading toolbelt from database (%d secrets)\n", len(secrets))
			config := &toolbelt.Config{}
			if token := secrets[db.SecretKeyGitHubToken]; token != "" {
				config.GitHub = &toolbelt.GitHubConfig{Token: token}
			}
			if key := secrets[db.SecretKeyAnthropicKey]; key != "" {
				config.Anthropic = &toolbelt.AnthropicConfig{APIKey: key}
			}
			tb, err = toolbelt.New(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to create toolbelt from database secrets: %v\n", err)
			}
		}

		// Fall back to secrets.json if database had no secrets (legacy/migration path)
		if tb == nil {
			secretsPath := filepath.Join(dataDir, "secrets.json")
			if _, err := os.Stat(secretsPath); err == nil {
				fmt.Printf("Loading toolbelt from secrets file: %s\n", secretsPath)
				tb, err = toolbelt.NewFromSecrets(secretsPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to load secrets file: %v\n", err)
				}
			} else {
				fmt.Println("No secrets configured yet (database empty, no secrets.json)")
			}
		}

		// Log what we loaded
		if tb != nil {
			status := tb.Status()
			configured := 0
			for _, s := range status {
				if s.HasToken {
					configured++
				}
			}
			fmt.Printf("Toolbelt loaded: %d/%d services configured\n", configured, len(status))
			if tb.Anthropic != nil {
				fmt.Println("  - Anthropic client: INITIALIZED")
			} else {
				fmt.Println("  - Anthropic client: NOT configured")
			}
			if tb.GitHub != nil {
				fmt.Println("  - GitHub client: INITIALIZED")
			} else {
				fmt.Println("  - GitHub client: NOT configured")
			}
		}
	}

	// Set up JWT token configuration with ED25519 keys
	// Generate new keys on each startup (tokens won't survive restarts)
	// For persistence, keys should be loaded from a file
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating JWT keys: %v\n", err)
		os.Exit(1)
	}

	tokenConfig := &auth.TokenConfig{
		Issuer:       "poindexter",
		ExpiryHours:  24 * 7, // 1 week
		RefreshHours: 24,     // Can refresh within 24 hours of expiry
		SigningKey:   privKey,
		VerifyingKey: pubKey,
	}
	// Configure mesh networking if enabled
	var meshConfig *mesh.Config
	if *meshEnabled {
		// Default mesh state directory
		meshState := *meshStateDir
		if meshState == "" {
			meshState = filepath.Join(dataDir, "mesh")
		}

		// Default hostname to OS hostname if not specified
		hostname := *meshHostname
		if hostname == "" {
			hostname, _ = os.Hostname()
		}

		meshConfig = &mesh.Config{
			Enabled:    true,
			Hostname:   hostname,
			StateDir:   meshState,
			ControlURL: *meshControlURL,
			AuthKey:    *meshAuthKey,
			IsHQ:       true, // dex server is always the HQ
		}
		fmt.Printf("Mesh networking enabled: hostname=%s, control=%s\n", hostname, *meshControlURL)
	}

	// Configure embedded Forgejo if enabled
	var forgejoConfig *forgejo.Config
	if *forgejoEnabled {
		cfg := forgejo.DefaultConfig(dataDir)
		cfg.HTTPPort = *forgejoPort
		if *forgejoBinary != "" {
			cfg.BinaryPath = *forgejoBinary
		}
		forgejoConfig = &cfg
		fmt.Printf("Embedded Forgejo enabled: port=%d, data=%s\n", cfg.HTTPPort, cfg.DataDir)
	}

	// Create API server
	server := api.NewServer(database, api.Config{
		Addr:        *addr,
		CertFile:    *certFile,
		KeyFile:     *keyFile,
		StaticDir:   *staticDir,
		Toolbelt:    tb,
		BaseDir:     dataDir,
		TokenConfig: tokenConfig,
		Mesh:        meshConfig,
		Encryption:  encConfig,
		Forgejo:     forgejoConfig,
	})

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Wait for interrupt signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	case sig := <-quit:
		fmt.Printf("\nReceived signal %s, shutting down...\n", sig)
	}

	// Graceful shutdown with 10 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped gracefully")
}
