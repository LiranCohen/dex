// Package github provides GitHub App authentication and management
package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v68/github"
)

// AppConfig holds GitHub App configuration
type AppConfig struct {
	AppID         int64  `json:"app_id"`
	AppSlug       string `json:"app_slug"`       // e.g., "my-dex"
	ClientID      string `json:"client_id"`      // OAuth client ID
	ClientSecret  string `json:"client_secret"`  // OAuth client secret
	PrivateKeyPEM string `json:"private_key"`    // PEM-encoded private key
	WebhookSecret string `json:"webhook_secret"` // Optional webhook secret
}

// Installation represents a GitHub App installation
type Installation struct {
	ID          int64     `json:"id"`
	AccountID   int64     `json:"account_id"`
	AccountType string    `json:"account_type"` // "User" or "Organization"
	Login       string    `json:"login"`        // Username or org name
	CreatedAt   time.Time `json:"created_at"`
}

// AppManager manages GitHub App authentication
type AppManager struct {
	config     *AppConfig
	privateKey *rsa.PrivateKey

	// Token cache
	mu              sync.RWMutex
	installTokens   map[int64]*cachedToken // installation_id -> token
	installationIDs map[string]int64       // login -> installation_id
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewAppManager creates a new GitHub App manager
func NewAppManager(config *AppConfig) (*AppManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Parse private key
	block, _ := pem.Decode([]byte(config.PrivateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		keyInterface, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse private key: %w (also tried PKCS8: %v)", err, err2)
		}
		var ok bool
		key, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
	}

	return &AppManager{
		config:          config,
		privateKey:      key,
		installTokens:   make(map[int64]*cachedToken),
		installationIDs: make(map[string]int64),
	}, nil
}

// GenerateJWT creates a JWT for authenticating as the GitHub App
func (m *AppManager) GenerateJWT() (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)), // Max 10 minutes
		Issuer:    fmt.Sprintf("%d", m.config.AppID),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.privateKey)
}

// GetInstallationToken returns an access token for the given installation
// Tokens are cached and automatically refreshed when expired
func (m *AppManager) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	// Check cache first
	m.mu.RLock()
	cached, ok := m.installTokens[installationID]
	m.mu.RUnlock()

	if ok && time.Now().Add(5*time.Minute).Before(cached.expiresAt) {
		return cached.token, nil
	}

	// Generate new token
	jwtToken, err := m.GenerateJWT()
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Exchange JWT for installation token
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID),
		nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get installation token: %s: %s", resp.Status, string(body))
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Cache the token
	m.mu.Lock()
	m.installTokens[installationID] = &cachedToken{
		token:     result.Token,
		expiresAt: result.ExpiresAt,
	}
	m.mu.Unlock()

	return result.Token, nil
}

// GetClientForInstallation returns a GitHub client authenticated as the installation
func (m *AppManager) GetClientForInstallation(ctx context.Context, installationID int64) (*github.Client, error) {
	token, err := m.GetInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}

	return github.NewClient(nil).WithAuthToken(token), nil
}

// ListInstallations returns all installations of the app
func (m *AppManager) ListInstallations(ctx context.Context) ([]*Installation, error) {
	jwtToken, err := m.GenerateJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.github.com/app/installations",
		nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list installations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list installations: %s: %s", resp.Status, string(body))
	}

	var ghInstalls []struct {
		ID      int64 `json:"id"`
		Account struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghInstalls); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	installs := make([]*Installation, len(ghInstalls))
	for i, gh := range ghInstalls {
		installs[i] = &Installation{
			ID:          gh.ID,
			AccountID:   gh.Account.ID,
			AccountType: gh.Account.Type,
			Login:       gh.Account.Login,
			CreatedAt:   gh.CreatedAt,
		}

		// Cache the installation ID
		m.mu.Lock()
		m.installationIDs[gh.Account.Login] = gh.ID
		m.mu.Unlock()
	}

	return installs, nil
}

// GetInstallationIDForLogin returns the installation ID for a given login (user or org)
func (m *AppManager) GetInstallationIDForLogin(ctx context.Context, login string) (int64, error) {
	// Check cache
	m.mu.RLock()
	id, ok := m.installationIDs[login]
	m.mu.RUnlock()
	if ok {
		return id, nil
	}

	// Refresh installations list
	installs, err := m.ListInstallations(ctx)
	if err != nil {
		return 0, err
	}

	for _, inst := range installs {
		if inst.Login == login {
			return inst.ID, nil
		}
	}

	return 0, fmt.Errorf("no installation found for %s", login)
}

// AppSlug returns the app's slug (used in installation URLs)
func (m *AppManager) AppSlug() string {
	return m.config.AppSlug
}

// InstallURL returns the URL to install the app
func (m *AppManager) InstallURL() string {
	return fmt.Sprintf("https://github.com/apps/%s/installations/new", m.config.AppSlug)
}

// AppManifest returns the manifest for creating a new GitHub App via the manifest flow
func AppManifest(name, callbackURL, setupURL string) map[string]any {
	return map[string]any{
		"name":             name,
		"url":              "https://github.com/lirancohen/dex",
		"callback_urls":    []string{callbackURL},
		"setup_url":        setupURL,
		"setup_on_update":  true,
		"redirect_url":     callbackURL,
		"public":           false,
		"default_events":   []string{},
		"default_permissions": map[string]string{
			"administration": "write", // Create/delete repos
			"contents":       "write", // Read/write repo contents
			"metadata":       "read",  // Read repo metadata
			"issues":         "write", // Create/update issues
			"pull_requests":  "write", // Create PRs
			"workflows":      "write", // Manage GitHub Actions workflows
		},
	}
}
