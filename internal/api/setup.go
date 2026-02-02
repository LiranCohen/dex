package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/workspace"
)

// SetupStatus represents the current setup status
type SetupStatus struct {
	PasskeyRegistered bool   `json:"passkey_registered"`
	GitHubTokenSet    bool   `json:"github_token_set"`
	GitHubAppSet      bool   `json:"github_app_set"`
	GitHubAppSlug     string `json:"github_app_slug,omitempty"`
	GitHubAuthMethod  string `json:"github_auth_method"` // "app", "token", or "none"
	AnthropicKeySet   bool   `json:"anthropic_key_set"`
	SetupComplete     bool   `json:"setup_complete"`
	AccessMethod      string `json:"access_method,omitempty"` // "tailscale" or "cloudflare"
	PermanentURL      string `json:"permanent_url,omitempty"`
	WorkspaceReady    bool   `json:"workspace_ready"`
	WorkspacePath     string `json:"workspace_path,omitempty"`
}

// SetupConfig holds the setup configuration file paths
type SetupConfig struct {
	DataDir string
}

// DefaultSetupConfig returns the default setup config
func DefaultSetupConfig() *SetupConfig {
	return &SetupConfig{
		DataDir: "/opt/dex",
	}
}

// handleSetupStatus returns the current setup status
func (s *Server) handleSetupStatus(c echo.Context) error {
	status := SetupStatus{
		GitHubAuthMethod: "none",
	}
	dataDir := s.getDataDir()

	// Check if any passkeys are registered
	hasCredentials, err := s.db.HasAnyCredentials()
	if err == nil && hasCredentials {
		status.PasskeyRegistered = true
	}

	// Check for GitHub App configuration (preferred)
	if appConfig, err := s.db.GetGitHubAppConfig(); err == nil && appConfig != nil {
		status.GitHubAppSet = true
		status.GitHubAppSlug = appConfig.AppSlug
		status.GitHubAuthMethod = "app"
	}

	// Check if toolbelt has GitHub configured (legacy token)
	if s.toolbelt != nil && s.toolbelt.GitHub != nil {
		status.GitHubTokenSet = true
		if status.GitHubAuthMethod == "none" {
			status.GitHubAuthMethod = "token"
		}
	}

	// Check if toolbelt has Anthropic configured
	if s.toolbelt != nil && s.toolbelt.Anthropic != nil {
		status.AnthropicKeySet = true
	}

	// Also check secrets.json file (tokens may have been saved but toolbelt not reloaded)
	secretsFile := filepath.Join(dataDir, "secrets.json")
	if data, err := os.ReadFile(secretsFile); err == nil {
		var secrets map[string]string
		if json.Unmarshal(data, &secrets) == nil {
			if secrets["github_token"] != "" {
				status.GitHubTokenSet = true
				if status.GitHubAuthMethod == "none" {
					status.GitHubAuthMethod = "token"
				}
			}
			if secrets["anthropic_key"] != "" {
				status.AnthropicKeySet = true
			}
		}
	}

	// Check access method
	if data, err := os.ReadFile(filepath.Join(dataDir, "access-method")); err == nil {
		status.AccessMethod = strings.TrimSpace(string(data))
	}

	// Check permanent URL
	if data, err := os.ReadFile(filepath.Join(dataDir, "permanent-url")); err == nil {
		status.PermanentURL = strings.TrimSpace(string(data))
	}

	// Check if setup is complete
	if _, err := os.Stat(filepath.Join(dataDir, "setup-complete")); err == nil {
		status.SetupComplete = true
	}

	// Check workspace status
	workspacePath := filepath.Join(dataDir, "repos", "dex-workspace")
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err == nil {
		status.WorkspaceReady = true
		status.WorkspacePath = workspacePath
	}

	return c.JSON(http.StatusOK, status)
}

// handleSetupGitHubToken validates and saves a GitHub token
func (s *Server) handleSetupGitHubToken(c echo.Context) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}

	// Basic validation - should start with ghp_ or github_pat_
	if !strings.HasPrefix(req.Token, "ghp_") && !strings.HasPrefix(req.Token, "github_pat_") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token format - should start with ghp_ or github_pat_")
	}

	// Validate the token by making a test API call
	if err := validateGitHubToken(c.Request().Context(), req.Token); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("token validation failed: %v", err))
	}

	// Save the token to the secrets file
	dataDir := s.getDataDir()
	secretsFile := filepath.Join(dataDir, "secrets.json")

	secrets := make(map[string]string)

	// Read existing secrets
	if data, err := os.ReadFile(secretsFile); err == nil {
		json.Unmarshal(data, &secrets)
	}

	secrets["github_token"] = req.Token

	// Write back
	data, _ := json.MarshalIndent(secrets, "", "  ")
	if err := os.WriteFile(secretsFile, data, 0600); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "GitHub token saved successfully",
	})
}

