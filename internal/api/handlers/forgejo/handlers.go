// Package forgejo provides HTTP handlers for Forgejo instance management.
package forgejo

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
)

// Handler handles Forgejo-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new Forgejo handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers the Forgejo routes on the given group.
func (h *Handler) RegisterRoutes(g *echo.Group) {
	fg := g.Group("/forgejo")
	fg.GET("/access", h.GetAccess)
	fg.GET("/status", h.GetStatus)
}

// GetAccess returns the Forgejo web UI URL and login credentials.
// GET /api/v1/forgejo/access
func (h *Handler) GetAccess(c echo.Context) error {
	mgr := h.deps.ForgejoManager
	if mgr == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Forgejo is not configured",
		})
	}
	if !mgr.IsRunning() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Forgejo is not running",
		})
	}

	access, err := mgr.WebAccess()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, access)
}

// GetStatus returns whether Forgejo is configured and running.
// GET /api/v1/forgejo/status
func (h *Handler) GetStatus(c echo.Context) error {
	mgr := h.deps.ForgejoManager
	if mgr == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"configured": false,
			"running":    false,
		})
	}

	resp := map[string]any{
		"configured": true,
		"running":    mgr.IsRunning(),
	}
	if mgr.IsRunning() {
		resp["url"] = mgr.BaseURL()
	}

	return c.JSON(http.StatusOK, resp)
}
