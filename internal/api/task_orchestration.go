package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
)

// startTaskResult contains the result of starting a task
type startTaskResult struct {
	Task         *db.Task
	WorktreePath string
	SessionID    string
}

// startTaskInternal starts a task by ID with an optional base branch
// This is a shared helper used by handleStartTask and auto-start logic
func (s *Server) startTaskInternal(ctx context.Context, taskID string, baseBranch string) (*startTaskResult, error) {
	// Get the task first
	t, err := s.taskService.Get(taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}

	// Get the project to find repo_path
	project, err := s.db.GetProjectByID(t.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found")
	}

	projectPath := project.RepoPath
	if baseBranch == "" {
		baseBranch = project.DefaultBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
	}

	// Check if already has a worktree
	if t.WorktreePath.Valid && t.WorktreePath.String != "" {
		return nil, fmt.Errorf("task already has a worktree")
	}

	// Check if project has a valid git repository that's appropriate for this project
	// For new projects (creating repos), we start without a worktree
	var worktreePath string
	hasGitRepo := s.isValidGitRepo(projectPath)
	isValidProjectPath := s.isValidProjectPath(projectPath)

	if hasGitRepo && isValidProjectPath && s.gitService != nil {
		// Project has a valid git repo - create worktree as usual
		worktreePath, err = s.gitService.SetupTaskWorktree(projectPath, taskID, baseBranch)
		if err != nil {
			return nil, fmt.Errorf("failed to create worktree: %w", err)
		}
	} else {
		// No valid git repo yet - create a task-specific directory
		// This happens for new projects or when project path is invalid/system path
		if isValidProjectPath && projectPath != "" {
			// Use the project path - the objective will create the repo here
			worktreePath = projectPath
			if err := os.MkdirAll(worktreePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create project directory: %w", err)
			}
		} else {
			// Project path is empty or invalid (e.g., system directory)
			// Create a task-specific directory in the worktrees directory
			if s.baseDir != "" {
				worktreePath = filepath.Join(s.baseDir, "worktrees", "task-"+taskID)
			} else {
				worktreePath = filepath.Join(os.TempDir(), "dex-task-"+taskID)
			}
			if err := os.MkdirAll(worktreePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create task directory: %w", err)
			}
		}
	}

	// Transition through ready to running status
	// First: pending -> ready
	if t.Status == "pending" {
		if err := s.taskService.UpdateStatus(taskID, "ready"); err != nil {
			if hasGitRepo && s.gitService != nil {
				_ = s.gitService.CleanupTaskWorktree(projectPath, taskID, true)
			}
			return nil, fmt.Errorf("failed to transition to ready: %w", err)
		}
	}
	// Then: ready -> running
	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		if hasGitRepo && s.gitService != nil {
			_ = s.gitService.CleanupTaskWorktree(projectPath, taskID, true)
		}
		return nil, fmt.Errorf("failed to transition to running: %w", err)
	}

	// Create and start a session for this task
	hat := "creator" // Default hat - could be determined from task type
	if t.Hat.Valid && t.Hat.String != "" {
		hat = t.Hat.String
	}

	sess, err := s.sessionManager.CreateSession(taskID, hat, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Start the session (runs Ralph loop in background)
	// Use background context since the session should live beyond the HTTP request
	if err := s.sessionManager.Start(ctx, sess.ID); err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	// Fetch updated task
	updated, _ := s.taskService.Get(taskID)

	return &startTaskResult{
		Task:         updated,
		WorktreePath: worktreePath,
		SessionID:    sess.ID,
	}, nil
}

