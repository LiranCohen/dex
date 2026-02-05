package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/realtime"
)

// startTaskResult contains the result of starting a task
type startTaskResult struct {
	Task         *db.Task
	WorktreePath string
	SessionID    string
}

// startTaskOptions configures how a task should be started
type startTaskOptions struct {
	BaseBranch         string // Base branch for worktree creation
	InheritedWorktree  string // Worktree to inherit from predecessor
	PredecessorHandoff string // Context from predecessor task
}

// startTask starts a task with the given options
// This is the single entry point for all task starting logic
func (s *Server) startTask(ctx context.Context, taskID string, opts startTaskOptions) (*startTaskResult, error) {
	// Get the task
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

	// Resolve the worktree path
	worktreePath, err := s.resolveWorktreePath(taskID, project, opts)
	if err != nil {
		return nil, err
	}

	// Transition to running status
	if err := s.transitionTaskToRunning(taskID, t.Status); err != nil {
		return nil, err
	}

	// Broadcast task started
	s.broadcastTaskUpdated(taskID, "running")

	// Create and start session
	sess, err := s.createAndStartSession(ctx, taskID, t, worktreePath, opts.PredecessorHandoff)
	if err != nil {
		return nil, err
	}

	// Fetch updated task
	updated, _ := s.taskService.Get(taskID)

	return &startTaskResult{
		Task:         updated,
		WorktreePath: worktreePath,
		SessionID:    sess.ID,
	}, nil
}

// resolveWorktreePath determines the appropriate working directory for a task
func (s *Server) resolveWorktreePath(taskID string, project *db.Project, opts startTaskOptions) (string, error) {
	// Try to inherit worktree from predecessor
	if opts.InheritedWorktree != "" {
		if _, err := os.Stat(opts.InheritedWorktree); err == nil {
			fmt.Printf("resolveWorktreePath: task %s inheriting worktree %s\n", taskID, opts.InheritedWorktree)
			if err := s.db.UpdateTaskWorktree(taskID, opts.InheritedWorktree, ""); err != nil {
				return "", fmt.Errorf("failed to save inherited worktree path: %w", err)
			}
			return opts.InheritedWorktree, nil
		}
		fmt.Printf("resolveWorktreePath: inherited worktree %s no longer exists, creating new\n", opts.InheritedWorktree)
	}

	projectPath := project.RepoPath
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = project.DefaultBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
	}

	hasGitRepo := s.isValidGitRepo(projectPath)
	isValidPath := s.isValidProjectPath(projectPath)

	// Case 1: Existing git repo - create a proper git worktree
	if hasGitRepo && isValidPath && s.gitService != nil {
		worktreePath, err := s.gitService.SetupTaskWorktree(projectPath, taskID, baseBranch)
		if err != nil {
			return "", fmt.Errorf("failed to create worktree: %w", err)
		}
		return worktreePath, nil
	}

	// Case 2: Valid project path but no git repo yet - work directly in project path
	// This is for new projects - the task will initialize git here
	if isValidPath && projectPath != "" {
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create project directory: %w", err)
		}
		if err := s.db.UpdateTaskWorktree(taskID, projectPath, ""); err != nil {
			return "", fmt.Errorf("failed to save worktree path: %w", err)
		}
		fmt.Printf("resolveWorktreePath: task %s working directly in project path %s\n", taskID, projectPath)
		return projectPath, nil
	}

	// Case 3: No valid project path - create task-specific directory
	// This shouldn't happen often, but handles edge cases
	var worktreePath string
	if s.baseDir != "" {
		worktreePath = filepath.Join(s.baseDir, "worktrees", "task-"+taskID)
	} else {
		worktreePath = filepath.Join(os.TempDir(), "dex-task-"+taskID)
	}
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create task directory: %w", err)
	}
	if err := s.db.UpdateTaskWorktree(taskID, worktreePath, ""); err != nil {
		return "", fmt.Errorf("failed to save worktree path: %w", err)
	}
	fmt.Printf("resolveWorktreePath: task %s using fallback directory %s (project path '%s' invalid)\n", taskID, worktreePath, projectPath)
	return worktreePath, nil
}

// transitionTaskToRunning moves a task through the state machine to running
func (s *Server) transitionTaskToRunning(taskID, currentStatus string) error {
	// Handle different starting states
	switch currentStatus {
	case "pending", "blocked":
		if err := s.taskService.UpdateStatus(taskID, "ready"); err != nil {
			return fmt.Errorf("failed to transition to ready: %w", err)
		}
	}

	if err := s.taskService.UpdateStatus(taskID, "running"); err != nil {
		return fmt.Errorf("failed to transition to running: %w", err)
	}

	return nil
}

