// Package devices provides HTTP handlers for device (client) management.
package devices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/mesh"
)

// Handler handles device-related HTTP requests.
type Handler struct {
	deps       *core.Deps
	namespace  string // From enrollment config
	tunnelToken string // For authenticating with Central
	centralURL string
}

// Config contains configuration for the devices handler.
type Config struct {
	Namespace   string
	TunnelToken string
	CentralURL  string
}

// New creates a new devices handler.
func New(deps *core.Deps, cfg Config) *Handler {
	return &Handler{
		deps:        deps,
		namespace:   cfg.Namespace,
		tunnelToken: cfg.TunnelToken,
		centralURL:  cfg.CentralURL,
	}
}

// RegisterRoutes registers the device routes on the given group.
func (h *Handler) RegisterRoutes(g *echo.Group) {
	devices := g.Group("/devices")
	devices.POST("/enrollment-key", h.CreateEnrollmentKey)
	devices.GET("", h.ListDevices)
	devices.DELETE("/:hostname", h.RemoveDevice)
}

// CreateEnrollmentKeyRequest is the request body for creating a client enrollment key.
type CreateEnrollmentKeyRequest struct {
	Hostname string `json:"hostname"` // Device hostname (optional, can be auto-detected by client)
}

// CreateEnrollmentKeyResponse is the response for creating a client enrollment key.
type CreateEnrollmentKeyResponse struct {
	Key            string `json:"key"`             // Enrollment key (dexkey-xxx)
	Hostname       string `json:"hostname"`        // Device hostname
	ExpiresAt      string `json:"expires_at"`      // Key expiration time
	InstallCommand string `json:"install_command"` // Full install command
}

// CentralEnrollmentKeyRequest is sent to Central to create an enrollment key.
type CentralEnrollmentKeyRequest struct {
	Hostname string `json:"hostname"`
	Type     string `json:"type"` // "client"
}

// CentralEnrollmentKeyResponse is returned by Central.
type CentralEnrollmentKeyResponse struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Hostname  string `json:"hostname"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ExpiresAt string `json:"expires_at"`
}

// CreateEnrollmentKey generates a client enrollment key via Central.
// POST /api/v1/devices/enrollment-key
func (h *Handler) CreateEnrollmentKey(c echo.Context) error {
	if h.centralURL == "" || h.tunnelToken == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Device enrollment not configured (missing Central connection)",
		})
	}

	var req CreateEnrollmentKeyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Default hostname if not provided
	hostname := strings.TrimSpace(strings.ToLower(req.Hostname))
	if hostname == "" {
		hostname = fmt.Sprintf("device-%d", time.Now().Unix()%10000)
	}

	// Call Central's enrollment key API
	centralReq := CentralEnrollmentKeyRequest{
		Hostname: hostname,
		Type:     "client",
	}

	reqBody, err := json.Marshal(centralReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create request",
		})
	}

	url := strings.TrimSuffix(h.centralURL, "/") + "/api/v1/enrollment-keys"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create request",
		})
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+h.tunnelToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "Failed to connect to Central: " + err.Error(),
		})
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to read Central response",
		})
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return c.JSON(resp.StatusCode, map[string]string{
			"error": "Central returned error: " + string(body),
		})
	}

	var centralResp CentralEnrollmentKeyResponse
	if err := json.Unmarshal(body, &centralResp); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to parse Central response",
		})
	}

	// Build install command
	installCommand := fmt.Sprintf("curl -fsSL https://get.enbox.id/client | sh -s -- %s", centralResp.Key)

	return c.JSON(http.StatusCreated, CreateEnrollmentKeyResponse{
		Key:            centralResp.Key,
		Hostname:       centralResp.Hostname,
		ExpiresAt:      centralResp.ExpiresAt,
		InstallCommand: installCommand,
	})
}

// DeviceInfo represents a device connected to the mesh network.
type DeviceInfo struct {
	Hostname string   `json:"hostname"`
	MeshIP   string   `json:"mesh_ip"`
	Online   bool     `json:"online"`
	Direct   bool     `json:"direct"`
	LastSeen string   `json:"last_seen,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	IsClient bool     `json:"is_client"` // True if this is a client device (tag:client)
}

