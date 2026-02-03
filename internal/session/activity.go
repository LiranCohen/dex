// Package session provides session lifecycle management for Poindexter
package session

import (
	"encoding/json"
	"fmt"

	"github.com/lirancohen/dex/internal/db"
)

// ActivityRecorder records session activity to the database and broadcasts via WebSocket
type ActivityRecorder struct {
	db        *db.DB
	sessionID string
	taskID    string
	hat       string
	broadcast func(eventType string, payload map[string]any)
}

// NewActivityRecorder creates a new ActivityRecorder for a session
func NewActivityRecorder(database *db.DB, sessionID, taskID string, broadcast func(eventType string, payload map[string]any)) *ActivityRecorder {
	return &ActivityRecorder{
		db:        database,
		sessionID: sessionID,
		taskID:    taskID,
		broadcast: broadcast,
	}
}

// SetHat sets the current hat for activity tracking
func (r *ActivityRecorder) SetHat(hat string) {
	r.hat = hat
}

// broadcastActivity sends an activity event through WebSocket
func (r *ActivityRecorder) broadcastActivity(activity *db.SessionActivity) {
	if r.broadcast == nil {
		return
	}

	// Extract values from nullable types to avoid serializing as {String, Valid} objects
	var hat *string
	if activity.Hat.Valid {
		hat = &activity.Hat.String
	}
	var content *string
	if activity.Content.Valid {
		content = &activity.Content.String
	}
	var tokensInput, tokensOutput *int64
	if activity.TokensInput.Valid {
		tokensInput = &activity.TokensInput.Int64
	}
	if activity.TokensOutput.Valid {
		tokensOutput = &activity.TokensOutput.Int64
	}

	r.broadcast("activity.new", map[string]any{
		"task_id":    r.taskID,
		"session_id": r.sessionID,
		"activity": map[string]any{
			"id":            activity.ID,
			"session_id":    activity.SessionID,
			"iteration":     activity.Iteration,
			"event_type":    activity.EventType,
			"hat":           hat,
			"content":       content,
			"tokens_input":  tokensInput,
			"tokens_output": tokensOutput,
			"created_at":    activity.CreatedAt,
		},
	})
}

