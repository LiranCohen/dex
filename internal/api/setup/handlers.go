package setup

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/workspace"
)

// Handler handles all setup-related HTTP requests
type Handler struct {
	db                  *db.DB
	getDataDir          func() string
	getToolbelt         func() *toolbelt.Toolbelt
	reloadToolbelt      func() error
	getGitHubClient     func(ctx context.Context, login string) (*toolbelt.GitHubClient, error)
	hasGitHubApp        func() bool
	initGitHubApp       func() error
	getGitService       func() GitService
	updateDefaultProject func(workspacePath string) error
}

// GitService is the interface for git operations needed by setup
type GitService interface {
	RepoExists(path string) bool
}

// HandlerConfig holds configuration for creating a Handler
type HandlerConfig struct {
	DB                   *db.DB
	GetDataDir           func() string
	GetToolbelt          func() *toolbelt.Toolbelt
	ReloadToolbelt       func() error
	GetGitHubClient      func(ctx context.Context, login string) (*toolbelt.GitHubClient, error)
	HasGitHubApp         func() bool
	InitGitHubApp        func() error
	GetGitService        func() GitService
	UpdateDefaultProject func(workspacePath string) error
}

// NewHandler creates a new setup handler
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		db:                   cfg.DB,
		getDataDir:           cfg.GetDataDir,
		getToolbelt:          cfg.GetToolbelt,
		reloadToolbelt:       cfg.ReloadToolbelt,
		getGitHubClient:      cfg.GetGitHubClient,
		hasGitHubApp:         cfg.HasGitHubApp,
		initGitHubApp:        cfg.InitGitHubApp,
		getGitService:        cfg.GetGitService,
		updateDefaultProject: cfg.UpdateDefaultProject,
	}
}

// HandleStatus returns the current setup status
func (h *Handler) HandleStatus(c echo.Context) error {
	// Get onboarding progress from database
	progress, err := h.db.GetOnboardingProgress()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get progress: %v", err))
	}

	// Check actual state for reconciliation
	hasPasskey, _ := h.db.HasAnyCredentials()
	hasGitHubApp := h.hasGitHubApp()
	hasInstallation := h.db.HasGitHubInstallation()
	hasAnthropicKey := h.db.HasSecret(db.SecretKeyAnthropicKey)

	// Determine current step based on actual state
	actualStep := DetermineCurrentStep(progress, hasPasskey, hasGitHubApp, hasInstallation, hasAnthropicKey)

	// If database step doesn't match actual state, update it
	if progress.CurrentStep != actualStep && actualStep != "" {
		h.db.AdvanceToStep(actualStep)
		progress.CurrentStep = actualStep
	}

	// Build status response
	status := SetupStatus{
		CurrentStep: progress.CurrentStep,
		Steps:       BuildSteps(progress),
		GitHubOrg:   progress.GetGitHubOrgName(),
		GitHubOrgID: progress.GetGitHubOrgID(),

		// Legacy compatibility fields
		PasskeyRegistered: hasPasskey,
		GitHubAppSet:      hasGitHubApp,
		GitHubAuthMethod:  "none",
		AnthropicKeySet:   hasAnthropicKey,
		SetupComplete:     progress.IsComplete(),
	}

	// Set GitHub auth method and app slug
	if hasGitHubApp {
		status.GitHubAuthMethod = "app"
		if appConfig, err := h.db.GetGitHubAppConfig(); err == nil && appConfig != nil {
			status.GitHubAppSlug = appConfig.AppSlug
		}
	}

	// Check legacy token (for backward compatibility)
	if githubToken, _ := h.db.GetSecret(db.SecretKeyGitHubToken); githubToken != "" {
		status.GitHubTokenSet = true
		if status.GitHubAuthMethod == "none" {
			status.GitHubAuthMethod = "token"
		}
	}

	// Check workspace status
	dataDir := h.getDataDir()
	workspacePath := filepath.Join(dataDir, "repos", "dex-workspace")
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err == nil {
		status.WorkspaceReady = true
		status.WorkspacePath = workspacePath

		// Check if GitHub remote is configured
		cmd := exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = workspacePath
		if output, err := cmd.Output(); err == nil {
			remoteURL := strings.TrimSpace(string(output))
			if remoteURL != "" {
				status.WorkspaceGitHubReady = true
				status.WorkspaceGitHubURL = remoteURL
				status.WorkspaceURL = remoteURL
			}
		}
	}

	// Check access method and permanent URL
	if data, err := os.ReadFile(filepath.Join(dataDir, "access-method")); err == nil {
		status.AccessMethod = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, "permanent-url")); err == nil {
		status.PermanentURL = strings.TrimSpace(string(data))
	}

	return c.JSON(http.StatusOK, status)
}

