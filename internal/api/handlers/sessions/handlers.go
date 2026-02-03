// Package sessions provides HTTP handlers for session management operations.
package sessions

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/session"
)

// Handler handles session-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new sessions handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all session routes on the given group.
// All routes require authentication.
//   - GET /sessions
//   - GET /sessions/:id
//   - POST /sessions/:id/kill
//   - GET /sessions/:id/activity
//   - POST /tasks/:id/pause
//   - POST /tasks/:id/resume
//   - POST /tasks/:id/cancel
//   - GET /tasks/:id/logs
//   - GET /tasks/:id/activity
func (h *Handler) RegisterRoutes(g *echo.Group) {
	// Session management
	g.GET("/sessions", h.HandleList)
	g.GET("/sessions/:id", h.HandleGet)
	g.POST("/sessions/:id/kill", h.HandleKill)
	g.GET("/sessions/:id/activity", h.HandleGetActivity)

	// Task session control
	g.POST("/tasks/:id/pause", h.HandlePauseTask)
	g.POST("/tasks/:id/resume", h.HandleResumeTask)
	g.POST("/tasks/:id/cancel", h.HandleCancelTask)
	g.GET("/tasks/:id/logs", h.HandleTaskLogs)
	g.GET("/tasks/:id/activity", h.HandleGetTaskActivity)
}

// HandleList returns all active sessions.
// GET /api/v1/sessions
func (h *Handler) HandleList(c echo.Context) error {
	sessions := h.deps.SessionManager.List()

	responses := make([]core.SessionResponse, len(sessions))
	for i, sess := range sessions {
		responses[i] = core.ToSessionResponse(sess)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"sessions": responses,
		"count":    len(responses),
	})
}

