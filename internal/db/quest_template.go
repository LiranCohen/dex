// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateQuestTemplate creates a new quest template
func (db *DB) CreateQuestTemplate(projectID, name, description, initialPrompt string) (*QuestTemplate, error) {
	template := &QuestTemplate{
		ID:            NewPrefixedID("qtpl"),
		ProjectID:     projectID,
		Name:          name,
		Description:   sql.NullString{String: description, Valid: description != ""},
		InitialPrompt: initialPrompt,
		CreatedAt:     time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO quest_templates (id, project_id, name, description, initial_prompt, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		template.ID, template.ProjectID, template.Name, template.Description, template.InitialPrompt, template.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create quest template: %w", err)
	}

	return template, nil
}

// GetQuestTemplateByID retrieves a quest template by its ID
func (db *DB) GetQuestTemplateByID(id string) (*QuestTemplate, error) {
	template := &QuestTemplate{}

	err := db.QueryRow(
		`SELECT id, project_id, name, description, initial_prompt, created_at
		 FROM quest_templates WHERE id = ?`,
		id,
	).Scan(
		&template.ID, &template.ProjectID, &template.Name,
		&template.Description, &template.InitialPrompt, &template.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get quest template: %w", err)
	}

	return template, nil
}

// GetQuestTemplatesByProjectID retrieves all quest templates for a project
func (db *DB) GetQuestTemplatesByProjectID(projectID string) ([]*QuestTemplate, error) {
	rows, err := db.Query(
		`SELECT id, project_id, name, description, initial_prompt, created_at
		 FROM quest_templates WHERE project_id = ? ORDER BY name ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get quest templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []*QuestTemplate
	for rows.Next() {
		template := &QuestTemplate{}
		err := rows.Scan(
			&template.ID, &template.ProjectID, &template.Name,
			&template.Description, &template.InitialPrompt, &template.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quest template: %w", err)
		}
		templates = append(templates, template)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating quest templates: %w", err)
	}

	return templates, nil
}

// UpdateQuestTemplate updates a quest template
func (db *DB) UpdateQuestTemplate(id, name, description, initialPrompt string) error {
	result, err := db.Exec(
		`UPDATE quest_templates SET name = ?, description = ?, initial_prompt = ? WHERE id = ?`,
		name, sql.NullString{String: description, Valid: description != ""}, initialPrompt, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quest template: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest template not found: %s", id)
	}

	return nil
}

// DeleteQuestTemplate removes a quest template
func (db *DB) DeleteQuestTemplate(id string) error {
	result, err := db.Exec(`DELETE FROM quest_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete quest template: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest template not found: %s", id)
	}

	return nil
}
