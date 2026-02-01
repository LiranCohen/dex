package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gogithub "github.com/google/go-github/v68/github"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// initGitHubApp initializes the GitHub App manager from database configuration
func (s *Server) initGitHubApp() {
	config, err := s.db.GetGitHubAppConfig()
	if err != nil || config == nil {
		return
	}

	appManager, err := github.NewAppManager(&github.AppConfig{
		AppID:         config.AppID,
		AppSlug:       config.AppSlug,
		ClientID:      config.ClientID,
		ClientSecret:  config.ClientSecret,
		PrivateKeyPEM: config.PrivateKey,
		WebhookSecret: config.WebhookSecret,
	})
	if err != nil {
		fmt.Printf("Warning: failed to initialize GitHub App manager: %v\n", err)
		return
	}

	s.githubAppMu.Lock()
	s.githubApp = appManager
	s.githubAppMu.Unlock()
}

// GetGitHubClientForInstallation returns a GitHub client for the specified installation login.
// Returns nil if no GitHub App is configured or installation not found.
func (s *Server) GetGitHubClientForInstallation(ctx context.Context, login string) (*gogithub.Client, error) {
	s.githubAppMu.RLock()
	appManager := s.githubApp
	s.githubAppMu.RUnlock()

	if appManager == nil {
		return nil, fmt.Errorf("GitHub App not configured")
	}

	// Get installation ID for login
	installID, err := appManager.GetInstallationIDForLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation for %s: %w", login, err)
	}

	return appManager.GetClientForInstallation(ctx, installID)
}

// GetToolbeltGitHubClient returns a toolbelt.GitHubClient for the specified installation login.
// This wraps the GitHub App installation token in a toolbelt-compatible client.
func (s *Server) GetToolbeltGitHubClient(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
	s.githubAppMu.RLock()
	appManager := s.githubApp
	s.githubAppMu.RUnlock()

	if appManager == nil {
		return nil, fmt.Errorf("GitHub App not configured")
	}

	// Get installation ID for login
	installID, err := appManager.GetInstallationIDForLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation for %s: %w", login, err)
	}

	// Get installation token
	token, err := appManager.GetInstallationToken(ctx, installID)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	// Create toolbelt client from token
	return toolbelt.NewGitHubClientFromToken(token, login), nil
}

// GitHubAppStatus represents the GitHub App configuration status
type GitHubAppStatus struct {
	AppConfigured   bool   `json:"app_configured"`
	AppSlug         string `json:"app_slug,omitempty"`
	InstallURL      string `json:"install_url,omitempty"`
	Installations   int    `json:"installations"`
	LegacyTokenSet  bool   `json:"legacy_token_set"`
	AuthMethod      string `json:"auth_method"` // "app", "token", or "none"
}

// handleGitHubAppStatus returns the current GitHub App configuration status
func (s *Server) handleGitHubAppStatus(c echo.Context) error {
	status := GitHubAppStatus{
		AuthMethod: "none",
	}

	// Check for GitHub App configuration
	config, err := s.db.GetGitHubAppConfig()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get GitHub App config: %v", err))
	}

	if config != nil {
		status.AppConfigured = true
		status.AppSlug = config.AppSlug
		status.InstallURL = fmt.Sprintf("https://github.com/apps/%s/installations/new", config.AppSlug)
		status.AuthMethod = "app"

		// Get installation count
		installs, err := s.db.ListGitHubInstallations()
		if err == nil {
			status.Installations = len(installs)
		}
	}

	// Check for legacy token
	dataDir := s.getDataDir()
	secretsFile := filepath.Join(dataDir, "secrets.json")
	if data, err := os.ReadFile(secretsFile); err == nil {
		var secrets map[string]string
		if json.Unmarshal(data, &secrets) == nil {
			if secrets["github_token"] != "" {
				status.LegacyTokenSet = true
				if status.AuthMethod == "none" {
					status.AuthMethod = "token"
				}
			}
		}
	}

	return c.JSON(http.StatusOK, status)
}

// handleGitHubAppManifest returns the manifest for creating a GitHub App
func (s *Server) handleGitHubAppManifest(c echo.Context) error {
	// Get the base URL from the request or config
	baseURL := c.Request().Header.Get("X-Forwarded-Host")
	if baseURL == "" {
		baseURL = c.Request().Host
	}

	// Use HTTPS
	scheme := "https"
	if c.Request().Header.Get("X-Forwarded-Proto") != "" {
		scheme = c.Request().Header.Get("X-Forwarded-Proto")
	}

	fullBaseURL := fmt.Sprintf("%s://%s", scheme, baseURL)

	// Generate a unique app name
	appName := fmt.Sprintf("dex-%s", generateShortID())

	manifest := github.AppManifest(
		appName,
		fullBaseURL+"/api/v1/setup/github/app/callback",
		fullBaseURL+"/api/v1/setup/github/install/callback",
	)

	// Return the manifest and the URL to redirect to
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal manifest")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"manifest":     manifest,
		"manifest_url": "https://github.com/settings/apps/new",
		"redirect_url": fmt.Sprintf("https://github.com/settings/apps/new?manifest=%s", string(manifestJSON)),
	})
}