// HandleAdvanceWelcome advances past the welcome step
func (h *Handler) HandleAdvanceWelcome(c echo.Context) error {
	if err := h.db.SetOnboardingStep(db.OnboardingStepPasskey); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to advance: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"next_step": db.OnboardingStepPasskey,
	})
}

// HandleCompletePasskey marks passkey step as complete
// Called after successful passkey registration
func (h *Handler) HandleCompletePasskey(c echo.Context) error {
	// Verify passkey is actually registered
	hasPasskey, err := h.db.HasAnyCredentials()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to check passkey: %v", err))
	}
	if !hasPasskey {
		return echo.NewHTTPError(http.StatusBadRequest, "no passkey registered")
	}

	if err := h.db.CompletePasskeyStep(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to complete step: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"next_step": db.OnboardingStepGitHubOrg,
	})
}

// HandleSetGitHubOrg sets the GitHub organization name
func (h *Handler) HandleSetGitHubOrg(c echo.Context) error {
	var req struct {
		OrgName string `json:"org_name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.OrgName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "org_name is required")
	}

	// Validate the organization exists
	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	orgInfo, err := ValidateGitHubOrg(ctx, req.OrgName)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Must be an organization, not a user account
	if orgInfo.Type != "Organization" {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("'%s' is a personal account, not an organization. GitHub Apps can only create repositories in organizations. Please create or use a GitHub organization.", req.OrgName))
	}

	// Save the org name and ID
	if err := h.db.SetGitHubOrg(orgInfo.Login, orgInfo.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to save org: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"org_id":    orgInfo.ID,
		"org_login": orgInfo.Login,
		"next_step": db.OnboardingStepGitHubApp,
	})
}

// HandleValidateGitHubOrg validates a GitHub organization without saving
func (h *Handler) HandleValidateGitHubOrg(c echo.Context) error {
	var req struct {
		OrgName string `json:"org_name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	orgInfo, err := ValidateGitHubOrg(ctx, req.OrgName)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"valid":     true,
		"org_id":    orgInfo.ID,
		"org_login": orgInfo.Login,
		"org_name":  orgInfo.Name,
		"org_type":  orgInfo.Type,
	})
}

// HandleCompleteGitHubApp marks the GitHub App creation as complete
func (h *Handler) HandleCompleteGitHubApp(c echo.Context) error {
	// Verify GitHub App is configured
	if !h.hasGitHubApp() {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not configured")
	}

	if err := h.db.CompleteGitHubAppStep(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to complete step: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"next_step": db.OnboardingStepGitHubInstall,
	})
}

// HandleCompleteGitHubInstall marks the GitHub App installation as complete
func (h *Handler) HandleCompleteGitHubInstall(c echo.Context) error {
	// Verify we have at least one installation
	if !h.db.HasGitHubInstallation() {
		return echo.NewHTTPError(http.StatusBadRequest, "no GitHub App installation found")
	}

	if err := h.db.CompleteGitHubInstallStep(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to complete step: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"next_step": db.OnboardingStepAnthropic,
	})
}

// HandleSetAnthropicKey validates and saves an Anthropic API key
func (h *Handler) HandleSetAnthropicKey(c echo.Context) error {
	var req struct {
		Key string `json:"key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate format
	if err := ValidateAnthropicKeyFormat(req.Key); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Validate the key by making a test API call
	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if err := ValidateAnthropicKey(ctx, req.Key); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("key validation failed: %v", err))
	}

	// Save to database
	if err := h.db.SetSecret(db.SecretKeyAnthropicKey, req.Key); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save key")
	}

	// Advance to next step
	if err := h.db.CompleteAnthropicStep(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to complete step: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"message":   "Anthropic API key saved successfully",
		"next_step": db.OnboardingStepComplete,
	})
}

// HandleValidateAnthropicKey validates an Anthropic API key without saving
func (h *Handler) HandleValidateAnthropicKey(c echo.Context) error {
	var req struct {
		Key string `json:"key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := ValidateAnthropicKeyFormat(req.Key); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if err := ValidateAnthropicKey(ctx, req.Key); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"valid": true,
	})
}

