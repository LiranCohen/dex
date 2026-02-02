package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/lirancohen/dex/internal/api/middleware"
	"github.com/lirancohen/dex/internal/api/setup"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/orchestrator"
	"github.com/lirancohen/dex/internal/planning"
	"github.com/lirancohen/dex/internal/quest"
	"github.com/lirancohen/dex/internal/session"
	"github.com/lirancohen/dex/internal/task"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// challengeEntry holds a challenge and its expiry time
type challengeEntry struct {
	Challenge string
	ExpiresAt time.Time
}

// Server represents the API server
type Server struct {
	echo           *echo.Echo
	db             *db.DB
	toolbelt       *toolbelt.Toolbelt
	taskService    *task.Service
	gitService     *git.Service
	sessionManager *session.Manager
	planner        *planning.Planner
	questHandler   *quest.Handler
	githubApp      *github.AppManager
	setupHandler   *setup.Handler
	hub            *websocket.Hub
	addr           string
	certFile       string
	keyFile        string
	tokenConfig    *auth.TokenConfig
	staticDir      string
	reposDir       string
	challenges     map[string]challengeEntry // challenge -> expiry
	challengesMu   sync.RWMutex
	toolbeltMu     sync.RWMutex // Protects toolbelt updates
	githubAppMu    sync.RWMutex // Protects GitHub App manager
}

// Config holds server configuration
type Config struct {
	Addr         string             // e.g., ":8443" or "0.0.0.0:8443"
	CertFile     string             // Path to TLS certificate (optional for dev)
	KeyFile      string             // Path to TLS key (optional for dev)
	TokenConfig  *auth.TokenConfig  // JWT configuration (optional for dev)
	StaticDir    string             // Path to frontend static files (e.g., "./frontend/dist")
	Toolbelt     *toolbelt.Toolbelt // Toolbelt for external service integrations (optional)
	WorktreeBase string             // Base directory for git worktrees (optional)
	ReposDir     string             // Base directory for git repositories (optional)
}

// NewServer creates a new API server
func NewServer(database *db.DB, cfg Config) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(echomw.Logger())
	e.Use(echomw.Recover())
	e.Use(echomw.RequestID())

	// Create WebSocket hub
	hub := websocket.NewHub()
	go hub.Run()

	s := &Server{
		echo:        e,
		db:          database,
		toolbelt:    cfg.Toolbelt,
		taskService: task.NewService(database),
		hub:         hub,
		addr:        cfg.Addr,
		certFile:    cfg.CertFile,
		keyFile:     cfg.KeyFile,
		tokenConfig: cfg.TokenConfig,
		staticDir:   cfg.StaticDir,
		reposDir:    cfg.ReposDir,
		challenges:  make(map[string]challengeEntry),
	}

	// Setup git service if worktree base is configured
	if cfg.WorktreeBase != "" {
		s.gitService = git.NewService(database, cfg.WorktreeBase, cfg.ReposDir)
	}

	// Create scheduler for session management
	scheduler := orchestrator.NewScheduler(database, s.taskService, 25) // Max 25 parallel sessions

	// Create session manager
	sessionMgr := session.NewManager(database, scheduler, "prompts")

	// Wire up git operations if git service is available
	if s.gitService != nil {
		sessionMgr.SetGitOperations(s.gitService.Operations())
	}

	// Wire up GitHub client if toolbelt has it configured
	if cfg.Toolbelt != nil && cfg.Toolbelt.GitHub != nil {
		sessionMgr.SetGitHubClient(cfg.Toolbelt.GitHub)
	}

	// Wire up WebSocket hub for real-time updates
	sessionMgr.SetWebSocketHub(s.hub)

	// Wire up Anthropic client for Ralph loop execution
	if cfg.Toolbelt != nil && cfg.Toolbelt.Anthropic != nil {
		sessionMgr.SetAnthropicClient(cfg.Toolbelt.Anthropic)
	}

	// Create and wire transition handler for hat transitions
	transitionHandler := orchestrator.NewTransitionHandler(database)
	sessionMgr.SetTransitionHandler(transitionHandler)

	s.sessionManager = sessionMgr

	// Create planner for task planning phase
	if cfg.Toolbelt != nil && cfg.Toolbelt.Anthropic != nil {
		s.planner = planning.NewPlanner(database, cfg.Toolbelt.Anthropic, hub)
		s.questHandler = quest.NewHandler(database, cfg.Toolbelt.Anthropic, hub)
		if cfg.Toolbelt.GitHub != nil {
			s.questHandler.SetGitHubClient(cfg.Toolbelt.GitHub)
		}
	}

	// Initialize GitHub App manager if configured (also sets up session manager fetcher)
	if err := s.initGitHubApp(); err != nil {
		// Not an error - GitHub App may not be configured yet during onboarding
		fmt.Printf("GitHub App not initialized at startup: %v\n", err)
	}

	// Initialize setup handler
	s.setupHandler = setup.NewHandler(setup.HandlerConfig{
		DB:         database,
		GetDataDir: s.getDataDir,
		GetToolbelt: func() *toolbelt.Toolbelt {
			s.toolbeltMu.RLock()
			defer s.toolbeltMu.RUnlock()
			return s.toolbelt
		},
		ReloadToolbelt: s.ReloadToolbelt,
		GetGitHubClient: func(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
			return s.GetToolbeltGitHubClient(ctx, login)
		},
		HasGitHubApp:  s.db.HasGitHubApp,
		InitGitHubApp: s.initGitHubApp,
		GetGitService: func() setup.GitService {
			return s.gitService
		},
		UpdateDefaultProject: func(workspacePath string) error {
			project, err := s.db.GetOrCreateDefaultProject()
			if err != nil {
				return err
			}
			if project.RepoPath == "." {
				return s.db.UpdateProject(project.ID, "Dex Workspace", workspacePath, "main")
			}
			return nil
		},
	})

	// Register routes
	s.registerRoutes()

	// Setup static file serving for frontend SPA
	if cfg.StaticDir != "" {
		s.setupStaticServing()
	}

	return s
}

