// Package mesh provides HTTP handlers for mesh network status and peer information.
package mesh

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/mesh"
)

// Handler handles mesh-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new mesh handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers the mesh routes on the given group.
func (h *Handler) RegisterRoutes(g *echo.Group) {
	mesh := g.Group("/mesh")
	mesh.GET("/status", h.GetStatus)
	mesh.GET("/peers", h.GetPeers)
}

// GetStatus returns the current mesh connection status.
// GET /api/v1/mesh/status
func (h *Handler) GetStatus(c echo.Context) error {
	if h.deps.MeshClient == nil {
		return c.JSON(http.StatusOK, mesh.Status{
			Connected: false,
		})
	}

	status := h.deps.MeshClient.Status()
	return c.JSON(http.StatusOK, status)
}

// GetPeers returns the list of peers on the Campus network.
// GET /api/v1/mesh/peers
func (h *Handler) GetPeers(c echo.Context) error {
	if h.deps.MeshClient == nil {
		return c.JSON(http.StatusOK, []mesh.Peer{})
	}

	peers := h.deps.MeshClient.Peers()
	if peers == nil {
		peers = []mesh.Peer{}
	}

	return c.JSON(http.StatusOK, peers)
}

// PeersResponse wraps the peers list for API responses.
type PeersResponse struct {
	Peers []mesh.Peer `json:"peers"`
	Count int         `json:"count"`
}

// GetPeersWithCount returns peers with a count field.
// This is an alternative endpoint that provides metadata.
func (h *Handler) GetPeersWithCount(c echo.Context) error {
	if h.deps.MeshClient == nil {
		return c.JSON(http.StatusOK, PeersResponse{
			Peers: []mesh.Peer{},
			Count: 0,
		})
	}

	peers := h.deps.MeshClient.Peers()
	if peers == nil {
		peers = []mesh.Peer{}
	}

	return c.JSON(http.StatusOK, PeersResponse{
		Peers: peers,
		Count: len(peers),
	})
}
