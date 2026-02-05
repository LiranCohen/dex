// Package workers provides HTTP handlers for worker management.
package workers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/worker"
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

	ctx := c.Request().Context()

	// Look up the task (objective) from DB
	task, err := h.deps.DB.GetTaskByID(req.ObjectiveID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to get task: %v", err),
		})
	}
	if task == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "task not found",
		})
	}

	// Get the project
	project, err := h.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to get project: %v", err),
		})
	}
	if project == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "project not found",
		})
	}

	// Get secrets from encrypted store
	secrets, err := h.getWorkerSecrets()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to get secrets: %v", err),
		})
	}

	// Build the objective payload
	objective := worker.Objective{
		ID:          task.ID,
		Title:       task.Title,
		Description: task.GetDescription(),
		Hat:         task.Hat.String,
		BaseBranch:  task.BaseBranch,
	}
	if task.TokenBudget.Valid {
		objective.TokenBudget = int(task.TokenBudget.Int64)
	}

	// Build project info
	projectInfo := worker.Project{
		ID:          project.ID,
		Name:        project.Name,
		GitHubOwner: project.GitHubOwner,
		GitHubRepo:  project.GitHubRepo,
		CloneURL:    fmt.Sprintf("https://github.com/%s/%s.git", project.GitHubOwner, project.GitHubRepo),
	}

	// Build sync config
	syncConfig := worker.SyncConfig{
		HQEndpoint:           "", // Will be set when mesh is configured
		ActivityIntervalSec:  30,
		HeartbeatIntervalSec: 10,
	}

	// Build the full payload (secrets will be encrypted by the manager)
	payload := &worker.ObjectivePayload{
		Objective:    objective,
		Project:      projectInfo,
		Sync:         syncConfig,
		DispatchedAt: time.Now(),
	}

	// Dispatch to an available worker with encrypted secrets
	if err := h.deps.WorkerManager.DispatchWithSecrets(ctx, payload, &secrets); err != nil {
		return c.JSON(http.StatusServiceUnavailable, DispatchResponse{
			Success: false,
			Message: fmt.Sprintf("failed to dispatch: %v", err),
		})
	}

	return c.JSON(http.StatusOK, DispatchResponse{
		Success: true,
		Message: "objective dispatched successfully",
	})
}

// getWorkerSecrets retrieves the secrets needed for worker execution.
func (h *Handler) getWorkerSecrets() (worker.WorkerSecrets, error) {
	var secrets worker.WorkerSecrets

	if h.deps.SecretsStore == nil {
		// Fallback to toolbelt if secrets store not configured
		tb := h.deps.GetToolbelt()
		if tb != nil && tb.Anthropic != nil {
			secrets.AnthropicKey = tb.Anthropic.GetAPIKey()
		}
		if tb != nil && tb.GitHub != nil {
			secrets.GitHubToken = tb.GitHub.GetToken()
		}
		return secrets, nil
	}

	// Get from encrypted secrets store
	key, err := h.deps.SecretsStore.GetSecret("anthropic_api_key")
	if err != nil {
		return secrets, fmt.Errorf("failed to get anthropic key: %w", err)
	}
	secrets.AnthropicKey = key

	token, err := h.deps.SecretsStore.GetSecret("github_token")
	if err != nil {
		return secrets, fmt.Errorf("failed to get github token: %w", err)
	}
	secrets.GitHubToken = token

	// Optional secrets
	secrets.FlyToken, _ = h.deps.SecretsStore.GetSecret("fly_token")
	secrets.CloudflareToken, _ = h.deps.SecretsStore.GetSecret("cloudflare_token")

	return secrets, nil
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
