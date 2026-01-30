// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CreateApproval inserts a new approval request into the database
func (db *DB) CreateApproval(taskID, sessionID *string, approvalType, title string, description *string, data json.RawMessage) (*Approval, error) {
	approval := &Approval{
		ID:        NewPrefixedID("appr"),
		Type:      approvalType,
		Title:     title,
		Status:    ApprovalStatusPending,
		CreatedAt: time.Now(),
	}

	if taskID != nil {
		approval.TaskID = sql.NullString{String: *taskID, Valid: true}
	}
	if sessionID != nil {
		approval.SessionID = sql.NullString{String: *sessionID, Valid: true}
	}
	if description != nil {
		approval.Description = sql.NullString{String: *description, Valid: true}
	}
	if data != nil {
		approval.Data = data
	}

	_, err := db.Exec(
		`INSERT INTO approvals (id, task_id, session_id, type, title, description, data, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ID, approval.TaskID, approval.SessionID, approval.Type,
		approval.Title, approval.Description, string(approval.Data), approval.Status, approval.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create approval: %w", err)
	}

	return approval, nil
}

// GetApprovalByID retrieves an approval by its ID
func (db *DB) GetApprovalByID(id string) (*Approval, error) {
	approval := &Approval{}
	var dataJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, task_id, session_id, type, title, description, data, status, created_at, resolved_at
		 FROM approvals WHERE id = ?`,
		id,
	).Scan(
		&approval.ID, &approval.TaskID, &approval.SessionID, &approval.Type,
		&approval.Title, &approval.Description, &dataJSON, &approval.Status,
		&approval.CreatedAt, &approval.ResolvedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get approval: %w", err)
	}

	if dataJSON.Valid {
		approval.Data = json.RawMessage(dataJSON.String)
	}

	return approval, nil
}

// ListPendingApprovals returns all approvals with pending status
func (db *DB) ListPendingApprovals() ([]*Approval, error) {
	return db.listApprovals(`WHERE status = ? ORDER BY created_at ASC`, ApprovalStatusPending)
}

// ListApprovalsByTask returns all approvals for a task
func (db *DB) ListApprovalsByTask(taskID string) ([]*Approval, error) {
	return db.listApprovals(`WHERE task_id = ? ORDER BY created_at DESC`, taskID)
}

// ListApprovalsBySession returns all approvals for a session
func (db *DB) ListApprovalsBySession(sessionID string) ([]*Approval, error) {
	return db.listApprovals(`WHERE session_id = ? ORDER BY created_at DESC`, sessionID)
}

// ListApprovalsByType returns all approvals of a specific type
func (db *DB) ListApprovalsByType(approvalType string) ([]*Approval, error) {
	return db.listApprovals(`WHERE type = ? ORDER BY created_at DESC`, approvalType)
}

// ListApprovalsByStatus returns all approvals with a given status
func (db *DB) ListApprovalsByStatus(status string) ([]*Approval, error) {
	return db.listApprovals(`WHERE status = ? ORDER BY created_at DESC`, status)
}

// listApprovals is a helper for listing approvals with a WHERE clause
func (db *DB) listApprovals(whereClause string, args ...any) ([]*Approval, error) {
	query := `SELECT id, task_id, session_id, type, title, description, data, status, created_at, resolved_at
	          FROM approvals ` + whereClause

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list approvals: %w", err)
	}
	defer rows.Close()

	var approvals []*Approval
	for rows.Next() {
		approval := &Approval{}
		var dataJSON sql.NullString

		err := rows.Scan(
			&approval.ID, &approval.TaskID, &approval.SessionID, &approval.Type,
			&approval.Title, &approval.Description, &dataJSON, &approval.Status,
			&approval.CreatedAt, &approval.ResolvedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval: %w", err)
		}

		if dataJSON.Valid {
			approval.Data = json.RawMessage(dataJSON.String)
		}
		approvals = append(approvals, approval)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approvals: %w", err)
	}

	return approvals, nil
}

// ApproveApproval marks an approval as approved
func (db *DB) ApproveApproval(id string) error {
	return db.resolveApproval(id, ApprovalStatusApproved)
}

// RejectApproval marks an approval as rejected
func (db *DB) RejectApproval(id string) error {
	return db.resolveApproval(id, ApprovalStatusRejected)
}

// resolveApproval updates an approval's status and sets resolved_at
func (db *DB) resolveApproval(id, status string) error {
	result, err := db.Exec(
		`UPDATE approvals SET status = ?, resolved_at = ? WHERE id = ? AND status = ?`,
		status, time.Now(), id, ApprovalStatusPending,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve approval: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Check if approval exists
		approval, err := db.GetApprovalByID(id)
		if err != nil {
			return err
		}
		if approval == nil {
			return fmt.Errorf("approval not found: %s", id)
		}
		return fmt.Errorf("approval already resolved: %s (status: %s)", id, approval.Status)
	}

	return nil
}

// DeleteApproval removes an approval from the database
func (db *DB) DeleteApproval(id string) error {
	result, err := db.Exec(`DELETE FROM approvals WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete approval: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("approval not found: %s", id)
	}

	return nil
}

// DeleteApprovalsByTask removes all approvals for a task
func (db *DB) DeleteApprovalsByTask(taskID string) error {
	_, err := db.Exec(`DELETE FROM approvals WHERE task_id = ?`, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task approvals: %w", err)
	}

	return nil
}

// DeleteApprovalsBySession removes all approvals for a session
func (db *DB) DeleteApprovalsBySession(sessionID string) error {
	_, err := db.Exec(`DELETE FROM approvals WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session approvals: %w", err)
	}

	return nil
}

// CountPendingApprovals returns the number of pending approvals
func (db *DB) CountPendingApprovals() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM approvals WHERE status = ?`, ApprovalStatusPending).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending approvals: %w", err)
	}
	return count, nil
}

// CountPendingApprovalsByTask returns the number of pending approvals for a task
func (db *DB) CountPendingApprovalsByTask(taskID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM approvals WHERE task_id = ? AND status = ?`, taskID, ApprovalStatusPending).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count task pending approvals: %w", err)
	}
	return count, nil
}

// HasPendingApproval checks if a task or session has any pending approvals
func (db *DB) HasPendingApproval(taskID, sessionID *string) (bool, error) {
	var query string
	var args []any

	if taskID != nil && sessionID != nil {
		query = `SELECT EXISTS(SELECT 1 FROM approvals WHERE (task_id = ? OR session_id = ?) AND status = ?)`
		args = []any{*taskID, *sessionID, ApprovalStatusPending}
	} else if taskID != nil {
		query = `SELECT EXISTS(SELECT 1 FROM approvals WHERE task_id = ? AND status = ?)`
		args = []any{*taskID, ApprovalStatusPending}
	} else if sessionID != nil {
		query = `SELECT EXISTS(SELECT 1 FROM approvals WHERE session_id = ? AND status = ?)`
		args = []any{*sessionID, ApprovalStatusPending}
	} else {
		return false, nil
	}

	var exists bool
	err := db.QueryRow(query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check pending approvals: %w", err)
	}

	return exists, nil
}
