package session

import (
	"sync"
)

// HealthStatus indicates the current health of the execution loop
type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"        // Operating normally
	HealthDegraded  HealthStatus = "degraded"  // Some failures but recoverable
	HealthThrashing HealthStatus = "thrashing" // Stuck in a loop, not making progress
	HealthExhausted HealthStatus = "exhausted" // Max attempts reached
)

// Default thresholds
const (
	DefaultMaxConsecutiveFailures   = 5
	DefaultMaxQualityGateAttempts   = 5
	DefaultMaxTaskBlocks            = 3
	DefaultMaxValidationFailures    = 3 // Validation failures (malformed JSON, invalid tool calls)
)

// LoopHealth tracks the health of the execution loop
type LoopHealth struct {
	mu sync.RWMutex

	// Failure tracking
	ConsecutiveFailures int // Consecutive tool/API failures
	ConsecutiveBlocked  int // Consecutive quality gate blocks
	TotalFailures       int // Total failures this session

	// Validation failure tracking (malformed JSON, invalid tool calls, etc.)
	ConsecutiveValidationFailures int // Consecutive validation failures
	TotalValidationFailures       int // Total validation failures this session

	// Quality gate tracking
	QualityGateAttempts int            // Total quality gate attempts this session
	TaskBlockCounts     map[string]int // Per-checklist-item block counts

	// Tool repetition detection
	Repetition *RepetitionInspector

	// Thresholds
	MaxConsecutiveFailures   int
	MaxQualityGateAttempts   int
	MaxTaskBlocks            int
	MaxValidationFailures    int

	// Last recorded status for change detection
	lastStatus HealthStatus
}

// NewLoopHealth creates a new LoopHealth tracker with default thresholds
func NewLoopHealth() *LoopHealth {
	return &LoopHealth{
		TaskBlockCounts:          make(map[string]int),
		Repetition:               NewRepetitionInspector(),
		MaxConsecutiveFailures:   DefaultMaxConsecutiveFailures,
		MaxQualityGateAttempts:   DefaultMaxQualityGateAttempts,
		MaxTaskBlocks:            DefaultMaxTaskBlocks,
		MaxValidationFailures:    DefaultMaxValidationFailures,
		lastStatus:               HealthOK,
	}
}

// RecordSuccess records a successful operation, resetting consecutive failure counts
func (h *LoopHealth) RecordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ConsecutiveFailures = 0
	h.ConsecutiveBlocked = 0
	h.ConsecutiveValidationFailures = 0
}

// RecordFailure records a failed operation (tool execution, API error, etc.)
func (h *LoopHealth) RecordFailure(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ConsecutiveFailures++
	h.TotalFailures++
}

// RecordValidationFailure records a validation failure (malformed JSON, invalid tool calls)
// These are distinct from execution failures and indicate Claude is producing invalid output
func (h *LoopHealth) RecordValidationFailure(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ConsecutiveValidationFailures++
	h.TotalValidationFailures++
}

// RecordQualityBlock records a quality gate block
func (h *LoopHealth) RecordQualityBlock() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.QualityGateAttempts++
	h.ConsecutiveBlocked++
}

// RecordQualityPass records a quality gate pass, resetting block count
func (h *LoopHealth) RecordQualityPass() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ConsecutiveBlocked = 0
}

// RecordTaskBlock records a block for a specific checklist item
func (h *LoopHealth) RecordTaskBlock(itemID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.TaskBlockCounts[itemID]++
}

// CheckToolCall checks if a tool call should be allowed based on repetition detection.
// Returns (allowed, reason) - if not allowed, reason explains why.
func (h *LoopHealth) CheckToolCall(toolName, paramsJSON string) (bool, string) {
	if h.Repetition == nil {
		return true, ""
	}
	return h.Repetition.Check(ToolCallSignature{Name: toolName, Params: paramsJSON})
}

// ResetRepetition resets the repetition counter (e.g., on user message or hat change)
func (h *LoopHealth) ResetRepetition() {
	if h.Repetition != nil {
		h.Repetition.Reset()
	}
}

