// Package memory provides HTTP handlers for memory operations.
package memory

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/security"
)

// Handler handles memory-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new memory handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all memory routes on the given group.
// All routes require authentication.
//   - GET /projects/:id/memories
//   - POST /projects/:id/memories
//   - GET /projects/:id/memories/search
//   - POST /projects/:id/memories/cleanup
//   - GET /memories/:id
//   - PUT /memories/:id
//   - DELETE /memories/:id
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/projects/:id/memories", h.HandleList)
	g.POST("/projects/:id/memories", h.HandleCreate)
	g.GET("/projects/:id/memories/search", h.HandleSearch)
	g.POST("/projects/:id/memories/cleanup", h.HandleCleanup)
	g.GET("/memories/:id", h.HandleGet)
	g.PUT("/memories/:id", h.HandleUpdate)
	g.DELETE("/memories/:id", h.HandleDelete)
}

// MemoryRequest is the request body for creating/updating memories.
type MemoryRequest struct {
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	FileRefs []string `json:"file_refs,omitempty"`
}

// MemoryResponse is the response format for memories.
type MemoryResponse struct {
	ID                 string   `json:"id"`
	ProjectID          string   `json:"project_id"`
	Type               string   `json:"type"`
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	Confidence         float64  `json:"confidence"`
	Tags               []string `json:"tags,omitempty"`
	FileRefs           []string `json:"file_refs,omitempty"`
	CreatedByHat       string   `json:"created_by_hat,omitempty"`
	CreatedByTaskID    string   `json:"created_by_task_id,omitempty"`
	CreatedBySessionID string   `json:"created_by_session_id,omitempty"`
	Source             string   `json:"source"`
	CreatedAt          string   `json:"created_at"`
	LastUsedAt         string   `json:"last_used_at,omitempty"`
	UseCount           int      `json:"use_count"`
}

// toResponse converts a db.Memory to MemoryResponse.
func toResponse(m *db.Memory) MemoryResponse {
	resp := MemoryResponse{
		ID:           m.ID,
		ProjectID:    m.ProjectID,
		Type:         string(m.Type),
		Title:        m.Title,
		Content:      m.Content,
		Confidence:   m.Confidence,
		Tags:         m.Tags,
		FileRefs:     m.FileRefs,
		CreatedByHat: m.CreatedByHat,
		Source:       string(m.Source),
		CreatedAt:    m.CreatedAt.Format(time.RFC3339),
		UseCount:     m.UseCount,
	}

	if m.CreatedByTaskID.Valid {
		resp.CreatedByTaskID = m.CreatedByTaskID.String
	}
	if m.CreatedBySessionID.Valid {
		resp.CreatedBySessionID = m.CreatedBySessionID.String
	}
	if m.LastUsedAt.Valid {
		resp.LastUsedAt = m.LastUsedAt.Time.Format(time.RFC3339)
	}

	return resp
}

// HandleList returns all memories for a project.
// GET /api/v1/projects/:id/memories?type=pattern&min_confidence=0.5
func (h *Handler) HandleList(c echo.Context) error {
	projectID := c.Param("id")

	// Parse optional filters
	var memType *db.MemoryType
	if t := c.QueryParam("type"); t != "" {
		if db.IsValidMemoryType(t) {
			mt := db.MemoryType(t)
			memType = &mt
		}
	}

	minConfidence := 0.0
	if mc := c.QueryParam("min_confidence"); mc != "" {
		if f, err := strconv.ParseFloat(mc, 64); err == nil {
			minConfidence = f
		}
	}

	memories, err := h.deps.DB.ListMemories(projectID, memType, minConfidence)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to list memories",
		})
	}

	responses := make([]MemoryResponse, len(memories))
	for i, m := range memories {
		responses[i] = toResponse(&m)
	}

	return c.JSON(http.StatusOK, responses)
}

