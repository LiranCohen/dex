// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/orchestrator"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// SessionState represents the current state of a session
type SessionState string

const (
	StateCreated   SessionState = "created"
	StateStarting  SessionState = "starting"
	StateRunning   SessionState = "running"
	StatePaused    SessionState = "paused"
	StateStopping  SessionState = "stopping"
	StateStopped   SessionState = "stopped"
	StateCompleted SessionState = "completed"
	StateFailed    SessionState = "failed"
)

// ActiveSession represents a session currently managed by the Manager
type ActiveSession struct {
	ID           string
	TaskID       string
	Hat          string
	State        SessionState
	WorktreePath string

	IterationCount int
	MaxIterations  int

	InputTokens   int64   // Total input tokens used
	OutputTokens  int64   // Total output tokens used
	InputRate     float64 // $/MTok for input (captured at session start)
	OutputRate    float64 // $/MTok for output (captured at session start)
	TokensBudget  *int64
	DollarsBudget *float64

	StartedAt    time.Time
	LastActivity time.Time

	// For cancellation
	cancel context.CancelFunc
	done   chan struct{}
}

// TotalTokens returns the combined input + output tokens
func (s *ActiveSession) TotalTokens() int64 {
	return s.InputTokens + s.OutputTokens
}

// Cost calculates the session cost from tokens and rates
func (s *ActiveSession) Cost() float64 {
	inputCost := float64(s.InputTokens) * s.InputRate / 1_000_000
	outputCost := float64(s.OutputTokens) * s.OutputRate / 1_000_000
	return inputCost + outputCost
}

// Manager manages Claude Code session lifecycle
// GitHubClientFetcher is a function that returns a GitHub client for a given login/org
// This allows the session manager to get installation-specific clients for GitHub Apps
type GitHubClientFetcher func(ctx context.Context, login string) (*toolbelt.GitHubClient, error)

// TaskCompletedCallback is called when a task completes (for GitHub sync)
type TaskCompletedCallback func(taskID string)

// TaskFailedCallback is called when a task fails or is cancelled (for GitHub sync)
type TaskFailedCallback func(taskID string, reason string)

// PRCreatedCallback is called when a PR is created for a task (for GitHub sync)
type PRCreatedCallback func(taskID string, prNumber int)

// ChecklistUpdatedCallback is called when a checklist item is updated (for GitHub sync)
type ChecklistUpdatedCallback func(taskID string)

// TaskStatusCallback is called when a task status changes (for GitHub sync)
type TaskStatusCallback func(taskID string, status string)

type Manager struct {
	db           *db.DB
	scheduler    *orchestrator.Scheduler
	promptLoader *PromptLoader

	// External dependencies for Ralph loop
	anthropicClient   *toolbelt.AnthropicClient
	wsHub             *websocket.Hub
	transitionHandler *orchestrator.TransitionHandler

	// Git and GitHub for PR creation on completion
	gitOps              *git.Operations
	githubClient        *toolbelt.GitHubClient // Static client (PAT-based)
	githubClientFetcher GitHubClientFetcher    // Dynamic client fetcher (GitHub App)

	// Event callbacks for GitHub sync
	onTaskCompleted    TaskCompletedCallback
	onTaskFailed       TaskFailedCallback
	onPRCreated        PRCreatedCallback
	onChecklistUpdated ChecklistUpdatedCallback
	onTaskStatus       TaskStatusCallback

	mu       sync.RWMutex
	sessions map[string]*ActiveSession // sessionID -> session
	byTask   map[string]string         // taskID -> sessionID

	// Configuration
	defaultMaxIterations int
	defaultTokenBudget   *int64
	defaultDollarBudget  *float64
}

// NewManager creates a session manager
func NewManager(database *db.DB, scheduler *orchestrator.Scheduler, promptsDir string) *Manager {
	loader := NewPromptLoader(promptsDir)
	// Load templates (log error but don't fail - prompts may not exist yet)
	if err := loader.LoadAll(); err != nil {
		fmt.Printf("warning: failed to load prompts: %v\n", err)
	}

	return &Manager{
		db:                   database,
		scheduler:            scheduler,
		promptLoader:         loader,
		sessions:             make(map[string]*ActiveSession),
		byTask:               make(map[string]string),
		defaultMaxIterations: 100,
	}
}

