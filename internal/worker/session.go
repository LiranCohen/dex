// Package worker provides types and utilities for Dex worker nodes.
package worker

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// HatVisit records a hat execution period during a session.
type HatVisit struct {
	Hat       string    // The hat that was active
	StartedAt time.Time // When this hat period started
	EndedAt   time.Time // When this hat period ended (zero if still active)
	Event     string    // The event that triggered transition away (empty if still active)
}

// WorkerSession tracks the current execution state for a worker running an objective.
// It's a lightweight version of session.ActiveSession without HQ database dependencies.
type WorkerSession struct {
	mu sync.RWMutex

	// Core identifiers
	ID          string // Session ID (generated locally)
	ObjectiveID string // Objective ID from HQ
	TaskID      string // Task ID (same as ObjectiveID for workers)

	// Execution context
	Hat     string // Current hat (explorer, planner, creator, critic, editor, resolver)
	WorkDir string // Working directory for the project

	// Progress tracking
	IterationCount int   // Number of API calls made
	InputTokens    int64 // Total input tokens used
	OutputTokens   int64 // Total output tokens used

	// Budget limits
	TokenBudget   int           // Max tokens allowed (0 = unlimited)
	MaxIterations int           // Max iterations allowed (0 = unlimited)
	MaxRuntime    time.Duration // Max runtime allowed (0 = unlimited)

	// Timing
	StartedAt    time.Time // When execution started
	LastActivity time.Time // Last API call or tool execution

	// Scratchpad for persistent thinking/notes
	Scratchpad string

	// Predecessor context for resumption
	PredecessorContext string

	// Quality gate tracking
	QualityGateAttempts int

	// Checklist tracking
	ChecklistDone   []string // IDs of completed checklist items
	ChecklistFailed []string // IDs of failed checklist items

	// Hat transition tracking
	HatHistory      []HatVisit // History of hat visits in this session
	TransitionCount int        // Total number of hat transitions
	PreviousHat     string     // Hat before current (for handoff context)
}

// NewWorkerSession creates a new WorkerSession for the given objective.
func NewWorkerSession(id, objectiveID, hat, workDir string) *WorkerSession {
	return &WorkerSession{
		ID:           id,
		ObjectiveID:  objectiveID,
		TaskID:       objectiveID, // Workers use objective ID as task ID
		Hat:          hat,
		WorkDir:      workDir,
		StartedAt:    time.Now(),
		LastActivity: time.Now(),
	}
}

// SetBudgets sets the execution budget limits.
func (s *WorkerSession) SetBudgets(tokenBudget, maxIterations int, maxRuntime time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TokenBudget = tokenBudget
	s.MaxIterations = maxIterations
	s.MaxRuntime = maxRuntime
}

// RecordIteration updates the session after an API call.
func (s *WorkerSession) RecordIteration(inputTokens, outputTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IterationCount++
	s.InputTokens += int64(inputTokens)
	s.OutputTokens += int64(outputTokens)
	s.LastActivity = time.Now()
}

// TotalTokens returns the total tokens used (input + output).
func (s *WorkerSession) TotalTokens() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InputTokens + s.OutputTokens
}

// GetIteration returns the current iteration count.
func (s *WorkerSession) GetIteration() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.IterationCount
}

// UpdateHat updates the current hat.
func (s *WorkerSession) UpdateHat(hat string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Hat = hat
}

// GetHat returns the current hat.
func (s *WorkerSession) GetHat() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Hat
}

// UpdateScratchpad updates the scratchpad content.
func (s *WorkerSession) UpdateScratchpad(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Scratchpad = content
}

// GetScratchpad returns the current scratchpad content.
func (s *WorkerSession) GetScratchpad() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Scratchpad
}

// MarkChecklistDone marks a checklist item as done.
func (s *WorkerSession) MarkChecklistDone(itemID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ChecklistDone = append(s.ChecklistDone, itemID)
}

// MarkChecklistFailed marks a checklist item as failed.
func (s *WorkerSession) MarkChecklistFailed(itemID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ChecklistFailed = append(s.ChecklistFailed, itemID)
}