// HandleCreate creates a new memory for a project.
// POST /api/v1/projects/:id/memories
func (h *Handler) HandleCreate(c echo.Context) error {
	projectID := c.Param("id")

	var req MemoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Title == "" || req.Content == "" || req.Type == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Title, content, and type are required",
		})
	}

	if !db.IsValidMemoryType(req.Type) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid memory type",
		})
	}

	// Sanitize user input to prevent unicode-based prompt injection
	sanitizedTitle := security.SanitizeForPrompt(req.Title)
	sanitizedContent := security.SanitizeForPrompt(req.Content)

	memory := &db.Memory{
		ID:         uuid.New().String(),
		ProjectID:  projectID,
		Type:       db.MemoryType(req.Type),
		Title:      sanitizedTitle,
		Content:    sanitizedContent,
		Tags:       req.Tags,
		FileRefs:   req.FileRefs,
		Confidence: db.InitialConfidenceManual,
		Source:     db.SourceManual,
		CreatedAt:  time.Now(),
	}

	if err := h.deps.DB.CreateMemory(memory); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create memory",
		})
	}

	return c.JSON(http.StatusCreated, toResponse(memory))
}

// HandleGet returns a single memory by ID.
// GET /api/v1/memories/:id
func (h *Handler) HandleGet(c echo.Context) error {
	memoryID := c.Param("id")

	memory, err := h.deps.DB.GetMemory(memoryID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Memory not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get memory",
		})
	}

	return c.JSON(http.StatusOK, toResponse(memory))
}

// HandleUpdate updates an existing memory.
// PUT /api/v1/memories/:id
func (h *Handler) HandleUpdate(c echo.Context) error {
	memoryID := c.Param("id")

	// Get existing memory
	memory, err := h.deps.DB.GetMemory(memoryID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Memory not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get memory",
		})
	}

	var req MemoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Update fields if provided (sanitize user input)
	if req.Title != "" {
		memory.Title = security.SanitizeForPrompt(req.Title)
	}
	if req.Content != "" {
		memory.Content = security.SanitizeForPrompt(req.Content)
	}
	if req.Type != "" && db.IsValidMemoryType(req.Type) {
		memory.Type = db.MemoryType(req.Type)
	}
	if req.Tags != nil {
		memory.Tags = req.Tags
	}
	if req.FileRefs != nil {
		memory.FileRefs = req.FileRefs
	}

	if err := h.deps.DB.UpdateMemory(memory); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update memory",
		})
	}

	return c.JSON(http.StatusOK, toResponse(memory))
}

// HandleDelete deletes a memory.
// DELETE /api/v1/memories/:id
func (h *Handler) HandleDelete(c echo.Context) error {
	memoryID := c.Param("id")

	if err := h.deps.DB.DeleteMemory(memoryID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete memory",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleSearch searches memories by query.
// GET /api/v1/projects/:id/memories/search?q=query&after_date=...&before_date=...&limit=50
func (h *Handler) HandleSearch(c echo.Context) error {
	projectID := c.Param("id")
	query := c.QueryParam("q")

	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Query parameter 'q' is required",
		})
	}

	params := db.MemorySearchParams{
		Query: query,
		Limit: 50,
	}

	// Parse optional date filters
	if after := c.QueryParam("after_date"); after != "" {
		if t, err := time.Parse(time.RFC3339, after); err == nil {
			params.AfterDate = &t
		} else if t, err := time.Parse("2006-01-02", after); err == nil {
			params.AfterDate = &t
		}
	}

	if before := c.QueryParam("before_date"); before != "" {
		if t, err := time.Parse(time.RFC3339, before); err == nil {
			params.BeforeDate = &t
		} else if t, err := time.Parse("2006-01-02", before); err == nil {
			params.BeforeDate = &t
		}
	}

	// Parse limit
	if limit := c.QueryParam("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 && l <= 100 {
			params.Limit = l
		}
	}

	memories, err := h.deps.DB.SearchMemories(projectID, params)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to search memories",
		})
	}

	responses := make([]MemoryResponse, len(memories))
	for i, m := range memories {
		responses[i] = toResponse(&m)
	}

	return c.JSON(http.StatusOK, responses)
}

// HandleCleanup runs cleanup on project memories.
// POST /api/v1/projects/:id/memories/cleanup
func (h *Handler) HandleCleanup(c echo.Context) error {
	if err := h.deps.DB.CleanupMemories(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to cleanup memories",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "cleanup completed",
	})
}