// registerRoutes sets up all API routes
func (s *Server) registerRoutes() {
	// API v1 group
	v1 := s.echo.Group("/api/v1")

	// Public endpoints (no auth required)
	v1.GET("/system/status", s.handleHealthCheck)
	v1.GET("/toolbelt/status", s.handleToolbeltStatus)
	v1.POST("/toolbelt/test", s.handleToolbeltTest)

	// Auth endpoints (public, handles its own auth)
	v1.POST("/auth/challenge", s.handleAuthChallenge)
	v1.POST("/auth/verify", s.handleAuthVerify)
	v1.POST("/auth/refresh", s.handleAuthRefresh)

	// Passkey endpoints (WebAuthn)
	v1.GET("/auth/passkey/status", s.handlePasskeyStatus)
	v1.POST("/auth/passkey/register/begin", s.handlePasskeyRegisterBegin)
	v1.POST("/auth/passkey/register/finish", s.handlePasskeyRegisterFinish)
	v1.POST("/auth/passkey/login/begin", s.handlePasskeyLoginBegin)
	v1.POST("/auth/passkey/login/finish", s.handlePasskeyLoginFinish)

	// Setup endpoints (for onboarding flow - public during initial setup)
	v1.GET("/setup/status", s.setupHandler.HandleStatus)
	v1.POST("/setup/github-token", s.setupHandler.HandleSetGitHubToken) // Legacy
	v1.POST("/setup/anthropic-key", s.setupHandler.HandleSetAnthropicKey)
	v1.POST("/setup/complete", s.setupHandler.HandleComplete)
	v1.POST("/setup/workspace", s.setupHandler.HandleWorkspaceSetup)

	// New onboarding step endpoints
	v1.POST("/setup/steps/welcome", s.setupHandler.HandleAdvanceWelcome)
	v1.POST("/setup/steps/passkey", s.setupHandler.HandleCompletePasskey)
	v1.POST("/setup/steps/github-org", s.setupHandler.HandleSetGitHubOrg)
	v1.POST("/setup/steps/github-app", s.setupHandler.HandleCompleteGitHubApp)
	v1.POST("/setup/steps/github-install", s.setupHandler.HandleCompleteGitHubInstall)
	v1.POST("/setup/steps/anthropic", s.setupHandler.HandleSetAnthropicKey)

	// Validation endpoints
	v1.POST("/setup/validate/github-org", s.setupHandler.HandleValidateGitHubOrg)
	v1.POST("/setup/validate/anthropic-key", s.setupHandler.HandleValidateAnthropicKey)

	// GitHub App endpoints (callbacks must be public for GitHub redirects)
	v1.GET("/setup/github/app/status", s.handleGitHubAppStatus)
	v1.GET("/setup/github/app/manifest", s.handleGitHubAppManifest)
	v1.GET("/setup/github/app/callback", s.handleGitHubAppCallback)
	v1.GET("/setup/github/install/callback", s.handleGitHubInstallCallback)
	v1.GET("/setup/github/installations", s.handleGitHubInstallations)
	v1.POST("/setup/github/installations/sync", s.handleGitHubSyncInstallations)
	v1.DELETE("/setup/github/app", s.handleGitHubDeleteApp)

	// Protected endpoints (require JWT auth)
	// Use middleware if token config is available, otherwise allow all (dev mode)
	protected := v1.Group("")
	if s.tokenConfig != nil {
		protected.Use(middleware.JWTAuth(s.tokenConfig))
	}

	// User info
	protected.GET("/me", s.handleMe)

	// Task endpoints
	protected.GET("/tasks", s.handleListTasks)
	protected.POST("/tasks", s.handleCreateTask)
	protected.GET("/tasks/:id", s.handleGetTask)
	protected.PUT("/tasks/:id", s.handleUpdateTask)
	protected.DELETE("/tasks/:id", s.handleDeleteTask)
	protected.POST("/tasks/:id/start", s.handleStartTask)
	protected.GET("/tasks/:id/worktree/status", s.handleTaskWorktreeStatus)

	// Worktree endpoints
	protected.GET("/worktrees", s.handleListWorktrees)
	protected.DELETE("/worktrees/:task_id", s.handleDeleteWorktree)

	// Project endpoints
	protected.GET("/projects", s.handleListProjects)
	protected.POST("/projects", s.handleCreateProject)
	protected.GET("/projects/:id", s.handleGetProject)
	protected.PUT("/projects/:id", s.handleUpdateProject)
	protected.DELETE("/projects/:id", s.handleDeleteProject)

	// Approval endpoints
	protected.GET("/approvals", s.handleListApprovals)
	protected.GET("/approvals/:id", s.handleGetApproval)
	protected.POST("/approvals/:id/approve", s.handleApproveApproval)
	protected.POST("/approvals/:id/reject", s.handleRejectApproval)

	// Session control endpoints
	protected.POST("/tasks/:id/pause", s.handlePauseTask)
	protected.POST("/tasks/:id/resume", s.handleResumeTask)
	protected.POST("/tasks/:id/cancel", s.handleCancelTask)
	protected.GET("/tasks/:id/logs", s.handleTaskLogs)

	// Planning endpoints
	protected.GET("/tasks/:id/planning", s.handleGetPlanning)
	protected.POST("/tasks/:id/planning/respond", s.handlePlanningRespond)
	protected.POST("/tasks/:id/planning/accept", s.handlePlanningAccept)
	protected.POST("/tasks/:id/planning/skip", s.handlePlanningSkip)

	// Checklist endpoints
	protected.GET("/tasks/:id/checklist", s.handleGetChecklist)
	protected.PUT("/tasks/:id/checklist/items/:itemId", s.handleUpdateChecklistItem)
	protected.POST("/tasks/:id/checklist/accept", s.handleAcceptChecklist)
	protected.POST("/tasks/:id/remediate", s.handleCreateRemediation)

	// Quest endpoints
	protected.GET("/projects/:id/quests", s.handleListQuests)
	protected.POST("/projects/:id/quests", s.handleCreateQuest)
	protected.GET("/quests/:id", s.handleGetQuest)
	protected.DELETE("/quests/:id", s.handleDeleteQuest)
	protected.POST("/quests/:id/messages", s.handleSendQuestMessage)
	protected.POST("/quests/:id/complete", s.handleCompleteQuest)
	protected.POST("/quests/:id/reopen", s.handleReopenQuest)
	protected.PUT("/quests/:id/model", s.handleUpdateQuestModel)
	protected.GET("/quests/:id/tasks", s.handleGetQuestTasks)
	protected.POST("/quests/:id/objectives", s.handleCreateObjective)
	protected.GET("/quests/:id/preflight", s.handleGetPreflightCheck)

	// Quest template endpoints
	protected.GET("/projects/:id/quest-templates", s.handleListQuestTemplates)
	protected.POST("/projects/:id/quest-templates", s.handleCreateQuestTemplate)
	protected.GET("/quest-templates/:id", s.handleGetQuestTemplate)
	protected.PUT("/quest-templates/:id", s.handleUpdateQuestTemplate)
	protected.DELETE("/quest-templates/:id", s.handleDeleteQuestTemplate)

	// Session management endpoints
	protected.GET("/sessions", s.handleListSessions)
	protected.GET("/sessions/:id", s.handleGetSession)
	protected.POST("/sessions/:id/kill", s.handleKillSession)

	// Activity endpoints
	protected.GET("/sessions/:id/activity", s.handleGetSessionActivity)
	protected.GET("/tasks/:id/activity", s.handleGetTaskActivity)

	// WebSocket endpoint for real-time updates
	protected.GET("/ws", func(c echo.Context) error {
		return websocket.ServeWS(s.hub, c)
	})
}

// handleHealthCheck returns system health status
func (s *Server) handleHealthCheck(c echo.Context) error {
	status := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "0.1.0-dev",
		"database":  "connected",
	}

	// Verify database connection
	if err := s.db.Ping(); err != nil {
		status["status"] = "unhealthy"
		status["database"] = "disconnected"
		status["error"] = err.Error()
		return c.JSON(http.StatusServiceUnavailable, status)
	}

	return c.JSON(http.StatusOK, status)
}

// handleAuthChallenge generates and returns a random challenge for authentication
func (s *Server) handleAuthChallenge(c echo.Context) error {
	// Generate 32 random bytes
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate challenge")
	}

	challenge := hex.EncodeToString(challengeBytes)
	expiresAt := time.Now().Add(5 * time.Minute)

	// Store challenge with TTL
	s.challengesMu.Lock()
	s.challenges[challenge] = challengeEntry{
		Challenge: challenge,
		ExpiresAt: expiresAt,
	}
	s.challengesMu.Unlock()

	return c.JSON(http.StatusOK, map[string]any{
		"challenge":  challenge,
		"expires_in": 300,
	})
}

// handleAuthVerify verifies a signed challenge and returns a JWT
func (s *Server) handleAuthVerify(c echo.Context) error {
	var req struct {
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
		Challenge string `json:"challenge"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.PublicKey == "" || req.Signature == "" || req.Challenge == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "public_key, signature, and challenge are required")
	}

	// Validate challenge exists and not expired
	s.challengesMu.Lock()
	entry, exists := s.challenges[req.Challenge]
	if exists {
		delete(s.challenges, req.Challenge) // One-time use
	}
	s.challengesMu.Unlock()

	if !exists {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired challenge")
	}
	if time.Now().After(entry.ExpiresAt) {
		return echo.NewHTTPError(http.StatusUnauthorized, "challenge expired")
	}

	// Decode public key and signature from hex
	publicKey, err := hex.DecodeString(req.PublicKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid public_key format")
	}
	signature, err := hex.DecodeString(req.Signature)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid signature format")
	}
	challengeBytes, err := hex.DecodeString(req.Challenge)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid challenge format")
	}

	// Verify signature
	if !auth.Verify(challengeBytes, signature, publicKey) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}

	// Get or create user
	user, _, err := s.db.GetOrCreateUser(req.PublicKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get or create user")
	}

	// Update last login
	_ = s.db.UpdateUserLastLogin(user.ID)

	// Generate JWT
	if s.tokenConfig == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "token configuration not available")
	}
	token, err := auth.GenerateToken(user.ID, s.tokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"user_id": user.ID,
	})
}

// handleAuthRefresh refreshes an existing JWT token
func (s *Server) handleAuthRefresh(c echo.Context) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}

	if s.tokenConfig == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "token configuration not available")
	}

	newToken, err := auth.RefreshToken(req.Token, s.tokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "failed to refresh token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"token": newToken,
	})
}

// handleToolbeltStatus returns the configuration status of all toolbelt services
func (s *Server) handleToolbeltStatus(c echo.Context) error {
	if s.toolbelt == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"configured": false,
			"services":   []toolbelt.ServiceStatus{},
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"configured": true,
		"services":   s.toolbelt.Status(),
	})
}

// handleToolbeltTest tests all configured toolbelt service connections
func (s *Server) handleToolbeltTest(c echo.Context) error {
	if s.toolbelt == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"tested":  false,
			"message": "toolbelt not configured",
			"results": []toolbelt.TestResult{},
		})
	}

	results := s.toolbelt.TestConnections(c.Request().Context())

	// Count successes
	successes := 0
	for _, r := range results {
		if r.Success {
			successes++
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"tested":     true,
		"total":      len(results),
		"successful": successes,
		"failed":     len(results) - successes,
		"results":    results,
	})
}

// handleMe returns the authenticated user info
func (s *Server) handleMe(c echo.Context) error {
	userID := middleware.GetUserID(c)

	user, err := s.db.GetUserByID(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get user")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":            user.ID,
		"created_at":    user.CreatedAt,
		"last_login_at": user.LastLoginAt,
	})
}

// handleListTasks returns tasks with optional filters
func (s *Server) handleListTasks(c echo.Context) error {
	filters := task.ListFilters{
		ProjectID: c.QueryParam("project_id"),
		Status:    c.QueryParam("status"),
	}

	tasks, err := s.taskService.List(filters)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert to response format with proper null handling
	taskResponses := make([]TaskResponse, len(tasks))
	for i, t := range tasks {
		taskResponses[i] = toTaskResponse(t)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"tasks": taskResponses,
		"count": len(tasks),
	})
}

// handleCreateTask creates a new task
func (s *Server) handleCreateTask(c echo.Context) error {
	var req struct {
		ProjectID   any    `json:"project_id"` // Accept string or number
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Priority    int    `json:"priority"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Check for skip_planning query parameter
	skipPlanning := c.QueryParam("skip_planning") == "true"

	// Get or create default project for single-user mode
	projectID := ""
	if req.ProjectID != nil {
		projectID = fmt.Sprintf("%v", req.ProjectID)
	}

	if projectID == "" || projectID == "0" || projectID == "1" {
		// Use or create default project
		project, err := s.db.GetOrCreateDefaultProject()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get default project")
		}
		projectID = project.ID
	}

	t, err := s.taskService.Create(projectID, req.Title, req.Type, req.Priority)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Update description if provided
	if req.Description != "" {
		updates := task.TaskUpdates{Description: &req.Description}
		t, err = s.taskService.Update(t.ID, updates)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to set description")
		}
	}

	// Start planning phase if planner is available and skip_planning is not set
	if s.planner != nil && !skipPlanning {
		// Get the prompt for planning (use description if available, otherwise title)
		planningPrompt := req.Description
		if planningPrompt == "" {
			planningPrompt = req.Title
		}

		// Transition task to planning status
		if err := s.taskService.UpdateStatus(t.ID, db.TaskStatusPlanning); err != nil {
			// Log warning but don't fail task creation
			fmt.Printf("warning: failed to transition task to planning: %v\n", err)
		} else {
			// Start planning session
			_, err := s.planner.StartPlanning(c.Request().Context(), t.ID, planningPrompt)
			if err != nil {
				// Log warning but don't fail task creation
				fmt.Printf("warning: failed to start planning: %v\n", err)
				// Transition back to pending if planning fails
				s.taskService.UpdateStatus(t.ID, db.TaskStatusPending)
			} else {
				// Update task status in response
				t.Status = db.TaskStatusPlanning
			}
		}
	}

	return c.JSON(http.StatusCreated, toTaskResponse(t))
}