// GetChecklistStatus returns copies of the done and failed checklist item IDs.
func (s *WorkerSession) GetChecklistStatus() (done, failed []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	done = make([]string, len(s.ChecklistDone))
	copy(done, s.ChecklistDone)
	failed = make([]string, len(s.ChecklistFailed))
	copy(failed, s.ChecklistFailed)
	return
}

// GetTokenUsage returns the current input and output token counts.
func (s *WorkerSession) GetTokenUsage() (input, output int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InputTokens, s.OutputTokens
}

// IncrementQualityGateAttempts increments the quality gate attempt counter.
func (s *WorkerSession) IncrementQualityGateAttempts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QualityGateAttempts++
}

// GetQualityGateAttempts returns the number of quality gate attempts.
func (s *WorkerSession) GetQualityGateAttempts() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.QualityGateAttempts
}

// Runtime returns the time since the session started.
func (s *WorkerSession) Runtime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.StartedAt)
}

// RecordHatTransition records a hat change and updates tracking state.
func (s *WorkerSession) RecordHatTransition(fromHat, toHat, event string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Close out the current hat visit if we have history
	if len(s.HatHistory) > 0 {
		s.HatHistory[len(s.HatHistory)-1].EndedAt = now
		s.HatHistory[len(s.HatHistory)-1].Event = event
	}

	// Add new hat visit
	s.HatHistory = append(s.HatHistory, HatVisit{
		Hat:       toHat,
		StartedAt: now,
	})

	// Update state
	s.PreviousHat = fromHat
	s.Hat = toHat
	s.TransitionCount++
}

// HatVisitCount returns how many times a specific hat has been visited.
func (s *WorkerSession) HatVisitCount(hat string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, visit := range s.HatHistory {
		if visit.Hat == hat {
			count++
		}
	}

	// Also count the current hat if it matches and we have no history yet
	if len(s.HatHistory) == 0 && s.Hat == hat {
		count = 1
	}

	return count
}

// BuildHandoffContext creates a summary of the previous hat's work for context.
func (s *WorkerSession) BuildHandoffContext() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.PreviousHat == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are continuing work that was started by the '%s' hat.\n", s.PreviousHat))

	// Build history summary
	if len(s.HatHistory) > 1 {
		sb.WriteString("\n### Hat Progression\n")
		for i, visit := range s.HatHistory {
			if i == len(s.HatHistory)-1 {
				// Current hat - skip
				continue
			}
			duration := ""
			if !visit.EndedAt.IsZero() {
				duration = fmt.Sprintf(" (%.1fs)", visit.EndedAt.Sub(visit.StartedAt).Seconds())
			}
			sb.WriteString(fmt.Sprintf("- %s%s → %s\n", visit.Hat, duration, visit.Event))
		}
	}

	// Include scratchpad if present
	if s.Scratchpad != "" {
		sb.WriteString("\n### Predecessor Notes\n")
		sb.WriteString(s.Scratchpad)
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetTransitionCount returns the total number of hat transitions.
func (s *WorkerSession) GetTransitionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TransitionCount
}

// GetHatHistory returns a copy of the hat history.
func (s *WorkerSession) GetHatHistory() []HatVisit {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := make([]HatVisit, len(s.HatHistory))
	copy(history, s.HatHistory)
	return history
}

// SetTransitionCount sets the transition count (used for restoration).
func (s *WorkerSession) SetTransitionCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TransitionCount = count
}

// RestoreHatHistory restores the hat history from a saved state.
func (s *WorkerSession) RestoreHatHistory(history []HatVisit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HatHistory = make([]HatVisit, len(history))
	copy(s.HatHistory, history)
}

// RestoreTokenUsage sets the token counts (used for restoration after crash).
func (s *WorkerSession) RestoreTokenUsage(input, output int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InputTokens = input
	s.OutputTokens = output
}

// RestoreIteration sets the iteration count (used for restoration after crash).
func (s *WorkerSession) RestoreIteration(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IterationCount = count
}

// SetPreviousHat sets the previous hat (used for restoration).
func (s *WorkerSession) SetPreviousHat(hat string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PreviousHat = hat
}
