// Package workers provides HTTP handlers for worker management.
package workers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
)

// Handler handles worker-related API requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new workers handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers worker-related routes.
func (h *Handler) RegisterRoutes(g *echo.Group) {
	workers := g.Group("/workers")
	workers.GET("", h.handleList)
	workers.GET("/status", h.handleStatus)
	workers.POST("/dispatch", h.handleDispatch)
	workers.POST("/:id/cancel", h.handleCancel)
}

// WorkerStatusResponse represents the response for worker status.
type WorkerStatusResponse struct {
	TotalWorkers   int                  `json:"total_workers"`
	IdleWorkers    int                  `json:"idle_workers"`
	RunningWorkers int                  `json:"running_workers"`
	Workers        []WorkerInfoResponse `json:"workers"`
}

// WorkerInfoResponse represents individual worker info.
type WorkerInfoResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	State       string `json:"state"`
	ObjectiveID string `json:"objective_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Iteration   int    `json:"iteration,omitempty"`
	TokensUsed  int    `json:"tokens_used,omitempty"`
}

// DispatchRequest represents a request to dispatch an objective to a worker.
type DispatchRequest struct {
	ObjectiveID string `json:"objective_id"`
}

// DispatchResponse represents the response from dispatching an objective.
type DispatchResponse struct {
	Success  bool   `json:"success"`
	WorkerID string `json:"worker_id,omitempty"`
	Message  string `json:"message,omitempty"`
}

// handleList returns the list of all workers.
func (h *Handler) handleList(c echo.Context) error {
	if h.deps.WorkerManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "worker manager not configured",
		})
	}

	workers := h.deps.WorkerManager.Workers()
	response := make([]WorkerInfoResponse, len(workers))

	for i, w := range workers {
		response[i] = WorkerInfoResponse{
			ID:          w.ID,
			Type:        string(w.Type),
			State:       string(w.State),
			ObjectiveID: w.ObjectiveID,
			SessionID:   w.SessionID,
			Iteration:   w.Iteration,
			TokensUsed:  w.TokensUsed,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// handleStatus returns the overall worker pool status.
func (h *Handler) handleStatus(c echo.Context) error {
	if h.deps.WorkerManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "worker manager not configured",
		})
	}

	workers := h.deps.WorkerManager.Workers()
	idleCount := h.deps.WorkerManager.IdleWorkerCount()
	runningCount := h.deps.WorkerManager.RunningWorkerCount()

	workerInfos := make([]WorkerInfoResponse, len(workers))
	for i, w := range workers {
		workerInfos[i] = WorkerInfoResponse{
			ID:          w.ID,
			Type:        string(w.Type),
			State:       string(w.State),
			ObjectiveID: w.ObjectiveID,
			SessionID:   w.SessionID,
			Iteration:   w.Iteration,
			TokensUsed:  w.TokensUsed,
		}
	}

	return c.JSON(http.StatusOK, WorkerStatusResponse{
		TotalWorkers:   len(workers),
		IdleWorkers:    idleCount,
		RunningWorkers: runningCount,
		Workers:        workerInfos,
	})
}

// handleDispatch dispatches an objective to an available worker.
func (h *Handler) handleDispatch(c echo.Context) error {
	if h.deps.WorkerManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "worker manager not configured",
		})
	}

	var req DispatchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.ObjectiveID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "objective_id is required",
		})
	}

	// TODO: Look up the objective from DB and create the full payload
	// For now, return that dispatch is not yet fully implemented
	return c.JSON(http.StatusNotImplemented, DispatchResponse{
		Success: false,
		Message: "dispatch endpoint not yet fully implemented - objective lookup pending",
	})
}

// handleCancel cancels an objective running on a worker.
func (h *Handler) handleCancel(c echo.Context) error {
	if h.deps.WorkerManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "worker manager not configured",
		})
	}

	objectiveID := c.Param("id")
	if objectiveID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "objective id is required",
		})
	}

	ctx := c.Request().Context()
	if err := h.deps.WorkerManager.CancelObjective(ctx, objectiveID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "cancelled",
	})
}
