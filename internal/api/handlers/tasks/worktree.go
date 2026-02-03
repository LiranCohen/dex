package tasks

import (
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
//   - DELETE /worktrees/:task_id
func (h *WorktreeHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/worktrees", h.HandleList)
	g.DELETE("/worktrees/:task_id", h.HandleDelete)
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

// HandleDelete removes a task's worktree.
// DELETE /api/v1/worktrees/:task_id?project_path=...&cleanup_branch=true
func (h *WorktreeHandler) HandleDelete(c echo.Context) error {
	taskID := c.Param("task_id")
	projectPath := c.QueryParam("project_path")
	cleanupBranch := c.QueryParam("cleanup_branch") == "true"

	if projectPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "project_path is required")
	}

	if h.deps.GitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	if err := h.deps.GitService.CleanupTaskWorktree(projectPath, taskID, cleanupBranch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}
