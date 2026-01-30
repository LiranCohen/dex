package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/liranmauda/dex/internal/api/middleware"
	"github.com/liranmauda/dex/internal/auth"
	"github.com/liranmauda/dex/internal/db"
	"github.com/liranmauda/dex/internal/git"
	"github.com/liranmauda/dex/internal/task"
	"github.com/liranmauda/dex/internal/toolbelt"
)

// Server represents the API server
type Server struct {
	echo        *echo.Echo
	db          *db.DB
	toolbelt    *toolbelt.Toolbelt
	taskService *task.Service
	gitService  *git.Service
	addr        string
	certFile    string
	keyFile     string
	tokenConfig *auth.TokenConfig
	staticDir   string
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

	s := &Server{
		echo:        e,
		db:          database,
		toolbelt:    cfg.Toolbelt,
		taskService: task.NewService(database),
		addr:        cfg.Addr,
		certFile:    cfg.CertFile,
		keyFile:     cfg.KeyFile,
		tokenConfig: cfg.TokenConfig,
		staticDir:   cfg.StaticDir,
	}

	// Setup git service if worktree base is configured
	if cfg.WorktreeBase != "" {
		s.gitService = git.NewService(database, cfg.WorktreeBase)
	}

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

	// Transition to running status
	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		// Try to clean up the worktree we just created
		_ = s.gitService.CleanupTaskWorktree(req.ProjectPath, taskID, true)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Fetch updated task
	updated, _ := s.taskService.Get(taskID)

	return c.JSON(http.StatusOK, map[string]any{
		"task":          updated,
		"worktree_path": worktreePath,
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
