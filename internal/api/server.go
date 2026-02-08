package api

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/lirancohen/dex/frontend"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/handlers/approvals"
	authhandlers "github.com/lirancohen/dex/internal/api/handlers/auth"
	deviceshandlers "github.com/lirancohen/dex/internal/api/handlers/devices"
	forgejohandlers "github.com/lirancohen/dex/internal/api/handlers/forgejo"
	"github.com/lirancohen/dex/internal/api/handlers/issuesync"
	"github.com/lirancohen/dex/internal/api/handlers/memory"
	meshhandlers "github.com/lirancohen/dex/internal/api/handlers/mesh"
	planninghandlers "github.com/lirancohen/dex/internal/api/handlers/planning"
	"github.com/lirancohen/dex/internal/api/handlers/projects"
	"github.com/lirancohen/dex/internal/api/handlers/quests"
	sessionshandlers "github.com/lirancohen/dex/internal/api/handlers/sessions"
	"github.com/lirancohen/dex/internal/api/handlers/tasks"
	toolbelthandlers "github.com/lirancohen/dex/internal/api/handlers/toolbelt"
	workershandlers "github.com/lirancohen/dex/internal/api/handlers/workers"
	"github.com/lirancohen/dex/internal/api/middleware"
	"github.com/lirancohen/dex/internal/api/setup"
	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/auth/oidc"
	"github.com/lirancohen/dex/internal/crypto"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/forgejo"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/mesh"
	"github.com/lirancohen/dex/internal/orchestrator"
	"github.com/lirancohen/dex/internal/planning"
	"github.com/lirancohen/dex/internal/quest"
	"github.com/lirancohen/dex/internal/realtime"
	"github.com/lirancohen/dex/internal/session"
	"github.com/lirancohen/dex/internal/task"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/worker"
)

// Server represents the API server
type Server struct {
	echo             *echo.Echo
	db               *db.DB
	toolbelt         *toolbelt.Toolbelt
	taskService      *task.Service
	gitService       *git.Service
	sessionManager   *session.Manager
	planner          *planning.Planner
	questHandler     *quest.Handler
	handlersSyncSvc  *issuesync.SyncService // Handler-level sync service wrapper
	setupHandler     *setup.Handler
	realtime         *realtime.Node // Centrifuge-based realtime messaging
	broadcaster      *realtime.Broadcaster
	meshClient       *mesh.Client                   // Mesh network client (dexnet)
	workerManager    *worker.Manager                // Worker pool manager for distributed execution
	meshProxy        *mesh.ServiceProxy             // Reverse proxy for mesh-exposed services
	forgejoManager   *forgejo.Manager               // Embedded Forgejo instance manager
	oidcHandler      *authhandlers.OIDCHandler      // OIDC provider for SSO
	oidcLoginHandler *authhandlers.OIDCLoginHandler // Passkey login for OIDC
	devicesHandler   *deviceshandlers.Handler       // Device management handler
	deps             *core.Deps
	encryption       *crypto.EncryptionConfig // Encryption for secrets and worker payloads
	addr             string
	certFile         string
	keyFile          string
	tokenConfig      *auth.TokenConfig
	staticDir        string
	baseDir          string       // Base Dex directory (e.g., /opt/dex)
	publicURL        string       // Public URL for OIDC issuer (e.g., https://hq.alice.enbox.id)
	namespace        string       // Account namespace (from enrollment)
	tunnelToken      string       // Token for Central API
	centralURL       string       // Central server URL
	toolbeltMu       sync.RWMutex // Protects toolbelt updates
}

