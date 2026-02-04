// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CreateQuest creates a new Quest
func (db *DB) CreateQuest(projectID string, model string) (*Quest, error) {
	quest := &Quest{
		ID:               NewPrefixedID("quest"),
		ProjectID:        projectID,
		Status:           QuestStatusActive,
		Model:            model,
		AutoStartDefault: true,
		CreatedAt:        time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO quests (id, project_id, status, model, auto_start_default, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		quest.ID, quest.ProjectID, quest.Status, quest.Model, quest.AutoStartDefault, quest.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create quest: %w", err)
	}

	return quest, nil
}

// GetQuestByID retrieves a Quest by its ID
func (db *DB) GetQuestByID(id string) (*Quest, error) {
	quest := &Quest{}

	err := db.QueryRow(
		`SELECT id, project_id, title, status, model, auto_start_default, conversation_path,
		        github_issue_number, created_at, completed_at
		 FROM quests WHERE id = ?`,
		id,
	).Scan(
		&quest.ID, &quest.ProjectID, &quest.Title, &quest.Status,
		&quest.Model, &quest.AutoStartDefault, &quest.ConversationPath,
		&quest.GitHubIssueNumber, &quest.CreatedAt, &quest.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get quest: %w", err)
	}

	return quest, nil
}

// GetQuestsByProjectID retrieves all Quests for a project
func (db *DB) GetQuestsByProjectID(projectID string) ([]*Quest, error) {
	rows, err := db.Query(
		`SELECT id, project_id, title, status, model, auto_start_default, conversation_path,
		        github_issue_number, created_at, completed_at
		 FROM quests WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get quests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var quests []*Quest
	for rows.Next() {
		quest := &Quest{}
		err := rows.Scan(
			&quest.ID, &quest.ProjectID, &quest.Title, &quest.Status,
			&quest.Model, &quest.AutoStartDefault, &quest.ConversationPath,
			&quest.GitHubIssueNumber, &quest.CreatedAt, &quest.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quest: %w", err)
		}
		quests = append(quests, quest)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating quests: %w", err)
	}

	return quests, nil
}

// GetActiveQuests retrieves all active Quests for a project
func (db *DB) GetActiveQuests(projectID string) ([]*Quest, error) {
	rows, err := db.Query(
		`SELECT id, project_id, title, status, model, auto_start_default, conversation_path,
		        github_issue_number, created_at, completed_at
		 FROM quests WHERE project_id = ? AND status = ? ORDER BY created_at DESC`,
		projectID, QuestStatusActive,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get active quests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var quests []*Quest
	for rows.Next() {
		quest := &Quest{}
		err := rows.Scan(
			&quest.ID, &quest.ProjectID, &quest.Title, &quest.Status,
			&quest.Model, &quest.AutoStartDefault, &quest.ConversationPath,
			&quest.GitHubIssueNumber, &quest.CreatedAt, &quest.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quest: %w", err)
		}
		quests = append(quests, quest)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating quests: %w", err)
	}

	return quests, nil
}

// UpdateQuestTitle updates the title of a Quest
func (db *DB) UpdateQuestTitle(id, title string) error {
	result, err := db.Exec(
		`UPDATE quests SET title = ? WHERE id = ?`,
		title, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quest title: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// UpdateQuestConversationPath updates the conversation_path of a Quest
func (db *DB) UpdateQuestConversationPath(id, conversationPath string) error {
	result, err := db.Exec(
		`UPDATE quests SET conversation_path = ? WHERE id = ?`,
		conversationPath, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quest conversation path: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// UpdateQuestGitHubIssue updates the GitHub Issue number for a Quest
func (db *DB) UpdateQuestGitHubIssue(id string, issueNumber int64) error {
	result, err := db.Exec(
		`UPDATE quests SET github_issue_number = ? WHERE id = ?`,
		issueNumber, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quest GitHub issue: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// UpdateQuestModel updates the model of a Quest (sonnet or opus)
func (db *DB) UpdateQuestModel(id, model string) error {
	if model != QuestModelSonnet && model != QuestModelOpus {
		return fmt.Errorf("invalid model: %s", model)
	}

	result, err := db.Exec(
		`UPDATE quests SET model = ? WHERE id = ?`,
		model, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quest model: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// UpdateQuestStatus updates the status of a Quest
func (db *DB) UpdateQuestStatus(id, status string) error {
	var completedAt sql.NullTime
	if status == QuestStatusCompleted {
		completedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	result, err := db.Exec(
		`UPDATE quests SET status = ?, completed_at = ? WHERE id = ?`,
		status, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quest status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// CompleteQuest marks a Quest as completed
func (db *DB) CompleteQuest(id string) error {
	return db.UpdateQuestStatus(id, QuestStatusCompleted)
}

// ReopenQuest marks a completed Quest as active again
func (db *DB) ReopenQuest(id string) error {
	result, err := db.Exec(
		`UPDATE quests SET status = ?, completed_at = NULL WHERE id = ?`,
		QuestStatusActive, id,
	)
	if err != nil {
		return fmt.Errorf("failed to reopen quest: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// DeleteQuest removes a Quest and its messages (cascade)
func (db *DB) DeleteQuest(id string) error {
	result, err := db.Exec(`DELETE FROM quests WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete quest: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("quest not found: %s", id)
	}

	return nil
}

// CreateQuestMessage creates a new message in a Quest conversation
func (db *DB) CreateQuestMessage(questID, role, content string) (*QuestMessage, error) {
	return db.CreateQuestMessageWithToolCalls(questID, role, content, nil)
}

// CreateQuestMessageWithToolCalls creates a new message with optional tool calls
func (db *DB) CreateQuestMessageWithToolCalls(questID, role, content string, toolCalls []QuestToolCall) (*QuestMessage, error) {
	msg := &QuestMessage{
		ID:        NewPrefixedID("qmsg"),
		QuestID:   questID,
		Role:      role,
		Content:   content,
		ToolCalls: toolCalls,
		CreatedAt: time.Now(),
	}

	var toolCallsJSON sql.NullString
	if len(toolCalls) > 0 {
		data, err := json.Marshal(toolCalls)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool calls: %w", err)
		}
		toolCallsJSON = sql.NullString{String: string(data), Valid: true}
	}

	_, err := db.Exec(
		`INSERT INTO quest_messages (id, quest_id, role, content, tool_calls, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.QuestID, msg.Role, msg.Content, toolCallsJSON, msg.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create quest message: %w", err)
	}

	return msg, nil
}

// GetQuestMessages retrieves all messages for a Quest in chronological order
func (db *DB) GetQuestMessages(questID string) ([]*QuestMessage, error) {
	rows, err := db.Query(
		`SELECT id, quest_id, role, content, tool_calls, created_at
		 FROM quest_messages WHERE quest_id = ?
		 ORDER BY created_at ASC`,
		questID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get quest messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []*QuestMessage
	for rows.Next() {
		msg := &QuestMessage{}
		var toolCallsJSON sql.NullString
		err := rows.Scan(&msg.ID, &msg.QuestID, &msg.Role, &msg.Content, &toolCallsJSON, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quest message: %w", err)
		}

		// Parse tool_calls JSON if present
		if toolCallsJSON.Valid && toolCallsJSON.String != "" {
			if err := json.Unmarshal([]byte(toolCallsJSON.String), &msg.ToolCalls); err != nil {
				// Log but don't fail - could be legacy data
				fmt.Printf("warning: failed to parse tool_calls for message %s: %v\n", msg.ID, err)
			}
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating quest messages: %w", err)
	}

	return messages, nil
}

// DeleteQuestMessages removes all messages for a Quest
func (db *DB) DeleteQuestMessages(questID string) error {
	_, err := db.Exec(`DELETE FROM quest_messages WHERE quest_id = ?`, questID)
	if err != nil {
		return fmt.Errorf("failed to delete quest messages: %w", err)
	}
	return nil
}

// GetTasksByQuestID retrieves all tasks spawned by a Quest
// Note: Token counts are computed from session_activity, not stored in tasks table
func (db *DB) GetTasksByQuestID(questID string) ([]*Task, error) {
	rows, err := db.Query(
		`SELECT id, project_id, quest_id, github_issue_number, title, description, parent_id, type, hat, model,
		        priority, autonomy_level, status, base_branch, worktree_path, branch_name, content_path, pr_number,
		        pr_merged_at, worktree_cleaned_at, token_budget, time_budget_min, time_used_min, dollar_budget, dollar_used,
		        created_at, started_at, completed_at
		 FROM tasks WHERE quest_id = ? ORDER BY created_at ASC`,
		questID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks by quest: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []*Task
	for rows.Next() {
		task := &Task{}
		err := rows.Scan(
			&task.ID, &task.ProjectID, &task.QuestID, &task.GitHubIssueNumber, &task.Title, &task.Description,
			&task.ParentID, &task.Type, &task.Hat, &task.Model, &task.Priority, &task.AutonomyLevel, &task.Status,
			&task.BaseBranch, &task.WorktreePath, &task.BranchName, &task.ContentPath, &task.PRNumber,
			&task.PRMergedAt, &task.WorktreeCleanedAt, &task.TokenBudget, &task.TimeBudgetMin, &task.TimeUsedMin,
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

// QuestSummary provides aggregate information about a Quest's tasks
type QuestSummary struct {
	TotalTasks       int
	CompletedTasks   int
	RunningTasks     int
	FailedTasks      int
	BlockedTasks     int
	PendingTasks     int
	TotalDollarsUsed float64
}

// GetQuestSummary calculates task statistics for a Quest (derived from tasks and sessions)
// BlockedTasks count is derived from dependencies, not from stored status
func (db *DB) GetQuestSummary(questID string) (*QuestSummary, error) {
	tasks, err := db.GetTasksByQuestID(questID)
	if err != nil {
		return nil, err
	}

	summary := &QuestSummary{}
	for _, task := range tasks {
		summary.TotalTasks++

		// Check if task is blocked by incomplete dependencies (derived, not stored)
		blockerIDs, _ := db.GetIncompleteBlockerIDs(task.ID)
		isBlocked := len(blockerIDs) > 0

		switch task.Status {
		case TaskStatusCompleted, TaskStatusCompletedWithIssues:
			summary.CompletedTasks++
		case TaskStatusRunning:
			summary.RunningTasks++
		case TaskStatusQuarantined:
			summary.FailedTasks++
		default:
			// For ready/pending tasks, check if they're blocked by dependencies
			if isBlocked {
				summary.BlockedTasks++
			} else {
				summary.PendingTasks++
			}
		}
	}

	// Aggregate cost from session_activity (single source of truth for tokens)
	// Tokens are summed from activity, then multiplied by rates stored in sessions
	err = db.QueryRow(
		`SELECT COALESCE(SUM(session_tokens.input_sum * s.input_rate + session_tokens.output_sum * s.output_rate) / 1000000.0, 0)
		 FROM sessions s
		 JOIN tasks t ON s.task_id = t.id
		 LEFT JOIN (
		     SELECT session_id,
		            COALESCE(SUM(tokens_input), 0) as input_sum,
		            COALESCE(SUM(tokens_output), 0) as output_sum
		     FROM session_activity
		     GROUP BY session_id
		 ) session_tokens ON session_tokens.session_id = s.id
		 WHERE t.quest_id = ?`,
		questID,
	).Scan(&summary.TotalDollarsUsed)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate quest cost: %w", err)
	}

	return summary, nil
}
