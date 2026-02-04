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
	"github.com/lirancohen/dex/internal/db"
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
	jwtSecret := flag.String("jwt-secret", "", "JWT signing secret (auto-generated if not provided)")
	showVersion := flag.Bool("version", false, "Show version and exit")
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
		// Try loading from database first (primary storage after onboarding)
		secrets, err := database.GetAllSecrets()
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
	_ = *jwtSecret // Reserved for future use (loading keys from file)

	// Create API server
	server := api.NewServer(database, api.Config{
		Addr:        *addr,
		CertFile:    *certFile,
		KeyFile:     *keyFile,
		StaticDir:   *staticDir,
		Toolbelt:    tb,
		BaseDir:     dataDir,
		TokenConfig: tokenConfig,
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