// GetPromptLoader returns the prompt loader for external use (e.g., quest handler)
func (m *Manager) GetPromptLoader() *PromptLoader {
	return m.promptLoader
}

// SetDefaults configures default budget limits for new sessions
func (m *Manager) SetDefaults(maxIterations int, tokenBudget *int64, dollarBudgetFloat *float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.defaultMaxIterations = maxIterations
	m.defaultTokenBudget = tokenBudget
	m.defaultDollarBudget = dollarBudgetFloat
}

// SetAnthropicClient sets the Anthropic client for the Ralph loop
func (m *Manager) SetAnthropicClient(client *toolbelt.AnthropicClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.anthropicClient = client
}

// SetWebSocketHub sets the WebSocket hub for broadcasting events
func (m *Manager) SetWebSocketHub(hub *websocket.Hub) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wsHub = hub
}

// SetTransitionHandler sets the transition handler for hat transitions
func (m *Manager) SetTransitionHandler(handler *orchestrator.TransitionHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transitionHandler = handler
}

// SetGitOperations sets the git operations for pushing branches
func (m *Manager) SetGitOperations(ops *git.Operations) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gitOps = ops
}

// SetGitHubClient sets the GitHub client for creating PRs
func (m *Manager) SetGitHubClient(client *toolbelt.GitHubClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.githubClient = client
}

// SetGitHubClientFetcher sets a function to dynamically fetch GitHub clients
// This is used for GitHub App installations where each org/user has a separate client
func (m *Manager) SetGitHubClientFetcher(fetcher GitHubClientFetcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.githubClientFetcher = fetcher
}

// SetOnTaskCompleted sets a callback for task completion events (for GitHub sync)
func (m *Manager) SetOnTaskCompleted(callback TaskCompletedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTaskCompleted = callback
}

// SetOnPRCreated sets a callback for PR creation events (for GitHub sync)
func (m *Manager) SetOnPRCreated(callback PRCreatedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPRCreated = callback
}

// SetOnTaskFailed sets a callback for task failure events (for GitHub sync)
func (m *Manager) SetOnTaskFailed(callback TaskFailedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTaskFailed = callback
}

// SetOnChecklistUpdated sets a callback for checklist update events (for GitHub sync)
func (m *Manager) SetOnChecklistUpdated(callback ChecklistUpdatedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChecklistUpdated = callback
}

// NotifyChecklistUpdated is called by ralph loop when a checklist item is updated
func (m *Manager) NotifyChecklistUpdated(taskID string) {
	m.mu.RLock()
	callback := m.onChecklistUpdated
	m.mu.RUnlock()
	if callback != nil {
		go callback(taskID)
	}
}

// SetOnTaskStatus sets a callback for task status change events (for GitHub sync)
func (m *Manager) SetOnTaskStatus(callback TaskStatusCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTaskStatus = callback
}

// notifyTaskStatus notifies listeners of a task status change
func (m *Manager) notifyTaskStatus(taskID string, status string) {
	m.mu.RLock()
	callback := m.onTaskStatus
	m.mu.RUnlock()
	if callback != nil {
		go callback(taskID, status)
	}
}

// CreateSession creates a new session for a task
// Does NOT start the session - call Start() separately
func (m *Manager) CreateSession(taskID, hat, worktreePath string) (*ActiveSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if task already has a session
	if existingID, exists := m.byTask[taskID]; exists {
		return nil, fmt.Errorf("task %s already has session %s", taskID, existingID)
	}

	// Create session record in DB
	dbSession, err := m.db.CreateSession(taskID, hat, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session in db: %w", err)
	}

	// Create active session
	session := &ActiveSession{
		ID:            dbSession.ID,
		TaskID:        taskID,
		Hat:           hat,
		State:         StateCreated,
		WorktreePath:  worktreePath,
		MaxIterations: m.defaultMaxIterations,
		TokensBudget:  m.defaultTokenBudget,
		DollarsBudget: m.defaultDollarBudget,
		done:          make(chan struct{}),
	}

	m.sessions[session.ID] = session
	m.byTask[taskID] = session.ID

	return session, nil
}

