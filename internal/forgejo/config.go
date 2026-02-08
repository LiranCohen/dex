// Package forgejo manages an embedded Forgejo instance as a child process.
package forgejo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the configuration for the embedded Forgejo instance.
type Config struct {
	// DataDir is the base directory for all Forgejo data.
	// Contains bin/, app.ini, forgejo.db, repositories/, log/.
	DataDir string

	// BinaryPath overrides the default binary location.
	// If empty, defaults to {DataDir}/bin/forgejo.
	BinaryPath string

	// HTTPAddr is the address Forgejo binds to (default: 127.0.1).
	HTTPAddr string

	// HTTPPort is the port Forgejo listens on (default: 3000).
	HTTPPort int

	// RootURL is the external-facing URL for Forgejo.
	// Used in generated links, clone URLs, etc.
	RootURL string

	// DefaultOrgName is the name of the default organization created during bootstrap.
	// Projects are created under this org. Defaults to "workspace".
	DefaultOrgName string

	// RunUser is the username to run Forgejo as.
	// If empty and running as root, defaults to "nobody".
	// Forgejo refuses to run as root for security reasons.
	RunUser string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(dataDir string) Config {
	return Config{
		DataDir:        filepath.Join(dataDir, "forgejo"),
		HTTPAddr:       "127.0.0.1",
		HTTPPort:       3000,
		RootURL:        "http://127.0.0.1:3000",
		DefaultOrgName: "workspace",
	}
}

// GetBinaryPath returns the path to the Forgejo binary.
func (c *Config) GetBinaryPath() string {
	if c.BinaryPath != "" {
		return c.BinaryPath
	}
	return filepath.Join(c.DataDir, "bin", "forgejo")
}

// GetAppIniPath returns the path to the generated app.ini.
func (c *Config) GetAppIniPath() string {
	return filepath.Join(c.DataDir, "custom", "conf", "app.ini")
}

// GetRepoRoot returns the path where Forgejo stores bare repositories.
func (c *Config) GetRepoRoot() string {
	return filepath.Join(c.DataDir, "repositories")
}

// GetDBPath returns the path to Forgejo's SQLite database.
func (c *Config) GetDBPath() string {
	return filepath.Join(c.DataDir, "forgejo.db")
}

// GetDefaultOrgName returns the default organization name.
func (c *Config) GetDefaultOrgName() string {
	if c.DefaultOrgName != "" {
		return c.DefaultOrgName
	}
	return "workspace"
}

// EnvVars returns the environment variables needed for Forgejo processes.
func (c *Config) EnvVars() []string {
	// HOME must point to a directory the Forgejo user can write to.
	// Forgejo tries to manage SSH keys in $HOME/.ssh even when SSH is disabled.
	homeDir := filepath.Join(c.DataDir, "data", "home")
	return []string{
		"HOME=" + homeDir,
		"FORGEJO_WORK_DIR=" + c.DataDir,
		"FORGEJO_CUSTOM=" + c.DataDir + "/custom",
	}
}

// WriteAppIni generates and writes the Forgejo configuration file.
// This is called on first run and whenever config needs updating.
func (c *Config) WriteAppIni() error {
	iniPath := c.GetAppIniPath()
	if err := os.MkdirAll(filepath.Dir(iniPath), 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	secretKey, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("failed to generate secret key: %w", err)
	}

	internalToken, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("failed to generate internal token: %w", err)
	}

	// If app.ini already exists, read existing secrets to avoid rotating them
	if data, err := os.ReadFile(iniPath); err == nil {
		if existing := extractIniValue(string(data), "security", "SECRET_KEY"); existing != "" {
			secretKey = existing
		}
		if existing := extractIniValue(string(data), "security", "INTERNAL_TOKEN"); existing != "" {
			internalToken = existing
		}
	}

	httpAddr := c.HTTPAddr
	if httpAddr == "" {
		httpAddr = "127.0.0.1"
	}
	httpPort := c.HTTPPort
	if httpPort == 0 {
		httpPort = 3000
	}
	rootURL := c.RootURL
	if rootURL == "" {
		rootURL = fmt.Sprintf("http://%s:%d", httpAddr, httpPort)
	}

	ini := fmt.Sprintf(`; Forgejo configuration — managed by Dex
; Do not edit manually; regenerated on startup.

[server]
HTTP_ADDR          = %s
HTTP_PORT          = %d
ROOT_URL           = %s
DISABLE_SSH        = true
LFS_START_SERVER   = false
APP_DATA_PATH      = %s

[database]
DB_TYPE = sqlite3
PATH    = %s

[security]
INSTALL_LOCK   = true
SECRET_KEY     = %s
INTERNAL_TOKEN = %s

[service]
DISABLE_REGISTRATION = true
REQUIRE_SIGNIN_VIEW  = true
ENABLE_NOTIFY_MAIL   = false

[repository]
DEFAULT_PRIVATE = private
ROOT            = %s

[log]
MODE  = console
LEVEL = debug

[actions]
ENABLED = false

[packages]
ENABLED = false

[indexer]
ISSUE_INDEXER_TYPE = bleve
REPO_INDEXER_ENABLED = false
`,
		httpAddr,
		httpPort,
		rootURL,
		filepath.Join(c.DataDir, "data"),
		c.GetDBPath(),
		secretKey,
		internalToken,
		c.GetRepoRoot(),
	)

	return os.WriteFile(iniPath, []byte(ini), 0600)
}

// EnsureDirectories creates all required directories for Forgejo.
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		filepath.Join(c.DataDir, "bin"),
		filepath.Join(c.DataDir, "custom", "conf"),
		filepath.Join(c.DataDir, "data"),
		filepath.Join(c.DataDir, "data", "home"), // HOME for Forgejo process (SSH keys, etc.)
		c.GetRepoRoot(),
		filepath.Join(c.DataDir, "log"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func generateSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// extractIniValue does a simple extraction of a value from an INI file.
// This avoids pulling in an INI parsing library for a single use case.
func extractIniValue(content, section, key string) string {
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[") == section
			continue
		}
		if inSection && strings.HasPrefix(line, key) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}
