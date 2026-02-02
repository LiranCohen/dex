// Package task provides task management services for Poindexter
package task

import (
	"fmt"

	"github.com/lirancohen/dex/internal/content"
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
	case db.TaskStatusPending, db.TaskStatusPlanning, db.TaskStatusBlocked, db.TaskStatusReady,
		db.TaskStatusRunning, db.TaskStatusPaused, db.TaskStatusQuarantined,
		db.TaskStatusCompleted, db.TaskStatusCancelled:
		return true
	}
	return false
}

// CreateTaskContentOptions holds options for creating task content files
type CreateTaskContentOptions struct {
	Title       string
	Description string
	ProjectName string
	QuestID     string
	Checklist   []content.ChecklistItem
}

// GitOperations interface for git operations needed by task service
type GitOperations interface {
	CommitTaskContent(dir, taskID, message string) (string, error)
}

// WriteTaskContent writes task content files to the given base path
// and updates the task's content_path in the database
func (s *Service) WriteTaskContent(taskID, basePath string, opts CreateTaskContentOptions) error {
	// Create content manager for this path
	mgr := content.NewManager(basePath)

	// Write the content files
	if err := mgr.InitTaskContent(
		taskID,
		opts.Title,
		opts.Description,
		opts.ProjectName,
		opts.QuestID,
		opts.Checklist,
	); err != nil {
		return fmt.Errorf("failed to write task content: %w", err)
	}

	// Update the task's content_path in the database
	contentPath := content.TaskContentPath(taskID)
	if err := s.db.UpdateTaskContentPath(taskID, contentPath); err != nil {
		return fmt.Errorf("failed to update task content path: %w", err)
	}

	return nil
}

// WriteTaskContentAndCommit writes task content files and commits them to git
func (s *Service) WriteTaskContentAndCommit(taskID, basePath string, opts CreateTaskContentOptions, git GitOperations) (string, error) {
	// Write the content files first
	if err := s.WriteTaskContent(taskID, basePath, opts); err != nil {
		return "", err
	}

	// Commit the content
	commitHash, err := git.CommitTaskContent(basePath, taskID, "")
	if err != nil {
		return "", fmt.Errorf("failed to commit task content: %w", err)
	}

	return commitHash, nil
}

// ReadTaskSpec reads the task specification from content files
func (s *Service) ReadTaskSpec(taskID, basePath string) (string, error) {
	mgr := content.NewManager(basePath)
	return mgr.ReadTaskSpec(taskID)
}

// ReadTaskChecklist reads the task checklist from content files
func (s *Service) ReadTaskChecklist(taskID, basePath string) ([]content.ChecklistItem, error) {
	mgr := content.NewManager(basePath)
	return mgr.ReadTaskChecklist(taskID)
}

// UpdateTaskChecklistItem updates a checklist item's status in the content files
func (s *Service) UpdateTaskChecklistItem(taskID, basePath, description string, status content.ChecklistStatus) error {
	mgr := content.NewManager(basePath)
	return mgr.UpdateTaskChecklistItem(taskID, description, status)
}

// GetTaskContentPath returns the full path to task content for a given base path
func (s *Service) GetTaskContentPath(taskID, basePath string) string {
	mgr := content.NewManager(basePath)
	return mgr.GetTaskContentPath(taskID)
}

// WriteTaskContentFromPlanning creates task content files from planning session data
// This is called when a planning session completes and the task is ready to be written to git
func (s *Service) WriteTaskContentFromPlanning(taskID, basePath string, opts CreateTaskContentOptions, mustHave, optional []string) error {
	// Convert planning checklist items to content checklist items
	if len(mustHave) > 0 || len(optional) > 0 {
		opts.Checklist = content.ChecklistFromPlanningItems(mustHave, optional)
	}

	return s.WriteTaskContent(taskID, basePath, opts)
}

// WriteTaskContentFromPlanningAndCommit creates task content from planning data and commits to git
func (s *Service) WriteTaskContentFromPlanningAndCommit(taskID, basePath string, opts CreateTaskContentOptions, mustHave, optional []string, git GitOperations) (string, error) {
	// Convert planning checklist items to content checklist items
	if len(mustHave) > 0 || len(optional) > 0 {
		opts.Checklist = content.ChecklistFromPlanningItems(mustHave, optional)
	}

	return s.WriteTaskContentAndCommit(taskID, basePath, opts, git)
}
