// Package tasks provides HTTP handlers for task operations.
package tasks

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/security"
	"github.com/lirancohen/dex/internal/task"
)

// Handler handles task-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new tasks handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all task routes on the given group.
// All routes require authentication.
//   - GET /tasks
//   - POST /tasks
//   - GET /tasks/:id
//   - PUT /tasks/:id
//   - DELETE /tasks/:id
//   - POST /tasks/:id/start
//   - GET /tasks/:id/worktree/status
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/tasks", h.HandleList)
	g.POST("/tasks", h.HandleCreate)
	g.GET("/tasks/:id", h.HandleGet)
	g.PUT("/tasks/:id", h.HandleUpdate)
	g.DELETE("/tasks/:id", h.HandleDelete)
	g.POST("/tasks/:id/start", h.HandleStart)
	g.GET("/tasks/:id/worktree/status", h.HandleWorktreeStatus)
}

// HandleList returns tasks with optional filters.
// GET /api/v1/tasks?project_id=...&status=...
func (h *Handler) HandleList(c echo.Context) error {
	filters := task.ListFilters{
		ProjectID: c.QueryParam("project_id"),
		Status:    c.QueryParam("status"),
	}

	tasks, err := h.deps.TaskService.List(filters)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	taskResponses := make([]core.TaskResponse, len(tasks))
	for i, t := range tasks {
		// Get blocking info for each task
		blockerIDs, _ := h.deps.DB.GetIncompleteBlockerIDs(t.ID)
		taskResponses[i] = core.ToTaskResponseWithBlocking(t, blockerIDs)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"tasks": taskResponses,
		"count": len(tasks),
	})
}

// HandleCreate creates a new task.
// POST /api/v1/tasks?skip_planning=true
func (h *Handler) HandleCreate(c echo.Context) error {
	var req struct {
		ProjectID   any    `json:"project_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Priority    int    `json:"priority"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	skipPlanning := c.QueryParam("skip_planning") == "true"

	// Get or create default project for single-user mode
	projectID := ""
	if req.ProjectID != nil {
		projectID = fmt.Sprintf("%v", req.ProjectID)
	}

	if projectID == "" || projectID == "0" || projectID == "1" {
		project, err := h.deps.DB.GetOrCreateDefaultProject()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get default project")
		}
		projectID = project.ID
	}

	// Sanitize user input
	sanitizedTitle := security.SanitizeForPrompt(req.Title)
	sanitizedDescription := security.SanitizeForPrompt(req.Description)

	t, err := h.deps.TaskService.Create(projectID, sanitizedTitle, req.Type, req.Priority)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Update description if provided
	if sanitizedDescription != "" {
		updates := task.TaskUpdates{Description: &sanitizedDescription}
		t, err = h.deps.TaskService.Update(t.ID, updates)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to set description")
		}
	}

	// Start planning phase if planner is available and skip_planning is not set
	if h.deps.Planner != nil && !skipPlanning {
		planningPrompt := sanitizedDescription
		if planningPrompt == "" {
			planningPrompt = sanitizedTitle
		}

		if err := h.deps.TaskService.UpdateStatus(t.ID, db.TaskStatusPlanning); err != nil {
			fmt.Printf("warning: failed to transition task to planning: %v\n", err)
		} else {
			_, err := h.deps.Planner.StartPlanning(c.Request().Context(), t.ID, planningPrompt)
			if err != nil {
				fmt.Printf("warning: failed to start planning: %v\n", err)
				h.deps.TaskService.UpdateStatus(t.ID, db.TaskStatusPending)
			} else {
				t.Status = db.TaskStatusPlanning
			}
		}
	}

	return c.JSON(http.StatusCreated, core.ToTaskResponse(t))
}

// HandleGet returns a single task by ID.
// GET /api/v1/tasks/:id
func (h *Handler) HandleGet(c echo.Context) error {
	id := c.Param("id")

	t, err := h.deps.TaskService.Get(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Get blocking info
	blockerIDs, _ := h.deps.DB.GetIncompleteBlockerIDs(t.ID)

	return c.JSON(http.StatusOK, core.ToTaskResponseWithBlocking(t, blockerIDs))
}

// HandleUpdate updates a task.
// PUT /api/v1/tasks/:id
func (h *Handler) HandleUpdate(c echo.Context) error {
	id := c.Param("id")

	var updates task.TaskUpdates
	if err := c.Bind(&updates); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updated, err := h.deps.TaskService.Update(id, updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, core.ToTaskResponse(updated))
}

// HandleDelete removes a task.
// DELETE /api/v1/tasks/:id
func (h *Handler) HandleDelete(c echo.Context) error {
	id := c.Param("id")

	if err := h.deps.TaskService.Delete(id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleStart transitions a task to running and sets up its worktree.
// POST /api/v1/tasks/:id/start
func (h *Handler) HandleStart(c echo.Context) error {
	taskID := c.Param("id")

	var req struct {
		BaseBranch string `json:"base_branch"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Check if task is blocked by incomplete dependencies
	blockerIDs, err := h.deps.DB.GetIncompleteBlockerIDs(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check dependencies")
	}
	if len(blockerIDs) > 0 {
		return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("task is blocked by incomplete dependencies: %v", blockerIDs))
	}

	result, err := h.deps.StartTaskInternal(context.Background(), taskID, req.BaseBranch)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if strings.Contains(err.Error(), "already has a worktree") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		if strings.Contains(err.Error(), "not configured") {
			return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"task":          result.Task,
		"worktree_path": result.WorktreePath,
		"session_id":    result.SessionID,
	})
}

// HandleWorktreeStatus returns the git status of a task's worktree.
// GET /api/v1/tasks/:id/worktree/status
func (h *Handler) HandleWorktreeStatus(c echo.Context) error {
	taskID := c.Param("id")

	if h.deps.GitService == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
	}

	status, err := h.deps.GitService.GetTaskWorktreeStatus(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, status)
}
