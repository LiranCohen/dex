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

// broadcastActivity sends an activity event through WebSocket
func (r *ActivityRecorder) broadcastActivity(activity *db.SessionActivity) {
	if r.broadcast == nil {
		return
	}

	// Extract values from nullable types to avoid serializing as {String, Valid} objects
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
	// Try to get item description from DB
	var description string
	if r.db != nil {
		if item, err := r.db.GetChecklistItem(itemID); err == nil && item != nil {
			description = item.Description
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
		string(content),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to record checklist update: %w", err)
	}

	r.broadcastActivity(activity)

	// Also broadcast a specific checklist event for real-time UI updates
	if r.broadcast != nil {
		r.broadcast("checklist.item_updated", map[string]any{
			"task_id":     r.taskID,
			"item_id":     itemID,
			"description": description,
			"status":      status,
			"notes":       notes,
		})
	}

	return nil
}
