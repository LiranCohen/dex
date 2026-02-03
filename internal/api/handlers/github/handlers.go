// Package github provides HTTP handlers for GitHub App operations.
package github

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/github"
)

// Handler handles GitHub App-related HTTP requests.
type Handler struct {
	deps *core.Deps

	// InitGitHubApp initializes the GitHub App manager from database configuration
	InitGitHubApp func() error
}

// New creates a new GitHub handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all GitHub App routes on the given group.
// These are all public routes (callbacks must be accessible to GitHub).
//   - GET /setup/github/app/status
//   - GET /setup/github/app/manifest
//   - GET /setup/github/app/callback
//   - GET /setup/github/install/callback
//   - GET /setup/github/installations
//   - POST /setup/github/installations/sync
//   - DELETE /setup/github/app
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/setup/github/app/status", h.HandleStatus)
	g.GET("/setup/github/app/manifest", h.HandleManifest)
	g.GET("/setup/github/app/callback", h.HandleAppCallback)
	g.GET("/setup/github/install/callback", h.HandleInstallCallback)
	g.GET("/setup/github/installations", h.HandleListInstallations)
	g.POST("/setup/github/installations/sync", h.HandleSyncInstallations)
	g.DELETE("/setup/github/app", h.HandleDelete)
}

// GitHubAppStatus represents the GitHub App configuration status.
type GitHubAppStatus struct {
	AppConfigured  bool   `json:"app_configured"`
	AppSlug        string `json:"app_slug,omitempty"`
	InstallURL     string `json:"install_url,omitempty"`
	Installations  int    `json:"installations"`
	LegacyTokenSet bool   `json:"legacy_token_set"`
	AuthMethod     string `json:"auth_method"` // "app", "token", or "none"
}

// buildInstallURL creates the GitHub App installation URL with optional org targeting.
func (h *Handler) buildInstallURL(appSlug string) string {
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", appSlug)
	if progress, err := h.deps.DB.GetOnboardingProgress(); err == nil && progress.GetGitHubOrgID() != 0 {
		installURL = fmt.Sprintf("%s?suggested_target_id=%d", installURL, progress.GetGitHubOrgID())
	}
	return installURL
}

// HandleStatus returns the current GitHub App configuration status.
// GET /api/v1/setup/github/app/status
func (h *Handler) HandleStatus(c echo.Context) error {
	status := GitHubAppStatus{
		AuthMethod: "none",
	}

	config, err := h.deps.DB.GetGitHubAppConfig()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get GitHub App config: %v", err))
	}

	if config != nil {
		status.AppConfigured = true
		status.AppSlug = config.AppSlug
		status.AuthMethod = "app"
		status.InstallURL = h.buildInstallURL(config.AppSlug)

		installs, err := h.deps.DB.ListGitHubInstallations()
		if err == nil {
			status.Installations = len(installs)
		}
	}

	if h.deps.DB.HasSecret(db.SecretKeyGitHubToken) {
		status.LegacyTokenSet = true
		if status.AuthMethod == "none" {
			status.AuthMethod = "token"
		}
	}

	return c.JSON(http.StatusOK, status)
}

// HandleManifest returns the manifest for creating a GitHub App.
// GET /api/v1/setup/github/app/manifest
func (h *Handler) HandleManifest(c echo.Context) error {
	progress, err := h.deps.DB.GetOnboardingProgress()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get onboarding progress")
	}
	orgName := progress.GetGitHubOrgName()
	if orgName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub organization must be set before creating the app")
	}

	baseURL := c.Request().Header.Get("X-Forwarded-Host")
	if baseURL == "" {
		baseURL = c.Request().Host
	}

	scheme := "https"
	if c.Request().Header.Get("X-Forwarded-Proto") != "" {
		scheme = c.Request().Header.Get("X-Forwarded-Proto")
	}

	fullBaseURL := fmt.Sprintf("%s://%s", scheme, baseURL)
	appName := fmt.Sprintf("dex-%s", generateShortID())

	manifest := github.AppManifest(
		appName,
		fullBaseURL+"/api/v1/setup/github/app/callback",
		fullBaseURL+"/api/v1/setup/github/install/callback",
	)

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal manifest")
	}

	orgManifestURL := fmt.Sprintf("https://github.com/organizations/%s/settings/apps/new", orgName)

	return c.JSON(http.StatusOK, map[string]any{
		"manifest":     manifest,
		"manifest_url": orgManifestURL,
		"redirect_url": fmt.Sprintf("%s?manifest=%s", orgManifestURL, string(manifestJSON)),
	})
}