// Config holds server configuration
type Config struct {
	Addr        string                   // e.g., ":8443" or "0.0.0.0:8443"
	CertFile    string                   // Path to TLS certificate (optional for dev)
	KeyFile     string                   // Path to TLS key (optional for dev)
	TokenConfig *auth.TokenConfig        // JWT configuration (optional for dev)
	StaticDir   string                   // Path to frontend static files (e.g., "./frontend/dist")
	Toolbelt    *toolbelt.Toolbelt       // Toolbelt for external service integrations (optional)
	BaseDir     string                   // Base Dex directory (default: /opt/dex). Derived: {BaseDir}/repos/, {BaseDir}/worktrees/
	Mesh        *mesh.Config             // Mesh networking configuration (optional)
	Encryption  *crypto.EncryptionConfig // Encryption configuration for secrets at rest and worker payloads
	Worker      *worker.ManagerConfig    // Worker pool configuration (optional)
	Forgejo     *forgejo.Config          // Embedded Forgejo configuration (optional)
	PublicURL   string                   // Public URL for OIDC issuer (e.g., https://hq.alice.enbox.id)

	// Enrollment configuration (from config.json, for device management)
	Namespace   string // Account namespace (e.g., "alice")
	TunnelToken string // Token for authenticating with Central
	CentralURL  string // Central server URL (e.g., "https://central.enbox.id")
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

	// Create Centrifuge realtime node with JWT validation if configured
	var tokenValidator realtime.TokenValidator
	if cfg.TokenConfig != nil {
		tokenValidator = realtime.NewJWTValidator(cfg.TokenConfig)
	}

	rtNode, err := realtime.NewNode(realtime.Config{
		ClientQueueMaxSize: 2 * 1024 * 1024, // 2MB per client
		ClientChannelLimit: 128,
		TokenValidator:     tokenValidator,
	})
	if err != nil {
		fmt.Printf("Warning: failed to create realtime node: %v\n", err)
	} else {
		if err := rtNode.Run(); err != nil {
			fmt.Printf("Warning: failed to start realtime node: %v\n", err)
			rtNode = nil
		}
	}

	// Create broadcaster for publishing events
	broadcaster := realtime.NewBroadcaster(rtNode)

	// Initialize mesh client if configured
	var meshClient *mesh.Client
	if cfg.Mesh != nil && cfg.Mesh.Enabled {
		meshClient = mesh.NewClient(*cfg.Mesh)
	}

	// Initialize worker manager if encryption is configured
	var workerMgr *worker.Manager
	if cfg.Encryption != nil && cfg.Encryption.HQKeyPair != nil {
		workerMgr = worker.NewManager(database, cfg.Worker, cfg.Encryption.HQKeyPair)
	}

	// Initialize encrypted secrets store if master key is available
	var secretsStore *db.EncryptedSecretsStore
	if cfg.Encryption != nil && cfg.Encryption.MasterKey != nil {
		secretsStore = db.NewEncryptedSecretsStore(database, cfg.Encryption.MasterKey)
	}

	// Initialize Forgejo manager if configured
	var forgejoMgr *forgejo.Manager
	if cfg.Forgejo != nil {
		forgejoMgr = forgejo.NewManager(*cfg.Forgejo, database)
	}

	s := &Server{
		echo:           e,
		db:             database,
		toolbelt:       cfg.Toolbelt,
		taskService:    task.NewService(database),
		realtime:       rtNode,
		broadcaster:    broadcaster,
		meshClient:     meshClient,
		workerManager:  workerMgr,
		forgejoManager: forgejoMgr,
		encryption:     cfg.Encryption,
		addr:           cfg.Addr,
		certFile:       cfg.CertFile,
		keyFile:        cfg.KeyFile,
		tokenConfig:    cfg.TokenConfig,
		staticDir:      cfg.StaticDir,
		baseDir:        cfg.BaseDir,
		publicURL:      cfg.PublicURL,
		namespace:      cfg.Namespace,
		tunnelToken:    cfg.TunnelToken,
		centralURL:     cfg.CentralURL,
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
		sessionMgr.SetRepoManager(s.gitService.RepoManager())
		sessionMgr.SetGitService(s.gitService) // For worktree cleanup after merge
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
		HasGitHubApp:   database.HasGitHubApp,
		InitGitHubApp: func() error {
			// Reload toolbelt to pick up any new GitHub App config
			return s.ReloadToolbelt()
		},
		GetGitHubClient: func(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
			// Try to get GitHub client from toolbelt
			s.toolbeltMu.RLock()
			tb := s.toolbelt
			s.toolbeltMu.RUnlock()
			if tb != nil && tb.GitHub != nil {
				return tb.GitHub, nil
			}
			return nil, fmt.Errorf("GitHub client not configured")
		},
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
		ForgejoService: forgejoMgr,
		ForgejoOrg: func() string {
			if cfg.Forgejo != nil {
				return cfg.Forgejo.GetDefaultOrgName()
			}
			return ""
		}(),
	})

	// Create the Deps struct for dependency injection
	s.deps = &core.Deps{
		DB:             database,
		TaskService:    s.taskService,
		SessionManager: sessionMgr,
		GitService:     s.gitService,
		ForgejoManager: s.forgejoManager,
		Planner:        s.planner,
		QuestHandler:   s.questHandler,
		Realtime:       rtNode,
		Broadcaster:    broadcaster,
		MeshClient:     meshClient,
		WorkerManager:  workerMgr,
		SecretsStore:   secretsStore,
		TokenConfig:    cfg.TokenConfig,
		BaseDir:        cfg.BaseDir,
		GetToolbelt: func() *toolbelt.Toolbelt {
			s.toolbeltMu.RLock()
			defer s.toolbeltMu.RUnlock()
			return s.toolbelt
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
		IsValidGitRepo:     s.isValidGitRepo,
		IsValidProjectPath: s.isValidProjectPath,
	}

	// Create handler-level sync service (uses deps for cross-service coordination)
	s.handlersSyncSvc = issuesync.NewSyncService(s.deps)

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

	// Wire up worker manager callbacks for realtime updates
	if workerMgr != nil {
		workerMgr.SetCallbacks(
			// onProgress: broadcast worker progress updates
			func(objectiveID string, progress *worker.ProgressPayload) {
				if broadcaster != nil {
					broadcaster.PublishWorkerProgress(objectiveID, map[string]any{
						"objective_id":  objectiveID,
						"session_id":    progress.SessionID,
						"iteration":     progress.Iteration,
						"tokens_input":  progress.TokensInput,
						"tokens_output": progress.TokensOutput,
						"hat":           progress.Hat,
						"status":        progress.Status,
					})
				}
			},
			// onActivity: store activity events in DB
			func(events []*worker.ActivityEvent) {
				for _, evt := range events {
					tokensIn := evt.TokensInput
					tokensOut := evt.TokensOutput
					_, _ = database.CreateSessionActivity(
						evt.SessionID,
						evt.Iteration,
						evt.EventType,
						evt.Hat,
						evt.Content,
						&tokensIn,
						&tokensOut,
					)
				}
			},
			// onCompleted: handle task completion
			func(report *worker.CompletionReport) {
				// Update task status
				_ = database.UpdateTaskStatus(report.ObjectiveID, report.Status)

				// Broadcast completion
				if broadcaster != nil {
					broadcaster.PublishWorkerCompletion(report.ObjectiveID, map[string]any{
						"objective_id": report.ObjectiveID,
						"session_id":   report.SessionID,
						"status":       report.Status,
						"summary":      report.Summary,
						"pr_number":    report.PRNumber,
						"pr_url":       report.PRURL,
						"total_tokens": report.TotalTokens,
						"iterations":   report.Iterations,
					})
				}
			},
			// onFailed: handle task failure
			func(objectiveID, sessionID, errMsg string) {
				_ = database.UpdateTaskStatus(objectiveID, "failed")

				if broadcaster != nil {
					broadcaster.PublishWorkerFailed(objectiveID, map[string]any{
						"objective_id": objectiveID,
						"session_id":   sessionID,
						"error":        errMsg,
					})
				}
			},
		)
	}

	// Initialize OIDC handler if public URL is configured (for SSO)
	if cfg.PublicURL != "" {
		oidcHandler, err := authhandlers.NewOIDCHandler(s.deps, authhandlers.OIDCConfig{
			Issuer:   cfg.PublicURL,
			DataDir:  filepath.Join(cfg.BaseDir, "oidc"),
			LoginURL: "/login",
		})
		if err != nil {
			fmt.Printf("Warning: failed to create OIDC handler: %v\n", err)
		} else if oidcHandler != nil {
			s.oidcHandler = oidcHandler
			s.oidcLoginHandler = authhandlers.NewOIDCLoginHandler(oidcHandler)
		}
	}

	// Register routes
	s.registerRoutes()

	// Setup static file serving for frontend SPA
	// Uses embedded files by default, or disk files if StaticDir is specified
	s.setupStaticServing()

	return s
}

