// Package planning provides HTTP handlers for task planning operations.
package planning

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/planning"
	"github.com/lirancohen/dex/internal/task"
)

// Handler handles planning-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new planning handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all planning routes on the given group.
// All routes require authentication.
//   - GET /tasks/:id/planning
//   - POST /tasks/:id/planning/respond
//   - POST /tasks/:id/planning/accept
//   - POST /tasks/:id/planning/skip
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/tasks/:id/planning", h.HandleGet)
	g.POST("/tasks/:id/planning/respond", h.HandleRespond)
	g.POST("/tasks/:id/planning/accept", h.HandleAccept)
	g.POST("/tasks/:id/planning/skip", h.HandleSkip)
}

// planner returns the planning service or nil if not configured.
func (h *Handler) planner() *planning.Planner {
	return h.deps.Planner
}

// taskService returns the task service.
func (h *Handler) taskService() *task.Service {
	return h.deps.TaskService
}

// HandleGet returns the planning session and messages for a task.
// GET /api/v1/tasks/:id/planning
func (h *Handler) HandleGet(c echo.Context) error {
	taskID := c.Param("id")

	if h.planner() == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	session, messages, err := h.planner().GetSessionByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	// Convert messages to response format
	msgResponses := make([]map[string]any, len(messages))
	for i, msg := range messages {
		msgResponses[i] = map[string]any{
			"id":         msg.ID,
			"role":       msg.Role,
			"content":    msg.Content,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		}
	}

	// Build session response
	sessionResp := map[string]any{
		"id":              session.ID,
		"task_id":         session.TaskID,
		"status":          session.Status,
		"original_prompt": session.OriginalPrompt,
		"refined_prompt":  session.RefinedPrompt.String,
		"created_at":      session.CreatedAt.Format(time.RFC3339),
	}

	// Include pending checklist if present
	if pendingChecklist := session.GetPendingChecklist(); pendingChecklist != nil {
		sessionResp["pending_checklist"] = pendingChecklist
	}

	return c.JSON(http.StatusOK, map[string]any{
		"session":  sessionResp,
		"messages": msgResponses,
	})
}

// HandleRespond handles a user response during planning.
// POST /api/v1/tasks/:id/planning/respond
func (h *Handler) HandleRespond(c echo.Context) error {
	taskID := c.Param("id")

	if h.planner() == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	var req struct {
		Response string `json:"response"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Response == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "response is required")
	}

	// Get the planning session for this task
	session, _, err := h.planner().GetSessionByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	// Process the response
	updatedSession, err := h.planner().ProcessResponse(c.Request().Context(), session.ID, req.Response)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Get updated messages
	_, messages, err := h.planner().GetSession(session.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert messages to response format
	msgResponses := make([]map[string]any, len(messages))
	for i, msg := range messages {
		msgResponses[i] = map[string]any{
			"id":         msg.ID,
			"role":       msg.Role,
			"content":    msg.Content,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"session": map[string]any{
			"id":              updatedSession.ID,
			"task_id":         updatedSession.TaskID,
			"status":          updatedSession.Status,
			"original_prompt": updatedSession.OriginalPrompt,
			"refined_prompt":  updatedSession.RefinedPrompt.String,
			"created_at":      updatedSession.CreatedAt.Format(time.RFC3339),
		},
		"messages": msgResponses,
	})
}

// HandleAccept accepts the current plan and transitions task to ready.
// POST /api/v1/tasks/:id/planning/accept
func (h *Handler) HandleAccept(c echo.Context) error {
	taskID := c.Param("id")

	if h.planner() == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	// Parse request body for optional item selection
	var req struct {
		SelectedOptional []int `json:"selected_optional"`
	}
	_ = c.Bind(&req) // Ignore error - optional

	// Get the planning session for this task
	session, _, err := h.planner().GetSessionByTask(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	// Accept the plan
	refinedPrompt, err := h.planner().AcceptPlan(c.Request().Context(), session.ID, req.SelectedOptional)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Transition task to ready
	if err := h.taskService().UpdateStatus(taskID, db.TaskStatusReady); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast task updated event
	h.deps.Hub.Broadcast(websocket.Message{
		Type: "task.updated",
		Payload: map[string]any{
			"task_id": taskID,
			"status":  db.TaskStatusReady,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message":        "plan accepted",
		"task_id":        taskID,
		"refined_prompt": refinedPrompt,
	})
}

// HandleSkip skips the planning phase and transitions task to ready.
// POST /api/v1/tasks/:id/planning/skip
func (h *Handler) HandleSkip(c echo.Context) error {
	taskID := c.Param("id")

	if h.planner() == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "planning not available")
	}

	// Skip the planning
	if err := h.planner().SkipPlanning(c.Request().Context(), taskID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Transition task to ready
	if err := h.taskService().UpdateStatus(taskID, db.TaskStatusReady); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast task updated event
	h.deps.Hub.Broadcast(websocket.Message{
		Type: "task.updated",
		Payload: map[string]any{
			"task_id": taskID,
			"status":  db.TaskStatusReady,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message": "planning skipped",
		"task_id": taskID,
	})
}