// createAndStartSession creates a session for a task and starts it
func (s *Server) createAndStartSession(ctx context.Context, taskID string, task *db.Task, worktreePath, predecessorHandoff string) (*struct{ ID string }, error) {
	hat := "creator"
	if task.Hat.Valid && task.Hat.String != "" {
		hat = task.Hat.String
	}

	sess, err := s.sessionManager.CreateSession(taskID, hat, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	if predecessorHandoff != "" {
		s.sessionManager.SetPredecessorContext(sess.ID, predecessorHandoff)
	}

	if err := s.sessionManager.Start(ctx, sess.ID); err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	return &struct{ ID string }{ID: sess.ID}, nil
}

// broadcastTaskUpdated sends a task.updated WebSocket event
func (s *Server) broadcastTaskUpdated(taskID, status string) {
	if s.broadcaster != nil {
		payload := map[string]any{
			"status": status,
		}
		// Include project_id for channel routing
		if task, err := s.db.GetTaskByID(taskID); err == nil && task != nil {
			payload["project_id"] = task.ProjectID
		}
		s.broadcaster.PublishTaskEvent(realtime.EventTaskUpdated, taskID, payload)
	}
}

// startTaskInternal starts a task by ID with an optional base branch
// This is a convenience wrapper for external callers
func (s *Server) startTaskInternal(ctx context.Context, taskID string, baseBranch string) (*startTaskResult, error) {
	return s.startTask(ctx, taskID, startTaskOptions{
		BaseBranch: baseBranch,
	})
}

// startTaskWithInheritance starts a task, optionally inheriting a worktree from a predecessor
// This is a convenience wrapper for task dependency handling
func (s *Server) startTaskWithInheritance(ctx context.Context, taskID string, inheritedWorktree string, predecessorHandoff string) (*startTaskResult, error) {
	return s.startTask(ctx, taskID, startTaskOptions{
		InheritedWorktree:  inheritedWorktree,
		PredecessorHandoff: predecessorHandoff,
	})
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

	// Find tasks that are now ready to auto-start
	tasksToAutoStart, err := s.db.GetTasksReadyToAutoStart(completedTaskID)
	if err != nil {
		fmt.Printf("handleTaskUnblocking: failed to get tasks ready to auto-start for %s: %v\n", completedTaskID, err)
		return
	}

	if len(tasksToAutoStart) == 0 {
		return
	}

	fmt.Printf("handleTaskUnblocking: %d tasks ready to auto-start after completion of %s\n", len(tasksToAutoStart), completedTaskID)

	// Get handoff summary from completed task for context passing
	var predecessorHandoff string
	if completedTask.WorktreePath.Valid && completedTask.WorktreePath.String != "" {
		predecessorHandoff = s.generatePredecessorHandoff(completedTask)
	}

	for _, task := range tasksToAutoStart {
		// Broadcast task unblocked event
		if s.broadcaster != nil {
			s.broadcaster.PublishTaskEvent(realtime.EventTaskUnblocked, task.ID, map[string]any{
				"unblocked_by": completedTaskID,
				"quest_id":     task.QuestID.String,
				"title":        task.Title,
				"project_id":   task.ProjectID,
			})
		}

		// Auto-start the task in a goroutine, inheriting predecessor's worktree
		taskID := task.ID
		projectID := task.ProjectID
		inheritedWorktree := completedTask.GetWorktreePath()
		handoff := predecessorHandoff
		go func() {
			startResult, err := s.startTaskWithInheritance(context.Background(), taskID, inheritedWorktree, handoff)
			if err != nil {
				fmt.Printf("handleTaskUnblocking: auto-start failed for task %s: %v\n", taskID, err)
				if s.broadcaster != nil {
					s.broadcaster.PublishTaskEvent(realtime.EventTaskAutoStartFailed, taskID, map[string]any{
						"error":      err.Error(),
						"project_id": projectID,
					})
				}
				return
			}

			fmt.Printf("handleTaskUnblocking: auto-started task %s (session %s) with inherited worktree from %s\n",
				taskID, startResult.SessionID, completedTaskID)

			if s.broadcaster != nil {
				s.broadcaster.PublishTaskEvent(realtime.EventTaskAutoStarted, taskID, map[string]any{
					"session_id":        startResult.SessionID,
					"worktree_path":     startResult.WorktreePath,
					"inherited_from":    completedTaskID,
					"predecessor_title": completedTask.Title,
					"project_id":        projectID,
				})
			}
		}()
	}
}

// generatePredecessorHandoff creates a handoff summary for the completed task
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
// Handles regular repos (.git directory), git worktrees (.git file),
// and bare repos (HEAD + objects/ + refs/ directly in path, used by Forgejo).
func (s *Server) isValidGitRepo(path string) bool {
	if path == "" {
		return false
	}
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err == nil {
		// Regular repo has .git directory, worktree has .git file
		return info.IsDir() || info.Mode().IsRegular()
	}
	// Check for bare repo (Forgejo stores repos as bare)
	return git.IsBareRepo(path)
}

// isValidProjectPath checks if the given path is appropriate for use as a project directory
// Returns false for system directories and the dex installation directory itself
func (s *Server) isValidProjectPath(path string) bool {
	if path == "" || path == "." || path == ".." {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// System directories that should never be used for project code
	// Note: We allow subdirectories of /opt/ (like /opt/dex/repos/) but not /opt/ itself
	systemPaths := []string{
		"/usr",
		"/bin",
		"/sbin",
		"/lib",
		"/lib64",
		"/etc",
		"/var",
		"/root",
		"/boot",
		"/dev",
		"/proc",
		"/sys",
	}

	for _, sysPath := range systemPaths {
		// Reject exact match or if path is under system directory
		if absPath == sysPath || strings.HasPrefix(absPath, sysPath+"/") {
			return false
		}
	}

	// Reject the dex base directory itself (but allow subdirectories like repos/)
	// s.baseDir is typically /opt/dex
	if s.baseDir != "" {
		dexBase, err := filepath.Abs(s.baseDir)
		if err == nil && absPath == dexBase {
			return false
		}
	}

	return true
}
