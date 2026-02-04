package quests

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/quest"
	"github.com/lirancohen/dex/internal/realtime"
	"github.com/lirancohen/dex/internal/security"
)

// ObjectivesHandler handles objective-related HTTP requests.
type ObjectivesHandler struct {
	deps *core.Deps

	// GitHub sync callback
	SyncObjectiveToGitHubIssue func(taskID string)
}

// NewObjectivesHandler creates a new objectives handler.
func NewObjectivesHandler(deps *core.Deps) *ObjectivesHandler {
	return &ObjectivesHandler{deps: deps}
}

// RegisterRoutes registers all objective routes on the given group.
// All routes require authentication.
//   - POST /quests/:id/objectives
//   - POST /quests/:id/objectives/batch
func (h *ObjectivesHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/quests/:id/objectives", h.HandleCreate)
	g.POST("/quests/:id/objectives/batch", h.HandleCreateBatch)
}

// HandleCreate creates a task from an accepted objective draft.
// POST /api/v1/quests/:id/objectives
func (h *ObjectivesHandler) HandleCreate(c echo.Context) error {
	questID := c.Param("id")

	questObj, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if questObj == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if h.deps.QuestHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured")
	}

	var req struct {
		DraftID          string   `json:"draft_id"`
		Title            string   `json:"title"`
		Description      string   `json:"description"`
		Hat              string   `json:"hat"`
		MustHave         []string `json:"must_have"`
		Optional         []string `json:"optional"`
		SelectedOptional []int    `json:"selected_optional"`
		AutoStart        bool     `json:"auto_start"`
		BlockedBy        []string `json:"blocked_by"`
		// Repository targeting
		GitHubOwner string `json:"github_owner"`
		GitHubRepo  string `json:"github_repo"`
		CloneURL    string `json:"clone_url"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	// Sanitize user input
	sanitizedTitle := security.SanitizeForPrompt(req.Title)
	sanitizedDescription := security.SanitizeForPrompt(req.Description)

	draft := quest.ObjectiveDraft{
		DraftID:     req.DraftID,
		Title:       sanitizedTitle,
		Description: sanitizedDescription,
		Hat:         req.Hat,
		Checklist: quest.Checklist{
			MustHave: req.MustHave,
			Optional: req.Optional,
		},
		AutoStart:   req.AutoStart,
		GitHubOwner: req.GitHubOwner,
		GitHubRepo:  req.GitHubRepo,
		CloneURL:    req.CloneURL,
	}

	createdTask, err := h.deps.QuestHandler.CreateObjectiveFromDraft(c.Request().Context(), questID, draft, req.SelectedOptional)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Wire up dependencies
	// Note: We don't change status to 'blocked' - blocked state is derived from dependencies
	var blockerIDs []string
	if len(req.BlockedBy) > 0 {
		for _, blockerID := range req.BlockedBy {
			if err := h.deps.DB.AddTaskDependency(blockerID, createdTask.ID); err != nil {
				fmt.Printf("warning: failed to add dependency %s -> %s: %v\n", blockerID, createdTask.ID, err)
			}
		}
		// Get the list of incomplete blockers for the response
		blockerIDs, _ = h.deps.DB.GetIncompleteBlockerIDs(createdTask.ID)
	}

	// Sync to GitHub Issue (async)
	if h.SyncObjectiveToGitHubIssue != nil {
		go h.SyncObjectiveToGitHubIssue(createdTask.ID)
	}

	// Add message to quest history
	acceptMessage := fmt.Sprintf("✓ Accepted objective: **%s**", sanitizedTitle)
	if msg, err := h.deps.DB.CreateQuestMessage(questID, "user", acceptMessage); err != nil {
		fmt.Printf("warning: failed to add accept message to quest: %v\n", err)
	} else if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestMessage, questID, map[string]any{
			"message": msg,
		})
	}

	response := map[string]any{
		"message": "objective created",
		"task":    core.ToTaskResponseWithBlocking(createdTask, blockerIDs),
	}

	// Auto-start if requested and not blocked (derived from dependencies)
	isBlocked := len(blockerIDs) > 0
	if req.AutoStart && !isBlocked {
		startResult, err := h.deps.StartTaskInternal(context.Background(), createdTask.ID, "")
		if err != nil {
			response["auto_start_error"] = err.Error()
			fmt.Printf("auto-start failed for task %s: %v\n", createdTask.ID, err)
		} else {
			// Get fresh blocking info for the started task
			startedBlockerIDs, _ := h.deps.DB.GetIncompleteBlockerIDs(startResult.Task.ID)
			response["task"] = core.ToTaskResponseWithBlocking(startResult.Task, startedBlockerIDs)
			response["worktree_path"] = startResult.WorktreePath
			response["session_id"] = startResult.SessionID
			response["auto_started"] = true
		}
	}

	return c.JSON(http.StatusCreated, response)
}

// HandleCreateBatch creates multiple tasks from accepted objective drafts atomically.
// POST /api/v1/quests/:id/objectives/batch
func (h *ObjectivesHandler) HandleCreateBatch(c echo.Context) error {
	questID := c.Param("id")

	questObj, err := h.deps.DB.GetQuestByID(questID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if questObj == nil {
		return echo.NewHTTPError(http.StatusNotFound, "quest not found")
	}

	if h.deps.QuestHandler == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "quest handler not configured")
	}

	var req struct {
		Drafts []struct {
			DraftID             string   `json:"draft_id"`
			Title               string   `json:"title"`
			Description         string   `json:"description"`
			Hat                 string   `json:"hat"`
			MustHave            []string `json:"must_have"`
			Optional            []string `json:"optional"`
			SelectedOptional    []int    `json:"selected_optional"`
			AutoStart           bool     `json:"auto_start"`
			BlockedBy           []string `json:"blocked_by"`
			Complexity          string   `json:"complexity,omitempty"`
			EstimatedIterations int      `json:"estimated_iterations,omitempty"`
			// Repository targeting
			GitHubOwner string `json:"github_owner"`
			GitHubRepo  string `json:"github_repo"`
			CloneURL    string `json:"clone_url"`
		} `json:"drafts"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if len(req.Drafts) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one draft is required")
	}

	// Phase 1: Create all tasks and build draft_id -> task_id mapping
	draftToTaskID := make(map[string]string)
	var createdTasks []*db.Task
	var taskResults []map[string]any

	for _, draft := range req.Drafts {
		if draft.Title == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "title is required for all drafts")
		}

		sanitizedTitle := security.SanitizeForPrompt(draft.Title)
		sanitizedDescription := security.SanitizeForPrompt(draft.Description)

		questDraft := quest.ObjectiveDraft{
			DraftID:             draft.DraftID,
			Title:               sanitizedTitle,
			Description:         sanitizedDescription,
			Hat:                 draft.Hat,
			Checklist:           quest.Checklist{MustHave: draft.MustHave, Optional: draft.Optional},
			AutoStart:           draft.AutoStart,
			Complexity:          draft.Complexity,
			EstimatedIterations: draft.EstimatedIterations,
			GitHubOwner:         draft.GitHubOwner,
			GitHubRepo:          draft.GitHubRepo,
			CloneURL:            draft.CloneURL,
		}

		task, err := h.deps.QuestHandler.CreateObjectiveFromDraft(c.Request().Context(), questID, questDraft, draft.SelectedOptional)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create objective '%s': %v", sanitizedTitle, err))
		}

		draftToTaskID[draft.DraftID] = task.ID
		createdTasks = append(createdTasks, task)
		// Initial response without blocking info (will be updated after dependencies are wired)
		taskResults = append(taskResults, map[string]any{
			"draft_id": draft.DraftID,
			"task":     core.ToTaskResponse(task),
		})
	}

	// Phase 2: Wire up dependencies (don't change status - blocked is derived)
	for i, draft := range req.Drafts {
		if len(draft.BlockedBy) == 0 {
			continue
		}

		task := createdTasks[i]

		for _, blockerDraftID := range draft.BlockedBy {
			blockerTaskID, ok := draftToTaskID[blockerDraftID]
			if !ok {
				blockerTaskID = blockerDraftID
			}

			if err := h.deps.DB.AddTaskDependency(blockerTaskID, task.ID); err != nil {
				fmt.Printf("warning: failed to add dependency %s -> %s: %v\n", blockerTaskID, task.ID, err)
			}
		}

		// Update response with derived blocking info
		blockerIDs, _ := h.deps.DB.GetIncompleteBlockerIDs(task.ID)
		taskResults[i]["task"] = core.ToTaskResponseWithBlocking(task, blockerIDs)
	}

	// Phase 3: Sync to GitHub and auto-start tasks that are not blocked
	var autoStarted []string
	var autoStartErrors []string

	for i, task := range createdTasks {
		// Sync to GitHub Issue (async)
		if h.SyncObjectiveToGitHubIssue != nil {
			go h.SyncObjectiveToGitHubIssue(task.ID)
		}

		// Auto-start if requested and not blocked (derived from dependencies)
		if req.Drafts[i].AutoStart {
			blockerIDs, _ := h.deps.DB.GetIncompleteBlockerIDs(task.ID)
			isBlocked := len(blockerIDs) > 0
			if !isBlocked {
				startResult, err := h.deps.StartTaskInternal(context.Background(), task.ID, "")
				if err != nil {
					autoStartErrors = append(autoStartErrors, fmt.Sprintf("%s: %v", task.Title, err))
					fmt.Printf("auto-start failed for task %s: %v\n", task.ID, err)
				} else {
					startedBlockerIDs, _ := h.deps.DB.GetIncompleteBlockerIDs(startResult.Task.ID)
					taskResults[i]["task"] = core.ToTaskResponseWithBlocking(startResult.Task, startedBlockerIDs)
					taskResults[i]["worktree_path"] = startResult.WorktreePath
					taskResults[i]["session_id"] = startResult.SessionID
					taskResults[i]["auto_started"] = true
					autoStarted = append(autoStarted, task.ID)
				}
			}
		}
	}

	// Add message to quest history
	acceptMessage := fmt.Sprintf("✓ Accepted %d objectives in batch", len(createdTasks))
	if msg, err := h.deps.DB.CreateQuestMessage(questID, "user", acceptMessage); err != nil {
		fmt.Printf("warning: failed to add accept message to quest: %v\n", err)
	} else if h.deps.Broadcaster != nil {
		h.deps.Broadcaster.PublishQuestEvent(realtime.EventQuestMessage, questID, map[string]any{
			"message": msg,
		})
	}

	response := map[string]any{
		"message":       fmt.Sprintf("created %d objectives", len(createdTasks)),
		"tasks":         taskResults,
		"draft_mapping": draftToTaskID,
	}

	if len(autoStarted) > 0 {
		response["auto_started"] = autoStarted
	}
	if len(autoStartErrors) > 0 {
		response["auto_start_errors"] = autoStartErrors
	}

	return c.JSON(http.StatusCreated, response)
}