// registerRoutes sets up all API routes
func (s *Server) registerRoutes() {
	// API v1 group
	v1 := s.echo.Group("/api/v1")

	// Create handlers
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
	meshHandler := meshhandlers.New(s.deps)
	workersHandler := workershandlers.New(s.deps)
	forgejoHandler := forgejohandlers.New(s.deps)
	devicesHandler := deviceshandlers.New(s.deps, deviceshandlers.Config{
		Namespace:   s.namespace,
		TunnelToken: s.tunnelToken,
		CentralURL:  s.centralURL,
	})

	// Wire up callbacks for issue sync (Forgejo)
	questsHandler.SyncQuestToIssue = s.handlersSyncSvc.SyncQuestToIssue
	questsHandler.CloseQuestIssue = s.handlersSyncSvc.CloseQuestIssue
	questsHandler.ReopenQuestIssue = s.handlersSyncSvc.ReopenQuestIssue
	objectivesHandler.SyncObjectiveToIssue = s.handlersSyncSvc.SyncObjectiveToIssue

	// Public endpoints (no auth required)
	v1.GET("/system/status", s.handleHealthCheck)

	// Register public routes
	toolbeltHandler.RegisterPublicRoutes(v1)
	passkeyHandler.RegisterRoutes(v1)

	// Setup endpoints (for onboarding flow - public during initial setup)
	v1.GET("/setup/status", s.setupHandler.HandleStatus)
	v1.POST("/setup/anthropic-key", s.setupHandler.HandleSetAnthropicKey)
	v1.POST("/setup/complete", s.setupHandler.HandleComplete)
	v1.POST("/setup/workspace", s.setupHandler.HandleWorkspaceSetup)

	// New onboarding step endpoints
	v1.POST("/setup/steps/welcome", s.setupHandler.HandleAdvanceWelcome)
	v1.POST("/setup/steps/passkey", s.setupHandler.HandleCompletePasskey)
	v1.POST("/setup/steps/anthropic", s.setupHandler.HandleSetAnthropicKey)

	// Validation endpoints
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
	meshHandler.RegisterRoutes(protected)
	workersHandler.RegisterRoutes(protected)
	forgejoHandler.RegisterRoutes(protected)
	devicesHandler.RegisterRoutes(protected)

	// Centrifuge WebSocket endpoint for real-time updates
	// Auth is handled via Centrifuge protocol in Node.OnConnecting, not HTTP middleware
	if s.realtime != nil {
		v1.GET("/realtime", echo.WrapHandler(s.realtime.WebSocketHandler()))
	}

	// OIDC routes (root level per spec, not under /api/v1)
	// These enable HQ to act as an OIDC provider for SSO
	if s.oidcHandler != nil {
		s.oidcHandler.RegisterRoutes(s.echo)
		s.oidcLoginHandler.RegisterRoutes(s.echo)
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

// setupStaticServing configures static file serving for the frontend SPA.
// If staticDir is set, serves from disk. Otherwise uses embedded frontend assets.
func (s *Server) setupStaticServing() {
	if s.staticDir != "" {
		// Serve from disk (development mode or custom frontend)
		s.echo.Static("/assets", s.staticDir+"/assets")
		s.echo.File("/vite.svg", s.staticDir+"/vite.svg")

		// SPA fallback for disk-based serving
		s.echo.GET("/*", func(c echo.Context) error {
			path := c.Request().URL.Path
			if len(path) >= 4 && path[:4] == "/api" {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			return c.File(s.staticDir + "/index.html")
		})
		return
	}

	// Serve from embedded assets (production mode)
	// The frontend package embeds dist/ which contains the built React app
	distFS, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		// This shouldn't happen unless the embed failed at compile time
		fmt.Printf("Warning: failed to load embedded frontend: %v\n", err)
		return
	}

	assetHandler := http.FileServer(http.FS(distFS))

	// Serve /assets/* from embedded files
	s.echo.GET("/assets/*", echo.WrapHandler(http.StripPrefix("/", assetHandler)))

	// Serve vite.svg
	s.echo.GET("/vite.svg", echo.WrapHandler(assetHandler))

	// SPA fallback: serve index.html for all non-API routes
	s.echo.GET("/*", func(c echo.Context) error {
		path := c.Request().URL.Path
		if len(path) >= 4 && path[:4] == "/api" {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}

		// Serve embedded index.html
		indexFile, err := distFS.Open("index.html")
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "frontend not available")
		}
		defer func() { _ = indexFile.Close() }()

		return c.Stream(http.StatusOK, "text/html; charset=utf-8", indexFile)
	})
}