// handleSetupAnthropicKey validates and saves an Anthropic API key
func (s *Server) handleSetupAnthropicKey(c echo.Context) error {
	var req struct {
		Key string `json:"key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}

	// Basic validation - should start with sk-ant
	if !strings.HasPrefix(req.Key, "sk-ant") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid key format - should start with sk-ant")
	}

	// Validate the key by making a test API call
	if err := validateAnthropicKey(c.Request().Context(), req.Key); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("key validation failed: %v", err))
	}

	// Save the key to the secrets file
	dataDir := s.getDataDir()
	secretsFile := filepath.Join(dataDir, "secrets.json")

	secrets := make(map[string]string)

	// Read existing secrets
	if data, err := os.ReadFile(secretsFile); err == nil {
		json.Unmarshal(data, &secrets)
	}

	secrets["anthropic_key"] = req.Key

	// Write back
	data, _ := json.MarshalIndent(secrets, "", "  ")
	if err := os.WriteFile(secretsFile, data, 0600); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save key")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Anthropic API key saved successfully",
	})
}

// handleSetupComplete marks the setup as complete
func (s *Server) handleSetupComplete(c echo.Context) error {
	dataDir := s.getDataDir()

	// Verify all required setup steps are done
	status := SetupStatus{}

	// Check if any passkeys are registered
	hasCredentials, _ := s.db.HasAnyCredentials()
	if hasCredentials {
		status.PasskeyRegistered = true
	}

	// Check secrets file
	secretsFile := filepath.Join(dataDir, "secrets.json")
	if data, err := os.ReadFile(secretsFile); err == nil {
		var secrets map[string]string
		if json.Unmarshal(data, &secrets) == nil {
			if secrets["github_token"] != "" {
				status.GitHubTokenSet = true
			}
			if secrets["anthropic_key"] != "" {
				status.AnthropicKeySet = true
			}
		}
	}

	// Also check toolbelt
	if s.toolbelt != nil {
		if s.toolbelt.GitHub != nil {
			status.GitHubTokenSet = true
		}
		if s.toolbelt.Anthropic != nil {
			status.AnthropicKeySet = true
		}
	}

	// Check for GitHub App configuration
	hasGitHubApp := s.db.HasGitHubApp()

	// Validate all steps are complete
	if !status.PasskeyRegistered {
		return echo.NewHTTPError(http.StatusBadRequest, "passkey not registered")
	}
	if !status.GitHubTokenSet && !hasGitHubApp {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub not configured (use App or token)")
	}
	if !status.AnthropicKeySet {
		return echo.NewHTTPError(http.StatusBadRequest, "Anthropic key not set")
	}

	// Reload toolbelt from secrets.json so API clients are available
	// This is necessary because the server started before keys were entered
	if err := s.ReloadToolbelt(); err != nil {
		fmt.Printf("Warning: failed to reload toolbelt: %v\n", err)
		// Continue - toolbelt will be loaded on next server restart
	}

	// Create workspace repo if it doesn't exist and git service is configured
	workspacePath := filepath.Join(dataDir, "repos", "dex-workspace")
	if s.gitService != nil && !s.gitService.RepoExists(workspacePath) {
		// Ensure repos directory exists
		reposDir := filepath.Join(dataDir, "repos")
		if err := os.MkdirAll(reposDir, 0755); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create repos directory: %v", err))
		}

		// Create workspace repo using git init directly since we may not have RepoManager
		// configured with the right base path
		if err := initWorkspaceRepo(workspacePath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create workspace: %v", err))
		}
	}

	// Optionally create GitHub workspace repository
	// First try GitHub App (preferred), then fall back to legacy PAT
	var githubClientForWorkspace *toolbelt.GitHubClient

	if hasGitHubApp {
		// Ensure GitHub App manager is initialized (may have been configured after server startup)
		if err := s.ensureGitHubAppInitialized(); err != nil {
			fmt.Printf("Warning: failed to initialize GitHub App: %v\n", err)
		}

		// Use GitHub App installation token
		client, err := s.GetToolbeltGitHubClient(c.Request().Context(), "")
		if err != nil {
			fmt.Printf("Warning: failed to get GitHub App client for workspace: %v\n", err)
		} else {
			githubClientForWorkspace = client
		}
	} else {
		// Fall back to legacy PAT
		s.toolbeltMu.RLock()
		if s.toolbelt != nil && s.toolbelt.GitHub != nil {
			githubClientForWorkspace = s.toolbelt.GitHub
		}
		s.toolbeltMu.RUnlock()
	}

	if githubClientForWorkspace != nil {
		ws := workspace.NewService(githubClientForWorkspace, workspacePath)
		if err := ws.EnsureRemoteExists(c.Request().Context()); err != nil {
			// Log warning but don't fail - GitHub workspace is optional
			fmt.Printf("Warning: failed to create GitHub workspace: %v\n", err)
		}
	}

	// Update default project to point to workspace
	project, err := s.db.GetOrCreateDefaultProject()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get default project: %v", err))
	}

	// Only update if repo_path is currently "." (the invalid default)
	if project.RepoPath == "." {
		if err := s.db.UpdateProject(project.ID, "Dex Workspace", workspacePath, "main"); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to update project: %v", err))
		}
	}

	// Write completion file
	completeFile := filepath.Join(dataDir, "setup-complete")
	if err := os.WriteFile(completeFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to signal completion")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":        true,
		"message":        "Setup complete!",
		"workspace_path": workspacePath,
	})
}

// initWorkspaceRepo initializes the dex-workspace git repository
func initWorkspaceRepo(repoPath string) error {
	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w\n%s", err, output)
	}

	// Create README
	readme := `# Dex Workspace

This is the default workspace repository for Dex tasks.
`
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("failed to create README: %w", err)
	}

	// Stage and commit
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, output)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Dex",
		"GIT_AUTHOR_EMAIL=dex@local",
		"GIT_COMMITTER_NAME=Dex",
		"GIT_COMMITTER_EMAIL=dex@local",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %w\n%s", err, output)
	}

	return nil
}

// getDataDir returns the data directory path
func (s *Server) getDataDir() string {
	// Check environment variable first
	if dir := os.Getenv("DEX_DATA_DIR"); dir != "" {
		return dir
	}
	return "/opt/dex"
}

// validateGitHubToken validates a GitHub personal access token
func validateGitHubToken(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	return nil
}

// validateAnthropicKey validates an Anthropic API key
func validateAnthropicKey(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return err
	}

	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("invalid key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	return nil
}
