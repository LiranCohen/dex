// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lirancohen/dex/internal/centralmail"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/gitprovider"
	forgejoclient "github.com/lirancohen/dex/internal/gitprovider/forgejo"
	"github.com/lirancohen/dex/internal/orchestrator"
	"github.com/lirancohen/dex/internal/realtime"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
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
	ProjectID    string
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
	MaxRuntime    time.Duration // Maximum runtime before termination (0 = unlimited)

	StartedAt    time.Time
	LastActivity time.Time

	// Scratchpad: persistent thinking document updated each iteration
	// Stores task understanding, plan, decisions, blockers, and last action
	Scratchpad string

	// PredecessorContext: handoff from a completed predecessor task in a dependency chain
	// Contains summary of what the predecessor accomplished and context for continuation
	PredecessorContext string

	// For resuming from a previous session's checkpoint
	RestoreFromSessionID string

	// Termination tracking (persisted to DB when session ends)
	TerminationReason   string // Why the session ended (e.g., "completed", "max_iterations", "quality_gate_exhausted")
	QualityGateAttempts int    // Number of quality gate validation attempts

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

// TaskCompletedCallback is called when a task completes (for issue sync)
type TaskCompletedCallback func(taskID string)

// TaskFailedCallback is called when a task fails or is cancelled (for issue sync)
type TaskFailedCallback func(taskID string, reason string)

// PRCreatedCallback is called when a PR is created for a task (for issue sync)
type PRCreatedCallback func(taskID string, prNumber int)

// ChecklistUpdatedCallback is called when a checklist item is updated (for issue sync)
type ChecklistUpdatedCallback func(taskID string)

// TaskStatusCallback is called when a task status changes (for issue sync)
type TaskStatusCallback func(taskID string, status string)

type Manager struct {
	db           *db.DB
	scheduler    *orchestrator.Scheduler
	promptLoader *PromptLoader

	// External dependencies for Ralph loop
	anthropicClient *toolbelt.AnthropicClient
	broadcaster     *realtime.Broadcaster // Publishes to both legacy and new systems

	// Central mail/calendar proxy (for MailExecutor in AI sessions)
	centralURL  string
	tunnelToken string

	// Git and Forgejo for PR creation on completion
	gitOps          *git.Operations
	gitService      *git.Service     // For worktree cleanup after merge
	repoManager     *git.RepoManager // For cloning repos to permanent location
	forgejoBaseURL  string           // Forgejo API base URL (e.g., http://127.0.0.1:3000)
	forgejoBotToken string           // Forgejo bot account API token

	// Event callbacks for issue sync
	onTaskCompleted    TaskCompletedCallback
	onTaskFailed       TaskFailedCallback
	onPRCreated        PRCreatedCallback
	onChecklistUpdated ChecklistUpdatedCallback
	onTaskStatus       TaskStatusCallback

	mu       sync.RWMutex
	sessions map[string]*ActiveSession // sessionID -> session
	byTask   map[string]string         // taskID -> sessionID

	// Transition tracking for loop detection (per task)
	transitionTrackers map[string]*TransitionTracker // taskID -> tracker

	// Configuration
	defaultMaxIterations int
	defaultTokenBudget   *int64
	defaultDollarBudget  *float64
	defaultMaxRuntime    time.Duration
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
		transitionTrackers:   make(map[string]*TransitionTracker),
		defaultMaxIterations: 100,
		defaultMaxRuntime:    4 * time.Hour, // Default: 4 hours
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

// SetMaxRuntime configures the default max runtime for new sessions
func (m *Manager) SetMaxRuntime(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultMaxRuntime = d
}

// SetAnthropicClient sets the Anthropic client for the Ralph loop
func (m *Manager) SetAnthropicClient(client *toolbelt.AnthropicClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.anthropicClient = client
}

// SetBroadcaster sets the broadcaster for publishing to both legacy and new systems
func (m *Manager) SetBroadcaster(broadcaster *realtime.Broadcaster) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcaster = broadcaster
}

// SetGitOperations sets the git operations for pushing branches
func (m *Manager) SetGitOperations(ops *git.Operations) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gitOps = ops
}

// SetRepoManager sets the repo manager for cloning repos to permanent location
func (m *Manager) SetRepoManager(rm *git.RepoManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repoManager = rm
}

// SetGitService sets the git service for worktree cleanup after merge
func (m *Manager) SetGitService(svc *git.Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gitService = svc
}

