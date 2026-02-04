package tasks

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
)

// WorktreeHandler handles worktree-related HTTP requests.
type WorktreeHandler struct {
	deps *core.Deps
}

// NewWorktreeHandler creates a new worktree handler.
func NewWorktreeHandler(deps *core.Deps) *WorktreeHandler {
	return &WorktreeHandler{deps: deps}
}

// RegisterRoutes registers all worktree routes on the given group.
// All routes require authentication.
//   - GET /worktrees
//   - GET /worktrees/stale
//   - DELETE /worktrees/:task_id
//   - POST /worktrees/cleanup-merged
func (h *WorktreeHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/worktrees", h.HandleList)
	g.GET("/worktrees/stale", h.HandleListStale)
	g.DELETE("/worktrees/:task_id", h.HandleDelete)
	g.POST("/worktrees/cleanup-merged", h.HandleCleanupMerged)
}

// HandleList returns all worktrees for a project.
// GET /api/v1/worktrees?project_path=...
func (h *WorktreeHandler) HandleList(c echo.Context) error {
	projectPath := c.QueryParam("project_path")
	if projectPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "project_path is required")
	}

	if h.deps.GitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	worktrees, err := h.deps.GitService.ListWorktrees(projectPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"worktrees": worktrees,
		"count":     len(worktrees),
	})
}

// HandleListStale returns tasks with worktrees that should be cleaned up.
// GET /api/v1/worktrees/stale
// Returns tasks that are completed/cancelled but still have worktrees.
// Checks git directly to determine if branches have been merged.
func (h *WorktreeHandler) HandleListStale(c echo.Context) error {
	tasks, err := h.deps.DB.GetTasksWithStaleWorktrees()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Build response with task info and worktree status
	type StaleWorktree struct {
		TaskID       string  `json:"task_id"`
		TaskTitle    string  `json:"task_title"`
		WorktreePath string  `json:"worktree_path"`
		BranchName   string  `json:"branch_name"`
		BaseBranch   string  `json:"base_branch"`
		PRNumber     *int64  `json:"pr_number,omitempty"`
		BranchMerged bool    `json:"branch_merged"`
		Status       string  `json:"status"`
		CompletedAt  *string `json:"completed_at,omitempty"`
	}

	stale := make([]StaleWorktree, 0, len(tasks))
	for _, t := range tasks {
		sw := StaleWorktree{
			TaskID:     t.ID,
			TaskTitle:  t.Title,
			Status:     t.Status,
			BaseBranch: t.BaseBranch,
		}
		if t.WorktreePath.Valid {
			sw.WorktreePath = t.WorktreePath.String
		}
		if t.BranchName.Valid {
			sw.BranchName = t.BranchName.String
		}
		if t.PRNumber.Valid {
			prNum := t.PRNumber.Int64
			sw.PRNumber = &prNum
		}
		if t.CompletedAt.Valid {
			ts := t.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
			sw.CompletedAt = &ts
		}

		// Check git merge status if we have branch info and git service
		if h.deps.GitService != nil && t.BranchName.Valid && t.BranchName.String != "" {
			merged, err := h.deps.GitService.IsTaskBranchMerged(t.ID)
			if err == nil {
				sw.BranchMerged = merged
			}
		}

		stale = append(stale, sw)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"stale_worktrees": stale,
		"count":           len(stale),
	})
}

// HandleDelete removes a task's worktree.
// DELETE /api/v1/worktrees/:task_id?project_path=...&cleanup_branch=true
func (h *WorktreeHandler) HandleDelete(c echo.Context) error {
	taskID := c.Param("task_id")
	projectPath := c.QueryParam("project_path")
	cleanupBranch := c.QueryParam("cleanup_branch") == "true"

	// If project_path not provided, try to get it from the task's project
	if projectPath == "" {
		task, err := h.deps.DB.GetTaskByID(taskID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if task == nil {
			return echo.NewHTTPError(http.StatusNotFound, "task not found")
		}

		project, err := h.deps.DB.GetProjectByID(task.ProjectID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if project == nil {
			return echo.NewHTTPError(http.StatusNotFound, "project not found")
		}
		projectPath = project.RepoPath
	}

	if h.deps.GitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	if err := h.deps.GitService.CleanupTaskWorktree(projectPath, taskID, cleanupBranch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Mark the worktree as cleaned in the database
	if err := h.deps.DB.MarkTaskWorktreeCleaned(taskID); err != nil {
		// Log but don't fail - the worktree was cleaned
		fmt.Printf("warning: failed to mark task %s worktree as cleaned: %v\n", taskID, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleCleanupMerged cleans up all worktrees for tasks whose branches have been merged.
// POST /api/v1/worktrees/cleanup-merged
// This is safe to run - it checks git directly to verify branches are merged before cleanup.
func (h *WorktreeHandler) HandleCleanupMerged(c echo.Context) error {
	if h.deps.GitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	// Get all stale worktrees (completed/cancelled tasks with worktrees)
	tasks, err := h.deps.DB.GetTasksWithStaleWorktrees()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var cleaned, skipped, failed int
	var errors []string

	for _, task := range tasks {
		if !task.WorktreePath.Valid || task.WorktreePath.String == "" {
			continue
		}

		if !task.BranchName.Valid || task.BranchName.String == "" {
			skipped++
			continue
		}

		// Check if branch is merged using git
		merged, err := h.deps.GitService.IsTaskBranchMerged(task.ID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("task %s: failed to check merge status: %v", task.ID, err))
			failed++
			continue
		}

		if !merged {
			skipped++ // Branch not merged yet, skip
			continue
		}

		// Get project path
		project, err := h.deps.DB.GetProjectByID(task.ProjectID)
		if err != nil || project == nil {
			errors = append(errors, fmt.Sprintf("task %s: failed to get project", task.ID))
			failed++
			continue
		}

		// Clean up the worktree (also delete the branch since it's merged)
		if err := h.deps.GitService.CleanupTaskWorktree(project.RepoPath, task.ID, true); err != nil {
			errors = append(errors, fmt.Sprintf("task %s: %v", task.ID, err))
			failed++
			continue
		}

		// Mark as cleaned
		if err := h.deps.DB.MarkTaskWorktreeCleaned(task.ID); err != nil {
			fmt.Printf("warning: failed to mark task %s worktree as cleaned: %v\n", task.ID, err)
		}

		cleaned++
	}

	return c.JSON(http.StatusOK, map[string]any{
		"cleaned": cleaned,
		"skipped": skipped,
		"failed":  failed,
		"errors":  errors,
	})
}