// HandleAppCallback handles the callback from GitHub after app creation.
// GET /api/v1/setup/github/app/callback?code=...
func (h *Handler) HandleAppCallback(c echo.Context) error {
	code := c.QueryParam("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing code parameter")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	appConfig, err := exchangeManifestCode(ctx, code)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to exchange code: %v", err))
	}

	dbConfig := &db.GitHubAppConfig{
		AppID:         appConfig.AppID,
		AppSlug:       appConfig.AppSlug,
		ClientID:      appConfig.ClientID,
		ClientSecret:  appConfig.ClientSecret,
		PrivateKey:    appConfig.PrivateKeyPEM,
		WebhookSecret: appConfig.WebhookSecret,
	}

	if err := h.deps.DB.SaveGitHubAppConfig(dbConfig); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to save app config: %v", err))
	}

	if h.InitGitHubApp != nil {
		if err := h.InitGitHubApp(); err != nil {
			fmt.Printf("Warning: failed to initialize GitHub App after creation: %v\n", err)
		}
	}

	installURL := h.buildInstallURL(appConfig.AppSlug)
	return c.Redirect(http.StatusFound, "/?github_app=created&install_url="+url.QueryEscape(installURL))
}

// HandleInstallCallback handles the callback after app installation.
// GET /api/v1/setup/github/install/callback?installation_id=...
func (h *Handler) HandleInstallCallback(c echo.Context) error {
	installationID := c.QueryParam("installation_id")
	if installationID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing installation_id")
	}

	appManager := h.deps.GetGitHubApp()
	if appManager == nil {
		// Try to initialize
		if h.InitGitHubApp != nil {
			if err := h.InitGitHubApp(); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("GitHub App not configured: %v", err))
			}
			appManager = h.deps.GetGitHubApp()
		}
		if appManager == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not configured")
		}
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	installations, err := appManager.ListInstallations(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to list installations: %v", err))
	}

	var savedInstall *github.Installation
	for _, install := range installations {
		if fmt.Sprintf("%d", install.ID) == installationID {
			savedInstall = install
			break
		}
	}

	if savedInstall == nil && len(installations) > 0 {
		savedInstall = installations[0]
	}

	if savedInstall != nil {
		dbInstall := &db.GitHubInstallation{
			ID:          savedInstall.ID,
			AccountID:   savedInstall.AccountID,
			AccountType: savedInstall.AccountType,
			Login:       savedInstall.Login,
		}
		if err := h.deps.DB.SaveGitHubInstallation(dbInstall); err != nil {
			fmt.Printf("Warning: failed to save installation: %v\n", err)
		}
	}

	return c.Redirect(http.StatusFound, "/?github_installed=true")
}

// HandleListInstallations returns the list of GitHub App installations.
// GET /api/v1/setup/github/installations
func (h *Handler) HandleListInstallations(c echo.Context) error {
	installations, err := h.deps.DB.ListGitHubInstallations()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to list installations: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"installations": installations,
	})
}

// HandleSyncInstallations syncs installations from GitHub.
// POST /api/v1/setup/github/installations/sync
func (h *Handler) HandleSyncInstallations(c echo.Context) error {
	appManager := h.deps.GetGitHubApp()
	if appManager == nil {
		if h.InitGitHubApp != nil {
			if err := h.InitGitHubApp(); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("GitHub App not configured: %v", err))
			}
			appManager = h.deps.GetGitHubApp()
		}
		if appManager == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not configured")
		}
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	installations, err := appManager.ListInstallations(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to list installations: %v", err))
	}

	for _, install := range installations {
		dbInstall := &db.GitHubInstallation{
			ID:          install.ID,
			AccountID:   install.AccountID,
			AccountType: install.AccountType,
			Login:       install.Login,
		}
		if err := h.deps.DB.SaveGitHubInstallation(dbInstall); err != nil {
			fmt.Printf("Warning: failed to save installation %s: %v\n", install.Login, err)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"synced":        len(installations),
		"installations": installations,
	})
}

// HandleDelete removes the GitHub App configuration.
// DELETE /api/v1/setup/github/app
func (h *Handler) HandleDelete(c echo.Context) error {
	if err := h.deps.DB.DeleteGitHubAppConfig(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to delete app config: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "GitHub App configuration removed",
	})
}

// exchangeManifestCode exchanges a manifest code for app credentials.
func exchangeManifestCode(ctx context.Context, code string) (*github.AppConfig, error) {
	apiURL := fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(nil))
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

// generateShortID generates a short random ID for the app name.
func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
