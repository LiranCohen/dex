// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateTask inserts a new task into the database
func (db *DB) CreateTask(projectID, title string, taskType string, priority int) (*Task, error) {
	task := &Task{
		ID:            NewPrefixedID("task"),
		ProjectID:     projectID,
		Title:         title,
		Type:          taskType,
		Priority:      priority,
		AutonomyLevel: 1,
		Status:        TaskStatusPending,
		BaseBranch:    "main",
		CreatedAt:     time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO tasks (id, project_id, title, type, priority, autonomy_level, status, base_branch, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ProjectID, task.Title, task.Type, task.Priority,
		task.AutonomyLevel, task.Status, task.BaseBranch, task.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return task, nil
}

// CreateTaskForQuest creates a new task spawned by a Quest
// Tasks from Quests are created with status 'ready' (or 'blocked' if they have dependencies)
// model should be "sonnet" (default) or "opus" (complex tasks)
func (db *DB) CreateTaskForQuest(questID, projectID, title, description, hat, taskType, model string, priority int) (*Task, error) {
	// Default to sonnet if not specified
	if model == "" {
		model = TaskModelSonnet
	}

	task := &Task{
		ID:            NewPrefixedID("task"),
		ProjectID:     projectID,
		QuestID:       sql.NullString{String: questID, Valid: true},
		Title:         title,
		Description:   sql.NullString{String: description, Valid: description != ""},
		Hat:           sql.NullString{String: hat, Valid: hat != ""},
		Model:         sql.NullString{String: model, Valid: true},
		Type:          taskType,
		Priority:      priority,
		AutonomyLevel: 1,
		Status:        TaskStatusReady, // Tasks from Quests start as ready
		BaseBranch:    "main",
		CreatedAt:     time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO tasks (id, project_id, quest_id, title, description, hat, model, type, priority, autonomy_level, status, base_branch, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ProjectID, task.QuestID, task.Title, task.Description, task.Hat, task.Model,
		task.Type, task.Priority, task.AutonomyLevel, task.Status, task.BaseBranch, task.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task for quest: %w", err)
	}

	return task, nil
}

// GetTaskByID retrieves a task by its ID
// Note: Token counts are computed from session_activity, not stored in tasks table
func (db *DB) GetTaskByID(id string) (*Task, error) {
	task := &Task{}
	err := db.QueryRow(
		`SELECT id, project_id, quest_id, github_issue_number, title, description, parent_id,
		        type, hat, model, priority, autonomy_level, status, base_branch,
		        worktree_path, branch_name, content_path, pr_number,
		        token_budget, time_budget_min, time_used_min,
		        dollar_budget, dollar_used, created_at, started_at, completed_at
		 FROM tasks WHERE id = ?`,
		id,
	).Scan(
		&task.ID, &task.ProjectID, &task.QuestID, &task.GitHubIssueNumber, &task.Title, &task.Description, &task.ParentID,
		&task.Type, &task.Hat, &task.Model, &task.Priority, &task.AutonomyLevel, &task.Status, &task.BaseBranch,
		&task.WorktreePath, &task.BranchName, &task.ContentPath, &task.PRNumber,
		&task.TokenBudget, &task.TimeBudgetMin, &task.TimeUsedMin,
		&task.DollarBudget, &task.DollarUsed, &task.CreatedAt, &task.StartedAt, &task.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return task, nil
}

// ListTasksByProject returns all tasks for a project
func (db *DB) ListTasksByProject(projectID string) ([]*Task, error) {
	return db.listTasks(`WHERE project_id = ? ORDER BY priority ASC, created_at DESC`, projectID)
}

// ListTasksByStatus returns all tasks with a given status
func (db *DB) ListTasksByStatus(status string) ([]*Task, error) {
	return db.listTasks(`WHERE status = ? ORDER BY priority ASC, created_at DESC`, status)
}

// ListReadyTasks returns all tasks that are ready to run (not blocked)
func (db *DB) ListReadyTasks() ([]*Task, error) {
	return db.listTasks(`WHERE status = ? ORDER BY priority ASC, created_at DESC`, TaskStatusReady)
}

// ListAllTasks returns all tasks ordered by priority and creation time
func (db *DB) ListAllTasks() ([]*Task, error) {
	return db.listTasks(`ORDER BY priority ASC, created_at DESC`)
}

// listTasks is a helper for listing tasks with a WHERE clause
// Note: Token counts are computed from session_activity, not stored in tasks table
func (db *DB) listTasks(whereClause string, args ...any) ([]*Task, error) {
	query := `SELECT id, project_id, quest_id, github_issue_number, title, description, parent_id,
	                 type, hat, model, priority, autonomy_level, status, base_branch,
	                 worktree_path, branch_name, content_path, pr_number,
	                 token_budget, time_budget_min, time_used_min,
	                 dollar_budget, dollar_used, created_at, started_at, completed_at
	          FROM tasks ` + whereClause

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		task := &Task{}
		err := rows.Scan(
			&task.ID, &task.ProjectID, &task.QuestID, &task.GitHubIssueNumber, &task.Title, &task.Description, &task.ParentID,
			&task.Type, &task.Hat, &task.Model, &task.Priority, &task.AutonomyLevel, &task.Status, &task.BaseBranch,
			&task.WorktreePath, &task.BranchName, &task.ContentPath, &task.PRNumber,
			&task.TokenBudget, &task.TimeBudgetMin, &task.TimeUsedMin,
			&task.DollarBudget, &task.DollarUsed, &task.CreatedAt, &task.StartedAt, &task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tasks: %w", err)
	}

	return tasks, nil
}

// UpdateTaskStatus updates a task's status
func (db *DB) UpdateTaskStatus(id, status string) error {
	var startedAt, completedAt any
	now := time.Now()

	switch status {
	case TaskStatusRunning:
		startedAt = now
	case TaskStatusCompleted, TaskStatusCancelled:
		completedAt = now
	}

	result, err := db.Exec(
		`UPDATE tasks SET status = ?, started_at = COALESCE(?, started_at), completed_at = COALESCE(?, completed_at) WHERE id = ?`,
		status, startedAt, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskHat updates a task's current hat assignment
func (db *DB) UpdateTaskHat(id, hat string) error {
	result, err := db.Exec(`UPDATE tasks SET hat = ? WHERE id = ?`, hat, id)
	if err != nil {
		return fmt.Errorf("failed to update task hat: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskWorktree sets the worktree path and branch name for a task
func (db *DB) UpdateTaskWorktree(id, worktreePath, branchName string) error {
	result, err := db.Exec(
		`UPDATE tasks SET worktree_path = ?, branch_name = ? WHERE id = ?`,
		worktreePath, branchName, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update task worktree: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskPRNumber sets the PR number for a task
func (db *DB) UpdateTaskPRNumber(id string, prNumber int) error {
	result, err := db.Exec(`UPDATE tasks SET pr_number = ? WHERE id = ?`, prNumber, id)
	if err != nil {
		return fmt.Errorf("failed to update task PR number: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskGitHubIssue sets the GitHub Issue number for a task/objective
func (db *DB) UpdateTaskGitHubIssue(id string, issueNumber int64) error {
	result, err := db.Exec(`UPDATE tasks SET github_issue_number = ? WHERE id = ?`, issueNumber, id)
	if err != nil {
		return fmt.Errorf("failed to update task GitHub issue: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskContentPath sets the content path for a task's git content files
func (db *DB) UpdateTaskContentPath(id, contentPath string) error {
	var path any
	if contentPath != "" {
		path = contentPath
	}
	result, err := db.Exec(`UPDATE tasks SET content_path = ? WHERE id = ?`, path, id)
	if err != nil {
		return fmt.Errorf("failed to update task content path: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// StatusMismatchError indicates the task status didn't match expected (concurrent modification)
type StatusMismatchError struct {
	TaskID   string
	Expected string
	Actual   string
}

func (e *StatusMismatchError) Error() string {
	return fmt.Sprintf("task %s status mismatch: expected %q, got %q", e.TaskID, e.Expected, e.Actual)
}

// TransitionTaskStatus atomically transitions a task's status if the current status matches
// Returns StatusMismatchError if current status doesn't match expectedStatus
// Returns error if task doesn't exist
func (db *DB) TransitionTaskStatus(id, expectedStatus, newStatus string) error {
	now := time.Now()
	result, err := db.Exec(
		`UPDATE tasks SET status = ?,
		 started_at = CASE WHEN ? = 'running' AND started_at IS NULL THEN ? ELSE started_at END,
		 completed_at = CASE WHEN ? IN ('completed', 'cancelled') THEN ? ELSE completed_at END
		 WHERE id = ? AND status = ?`,
		newStatus, newStatus, now, newStatus, now, id, expectedStatus,
	)
	if err != nil {
		return fmt.Errorf("failed to transition task status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Check if task exists with different status or doesn't exist
		task, _ := db.GetTaskByID(id)
		if task == nil {
			return fmt.Errorf("task not found: %s", id)
		}
		return &StatusMismatchError{TaskID: id, Expected: expectedStatus, Actual: task.Status}
	}
	return nil
}

// DeleteTask removes a task from the database
func (db *DB) DeleteTask(id string) error {
	// First delete dependencies
	_, err := db.Exec(`DELETE FROM task_dependencies WHERE blocker_id = ? OR blocked_id = ?`, id, id)
	if err != nil {
		return fmt.Errorf("failed to delete task dependencies: %w", err)
	}

	result, err := db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// AddTaskDependency creates a dependency relationship between two tasks
func (db *DB) AddTaskDependency(blockerID, blockedID string) error {
	// Prevent self-reference
	if blockerID == blockedID {
		return fmt.Errorf("task cannot depend on itself: %s", blockerID)
	}

	_, err := db.Exec(
		`INSERT INTO task_dependencies (blocker_id, blocked_id, created_at) VALUES (?, ?, ?)`,
		blockerID, blockedID, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to add task dependency: %w", err)
	}

	return nil
}

// RemoveTaskDependency removes a dependency relationship between two tasks
func (db *DB) RemoveTaskDependency(blockerID, blockedID string) error {
	result, err := db.Exec(
		`DELETE FROM task_dependencies WHERE blocker_id = ? AND blocked_id = ?`,
		blockerID, blockedID,
	)
	if err != nil {
		return fmt.Errorf("failed to remove task dependency: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("dependency not found: %s -> %s", blockerID, blockedID)
	}

	return nil
}

// GetTaskBlockers returns all tasks that block the given task
func (db *DB) GetTaskBlockers(taskID string) ([]*Task, error) {
	return db.listTasks(
		`WHERE id IN (SELECT blocker_id FROM task_dependencies WHERE blocked_id = ?) ORDER BY priority ASC`,
		taskID,
	)
}

// GetTasksBlockedBy returns all tasks that are blocked by the given task
func (db *DB) GetTasksBlockedBy(taskID string) ([]*Task, error) {
	return db.listTasks(
		`WHERE id IN (SELECT blocked_id FROM task_dependencies WHERE blocker_id = ?) ORDER BY priority ASC`,
		taskID,
	)
}

// IsTaskReady returns true if a task has no incomplete blockers
func (db *DB) IsTaskReady(taskID string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM task_dependencies td
		 JOIN tasks t ON td.blocker_id = t.id
		 WHERE td.blocked_id = ? AND t.status != ?`,
		taskID, TaskStatusCompleted,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check task readiness: %w", err)
	}

	return count == 0, nil
}

// CreateTaskForQuestWithStatus creates a new task with a specific initial status and auto_start preference
// This is used for batch objective creation where some tasks may start as blocked
func (db *DB) CreateTaskForQuestWithStatus(questID, projectID, title, description, hat, taskType, model string, priority int, status string, autoStart bool) (*Task, error) {
	// Default to sonnet if not specified
	if model == "" {
		model = TaskModelSonnet
	}

	task := &Task{
		ID:            NewPrefixedID("task"),
		ProjectID:     projectID,
		QuestID:       sql.NullString{String: questID, Valid: true},
		Title:         title,
		Description:   sql.NullString{String: description, Valid: description != ""},
		Hat:           sql.NullString{String: hat, Valid: hat != ""},
		Model:         sql.NullString{String: model, Valid: true},
		Type:          taskType,
		Priority:      priority,
		AutonomyLevel: 1,
		Status:        status,
		BaseBranch:    "main",
		CreatedAt:     time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO tasks (id, project_id, quest_id, title, description, hat, model, type, priority, autonomy_level, status, base_branch, auto_start, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ProjectID, task.QuestID, task.Title, task.Description, task.Hat, task.Model,
		task.Type, task.Priority, task.AutonomyLevel, task.Status, task.BaseBranch, autoStart, task.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task for quest: %w", err)
	}

	return task, nil
}

// GetTasksUnblockedBy returns tasks that became ready because the given task completed
// These are tasks that:
// 1. Were blocked by the completed task
// 2. Have status 'blocked'
// 3. Now have all their blockers completed
// Note: Token counts are computed from session_activity, not stored in tasks table
func (db *DB) GetTasksUnblockedBy(completedTaskID string) ([]*Task, error) {
	// Find tasks that were blocked by the completed task and are now ready
	query := `
		SELECT DISTINCT t.id, t.project_id, t.quest_id, t.github_issue_number, t.title, t.description, t.parent_id,
		       t.type, t.hat, t.model, t.priority, t.autonomy_level, t.status, t.base_branch,
		       t.worktree_path, t.branch_name, t.content_path, t.pr_number,
		       t.token_budget, t.time_budget_min, t.time_used_min,
		       t.dollar_budget, t.dollar_used, t.created_at, t.started_at, t.completed_at
		FROM tasks t
		JOIN task_dependencies td ON t.id = td.blocked_id
		WHERE td.blocker_id = ?
		  AND t.status = ?
		  AND NOT EXISTS (
		      SELECT 1 FROM task_dependencies td2
		      JOIN tasks blocker ON td2.blocker_id = blocker.id
		      WHERE td2.blocked_id = t.id
		        AND blocker.status != ?
		  )
	`

	rows, err := db.Query(query, completedTaskID, TaskStatusBlocked, TaskStatusCompleted)
	if err != nil {
		return nil, fmt.Errorf("failed to get unblocked tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		task := &Task{}
		err := rows.Scan(
			&task.ID, &task.ProjectID, &task.QuestID, &task.GitHubIssueNumber, &task.Title, &task.Description, &task.ParentID,
			&task.Type, &task.Hat, &task.Model, &task.Priority, &task.AutonomyLevel, &task.Status, &task.BaseBranch,
			&task.WorktreePath, &task.BranchName, &task.ContentPath, &task.PRNumber,
			&task.TokenBudget, &task.TimeBudgetMin, &task.TimeUsedMin,
			&task.DollarBudget, &task.DollarUsed, &task.CreatedAt, &task.StartedAt, &task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan unblocked task: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating unblocked tasks: %w", err)
	}

	return tasks, nil
}

// GetTaskAutoStart returns whether a task should auto-start when unblocked
func (db *DB) GetTaskAutoStart(taskID string) (bool, error) {
	var autoStart bool
	err := db.QueryRow(`SELECT COALESCE(auto_start, FALSE) FROM tasks WHERE id = ?`, taskID).Scan(&autoStart)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return false, fmt.Errorf("failed to get task auto_start: %w", err)
	}
	return autoStart, nil
}

// GetIncompleteBlockerIDs returns the IDs of tasks that block the given task and are not completed
// This is used for deriving the blocked status at query time
func (db *DB) GetIncompleteBlockerIDs(taskID string) ([]string, error) {
	rows, err := db.Query(
		`SELECT t.id FROM tasks t
		 JOIN task_dependencies td ON t.id = td.blocker_id
		 WHERE td.blocked_id = ? AND t.status NOT IN (?, ?)`,
		taskID, TaskStatusCompleted, TaskStatusCancelled,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get incomplete blockers: %w", err)
	}
	defer rows.Close()

	var blockerIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan blocker id: %w", err)
		}
		blockerIDs = append(blockerIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blocker ids: %w", err)
	}

	return blockerIDs, nil
}

// GetTasksReadyToAutoStart returns tasks that became ready because the given task completed
// These are tasks that:
// 1. Were blocked by the completed task
// 2. Have auto_start = true
// 3. Now have all their blockers completed (no incomplete blockers remaining)
// 4. Are in 'ready' status (not already running/completed/etc.)
// Note: Token counts are computed from session_activity, not stored in tasks table
func (db *DB) GetTasksReadyToAutoStart(completedTaskID string) ([]*Task, error) {
	query := `
		SELECT DISTINCT t.id, t.project_id, t.quest_id, t.github_issue_number, t.title, t.description, t.parent_id,
		       t.type, t.hat, t.model, t.priority, t.autonomy_level, t.status, t.base_branch,
		       t.worktree_path, t.branch_name, t.content_path, t.pr_number,
		       t.token_budget, t.time_budget_min, t.time_used_min,
		       t.dollar_budget, t.dollar_used, t.created_at, t.started_at, t.completed_at
		FROM tasks t
		JOIN task_dependencies td ON t.id = td.blocked_id
		WHERE td.blocker_id = ?
		  AND t.status = ?
		  AND COALESCE(t.auto_start, FALSE) = TRUE
		  AND NOT EXISTS (
		      SELECT 1 FROM task_dependencies td2
		      JOIN tasks blocker ON td2.blocker_id = blocker.id
		      WHERE td2.blocked_id = t.id
		        AND blocker.status NOT IN (?, ?)
		  )
	`

	rows, err := db.Query(query, completedTaskID, TaskStatusReady, TaskStatusCompleted, TaskStatusCancelled)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks ready to auto-start: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		task := &Task{}
		err := rows.Scan(
			&task.ID, &task.ProjectID, &task.QuestID, &task.GitHubIssueNumber, &task.Title, &task.Description, &task.ParentID,
			&task.Type, &task.Hat, &task.Model, &task.Priority, &task.AutonomyLevel, &task.Status, &task.BaseBranch,
			&task.WorktreePath, &task.BranchName, &task.ContentPath, &task.PRNumber,
			&task.TokenBudget, &task.TimeBudgetMin, &task.TimeUsedMin,
			&task.DollarBudget, &task.DollarUsed, &task.CreatedAt, &task.StartedAt, &task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tasks: %w", err)
	}

	return tasks, nil
}
