package session

import (
	"fmt"
	"sync"
)

// Default repetition thresholds
const (
	DefaultMaxRepetitions      = 5 // Max consecutive identical tool calls
	DefaultMaxRepetitionBlocks = 3 // Max times we block before termination
)

// ToolCallSignature uniquely identifies a tool call for repetition detection
type ToolCallSignature struct {
	Name   string // Tool name
	Params string // JSON-serialized params for comparison
}

// Equals returns true if two signatures are identical
func (t ToolCallSignature) Equals(other ToolCallSignature) bool {
	return t.Name == other.Name && t.Params == other.Params
}

// RepetitionInspector detects infinite loops from identical consecutive tool calls.
// Inspired by Goose's RepetitionInspector.
type RepetitionInspector struct {
	mu sync.RWMutex

	// Configuration
	maxRepetitions int // Max consecutive identical calls before blocking
	maxBlocks      int // Max times we block before recommending termination

	// State
	lastCall    *ToolCallSignature // Last tool call signature
	repeatCount int                // Consecutive identical calls
	blockCount  int                // Times we've blocked repetitions
	callCounts  map[string]int     // Total calls per tool name (for stats)
}

// NewRepetitionInspector creates a new inspector with default thresholds
func NewRepetitionInspector() *RepetitionInspector {
	return &RepetitionInspector{
		maxRepetitions: DefaultMaxRepetitions,
		maxBlocks:      DefaultMaxRepetitionBlocks,
		callCounts:     make(map[string]int),
	}
}

// NewRepetitionInspectorWithConfig creates an inspector with custom thresholds
func NewRepetitionInspectorWithConfig(maxRepetitions, maxBlocks int) *RepetitionInspector {
	return &RepetitionInspector{
		maxRepetitions: maxRepetitions,
		maxBlocks:      maxBlocks,
		callCounts:     make(map[string]int),
	}
}

// Check evaluates a tool call and returns whether it should be allowed.
// Returns (allowed, reason) - if not allowed, reason explains why.
func (ri *RepetitionInspector) Check(call ToolCallSignature) (allowed bool, reason string) {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	// Track total calls per tool
	ri.callCounts[call.Name]++

	// Check if this is a repeat of the last call
	if ri.lastCall != nil && ri.lastCall.Equals(call) {
		ri.repeatCount++
		if ri.repeatCount > ri.maxRepetitions {
			ri.blockCount++
			return false, fmt.Sprintf(
				"tool '%s' called %d times consecutively with identical parameters (blocked %d/%d)",
				call.Name, ri.repeatCount, ri.blockCount, ri.maxBlocks)
		}
	} else {
		// Different call, reset repeat count
		ri.repeatCount = 1
	}

	// Update last call
	ri.lastCall = &call
	return true, ""
}

// RecordBlock records that a repetition was blocked.
// Returns true if we should terminate due to too many blocks.
func (ri *RepetitionInspector) ShouldTerminate() bool {
	ri.mu.RLock()
	defer ri.mu.RUnlock()
	return ri.blockCount >= ri.maxBlocks
}

// Reset clears all state (e.g., on user message or hat transition)
func (ri *RepetitionInspector) Reset() {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	ri.lastCall = nil
	ri.repeatCount = 0
	// Note: we don't reset blockCount or callCounts - those are session-wide stats
}

// ResetAll clears all state including block counts (for new session)
func (ri *RepetitionInspector) ResetAll() {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	ri.lastCall = nil
	ri.repeatCount = 0
	ri.blockCount = 0
	ri.callCounts = make(map[string]int)
}

// Stats returns current statistics
func (ri *RepetitionInspector) Stats() RepetitionStats {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	counts := make(map[string]int, len(ri.callCounts))
	for k, v := range ri.callCounts {
		counts[k] = v
	}

	var lastTool string
	if ri.lastCall != nil {
		lastTool = ri.lastCall.Name
	}

	return RepetitionStats{
		LastTool:        lastTool,
		RepeatCount:     ri.repeatCount,
		BlockCount:      ri.blockCount,
		TotalCallCounts: counts,
	}
}

// RepetitionStats provides a snapshot of repetition detection state
type RepetitionStats struct {
	LastTool        string         `json:"last_tool,omitempty"`
	RepeatCount     int            `json:"repeat_count"`
	BlockCount      int            `json:"block_count"`
	TotalCallCounts map[string]int `json:"total_call_counts,omitempty"`
}