// HandleGet returns a single session by ID.
// GET /api/v1/sessions/:id
func (h *Handler) HandleGet(c echo.Context) error {
	sessionID := c.Param("id")

	sess := h.deps.SessionManager.Get(sessionID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	return c.JSON(http.StatusOK, core.ToSessionResponse(sess))
}

// HandleKill forcefully stops a session.
// POST /api/v1/sessions/:id/kill
func (h *Handler) HandleKill(c echo.Context) error {
	sessionID := c.Param("id")

	sess := h.deps.SessionManager.Get(sessionID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	if err := h.deps.SessionManager.Stop(sessionID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	h.deps.Hub.Broadcast(websocket.Message{
		Type: "session.killed",
		Payload: map[string]any{
			"session_id": sessionID,
			"task_id":    sess.TaskID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message":    "session killed",
		"session_id": sessionID,
	})
}

// HandlePauseTask pauses the running session for a task.
// POST /api/v1/tasks/:id/pause
func (h *Handler) HandlePauseTask(c echo.Context) error {
	taskID := c.Param("id")

	sess := h.deps.SessionManager.GetByTask(taskID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active session for task")
	}

	if err := h.deps.SessionManager.Pause(sess.ID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Update task status in database
	if err := h.deps.TaskService.UpdateStatus(taskID, "paused"); err != nil {
		fmt.Printf("warning: failed to update task status to paused: %v\n", err)
	}

	h.deps.Hub.Broadcast(websocket.Message{
		Type: "task.paused",
		Payload: map[string]any{
			"task_id":    taskID,
			"session_id": sess.ID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "task paused",
		"task_id": taskID,
	})
}

// HandleResumeTask resumes a paused session for a task.
// POST /api/v1/tasks/:id/resume
func (h *Handler) HandleResumeTask(c echo.Context) error {
	taskID := c.Param("id")

	// First check if there's an active session in memory
	sess := h.deps.SessionManager.GetByTask(taskID)

	if sess != nil {
		// Active session exists - check that it's paused
		if sess.State != session.StatePaused {
			return echo.NewHTTPError(http.StatusBadRequest, "session is not paused")
		}

		if err := h.deps.SessionManager.Start(c.Request().Context(), sess.ID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	} else {
		// No active session - check if task is paused and recreate session
		task, err := h.deps.DB.GetTaskByID(taskID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get task: %v", err))
		}
		if task == nil {
			return echo.NewHTTPError(http.StatusNotFound, "task not found")
		}
		if task.Status != db.TaskStatusPaused {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("task is not paused (status: %s)", task.Status))
		}

		// Get the last session for this task to restore hat and worktree path
		sessions, err := h.deps.DB.ListSessionsByTask(taskID)
		if err != nil || len(sessions) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no previous session found for task")
		}
		lastSession := sessions[0] // Most recent first

		// Create a new session with the same parameters
		newSess, err := h.deps.SessionManager.CreateSession(taskID, lastSession.Hat, lastSession.WorktreePath)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create session: %v", err))
		}

		// Set the session to restore from the previous session's checkpoint
		newSess.RestoreFromSessionID = lastSession.ID

		if err := h.deps.SessionManager.Start(c.Request().Context(), newSess.ID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		sess = newSess
	}

	// Update task status in database
	if err := h.deps.TaskService.UpdateStatus(taskID, "running"); err != nil {
		fmt.Printf("warning: failed to update task status to running: %v\n", err)
	}

	h.deps.Hub.Broadcast(websocket.Message{
		Type: "task.resumed",
		Payload: map[string]any{
			"task_id":    taskID,
			"session_id": sess.ID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "task resumed",
		"task_id": taskID,
	})
}

// HandleCancelTask cancels a task and its session.
// POST /api/v1/tasks/:id/cancel
func (h *Handler) HandleCancelTask(c echo.Context) error {
	taskID := c.Param("id")

	sess := h.deps.SessionManager.GetByTask(taskID)
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active session for task")
	}

	if err := h.deps.SessionManager.Stop(sess.ID); err != nil {
		fmt.Printf("warning: failed to stop session %s: %v\n", sess.ID, err)
	}

	if err := h.deps.TaskService.UpdateStatus(taskID, "cancelled"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.deps.Hub.Broadcast(websocket.Message{
		Type: "task.cancelled",
		Payload: map[string]any{
			"task_id":    taskID,
			"session_id": sess.ID,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "task cancelled",
		"task_id": taskID,
	})
}

// HandleTaskLogs returns logs for a task's session.
// GET /api/v1/tasks/:id/logs
func (h *Handler) HandleTaskLogs(c echo.Context) error {
	taskID := c.Param("id")

	_, err := h.deps.TaskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Placeholder response
	return c.JSON(http.StatusOK, map[string]any{
		"logs":    []any{},
		"message": "log streaming not yet implemented",
		"task_id": taskID,
	})
}

// HandleGetActivity returns all activity for a session.
// GET /api/v1/sessions/:id/activity
func (h *Handler) HandleGetActivity(c echo.Context) error {
	sessionID := c.Param("id")

	sess, err := h.deps.DB.GetSessionByID(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if sess == nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	activities, err := h.deps.DB.ListSessionActivity(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	summary, err := h.deps.DB.GetSessionActivitySummary(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	responses := make([]core.ActivityResponse, len(activities))
	for i, a := range activities {
		responses[i] = core.ToActivityResponse(a)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"activity": responses,
		"summary":  summary,
	})
}

// HandleGetTaskActivity returns all activity for all sessions of a task.
// GET /api/v1/tasks/:id/activity
func (h *Handler) HandleGetTaskActivity(c echo.Context) error {
	taskID := c.Param("id")

	task, err := h.deps.TaskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if task == nil {
		return echo.NewHTTPError(http.StatusNotFound, "task not found")
	}

	activities, err := h.deps.DB.ListTaskActivity(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sessions, err := h.deps.DB.ListSessionsByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Calculate totals - tokens from activity (single source of truth)
	inputTokens, outputTokens, _ := h.deps.DB.GetTaskTokensFromActivity(taskID)
	totalTokens := inputTokens + outputTokens
	var totalIterations int
	for _, sess := range sessions {
		if sess.IterationCount > totalIterations {
			totalIterations = sess.IterationCount
		}
	}

	responses := make([]core.ActivityResponse, len(activities))
	for i, a := range activities {
		responses[i] = core.ToActivityResponse(a)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"activity": responses,
		"summary": map[string]any{
			"total_sessions":   len(sessions),
			"total_iterations": totalIterations,
			"total_tokens":     totalTokens,
			"input_tokens":     inputTokens,
			"output_tokens":    outputTokens,
		},
	})
}