// startTaskWithInheritance starts a task, optionally inheriting a worktree from a predecessor
func (s *Server) startTaskWithInheritance(ctx context.Context, taskID string, inheritedWorktree string, predecessorHandoff string) (*startTaskResult, error) {
	// Get the task first
	t, err := s.taskService.Get(taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}

	// Get the project
	project, err := s.db.GetProjectByID(t.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found")
	}

	// Check if already has a worktree
	if t.WorktreePath.Valid && t.WorktreePath.String != "" {
		return nil, fmt.Errorf("task already has a worktree")
	}

	// Use inherited worktree if provided and valid, otherwise fall back to normal logic
	var worktreePath string
	if inheritedWorktree != "" {
		// Verify the inherited worktree still exists
		if _, err := os.Stat(inheritedWorktree); err == nil {
			worktreePath = inheritedWorktree
			fmt.Printf("startTaskWithInheritance: task %s inheriting worktree %s\n", taskID, worktreePath)
		} else {
			fmt.Printf("startTaskWithInheritance: inherited worktree %s no longer exists, creating new\n", inheritedWorktree)
		}
	}

	// If no inherited worktree, fall back to normal worktree creation
	if worktreePath == "" {
		projectPath := project.RepoPath
		baseBranch := project.DefaultBranch
		if baseBranch == "" {
			baseBranch = "main"
		}

		hasGitRepo := s.isValidGitRepo(projectPath)
		isValidProjectPath := s.isValidProjectPath(projectPath)

		if hasGitRepo && isValidProjectPath && s.gitService != nil {
			worktreePath, err = s.gitService.SetupTaskWorktree(projectPath, taskID, baseBranch)
			if err != nil {
				return nil, fmt.Errorf("failed to create worktree: %w", err)
			}
		} else if isValidProjectPath && projectPath != "" {
			worktreePath = projectPath
			if err := os.MkdirAll(worktreePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create project directory: %w", err)
			}
		} else {
			if s.baseDir != "" {
				worktreePath = filepath.Join(s.baseDir, "worktrees", "task-"+taskID)
			} else {
				worktreePath = filepath.Join(os.TempDir(), "dex-task-"+taskID)
			}
			if err := os.MkdirAll(worktreePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create task directory: %w", err)
			}
		}
	}

	// Transition through ready to running status
	if t.Status == "pending" || t.Status == "blocked" {
		if err := s.taskService.UpdateStatus(taskID, "ready"); err != nil {
			return nil, fmt.Errorf("failed to transition to ready: %w", err)
		}
	}
	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		return nil, fmt.Errorf("failed to transition to running: %w", err)
	}

	// Create and start a session for this task
	hat := "creator"
	if t.Hat.Valid && t.Hat.String != "" {
		hat = t.Hat.String
	}

	sess, err := s.sessionManager.CreateSession(taskID, hat, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// If we have a predecessor handoff, inject it into the session context
	if predecessorHandoff != "" {
		s.sessionManager.SetPredecessorContext(sess.ID, predecessorHandoff)
	}

	// Start the session
	if err := s.sessionManager.Start(ctx, sess.ID); err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	// Fetch updated task
	updated, _ := s.taskService.Get(taskID)

	return &startTaskResult{
		Task:         updated,
		WorktreePath: worktreePath,
		SessionID:    sess.ID,
	}, nil
}

// handleTaskUnblocking finds tasks that became ready because the given task completed
// and transitions them from blocked to ready (auto-starting if configured)
func (s *Server) handleTaskUnblocking(ctx context.Context, completedTaskID string) {
	// Get the completed task to capture its worktree and context
	completedTask, err := s.db.GetTaskByID(completedTaskID)
	if err != nil || completedTask == nil {
		fmt.Printf("handleTaskUnblocking: failed to get completed task %s: %v\n", completedTaskID, err)
		return
	}

	// Find tasks that are now unblocked
	unblockedTasks, err := s.db.GetTasksUnblockedBy(completedTaskID)
	if err != nil {
		fmt.Printf("handleTaskUnblocking: failed to get unblocked tasks for %s: %v\n", completedTaskID, err)
		return
	}

	if len(unblockedTasks) == 0 {
		return
	}

	fmt.Printf("handleTaskUnblocking: %d tasks unblocked by completion of %s\n", len(unblockedTasks), completedTaskID)

	// Get handoff summary from completed task for context passing
	var predecessorHandoff string
	if completedTask.WorktreePath.Valid && completedTask.WorktreePath.String != "" {
		predecessorHandoff = s.generatePredecessorHandoff(completedTask)
	}

	for _, task := range unblockedTasks {
		// Transition from blocked to ready
		if err := s.db.TransitionTaskStatus(task.ID, db.TaskStatusBlocked, db.TaskStatusReady); err != nil {
			fmt.Printf("handleTaskUnblocking: failed to transition task %s to ready: %v\n", task.ID, err)
			continue
		}

		fmt.Printf("handleTaskUnblocking: task %s transitioned to ready\n", task.ID)

		// Broadcast task unblocked event
		if s.hub != nil {
			s.hub.Broadcast(websocket.Message{
				Type: "task.unblocked",
				Payload: map[string]any{
					"task_id":         task.ID,
					"unblocked_by":    completedTaskID,
					"quest_id":        task.QuestID.String,
					"title":           task.Title,
					"previous_status": db.TaskStatusBlocked,
					"new_status":      db.TaskStatusReady,
				},
			})
		}

		// Check if task should auto-start
		autoStart, err := s.db.GetTaskAutoStart(task.ID)
		if err != nil {
			fmt.Printf("handleTaskUnblocking: failed to get auto_start for task %s: %v\n", task.ID, err)
			continue
		}

		if autoStart {
			// Auto-start the task in a goroutine, inheriting predecessor's worktree
			taskID := task.ID
			inheritedWorktree := completedTask.GetWorktreePath()
			handoff := predecessorHandoff
			go func() {
				startResult, err := s.startTaskWithInheritance(context.Background(), taskID, inheritedWorktree, handoff)
				if err != nil {
					fmt.Printf("handleTaskUnblocking: auto-start failed for task %s: %v\n", taskID, err)
					// Broadcast auto-start failure
					if s.hub != nil {
						s.hub.Broadcast(websocket.Message{
							Type: "task.auto_start_failed",
							Payload: map[string]any{
								"task_id": taskID,
								"error":   err.Error(),
							},
						})
					}
					return
				}

				fmt.Printf("handleTaskUnblocking: auto-started task %s (session %s) with inherited worktree from %s\n",
					taskID, startResult.SessionID, completedTaskID)

				// Broadcast auto-start success
				if s.hub != nil {
					s.hub.Broadcast(websocket.Message{
						Type: "task.auto_started",
						Payload: map[string]any{
							"task_id":           taskID,
							"session_id":        startResult.SessionID,
							"worktree_path":     startResult.WorktreePath,
							"inherited_from":    completedTaskID,
							"predecessor_title": completedTask.Title,
						},
					})
				}
			}()
		}
	}
}