// handleGetTask returns a single task by ID
func (s *Server) handleGetTask(c echo.Context) error {
	id := c.Param("id")

	t, err := s.taskService.Get(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, toTaskResponse(t))
}

// handleUpdateTask updates a task
func (s *Server) handleUpdateTask(c echo.Context) error {
	id := c.Param("id")

	var updates task.TaskUpdates
	if err := c.Bind(&updates); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updated, err := s.taskService.Update(id, updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, toTaskResponse(updated))
}

// handleDeleteTask removes a task
func (s *Server) handleDeleteTask(c echo.Context) error {
	id := c.Param("id")

	if err := s.taskService.Delete(id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// startTaskResult contains the result of starting a task
type startTaskResult struct {
	Task         *db.Task
	WorktreePath string
	SessionID    string
}

// startTaskInternal starts a task by ID with an optional base branch
// This is a shared helper used by handleStartTask and auto-start logic
func (s *Server) startTaskInternal(ctx context.Context, taskID string, baseBranch string) (*startTaskResult, error) {
	// Get the task first
	t, err := s.taskService.Get(taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}

	// Get the project to find repo_path
	project, err := s.db.GetProjectByID(t.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found")
	}

	projectPath := project.RepoPath
	if baseBranch == "" {
		baseBranch = project.DefaultBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
	}

	// Check if already has a worktree
	if t.WorktreePath.Valid && t.WorktreePath.String != "" {
		return nil, fmt.Errorf("task already has a worktree")
	}

	// Check if project has a valid git repository that's appropriate for this project
	// For new projects (creating repos), we start without a worktree
	var worktreePath string
	hasGitRepo := s.isValidGitRepo(projectPath)
	isValidProjectPath := s.isValidProjectPath(projectPath)

	if hasGitRepo && isValidProjectPath && s.gitService != nil {
		// Project has a valid git repo - create worktree as usual
		worktreePath, err = s.gitService.SetupTaskWorktree(projectPath, taskID, baseBranch)
		if err != nil {
			return nil, fmt.Errorf("failed to create worktree: %w", err)
		}
	} else {
		// No valid git repo yet - create a task-specific directory
		// This happens for new projects or when project path is invalid/system path
		if isValidProjectPath && projectPath != "" {
			// Use the project path - the objective will create the repo here
			worktreePath = projectPath
			if err := os.MkdirAll(worktreePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create project directory: %w", err)
			}
		} else {
			// Project path is empty or invalid (e.g., system directory)
			// Create a task-specific directory in the configured repos directory
			if s.reposDir != "" {
				worktreePath = filepath.Join(s.reposDir, "task-"+taskID)
			} else {
				worktreePath = filepath.Join(os.TempDir(), "dex-task-"+taskID)
			}
			if err := os.MkdirAll(worktreePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create task directory: %w", err)
			}
		}
	}

	// Transition through ready to running status
	// First: pending -> ready
	if t.Status == "pending" {
		if err := s.taskService.UpdateStatus(taskID, "ready"); err != nil {
			if hasGitRepo && s.gitService != nil {
				_ = s.gitService.CleanupTaskWorktree(projectPath, taskID, true)
			}
			return nil, fmt.Errorf("failed to transition to ready: %w", err)
		}
	}
	// Then: ready -> running
	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		if hasGitRepo && s.gitService != nil {
			_ = s.gitService.CleanupTaskWorktree(projectPath, taskID, true)
		}
		return nil, fmt.Errorf("failed to transition to running: %w", err)
	}

	// Create and start a session for this task
	hat := "creator" // Default hat - could be determined from task type
	if t.Hat.Valid && t.Hat.String != "" {
		hat = t.Hat.String
	}

	session, err := s.sessionManager.CreateSession(taskID, hat, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Start the session (runs Ralph loop in background)
	// Use background context since the session should live beyond the HTTP request
	if err := s.sessionManager.Start(ctx, session.ID); err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	// Fetch updated task
	updated, _ := s.taskService.Get(taskID)

	return &startTaskResult{
		Task:         updated,
		WorktreePath: worktreePath,
		SessionID:    session.ID,
	}, nil
}

// isValidGitRepo checks if the given path is a valid git repository
func (s *Server) isValidGitRepo(path string) bool {
	if path == "" {
		return false
	}
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isValidProjectPath checks if the given path is appropriate for use as a project directory
// It returns false for system directories, dex installation directories, and other invalid paths
func (s *Server) isValidProjectPath(path string) bool {
	if path == "" || path == "." || path == ".." {
		return false
	}

	// Normalize the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// System directories that should never be used (including subdirectories)
	systemPrefixes := []string{
		"/usr/",
		"/bin/",
		"/sbin/",
		"/lib/",
		"/etc/",
	}

	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(absPath, prefix) {
			return false
		}
	}

	// Check if this looks like the dex installation by checking for cmd/main.go
	// This catches /opt/dex or any other location where dex source is installed
	dexMarker := filepath.Join(absPath, "cmd", "main.go")
	if _, err := os.Stat(dexMarker); err == nil {
		// This is likely the dex source directory
		return false
	}

	return true
}

// handleStartTask transitions a task to running and sets up its worktree
func (s *Server) handleStartTask(c echo.Context) error {
	taskID := c.Param("id")

	var req struct {
		BaseBranch string `json:"base_branch"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	result, err := s.startTaskInternal(context.Background(), taskID, req.BaseBranch)
	if err != nil {
		// Determine appropriate HTTP status based on error
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if strings.Contains(err.Error(), "already has a worktree") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		if strings.Contains(err.Error(), "not configured") {
			return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"task":          result.Task,
		"worktree_path": result.WorktreePath,
		"session_id":    result.SessionID,
	})
}

// handleTaskWorktreeStatus returns the git status of a task's worktree
func (s *Server) handleTaskWorktreeStatus(c echo.Context) error {
	taskID := c.Param("id")

	if s.gitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	status, err := s.gitService.GetTaskWorktreeStatus(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, status)
}

// handleListWorktrees returns all worktrees for a project
func (s *Server) handleListWorktrees(c echo.Context) error {
	projectPath := c.QueryParam("project_path")
	if projectPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "project_path is required")
	}

	if s.gitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	worktrees, err := s.gitService.ListWorktrees(projectPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"worktrees": worktrees,
		"count":     len(worktrees),
	})
}

// handleDeleteWorktree removes a task's worktree
func (s *Server) handleDeleteWorktree(c echo.Context) error {
	taskID := c.Param("task_id")
	projectPath := c.QueryParam("project_path")
	cleanupBranch := c.QueryParam("cleanup_branch") == "true"

	if projectPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "project_path is required")
	}

	if s.gitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	if err := s.gitService.CleanupTaskWorktree(projectPath, taskID, cleanupBranch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// handleListProjects returns all projects
func (s *Server) handleListProjects(c echo.Context) error {
	projects, err := s.db.ListProjects()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"projects": projects,
		"count":    len(projects),
	})
}

// handleCreateProject creates a new project
func (s *Server) handleCreateProject(c echo.Context) error {
	var req struct {
		Name string `json:"name"`

		// Option 1: Use existing repo
		RepoPath string `json:"repo_path,omitempty"`

		// Option 2: Create new repo
		CreateRepo bool `json:"create_repo,omitempty"`

		// Option 3: Clone from URL
		CloneURL string `json:"clone_url,omitempty"`

		// GitHub options (when create_repo=true)
		GitHubCreate  bool `json:"github_create,omitempty"`
		GitHubPrivate bool `json:"github_private,omitempty"`

		Description string `json:"description,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	var repoPath string

	if req.CreateRepo {
		// Create new local repository
		if s.gitService == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
		}

		var err error
		repoPath, err = s.gitService.CreateRepo(git.CreateOptions{
			Name:          req.Name,
			Description:   req.Description,
			InitialCommit: true,
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create repository: %v", err))
		}

		// Optionally create on GitHub
		if req.GitHubCreate && s.toolbelt != nil && s.toolbelt.GitHub != nil {
			ghRepo, err := s.toolbelt.GitHub.CreateRepo(c.Request().Context(), toolbelt.CreateRepoOptions{
				Name:        req.Name,
				Description: req.Description,
				Private:     req.GitHubPrivate,
			})
			if err != nil {
				// Log but don't fail - local repo was created successfully
				fmt.Printf("warning: failed to create GitHub repo: %v\n", err)
			} else if ghRepo != nil && ghRepo.CloneURL != nil && *ghRepo.CloneURL != "" {
				if err := s.gitService.SetRepoRemote(repoPath, *ghRepo.CloneURL); err != nil {
					fmt.Printf("warning: failed to set remote: %v\n", err)
				}
			}
		}
	} else if req.CloneURL != "" {
		// Clone from URL
		if s.gitService == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
		}

		var err error
		repoPath, err = s.gitService.CloneRepo(req.CloneURL, req.Name)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to clone repository: %v", err))
		}
	} else {
		// Use existing repo path
		if req.RepoPath == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "repo_path is required when not creating or cloning a repo")
		}
		repoPath = req.RepoPath
	}

	project, err := s.db.CreateProject(req.Name, repoPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, project)
}