// HandleComplete finalizes the setup process
func (h *Handler) HandleComplete(c echo.Context) error {
	dataDir := h.getDataDir()

	// Verify all required steps are done
	hasPasskey, _ := h.db.HasAnyCredentials()
	if !hasPasskey {
		return echo.NewHTTPError(http.StatusBadRequest, "passkey not registered")
	}

	hasGitHubApp := h.hasGitHubApp()
	if !hasGitHubApp {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not configured")
	}

	if !h.db.HasGitHubInstallation() {
		return echo.NewHTTPError(http.StatusBadRequest, "GitHub App not installed on any organization")
	}

	if !h.db.HasSecret(db.SecretKeyAnthropicKey) {
		return echo.NewHTTPError(http.StatusBadRequest, "Anthropic API key not set")
	}

	// Reload toolbelt from database secrets
	if err := h.reloadToolbelt(); err != nil {
		fmt.Printf("Warning: failed to reload toolbelt: %v\n", err)
	}

	// Create workspace repo if it doesn't exist
	workspacePath := filepath.Join(dataDir, "repos", "dex-workspace")
	gitService := h.getGitService()
	if gitService != nil && !gitService.RepoExists(workspacePath) {
		reposDir := filepath.Join(dataDir, "repos")
		if err := os.MkdirAll(reposDir, 0755); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create repos directory: %v", err))
		}

		if err := initWorkspaceRepo(workspacePath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create workspace: %v", err))
		}
	}

	// Initialize GitHub App if needed
	if err := h.initGitHubApp(); err != nil {
		fmt.Printf("Warning: failed to initialize GitHub App: %v\n", err)
	}

	// Create GitHub workspace repository
	var workspaceError string
	githubClient, err := h.getGitHubClient(c.Request().Context(), "")
	if err != nil {
		workspaceError = fmt.Sprintf("failed to get GitHub client: %v", err)
	} else {
		ws := workspace.NewService(githubClient, workspacePath)
		if err := ws.EnsureRemoteExists(c.Request().Context()); err != nil {
			workspaceError = fmt.Sprintf("failed to create GitHub workspace: %v", err)
		}
	}

	// Update default project to point to workspace
	if h.updateDefaultProject != nil {
		if err := h.updateDefaultProject(workspacePath); err != nil {
			fmt.Printf("Warning: failed to update default project: %v\n", err)
		}
	}

	// Mark onboarding as complete
	if err := h.db.CompleteOnboarding(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to complete onboarding: %v", err))
	}

	// Write completion file (legacy support)
	completeFile := filepath.Join(dataDir, "setup-complete")
	if err := os.WriteFile(completeFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		fmt.Printf("Warning: failed to write completion file: %v\n", err)
	}

	result := map[string]any{
		"success":        true,
		"message":        "Setup complete!",
		"workspace_path": workspacePath,
	}

	if workspaceError != "" {
		result["workspace_error"] = workspaceError
	}

	return c.JSON(http.StatusOK, result)
}

// HandleWorkspaceSetup creates or repairs the dex-workspace repository
func (h *Handler) HandleWorkspaceSetup(c echo.Context) error {
	dataDir := h.getDataDir()
	workspacePath := filepath.Join(dataDir, "repos", "dex-workspace")

	// Ensure local repo exists
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); os.IsNotExist(err) {
		reposDir := filepath.Join(dataDir, "repos")
		if err := os.MkdirAll(reposDir, 0755); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create repos directory: %v", err))
		}

		if err := initWorkspaceRepo(workspacePath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create local workspace: %v", err))
		}
	}

	// Try to set up GitHub remote
	var githubURL string
	var githubError string

	if h.hasGitHubApp() {
		if err := h.initGitHubApp(); err != nil {
			githubError = fmt.Sprintf("GitHub App initialization failed: %v", err)
		} else {
			client, err := h.getGitHubClient(c.Request().Context(), "")
			if err != nil {
				githubError = fmt.Sprintf("failed to get GitHub client: %v", err)
			} else {
				ws := workspace.NewService(client, workspacePath)
				if err := ws.EnsureRemoteExists(c.Request().Context()); err != nil {
					githubError = fmt.Sprintf("failed to create GitHub workspace: %v", err)
				} else {
					githubURL = ws.GetRemoteURL()
				}
			}
		}
	} else {
		githubError = "no GitHub App configured"
	}

	result := map[string]any{
		"workspace_path":         workspacePath,
		"workspace_ready":        true,
		"workspace_github_ready": githubURL != "",
	}

	if githubURL != "" {
		result["workspace_github_url"] = githubURL
	}

	if githubError != "" {
		result["workspace_error"] = githubError
	}

	return c.JSON(http.StatusOK, result)
}

// Legacy handlers for backward compatibility

// HandleSetGitHubToken validates and saves a GitHub token (legacy)
func (h *Handler) HandleSetGitHubToken(c echo.Context) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if err := ValidateGitHubTokenFormat(req.Token); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	if err := ValidateGitHubToken(ctx, req.Token); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("token validation failed: %v", err))
	}

	// Save to database
	if err := h.db.SetSecret(db.SecretKeyGitHubToken, req.Token); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "GitHub token saved successfully",
	})
}

// initWorkspaceRepo initializes the dex-workspace git repository
func initWorkspaceRepo(repoPath string) error {
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w\n%s", err, output)
	}

	readme := `# Dex Workspace

This is the default workspace repository for Dex tasks.
`
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("failed to create README: %w", err)
	}

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
