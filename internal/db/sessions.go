// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CreateSession inserts a new session into the database
func (db *DB) CreateSession(taskID, hat, worktreePath string) (*Session, error) {
	session := &Session{
		ID:           NewPrefixedID("sess"),
		TaskID:       taskID,
		Hat:          hat,
		Status:       SessionStatusPending,
		WorktreePath: worktreePath,
		MaxIterations: 100,
		CreatedAt:    time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO sessions (id, task_id, hat, status, worktree_path, max_iterations, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.TaskID, session.Hat, session.Status,
		session.WorktreePath, session.MaxIterations, session.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSessionByID retrieves a session by its ID
func (db *DB) GetSessionByID(id string) (*Session, error) {
	session := &Session{}
	err := db.QueryRow(
		`SELECT id, task_id, hat, claude_session_id, status, worktree_path,
		        iteration_count, max_iterations, completion_promise,
		        tokens_used, tokens_budget, dollars_used, dollars_budget,
		        created_at, started_at, ended_at, outcome
		 FROM sessions WHERE id = ?`,
		id,
	).Scan(
		&session.ID, &session.TaskID, &session.Hat, &session.ClaudeSessionID,
		&session.Status, &session.WorktreePath, &session.IterationCount,
		&session.MaxIterations, &session.CompletionPromise, &session.TokensUsed,
		&session.TokensBudget, &session.DollarsUsed, &session.DollarsBudget,
		&session.CreatedAt, &session.StartedAt, &session.EndedAt, &session.Outcome,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// ListSessionsByTask returns all sessions for a task
func (db *DB) ListSessionsByTask(taskID string) ([]*Session, error) {
	return db.listSessions(`WHERE task_id = ? ORDER BY created_at DESC`, taskID)
}

// ListSessionsByStatus returns all sessions with a given status
func (db *DB) ListSessionsByStatus(status string) ([]*Session, error) {
	return db.listSessions(`WHERE status = ? ORDER BY created_at DESC`, status)
}

// ListActiveSessions returns all running or paused sessions
func (db *DB) ListActiveSessions() ([]*Session, error) {
	return db.listSessions(`WHERE status IN (?, ?) ORDER BY created_at DESC`, SessionStatusRunning, SessionStatusPaused)
}

// listSessions is a helper for listing sessions with a WHERE clause
func (db *DB) listSessions(whereClause string, args ...any) ([]*Session, error) {
	query := `SELECT id, task_id, hat, claude_session_id, status, worktree_path,
	                 iteration_count, max_iterations, completion_promise,
	                 tokens_used, tokens_budget, dollars_used, dollars_budget,
	                 created_at, started_at, ended_at, outcome
	          FROM sessions ` + whereClause

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		err := rows.Scan(
			&session.ID, &session.TaskID, &session.Hat, &session.ClaudeSessionID,
			&session.Status, &session.WorktreePath, &session.IterationCount,
			&session.MaxIterations, &session.CompletionPromise, &session.TokensUsed,
			&session.TokensBudget, &session.DollarsUsed, &session.DollarsBudget,
			&session.CreatedAt, &session.StartedAt, &session.EndedAt, &session.Outcome,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// UpdateSessionStatus updates a session's status
func (db *DB) UpdateSessionStatus(id, status string) error {
	var startedAt, endedAt any
	now := time.Now()

	switch status {
	case SessionStatusRunning:
		startedAt = now
	case SessionStatusCompleted, SessionStatusFailed:
		endedAt = now
	}

	result, err := db.Exec(
		`UPDATE sessions SET status = ?, started_at = COALESCE(?, started_at), ended_at = COALESCE(?, ended_at) WHERE id = ?`,
		status, startedAt, endedAt, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// UpdateSessionClaudeID sets the Claude session ID for resume capability
func (db *DB) UpdateSessionClaudeID(id, claudeSessionID string) error {
	result, err := db.Exec(`UPDATE sessions SET claude_session_id = ? WHERE id = ?`, claudeSessionID, id)
	if err != nil {
		return fmt.Errorf("failed to update session claude ID: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// UpdateSessionIteration updates the iteration count for a session
func (db *DB) UpdateSessionIteration(id string, iterationCount int) error {
	result, err := db.Exec(`UPDATE sessions SET iteration_count = ? WHERE id = ?`, iterationCount, id)
	if err != nil {
		return fmt.Errorf("failed to update session iteration: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// UpdateSessionUsage updates the token and dollar usage for a session
func (db *DB) UpdateSessionUsage(id string, tokensUsed int64, dollarsUsed float64) error {
	result, err := db.Exec(
		`UPDATE sessions SET tokens_used = ?, dollars_used = ? WHERE id = ?`,
		tokensUsed, dollarsUsed, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update session usage: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// UpdateSessionOutcome sets the outcome for a completed or failed session
func (db *DB) UpdateSessionOutcome(id, outcome string) error {
	result, err := db.Exec(`UPDATE sessions SET outcome = ? WHERE id = ?`, outcome, id)
	if err != nil {
		return fmt.Errorf("failed to update session outcome: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// SetSessionBudgets sets the token and dollar budgets for a session
func (db *DB) SetSessionBudgets(id string, tokensBudget *int64, dollarsBudget *float64) error {
	result, err := db.Exec(
		`UPDATE sessions SET tokens_budget = ?, dollars_budget = ? WHERE id = ?`,
		tokensBudget, dollarsBudget, id,
	)
	if err != nil {
		return fmt.Errorf("failed to set session budgets: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// DeleteSession removes a session from the database
func (db *DB) DeleteSession(id string) error {
	// First delete activity
	_, err := db.Exec(`DELETE FROM session_activity WHERE session_id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete session activity: %w", err)
	}

	// Delete checkpoints
	_, err = db.Exec(`DELETE FROM session_checkpoints WHERE session_id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete session checkpoints: %w", err)
	}

	result, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// CreateSessionCheckpoint saves a checkpoint for a session
func (db *DB) CreateSessionCheckpoint(sessionID string, iteration int, state json.RawMessage) (*SessionCheckpoint, error) {
	checkpoint := &SessionCheckpoint{
		ID:        NewPrefixedID("ckpt"),
		SessionID: sessionID,
		Iteration: iteration,
		State:     state,
		CreatedAt: time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO session_checkpoints (id, session_id, iteration, state, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		checkpoint.ID, checkpoint.SessionID, checkpoint.Iteration, string(checkpoint.State), checkpoint.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session checkpoint: %w", err)
	}

	return checkpoint, nil
}

// GetLatestSessionCheckpoint retrieves the most recent checkpoint for a session
func (db *DB) GetLatestSessionCheckpoint(sessionID string) (*SessionCheckpoint, error) {
	checkpoint := &SessionCheckpoint{}
	var stateJSON string

	err := db.QueryRow(
		`SELECT id, session_id, iteration, state, created_at
		 FROM session_checkpoints WHERE session_id = ?
		 ORDER BY iteration DESC LIMIT 1`,
		sessionID,
	).Scan(&checkpoint.ID, &checkpoint.SessionID, &checkpoint.Iteration, &stateJSON, &checkpoint.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest checkpoint: %w", err)
	}

	checkpoint.State = json.RawMessage(stateJSON)
	return checkpoint, nil
}

// ListSessionCheckpoints returns all checkpoints for a session
func (db *DB) ListSessionCheckpoints(sessionID string) ([]*SessionCheckpoint, error) {
	rows, err := db.Query(
		`SELECT id, session_id, iteration, state, created_at
		 FROM session_checkpoints WHERE session_id = ?
		 ORDER BY iteration ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list session checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []*SessionCheckpoint
	for rows.Next() {
		checkpoint := &SessionCheckpoint{}
		var stateJSON string

		err := rows.Scan(&checkpoint.ID, &checkpoint.SessionID, &checkpoint.Iteration, &stateJSON, &checkpoint.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan checkpoint: %w", err)
		}

		checkpoint.State = json.RawMessage(stateJSON)
		checkpoints = append(checkpoints, checkpoint)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating checkpoints: %w", err)
	}

	return checkpoints, nil
}

// DeleteSessionCheckpoints removes all checkpoints for a session
func (db *DB) DeleteSessionCheckpoints(sessionID string) error {
	_, err := db.Exec(`DELETE FROM session_checkpoints WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session checkpoints: %w", err)
	}

	return nil
}
