package content

import (
	"fmt"
	"path/filepath"
	"time"
)

const (
	specFileName      = "spec.md"
	checklistFileName = "checklist.md"
	notesFileName     = "notes.md"
)

// WriteTaskSpec writes the task specification to a git file
func (m *Manager) WriteTaskSpec(taskID string, spec TaskSpec) error {
	content := FormatTaskSpec(spec)
	path := filepath.Join(TaskContentPath(taskID), specFileName)
	return m.writeFile(path, content)
}

// ReadTaskSpec reads and parses the task specification from a git file
func (m *Manager) ReadTaskSpec(taskID string) (string, error) {
	path := filepath.Join(TaskContentPath(taskID), specFileName)
	return m.readFile(path)
}

// WriteTaskChecklist writes the task checklist to a git file
func (m *Manager) WriteTaskChecklist(taskID string, items []ChecklistItem) error {
	content := FormatChecklist(items)
	path := filepath.Join(TaskContentPath(taskID), checklistFileName)
	return m.writeFile(path, content)
}

// ReadTaskChecklist reads and parses the task checklist from a git file
func (m *Manager) ReadTaskChecklist(taskID string) ([]ChecklistItem, error) {
	path := filepath.Join(TaskContentPath(taskID), checklistFileName)
	content, err := m.readFile(path)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, nil
	}
	return ParseChecklist(content), nil
}

// UpdateTaskChecklistItem updates a specific checklist item's status
func (m *Manager) UpdateTaskChecklistItem(taskID, description string, status ChecklistStatus) error {
	path := filepath.Join(TaskContentPath(taskID), checklistFileName)
	content, err := m.readFile(path)
	if err != nil {
		return fmt.Errorf("failed to read checklist: %w", err)
	}
	if content == "" {
		return fmt.Errorf("checklist not found for task %s", taskID)
	}

	updated := UpdateChecklistItemStatus(content, description, status)
	return m.writeFile(path, updated)
}

// WriteTaskNotes writes additional notes for a task
func (m *Manager) WriteTaskNotes(taskID, notes string) error {
	path := filepath.Join(TaskContentPath(taskID), notesFileName)
	return m.writeFile(path, notes)
}

// ReadTaskNotes reads additional notes for a task
func (m *Manager) ReadTaskNotes(taskID string) (string, error) {
	path := filepath.Join(TaskContentPath(taskID), notesFileName)
	return m.readFile(path)
}

// TaskContentExists checks if task content files exist
func (m *Manager) TaskContentExists(taskID string) bool {
	path := filepath.Join(TaskContentPath(taskID), specFileName)
	return m.fileExists(path)
}

// InitTaskContent creates the initial content files for a task
// This is a convenience method that creates spec and optionally checklist
func (m *Manager) InitTaskContent(taskID, title, description, projectName, questID string, checklist []ChecklistItem) error {
	// Create the spec file
	spec := TaskSpec{
		Title:       title,
		Description: description,
		ProjectName: projectName,
		QuestID:     questID,
		CreatedAt:   time.Now(),
	}

	if err := m.WriteTaskSpec(taskID, spec); err != nil {
		return fmt.Errorf("failed to write task spec: %w", err)
	}

	// Create checklist if provided
	if len(checklist) > 0 {
		if err := m.WriteTaskChecklist(taskID, checklist); err != nil {
			return fmt.Errorf("failed to write task checklist: %w", err)
		}
	}

	return nil
}

// GetTaskContentPath returns the full path to task content directory
func (m *Manager) GetTaskContentPath(taskID string) string {
	return filepath.Join(m.basePath, TaskContentPath(taskID))
}

// GetTaskSpecPath returns the full path to the task spec file
func (m *Manager) GetTaskSpecPath(taskID string) string {
	return filepath.Join(m.basePath, TaskContentPath(taskID), specFileName)
}

// GetTaskChecklistPath returns the full path to the task checklist file
func (m *Manager) GetTaskChecklistPath(taskID string) string {
	return filepath.Join(m.basePath, TaskContentPath(taskID), checklistFileName)
}
