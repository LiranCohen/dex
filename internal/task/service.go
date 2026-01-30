// Package task provides task management services for Poindexter
package task

import (
	"fmt"

	"github.com/lirancohen/dex/internal/db"
)

// Service handles task-related business logic
type Service struct {
	db           *db.DB
	stateMachine *StateMachine
}

// NewService creates a new task service
func NewService(database *db.DB) *Service {
	return &Service{
		db:           database,
		stateMachine: NewStateMachine(database),
	}
}

// NewServiceWithStateMachine creates a new task service with a custom state machine
func NewServiceWithStateMachine(database *db.DB, sm *StateMachine) *Service {
	return &Service{
		db:           database,
		stateMachine: sm,
	}
}

// Create creates a new task with default values
func (s *Service) Create(projectID, title, taskType string, priority int) (*db.Task, error) {
	// Validate inputs
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if priority < 1 || priority > 5 {
		priority = 3 // Default to medium priority
	}
	if !IsValidTaskType(taskType) {
		taskType = db.TaskTypeTask // Default to generic task
	}

	return s.db.CreateTask(projectID, title, taskType, priority)
}

// Get retrieves a task by ID
func (s *Service) Get(id string) (*db.Task, error) {
	task, err := s.db.GetTaskByID(id)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return task, nil
}

// List returns tasks with optional filters
func (s *Service) List(filters ListFilters) ([]*db.Task, error) {
	if filters.ProjectID != "" {
		return s.db.ListTasksByProject(filters.ProjectID)
	}
	if filters.Status != "" {
		return s.db.ListTasksByStatus(filters.Status)
	}
	// Return all tasks if no filter
	return s.db.ListAllTasks()
}

// UpdateStatus changes a task's status using the state machine for transition validation
func (s *Service) UpdateStatus(id, status string) error {
	return s.stateMachine.Transition(id, status)
}

// Delete removes a task
func (s *Service) Delete(id string) error {
	return s.db.DeleteTask(id)
}

// Update updates task fields
func (s *Service) Update(id string, updates TaskUpdates) (*db.Task, error) {
	// Verify task exists
	_, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	// Apply updates if provided
	if updates.Status != nil && *updates.Status != "" {
		if err := s.stateMachine.Transition(id, *updates.Status); err != nil {
			return nil, err
		}
	}
	if updates.Hat != nil && *updates.Hat != "" {
		if err := s.db.UpdateTaskHat(id, *updates.Hat); err != nil {
			return nil, err
		}
	}

	// Fetch and return updated task
	return s.Get(id)
}

// TaskUpdates holds optional fields for updating a task
type TaskUpdates struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	Hat         *string `json:"hat,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
}

// ListFilters defines optional filters for listing tasks
type ListFilters struct {
	ProjectID string
	Status    string
	Priority  int
}

// IsValidTaskType checks if the task type is valid
func IsValidTaskType(t string) bool {
	switch t {
	case db.TaskTypeEpic, db.TaskTypeFeature, db.TaskTypeBug, db.TaskTypeTask, db.TaskTypeChore:
		return true
	}
	return false
}

// IsValidStatus checks if the status is valid
func IsValidStatus(s string) bool {
	switch s {
	case db.TaskStatusPending, db.TaskStatusBlocked, db.TaskStatusReady,
		db.TaskStatusRunning, db.TaskStatusPaused, db.TaskStatusQuarantined,
		db.TaskStatusCompleted, db.TaskStatusCancelled:
		return true
	}
	return false
}