// Start begins executing a session
// Returns immediately - session runs in background
func (m *Manager) Start(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.State != StateCreated && session.State != StatePaused {
		m.mu.Unlock()
		return fmt.Errorf("session %s cannot be started from state %s", sessionID, session.State)
	}

	session.State = StateStarting
	session.StartedAt = time.Now()
	session.LastActivity = time.Now()

	// Create cancellable context
	sessionCtx, cancel := context.WithCancel(ctx)
	session.cancel = cancel
	m.mu.Unlock()

	// Update DB
	if err := m.db.UpdateSessionStatus(sessionID, string(StateRunning)); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Notify task started (for GitHub sync)
	m.notifyTaskStatus(session.TaskID, "running")

	// Launch session in background
	go m.runSession(sessionCtx, session)

	return nil
}

// Stop gracefully stops a session
func (m *Manager) Stop(sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.State != StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("session %s is not running (state: %s)", sessionID, session.State)
	}

	session.State = StateStopping
	if session.cancel != nil {
		session.cancel()
	}
	m.mu.Unlock()

	// Wait for session to stop (with timeout)
	select {
	case <-session.done:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for session %s to stop", sessionID)
	}
}

// Pause pauses a session (can be resumed later)
func (m *Manager) Pause(sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.State != StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("session %s cannot be paused from state %s", sessionID, session.State)
	}

	session.State = StatePaused
	taskID := session.TaskID
	if session.cancel != nil {
		session.cancel()
	}
	m.mu.Unlock()

	// Update DB
	if err := m.db.UpdateSessionStatus(sessionID, string(StatePaused)); err != nil {
		return err
	}

	// Notify task paused (for GitHub sync)
	m.notifyTaskStatus(taskID, "paused")

	return nil
}

// Get returns an active session by ID
func (m *Manager) Get(sessionID string) *ActiveSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, exists := m.sessions[sessionID]; exists {
		// Return a copy to prevent external modification
		return m.copySession(session)
	}
	return nil
}

// GetByTask returns the active session for a task
func (m *Manager) GetByTask(taskID string) *ActiveSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if sessionID, exists := m.byTask[taskID]; exists {
		if session, exists := m.sessions[sessionID]; exists {
			return m.copySession(session)
		}
	}
	return nil
}

// List returns all active sessions
func (m *Manager) List() []*ActiveSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ActiveSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		result = append(result, m.copySession(session))
	}
	return result
}

// ActiveCount returns the number of active sessions
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, s := range m.sessions {
		if s.State == StateRunning || s.State == StateStarting {
			count++
		}
	}
	return count
}

// copySession creates a copy of a session to prevent external modification
func (m *Manager) copySession(s *ActiveSession) *ActiveSession {
	copy := &ActiveSession{
		ID:             s.ID,
		TaskID:         s.TaskID,
		Hat:            s.Hat,
		State:          s.State,
		WorktreePath:   s.WorktreePath,
		IterationCount: s.IterationCount,
		MaxIterations:  s.MaxIterations,
		InputTokens:    s.InputTokens,
		OutputTokens:   s.OutputTokens,
		InputRate:      s.InputRate,
		OutputRate:     s.OutputRate,
		StartedAt:      s.StartedAt,
		LastActivity:   s.LastActivity,
	}
	// Copy pointers by creating new pointers to the same values
	if s.TokensBudget != nil {
		v := *s.TokensBudget
		copy.TokensBudget = &v
	}
	if s.DollarsBudget != nil {
		v := *s.DollarsBudget
		copy.DollarsBudget = &v
	}
	return copy
}

