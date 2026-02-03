// Package toolbelt provides HTTP handlers for toolbelt service operations.
package toolbelt

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/middleware"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// Handler handles toolbelt-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new toolbelt handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all toolbelt routes on the given group.
// Public routes (no auth required):
//   - GET /toolbelt/status
//   - POST /toolbelt/test
//
// Protected routes (auth required):
//   - GET /me
func (h *Handler) RegisterPublicRoutes(g *echo.Group) {
	g.GET("/toolbelt/status", h.HandleStatus)
	g.POST("/toolbelt/test", h.HandleTest)
}

// RegisterProtectedRoutes registers protected toolbelt routes.
func (h *Handler) RegisterProtectedRoutes(g *echo.Group) {
	g.GET("/me", h.HandleMe)
}

// HandleStatus returns the configuration status of all toolbelt services.
// GET /api/v1/toolbelt/status
func (h *Handler) HandleStatus(c echo.Context) error {
	tb := h.deps.GetToolbelt()
	if tb == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"configured": false,
			"services":   []toolbelt.ServiceStatus{},
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"configured": true,
		"services":   tb.Status(),
	})
}

// HandleTest tests all configured toolbelt service connections.
// POST /api/v1/toolbelt/test
func (h *Handler) HandleTest(c echo.Context) error {
	tb := h.deps.GetToolbelt()
	if tb == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"tested":  false,
			"message": "toolbelt not configured",
			"results": []toolbelt.TestResult{},
		})
	}

	results := tb.TestConnections(c.Request().Context())

	// Count successes
	successes := 0
	for _, r := range results {
		if r.Success {
			successes++
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"tested":     true,
		"total":      len(results),
		"successful": successes,
		"failed":     len(results) - successes,
		"results":    results,
	})
}

// HandleMe returns the authenticated user info.
// GET /api/v1/me
func (h *Handler) HandleMe(c echo.Context) error {
	userID := middleware.GetUserID(c)

	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get user")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":            user.ID,
		"created_at":    user.CreatedAt,
		"last_login_at": user.LastLoginAt,
	})
}
