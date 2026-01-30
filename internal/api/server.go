package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/lirancohen/dex/internal/api/middleware"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/orchestrator"
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
	hub            *websocket.Hub
	addr           string
	certFile       string
	keyFile        string
	tokenConfig    *auth.TokenConfig
	staticDir      string
	challenges     map[string]challengeEntry // challenge -> expiry
	challengesMu   sync.RWMutex
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
		challenges:  make(map[string]challengeEntry),
	}

	// Setup git service if worktree base is configured
	if cfg.WorktreeBase != "" {
		s.gitService = git.NewService(database, cfg.WorktreeBase)
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

	// Task endpoints (public for now, will add auth later)
	v1.GET("/tasks", s.handleListTasks)
	v1.POST("/tasks", s.handleCreateTask)
	v1.GET("/tasks/:id", s.handleGetTask)
	v1.PUT("/tasks/:id", s.handleUpdateTask)
	v1.DELETE("/tasks/:id", s.handleDeleteTask)
	v1.POST("/tasks/:id/start", s.handleStartTask)
	v1.GET("/tasks/:id/worktree/status", s.handleTaskWorktreeStatus)

	// Worktree endpoints (public for now)
	v1.GET("/worktrees", s.handleListWorktrees)
	v1.DELETE("/worktrees/:task_id", s.handleDeleteWorktree)

	// Project endpoints (public for now, will add auth later)
	v1.GET("/projects", s.handleListProjects)
	v1.POST("/projects", s.handleCreateProject)
	v1.GET("/projects/:id", s.handleGetProject)
	v1.PUT("/projects/:id", s.handleUpdateProject)
	v1.DELETE("/projects/:id", s.handleDeleteProject)

	// Approval endpoints (public for now, will add auth later)
	v1.GET("/approvals", s.handleListApprovals)
	v1.GET("/approvals/:id", s.handleGetApproval)
	v1.POST("/approvals/:id/approve", s.handleApproveApproval)
	v1.POST("/approvals/:id/reject", s.handleRejectApproval)

	// Session control endpoints
	v1.POST("/tasks/:id/pause", s.handlePauseTask)
	v1.POST("/tasks/:id/resume", s.handleResumeTask)
	v1.POST("/tasks/:id/cancel", s.handleCancelTask)
	v1.GET("/tasks/:id/logs", s.handleTaskLogs)

	// Session management endpoints
	v1.GET("/sessions", s.handleListSessions)
	v1.GET("/sessions/:id", s.handleGetSession)
	v1.POST("/sessions/:id/kill", s.handleKillSession)

	// WebSocket endpoint for real-time updates
	v1.GET("/ws", func(c echo.Context) error {
		return websocket.ServeWS(s.hub, c)
	})

	// Protected endpoints (require auth)
	if s.tokenConfig != nil {
		protected := v1.Group("")
		protected.Use(middleware.JWTAuth(s.tokenConfig))
		protected.GET("/me", s.handleMe)
	}
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

	return c.JSON(http.StatusOK, map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// handleCreateTask creates a new task
func (s *Server) handleCreateTask(c echo.Context) error {
	var req struct {
		ProjectID string `json:"project_id"`
		Title     string `json:"title"`
		Type      string `json:"type"`
		Priority  int    `json:"priority"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	t, err := s.taskService.Create(req.ProjectID, req.Title, req.Type, req.Priority)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, t)
}

// handleGetTask returns a single task by ID
func (s *Server) handleGetTask(c echo.Context) error {
	id := c.Param("id")

	t, err := s.taskService.Get(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, t)
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

	return c.JSON(http.StatusOK, updated)
}

// handleDeleteTask removes a task
func (s *Server) handleDeleteTask(c echo.Context) error {
	id := c.Param("id")

	if err := s.taskService.Delete(id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// handleStartTask transitions a task to running and sets up its worktree
func (s *Server) handleStartTask(c echo.Context) error {
	taskID := c.Param("id")

	var req struct {
		ProjectPath string `json:"project_path"`
		BaseBranch  string `json:"base_branch"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.ProjectPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "project_path is required")
	}
	if req.BaseBranch == "" {
		req.BaseBranch = "main" // Default to main
	}

	// Get the task first
	t, err := s.taskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Check if already has a worktree
	if t.WorktreePath.Valid && t.WorktreePath.String != "" {
		return echo.NewHTTPError(http.StatusConflict, "task already has a worktree")
	}

	// Create worktree
	if s.gitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	worktreePath, err := s.gitService.SetupTaskWorktree(req.ProjectPath, taskID, req.BaseBranch)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create worktree: %v", err))
	}

	// Transition through ready to running status
	// First: pending -> ready
	if t.Status == "pending" {
		if err := s.taskService.UpdateStatus(taskID, "ready"); err != nil {
			_ = s.gitService.CleanupTaskWorktree(req.ProjectPath, taskID, true)
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	// Then: ready -> running
	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		// Try to clean up the worktree we just created
		_ = s.gitService.CleanupTaskWorktree(req.ProjectPath, taskID, true)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Create and start a session for this task
	hat := "implementer" // Default hat - could be determined from task type
	if t.Hat.Valid && t.Hat.String != "" {
		hat = t.Hat.String
	}

	session, err := s.sessionManager.CreateSession(taskID, hat, worktreePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create session: %v", err))
	}

	// Start the session (runs Ralph loop in background)
	if err := s.sessionManager.Start(c.Request().Context(), session.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to start session: %v", err))
	}

	// Fetch updated task
	updated, _ := s.taskService.Get(taskID)

	return c.JSON(http.StatusOK, map[string]any{
		"task":          updated,
		"worktree_path": worktreePath,
		"session_id":    session.ID,
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
		Name     string `json:"name"`
		RepoPath string `json:"repo_path"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.RepoPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "repo_path is required")
	}

	project, err := s.db.CreateProject(req.Name, req.RepoPath)
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
	TokensUsed     int64    `json:"tokens_used"`
	TokensBudget   *int64   `json:"tokens_budget,omitempty"`
	DollarsUsed    float64  `json:"dollars_used"`
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
		TokensUsed:     s.TokensUsed,
		TokensBudget:   s.TokensBudget,
		DollarsUsed:    s.DollarsUsed,
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

// GetHub returns the WebSocket hub for broadcasting events
func (s *Server) GetHub() *websocket.Hub {
	return s.hub
}
