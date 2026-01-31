// Package session provides session lifecycle management for Poindexter
package session

import (
	"encoding/json"
	"fmt"

	"github.com/lirancohen/dex/internal/db"
)

// ActivityRecorder records session activity to the database
type ActivityRecorder struct {
	db        *db.DB
	sessionID string
}

// NewActivityRecorder creates a new ActivityRecorder for a session
func NewActivityRecorder(database *db.DB, sessionID string) *ActivityRecorder {
	return &ActivityRecorder{
		db:        database,
		sessionID: sessionID,
	}
}

// RecordUserMessage records a user message sent to Claude
func (r *ActivityRecorder) RecordUserMessage(iteration int, content string) error {
	_, err := r.db.CreateSessionActivity(
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
	return nil
}

// RecordAssistantResponse records Claude's response
func (r *ActivityRecorder) RecordAssistantResponse(iteration int, content string, inputTokens, outputTokens int) error {
	_, err := r.db.CreateSessionActivity(
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

	_, err = r.db.CreateSessionActivity(
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

	_, err = r.db.CreateSessionActivity(
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
	return nil
}

// RecordCompletion records a completion signal (task complete, hat complete, etc.)
func (r *ActivityRecorder) RecordCompletion(iteration int, signal string) error {
	_, err := r.db.CreateSessionActivity(
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

	_, err = r.db.CreateSessionActivity(
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
	return nil
}
