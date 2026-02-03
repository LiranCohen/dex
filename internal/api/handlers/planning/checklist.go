package planning

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/task"
)

// ChecklistHandler handles checklist-related HTTP requests.
type ChecklistHandler struct {
	deps *core.Deps
}

// NewChecklistHandler creates a new checklist handler.
func NewChecklistHandler(deps *core.Deps) *ChecklistHandler {
	return &ChecklistHandler{deps: deps}
}

// RegisterRoutes registers all checklist routes on the given group.
// All routes require authentication.
//   - GET /tasks/:id/checklist
//   - PUT /tasks/:id/checklist/items/:itemId
//   - POST /tasks/:id/checklist/accept
//   - POST /tasks/:id/remediate
func (h *ChecklistHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/tasks/:id/checklist", h.HandleGet)
	g.PUT("/tasks/:id/checklist/items/:itemId", h.HandleUpdateItem)
	g.POST("/tasks/:id/checklist/accept", h.HandleAccept)
	g.POST("/tasks/:id/remediate", h.HandleCreateRemediation)
}

// HandleGet returns the checklist and items for a task.
// GET /api/v1/tasks/:id/checklist
func (h *ChecklistHandler) HandleGet(c echo.Context) error {
	taskID := c.Param("id")

	checklist, err := h.deps.DB.GetChecklistByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if checklist == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no checklist for task")
	}

	items, err := h.deps.DB.GetChecklistItems(checklist.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Convert items to response format
	itemResponses := make([]core.ChecklistItemResponse, len(items))
	for i, item := range items {
		itemResponses[i] = core.ToChecklistItemResponse(item)
	}

	// Calculate summary
	totalCount := len(items)
	doneCount := 0
	failedCount := 0
	pendingCount := 0
	for _, item := range items {
		switch item.Status {
		case db.ChecklistItemStatusDone:
			doneCount++
		case db.ChecklistItemStatusFailed:
			failedCount++
		case db.ChecklistItemStatusPending, db.ChecklistItemStatusInProgress:
			pendingCount++
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"checklist": map[string]any{
			"id":         checklist.ID,
			"task_id":    checklist.TaskID,
			"created_at": checklist.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
		"items": itemResponses,
		"summary": map[string]any{
			"total":    totalCount,
			"done":     doneCount,
			"failed":   failedCount,
			"pending":  pendingCount,
			"all_done": doneCount == totalCount,
		},
	})
}

// HandleUpdateItem updates a checklist item status.
// PUT /api/v1/tasks/:id/checklist/items/:itemId
func (h *ChecklistHandler) HandleUpdateItem(c echo.Context) error {
	taskID := c.Param("id")
	itemID := c.Param("itemId")

	checklist, err := h.deps.DB.GetChecklistByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if checklist == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no checklist for task")
	}

	item, err := h.deps.DB.GetChecklistItem(itemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if item == nil || item.ChecklistID != checklist.ID {
		return echo.NewHTTPError(http.StatusNotFound, "checklist item not found")
	}

	var req struct {
		Status            *string `json:"status"`
		VerificationNotes *string `json:"verification_notes"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Status != nil {
		notes := ""
		if req.VerificationNotes != nil {
			notes = *req.VerificationNotes
		}
		if err := h.deps.DB.UpdateChecklistItemStatus(itemID, *req.Status, notes); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	updatedItem, err := h.deps.DB.GetChecklistItem(itemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.deps.Hub.Broadcast(websocket.Message{
		Type: "checklist.updated",
		Payload: map[string]any{
			"task_id":      taskID,
			"checklist_id": checklist.ID,
			"item":         core.ToChecklistItemResponse(updatedItem),
		},
	})

	return c.JSON(http.StatusOK, core.ToChecklistItemResponse(updatedItem))
}

// HandleAccept creates checklist items from pending checklist and transitions task to ready.
// POST /api/v1/tasks/:id/checklist/accept
func (h *ChecklistHandler) HandleAccept(c echo.Context) error {
	taskID := c.Param("id")

	planningSession, err := h.deps.DB.GetPlanningSessionByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if planningSession == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no planning session for task")
	}

	pendingChecklist := planningSession.GetPendingChecklist()
	if pendingChecklist == nil {
		// No checklist - just transition to ready
		if err := h.deps.TaskService.UpdateStatus(taskID, db.TaskStatusReady); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		h.deps.Hub.Broadcast(websocket.Message{
			Type: "task.updated",
			Payload: map[string]any{
				"task_id": taskID,
				"status":  db.TaskStatusReady,
			},
		})
		return c.JSON(http.StatusOK, map[string]any{
			"message": "plan accepted (no checklist)",
			"task_id": taskID,
		})
	}

	// Parse request for selected optional items
	var req struct {
		SelectedOptional []int `json:"selected_optional"`
	}
	c.Bind(&req) // Ignore error

	// Create a set of selected optional indices
	selectedOptionalSet := make(map[int]bool)
	for _, idx := range req.SelectedOptional {
		selectedOptionalSet[idx] = true
	}

	// Create the checklist
	checklist, err := h.deps.DB.CreateTaskChecklist(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sortOrder := 0

	// Add all must-have items
	for _, desc := range pendingChecklist.MustHave {
		if _, err := h.deps.DB.CreateChecklistItem(checklist.ID, desc, sortOrder); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		sortOrder++
	}

	// Add selected optional items
	for idx, desc := range pendingChecklist.Optional {
		if selectedOptionalSet[idx] {
			if _, err := h.deps.DB.CreateChecklistItem(checklist.ID, desc, sortOrder); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			sortOrder++
		}
	}

	// Transition task to ready
	if err := h.deps.TaskService.UpdateStatus(taskID, db.TaskStatusReady); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.deps.Hub.Broadcast(websocket.Message{
		Type: "task.updated",
		Payload: map[string]any{
			"task_id": taskID,
			"status":  db.TaskStatusReady,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"message":      "checklist accepted",
		"task_id":      taskID,
		"checklist_id": checklist.ID,
		"items_count":  sortOrder,
	})
}

// HandleCreateRemediation creates a new task to remediate failed checklist items.
// POST /api/v1/tasks/:id/remediate
func (h *ChecklistHandler) HandleCreateRemediation(c echo.Context) error {
	taskID := c.Param("id")

	originalTask, err := h.deps.TaskService.Get(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	checklist, err := h.deps.DB.GetChecklistByTaskID(taskID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if checklist == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no checklist for task")
	}

	issues, err := h.deps.DB.GetChecklistIssues(checklist.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(issues) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no issues to remediate")
	}

	// Build remediation description
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Remediation for task %s:\n\n", taskID))
	sb.WriteString("The following items need to be addressed:\n\n")
	for _, issue := range issues {
		sb.WriteString(fmt.Sprintf("- %s\n", issue.Description))
		if issue.Notes != "" {
			sb.WriteString(fmt.Sprintf("  Previous attempt failed: %q\n", issue.Notes))
		}
	}

	// Create the remediation task
	title := fmt.Sprintf("Fix: %s", originalTask.Title)
	newTask, err := h.deps.TaskService.Create(originalTask.ProjectID, title, originalTask.Type, originalTask.Priority)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Update description
	description := sb.String()
	updates := task.TaskUpdates{Description: &description}
	newTask, err = h.deps.TaskService.Update(newTask.ID, updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"message":          "remediation task created",
		"task":             core.ToTaskResponse(newTask),
		"original_task_id": taskID,
		"issues_count":     len(issues),
	})
}