// SetOnTaskCompleted sets a callback for task completion events (for issue sync)
func (m *Manager) SetOnTaskCompleted(callback TaskCompletedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTaskCompleted = callback
}

// SetOnPRCreated sets a callback for PR creation events (for issue sync)
func (m *Manager) SetOnPRCreated(callback PRCreatedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPRCreated = callback
}

// SetOnTaskFailed sets a callback for task failure events (for issue sync)
func (m *Manager) SetOnTaskFailed(callback TaskFailedCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTaskFailed = callback
}

// SetOnChecklistUpdated sets a callback for checklist update events (for issue sync)
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

// SetOnTaskStatus sets a callback for task status change events (for issue sync)
func (m *Manager) SetOnTaskStatus(callback TaskStatusCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTaskStatus = callback
}

// SetForgejoCredentials sets the Forgejo API credentials for PR creation.
func (m *Manager) SetForgejoCredentials(baseURL, botToken string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forgejoBaseURL = baseURL
	m.forgejoBotToken = botToken
}

// SetMailConfig sets the Central URL and tunnel token for mail/calendar tool access.
// When set, AI sessions can use mail_* and calendar_* tools via Central's Zoho proxy.
func (m *Manager) SetMailConfig(centralURL, tunnelToken string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.centralURL = centralURL
	m.tunnelToken = tunnelToken
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

// broadcastTaskUpdated sends a task.updated WebSocket event
func (m *Manager) broadcastTaskUpdated(taskID string, status string) {
	m.mu.RLock()
	broadcaster := m.broadcaster
	m.mu.RUnlock()

	if broadcaster != nil {
		payload := map[string]any{
			"status": status,
		}
		// Include project_id for channel routing
		if task, err := m.db.GetTaskByID(taskID); err == nil && task != nil {
			payload["project_id"] = task.ProjectID
		}
		broadcaster.PublishTaskEvent(realtime.EventTaskUpdated, taskID, payload)
	}
}

// SetPredecessorContext sets the context from a predecessor task in a dependency chain
// This should be called after CreateSession but before Start
func (m *Manager) SetPredecessorContext(sessionID string, context string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, exists := m.sessions[sessionID]; exists {
		session.PredecessorContext = context
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

	// Get task to retrieve project_id for channel routing
	task, err := m.db.GetTaskByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
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
		ProjectID:     task.ProjectID,
		Hat:           hat,
		State:         StateCreated,
		WorktreePath:  worktreePath,
		MaxIterations: m.defaultMaxIterations,
		TokensBudget:  m.defaultTokenBudget,
		DollarsBudget: m.defaultDollarBudget,
		MaxRuntime:    m.defaultMaxRuntime,
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

	// Notify task started (for issue sync)
	m.notifyTaskStatus(session.TaskID, "running")

	// Broadcast task.updated event for WebSocket clients
	m.broadcastTaskUpdated(session.TaskID, db.TaskStatusRunning)

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

	// Notify task paused (for issue sync)
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
		ID:                  s.ID,
		TaskID:              s.TaskID,
		ProjectID:           s.ProjectID,
		Hat:                 s.Hat,
		State:               s.State,
		WorktreePath:        s.WorktreePath,
		IterationCount:      s.IterationCount,
		MaxIterations:       s.MaxIterations,
		InputTokens:         s.InputTokens,
		OutputTokens:        s.OutputTokens,
		InputRate:           s.InputRate,
		OutputRate:          s.OutputRate,
		MaxRuntime:          s.MaxRuntime,
		StartedAt:           s.StartedAt,
		LastActivity:        s.LastActivity,
		Scratchpad:          s.Scratchpad,
		TerminationReason:   s.TerminationReason,
		QualityGateAttempts: s.QualityGateAttempts,
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
	broadcaster := m.broadcaster
	originalHat := session.Hat
	m.mu.Unlock()

	fmt.Printf("runSession: starting session %s for task %s (hat: %s)\n", session.ID, session.TaskID, session.Hat)

	var loopErr error

	// Run the Ralph loop if we have an Anthropic client
	if anthropicClient != nil {
		fmt.Printf("runSession: Anthropic client is configured, starting Ralph loop\n")
		loop := NewRalphLoop(m, session, anthropicClient, broadcaster, m.db)

		// Get or create transition tracker for this task and set up event router
		m.mu.Lock()
		tracker := m.transitionTrackers[session.TaskID]
		if tracker == nil {
			tracker = NewTransitionTracker()
			m.transitionTrackers[session.TaskID] = tracker
		}
		m.mu.Unlock()
		loop.SetEventRouter(NewEventRouter(m.db, tracker, broadcaster))

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
				owner := project.GetOwner()
				repo := project.GetRepo()

				// Initialize executor (no GitHub client - using Forgejo)
				loop.InitExecutor(session.WorktreePath, m.gitOps, nil, owner, repo)
				fmt.Printf("runSession: initialized tool executor (owner=%s, repo=%s)\n", owner, repo)

				// Wire up mail/calendar executor if Central is configured
				m.mu.RLock()
				centralURL := m.centralURL
				tunnelToken := m.tunnelToken
				m.mu.RUnlock()

				if centralURL != "" && tunnelToken != "" {
					mailClient := centralmail.NewClient(centralURL, tunnelToken)
					mailExec := tools.NewMailExecutor(mailClient)
					if mailExec != nil {
						loop.SetMailExecutor(mailExec)
						fmt.Printf("runSession: initialized mail/calendar executor\n")
					}
				}

				// Set Forgejo provider for issue commenting if credentials are available
				m.mu.RLock()
				forgejoBaseURL := m.forgejoBaseURL
				forgejoBotToken := m.forgejoBotToken
				m.mu.RUnlock()

				if forgejoBaseURL != "" && forgejoBotToken != "" {
					forgejoProvider := forgejoclient.New(forgejoBaseURL, forgejoBotToken)
					loop.SetForgejoProvider(forgejoProvider)
					fmt.Printf("runSession: set Forgejo provider for issue commenting\n")
				}

				// Set callback to update project when a repo is created
				projectID := project.ID
				projectProvider := project.GetGitProvider()
				loop.SetOnRepoCreated(func(newOwner, newRepo string) {
					// Update provider-agnostic git info
					if err := m.db.UpdateProjectGitProvider(projectID, projectProvider, newOwner, newRepo); err != nil {
						fmt.Printf("runSession: warning - failed to update project git provider info: %v\n", err)
					}
					fmt.Printf("runSession: updated project %s with %s %s/%s\n", projectID, projectProvider, newOwner, newRepo)
				})
			}
		}

		// Try to restore from checkpoint
		// Use RestoreFromSessionID if set (for resuming from a previous session's state)
		checkpointSessionID := session.ID
		if session.RestoreFromSessionID != "" {
			checkpointSessionID = session.RestoreFromSessionID
			fmt.Printf("runSession: restoring from previous session %s\n", checkpointSessionID)
		}
		checkpoint, err := m.db.GetLatestSessionCheckpoint(checkpointSessionID)
		if err != nil {
			fmt.Printf("runSession: error getting checkpoint for session %s: %v\n", checkpointSessionID, err)
		} else if checkpoint == nil {
			fmt.Printf("runSession: no checkpoint found for session %s\n", checkpointSessionID)
		} else {
			if restoreErr := loop.RestoreFromCheckpoint(checkpoint); restoreErr != nil {
				fmt.Printf("warning: failed to restore checkpoint: %v\n", restoreErr)
			} else {
				fmt.Printf("runSession: restored from checkpoint (iteration %d)\n", checkpoint.Iteration)
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

	// Determine final state and termination reason based on error
	m.mu.Lock()
	nextHat := session.Hat
	hatTransition := loopErr == nil && nextHat != originalHat
	worktreePath := session.WorktreePath
	taskID := session.TaskID
	sessionID := session.ID
	qualityGateAttempts := session.QualityGateAttempts

	// Determine termination reason
	var terminationReason string
	if session.State == StateStopping {
		session.State = StateStopped
		terminationReason = string(TerminationUserStopped)
	} else if session.State == StatePaused {
		// Keep paused state
		terminationReason = "paused"
	} else if loopErr != nil {
		// Check if it's a budget error (requires approval, not a failure)
		switch loopErr {
		case ErrIterationLimit:
			session.State = StatePaused
			terminationReason = string(TerminationMaxIterations)
		case ErrTokenBudget:
			session.State = StatePaused
			terminationReason = string(TerminationMaxTokens)
		case ErrDollarBudget:
			session.State = StatePaused
			terminationReason = string(TerminationMaxCost)
		case ErrRuntimeLimit:
			session.State = StatePaused
			terminationReason = string(TerminationMaxRuntime)
		case ErrBudgetExceeded:
			session.State = StatePaused
			terminationReason = "budget_exceeded"
		case context.Canceled:
			session.State = StateStopped
			terminationReason = string(TerminationUserStopped)
		default:
			session.State = StateFailed
			// Check if it's a loop health termination
			errStr := loopErr.Error()
			if strings.Contains(errStr, "loop terminated:") {
				// Extract reason from "loop terminated: <reason>"
				parts := strings.SplitN(errStr, "loop terminated: ", 2)
				if len(parts) == 2 {
					terminationReason = parts[1]
				} else {
					terminationReason = string(TerminationError)
				}
			} else {
				terminationReason = string(TerminationError)
			}
		}
	} else {
		if hatTransition {
			terminationReason = string(TerminationHatTransition)
		} else {
			terminationReason = string(TerminationCompleted)
		}
		session.State = StateCompleted
	}
	finalState := session.State
	m.mu.Unlock()

	// Update DB with final state and outcome
	_ = m.db.UpdateSessionStatus(sessionID, string(finalState))

	// Persist termination info for audit trail
	_ = m.db.UpdateSessionTermination(sessionID, terminationReason, qualityGateAttempts)

	// Handle hat transition: create and start new session with next hat
	if hatTransition {
		m.handleHatTransition(ctx, taskID, originalHat, nextHat, worktreePath)
		return
	}

	// If no transition, clean up normally
	m.mu.Lock()
	delete(m.sessions, sessionID)
	delete(m.byTask, taskID)
	delete(m.transitionTrackers, taskID) // Clean up transition tracker
	m.mu.Unlock()

	// Update task status based on final state
	switch finalState {
	case StateCompleted:
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusCompleted)
		m.broadcastTaskUpdated(taskID, db.TaskStatusCompleted)

		// Notify task completed (for issue sync)
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
		m.broadcastTaskUpdated(taskID, db.TaskStatusPaused)

		// Notify with error status (adds comment to linked issue, doesn't close it)
		reason := "Session failed"
		if loopErr != nil {
			reason = loopErr.Error()
		}
		m.notifyTaskStatus(taskID, "error:"+reason)

	case StatePaused, StateStopped:
		// Mark task as paused so it can be resumed
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusPaused)
		m.broadcastTaskUpdated(taskID, db.TaskStatusPaused)
		m.notifyTaskStatus(taskID, "paused")
	}
}

// handleHatTransition handles transitioning a task to a new hat
func (m *Manager) handleHatTransition(ctx context.Context, taskID, originalHat, nextHat, worktreePath string) {
	// Get transition tracker and old session ID
	m.mu.Lock()
	oldSessionID := m.byTask[taskID]
	tracker := m.transitionTrackers[taskID]
	if tracker == nil {
		tracker = NewTransitionTracker()
		m.transitionTrackers[taskID] = tracker
	}
	m.mu.Unlock()

	// Check for transition loops
	if err := tracker.RecordTransition(originalHat, nextHat); err != nil {
		fmt.Printf("error: %v (history: %s), marking task quarantined\n", err, tracker.History())
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusQuarantined)
		m.broadcastTaskUpdated(taskID, db.TaskStatusQuarantined)
		m.cleanupTransitionTracker(taskID)
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
		m.broadcastTaskUpdated(taskID, db.TaskStatusCancelled)
		return
	}

	// Start the new session
	if err := m.Start(ctx, newSession.ID); err != nil {
		fmt.Printf("error: failed to start session for hat transition: %v\n", err)
		_ = m.db.UpdateTaskStatus(taskID, db.TaskStatusCancelled)
		m.broadcastTaskUpdated(taskID, db.TaskStatusCancelled)
		return
	}

	fmt.Printf("hat transition: task %s transitioned from %s to %s (session %s)\n", taskID, originalHat, nextHat, newSession.ID)
}

