// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// SessionActivity represents a recorded activity event during session execution
type SessionActivity struct {
	ID           string
	SessionID    string
	Iteration    int
	EventType    string // "user_message", "assistant_response", "tool_call", "tool_result", "completion_signal", "hat_transition"
	Content      sql.NullString
	TokensInput  sql.NullInt64
	TokensOutput sql.NullInt64
	CreatedAt    time.Time
}

// Activity event type constants
const (
	ActivityTypeUserMessage       = "user_message"
	ActivityTypeAssistantResponse = "assistant_response"
	ActivityTypeToolCall          = "tool_call"
	ActivityTypeToolResult        = "tool_result"
	ActivityTypeCompletion        = "completion_signal"
	ActivityTypeHatTransition     = "hat_transition"
	ActivityTypeDebugLog          = "debug_log"
	ActivityTypeChecklistUpdate   = "checklist_update"
)

// CreateSessionActivity inserts a new activity record
func (db *DB) CreateSessionActivity(sessionID string, iteration int, eventType string, content string, tokensInput, tokensOutput *int) (*SessionActivity, error) {
	activity := &SessionActivity{
		ID:        NewPrefixedID("act"),
		SessionID: sessionID,
		Iteration: iteration,
		EventType: eventType,
		CreatedAt: time.Now(),
	}

	if content != "" {
		activity.Content = sql.NullString{String: content, Valid: true}
	}

	var inputVal, outputVal any
	if tokensInput != nil {
		activity.TokensInput = sql.NullInt64{Int64: int64(*tokensInput), Valid: true}
		inputVal = *tokensInput
	}
	if tokensOutput != nil {
		activity.TokensOutput = sql.NullInt64{Int64: int64(*tokensOutput), Valid: true}
		outputVal = *tokensOutput
	}

	_, err := db.Exec(
		`INSERT INTO session_activity (id, session_id, iteration, event_type, content, tokens_input, tokens_output, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		activity.ID, activity.SessionID, activity.Iteration, activity.EventType,
		activity.Content, inputVal, outputVal, activity.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session activity: %w", err)
	}

	return activity, nil
}

// ListSessionActivity returns all activity for a session, ordered by creation time
func (db *DB) ListSessionActivity(sessionID string) ([]*SessionActivity, error) {
	rows, err := db.Query(
		`SELECT id, session_id, iteration, event_type, content, tokens_input, tokens_output, created_at
		 FROM session_activity WHERE session_id = ?
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list session activity: %w", err)
	}
	defer rows.Close()

	var activities []*SessionActivity
	for rows.Next() {
		activity := &SessionActivity{}
		err := rows.Scan(
			&activity.ID, &activity.SessionID, &activity.Iteration,
			&activity.EventType, &activity.Content, &activity.TokensInput,
			&activity.TokensOutput, &activity.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating activities: %w", err)
	}

	return activities, nil
}

// ListTaskActivity returns all activity for all sessions of a task
func (db *DB) ListTaskActivity(taskID string) ([]*SessionActivity, error) {
	rows, err := db.Query(
		`SELECT a.id, a.session_id, a.iteration, a.event_type, a.content, a.tokens_input, a.tokens_output, a.created_at
		 FROM session_activity a
		 JOIN sessions s ON a.session_id = s.id
		 WHERE s.task_id = ?
		 ORDER BY a.created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list task activity: %w", err)
	}
	defer rows.Close()

	var activities []*SessionActivity
	for rows.Next() {
		activity := &SessionActivity{}
		err := rows.Scan(
			&activity.ID, &activity.SessionID, &activity.Iteration,
			&activity.EventType, &activity.Content, &activity.TokensInput,
			&activity.TokensOutput, &activity.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating activities: %w", err)
	}

	return activities, nil
}

// GetSessionActivitySummary returns a summary of activity for a session
func (db *DB) GetSessionActivitySummary(sessionID string) (*SessionActivitySummary, error) {
	summary := &SessionActivitySummary{}

	// Get max iteration
	err := db.QueryRow(
		`SELECT COALESCE(MAX(iteration), 0) FROM session_activity WHERE session_id = ?`,
		sessionID,
	).Scan(&summary.TotalIterations)
	if err != nil {
		return nil, fmt.Errorf("failed to get max iteration: %w", err)
	}

	// Get total tokens
	err = db.QueryRow(
		`SELECT COALESCE(SUM(tokens_input), 0) + COALESCE(SUM(tokens_output), 0)
		 FROM session_activity WHERE session_id = ?`,
		sessionID,
	).Scan(&summary.TotalTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to get total tokens: %w", err)
	}

	// Get completion reason (from the last completion_signal event)
	var completionReason sql.NullString
	err = db.QueryRow(
		`SELECT content FROM session_activity
		 WHERE session_id = ? AND event_type = ?
		 ORDER BY created_at DESC LIMIT 1`,
		sessionID, ActivityTypeCompletion,
	).Scan(&completionReason)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get completion reason: %w", err)
	}
	if completionReason.Valid {
		summary.CompletionReason = completionReason.String
	}

	return summary, nil
}

// SessionActivitySummary provides aggregated stats for a session's activity
type SessionActivitySummary struct {
	TotalIterations  int    `json:"total_iterations"`
	TotalTokens      int64  `json:"total_tokens"`
	CompletionReason string `json:"completion_reason,omitempty"`
}

// DeleteSessionActivity removes all activity records for a session
func (db *DB) DeleteSessionActivity(sessionID string) error {
	_, err := db.Exec(`DELETE FROM session_activity WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session activity: %w", err)
	}
	return nil
}
