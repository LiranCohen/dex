// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/liranmauda/dex/internal/db"
	"github.com/liranmauda/dex/internal/orchestrator"
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

	TokensUsed    int64
	TokensBudget  *int64
	DollarsUsed   float64
	DollarsBudget *float64

	StartedAt    time.Time
	LastActivity time.Time

	// For cancellation
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager manages Claude Code session lifecycle
type Manager struct {
	db           *db.DB
	scheduler    *orchestrator.Scheduler
	promptLoader *PromptLoader

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

// SetDefaults configures default budget limits for new sessions
func (m *Manager) SetDefaults(maxIterations int, tokenBudget *int64, dollarBudgetFloat *float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.defaultMaxIterations = maxIterations
	m.defaultTokenBudget = tokenBudget
	m.defaultDollarBudget = dollarBudgetFloat
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
	if session.cancel != nil {
		session.cancel()
	}
	m.mu.Unlock()

	// Update DB
	return m.db.UpdateSessionStatus(sessionID, string(StatePaused))
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
		TokensUsed:     s.TokensUsed,
		DollarsUsed:    s.DollarsUsed,
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
// This is a placeholder - will be implemented in Checkpoint 5.3
func (m *Manager) runSession(ctx context.Context, session *ActiveSession) {
	defer close(session.done)

	m.mu.Lock()
	session.State = StateRunning
	m.mu.Unlock()

	// Placeholder: just wait for cancellation
	// Real implementation will run the Ralph loop
	<-ctx.Done()

	m.mu.Lock()
	if session.State == StateStopping {
		session.State = StateStopped
	} else if session.State == StatePaused {
		// Keep paused state
	} else {
		session.State = StateCompleted
	}
	finalState := session.State
	sessionID := session.ID
	taskID := session.TaskID
	m.mu.Unlock()

	// Update DB
	_ = m.db.UpdateSessionStatus(sessionID, string(finalState))

	// Remove from active sessions
	m.mu.Lock()
	delete(m.sessions, sessionID)
	delete(m.byTask, taskID)
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

		session := &ActiveSession{
			ID:             dbSession.ID,
			TaskID:         dbSession.TaskID,
			Hat:            dbSession.Hat,
			State:          state,
			WorktreePath:   dbSession.WorktreePath,
			IterationCount: dbSession.IterationCount,
			MaxIterations:  dbSession.MaxIterations,
			TokensUsed:     dbSession.TokensUsed,
			DollarsUsed:    dbSession.DollarsUsed,
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