// Status returns the current health status
func (h *LoopHealth) Status() HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Check for exhaustion first
	if h.QualityGateAttempts >= h.MaxQualityGateAttempts {
		return HealthExhausted
	}

	// Check for thrashing (stuck on same item)
	for _, count := range h.TaskBlockCounts {
		if count >= h.MaxTaskBlocks {
			return HealthThrashing
		}
	}

	// Check for consecutive failures (either execution or validation)
	if h.ConsecutiveFailures >= h.MaxConsecutiveFailures {
		return HealthThrashing
	}
	if h.ConsecutiveValidationFailures >= h.MaxValidationFailures {
		return HealthThrashing
	}

	// Check for degraded state
	if h.ConsecutiveFailures > 0 || h.ConsecutiveBlocked > 1 || h.ConsecutiveValidationFailures > 0 {
		return HealthDegraded
	}

	return HealthOK
}

// ShouldTerminate returns true if the loop should terminate due to health issues
func (h *LoopHealth) ShouldTerminate() (bool, TerminationReason) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Check for quality gate exhaustion
	if h.QualityGateAttempts >= h.MaxQualityGateAttempts {
		return true, TerminationQualityGateExhausted
	}

	// Check for consecutive execution failures
	if h.ConsecutiveFailures >= h.MaxConsecutiveFailures {
		return true, TerminationConsecutiveFailures
	}

	// Check for consecutive validation failures (malformed output from Claude)
	if h.ConsecutiveValidationFailures >= h.MaxValidationFailures {
		return true, TerminationValidationFailure
	}

	// Check for tool repetition loop (too many blocks of identical consecutive calls)
	if h.Repetition != nil && h.Repetition.ShouldTerminate() {
		return true, TerminationRepetitionLoop
	}

	// Check for thrashing on specific items
	for _, count := range h.TaskBlockCounts {
		if count >= h.MaxTaskBlocks {
			return true, TerminationLoopThrashing
		}
	}

	return false, ""
}

// StatusChanged returns true if the status has changed since last check
func (h *LoopHealth) StatusChanged() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	current := h.statusUnsafe()
	if current != h.lastStatus {
		h.lastStatus = current
		return true
	}
	return false
}

// statusUnsafe returns status without locking (caller must hold lock)
func (h *LoopHealth) statusUnsafe() HealthStatus {
	if h.QualityGateAttempts >= h.MaxQualityGateAttempts {
		return HealthExhausted
	}

	for _, count := range h.TaskBlockCounts {
		if count >= h.MaxTaskBlocks {
			return HealthThrashing
		}
	}

	if h.ConsecutiveFailures >= h.MaxConsecutiveFailures {
		return HealthThrashing
	}
	if h.ConsecutiveValidationFailures >= h.MaxValidationFailures {
		return HealthThrashing
	}

	if h.ConsecutiveFailures > 0 || h.ConsecutiveBlocked > 1 || h.ConsecutiveValidationFailures > 0 {
		return HealthDegraded
	}

	return HealthOK
}

// Snapshot returns a point-in-time snapshot of health metrics
func (h *LoopHealth) Snapshot() LoopHealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	taskBlocks := make(map[string]int, len(h.TaskBlockCounts))
	for k, v := range h.TaskBlockCounts {
		taskBlocks[k] = v
	}

	return LoopHealthSnapshot{
		Status:                        h.statusUnsafe(),
		ConsecutiveFailures:           h.ConsecutiveFailures,
		ConsecutiveBlocked:            h.ConsecutiveBlocked,
		TotalFailures:                 h.TotalFailures,
		ConsecutiveValidationFailures: h.ConsecutiveValidationFailures,
		TotalValidationFailures:       h.TotalValidationFailures,
		QualityGateAttempts:           h.QualityGateAttempts,
		TaskBlockCounts:               taskBlocks,
	}
}

// LoopHealthSnapshot is an immutable snapshot of loop health
type LoopHealthSnapshot struct {
	Status                        HealthStatus   `json:"status"`
	ConsecutiveFailures           int            `json:"consecutive_failures"`
	ConsecutiveBlocked            int            `json:"consecutive_blocked"`
	TotalFailures                 int            `json:"total_failures"`
	ConsecutiveValidationFailures int            `json:"consecutive_validation_failures"`
	TotalValidationFailures       int            `json:"total_validation_failures"`
	QualityGateAttempts           int            `json:"quality_gate_attempts"`
	TaskBlockCounts               map[string]int `json:"task_block_counts,omitempty"`
}