// runSession is the main session execution loop (Ralph loop)
func (m *Manager) runSession(ctx context.Context, session *ActiveSession) {
	defer close(session.done)

	m.mu.Lock()
	session.State = StateRunning
	anthropicClient := m.anthropicClient
	wsHub := m.wsHub
	originalHat := session.Hat
	m.mu.Unlock()

	fmt.Printf("runSession: starting session %s for task %s (hat: %s)\n", session.ID, session.TaskID, session.Hat)

	var loopErr error

	// Run the Ralph loop if we have an Anthropic client
	if anthropicClient != nil {
		fmt.Printf("runSession: Anthropic client is configured, starting Ralph loop\n")
		loop := NewRalphLoop(m, session, anthropicClient, wsHub, m.db)

		// Get task and project for tool executor context
		task, err := m.db.GetTaskByID(session.TaskID)
		if err != nil {
			fmt.Printf("runSession: warning - failed to get task for executor: %v\n", err)
		}
		if task != nil {
			// Set the AI model to use based on task complexity
			if task.Model.Valid && task.Model.String != "" {
				loop.SetModel(task.Model.String)
				fmt.Printf("runSession: using model %s for task %s\n", task.Model.String, task.ID)
			}

			project, err := m.db.GetProjectByID(task.ProjectID)
			if err != nil {
				fmt.Printf("runSession: warning - failed to get project for executor: %v\n", err)
			}
			if project != nil {
				var owner, repo string
				if project.GitHubOwner.Valid {
					owner = project.GitHubOwner.String
				}
				if project.GitHubRepo.Valid {
					repo = project.GitHubRepo.String
				}

				// Get GitHub client - try static client first, then fetcher
				githubClient := m.githubClient
				if githubClient == nil && m.githubClientFetcher != nil {
					// Try to get a client from the fetcher (e.g., GitHub App installation)
					// If owner is empty, the fetcher will use the first available installation
					fetchedClient, err := m.githubClientFetcher(ctx, owner)
					if err != nil {
						fmt.Printf("runSession: warning - failed to fetch GitHub client for %q: %v\n", owner, err)
					} else {
						githubClient = fetchedClient
						fmt.Printf("runSession: using GitHub App client for %q\n", owner)
					}
				}

				loop.InitExecutor(session.WorktreePath, m.gitOps, githubClient, owner, repo)
				fmt.Printf("runSession: initialized tool executor (owner=%s, repo=%s, hasGitHub=%v)\n", owner, repo, githubClient != nil)
			}
		}

		// Try to restore from checkpoint
		checkpoint, err := m.db.GetLatestSessionCheckpoint(session.ID)
		if err == nil && checkpoint != nil {
			if restoreErr := loop.RestoreFromCheckpoint(checkpoint); restoreErr != nil {
				fmt.Printf("warning: failed to restore checkpoint: %v\n", restoreErr)
			}
		}

		// Run the loop
		loopErr = loop.Run(ctx)
		if loopErr != nil {
			fmt.Printf("runSession: Ralph loop ended with error: %v\n", loopErr)
		} else {
			fmt.Printf("runSession: Ralph loop completed successfully\n")
		}
	} else {
		// Fallback: wait for cancellation if no client
		fmt.Printf("runSession: WARNING - No Anthropic client configured! Session will wait for cancellation.\n")
		<-ctx.Done()
		loopErr = ctx.Err()
	}

	// Determine final state based on error
	m.mu.Lock()
	nextHat := session.Hat
	hatTransition := loopErr == nil && nextHat != originalHat
	worktreePath := session.WorktreePath
	taskID := session.TaskID
	sessionID := session.ID

	if session.State == StateStopping {
		session.State = StateStopped
	} else if session.State == StatePaused {
		// Keep paused state
	} else if loopErr != nil {
		// Check if it's a budget error (requires approval, not a failure)
		if loopErr == ErrBudgetExceeded || loopErr == ErrIterationLimit ||
			loopErr == ErrTokenBudget || loopErr == ErrDollarBudget {
			session.State = StatePaused
		} else if loopErr == context.Canceled {
			session.State = StateStopped
		} else {
			session.State = StateFailed
		}
	} else {
		session.State = StateCompleted
	}
	finalState := session.State
	m.mu.Unlock()

	// Update DB with final state and outcome
	_ = m.db.UpdateSessionStatus(sessionID, string(finalState))

	// Handle hat transition: create and start new session with next hat
	if hatTransition {
		m.handleHatTransition(ctx, taskID, originalHat, nextHat, worktreePath)
		return
	}

	// If no transition, clean up normally
	m.mu.Lock()
	delete(m.sessions, sessionID)
	delete(m.byTask, taskID)
	m.mu.Unlock()

	// Update task status based on final state
	switch finalState {
	case StateCompleted:
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusCompleted)

		// Notify task completed (for GitHub sync)
		m.mu.RLock()
		onTaskCompleted := m.onTaskCompleted
		m.mu.RUnlock()
		if onTaskCompleted != nil {
			go onTaskCompleted(taskID)
		}

		// Push branch and create PR (non-blocking, log errors)
		go m.createPRForTask(taskID, worktreePath)

	case StateFailed:
		// Mark task as paused so it can be resumed after fixing the issue
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusPaused)

		// Notify with error status (adds comment to GitHub issue, doesn't close it)
		reason := "Session failed"
		if loopErr != nil {
			reason = loopErr.Error()
		}
		m.notifyTaskStatus(taskID, "error:"+reason)

	case StatePaused, StateStopped:
		// Mark task as paused so it can be resumed
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusPaused)
		m.notifyTaskStatus(taskID, "paused")
	}
}

