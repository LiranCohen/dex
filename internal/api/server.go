package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/handlers/approvals"
	authhandlers "github.com/lirancohen/dex/internal/api/handlers/auth"
	githubhandlers "github.com/lirancohen/dex/internal/api/handlers/github"
	githubsync "github.com/lirancohen/dex/internal/api/handlers/github"
	"github.com/lirancohen/dex/internal/api/handlers/memory"
	planninghandlers "github.com/lirancohen/dex/internal/api/handlers/planning"
	"github.com/lirancohen/dex/internal/api/handlers/projects"
	"github.com/lirancohen/dex/internal/api/handlers/quests"
	sessionshandlers "github.com/lirancohen/dex/internal/api/handlers/sessions"
	"github.com/lirancohen/dex/internal/api/handlers/tasks"
	toolbelthandlers "github.com/lirancohen/dex/internal/api/handlers/toolbelt"
	"github.com/lirancohen/dex/internal/api/middleware"
	"github.com/lirancohen/dex/internal/api/setup"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/realtime"
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

// challengeEntry is an alias for core.ChallengeEntry
type challengeEntry = core.ChallengeEntry

// Server represents the API server
type Server struct {
	echo              *echo.Echo
	db                *db.DB
	toolbelt          *toolbelt.Toolbelt
	taskService       *task.Service
	gitService        *git.Service
	sessionManager    *session.Manager
	planner           *planning.Planner
	questHandler      *quest.Handler
	githubApp         *github.AppManager
	githubSyncService *github.SyncService     // Underlying GitHub sync service
	handlersSyncSvc   *githubsync.SyncService // Handler-level sync service wrapper
	setupHandler      *setup.Handler
	hub               *websocket.Hub
	realtime          *realtime.Node         // Centrifuge-based realtime messaging
	broadcaster       *realtime.Broadcaster  // Dual-publish to legacy and new
	deps              *core.Deps
	addr              string
	certFile          string
	keyFile           string
	tokenConfig       *auth.TokenConfig
	staticDir         string
	baseDir           string                    // Base Dex directory (e.g., /opt/dex)
	challenges        map[string]challengeEntry // challenge -> expiry
	challengesMu      sync.RWMutex
	toolbeltMu        sync.RWMutex // Protects toolbelt updates
	githubAppMu       sync.RWMutex // Protects GitHub App manager
}

