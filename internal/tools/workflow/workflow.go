// Package workflow provides shared workflow tools for Dex
// Used by both HQ (session/ralph.go) and workers (worker/ralph.go)
package workflow

import (
	"encoding/json"
	"fmt"
	"time"
)

// Result represents the outcome of executing a workflow tool
type Result struct {
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// Scratchpad represents the structured scratchpad content
type Scratchpad struct {
	Understanding string `json:"understanding"`
	Plan          string `json:"plan"`
	Decisions     string `json:"decisions,omitempty"`
	Blockers      string `json:"blockers,omitempty"`
	LastAction    string `json:"last_action"`
}

// Memory represents a project memory to store
type Memory struct {
	Category string `json:"category"`
	Content  string `json:"content"`
	Source   string `json:"source,omitempty"`
}

// ChecklistItemStatus represents the status of a checklist item
type ChecklistItemStatus string

const (
	ChecklistStatusDone    ChecklistItemStatus = "done"
	ChecklistStatusFailed  ChecklistItemStatus = "failed"
	ChecklistStatusSkipped ChecklistItemStatus = "skipped"
)

// EventType represents workflow event types
type EventType string

const (
	EventPlanComplete       EventType = "plan.complete"
	EventDesignComplete     EventType = "design.complete"
	EventImplementationDone EventType = "implementation.done"
	EventReviewApproved     EventType = "review.approved"
	EventReviewRejected     EventType = "review.rejected"
	EventTaskBlocked        EventType = "task.blocked"
	EventResolved           EventType = "resolved"
	EventTaskComplete       EventType = "task.complete"
)

// ValidEventTypes returns all valid event types
func ValidEventTypes() []EventType {
	return []EventType{
		EventPlanComplete,
		EventDesignComplete,
		EventImplementationDone,
		EventReviewApproved,
		EventReviewRejected,
		EventTaskBlocked,
		EventResolved,
		EventTaskComplete,
	}
}

// IsValidEventType checks if an event type is valid
func IsValidEventType(event string) bool {
	for _, e := range ValidEventTypes() {
		if string(e) == event {
			return true
		}
	}
	return false
}

// EventResult represents the result of signaling an event
type EventResult struct {
	Event   EventType `json:"event"`
	NextHat string    `json:"next_hat,omitempty"`
}

// GetNextHat returns the next hat for an event type
func GetNextHat(event EventType) string {
	switch event {
	case EventPlanComplete:
		return "designer"
	case EventDesignComplete:
		return "creator"
	case EventImplementationDone:
		return "critic"
	case EventReviewApproved:
		return "editor"
	case EventReviewRejected:
		return "creator"
	case EventTaskBlocked:
		return "resolver"
	case EventResolved:
		return "creator"
	case EventTaskComplete:
		return "" // Terminal
	default:
		return ""
	}
}

// ChecklistUpdateHandler is called when a checklist item is updated
type ChecklistUpdateHandler func(itemID string, status ChecklistItemStatus, reason string) error

// EventHandler is called when an event is signaled
type EventHandler func(event EventType, payload map[string]any, ackFailures bool) error

// ScratchpadUpdateHandler is called when the scratchpad is updated
type ScratchpadUpdateHandler func(scratchpad Scratchpad) error

// MemoryStoreHandler is called when a memory is stored
type MemoryStoreHandler func(memory Memory) (string, error)

// Executor executes workflow tools
type Executor struct {
	TaskID    string
	SessionID string

	// Handlers for tool effects
	OnChecklistUpdate  ChecklistUpdateHandler
	OnEvent            EventHandler
	OnScratchpadUpdate ScratchpadUpdateHandler
	OnMemoryStore      MemoryStoreHandler
}

// NewExecutor creates a new workflow executor
func NewExecutor(taskID, sessionID string) *Executor {
	return &Executor{
		TaskID:    taskID,
		SessionID: sessionID,
	}
}

// MarkChecklistItem updates the status of a checklist item
func (e *Executor) MarkChecklistItem(itemID string, status ChecklistItemStatus, reason string) Result {
	start := time.Now()

	// Validate status
	switch status {
	case ChecklistStatusDone, ChecklistStatusFailed, ChecklistStatusSkipped:
		// Valid
	default:
		return Result{
			Output:     fmt.Sprintf("Invalid status: %s. Must be one of: done, failed, skipped", status),
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Validate reason for failed/skipped
	if (status == ChecklistStatusFailed || status == ChecklistStatusSkipped) && reason == "" {
		return Result{
			Output:     fmt.Sprintf("Reason is required when status is '%s'", status),
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Call handler if set
	if e.OnChecklistUpdate != nil {
		if err := e.OnChecklistUpdate(itemID, status, reason); err != nil {
			return Result{
				Output:     fmt.Sprintf("Failed to update checklist item: %v", err),
				IsError:    true,
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	result := map[string]any{
		"item_id": itemID,
		"status":  status,
	}
	if reason != "" {
		result["reason"] = reason
	}

	output, _ := json.Marshal(result)
	return Result{
		Output:     string(output),
		IsError:    false,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// SignalEvent signals a workflow state transition
func (e *Executor) SignalEvent(event string, payload map[string]any, ackFailures bool) Result {
	start := time.Now()

	// Validate event type
	if !IsValidEventType(event) {
		return Result{
			Output:     fmt.Sprintf("Invalid event type: %s. Valid types: %v", event, ValidEventTypes()),
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	eventType := EventType(event)

	// Call handler if set
	if e.OnEvent != nil {
		if err := e.OnEvent(eventType, payload, ackFailures); err != nil {
			return Result{
				Output:     fmt.Sprintf("Failed to signal event: %v", err),
				IsError:    true,
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	result := EventResult{
		Event:   eventType,
		NextHat: GetNextHat(eventType),
	}

	output, _ := json.Marshal(result)
	return Result{
		Output:     string(output),
		IsError:    false,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// UpdateScratchpad updates the scratchpad content
func (e *Executor) UpdateScratchpad(understanding, plan, decisions, blockers, lastAction string) Result {
	start := time.Now()

	// Validate required fields
	if understanding == "" {
		return Result{
			Output:     "understanding is required",
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	if plan == "" {
		return Result{
			Output:     "plan is required",
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	if lastAction == "" {
		return Result{
			Output:     "last_action is required",
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	scratchpad := Scratchpad{
		Understanding: understanding,
		Plan:          plan,
		Decisions:     decisions,
		Blockers:      blockers,
		LastAction:    lastAction,
	}

	// Call handler if set
	if e.OnScratchpadUpdate != nil {
		if err := e.OnScratchpadUpdate(scratchpad); err != nil {
			return Result{
				Output:     fmt.Sprintf("Failed to update scratchpad: %v", err),
				IsError:    true,
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	return Result{
		Output:     `{"updated": true}`,
		IsError:    false,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// StoreMemory stores a project memory
func (e *Executor) StoreMemory(category, content, source string) Result {
	start := time.Now()

	// Validate category
	validCategories := []string{
		"architecture", "pattern", "pitfall", "decision",
		"fix", "convention", "dependency", "constraint",
	}
	valid := false
	for _, c := range validCategories {
		if c == category {
			valid = true
			break
		}
	}
	if !valid {
		return Result{
			Output:     fmt.Sprintf("Invalid category: %s. Valid categories: %v", category, validCategories),
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Validate content
	if content == "" {
		return Result{
			Output:     "content is required",
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	memory := Memory{
		Category: category,
		Content:  content,
		Source:   source,
	}

	// Call handler if set
	var memoryID string
	if e.OnMemoryStore != nil {
		var err error
		memoryID, err = e.OnMemoryStore(memory)
		if err != nil {
			return Result{
				Output:     fmt.Sprintf("Failed to store memory: %v", err),
				IsError:    true,
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	} else {
		memoryID = "mem-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}

	result := map[string]any{
		"memory_id": memoryID,
		"category":  category,
	}
	output, _ := json.Marshal(result)

	return Result{
		Output:     string(output),
		IsError:    false,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// FormatScratchpad formats a scratchpad for display/storage
func FormatScratchpad(s Scratchpad) string {
	var result string
	result += "## Current Understanding\n" + s.Understanding + "\n\n"
	result += "## Current Plan\n" + s.Plan + "\n\n"
	if s.Decisions != "" {
		result += "## Key Decisions\n" + s.Decisions + "\n\n"
	}
	if s.Blockers != "" {
		result += "## Blockers\n" + s.Blockers + "\n\n"
	}
	result += "## Last Action\n" + s.LastAction
	return result
}

// ParseScratchpad attempts to parse a scratchpad from formatted text
func ParseScratchpad(text string) (Scratchpad, error) {
	// This is a best-effort parser for legacy scratchpad format
	// New code should use the structured Scratchpad type directly
	return Scratchpad{
		Understanding: text,
		Plan:          "",
		LastAction:    "",
	}, nil
}
