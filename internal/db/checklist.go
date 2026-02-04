// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateTaskChecklist creates a new checklist for a task
func (db *DB) CreateTaskChecklist(taskID string) (*TaskChecklist, error) {
	checklist := &TaskChecklist{
		ID:        NewPrefixedID("chkl"),
		TaskID:    taskID,
		CreatedAt: time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO task_checklists (id, task_id, created_at)
		 VALUES (?, ?, ?)`,
		checklist.ID, checklist.TaskID, checklist.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task checklist: %w", err)
	}

	return checklist, nil
}

// GetChecklistByID retrieves a checklist by its ID
func (db *DB) GetChecklistByID(id string) (*TaskChecklist, error) {
	checklist := &TaskChecklist{}

	err := db.QueryRow(
		`SELECT id, task_id, created_at FROM task_checklists WHERE id = ?`,
		id,
	).Scan(&checklist.ID, &checklist.TaskID, &checklist.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get checklist: %w", err)
	}

	return checklist, nil
}

// GetChecklistByTaskID retrieves a checklist by task ID
func (db *DB) GetChecklistByTaskID(taskID string) (*TaskChecklist, error) {
	checklist := &TaskChecklist{}

	err := db.QueryRow(
		`SELECT id, task_id, created_at FROM task_checklists WHERE task_id = ?`,
		taskID,
	).Scan(&checklist.ID, &checklist.TaskID, &checklist.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get checklist by task: %w", err)
	}

	return checklist, nil
}

// DeleteTaskChecklist deletes a checklist and its items (cascades via foreign key)
func (db *DB) DeleteTaskChecklist(id string) error {
	result, err := db.Exec(`DELETE FROM task_checklists WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete checklist: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("checklist not found: %s", id)
	}

	return nil
}

// CreateChecklistItem creates a new item in a checklist
func (db *DB) CreateChecklistItem(checklistID, description string, sortOrder int) (*ChecklistItem, error) {
	item := &ChecklistItem{
		ID:          NewPrefixedID("citm"),
		ChecklistID: checklistID,
		Description: description,
		Status:      ChecklistItemStatusPending,
		SortOrder:   sortOrder,
	}

	_, err := db.Exec(
		`INSERT INTO checklist_items (id, checklist_id, description, status, sort_order)
		 VALUES (?, ?, ?, ?, ?)`,
		item.ID, item.ChecklistID, item.Description, item.Status, item.SortOrder,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create checklist item: %w", err)
	}

	return item, nil
}

// GetChecklistItem retrieves a checklist item by ID
func (db *DB) GetChecklistItem(id string) (*ChecklistItem, error) {
	item := &ChecklistItem{}

	err := db.QueryRow(
		`SELECT id, checklist_id, parent_id, description, status, verification_notes, completed_at, sort_order
		 FROM checklist_items WHERE id = ?`,
		id,
	).Scan(
		&item.ID, &item.ChecklistID, &item.ParentID, &item.Description,
		&item.Status, &item.VerificationNotes, &item.CompletedAt, &item.SortOrder,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get checklist item: %w", err)
	}

	return item, nil
}

// GetChecklistItems retrieves all items for a checklist ordered by sort_order
func (db *DB) GetChecklistItems(checklistID string) ([]*ChecklistItem, error) {
	rows, err := db.Query(
		`SELECT id, checklist_id, parent_id, description, status, verification_notes, completed_at, sort_order
		 FROM checklist_items WHERE checklist_id = ? ORDER BY sort_order ASC`,
		checklistID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get checklist items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*ChecklistItem
	for rows.Next() {
		item := &ChecklistItem{}
		err := rows.Scan(
			&item.ID, &item.ChecklistID, &item.ParentID, &item.Description,
			&item.Status, &item.VerificationNotes, &item.CompletedAt, &item.SortOrder,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan checklist item: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating checklist items: %w", err)
	}

	return items, nil
}

// UpdateChecklistItemStatus updates the status of a checklist item
func (db *DB) UpdateChecklistItemStatus(id, status string, verificationNotes string) error {
	var completedAt sql.NullTime
	if status == ChecklistItemStatusDone || status == ChecklistItemStatusFailed || status == ChecklistItemStatusSkipped {
		completedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	var notes sql.NullString
	if verificationNotes != "" {
		notes = sql.NullString{String: verificationNotes, Valid: true}
	}

	result, err := db.Exec(
		`UPDATE checklist_items SET status = ?, verification_notes = ?, completed_at = ? WHERE id = ?`,
		status, notes, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update checklist item status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("checklist item not found: %s", id)
	}

	return nil
}

// DeleteChecklistItem deletes a checklist item
func (db *DB) DeleteChecklistItem(id string) error {
	result, err := db.Exec(`DELETE FROM checklist_items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete checklist item: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("checklist item not found: %s", id)
	}

	return nil
}

// ChecklistIssue represents an issue with a checklist item
type ChecklistIssue struct {
	ItemID      string
	Description string
	Status      string
	Notes       string
}

// GetChecklistIssues returns all items that are not done
func (db *DB) GetChecklistIssues(checklistID string) ([]ChecklistIssue, error) {
	items, err := db.GetChecklistItems(checklistID)
	if err != nil {
		return nil, err
	}

	var issues []ChecklistIssue
	for _, item := range items {
		if item.Status != ChecklistItemStatusDone {
			issues = append(issues, ChecklistIssue{
				ItemID:      item.ID,
				Description: item.Description,
				Status:      item.Status,
				Notes:       item.GetVerificationNotes(),
			})
		}
	}

	return issues, nil
}

// HasChecklistIssues checks if a checklist has any unfinished items
func (db *DB) HasChecklistIssues(checklistID string) (bool, error) {
	issues, err := db.GetChecklistIssues(checklistID)
	if err != nil {
		return false, err
	}
	return len(issues) > 0, nil
}
