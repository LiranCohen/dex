// Package task provides task management services for Poindexter
package task

import (
	"fmt"
	"slices"
	"time"

	"github.com/lirancohen/dex/internal/db"
)

// validTransitions defines allowed status transitions
// Key is the source status, value is a slice of valid target statuses
var validTransitions = map[string][]string{
	db.TaskStatusPending:     {db.TaskStatusPlanning, db.TaskStatusReady, db.TaskStatusBlocked, db.TaskStatusCancelled},
	db.TaskStatusPlanning:    {db.TaskStatusReady, db.TaskStatusCancelled},
	db.TaskStatusBlocked:     {db.TaskStatusReady, db.TaskStatusCancelled},
	db.TaskStatusReady:       {db.TaskStatusRunning, db.TaskStatusBlocked, db.TaskStatusCancelled},
	db.TaskStatusRunning:     {db.TaskStatusPaused, db.TaskStatusCompleted, db.TaskStatusQuarantined, db.TaskStatusCancelled},
	db.TaskStatusPaused:      {db.TaskStatusRunning, db.TaskStatusCancelled},
	db.TaskStatusQuarantined: {db.TaskStatusRunning, db.TaskStatusCancelled},
	db.TaskStatusCompleted:   {}, // Terminal state
	db.TaskStatusCancelled:   {}, // Terminal state
}

// TransitionEvent represents a status change for notification
type TransitionEvent struct {
	TaskID    string
	From      string
	To        string
	Timestamp time.Time
}

// StateMachine manages task status transitions with validation and event emission
type StateMachine struct {
	db        *db.DB
	eventChan chan TransitionEvent // For future WebSocket integration
}

// NewStateMachine creates a new state machine instance
func NewStateMachine(database *db.DB) *StateMachine {
	return &StateMachine{
		db:        database,
		eventChan: nil, // Will be set when WebSocket integration is added
	}
}

// canTransition checks if a transition from the current status to the target status is valid
func (sm *StateMachine) canTransition(from, to string) bool {
	validTargets, exists := validTransitions[from]
	if !exists {
		return false
	}
	return slices.Contains(validTargets, to)
}

// Transition validates and executes a status change for a task
// Returns an error if the task doesn't exist, the transition is invalid, or the DB update fails
func (sm *StateMachine) Transition(taskID, targetStatus string) error {
	// Validate target status is a known status
	if !IsValidStatus(targetStatus) {
		return fmt.Errorf("invalid target status: %s", targetStatus)
	}

	// Get current task to check current status
	task, err := sm.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	currentStatus := task.Status

	// Validate transition is allowed
	if !sm.canTransition(currentStatus, targetStatus) {
		return &InvalidTransitionError{
			TaskID:       taskID,
			FromStatus:   currentStatus,
			TargetStatus: targetStatus,
		}
	}

	// Additional validation for completion: check checklist status
	if targetStatus == db.TaskStatusCompleted {
		if canComplete, reason := sm.CanComplete(taskID); !canComplete {
			return &ChecklistIncompleteError{
				TaskID: taskID,
				Reason: reason,
			}
		}
	}

	// Execute the transition atomically in the database
	if err := sm.db.TransitionTaskStatus(taskID, currentStatus, targetStatus); err != nil {
		// Check for concurrent modification
		if _, ok := err.(*db.StatusMismatchError); ok {
			return fmt.Errorf("concurrent status change detected, retry transition: %w", err)
		}
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// Emit event if channel is configured
	if sm.eventChan != nil {
		event := TransitionEvent{
			TaskID:    taskID,
			From:      currentStatus,
			To:        targetStatus,
			Timestamp: time.Now(),
		}
		// Non-blocking send to avoid deadlock
		select {
		case sm.eventChan <- event:
		default:
			// Channel full or closed, log would go here in production
		}
	}

	return nil
}

// CanComplete checks if a task can be marked as completed
// Returns true if the task can complete, false with reason if not
func (sm *StateMachine) CanComplete(taskID string) (bool, string) {
	checklist, err := sm.db.GetChecklistByTaskID(taskID)
	if err != nil || checklist == nil {
		// No checklist, can complete
		return true, ""
	}

	items, err := sm.db.GetChecklistItems(checklist.ID)
	if err != nil {
		// Error getting items, allow completion (don't block on errors)
		return true, ""
	}

	// Check for pending or in_progress items
	for _, item := range items {
		if item.Status == db.ChecklistItemStatusPending || item.Status == db.ChecklistItemStatusInProgress {
			return false, fmt.Sprintf("checklist item %s is still %s: %s", item.ID, item.Status, item.Description)
		}
	}

	return true, ""
}

// ChecklistIncompleteError is returned when completing a task with incomplete checklist
type ChecklistIncompleteError struct {
	TaskID string
	Reason string
}

func (e *ChecklistIncompleteError) Error() string {
	return fmt.Sprintf("cannot complete task %s: %s", e.TaskID, e.Reason)
}

// IsChecklistIncomplete checks if an error is a ChecklistIncompleteError
func IsChecklistIncomplete(err error) bool {
	_, ok := err.(*ChecklistIncompleteError)
	return ok
}

// InvalidTransitionError is returned when an invalid status transition is attempted
type InvalidTransitionError struct {
	TaskID       string
	FromStatus   string
	TargetStatus string
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("invalid transition for task %s: cannot transition from %q to %q",
		e.TaskID, e.FromStatus, e.TargetStatus)
}

// IsInvalidTransition checks if an error is an InvalidTransitionError
func IsInvalidTransition(err error) bool {
	_, ok := err.(*InvalidTransitionError)
	return ok
}