// handleHatTransition handles transitioning a task to a new hat
func (m *Manager) handleHatTransition(ctx context.Context, taskID, originalHat, nextHat, worktreePath string) {
	// Validate transition BEFORE removing old session
	m.mu.RLock()
	handler := m.transitionHandler
	oldSessionID := m.byTask[taskID]
	m.mu.RUnlock()

	if handler != nil && !handler.ValidateTransition(originalHat, nextHat) {
		fmt.Printf("warning: invalid hat transition from %s to %s, marking task failed\n", originalHat, nextHat)
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusCancelled)
		return
	}

	// Now safe to remove old session
	m.mu.Lock()
	delete(m.sessions, oldSessionID)
	delete(m.byTask, taskID)
	m.mu.Unlock()

	// Create new session with next hat
	newSession, err := m.CreateSession(taskID, nextHat, worktreePath)
	if err != nil {
		fmt.Printf("error: failed to create session for hat transition: %v\n", err)
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusCancelled)
		return
	}

	// Start the new session
	if err := m.Start(ctx, newSession.ID); err != nil {
		fmt.Printf("error: failed to start session for hat transition: %v\n", err)
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusCancelled)
		return
	}

	fmt.Printf("hat transition: task %s transitioned from %s to %s (session %s)\n", taskID, originalHat, nextHat, newSession.ID)
}

