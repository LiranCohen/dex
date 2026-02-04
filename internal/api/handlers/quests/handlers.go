// Package quests provides HTTP handlers for quest operations.
package quests

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/realtime"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// Handler handles quest-related HTTP requests.
type Handler struct {
	deps *core.Deps

	// GitHub sync callback (injected to avoid circular deps)
	SyncQuestToGitHubIssue  func(questID string)
	CloseQuestGitHubIssue   func(questID string, summary *db.QuestSummary)
	ReopenQuestGitHubIssue  func(questID string)
}

// New creates a new quests handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all quest routes on the given group.
// All routes require authentication.
//   - GET /projects/:id/quests
//   - POST /projects/:id/quests
//   - GET /quests/:id
//   - DELETE /quests/:id
//   - POST /quests/:id/messages
//   - POST /quests/:id/complete
//   - POST /quests/:id/reopen
//   - PUT /quests/:id/model
//   - GET /quests/:id/tasks
//   - GET /quests/:id/preflight
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/projects/:id/quests", h.HandleList)
	g.POST("/projects/:id/quests", h.HandleCreate)
	g.GET("/quests/:id", h.HandleGet)
	g.DELETE("/quests/:id", h.HandleDelete)
	g.POST("/quests/:id/messages", h.HandleSendMessage)
	g.POST("/quests/:id/complete", h.HandleComplete)
	g.POST("/quests/:id/reopen", h.HandleReopen)
	g.PUT("/quests/:id/model", h.HandleUpdateModel)
	g.GET("/quests/:id/tasks", h.HandleGetTasks)
	g.GET("/quests/:id/preflight", h.HandleGetPreflight)
}

// ensureDefaultProject creates the default project if it doesn't exist.
func (h *Handler) ensureDefaultProject(projectID string) error {
	if projectID != "proj_default" {
		return nil
	}

	project, err := h.deps.DB.GetProjectByID(projectID)
	if err != nil {
		return err
	}
	if project == nil {
		_, err = h.deps.DB.CreateProjectWithID(projectID, "Default Project", ".")
		if err != nil {
			return err
		}
	}
	return nil
}

// HandleList returns all quests for a project.
// GET /api/v1/projects/:id/quests
func (h *Handler) HandleList(c echo.Context) error {
	projectID := c.Param("id")

	if err := h.ensureDefaultProject(projectID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	project, err := h.deps.DB.GetProjectByID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	quests, err := h.deps.DB.GetQuestsByProjectID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	response := make([]core.QuestResponse, 0, len(quests))
	for _, q := range quests {
		summary, _ := h.deps.DB.GetQuestSummary(q.ID)
		response = append(response, core.ToQuestResponse(q, summary))
	}

	return c.JSON(http.StatusOK, response)
}

// HandleCreate creates a new quest for a project.
// POST /api/v1/projects/:id/quests
func (h *Handler) HandleCreate(c echo.Context) error {
	projectID := c.Param("id")

	if err := h.ensureDefaultProject(projectID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	project, err := h.deps.DB.GetProjectByID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	model := req.Model
	if model == "" {
		model = db.QuestModelSonnet
	}
	if model != db.QuestModelSonnet && model != db.QuestModelOpus {
		return echo.NewHTTPError(http.StatusBadRequest, "model must be 'sonnet' or 'opus'")
	}

	quest, err := h.deps.DB.CreateQuest(projectID, model)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestCreated, quest.ID, map[string]any{
			"project_id": projectID,
		})
	}

	return c.JSON(http.StatusCreated, core.ToQuestResponse(quest, nil))
}

// HandleGet returns a quest by ID with its messages.
// GET /api/v1/quests/:id
func (h *Handler) HandleGet(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	messages, err := h.deps.DB.GetQuestMessages(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	summary, _ := h.deps.DB.GetQuestSummary(questID)

	msgResponses := make([]core.QuestMessageResponse, 0, len(messages))
	for _, m := range messages {
		msgResponses = append(msgResponses, core.ToQuestMessageResponse(m))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"quest":    core.ToQuestResponse(quest, summary),
		"messages": msgResponses,
	})
}

// HandleDelete deletes a quest and all its messages.
// DELETE /api/v1/quests/:id
func (h *Handler) HandleDelete(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	tasks, err := h.deps.DB.GetTasksByQuestID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(tasks) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete quest with existing tasks")
	}

	if err := h.deps.DB.DeleteQuest(questID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestDeleted, questID, map[string]any{
			"project_id": quest.ProjectID,
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "quest deleted",
	})
}