// cleanupTransitionTracker removes the transition tracker for a task
func (m *Manager) cleanupTransitionTracker(taskID string) {
	m.mu.Lock()
	delete(m.transitionTrackers, taskID)
	m.mu.Unlock()
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

		// Compute token counts from session_activity (single source of truth)
		inputTokens, outputTokens, _ := m.db.GetSessionTokensFromActivity(dbSession.ID)

		// Get termination reason from DB if set
		var terminationReason string
		if dbSession.TerminationReason.Valid {
			terminationReason = dbSession.TerminationReason.String
		}

		// Get task to populate project_id for channel routing
		var projectID string
		if task, err := m.db.GetTaskByID(dbSession.TaskID); err == nil && task != nil {
			projectID = task.ProjectID
		}

		session := &ActiveSession{
			ID:                  dbSession.ID,
			TaskID:              dbSession.TaskID,
			ProjectID:           projectID,
			Hat:                 dbSession.Hat,
			State:               state,
			WorktreePath:        dbSession.WorktreePath,
			IterationCount:      dbSession.IterationCount,
			MaxIterations:       dbSession.MaxIterations,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			InputRate:           dbSession.InputRate,
			OutputRate:          dbSession.OutputRate,
			MaxRuntime:          m.defaultMaxRuntime, // Use default for restored sessions
			TerminationReason:   terminationReason,
			QualityGateAttempts: dbSession.QualityGateAttempts,
			done:                make(chan struct{}),
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
	ctx := context.Background()

	m.mu.RLock()
	gitOps := m.gitOps
	m.mu.RUnlock()

	// Get task from DB
	task, err := m.db.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("createPRForTask: failed to get task %s: %v\n", taskID, err)
		return
	}

	// Get project from DB to find git provider owner/repo
	project, err := m.db.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("createPRForTask: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	owner := project.GetOwner()
	repo := project.GetRepo()
	if owner == "" || repo == "" {
		fmt.Printf("createPRForTask: project %s has no owner/repo configured\n", project.ID)
		return
	}

	// For Forgejo projects, PRs are created via the Forgejo API.
	// The push is a no-op (bare repo worktrees), so we just create the PR.
	if project.IsForgejo() {
		m.mu.RLock()
		baseURL := m.forgejoBaseURL
		botToken := m.forgejoBotToken
		m.mu.RUnlock()

		if baseURL == "" || botToken == "" {
			fmt.Printf("createPRForTask: Forgejo credentials not configured, skipping PR for task %s\n", taskID)
			return
		}

		branchName, err := gitOps.GetCurrentBranch(worktreePath)
		if err != nil {
			fmt.Printf("createPRForTask: failed to get branch for task %s: %v\n", taskID, err)
			return
		}

		forgejoProvider := forgejoclient.New(baseURL, botToken)
		pr, err := forgejoProvider.CreatePR(ctx, owner, repo, gitprovider.CreatePROpts{
			Title: task.Title,
			Body:  fmt.Sprintf("Closes task: %s\n\n%s", taskID, task.GetDescription()),
			Head:  branchName,
			Base:  project.DefaultBranch,
		})
		if err != nil {
			fmt.Printf("createPRForTask: failed to create Forgejo PR for task %s: %v\n", taskID, err)
			return
		}

		if err := m.db.UpdateTaskPRNumber(taskID, pr.Number); err != nil {
			fmt.Printf("createPRForTask: failed to update task %s with PR number: %v\n", taskID, err)
			return
		}
		fmt.Printf("createPRForTask: created Forgejo PR #%d for task %s\n", pr.Number, taskID)

		m.mu.RLock()
		onPRCreated := m.onPRCreated
		m.mu.RUnlock()
		if onPRCreated != nil {
			go onPRCreated(taskID, pr.Number)
		}

		// Auto-merge the PR unless autonomy_level is 0 (requires manual approval)
		if task.AutonomyLevel == 0 {
			fmt.Printf("createPRForTask: autonomy_level=0 for task %s, skipping auto-merge\n", taskID)
			return
		}

		if err := forgejoProvider.MergePR(ctx, owner, repo, pr.Number, gitprovider.MergeSquash); err != nil {
			fmt.Printf("createPRForTask: failed to merge Forgejo PR #%d for task %s: %v (left open for manual merge)\n", pr.Number, taskID, err)
			return
		}
		fmt.Printf("createPRForTask: merged Forgejo PR #%d for task %s\n", pr.Number, taskID)

		// Cleanup worktree after successful merge
		m.mu.RLock()
		gitService := m.gitService
		m.mu.RUnlock()

		if gitService != nil {
			if err := gitService.CleanupTaskWorktree(project.RepoPath, taskID, true); err != nil {
				fmt.Printf("createPRForTask: failed to cleanup worktree for task %s: %v\n", taskID, err)
			} else {
				if err := m.db.MarkTaskWorktreeCleaned(taskID); err != nil {
					fmt.Printf("createPRForTask: failed to mark worktree cleaned for task %s: %v\n", taskID, err)
				}
				fmt.Printf("createPRForTask: cleaned up worktree for task %s after merge\n", taskID)
			}
		}
		return
	}

	// Non-Forgejo projects are not supported for PR creation
	fmt.Printf("createPRForTask: project %s is not a Forgejo project, skipping PR creation\n", project.ID)
}