// handleGetProject returns a single project by ID
func (s *Server) handleGetProject(c echo.Context) error {
	id := c.Param("id")

	project, err := s.db.GetProjectByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	return c.JSON(http.StatusOK, project)
}

// handleUpdateProject updates a project
func (s *Server) handleUpdateProject(c echo.Context) error {
	id := c.Param("id")

	// First fetch the existing project to get current values
	existing, err := s.db.GetProjectByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	var req struct {
		Name          *string             `json:"name"`
		RepoPath      *string             `json:"repo_path"`
		DefaultBranch *string             `json:"default_branch"`
		GitHubOwner   *string             `json:"github_owner"`
		GitHubRepo    *string             `json:"github_repo"`
		Services      *db.ProjectServices `json:"services"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Update basic fields (use existing values if not provided)
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	repoPath := existing.RepoPath
	if req.RepoPath != nil {
		repoPath = *req.RepoPath
	}
	defaultBranch := existing.DefaultBranch
	if req.DefaultBranch != nil {
		defaultBranch = *req.DefaultBranch
	}

	if err := s.db.UpdateProject(id, name, repoPath, defaultBranch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Update GitHub info if provided
	if req.GitHubOwner != nil || req.GitHubRepo != nil {
		owner := ""
		repo := ""
		if existing.GitHubOwner.Valid {
			owner = existing.GitHubOwner.String
		}
		if existing.GitHubRepo.Valid {
			repo = existing.GitHubRepo.String
		}
		if req.GitHubOwner != nil {
			owner = *req.GitHubOwner
		}
		if req.GitHubRepo != nil {
			repo = *req.GitHubRepo
		}
		if err := s.db.UpdateProjectGitHub(id, owner, repo); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	// Update services if provided
	if req.Services != nil {
		if err := s.db.UpdateProjectServices(id, *req.Services); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	// Return updated project
	updated, err := s.db.GetProjectByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, updated)
}

// handleDeleteProject removes a project
func (s *Server) handleDeleteProject(c echo.Context) error {
	id := c.Param("id")

	if err := s.db.DeleteProject(id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// TaskResponse is the JSON response format for tasks
// This properly handles sql.Null* types for JSON serialization
type TaskResponse struct {
	ID                string   `json:"ID"`
	ProjectID         string   `json:"ProjectID"`
	QuestID           *string  `json:"QuestID"`
	GitHubIssueNumber *int64   `json:"GitHubIssueNumber"`
	Title             string   `json:"Title"`
	Description       *string  `json:"Description"`
	ParentID          *string  `json:"ParentID"`
	Type              string   `json:"Type"`
	Hat               *string  `json:"Hat"`
	Priority          int      `json:"Priority"`
	AutonomyLevel     int      `json:"AutonomyLevel"`
	Status            string   `json:"Status"`
	BaseBranch        string   `json:"BaseBranch"`
	WorktreePath      *string  `json:"WorktreePath"`
	BranchName        *string  `json:"BranchName"`
	PRNumber          *int64   `json:"PRNumber"`
	TokenBudget       *int64   `json:"TokenBudget"`
	TokenUsed         int64    `json:"TokenUsed"`
	TimeBudgetMin     *int64   `json:"TimeBudgetMin"`
	TimeUsedMin       int64    `json:"TimeUsedMin"`
	DollarBudget      *float64 `json:"DollarBudget"`
	DollarUsed        float64  `json:"DollarUsed"`
	CreatedAt         string   `json:"CreatedAt"`
	StartedAt         *string  `json:"StartedAt"`
	CompletedAt       *string  `json:"CompletedAt"`
}

// toTaskResponse converts a db.Task to TaskResponse for clean JSON
func toTaskResponse(t *db.Task) TaskResponse {
	resp := TaskResponse{
		ID:            t.ID,
		ProjectID:     t.ProjectID,
		Title:         t.Title,
		Type:          t.Type,
		Priority:      t.Priority,
		AutonomyLevel: t.AutonomyLevel,
		Status:        t.Status,
		BaseBranch:    t.BaseBranch,
		TokenUsed:     t.TokenUsed,
		TimeUsedMin:   t.TimeUsedMin,
		DollarUsed:    t.DollarUsed,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
	}
	if t.QuestID.Valid {
		resp.QuestID = &t.QuestID.String
	}
	if t.GitHubIssueNumber.Valid {
		resp.GitHubIssueNumber = &t.GitHubIssueNumber.Int64
	}
	if t.Description.Valid {
		resp.Description = &t.Description.String
	}
	if t.ParentID.Valid {
		resp.ParentID = &t.ParentID.String
	}
	if t.Hat.Valid {
		resp.Hat = &t.Hat.String
	}
	if t.WorktreePath.Valid {
		resp.WorktreePath = &t.WorktreePath.String
	}
	if t.BranchName.Valid {
		resp.BranchName = &t.BranchName.String
	}
	if t.PRNumber.Valid {
		resp.PRNumber = &t.PRNumber.Int64
	}
	if t.TokenBudget.Valid {
		resp.TokenBudget = &t.TokenBudget.Int64
	}
	if t.TimeBudgetMin.Valid {
		resp.TimeBudgetMin = &t.TimeBudgetMin.Int64
	}
	if t.DollarBudget.Valid {
		resp.DollarBudget = &t.DollarBudget.Float64
	}
	if t.StartedAt.Valid {
		s := t.StartedAt.Time.Format(time.RFC3339)
		resp.StartedAt = &s
	}
	if t.CompletedAt.Valid {
		s := t.CompletedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &s
	}
	return resp
}

// ApprovalResponse is the JSON response format for approvals
type ApprovalResponse struct {
	ID          string          `json:"id"`
	TaskID      *string         `json:"task_id,omitempty"`
	SessionID   *string         `json:"session_id,omitempty"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description *string         `json:"description,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
}

// toApprovalResponse converts a db.Approval to ApprovalResponse for clean JSON
func toApprovalResponse(a *db.Approval) ApprovalResponse {
	resp := ApprovalResponse{
		ID:        a.ID,
		Type:      a.Type,
		Title:     a.Title,
		Data:      a.Data,
		Status:    a.Status,
		CreatedAt: a.CreatedAt,
	}
	if a.TaskID.Valid {
		resp.TaskID = &a.TaskID.String
	}
	if a.SessionID.Valid {
		resp.SessionID = &a.SessionID.String
	}
	if a.Description.Valid {
		resp.Description = &a.Description.String
	}
	if a.ResolvedAt.Valid {
		resp.ResolvedAt = &a.ResolvedAt.Time
	}
	return resp
}

// SessionResponse is the JSON response format for sessions
type SessionResponse struct {
	ID             string   `json:"id"`
	TaskID         string   `json:"task_id"`
	Hat            string   `json:"hat"`
	State          string   `json:"state"`
	WorktreePath   string   `json:"worktree_path"`
	IterationCount int      `json:"iteration_count"`
	MaxIterations  int      `json:"max_iterations"`
	InputTokens    int64    `json:"input_tokens"`
	OutputTokens   int64    `json:"output_tokens"`
	TokensUsed     int64    `json:"tokens_used"`     // Derived: input + output
	TokensBudget   *int64   `json:"tokens_budget,omitempty"`
	DollarsUsed    float64  `json:"dollars_used"`    // Derived from tokens and rates
	DollarsBudget  *float64 `json:"dollars_budget,omitempty"`
	StartedAt      string   `json:"started_at,omitempty"`
	LastActivity   string   `json:"last_activity,omitempty"`
}

// toSessionResponse converts an ActiveSession to SessionResponse for clean JSON
func toSessionResponse(s *session.ActiveSession) SessionResponse {
	resp := SessionResponse{
		ID:             s.ID,
		TaskID:         s.TaskID,
		Hat:            s.Hat,
		State:          string(s.State),
		WorktreePath:   s.WorktreePath,
		IterationCount: s.IterationCount,
		MaxIterations:  s.MaxIterations,
		InputTokens:    s.InputTokens,
		OutputTokens:   s.OutputTokens,
		TokensUsed:     s.TotalTokens(),
		TokensBudget:   s.TokensBudget,
		DollarsUsed:    s.Cost(),
		DollarsBudget:  s.DollarsBudget,
	}
	if !s.StartedAt.IsZero() {
		resp.StartedAt = s.StartedAt.Format(time.RFC3339)
	}
	if !s.LastActivity.IsZero() {
		resp.LastActivity = s.LastActivity.Format(time.RFC3339)
	}
	return resp
}

// handleListApprovals returns approvals with optional filters
func (s *Server) handleListApprovals(c echo.Context) error {
	status := c.QueryParam("status")
	taskID := c.QueryParam("task_id")

	var approvals []*db.Approval
	var err error

	switch {
	case taskID != "":
		approvals, err = s.db.ListApprovalsByTask(taskID)
	case status != "":
		approvals, err = s.db.ListApprovalsByStatus(status)
	default:
		approvals, err = s.db.ListPendingApprovals()
	}

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert to response format
	responses := make([]ApprovalResponse, len(approvals))
	for i, a := range approvals {
		responses[i] = toApprovalResponse(a)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"approvals": responses,
		"count":     len(responses),
	})
}

// handleGetApproval returns a single approval by ID
func (s *Server) handleGetApproval(c echo.Context) error {
	id := c.Param("id")

	approval, err := s.db.GetApprovalByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if approval == nil {
		return echo.NewHTTPError(http.StatusNotFound, "approval not found")
	}

	return c.JSON(http.StatusOK, toApprovalResponse(approval))
}

// handleApproveApproval marks an approval as approved
func (s *Server) handleApproveApproval(c echo.Context) error {
	id := c.Param("id")

	if err := s.db.ApproveApproval(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, "approval not found")
		}
		if strings.Contains(err.Error(), "already resolved") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast WebSocket event
	s.hub.Broadcast(websocket.Message{
		Type: "approval.resolved",
		Payload: map[string]any{
			"id":     id,
			"status": "approved",
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "approval approved",
		"id":      id,
	})
}

// handleRejectApproval marks an approval as rejected
func (s *Server) handleRejectApproval(c echo.Context) error {
	id := c.Param("id")

	if err := s.db.RejectApproval(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, "approval not found")
		}
		if strings.Contains(err.Error(), "already resolved") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast WebSocket event
	s.hub.Broadcast(websocket.Message{
		Type: "approval.resolved",
		Payload: map[string]any{
			"id":     id,
			"status": "rejected",
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "approval rejected",
		"id":      id,
	})
}

// handlePauseTask pauses the running session for a task
func (s *Server) handlePauseTask(c echo.Context) error {
	taskID := c.Param("id")

	// Find the session for this task
	sess := s.sessionManager.GetByTask(taskID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active session for task")
	}

	// Pause the session
	if err := s.sessionManager.Pause(sess.ID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Update task status in database so frontend sees the change
	if err := s.taskService.UpdateStatus(taskID, "paused"); err != nil {
		fmt.Printf("warning: failed to update task status to paused: %v\n", err)
	}

	// Broadcast WebSocket event
	s.hub.Broadcast(websocket.Message{
		Type: "task.paused",
		Payload: map[string]any{
			"task_id":    taskID,
			"session_id": sess.ID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "task paused",
		"task_id": taskID,
	})
}

// handleResumeTask resumes a paused session for a task
func (s *Server) handleResumeTask(c echo.Context) error {
	taskID := c.Param("id")

	// Find the session for this task
	sess := s.sessionManager.GetByTask(taskID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active session for task")
	}

	// Check that session is paused
	if sess.State != session.StatePaused {
		return echo.NewHTTPError(http.StatusBadRequest, "session is not paused")
	}

	// Resume by starting the session again
	if err := s.sessionManager.Start(c.Request().Context(), sess.ID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Update task status in database so frontend sees the change
	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		fmt.Printf("warning: failed to update task status to running: %v\n", err)
	}

	// Broadcast WebSocket event
	s.hub.Broadcast(websocket.Message{
		Type: "task.resumed",
		Payload: map[string]any{
			"task_id":    taskID,
			"session_id": sess.ID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "task resumed",
		"task_id": taskID,
	})
}

// handleCancelTask cancels a task and its session
func (s *Server) handleCancelTask(c echo.Context) error {
	taskID := c.Param("id")

	// Find the session for this task
	sess := s.sessionManager.GetByTask(taskID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active session for task")
	}

	// Stop the session
	if err := s.sessionManager.Stop(sess.ID); err != nil {
		// Session might not be running, but we still want to cancel the task
		// Log the error but continue
		fmt.Printf("warning: failed to stop session %s: %v\n", sess.ID, err)
	}

	// Update task status to cancelled
	if err := s.taskService.UpdateStatus(taskID, "cancelled"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast WebSocket event
	s.hub.Broadcast(websocket.Message{
		Type: "task.cancelled",
		Payload: map[string]any{
			"task_id":    taskID,
			"session_id": sess.ID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "task cancelled",
		"task_id": taskID,
	})
}

// handleTaskLogs returns logs for a task's session (placeholder for now)
func (s *Server) handleTaskLogs(c echo.Context) error {
	taskID := c.Param("id")

	// Verify task exists
	_, err := s.taskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Placeholder response - real implementation will need session log storage
	return c.JSON(http.StatusOK, map[string]any{
		"logs":    []any{},
		"message": "log streaming not yet implemented",
		"task_id": taskID,
	})
}

// handleListSessions returns all active sessions
func (s *Server) handleListSessions(c echo.Context) error {
	sessions := s.sessionManager.List()

	// Convert to response format
	responses := make([]SessionResponse, len(sessions))
	for i, sess := range sessions {
		responses[i] = toSessionResponse(sess)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"sessions": responses,
		"count":    len(responses),
	})
}

// handleGetSession returns a single session by ID
func (s *Server) handleGetSession(c echo.Context) error {
	sessionID := c.Param("id")

	sess := s.sessionManager.Get(sessionID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	return c.JSON(http.StatusOK, toSessionResponse(sess))
}

// handleKillSession forcefully stops a session
func (s *Server) handleKillSession(c echo.Context) error {
	sessionID := c.Param("id")

	// Verify session exists
	sess := s.sessionManager.Get(sessionID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	// Stop the session
	if err := s.sessionManager.Stop(sessionID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Broadcast WebSocket event
	s.hub.Broadcast(websocket.Message{
		Type: "session.killed",
		Payload: map[string]any{
			"session_id": sessionID,
			"task_id":    sess.TaskID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message":    "session killed",
		"session_id": sessionID,
	})
}

// ActivityResponse is the JSON response format for session activity
type ActivityResponse struct {
	ID           string  `json:"id"`
	SessionID    string  `json:"session_id"`
	Iteration    int     `json:"iteration"`
	EventType    string  `json:"event_type"`
	Hat          *string `json:"hat,omitempty"`
	Content      *string `json:"content,omitempty"`
	TokensInput  *int64  `json:"tokens_input,omitempty"`
	TokensOutput *int64  `json:"tokens_output,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// toActivityResponse converts a db.SessionActivity to ActivityResponse
func toActivityResponse(a *db.SessionActivity) ActivityResponse {
	resp := ActivityResponse{
		ID:        a.ID,
		SessionID: a.SessionID,
		Iteration: a.Iteration,
		EventType: a.EventType,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
	if a.Hat.Valid {
		resp.Hat = &a.Hat.String
	}
	if a.Content.Valid {
		resp.Content = &a.Content.String
	}
	if a.TokensInput.Valid {
		resp.TokensInput = &a.TokensInput.Int64
	}
	if a.TokensOutput.Valid {
		resp.TokensOutput = &a.TokensOutput.Int64
	}
	return resp
}

// handleGetSessionActivity returns all activity for a session
func (s *Server) handleGetSessionActivity(c echo.Context) error {
	sessionID := c.Param("id")

	// Verify session exists
	sess, err := s.db.GetSessionByID(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	// Get activity
	activities, err := s.db.ListSessionActivity(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get summary
	summary, err := s.db.GetSessionActivitySummary(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert to response format
	responses := make([]ActivityResponse, len(activities))
	for i, a := range activities {
		responses[i] = toActivityResponse(a)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"activity": responses,
		"summary":  summary,
	})
}

// handleGetTaskActivity returns all activity for all sessions of a task
func (s *Server) handleGetTaskActivity(c echo.Context) error {
	taskID := c.Param("id")

	// Verify task exists
	task, err := s.taskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if task == nil {
		return echo.NewHTTPError(http.StatusNotFound, "task not found")
	}

	// Get all activity for this task's sessions
	activities, err := s.db.ListTaskActivity(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get sessions for this task
	sessions, err := s.db.ListSessionsByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Calculate total tokens across all sessions
	var totalTokens int64
	var totalIterations int
	for _, sess := range sessions {
		totalTokens += sess.TotalTokens()
		if sess.IterationCount > totalIterations {
			totalIterations = sess.IterationCount
		}
	}

	// Convert to response format
	responses := make([]ActivityResponse, len(activities))
	for i, a := range activities {
		responses[i] = toActivityResponse(a)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"activity": responses,
		"summary": map[string]any{
			"total_sessions":   len(sessions),
			"total_iterations": totalIterations,
			"total_tokens":     totalTokens,
		},
	})
}

// setupStaticServing configures static file serving for the frontend SPA
// It serves files from staticDir and falls back to index.html for SPA routing
func (s *Server) setupStaticServing() {
	// Serve static files from the staticDir
	s.echo.Static("/assets", s.staticDir+"/assets")

	// Serve other static files (favicon, etc.) from root
	s.echo.File("/vite.svg", s.staticDir+"/vite.svg")

	// SPA fallback: serve index.html for all non-API, non-asset routes
	// This must be registered AFTER API routes
	s.echo.GET("/*", func(c echo.Context) error {
		path := c.Request().URL.Path

		// Don't serve index.html for API routes (already handled)
		if len(path) >= 4 && path[:4] == "/api" {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}

		// Serve index.html for all other routes (SPA client-side routing)
		return c.File(s.staticDir + "/index.html")
	})
}

// Start begins serving HTTP/HTTPS requests
func (s *Server) Start() error {
	if s.certFile != "" && s.keyFile != "" {
		// HTTPS mode (for Tailscale)
		fmt.Printf("Starting HTTPS server on %s\n", s.addr)
		return s.echo.StartTLS(s.addr, s.certFile, s.keyFile)
	}

	// HTTP mode (for local development)
	fmt.Printf("Starting HTTP server on %s\n", s.addr)
	return s.echo.Start(s.addr)
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

// getDataDir returns the data directory path
func (s *Server) getDataDir() string {
	if dir := os.Getenv("DEX_DATA_DIR"); dir != "" {
		return dir
	}
	return "/opt/dex"
}

// GetHub returns the WebSocket hub for broadcasting events
func (s *Server) GetHub() *websocket.Hub {
	return s.hub
}

// ReloadToolbelt reloads the toolbelt from database secrets and updates the session manager
// This is called after setup completes when API keys are first entered
func (s *Server) ReloadToolbelt() error {
	// First try to migrate any existing secrets from file to database
	dataDir := s.getDataDir()
	if count, err := s.db.MigrateSecretsFromFile(dataDir); err != nil {
		fmt.Printf("ReloadToolbelt: failed to migrate secrets from file: %v\n", err)
	} else if count > 0 {
		fmt.Printf("ReloadToolbelt: migrated %d secrets from file to database\n", count)
	}

	// Load secrets from database
	secrets, err := s.db.GetAllSecrets()
	if err != nil {
		return fmt.Errorf("failed to load secrets from database: %w", err)
	}

	fmt.Printf("ReloadToolbelt: loading from database (%d secrets)\n", len(secrets))

	// Build toolbelt config from secrets
	config := &toolbelt.Config{}
	if token := secrets[db.SecretKeyGitHubToken]; token != "" {
		config.GitHub = &toolbelt.GitHubConfig{Token: token}
	}
	if key := secrets[db.SecretKeyAnthropicKey]; key != "" {
		config.Anthropic = &toolbelt.AnthropicConfig{APIKey: key}
	}

	tb, err := toolbelt.New(config)
	if err != nil {
		return fmt.Errorf("failed to create toolbelt: %w", err)
	}

	s.toolbeltMu.Lock()
	s.toolbelt = tb
	s.toolbeltMu.Unlock()

	// Update session manager with new clients
	if tb.Anthropic != nil {
		fmt.Println("ReloadToolbelt: Anthropic client initialized, updating session manager")
		s.sessionManager.SetAnthropicClient(tb.Anthropic)

		// Update planner with new Anthropic client
		if s.planner == nil {
			s.planner = planning.NewPlanner(s.db, tb.Anthropic, s.hub)
			fmt.Println("ReloadToolbelt: Planner created")
		}

		// Update quest handler with new Anthropic client
		if s.questHandler == nil {
			s.questHandler = quest.NewHandler(s.db, tb.Anthropic, s.hub)
			fmt.Println("ReloadToolbelt: Quest handler created")
		}
	}
	if tb.GitHub != nil {
		fmt.Println("ReloadToolbelt: GitHub client initialized, updating session manager")
		s.sessionManager.SetGitHubClient(tb.GitHub)
		if s.questHandler != nil {
			s.questHandler.SetGitHubClient(tb.GitHub)
			fmt.Println("ReloadToolbelt: Quest handler GitHub client updated")
		}
	}

	// Log status
	status := tb.Status()
	configured := 0
	for _, svc := range status {
		if svc.HasToken {
			configured++
		}
	}
	fmt.Printf("ReloadToolbelt: %d/%d services configured\n", configured, len(status))

	return nil
}

// handleGetPlanning returns the planning session and messages for a task
func (s *Server) handleGetPlanning(c echo.Context) error {
	taskID := c.Param("id")

	if s.planner == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	session, messages, err := s.planner.GetSessionByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	// Convert messages to response format
	msgResponses := make([]map[string]any, len(messages))
	for i, msg := range messages {
		msgResponses[i] = map[string]any{
			"id":         msg.ID,
			"role":       msg.Role,
			"content":    msg.Content,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		}
	}

	// Build session response
	sessionResp := map[string]any{
		"id":              session.ID,
		"task_id":         session.TaskID,
		"status":          session.Status,
		"original_prompt": session.OriginalPrompt,
		"refined_prompt":  session.RefinedPrompt.String,
		"created_at":      session.CreatedAt.Format(time.RFC3339),
	}

	// Include pending checklist if present
	if pendingChecklist := session.GetPendingChecklist(); pendingChecklist != nil {
		sessionResp["pending_checklist"] = pendingChecklist
	}

	return c.JSON(http.StatusOK, map[string]any{
		"session":  sessionResp,
		"messages": msgResponses,
	})
}

// handlePlanningRespond handles a user response during planning
func (s *Server) handlePlanningRespond(c echo.Context) error {
	taskID := c.Param("id")

	if s.planner == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	var req struct {
		Response string `json:"response"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Response == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "response is required")
	}

	// Get the planning session for this task
	session, _, err := s.planner.GetSessionByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	// Process the response
	updatedSession, err := s.planner.ProcessResponse(c.Request().Context(), session.ID, req.Response)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get updated messages
	_, messages, err := s.planner.GetSession(session.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert messages to response format
	msgResponses := make([]map[string]any, len(messages))
	for i, msg := range messages {
		msgResponses[i] = map[string]any{
			"id":         msg.ID,
			"role":       msg.Role,
			"content":    msg.Content,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"session": map[string]any{
			"id":              updatedSession.ID,
			"task_id":         updatedSession.TaskID,
			"status":          updatedSession.Status,
			"original_prompt": updatedSession.OriginalPrompt,
			"refined_prompt":  updatedSession.RefinedPrompt.String,
			"created_at":      updatedSession.CreatedAt.Format(time.RFC3339),
		},
		"messages": msgResponses,
	})
}

// handlePlanningAccept accepts the current plan and transitions task to ready
func (s *Server) handlePlanningAccept(c echo.Context) error {
	taskID := c.Param("id")

	if s.planner == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	// Get the planning session for this task
	session, _, err := s.planner.GetSessionByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	// Accept the plan
	refinedPrompt, err := s.planner.AcceptPlan(c.Request().Context(), session.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Transition task to ready
	if err := s.taskService.UpdateStatus(taskID, db.TaskStatusReady); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast task updated event
	s.hub.Broadcast(websocket.Message{
		Type: "task.updated",
		Payload: map[string]any{
			"task_id": taskID,
			"status":  db.TaskStatusReady,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message":        "plan accepted",
		"task_id":        taskID,
		"refined_prompt": refinedPrompt,
	})
}

// handlePlanningSkip skips the planning phase and transitions task to ready
func (s *Server) handlePlanningSkip(c echo.Context) error {
	taskID := c.Param("id")

	if s.planner == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	// Skip the planning
	if err := s.planner.SkipPlanning(c.Request().Context(), taskID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Transition task to ready
	if err := s.taskService.UpdateStatus(taskID, db.TaskStatusReady); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast task updated event
	s.hub.Broadcast(websocket.Message{
		Type: "task.updated",
		Payload: map[string]any{
			"task_id": taskID,
			"status":  db.TaskStatusReady,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "planning skipped",
		"task_id": taskID,
	})
}

// ChecklistItemResponse is the JSON response format for checklist items
type ChecklistItemResponse struct {
	ID                string  `json:"id"`
	ChecklistID       string  `json:"checklist_id"`
	ParentID          *string `json:"parent_id,omitempty"`
	Description       string  `json:"description"`
	Status            string  `json:"status"`
	VerificationNotes *string `json:"verification_notes,omitempty"`
	CompletedAt       *string `json:"completed_at,omitempty"`
	SortOrder         int     `json:"sort_order"`
}

// toChecklistItemResponse converts a db.ChecklistItem to ChecklistItemResponse
func toChecklistItemResponse(item *db.ChecklistItem) ChecklistItemResponse {
	resp := ChecklistItemResponse{
		ID:          item.ID,
		ChecklistID: item.ChecklistID,
		Description: item.Description,
		Status:      item.Status,
		SortOrder:   item.SortOrder,
	}
	if item.ParentID.Valid {
		resp.ParentID = &item.ParentID.String
	}
	if item.VerificationNotes.Valid {
		resp.VerificationNotes = &item.VerificationNotes.String
	}
	if item.CompletedAt.Valid {
		s := item.CompletedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &s
	}
	return resp
}

// handleGetChecklist returns the checklist and items for a task
func (s *Server) handleGetChecklist(c echo.Context) error {
	taskID := c.Param("id")

	// Get the checklist
	checklist, err := s.db.GetChecklistByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if checklist == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no checklist for task")
	}

	// Get the items
	items, err := s.db.GetChecklistItems(checklist.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert items to response format
	itemResponses := make([]ChecklistItemResponse, len(items))
	for i, item := range items {
		itemResponses[i] = toChecklistItemResponse(item)
	}

	// Calculate summary
	totalCount := len(items)
	doneCount := 0
	failedCount := 0
	pendingCount := 0
	for _, item := range items {
		switch item.Status {
		case db.ChecklistItemStatusDone:
			doneCount++
		case db.ChecklistItemStatusFailed:
			failedCount++
		case db.ChecklistItemStatusPending, db.ChecklistItemStatusInProgress:
			pendingCount++
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"checklist": map[string]any{
			"id":         checklist.ID,
			"task_id":    checklist.TaskID,
			"created_at": checklist.CreatedAt.Format(time.RFC3339),
		},
		"items": itemResponses,
		"summary": map[string]any{
			"total":    totalCount,
			"done":     doneCount,
			"failed":   failedCount,
			"pending":  pendingCount,
			"all_done": doneCount == totalCount,
		},
	})
}

// handleUpdateChecklistItem updates a checklist item status
func (s *Server) handleUpdateChecklistItem(c echo.Context) error {
	taskID := c.Param("id")
	itemID := c.Param("itemId")

	// Verify task and checklist exist
	checklist, err := s.db.GetChecklistByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if checklist == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no checklist for task")
	}

	// Get the item
	item, err := s.db.GetChecklistItem(itemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if item == nil || item.ChecklistID != checklist.ID {
		return echo.NewHTTPError(http.StatusNotFound, "checklist item not found")
	}

	var req struct {
		Status            *string `json:"status"`
		VerificationNotes *string `json:"verification_notes"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Update status if provided
	if req.Status != nil {
		notes := ""
		if req.VerificationNotes != nil {
			notes = *req.VerificationNotes
		}
		if err := s.db.UpdateChecklistItemStatus(itemID, *req.Status, notes); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	// Get updated item
	updatedItem, err := s.db.GetChecklistItem(itemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast checklist update
	s.hub.Broadcast(websocket.Message{
		Type: "checklist.updated",
		Payload: map[string]any{
			"task_id":      taskID,
			"checklist_id": checklist.ID,
			"item":         toChecklistItemResponse(updatedItem),
		},
	})

	return c.JSON(http.StatusOK, toChecklistItemResponse(updatedItem))
}

// handleAcceptChecklist creates checklist items from pending checklist and transitions task to ready
func (s *Server) handleAcceptChecklist(c echo.Context) error {
	taskID := c.Param("id")

	// Get the planning session to access pending checklist
	planningSession, err := s.db.GetPlanningSessionByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if planningSession == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	pendingChecklist := planningSession.GetPendingChecklist()
	if pendingChecklist == nil {
		// No checklist - just transition to ready
		if err := s.taskService.UpdateStatus(taskID, db.TaskStatusReady); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.hub.Broadcast(websocket.Message{
			Type: "task.updated",
			Payload: map[string]any{
				"task_id": taskID,
				"status":  db.TaskStatusReady,
			},
		})
		return c.JSON(http.StatusOK, map[string]any{
			"message": "plan accepted (no checklist)",
			"task_id": taskID,
		})
	}

	// Parse request for selected optional items
	var req struct {
		SelectedOptional []int `json:"selected_optional"` // Indices of selected optional items
	}
	c.Bind(&req) // Ignore error - defaults to empty

	// Create a set of selected optional indices
	selectedOptionalSet := make(map[int]bool)
	for _, idx := range req.SelectedOptional {
		selectedOptionalSet[idx] = true
	}

	// Create the checklist
	checklist, err := s.db.CreateTaskChecklist(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sortOrder := 0

	// Add all must-have items
	for _, desc := range pendingChecklist.MustHave {
		if _, err := s.db.CreateChecklistItem(checklist.ID, desc, sortOrder); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		sortOrder++
	}

	// Add selected optional items
	for idx, desc := range pendingChecklist.Optional {
		if selectedOptionalSet[idx] {
			if _, err := s.db.CreateChecklistItem(checklist.ID, desc, sortOrder); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			sortOrder++
		}
	}

	// Transition task to ready
	if err := s.taskService.UpdateStatus(taskID, db.TaskStatusReady); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast task updated event
	s.hub.Broadcast(websocket.Message{
		Type: "task.updated",
		Payload: map[string]any{
			"task_id": taskID,
			"status":  db.TaskStatusReady,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message":      "checklist accepted",
		"task_id":      taskID,
		"checklist_id": checklist.ID,
		"items_count":  sortOrder,
	})
}

// handleCreateRemediation creates a new task to remediate failed checklist items
func (s *Server) handleCreateRemediation(c echo.Context) error {
	taskID := c.Param("id")

	// Get the original task
	originalTask, err := s.taskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Get the checklist
	checklist, err := s.db.GetChecklistByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if checklist == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no checklist for task")
	}

	// Get issues
	issues, err := s.db.GetChecklistIssues(checklist.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(issues) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no issues to remediate")
	}

	// Build remediation description
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Remediation for task %s:\n\n", taskID))
	sb.WriteString("The following items need to be addressed:\n\n")
	for _, issue := range issues {
		sb.WriteString(fmt.Sprintf("- %s\n", issue.Description))
		if issue.Notes != "" {
			sb.WriteString(fmt.Sprintf("  Previous attempt failed: %q\n", issue.Notes))
		}
	}

	// Create the remediation task
	title := fmt.Sprintf("Fix: %s", originalTask.Title)
	newTask, err := s.taskService.Create(originalTask.ProjectID, title, originalTask.Type, originalTask.Priority)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Update description
	description := sb.String()
	updates := task.TaskUpdates{Description: &description}
	newTask, err = s.taskService.Update(newTask.ID, updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"message":          "remediation task created",
		"task":             toTaskResponse(newTask),
		"original_task_id": taskID,
		"issues_count":     len(issues),
	})
}

// Quest response types
type questResponse struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	Title            string     `json:"title,omitempty"`
	Status           string     `json:"status"`
	Model            string     `json:"model"`
	AutoStartDefault bool       `json:"auto_start_default"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	Summary          *questSummaryResponse `json:"summary,omitempty"`
}

type questSummaryResponse struct {
	TotalTasks       int     `json:"total_tasks"`
	CompletedTasks   int     `json:"completed_tasks"`
	RunningTasks     int     `json:"running_tasks"`
	FailedTasks      int     `json:"failed_tasks"`
	BlockedTasks     int     `json:"blocked_tasks"`
	PendingTasks     int     `json:"pending_tasks"`
	TotalDollarsUsed float64 `json:"total_dollars_used"`
}

type questToolCallResponse struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input"`
	Output     string         `json:"output"`
	IsError    bool           `json:"is_error"`
	DurationMs int64          `json:"duration_ms"`
}

type questMessageResponse struct {
	ID        string                  `json:"id"`
	QuestID   string                  `json:"quest_id"`
	Role      string                  `json:"role"`
	Content   string                  `json:"content"`
	ToolCalls []questToolCallResponse `json:"tool_calls,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
}

func toQuestResponse(q *db.Quest, summary *db.QuestSummary) questResponse {
	resp := questResponse{
		ID:               q.ID,
		ProjectID:        q.ProjectID,
		Title:            q.GetTitle(),
		Status:           q.Status,
		Model:            q.Model,
		AutoStartDefault: q.AutoStartDefault,
		CreatedAt:        q.CreatedAt,
	}
	if q.CompletedAt.Valid {
		resp.CompletedAt = &q.CompletedAt.Time
	}
	if summary != nil {
		resp.Summary = &questSummaryResponse{
			TotalTasks:       summary.TotalTasks,
			CompletedTasks:   summary.CompletedTasks,
			RunningTasks:     summary.RunningTasks,
			FailedTasks:      summary.FailedTasks,
			BlockedTasks:     summary.BlockedTasks,
			PendingTasks:     summary.PendingTasks,
			TotalDollarsUsed: summary.TotalDollarsUsed,
		}
	}
	return resp
}

func toQuestMessageResponse(m *db.QuestMessage) questMessageResponse {
	resp := questMessageResponse{
		ID:        m.ID,
		QuestID:   m.QuestID,
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}

	// Convert tool calls
	if len(m.ToolCalls) > 0 {
		resp.ToolCalls = make([]questToolCallResponse, len(m.ToolCalls))
		for i, tc := range m.ToolCalls {
			resp.ToolCalls[i] = questToolCallResponse{
				ToolName:   tc.ToolName,
				Input:      tc.Input,
				Output:     tc.Output,
				IsError:    tc.IsError,
				DurationMs: tc.DurationMs,
			}
		}
	}

	return resp
}

// ensureDefaultProject creates the default project if it doesn't exist
func (s *Server) ensureDefaultProject(projectID string) error {
	if projectID != "proj_default" {
		return nil
	}

	project, err := s.db.GetProjectByID(projectID)
	if err != nil {
		return err
	}
	if project == nil {
		// Create the default project
		_, err = s.db.CreateProjectWithID(projectID, "Default Project", ".")
		if err != nil {
			return err
		}
	}
	return nil
}

// handleListQuests returns all quests for a project
func (s *Server) handleListQuests(c echo.Context) error {
	projectID := c.Param("id")

	// Auto-create default project if needed
	if err := s.ensureDefaultProject(projectID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Verify project exists
	project, err := s.db.GetProjectByID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	quests, err := s.db.GetQuestsByProjectID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Build response with summaries
	response := make([]questResponse, 0, len(quests))
	for _, q := range quests {
		summary, _ := s.db.GetQuestSummary(q.ID)
		response = append(response, toQuestResponse(q, summary))
	}

	return c.JSON(http.StatusOK, response)
}

// handleCreateQuest creates a new quest for a project
func (s *Server) handleCreateQuest(c echo.Context) error {
	projectID := c.Param("id")

	// Auto-create default project if needed
	if err := s.ensureDefaultProject(projectID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Verify project exists
	project, err := s.db.GetProjectByID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Default to sonnet if not specified
	model := req.Model
	if model == "" {
		model = db.QuestModelSonnet
	}
	if model != db.QuestModelSonnet && model != db.QuestModelOpus {
		return echo.NewHTTPError(http.StatusBadRequest, "model must be 'sonnet' or 'opus'")
	}

	quest, err := s.db.CreateQuest(projectID, model)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast quest created event
	s.hub.Broadcast(websocket.Message{
		Type: "quest.created",
		Payload: map[string]any{
			"quest_id":   quest.ID,
			"project_id": projectID,
		},
	})

	return c.JSON(http.StatusCreated, toQuestResponse(quest, nil))
}

// handleGetQuest returns a quest by ID with its messages
func (s *Server) handleGetQuest(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	messages, err := s.db.GetQuestMessages(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	summary, _ := s.db.GetQuestSummary(questID)

	// Build messages response
	msgResponses := make([]questMessageResponse, 0, len(messages))
	for _, m := range messages {
		msgResponses = append(msgResponses, toQuestMessageResponse(m))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"quest":    toQuestResponse(quest, summary),
		"messages": msgResponses,
	})
}

// handleDeleteQuest deletes a quest and all its messages
func (s *Server) handleDeleteQuest(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	// Check if quest has any tasks
	tasks, err := s.db.GetTasksByQuestID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(tasks) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete quest with existing tasks")
	}

	if err := s.db.DeleteQuest(questID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast quest deleted event
	s.hub.Broadcast(websocket.Message{
		Type: "quest.deleted",
		Payload: map[string]any{
			"quest_id":   questID,
			"project_id": quest.ProjectID,
		},
	})

	return c.JSON(http.StatusOK, map[string]string{
		"message": "quest deleted",
	})
}

// handleSendQuestMessage adds a user message to a quest and gets Dex's response
func (s *Server) handleSendQuestMessage(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if quest.Status != db.QuestStatusActive {
		return echo.NewHTTPError(http.StatusBadRequest, "quest is not active")
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Content) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "content is required")
	}

	// Create user message
	userMsg, err := s.db.CreateQuestMessage(questID, "user", req.Content)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast user message event
	s.hub.Broadcast(websocket.Message{
		Type: "quest.message",
		Payload: map[string]any{
			"quest_id": questID,
			"message":  toQuestMessageResponse(userMsg),
		},
	})

	// Call Dex to generate response
	if s.questHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured (missing Anthropic API key)")
	}

	assistantMsg, err := s.questHandler.ProcessMessage(c.Request().Context(), questID, req.Content)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to process message: %v", err))
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"user_message":      toQuestMessageResponse(userMsg),
		"assistant_message": toQuestMessageResponse(assistantMsg),
	})
}

// handleCompleteQuest marks a quest as completed
func (s *Server) handleCompleteQuest(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if quest.Status == db.QuestStatusCompleted {
		return echo.NewHTTPError(http.StatusBadRequest, "quest is already completed")
	}

	if err := s.db.CompleteQuest(questID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get updated quest
	quest, _ = s.db.GetQuestByID(questID)
	summary, _ := s.db.GetQuestSummary(questID)

	// Broadcast quest completed event
	s.hub.Broadcast(websocket.Message{
		Type: "quest.completed",
		Payload: map[string]any{
			"quest_id":   questID,
			"project_id": quest.ProjectID,
		},
	})

	return c.JSON(http.StatusOK, toQuestResponse(quest, summary))
}

// handleReopenQuest reopens a completed quest
func (s *Server) handleReopenQuest(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if quest.Status != db.QuestStatusCompleted {
		return echo.NewHTTPError(http.StatusBadRequest, "quest is not completed")
	}

	if err := s.db.ReopenQuest(questID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get updated quest
	quest, _ = s.db.GetQuestByID(questID)
	summary, _ := s.db.GetQuestSummary(questID)

	// Broadcast quest reopened event
	s.hub.Broadcast(websocket.Message{
		Type: "quest.reopened",
		Payload: map[string]any{
			"quest_id":   questID,
			"project_id": quest.ProjectID,
		},
	})

	return c.JSON(http.StatusOK, toQuestResponse(quest, summary))
}

// handleUpdateQuestModel updates the model for a quest (sonnet or opus)
func (s *Server) handleUpdateQuestModel(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Model != db.QuestModelSonnet && req.Model != db.QuestModelOpus {
		return echo.NewHTTPError(http.StatusBadRequest, "model must be 'sonnet' or 'opus'")
	}

	if err := s.db.UpdateQuestModel(questID, req.Model); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get updated quest
	quest, _ = s.db.GetQuestByID(questID)
	summary, _ := s.db.GetQuestSummary(questID)

	// Broadcast quest updated event
	s.hub.Broadcast(websocket.Message{
		Type: "quest.updated",
		Payload: map[string]any{
			"quest_id": questID,
			"model":    req.Model,
		},
	})

	return c.JSON(http.StatusOK, toQuestResponse(quest, summary))
}

// handleGetQuestTasks returns all tasks spawned by a quest
func (s *Server) handleGetQuestTasks(c echo.Context) error {
	questID := c.Param("id")

	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	tasks, err := s.db.GetTasksByQuestID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert to response format
	response := make([]TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		response = append(response, toTaskResponse(t))
	}

	return c.JSON(http.StatusOK, response)
}

// handleCreateObjective creates a task from an accepted objective draft
func (s *Server) handleCreateObjective(c echo.Context) error {
	questID := c.Param("id")

	questObj, err := s.db.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if questObj == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if s.questHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured")
	}

	var req struct {
		DraftID          string   `json:"draft_id"`
		Title            string   `json:"title"`
		Description      string   `json:"description"`
		Hat              string   `json:"hat"`
		MustHave         []string `json:"must_have"`
		Optional         []string `json:"optional"`
		SelectedOptional []int    `json:"selected_optional"`
		AutoStart        bool     `json:"auto_start"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	// Create draft from request
	draft := quest.ObjectiveDraft{
		DraftID:     req.DraftID,
		Title:       req.Title,
		Description: req.Description,
		Hat:         req.Hat,
		Checklist: quest.Checklist{
			MustHave: req.MustHave,
			Optional: req.Optional,
		},
		AutoStart: req.AutoStart,
	}

	// Create the objective (task) from the draft
	createdTask, err := s.questHandler.CreateObjectiveFromDraft(c.Request().Context(), questID, draft, req.SelectedOptional)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Add a message to the quest history indicating the objective was accepted
	acceptMessage := fmt.Sprintf("✓ Accepted objective: **%s**", req.Title)
	if msg, err := s.db.CreateQuestMessage(questID, "user", acceptMessage); err != nil {
		fmt.Printf("warning: failed to add accept message to quest: %v\n", err)
	} else if s.hub != nil {
		// Broadcast the new message via WebSocket
		s.hub.Broadcast(websocket.Message{
			Type: "quest.message",
			Payload: map[string]any{
				"quest_id": questID,
				"message":  msg,
			},
		})
	}

	response := map[string]any{
		"message": "objective created",
		"task":    toTaskResponse(createdTask),
	}

	// Auto-start the task if requested
	if req.AutoStart {
		startResult, err := s.startTaskInternal(context.Background(), createdTask.ID, "")
		if err != nil {
			// Task was created but couldn't be started - return partial success
			response["auto_start_error"] = err.Error()
			fmt.Printf("auto-start failed for task %s: %v\n", createdTask.ID, err)
		} else {
			response["task"] = toTaskResponse(startResult.Task)
			response["worktree_path"] = startResult.WorktreePath
			response["session_id"] = startResult.SessionID
			response["auto_started"] = true
		}
	}

	return c.JSON(http.StatusCreated, response)
}

// handleGetPreflightCheck returns pre-flight check results for a quest's project
func (s *Server) handleGetPreflightCheck(c echo.Context) error {
	questID := c.Param("id")

	if s.questHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured")
	}

	check, err := s.questHandler.GetPreflightCheck(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, check)
}

// handleListQuestTemplates returns all quest templates for a project
func (s *Server) handleListQuestTemplates(c echo.Context) error {
	projectID := c.Param("id")

	templates, err := s.db.GetQuestTemplatesByProjectID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	result := make([]map[string]any, len(templates))
	for i, t := range templates {
		result[i] = map[string]any{
			"id":             t.ID,
			"project_id":     t.ProjectID,
			"name":           t.Name,
			"description":    t.Description.String,
			"initial_prompt": t.InitialPrompt,
			"created_at":     t.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// handleCreateQuestTemplate creates a new quest template
func (s *Server) handleCreateQuestTemplate(c echo.Context) error {
	projectID := c.Param("id")

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		InitialPrompt string `json:"initial_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.InitialPrompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "initial_prompt is required")
	}

	template, err := s.db.CreateQuestTemplate(projectID, req.Name, req.Description, req.InitialPrompt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"id":             template.ID,
		"project_id":     template.ProjectID,
		"name":           template.Name,
		"description":    template.Description.String,
		"initial_prompt": template.InitialPrompt,
		"created_at":     template.CreatedAt,
	})
}

// handleGetQuestTemplate returns a quest template by ID
func (s *Server) handleGetQuestTemplate(c echo.Context) error {
	templateID := c.Param("id")

	template, err := s.db.GetQuestTemplateByID(templateID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if template == nil {
		return echo.NewHTTPError(http.StatusNotFound, "template not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":             template.ID,
		"project_id":     template.ProjectID,
		"name":           template.Name,
		"description":    template.Description.String,
		"initial_prompt": template.InitialPrompt,
		"created_at":     template.CreatedAt,
	})
}

// handleUpdateQuestTemplate updates a quest template
func (s *Server) handleUpdateQuestTemplate(c echo.Context) error {
	templateID := c.Param("id")

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		InitialPrompt string `json:"initial_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.InitialPrompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "initial_prompt is required")
	}

	err := s.db.UpdateQuestTemplate(templateID, req.Name, req.Description, req.InitialPrompt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	template, _ := s.db.GetQuestTemplateByID(templateID)
	return c.JSON(http.StatusOK, map[string]any{
		"id":             template.ID,
		"project_id":     template.ProjectID,
		"name":           template.Name,
		"description":    template.Description.String,
		"initial_prompt": template.InitialPrompt,
		"created_at":     template.CreatedAt,
	})
}

// handleDeleteQuestTemplate deletes a quest template
func (s *Server) handleDeleteQuestTemplate(c echo.Context) error {
	templateID := c.Param("id")

	err := s.db.DeleteQuestTemplate(templateID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "template deleted",
	})
}