// RecordUserMessage records a user message sent to Claude
func (r *ActivityRecorder) RecordUserMessage(iteration int, content string) error {
	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeUserMessage,
		r.hat,
		content,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record user message: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// RecordAssistantResponse records Claude's response
func (r *ActivityRecorder) RecordAssistantResponse(iteration int, content string, inputTokens, outputTokens int) error {
	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeAssistantResponse,
		r.hat,
		content,
		&inputTokens,
		&outputTokens,
	)
	if err != nil {
		return fmt.Errorf("failed to record assistant response: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// ToolCallData represents a tool call for activity recording
type ToolCallData struct {
	Name  string `json:"name"`
	Input any    `json:"input"`
}

// RecordToolCall records a tool call made by Claude
func (r *ActivityRecorder) RecordToolCall(iteration int, toolName string, input any) error {
	data := ToolCallData{
		Name:  toolName,
		Input: input,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal tool call: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeToolCall,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record tool call: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// ToolResultData represents a tool result for activity recording
type ToolResultData struct {
	Name   string `json:"name"`
	Result any    `json:"result"`
}

// RecordToolResult records the result of a tool call
func (r *ActivityRecorder) RecordToolResult(iteration int, toolName string, result any) error {
	data := ToolResultData{
		Name:   toolName,
		Result: result,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal tool result: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeToolResult,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record tool result: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// RecordCompletion records a completion signal (task complete, hat complete, etc.)
func (r *ActivityRecorder) RecordCompletion(iteration int, signal string) error {
	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeCompletion,
		r.hat,
		signal,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record completion: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// HatTransitionData represents a hat transition for activity recording
type HatTransitionData struct {
	FromHat string `json:"from_hat"`
	ToHat   string `json:"to_hat"`
}

// RecordHatTransition records a hat transition
func (r *ActivityRecorder) RecordHatTransition(iteration int, fromHat, toHat string) error {
	data := HatTransitionData{
		FromHat: fromHat,
		ToHat:   toHat,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal hat transition: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeHatTransition,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record hat transition: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// DebugLogData represents a debug log entry
type DebugLogData struct {
	Level      string `json:"level"`       // "info", "warn", "error"
	Message    string `json:"message"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Details    any    `json:"details,omitempty"`
}

// RecordDebugLog records a debug-level log entry
func (r *ActivityRecorder) RecordDebugLog(iteration int, level, message string, durationMs int64, details any) error {
	data := DebugLogData{
		Level:      level,
		Message:    message,
		DurationMs: durationMs,
		Details:    details,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal debug log: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeDebugLog,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record debug log: %w", err)
	}
	r.broadcastActivity(activity)
	return nil
}

// Debug is a convenience method for info-level debug logs
func (r *ActivityRecorder) Debug(iteration int, message string) {
	_ = r.RecordDebugLog(iteration, "info", message, 0, nil)
}

// DebugWithDuration logs with timing information
func (r *ActivityRecorder) DebugWithDuration(iteration int, message string, durationMs int64) {
	_ = r.RecordDebugLog(iteration, "info", message, durationMs, nil)
}

// DebugError logs an error-level debug message
func (r *ActivityRecorder) DebugError(iteration int, message string, details any) {
	_ = r.RecordDebugLog(iteration, "error", message, 0, details)
}

// ChecklistUpdateData represents a checklist item update for activity recording
type ChecklistUpdateData struct {
	ItemID      string `json:"item_id"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Notes       string `json:"notes,omitempty"`
}

// RecordChecklistUpdate records a checklist item status change
func (r *ActivityRecorder) RecordChecklistUpdate(iteration int, itemID, status, notes string) error {
	// Try to get item details from DB
	var description string
	var checklistID string
	if r.db != nil {
		if item, err := r.db.GetChecklistItem(itemID); err == nil && item != nil {
			description = item.Description
			checklistID = item.ChecklistID
		}
	}

	data := ChecklistUpdateData{
		ItemID:      itemID,
		Description: description,
		Status:      status,
		Notes:       notes,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal checklist update: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeChecklistUpdate,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record checklist update: %w", err)
	}

	r.broadcastActivity(activity)

	// Also broadcast a specific checklist event for real-time UI updates
	// Using 'checklist.updated' event type with nested 'item' object to match frontend expectations
	if r.broadcast != nil {
		r.broadcast("checklist.updated", map[string]any{
			"task_id":      r.taskID,
			"checklist_id": checklistID,
			"item": map[string]any{
				"id":                 itemID,
				"checklist_id":       checklistID,
				"description":        description,
				"status":             status,
				"verification_notes": notes,
			},
		})
	}

	return nil
}

// QualityGateData represents a quality gate validation attempt
type QualityGateData struct {
	Attempt    int            `json:"attempt"`
	Passed     bool           `json:"passed"`
	Tests      *CheckData     `json:"tests,omitempty"`
	Lint       *CheckData     `json:"lint,omitempty"`
	Build      *CheckData     `json:"build,omitempty"`
	DurationMs int64          `json:"duration_ms"`
}

// CheckData represents a single quality check result
type CheckData struct {
	Passed     bool   `json:"passed"`
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skip_reason,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// RecordQualityGate records a quality gate validation attempt
func (r *ActivityRecorder) RecordQualityGate(iteration int, data *QualityGateData) error {
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal quality gate data: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeQualityGate,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record quality gate: %w", err)
	}

	r.broadcastActivity(activity)
	return nil
}

// LoopHealthData represents a loop health status change
type LoopHealthData struct {
	Status              string `json:"status"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	QualityGateAttempts int    `json:"quality_gate_attempts"`
	TotalFailures       int    `json:"total_failures"`
}

// RecordLoopHealth records a loop health status change
func (r *ActivityRecorder) RecordLoopHealth(iteration int, data *LoopHealthData) error {
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal loop health data: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeLoopHealth,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record loop health: %w", err)
	}

	r.broadcastActivity(activity)
	return nil
}

// DecisionData represents a completion/transition decision
type DecisionData struct {
	Type    string `json:"type"`              // "completion", "transition", "blocked", "quality_gate"
	Signal  string `json:"signal,omitempty"`  // The signal that triggered this decision
	FromHat string `json:"from_hat,omitempty"`
	ToHat   string `json:"to_hat,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// MemoryCreatedData represents a memory creation event
type MemoryCreatedData struct {
	MemoryID string `json:"memory_id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Source   string `json:"source"` // explicit, automatic
}

// RecordDecision records a completion or transition decision
func (r *ActivityRecorder) RecordDecision(iteration int, data *DecisionData) error {
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal decision data: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeDecision,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record decision: %w", err)
	}

	r.broadcastActivity(activity)
	return nil
}

// RecordMemoryCreated records a memory creation event
func (r *ActivityRecorder) RecordMemoryCreated(iteration int, data *MemoryCreatedData) error {
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal memory data: %w", err)
	}

	activity, err := r.db.CreateSessionActivity(
		r.sessionID,
		iteration,
		db.ActivityTypeMemoryCreated,
		r.hat,
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record memory created: %w", err)
	}

	r.broadcastActivity(activity)
	return nil
}