// ListDevicesResponse is the response for listing devices.
type ListDevicesResponse struct {
	Devices []DeviceInfo `json:"devices"`
	Count   int          `json:"count"`
}

// ListDevices returns the list of client devices connected to the mesh.
// GET /api/v1/devices
func (h *Handler) ListDevices(c echo.Context) error {
	if h.deps.MeshClient == nil {
		return c.JSON(http.StatusOK, ListDevicesResponse{
			Devices: []DeviceInfo{},
			Count:   0,
		})
	}

	peers := h.deps.MeshClient.Peers()
	if peers == nil {
		return c.JSON(http.StatusOK, ListDevicesResponse{
			Devices: []DeviceInfo{},
			Count:   0,
		})
	}

	devices := make([]DeviceInfo, 0, len(peers))
	for _, p := range peers {
		// Check if this peer is a client (has tag:client)
		isClient := false
		for _, tag := range p.Tags {
			if tag == "tag:client" {
				isClient = true
				break
			}
		}

		devices = append(devices, DeviceInfo{
			Hostname: p.Hostname,
			MeshIP:   p.MeshIP,
			Online:   p.Online,
			Direct:   p.Direct,
			LastSeen: p.LastSeen,
			Tags:     p.Tags,
			IsClient: isClient,
		})
	}

	return c.JSON(http.StatusOK, ListDevicesResponse{
		Devices: devices,
		Count:   len(devices),
	})
}

// ListClientDevices returns only client devices (filtered by tag:client).
// This is a convenience method to show only enrolled client devices.
func (h *Handler) ListClientDevices(c echo.Context) error {
	if h.deps.MeshClient == nil {
		return c.JSON(http.StatusOK, ListDevicesResponse{
			Devices: []DeviceInfo{},
			Count:   0,
		})
	}

	peers := h.deps.MeshClient.Peers()
	if peers == nil {
		return c.JSON(http.StatusOK, ListDevicesResponse{
			Devices: []DeviceInfo{},
			Count:   0,
		})
	}

	var devices []DeviceInfo
	for _, p := range peers {
		// Only include clients
		isClient := hasTag(p.Tags, "tag:client")
		if !isClient {
			continue
		}

		devices = append(devices, DeviceInfo{
			Hostname: p.Hostname,
			MeshIP:   p.MeshIP,
			Online:   p.Online,
			Direct:   p.Direct,
			LastSeen: p.LastSeen,
			Tags:     p.Tags,
			IsClient: true,
		})
	}

	return c.JSON(http.StatusOK, ListDevicesResponse{
		Devices: devices,
		Count:   len(devices),
	})
}

// RemoveDevice removes a device from the mesh network.
// DELETE /api/v1/devices/:hostname
func (h *Handler) RemoveDevice(c echo.Context) error {
	hostname := c.Param("hostname")
	if hostname == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Hostname is required",
		})
	}

	if h.centralURL == "" || h.tunnelToken == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Device removal not configured (missing Central connection)",
		})
	}

	// TODO: Implement device removal via Central API
	// This requires adding an endpoint to Central to remove nodes by hostname
	// For now, return a not implemented error

	return c.JSON(http.StatusNotImplemented, map[string]string{
		"error":   "Device removal not yet implemented",
		"message": "Contact support to remove devices from your network",
	})
}

// hasTag checks if a tag list contains a specific tag.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Helper to filter peers to only clients
func filterClientPeers(peers []mesh.Peer) []mesh.Peer {
	var clients []mesh.Peer
	for _, p := range peers {
		if hasTag(p.Tags, "tag:client") {
			clients = append(clients, p)
		}
	}
	return clients
}
