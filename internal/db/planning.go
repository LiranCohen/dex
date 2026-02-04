// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreatePlanningSession creates a new planning session for a task
func (db *DB) CreatePlanningSession(taskID, originalPrompt string) (*PlanningSession, error) {
	session := &PlanningSession{
		ID:             NewPrefixedID("plan"),
		TaskID:         taskID,
		Status:         PlanningStatusProcessing,
		OriginalPrompt: originalPrompt,
		CreatedAt:      time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO planning_sessions (id, task_id, status, original_prompt, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.TaskID, session.Status, session.OriginalPrompt, session.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning session: %w", err)
	}

	return session, nil
}

// GetPlanningSessionByID retrieves a planning session by its ID
func (db *DB) GetPlanningSessionByID(id string) (*PlanningSession, error) {
	session := &PlanningSession{}

	err := db.QueryRow(
		`SELECT id, task_id, status, refined_prompt, original_prompt, pending_checklist, created_at, completed_at
		 FROM planning_sessions WHERE id = ?`,
		id,
	).Scan(
		&session.ID, &session.TaskID, &session.Status,
		&session.RefinedPrompt, &session.OriginalPrompt, &session.PendingChecklist,
		&session.CreatedAt, &session.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get planning session: %w", err)
	}

	return session, nil
}

// GetPlanningSessionByTaskID retrieves a planning session by task ID
func (db *DB) GetPlanningSessionByTaskID(taskID string) (*PlanningSession, error) {
	session := &PlanningSession{}

	err := db.QueryRow(
		`SELECT id, task_id, status, refined_prompt, original_prompt, pending_checklist, created_at, completed_at
		 FROM planning_sessions WHERE task_id = ?`,
		taskID,
	).Scan(
		&session.ID, &session.TaskID, &session.Status,
		&session.RefinedPrompt, &session.OriginalPrompt, &session.PendingChecklist,
		&session.CreatedAt, &session.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get planning session by task: %w", err)
	}

	return session, nil
}

// SetPendingChecklist stores the pending checklist JSON in the planning session
func (db *DB) SetPendingChecklist(id, checklistJSON string) error {
	result, err := db.Exec(
		`UPDATE planning_sessions SET pending_checklist = ? WHERE id = ?`,
		checklistJSON, id,
	)
	if err != nil {
		return fmt.Errorf("failed to set pending checklist: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("planning session not found: %s", id)
	}

	return nil
}

// UpdatePlanningSessionStatus updates the status of a planning session
func (db *DB) UpdatePlanningSessionStatus(id, status string) error {
	result, err := db.Exec(
		`UPDATE planning_sessions SET status = ? WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update planning session status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("planning session not found: %s", id)
	}

	return nil
}

// CompletePlanningSession marks a planning session as completed with the refined prompt
func (db *DB) CompletePlanningSession(id, refinedPrompt string) error {
	result, err := db.Exec(
		`UPDATE planning_sessions SET status = ?, refined_prompt = ?, completed_at = ? WHERE id = ?`,
		PlanningStatusCompleted, refinedPrompt, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to complete planning session: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("planning session not found: %s", id)
	}

	return nil
}

// SkipPlanningSession marks a planning session as skipped
func (db *DB) SkipPlanningSession(id string) error {
	result, err := db.Exec(
		`UPDATE planning_sessions SET status = ?, completed_at = ? WHERE id = ?`,
		PlanningStatusSkipped, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to skip planning session: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("planning session not found: %s", id)
	}

	return nil
}

// DeletePlanningSession removes a planning session and its messages
func (db *DB) DeletePlanningSession(id string) error {
	result, err := db.Exec(`DELETE FROM planning_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete planning session: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("planning session not found: %s", id)
	}

	return nil
}

// CreatePlanningMessage creates a new message in a planning session
func (db *DB) CreatePlanningMessage(planningSessionID, role, content string) (*PlanningMessage, error) {
	msg := &PlanningMessage{
		ID:                NewPrefixedID("pmsg"),
		PlanningSessionID: planningSessionID,
		Role:              role,
		Content:           content,
		CreatedAt:         time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO planning_messages (id, planning_session_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.PlanningSessionID, msg.Role, msg.Content, msg.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning message: %w", err)
	}

	return msg, nil
}

// GetPlanningMessages retrieves all messages for a planning session in chronological order
func (db *DB) GetPlanningMessages(planningSessionID string) ([]*PlanningMessage, error) {
	rows, err := db.Query(
		`SELECT id, planning_session_id, role, content, created_at
		 FROM planning_messages WHERE planning_session_id = ?
		 ORDER BY created_at ASC`,
		planningSessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get planning messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []*PlanningMessage
	for rows.Next() {
		msg := &PlanningMessage{}
		err := rows.Scan(&msg.ID, &msg.PlanningSessionID, &msg.Role, &msg.Content, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan planning message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating planning messages: %w", err)
	}

	return messages, nil
}

// DeletePlanningMessages removes all messages for a planning session
func (db *DB) DeletePlanningMessages(planningSessionID string) error {
	_, err := db.Exec(`DELETE FROM planning_messages WHERE planning_session_id = ?`, planningSessionID)
	if err != nil {
		return fmt.Errorf("failed to delete planning messages: %w", err)
	}
	return nil
}