// generatePredecessorHandoff creates a handoff summary for the completed task
// to provide context to dependent tasks
func (s *Server) generatePredecessorHandoff(task *db.Task) string {
	var sb strings.Builder

	sb.WriteString("## Predecessor Task Completed\n\n")
	sb.WriteString(fmt.Sprintf("**Previous Task**: %s\n", task.Title))

	if task.Description.Valid && task.Description.String != "" {
		sb.WriteString(fmt.Sprintf("**Description**: %s\n", task.Description.String))
	}

	sb.WriteString("**Status**: Completed\n")

	if task.WorktreePath.Valid && task.WorktreePath.String != "" {
		sb.WriteString(fmt.Sprintf("**Working Directory**: %s\n", task.WorktreePath.String))
	}

	if task.BranchName.Valid && task.BranchName.String != "" {
		sb.WriteString(fmt.Sprintf("**Branch**: %s\n", task.BranchName.String))
	}

	// Get checklist summary if available
	if checklist, err := s.db.GetChecklistByTaskID(task.ID); err == nil && checklist != nil {
		if items, err := s.db.GetChecklistItems(checklist.ID); err == nil && len(items) > 0 {
			sb.WriteString("\n**Completed Work**:\n")
			for _, item := range items {
				if item.Status == db.ChecklistItemStatusDone {
					sb.WriteString(fmt.Sprintf("- [x] %s\n", item.Description))
				}
			}
		}
	}

	sb.WriteString("\n**Your Task**: Continue from where the previous task left off. Use the same working directory and build upon the completed work.\n")

	return sb.String()
}

// isValidGitRepo checks if the given path is a valid git repository
func (s *Server) isValidGitRepo(path string) bool {
	if path == "" {
		return false
	}
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isValidProjectPath checks if the given path is appropriate for use as a project directory
// It returns false for system directories, dex installation directories, and other invalid paths
func (s *Server) isValidProjectPath(path string) bool {
	if path == "" || path == "." || path == ".." {
		return false
	}

	// Normalize the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// System directories that should never be used (including subdirectories)
	systemPrefixes := []string{
		"/usr/",
		"/bin/",
		"/sbin/",
		"/lib/",
		"/etc/",
	}

	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(absPath, prefix) {
			return false
		}
	}

	// Check if this looks like the dex installation by checking for cmd/main.go
	// This catches /opt/dex or any other location where dex source is installed
	dexMarker := filepath.Join(absPath, "cmd", "main.go")
	if _, err := os.Stat(dexMarker); err == nil {
		// This is likely the dex source directory
		return false
	}

	return true
}