// Start begins serving HTTP/HTTPS requests
func (s *Server) Start() error {
	// Start worker manager if configured
	if s.workerManager != nil {
		ctx := context.Background()
		if err := s.workerManager.Start(ctx); err != nil {
			return fmt.Errorf("worker manager start failed: %w", err)
		}
		fmt.Println("Worker manager started")
	}

	// Start embedded Forgejo if configured
	if s.forgejoManager != nil {
		ctx := context.Background()
		if err := s.forgejoManager.Start(ctx); err != nil {
			return fmt.Errorf("forgejo start failed: %w", err)
		}
		// Pass Forgejo credentials to session manager for PR creation
		if s.sessionManager != nil {
			botToken, err := s.forgejoManager.BotToken()
			if err == nil {
				s.sessionManager.SetForgejoCredentials(s.forgejoManager.BaseURL(), botToken)
			} else {
				fmt.Printf("Warning: Forgejo started but bot token unavailable: %v\n", err)
			}
		}

		// Ensure dex-workspace repo exists in Forgejo (for existing installations)
		if err := s.forgejoManager.EnsureWorkspaceRepo(ctx); err != nil {
			fmt.Printf("Warning: failed to ensure workspace repo in Forgejo: %v\n", err)
		}

		// Register Forgejo as OIDC client if both OIDC and OAuth are configured
		if s.oidcHandler != nil {
			if err := s.registerForgejoOIDCClient(); err != nil {
				fmt.Printf("Warning: failed to register Forgejo as OIDC client: %v\n", err)
			}
		}
	}

	// Start HTTP server FIRST in a goroutine, before mesh/tunnel
	// This ensures the local services are listening before the tunnel starts routing traffic
	httpErr := make(chan error, 1)

	go func() {
		var err error
		if s.certFile != "" && s.keyFile != "" {
			fmt.Printf("Starting HTTPS server on %s\n", s.addr)
			err = s.echo.StartTLS(s.addr, s.certFile, s.keyFile)
		} else {
			fmt.Printf("Starting HTTP server on %s\n", s.addr)
			err = s.echo.Start(s.addr)
		}
		// Always send the result (even http.ErrServerClosed for clean shutdown)
		httpErr <- err
	}()

	// Wait for HTTP server to be ready by polling the port
	httpAddr := s.addr
	if httpAddr == "" || httpAddr[0] == ':' {
		httpAddr = "127.0.0.1" + s.addr
	}
	httpReady := false
	for i := 0; i < 50; i++ { // Try for up to 5 seconds
		select {
		case err := <-httpErr:
			// Server failed to start
			return err
		default:
		}
		conn, err := net.DialTimeout("tcp", httpAddr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			httpReady = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !httpReady {
		return fmt.Errorf("HTTP server failed to start within timeout")
	}
	fmt.Printf("HTTP server ready on %s\n", s.addr)

	// Start mesh client AFTER HTTP server is ready
	if s.meshClient != nil {
		ctx := context.Background()
		if err := s.meshClient.Start(ctx); err != nil {
			return fmt.Errorf("mesh client start failed: %w", err)
		}
		fmt.Println("Mesh networking started")
	}

	// Setup Forgejo SSO provider AFTER HTTP is ready (needs OIDC discovery to be reachable)
	if s.forgejoManager != nil && s.forgejoManager.IsRunning() {
		ctx := context.Background()
		if err := s.forgejoManager.SetupSSOProvider(ctx, s.publicURL); err != nil {
			fmt.Printf("Warning: failed to setup Forgejo SSO: %v\n", err)
		}
	}

	// Expose services on mesh network if both mesh and services are available
	if s.meshClient != nil && s.meshClient.IsRunning() {
		sp := mesh.NewServiceProxy(s.meshClient)
		s.meshProxy = sp

		// Expose Forgejo on mesh port 3000
		if s.forgejoManager != nil && s.forgejoManager.IsRunning() {
			if err := sp.Expose("forgejo", 3000, s.forgejoManager.BaseURL()); err != nil {
				fmt.Printf("Warning: failed to expose Forgejo on mesh: %v\n", err)
			}
		}

		// Expose Dex API on mesh port 8080
		dexAddr := s.addr
		if dexAddr == "" || dexAddr[0] == ':' {
			dexAddr = "127.0.0.1" + dexAddr
		}
		dexScheme := "http"
		if s.certFile != "" {
			dexScheme = "https"
		}
		if err := sp.Expose("dex-api", 8080, fmt.Sprintf("%s://%s", dexScheme, dexAddr)); err != nil {
			fmt.Printf("Warning: failed to expose Dex API on mesh: %v\n", err)
		}
	}

	// Block waiting for HTTP server to finish (error or clean shutdown)
	err := <-httpErr
	if err == http.ErrServerClosed {
		return nil // Clean shutdown
	}
	return err
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	// Stop worker manager
	if s.workerManager != nil {
		if err := s.workerManager.Stop(ctx); err != nil {
			fmt.Printf("Warning: failed to stop worker manager: %v\n", err)
		}
	}

	// Stop mesh proxy first (before stopping the services it proxies)
	if s.meshProxy != nil {
		s.meshProxy.Stop()
	}

	// Stop embedded Forgejo
	if s.forgejoManager != nil {
		if err := s.forgejoManager.Stop(); err != nil {
			fmt.Printf("Warning: failed to stop forgejo: %v\n", err)
		}
	}

	// Stop mesh client
	if s.meshClient != nil {
		if err := s.meshClient.Stop(); err != nil {
			fmt.Printf("Warning: failed to stop mesh client: %v\n", err)
		}
	}

	// Shutdown the realtime node
	if s.realtime != nil {
		if err := s.realtime.Shutdown(ctx); err != nil {
			fmt.Printf("Warning: failed to shutdown realtime node: %v\n", err)
		}
	}
	return s.echo.Shutdown(ctx)
}

// registerForgejoOIDCClient registers Forgejo as an OIDC client with HQ's OIDC provider.
// This allows Forgejo to authenticate users via HQ's passkey-based login.
func (s *Server) registerForgejoOIDCClient() error {
	// Ensure OAuth secret exists (generate if needed for existing installations)
	if err := s.forgejoManager.EnsureOAuthSecret(); err != nil {
		return fmt.Errorf("failed to ensure OAuth secret: %w", err)
	}

	oauthSecret, err := s.forgejoManager.OAuthSecret()
	if err != nil {
		return fmt.Errorf("OAuth secret not available: %w", err)
	}

	// Build the redirect URI based on Forgejo's root URL
	// Forgejo uses /user/oauth2/{provider}/callback as the callback path
	forgejoConfig := s.forgejoManager.Config()
	rootURL := forgejoConfig.RootURL
	if rootURL == "" {
		rootURL = s.forgejoManager.BaseURL()
	}
	callbackURL := rootURL + "/user/oauth2/hq/callback"

	// Register Forgejo as an OIDC client
	client := &oidc.Client{
		ID:           forgejo.OAuthClientID,
		Secret:       oauthSecret,
		RedirectURIs: []string{callbackURL},
		Name:         "Forgejo",
	}

	if err := s.oidcHandler.RegisterClient(client); err != nil {
		return fmt.Errorf("failed to register OIDC client: %w", err)
	}

	fmt.Printf("Registered Forgejo as OIDC client (callback: %s)\n", callbackURL)
	return nil
}