// handleGitHubAppCallback handles the callback from GitHub after app creation
func (s *Server) handleGitHubAppCallback(c echo.Context) error {
	// GitHub sends the code as a query parameter
	code := c.QueryParam("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing code parameter")
	}

	// Exchange the code for app credentials
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	appConfig, err := exchangeManifestCode(ctx, code)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to exchange code: %v", err))
	}

	// Save to database
	dbConfig := &db.GitHubAppConfig{
		AppID:         appConfig.AppID,
		AppSlug:       appConfig.AppSlug,
		ClientID:      appConfig.ClientID,
		ClientSecret:  appConfig.ClientSecret,
		PrivateKey:    appConfig.PrivateKeyPEM,
		WebhookSecret: appConfig.WebhookSecret,
	}

	if err := s.db.SaveGitHubAppConfig(dbConfig); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to save app config: %v", err))
	}

	// Return success and redirect URL to install the app
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", appConfig.AppSlug)

	// Redirect to frontend with success parameter
	return c.Redirect(http.StatusFound, "/setup?github_app=created&install_url="+installURL)
}

// handleGitHubInstallCallback handles the callback after app installation
func (s *Server) handleGitHubInstallCallback(c echo.Context) error {
	installationID := c.QueryParam("installation_id")
	if installationID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing installation_id")
	}

	// Get the app config
	config, err := s.db.GetGitHubAppConfig()
	if err != nil || config == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not configured")
	}

	// Create app manager to fetch installation details
	appManager, err := github.NewAppManager(&github.AppConfig{
		AppID:         config.AppID,
		AppSlug:       config.AppSlug,
		ClientID:      config.ClientID,
		ClientSecret:  config.ClientSecret,
		PrivateKeyPEM: config.PrivateKey,
		WebhookSecret: config.WebhookSecret,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create app manager: %v", err))
	}

	// Fetch all installations and find the one we just received
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	installations, err := appManager.ListInstallations(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to list installations: %v", err))
	}

	// Save the installation
	var savedInstall *github.Installation
	for _, install := range installations {
		if fmt.Sprintf("%d", install.ID) == installationID {
			savedInstall = install
			break
		}
	}

	if savedInstall == nil {
		// Try to find it in case the ID format is different
		for _, install := range installations {
			savedInstall = install
			break // Just take the first one
		}
	}

	if savedInstall != nil {
		dbInstall := &db.GitHubInstallation{
			ID:          savedInstall.ID,
			AccountID:   savedInstall.AccountID,
			AccountType: savedInstall.AccountType,
			Login:       savedInstall.Login,
		}
		if err := s.db.SaveGitHubInstallation(dbInstall); err != nil {
			fmt.Printf("Warning: failed to save installation: %v\n", err)
		}
	}

	// Redirect to frontend with success
	return c.Redirect(http.StatusFound, "/setup?github_installed=true")
}

// handleGitHubInstallations returns the list of GitHub App installations
func (s *Server) handleGitHubInstallations(c echo.Context) error {
	installations, err := s.db.ListGitHubInstallations()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to list installations: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"installations": installations,
	})
}

// handleGitHubSyncInstallations syncs installations from GitHub
func (s *Server) handleGitHubSyncInstallations(c echo.Context) error {
	config, err := s.db.GetGitHubAppConfig()
	if err != nil || config == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not configured")
	}

	appManager, err := github.NewAppManager(&github.AppConfig{
		AppID:         config.AppID,
		AppSlug:       config.AppSlug,
		ClientID:      config.ClientID,
		ClientSecret:  config.ClientSecret,
		PrivateKeyPEM: config.PrivateKey,
		WebhookSecret: config.WebhookSecret,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create app manager: %v", err))
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	installations, err := appManager.ListInstallations(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to list installations: %v", err))
	}

	// Save all installations
	for _, install := range installations {
		dbInstall := &db.GitHubInstallation{
			ID:          install.ID,
			AccountID:   install.AccountID,
			AccountType: install.AccountType,
			Login:       install.Login,
		}
		if err := s.db.SaveGitHubInstallation(dbInstall); err != nil {
			fmt.Printf("Warning: failed to save installation %s: %v\n", install.Login, err)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"synced":        len(installations),
		"installations": installations,
	})
}

// handleGitHubDeleteApp removes the GitHub App configuration
func (s *Server) handleGitHubDeleteApp(c echo.Context) error {
	if err := s.db.DeleteGitHubAppConfig(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to delete app config: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "GitHub App configuration removed",
	})
}

// exchangeManifestCode exchanges a manifest code for app credentials
func exchangeManifestCode(ctx context.Context, code string) (*github.AppConfig, error) {
	url := fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(nil))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected response %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		PEM           string `json:"pem"`
		WebhookSecret string `json:"webhook_secret"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &github.AppConfig{
		AppID:         result.ID,
		AppSlug:       result.Slug,
		ClientID:      result.ClientID,
		ClientSecret:  result.ClientSecret,
		PrivateKeyPEM: result.PEM,
		WebhookSecret: result.WebhookSecret,
	}, nil
}

// generateShortID generates a short random ID for the app name
func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
