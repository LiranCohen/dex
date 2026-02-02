package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/security"
)

// MemoryRequest is the request body for creating/updating memories
type MemoryRequest struct {
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	FileRefs []string `json:"file_refs,omitempty"`
}

// MemoryResponse is the response format for memories
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

// memoryToResponse converts a db.Memory to MemoryResponse
func memoryToResponse(m *db.Memory) MemoryResponse {
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

// handleListMemories returns all memories for a project
func (s *Server) handleListMemories(c echo.Context) error {
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

	memories, err := s.db.ListMemories(projectID, memType, minConfidence)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to list memories",
		})
	}

	responses := make([]MemoryResponse, len(memories))
	for i, m := range memories {
		responses[i] = memoryToResponse(&m)
	}

	return c.JSON(http.StatusOK, responses)
}

// handleCreateMemory creates a new memory for a project
func (s *Server) handleCreateMemory(c echo.Context) error {
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

	if err := s.db.CreateMemory(memory); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create memory",
		})
	}

	return c.JSON(http.StatusCreated, memoryToResponse(memory))
}

// handleGetMemory returns a single memory by ID
func (s *Server) handleGetMemory(c echo.Context) error {
	memoryID := c.Param("id")

	memory, err := s.db.GetMemory(memoryID)
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

	return c.JSON(http.StatusOK, memoryToResponse(memory))
}

// handleUpdateMemory updates an existing memory
func (s *Server) handleUpdateMemory(c echo.Context) error {
	memoryID := c.Param("id")

	// Get existing memory
	memory, err := s.db.GetMemory(memoryID)
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

	if err := s.db.UpdateMemory(memory); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update memory",
		})
	}

	return c.JSON(http.StatusOK, memoryToResponse(memory))
}

// handleDeleteMemory deletes a memory
func (s *Server) handleDeleteMemory(c echo.Context) error {
	memoryID := c.Param("id")

	if err := s.db.DeleteMemory(memoryID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete memory",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// handleSearchMemories searches memories by query
func (s *Server) handleSearchMemories(c echo.Context) error {
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

	memories, err := s.db.SearchMemories(projectID, params)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to search memories",
		})
	}

	responses := make([]MemoryResponse, len(memories))
	for i, m := range memories {
		responses[i] = memoryToResponse(&m)
	}

	return c.JSON(http.StatusOK, responses)
}

// handleCleanupMemories runs cleanup on project memories
func (s *Server) handleCleanupMemories(c echo.Context) error {
	if err := s.db.CleanupMemories(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to cleanup memories",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "cleanup completed",
	})
}
