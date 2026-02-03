package quests

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
)

// TemplatesHandler handles quest template-related HTTP requests.
type TemplatesHandler struct {
	deps *core.Deps
}

// NewTemplatesHandler creates a new templates handler.
func NewTemplatesHandler(deps *core.Deps) *TemplatesHandler {
	return &TemplatesHandler{deps: deps}
}

// RegisterRoutes registers all quest template routes on the given group.
// All routes require authentication.
//   - GET /projects/:id/quest-templates
//   - POST /projects/:id/quest-templates
//   - GET /quest-templates/:id
//   - PUT /quest-templates/:id
//   - DELETE /quest-templates/:id
func (h *TemplatesHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/projects/:id/quest-templates", h.HandleList)
	g.POST("/projects/:id/quest-templates", h.HandleCreate)
	g.GET("/quest-templates/:id", h.HandleGet)
	g.PUT("/quest-templates/:id", h.HandleUpdate)
	g.DELETE("/quest-templates/:id", h.HandleDelete)
}

// HandleList returns all quest templates for a project.
// GET /api/v1/projects/:id/quest-templates
func (h *TemplatesHandler) HandleList(c echo.Context) error {
	projectID := c.Param("id")

	templates, err := h.deps.DB.GetQuestTemplatesByProjectID(projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	result := make([]map[string]any, len(templates))
	for i, t := range templates {
		result[i] = map[string]any{
			"id":             t.ID,
			"project_id":     t.ProjectID,
			"name":           t.Name,
			"description":    t.Description.String,
			"initial_prompt": t.InitialPrompt,
			"created_at":     t.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// HandleCreate creates a new quest template.
// POST /api/v1/projects/:id/quest-templates
func (h *TemplatesHandler) HandleCreate(c echo.Context) error {
	projectID := c.Param("id")

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		InitialPrompt string `json:"initial_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.InitialPrompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "initial_prompt is required")
	}

	template, err := h.deps.DB.CreateQuestTemplate(projectID, req.Name, req.Description, req.InitialPrompt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"id":             template.ID,
		"project_id":     template.ProjectID,
		"name":           template.Name,
		"description":    template.Description.String,
		"initial_prompt": template.InitialPrompt,
		"created_at":     template.CreatedAt,
	})
}

// HandleGet returns a quest template by ID.
// GET /api/v1/quest-templates/:id
func (h *TemplatesHandler) HandleGet(c echo.Context) error {
	templateID := c.Param("id")

	template, err := h.deps.DB.GetQuestTemplateByID(templateID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if template == nil {
		return echo.NewHTTPError(http.StatusNotFound, "template not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":             template.ID,
		"project_id":     template.ProjectID,
		"name":           template.Name,
		"description":    template.Description.String,
		"initial_prompt": template.InitialPrompt,
		"created_at":     template.CreatedAt,
	})
}

// HandleUpdate updates a quest template.
// PUT /api/v1/quest-templates/:id
func (h *TemplatesHandler) HandleUpdate(c echo.Context) error {
	templateID := c.Param("id")

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		InitialPrompt string `json:"initial_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.InitialPrompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "initial_prompt is required")
	}

	err := h.deps.DB.UpdateQuestTemplate(templateID, req.Name, req.Description, req.InitialPrompt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	template, _ := h.deps.DB.GetQuestTemplateByID(templateID)
	return c.JSON(http.StatusOK, map[string]any{
		"id":             template.ID,
		"project_id":     template.ProjectID,
		"name":           template.Name,
		"description":    template.Description.String,
		"initial_prompt": template.InitialPrompt,
		"created_at":     template.CreatedAt,
	})
}

// HandleDelete deletes a quest template.
// DELETE /api/v1/quest-templates/:id
func (h *TemplatesHandler) HandleDelete(c echo.Context) error {
	templateID := c.Param("id")

	err := h.deps.DB.DeleteQuestTemplate(templateID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "template deleted",
	})
}
