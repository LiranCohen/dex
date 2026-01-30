// Package task provides task management services for Poindexter
package task

import (
	"fmt"

	"github.com/liranmauda/dex/internal/db"
)

// Graph manages task dependency relationships
type Graph struct {
	db *db.DB
}

// NewGraph creates a new dependency graph manager
func NewGraph(database *db.DB) *Graph {
	return &Graph{db: database}
}

// AddDependency creates a dependency: blocked task waits for blocker to complete.
// Returns an error if either task doesn't exist or if this would create a cycle.
func (g *Graph) AddDependency(blockerID, blockedID string) error {
	// Verify blocker task exists
	blocker, err := g.db.GetTaskByID(blockerID)
	if err != nil {
		return fmt.Errorf("failed to verify blocker task: %w", err)
	}
	if blocker == nil {
		return fmt.Errorf("blocker task not found: %s", blockerID)
	}

	// Verify blocked task exists
	blocked, err := g.db.GetTaskByID(blockedID)
	if err != nil {
		return fmt.Errorf("failed to verify blocked task: %w", err)
	}
	if blocked == nil {
		return fmt.Errorf("blocked task not found: %s", blockedID)
	}

	// Check for cycle: would blockerID become reachable from blockedID?
	if wouldCreateCycle, err := g.wouldCreateCycle(blockerID, blockedID); err != nil {
		return fmt.Errorf("failed to check for cycle: %w", err)
	} else if wouldCreateCycle {
		return fmt.Errorf("adding dependency would create cycle: %s -> %s", blockerID, blockedID)
	}

	return g.db.AddTaskDependency(blockerID, blockedID)
}

// wouldCreateCycle checks if adding blockerID -> blockedID would create a cycle.
// A cycle exists if blockerID is reachable from blockedID (i.e., blockedID already depends on blockerID).
func (g *Graph) wouldCreateCycle(blockerID, blockedID string) (bool, error) {
	// BFS from blockedID's blocked tasks to see if we can reach blockerID
	visited := make(map[string]bool)
	queue := []string{blockedID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		// Get tasks that are blocked by current (i.e., current is a blocker)
		blockedTasks, err := g.db.GetTasksBlockedBy(current)
		if err != nil {
			return false, err
		}

		for _, task := range blockedTasks {
			if task.ID == blockerID {
				return true, nil
			}
			if !visited[task.ID] {
				queue = append(queue, task.ID)
			}
		}
	}

	return false, nil
}

// RemoveDependency removes a dependency relationship
func (g *Graph) RemoveDependency(blockerID, blockedID string) error {
	return g.db.RemoveTaskDependency(blockerID, blockedID)
}

// GetBlockers returns all tasks that must complete before this task can start
func (g *Graph) GetBlockers(taskID string) ([]*db.Task, error) {
	return g.db.GetTaskBlockers(taskID)
}

// GetBlocked returns all tasks waiting for this task to complete
func (g *Graph) GetBlocked(taskID string) ([]*db.Task, error) {
	return g.db.GetTasksBlockedBy(taskID)
}

// IsReady returns true if all blockers for this task are completed
func (g *Graph) IsReady(taskID string) (bool, error) {
	return g.db.IsTaskReady(taskID)
}

// GetReadyTasks returns all pending/ready tasks with no incomplete blockers.
// These are tasks that can be scheduled for execution.
func (g *Graph) GetReadyTasks() ([]*db.Task, error) {
	// Get all pending and ready tasks
	pendingTasks, err := g.db.ListTasksByStatus(db.TaskStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending tasks: %w", err)
	}

	readyTasks, err := g.db.ListTasksByStatus(db.TaskStatusReady)
	if err != nil {
		return nil, fmt.Errorf("failed to list ready tasks: %w", err)
	}

	// Combine and filter by dependency readiness
	candidates := append(pendingTasks, readyTasks...)
	var result []*db.Task

	for _, task := range candidates {
		ready, err := g.db.IsTaskReady(task.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check task readiness for %s: %w", task.ID, err)
		}
		if ready {
			result = append(result, task)
		}
	}

	return result, nil
}

// GetDependencyCount returns the number of blockers and blocked tasks for a task
func (g *Graph) GetDependencyCount(taskID string) (blockers int, blocked int, err error) {
	blockerTasks, err := g.GetBlockers(taskID)
	if err != nil {
		return 0, 0, err
	}

	blockedTasks, err := g.GetBlocked(taskID)
	if err != nil {
		return 0, 0, err
	}

	return len(blockerTasks), len(blockedTasks), nil
}
