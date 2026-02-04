// Package approvals provides HTTP handlers for approval operations.
package approvals

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/realtime"
)

// Handler handles approval-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new approvals handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all approval routes on the given group.
// All routes require authentication.
//   - GET /approvals
//   - GET /approvals/:id
//   - POST /approvals/:id/approve
//   - POST /approvals/:id/reject
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/approvals", h.HandleList)
	g.GET("/approvals/:id", h.HandleGet)
	g.POST("/approvals/:id/approve", h.HandleApprove)
	g.POST("/approvals/:id/reject", h.HandleReject)
}

// HandleList returns approvals with optional filters.
// GET /api/v1/approvals?status=pending&task_id=xyz
func (h *Handler) HandleList(c echo.Context) error {
	status := c.QueryParam("status")
	taskID := c.QueryParam("task_id")

	var approvals []*core.ApprovalResponse
	var err error

	switch {
	case taskID != "":
		dbApprovals, err := h.deps.DB.ListApprovalsByTask(taskID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		approvals = make([]*core.ApprovalResponse, len(dbApprovals))
		for i, a := range dbApprovals {
			resp := core.ToApprovalResponse(a)
			approvals[i] = &resp
		}
	case status != "":
		dbApprovals, err := h.deps.DB.ListApprovalsByStatus(status)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		approvals = make([]*core.ApprovalResponse, len(dbApprovals))
		for i, a := range dbApprovals {
			resp := core.ToApprovalResponse(a)
			approvals[i] = &resp
		}
	default:
		dbApprovals, err := h.deps.DB.ListPendingApprovals()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		approvals = make([]*core.ApprovalResponse, len(dbApprovals))
		for i, a := range dbApprovals {
			resp := core.ToApprovalResponse(a)
			approvals[i] = &resp
		}
	}

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"approvals": approvals,
		"count":     len(approvals),
	})
}

// HandleGet returns a single approval by ID.
// GET /api/v1/approvals/:id
func (h *Handler) HandleGet(c echo.Context) error {
	id := c.Param("id")

	approval, err := h.deps.DB.GetApprovalByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if approval == nil {
		return echo.NewHTTPError(http.StatusNotFound, "approval not found")
	}

	return c.JSON(http.StatusOK, core.ToApprovalResponse(approval))
}

// HandleApprove marks an approval as approved.
// POST /api/v1/approvals/:id/approve
func (h *Handler) HandleApprove(c echo.Context) error {
	id := c.Param("id")

	// Get approval first to include routing info in broadcast
	approval, err := h.deps.DB.GetApprovalByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if approval == nil {
		return echo.NewHTTPError(http.StatusNotFound, "approval not found")
	}

	if err := h.deps.DB.ApproveApproval(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, "approval not found")
		}
		if strings.Contains(err.Error(), "already resolved") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast WebSocket event with routing info
	if h.deps.Broadcaster != nil {
		payload := map[string]any{
			"id":     id,
			"status": "approved",
		}
		// Include task_id for channel routing
		if approval.TaskID.Valid {
			payload["task_id"] = approval.TaskID.String
			// Also fetch project_id from task
			if task, err := h.deps.DB.GetTaskByID(approval.TaskID.String); err == nil && task != nil {
				payload["project_id"] = task.ProjectID
			}
		}
		// Include user_id from auth context
		if userID := c.Get("user_id"); userID != nil {
			payload["user_id"] = userID
		}
		h.deps.Broadcaster.Publish(realtime.EventApprovalResolved, payload)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "approval approved",
		"id":      id,
	})
}

// HandleReject marks an approval as rejected.
// POST /api/v1/approvals/:id/reject
func (h *Handler) HandleReject(c echo.Context) error {
	id := c.Param("id")

	// Get approval first to include routing info in broadcast
	approval, err := h.deps.DB.GetApprovalByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if approval == nil {
		return echo.NewHTTPError(http.StatusNotFound, "approval not found")
	}

	if err := h.deps.DB.RejectApproval(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, "approval not found")
		}
		if strings.Contains(err.Error(), "already resolved") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Broadcast WebSocket event with routing info
	if h.deps.Broadcaster != nil {
		payload := map[string]any{
			"id":     id,
			"status": "rejected",
		}
		// Include task_id for channel routing
		if approval.TaskID.Valid {
			payload["task_id"] = approval.TaskID.String
			// Also fetch project_id from task
			if task, err := h.deps.DB.GetTaskByID(approval.TaskID.String); err == nil && task != nil {
				payload["project_id"] = task.ProjectID
			}
		}
		// Include user_id from auth context
		if userID := c.Get("user_id"); userID != nil {
			payload["user_id"] = userID
		}
		h.deps.Broadcaster.Publish(realtime.EventApprovalResolved, payload)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "approval rejected",
		"id":      id,
	})
}