// Config holds server configuration
type Config struct {
	Addr        string             // e.g., ":8443" or "0.0.0.0:8443"
	CertFile    string             // Path to TLS certificate (optional for dev)
	KeyFile     string             // Path to TLS key (optional for dev)
	TokenConfig *auth.TokenConfig  // JWT configuration (optional for dev)
	StaticDir   string             // Path to frontend static files (e.g., "./frontend/dist")
	Toolbelt    *toolbelt.Toolbelt // Toolbelt for external service integrations (optional)
	BaseDir     string             // Base Dex directory (default: /opt/dex). Derived: {BaseDir}/repos/, {BaseDir}/worktrees/
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

	// Create WebSocket hub (legacy - will be replaced by realtime)
	hub := websocket.NewHub()
	go hub.Run()

	// Create Centrifuge realtime node
	rtNode, err := realtime.NewNode(realtime.Config{
		ClientQueueMaxSize: 2 * 1024 * 1024, // 2MB per client
		ClientChannelLimit: 128,
	})
	if err != nil {
		fmt.Printf("Warning: failed to create realtime node: %v\n", err)
	} else {
		if err := rtNode.Run(); err != nil {
			fmt.Printf("Warning: failed to start realtime node: %v\n", err)
			rtNode = nil
		}
	}

	// Create broadcaster for dual-publishing during migration
	broadcaster := realtime.NewBroadcaster(hub, rtNode)

	s := &Server{
		echo:        e,
		db:          database,
		toolbelt:    cfg.Toolbelt,
		taskService: task.NewService(database),
		hub:         hub,
		realtime:    rtNode,
		broadcaster: broadcaster,
		addr:        cfg.Addr,
		certFile:    cfg.CertFile,
		keyFile:     cfg.KeyFile,
		tokenConfig: cfg.TokenConfig,
		staticDir:   cfg.StaticDir,
		baseDir:     cfg.BaseDir,
		challenges:  make(map[string]challengeEntry),
	}

	// Setup git service with derived paths from base directory
	if cfg.BaseDir != "" {
		worktreeDir := filepath.Join(cfg.BaseDir, "worktrees")
		reposDir := filepath.Join(cfg.BaseDir, "repos")
		s.gitService = git.NewService(database, worktreeDir, reposDir)
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

	// Wire up broadcaster for real-time updates (dual-publishes to legacy and new systems)
	sessionMgr.SetBroadcaster(broadcaster)

	// Wire up Anthropic client for Ralph loop execution
	if cfg.Toolbelt != nil && cfg.Toolbelt.Anthropic != nil {
		sessionMgr.SetAnthropicClient(cfg.Toolbelt.Anthropic)
	}

	s.sessionManager = sessionMgr

	// Create planner for task planning phase
	if cfg.Toolbelt != nil && cfg.Toolbelt.Anthropic != nil {
		s.planner = planning.NewPlanner(database, cfg.Toolbelt.Anthropic, broadcaster)
		s.planner.SetPromptLoader(sessionMgr.GetPromptLoader())
		s.questHandler = quest.NewHandler(database, cfg.Toolbelt.Anthropic, broadcaster)
		s.questHandler.SetPromptLoader(sessionMgr.GetPromptLoader())
		s.questHandler.SetBaseDir(cfg.BaseDir)
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

	// Create the Deps struct for dependency injection
	s.deps = &core.Deps{
		DB:             database,
		TaskService:    s.taskService,
		SessionManager: sessionMgr,
		GitService:     s.gitService,
		Planner:        s.planner,
		QuestHandler:   s.questHandler,
		Hub:            hub,
		Realtime:       rtNode,
		Broadcaster:    broadcaster,
		TokenConfig:    cfg.TokenConfig,
		BaseDir:        cfg.BaseDir,
		Challenges:     s.challenges,
		ChallengesMu:   &s.challengesMu,
		GetToolbelt: func() *toolbelt.Toolbelt {
			s.toolbeltMu.RLock()
			defer s.toolbeltMu.RUnlock()
			return s.toolbelt
		},
		GetGitHubApp: func() *github.AppManager {
			s.githubAppMu.RLock()
			defer s.githubAppMu.RUnlock()
			return s.githubApp
		},
		GetGitHubSync: func() *github.SyncService {
			s.githubAppMu.RLock()
			defer s.githubAppMu.RUnlock()
			return s.githubSyncService
		},
		StartTaskInternal: func(ctx context.Context, taskID string, baseBranch string) (*core.StartTaskResult, error) {
			result, err := s.startTaskInternal(ctx, taskID, baseBranch)
			if err != nil {
				return nil, err
			}
			return &core.StartTaskResult{
				Task:         result.Task,
				WorktreePath: result.WorktreePath,
				SessionID:    result.SessionID,
			}, nil
		},
		StartTaskWithInheritance: func(ctx context.Context, taskID string, inheritedWorktree string, predecessorHandoff string) (*core.StartTaskResult, error) {
			result, err := s.startTaskWithInheritance(ctx, taskID, inheritedWorktree, predecessorHandoff)
			if err != nil {
				return nil, err
			}
			return &core.StartTaskResult{
				Task:         result.Task,
				WorktreePath: result.WorktreePath,
				SessionID:    result.SessionID,
			}, nil
		},
		HandleTaskUnblocking: func(ctx context.Context, completedTaskID string) {
			s.handleTaskUnblocking(ctx, completedTaskID)
		},
		GeneratePredecessorHandoff: func(t *db.Task) string {
			return s.generatePredecessorHandoff(t)
		},
		GetToolbeltGitHubClient: func(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
			return s.GetToolbeltGitHubClient(ctx, login)
		},
		IsValidGitRepo:     s.isValidGitRepo,
		IsValidProjectPath: s.isValidProjectPath,
	}

	// Create handler-level sync service (uses deps for cross-service coordination)
	s.handlersSyncSvc = githubsync.NewSyncService(s.deps)

	// Wire up GitHub sync callbacks now that handlersSyncSvc exists
	sessionMgr.SetOnTaskCompleted(func(taskID string) {
		s.handlersSyncSvc.OnTaskCompleted(taskID)
	})
	sessionMgr.SetOnTaskFailed(func(taskID string, reason string) {
		s.handlersSyncSvc.OnTaskFailed(taskID, reason)
	})
	sessionMgr.SetOnPRCreated(func(taskID string, prNumber int) {
		s.handlersSyncSvc.OnPRCreated(taskID, prNumber)
	})
	sessionMgr.SetOnChecklistUpdated(func(taskID string) {
		s.handlersSyncSvc.UpdateObjectiveChecklistSync(taskID)
	})
	sessionMgr.SetOnTaskStatus(func(taskID string, status string) {
		s.handlersSyncSvc.UpdateObjectiveStatusSync(taskID, status)
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

	// Create handlers
	authHandler := authhandlers.New(s.deps)
	passkeyHandler := authhandlers.NewPasskeyHandler(s.deps)
	toolbeltHandler := toolbelthandlers.New(s.deps)
	tasksHandler := tasks.New(s.deps)
	projectsHandler := projects.New(s.deps)
	memoryHandler := memory.New(s.deps)
	approvalsHandler := approvals.New(s.deps)
	sessionsHandler := sessionshandlers.New(s.deps)
	planningHandler := planninghandlers.New(s.deps)
	checklistHandler := planninghandlers.NewChecklistHandler(s.deps)
	questsHandler := quests.New(s.deps)
	objectivesHandler := quests.NewObjectivesHandler(s.deps)
	templatesHandler := quests.NewTemplatesHandler(s.deps)
	githubHandler := githubhandlers.New(s.deps)

	// Wire up callbacks for GitHub sync
	githubHandler.InitGitHubApp = s.initGitHubApp
	questsHandler.SyncQuestToGitHubIssue = s.syncQuestToGitHubIssue
	questsHandler.CloseQuestGitHubIssue = s.closeQuestGitHubIssue
	questsHandler.ReopenQuestGitHubIssue = s.reopenQuestGitHubIssue
	objectivesHandler.SyncObjectiveToGitHubIssue = s.syncObjectiveToGitHubIssue

	// Public endpoints (no auth required)
	v1.GET("/system/status", s.handleHealthCheck)

	// Register public routes
	toolbeltHandler.RegisterPublicRoutes(v1)
	authHandler.RegisterRoutes(v1)
	passkeyHandler.RegisterRoutes(v1)
	githubHandler.RegisterRoutes(v1)

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

	// Protected endpoints (require JWT auth)
	// Use middleware if token config is available, otherwise allow all (dev mode)
	protected := v1.Group("")
	if s.tokenConfig != nil {
		protected.Use(middleware.JWTAuth(s.tokenConfig))
	}

	// User info
	protected.GET("/me", toolbeltHandler.HandleMe)

	// Register protected routes from handlers
	tasksHandler.RegisterRoutes(protected)
	projectsHandler.RegisterRoutes(protected)
	memoryHandler.RegisterRoutes(protected)
	approvalsHandler.RegisterRoutes(protected)
	sessionsHandler.RegisterRoutes(protected)
	planningHandler.RegisterRoutes(protected)
	checklistHandler.RegisterRoutes(protected)
	questsHandler.RegisterRoutes(protected)
	objectivesHandler.RegisterRoutes(protected)
	templatesHandler.RegisterRoutes(protected)

	// WebSocket endpoint for real-time updates (legacy hub)
	protected.GET("/ws", func(c echo.Context) error {
		return websocket.ServeWS(s.hub, c)
	})

	// Centrifuge WebSocket endpoint (new realtime system)
	if s.realtime != nil {
		wsHandler := s.realtime.WebSocketHandler()

		// Apply auth middleware based on token config
		if s.tokenConfig != nil {
			validator := realtime.NewJWTValidator(s.tokenConfig)
			wsHandler = realtime.AuthMiddleware(validator)(wsHandler)
		} else {
			wsHandler = realtime.NoAuthMiddleware()(wsHandler)
		}

		// Register the Centrifuge WebSocket endpoint
		v1.GET("/realtime", echo.WrapHandler(wsHandler))
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
	// Shutdown the realtime node first
	if s.realtime != nil {
		if err := s.realtime.Shutdown(ctx); err != nil {
			fmt.Printf("Warning: failed to shutdown realtime node: %v\n", err)
		}
	}
	return s.echo.Shutdown(ctx)
}
