package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/lirancohen/dex/internal/db"
)

// Secret keys used by the Forgejo subsystem.
const (
	SecretKeyAdminToken    = "forgejo_admin_token"
	SecretKeyBotToken      = "forgejo_bot_token"
	SecretKeyAdminPassword = "forgejo_admin_password"
	SecretKeyOAuthSecret   = "forgejo_oauth_secret"
)

// OAuthClientID is the client ID used when registering Forgejo with HQ's OIDC provider.
const OAuthClientID = "forgejo"

// Manager controls the lifecycle of an embedded Forgejo instance.
type Manager struct {
	config Config
	db     *db.DB

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	cancel  context.CancelFunc
}

// NewManager creates a Forgejo manager.
func NewManager(config Config, database *db.DB) *Manager {
	return &Manager{
		config: config,
		db:     database,
	}
}

// Start launches the Forgejo process and waits until it is healthy.
// If this is the first run (no admin token in DB), it performs bootstrap setup.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("forgejo is already running")
	}
	m.mu.Unlock()

	// Ensure binary exists
	binaryPath := m.config.GetBinaryPath()
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		if err := m.ensureBinary(ctx); err != nil {
			return fmt.Errorf("forgejo binary not available: %w", err)
		}
	}

	// Ensure directories and config
	if err := m.config.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	if err := m.config.WriteAppIni(); err != nil {
		return fmt.Errorf("failed to write app.ini: %w", err)
	}

	// Check if bootstrap is needed (first run)
	needsBootstrap := !m.db.HasSecret(SecretKeyAdminToken)

	// Start the process
	procCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()

	if err := m.startProcess(procCtx); err != nil {
		cancel()
		return fmt.Errorf("failed to start forgejo process: %w", err)
	}

	// Wait for health
	if err := m.waitForHealthy(ctx, 60*time.Second); err != nil {
		m.Stop()
		return fmt.Errorf("forgejo failed to become healthy: %w", err)
	}

	fmt.Println("Forgejo is running and healthy")

	// Bootstrap on first run
	if needsBootstrap {
		fmt.Println("First run detected — bootstrapping Forgejo...")
		if err := m.bootstrap(ctx); err != nil {
			m.Stop()
			return fmt.Errorf("forgejo bootstrap failed: %w", err)
		}
		fmt.Println("Forgejo bootstrap complete")
	}

	return nil
}

// Stop shuts down the Forgejo process gracefully.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil {
		return nil
	}

	// Cancel the process context
	if m.cancel != nil {
		m.cancel()
	}

	// Send SIGTERM and wait
	if m.cmd.Process != nil {
		_ = m.cmd.Process.Signal(os.Interrupt)

		// Give it 10 seconds to shut down
		done := make(chan error, 1)
		go func() { done <- m.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = m.cmd.Process.Kill()
		}
	}

	m.running = false
	m.cmd = nil
	fmt.Println("Forgejo stopped")
	return nil
}

// IsRunning returns whether the Forgejo process is currently running.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// BaseURL returns the HTTP base URL for the Forgejo instance.
func (m *Manager) BaseURL() string {
	addr := m.config.HTTPAddr
	if addr == "" {
		addr = "127.0.0.1"
	}
	port := m.config.HTTPPort
	if port == 0 {
		port = 3000
	}
	return fmt.Sprintf("http://%s:%d", addr, port)
}

// BotToken returns the bot account's API token from the database.
func (m *Manager) BotToken() (string, error) {
	token, err := m.db.GetSecret(SecretKeyBotToken)
	if err != nil {
		return "", fmt.Errorf("failed to get bot token: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("forgejo bot token not configured (bootstrap may not have run)")
	}
	return token, nil
}

// AdminToken returns the admin account's API token from the database.
func (m *Manager) AdminToken() (string, error) {
	token, err := m.db.GetSecret(SecretKeyAdminToken)
	if err != nil {
		return "", fmt.Errorf("failed to get admin token: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("forgejo admin token not configured (bootstrap may not have run)")
	}
	return token, nil
}

// OAuthSecret returns the OAuth client secret for OIDC integration.
func (m *Manager) OAuthSecret() (string, error) {
	secret, err := m.db.GetSecret(SecretKeyOAuthSecret)
	if err != nil {
		return "", fmt.Errorf("failed to get OAuth secret: %w", err)
	}
	if secret == "" {
		return "", fmt.Errorf("forgejo OAuth secret not configured (SSO bootstrap may not have run)")
	}
	return secret, nil
}

// AccessInfo returns the credentials needed to log into the Forgejo web UI.
type AccessInfo struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// WebAccess returns the URL and credentials for the Forgejo web UI.
func (m *Manager) WebAccess() (*AccessInfo, error) {
	password, err := m.db.GetSecret(SecretKeyAdminPassword)
	if err != nil || password == "" {
		return nil, fmt.Errorf("admin password not available (bootstrap may not have run)")
	}
	return &AccessInfo{
		URL:      m.BaseURL(),
		Username: AdminUsername,
		Password: password,
	}, nil
}

// RepoPath returns the filesystem path to a bare repo in Forgejo's repository root.
// owner and repo correspond to the Forgejo organization and repository name.
func (m *Manager) RepoPath(owner, repo string) string {
	return fmt.Sprintf("%s/%s/%s.git", m.config.GetRepoRoot(), owner, repo)
}

// Config returns the current configuration (read-only copy).
func (m *Manager) Config() Config {
	return m.config
}

func (m *Manager) startProcess(ctx context.Context) error {
	binaryPath := m.config.GetBinaryPath()

	cmd := exec.CommandContext(ctx, binaryPath, "web",
		"--config", m.config.GetAppIniPath(),
		"--work-path", m.config.DataDir,
	)

	// Forgejo logs to console in our config; capture stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Set FORGEJO_WORK_DIR so Forgejo can find its data
	cmd.Env = append(os.Environ(), m.config.EnvVars()...)

	// If running as root, drop privileges for Forgejo subprocess
	// Forgejo refuses to run as root for security reasons
	if os.Getuid() == 0 {
		runUser := m.config.RunUser
		if runUser == "" {
			runUser = "nobody"
		}

		u, err := user.Lookup(runUser)
		if err != nil {
			return fmt.Errorf("failed to lookup user %s: %w", runUser, err)
		}

		uid, _ := strconv.ParseUint(u.Uid, 10, 32)
		gid, _ := strconv.ParseUint(u.Gid, 10, 32)

		// Chown the data directory so Forgejo can write to it
		if err := chownRecursive(m.config.DataDir, int(uid), int(gid)); err != nil {
			return fmt.Errorf("failed to chown forgejo data dir: %w", err)
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
		fmt.Printf("Forgejo will run as user %s (uid=%d, gid=%d)\n", runUser, uid, gid)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start forgejo: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.running = true
	m.mu.Unlock()

	// Monitor the process in background
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		m.running = false
		m.cmd = nil
		m.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "Forgejo process exited unexpectedly: %v\n", err)
		}
	}()

	return nil
}

func (m *Manager) waitForHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	// Use root path - we just need to verify Forgejo responds to HTTP
	healthURL := m.BaseURL() + "/"

	client := &http.Client{
		Timeout: 2 * time.Second,
		// Don't follow redirects - we just want to know if Forgejo responds
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			// Any HTTP response means Forgejo is running.
			// Could be 200, 302 redirect, 403 forbidden - all indicate healthy.
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("forgejo did not become healthy within %s", timeout)
}

// chownRecursive changes ownership of a directory and all its contents.
func chownRecursive(path string, uid, gid int) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(name, uid, gid)
	})
}