// GetPrompt returns the rendered prompt for a session's hat
func (m *Manager) GetPrompt(sessionID string) (string, error) {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.RUnlock()
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	taskID := session.TaskID
	hat := session.Hat
	sessionCopy := m.copySession(session)
	m.mu.RUnlock()

	// Get task from DB
	task, err := m.db.GetTaskByID(taskID)
	if err != nil {
		return "", fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	ctx := &PromptContext{
		Task:    task,
		Session: sessionCopy,
	}

	return m.promptLoader.Get(hat, ctx)
}

// LoadActiveSessions loads any active sessions from the database on startup
// This allows recovery after a restart
func (m *Manager) LoadActiveSessions() error {
	sessions, err := m.db.ListActiveSessions()
	if err != nil {
		return fmt.Errorf("failed to load active sessions: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dbSession := range sessions {
		// Map DB status to session state
		var state SessionState
		switch dbSession.Status {
		case db.SessionStatusRunning:
			state = StateRunning
		case db.SessionStatusPaused:
			state = StatePaused
		default:
			continue // Skip unknown states
		}

		session := &ActiveSession{
			ID:             dbSession.ID,
			TaskID:         dbSession.TaskID,
			Hat:            dbSession.Hat,
			State:          state,
			WorktreePath:   dbSession.WorktreePath,
			IterationCount: dbSession.IterationCount,
			MaxIterations:  dbSession.MaxIterations,
			InputTokens:    dbSession.InputTokens,
			OutputTokens:   dbSession.OutputTokens,
			InputRate:      dbSession.InputRate,
			OutputRate:     dbSession.OutputRate,
			done:           make(chan struct{}),
		}

		if dbSession.TokensBudget.Valid {
			v := dbSession.TokensBudget.Int64
			session.TokensBudget = &v
		}
		if dbSession.DollarsBudget.Valid {
			v := dbSession.DollarsBudget.Float64
			session.DollarsBudget = &v
		}
		if dbSession.StartedAt.Valid {
			session.StartedAt = dbSession.StartedAt.Time
		}

		m.sessions[session.ID] = session
		m.byTask[session.TaskID] = session.ID
	}

	return nil
}

// createPRForTask pushes the branch and creates a PR after task completion
// This runs in a goroutine and logs errors without failing the session
func (m *Manager) createPRForTask(taskID, worktreePath string) {
	m.mu.RLock()
	gitOps := m.gitOps
	githubClient := m.githubClient
	m.mu.RUnlock()

	// Get task from DB
	task, err := m.db.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("createPRForTask: failed to get task %s: %v\n", taskID, err)
		return
	}

	// Get project from DB to find GitHub owner/repo
	project, err := m.db.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("createPRForTask: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	// Check if project has GitHub configured
	if !project.GitHubOwner.Valid || !project.GitHubRepo.Valid {
		fmt.Printf("createPRForTask: project %s has no GitHub owner/repo configured\n", project.ID)
		return
	}

	owner := project.GitHubOwner.String
	repo := project.GitHubRepo.String

	// Get current branch name from worktree
	if gitOps == nil {
		fmt.Printf("createPRForTask: no git operations configured\n")
		return
	}

	branchName, err := gitOps.GetCurrentBranch(worktreePath)
	if err != nil {
		fmt.Printf("createPRForTask: failed to get current branch for task %s: %v\n", taskID, err)
		return
	}

	// Push the branch to remote
	pushOpts := git.PushOptions{
		Remote:      "origin",
		Branch:      branchName,
		SetUpstream: true,
	}
	if err := gitOps.Push(worktreePath, pushOpts); err != nil {
		fmt.Printf("createPRForTask: failed to push branch %s for task %s: %v\n", branchName, taskID, err)
		return
	}
	fmt.Printf("createPRForTask: pushed branch %s for task %s\n", branchName, taskID)

	// Create PR via GitHub client if configured
	if githubClient == nil {
		fmt.Printf("createPRForTask: no GitHub client configured, skipping PR creation\n")
		return
	}

	prOpts := toolbelt.CreatePROptions{
		Owner: owner,
		Repo:  repo,
		Title: task.Title,
		Body:  fmt.Sprintf("Closes task: %s\n\n%s", taskID, task.GetDescription()),
		Head:  branchName,
		Base:  project.DefaultBranch,
		Draft: false,
	}

	pr, err := githubClient.CreatePR(context.Background(), prOpts)
	if err != nil {
		fmt.Printf("createPRForTask: failed to create PR for task %s: %v\n", taskID, err)
		return
	}

	// Update task with PR number
	if pr.Number != nil {
		if err := m.db.UpdateTaskPRNumber(taskID, *pr.Number); err != nil {
			fmt.Printf("createPRForTask: failed to update task %s with PR number: %v\n", taskID, err)
			return
		}
		fmt.Printf("createPRForTask: created PR #%d for task %s\n", *pr.Number, taskID)

		// Notify PR created (for GitHub sync)
		m.mu.RLock()
		onPRCreated := m.onPRCreated
		m.mu.RUnlock()
		if onPRCreated != nil {
			onPRCreated(taskID, *pr.Number)
		}
	}
}