// HandleSendMessage adds a user message to a quest and gets Dex's response.
// POST /api/v1/quests/:id/messages
func (h *Handler) HandleSendMessage(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if quest.Status != db.QuestStatusActive {
		return echo.NewHTTPError(http.StatusBadRequest, "quest is not active")
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Content) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "content is required")
	}

	// Create user message
	userMsg, err := h.deps.DB.CreateQuestMessage(questID, "user", req.Content)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestMessage, questID, map[string]any{
			"message": core.ToQuestMessageResponse(userMsg),
		})
	}

	if h.deps.QuestHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured (missing Anthropic API key)")
	}

	assistantMsg, err := h.deps.QuestHandler.ProcessMessage(c.Request().Context(), questID, req.Content)
	if err != nil {
		// Check if this is a billing/credit error from Anthropic
		var apiErr *toolbelt.AnthropicAPIError
		if errors.As(err, &apiErr) && apiErr.IsBillingError() {
			return c.JSON(http.StatusPaymentRequired, map[string]any{
				"error":        "billing_error",
				"message":      "Your Anthropic API credit balance is too low. Please add credits at console.anthropic.com and try again.",
				"retryable":    true,
				"user_message": core.ToQuestMessageResponse(userMsg),
			})
		}
		// Check for rate limit errors
		if errors.As(err, &apiErr) && apiErr.IsRateLimitError() {
			return c.JSON(http.StatusTooManyRequests, map[string]any{
				"error":        "rate_limit",
				"message":      "Rate limit exceeded. Please wait a moment and try again.",
				"retryable":    true,
				"user_message": core.ToQuestMessageResponse(userMsg),
			})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to process message: %v", err))
	}

	// Sync to GitHub Issue (async)
	if h.SyncQuestToGitHubIssue != nil {
		go h.SyncQuestToGitHubIssue(questID)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"user_message":      core.ToQuestMessageResponse(userMsg),
		"assistant_message": core.ToQuestMessageResponse(assistantMsg),
	})
}

// HandleComplete marks a quest as completed.
// POST /api/v1/quests/:id/complete
func (h *Handler) HandleComplete(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if quest.Status == db.QuestStatusCompleted {
		return echo.NewHTTPError(http.StatusBadRequest, "quest is already completed")
	}

	if err := h.deps.DB.CompleteQuest(questID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	quest, _ = h.deps.DB.GetQuestByID(questID)
	summary, _ := h.deps.DB.GetQuestSummary(questID)

	if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestCompleted, questID, map[string]any{
			"project_id": quest.ProjectID,
		})
	}

	// Close GitHub Issue (async)
	if h.CloseQuestGitHubIssue != nil {
		go h.CloseQuestGitHubIssue(questID, summary)
	}

	return c.JSON(http.StatusOK, core.ToQuestResponse(quest, summary))
}

// HandleReopen reopens a completed quest.
// POST /api/v1/quests/:id/reopen
func (h *Handler) HandleReopen(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if quest.Status != db.QuestStatusCompleted {
		return echo.NewHTTPError(http.StatusBadRequest, "quest is not completed")
	}

	if err := h.deps.DB.ReopenQuest(questID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	quest, _ = h.deps.DB.GetQuestByID(questID)
	summary, _ := h.deps.DB.GetQuestSummary(questID)

	if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestReopened, questID, map[string]any{
			"project_id": quest.ProjectID,
		})
	}

	// Reopen GitHub Issue (async)
	if h.ReopenQuestGitHubIssue != nil {
		go h.ReopenQuestGitHubIssue(questID)
	}

	return c.JSON(http.StatusOK, core.ToQuestResponse(quest, summary))
}

// HandleUpdateModel updates the model for a quest.
// PUT /api/v1/quests/:id/model
func (h *Handler) HandleUpdateModel(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Model != db.QuestModelSonnet && req.Model != db.QuestModelOpus {
		return echo.NewHTTPError(http.StatusBadRequest, "model must be 'sonnet' or 'opus'")
	}

	if err := h.deps.DB.UpdateQuestModel(questID, req.Model); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	quest, _ = h.deps.DB.GetQuestByID(questID)
	summary, _ := h.deps.DB.GetQuestSummary(questID)

	if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestUpdated, questID, map[string]any{
			"model": req.Model,
		})
	}

	return c.JSON(http.StatusOK, core.ToQuestResponse(quest, summary))
}

// HandleGetTasks returns all tasks spawned by a quest.
// GET /api/v1/quests/:id/tasks
func (h *Handler) HandleGetTasks(c echo.Context) error {
	questID := c.Param("id")

	quest, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if quest == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	tasks, err := h.deps.DB.GetTasksByQuestID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	response := make([]core.TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		resp := core.ToTaskResponse(t)
		// Compute tokens from activity (single source of truth)
		if inputTokens, outputTokens, err := h.deps.DB.GetTaskTokensFromActivity(t.ID); err == nil {
			resp.SetTokensFromActivity(inputTokens, outputTokens)
		}
		response = append(response, resp)
	}

	return c.JSON(http.StatusOK, response)
}

// HandleGetPreflight returns pre-flight check results for a quest's project.
// GET /api/v1/quests/:id/preflight
func (h *Handler) HandleGetPreflight(c echo.Context) error {
	questID := c.Param("id")

	if h.deps.QuestHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured")
	}

	check, err := h.deps.QuestHandler.GetPreflightCheck(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, check)
}
