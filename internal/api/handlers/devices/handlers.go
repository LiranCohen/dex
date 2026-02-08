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
)

// Handler handles device-related HTTP requests.
type Handler struct {
	deps        *core.Deps
	namespace   string // From enrollment config
	tunnelToken string // For authenticating with Central
	centralURL  string
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
	devices.PATCH("/:hostname/expiry", h.UpdateDeviceExpiry)
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

	url := strings.TrimSuffix(h.centralURL, "/") + "/api/v1/hq/client-enrollment-key"
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
		// Don't forward Central's status code (especially 401) to frontend
		// as it triggers frontend logout. Use 502 Bad Gateway instead.
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("Central returned %d: %s", resp.StatusCode, string(body)),
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
	ID        string   `json:"id,omitempty"` // Node ID from Central (for deletion)
	Hostname  string   `json:"hostname"`
	Type      string   `json:"type,omitempty"` // "hq", "outpost", or "client"
	MeshIP    string   `json:"mesh_ip"`
	Online    bool     `json:"online"`
	Direct    bool     `json:"direct"`
	LastSeen  string   `json:"last_seen,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"` // When the node expires (empty = permanent)
	Tags      []string `json:"tags,omitempty"`
	IsClient  bool     `json:"is_client"` // True if this is a client device (tag:client)
}

// CentralDeviceResponse is the response from Central's GET /api/v1/hq/devices
type CentralDeviceResponse struct {
	ID        string   `json:"id"`
	Hostname  string   `json:"hostname"`
	Type      string   `json:"type"`
	Status    string   `json:"status"`
	MeshIP    string   `json:"mesh_ip"`
	LastSeen  *string  `json:"last_seen"`
	ExpiresAt *string  `json:"expires_at"`
	Tags      []string `json:"tags"`
}

// ListDevicesResponse is the response for listing devices.
type ListDevicesResponse struct {
	Devices []DeviceInfo `json:"devices"`
	Count   int          `json:"count"`
}

// ListDevices returns the list of client devices connected to the mesh.
// GET /api/v1/devices
// When Central is available, fetches devices from Central (includes IDs for deletion).
// Falls back to local mesh peer status if Central is unavailable.
func (h *Handler) ListDevices(c echo.Context) error {
	// Try to get devices from Central first (includes IDs needed for deletion)
	if h.centralURL != "" && h.tunnelToken != "" {
		devices, err := h.listDevicesFromCentral()
		if err == nil {
			return c.JSON(http.StatusOK, ListDevicesResponse{
				Devices: devices,
				Count:   len(devices),
			})
		}
		// Log error but fall back to mesh status
		c.Logger().Warnf("Failed to get devices from Central, falling back to mesh: %v", err)
	}

	// Fallback: get devices from local mesh peer status
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
		deviceType := "outpost"
		for _, tag := range p.Tags {
			switch tag {
			case "tag:client":
				isClient = true
				deviceType = "client"
			case "tag:hq":
				deviceType = "hq"
			case "tag:outpost":
				deviceType = "outpost"
			}
		}

		devices = append(devices, DeviceInfo{
			Hostname: p.Hostname,
			Type:     deviceType,
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

// listDevicesFromCentral fetches devices from Central's HQ API.
func (h *Handler) listDevicesFromCentral() ([]DeviceInfo, error) {
	url := strings.TrimSuffix(h.centralURL, "/") + "/api/v1/hq/devices"
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+h.tunnelToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Central: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("central returned %d: %s", resp.StatusCode, string(body))
	}

	var centralDevices []CentralDeviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&centralDevices); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert Central response to DeviceInfo
	devices := make([]DeviceInfo, 0, len(centralDevices))
	for _, cd := range centralDevices {
		isClient := cd.Type == "client"
		lastSeen := ""
		if cd.LastSeen != nil {
			lastSeen = *cd.LastSeen
		}
		expiresAt := ""
		if cd.ExpiresAt != nil {
			expiresAt = *cd.ExpiresAt
		}

		devices = append(devices, DeviceInfo{
			ID:        cd.ID,
			Hostname:  cd.Hostname,
			Type:      cd.Type,
			MeshIP:    cd.MeshIP,
			Online:    cd.Status == "online",
			Direct:    false, // Central doesn't track direct connections
			LastSeen:  lastSeen,
			ExpiresAt: expiresAt,
			Tags:      cd.Tags,
			IsClient:  isClient,
		})
	}

	return devices, nil
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

	// Call Central's HQ API to delete the device
	url := strings.TrimSuffix(h.centralURL, "/") + "/api/v1/hq/devices/" + hostname
	httpReq, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create request",
		})
	}

	httpReq.Header.Set("Authorization", "Bearer "+h.tunnelToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "Failed to connect to Central: " + err.Error(),
		})
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle response based on status code
	switch resp.StatusCode {
	case http.StatusNoContent:
		// Success - device was deleted
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Device removed successfully",
		})
	case http.StatusNotFound:
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Device not found",
		})
	case http.StatusForbidden:
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Cannot remove this device (HQ nodes cannot be removed via this API)",
		})
	case http.StatusUnauthorized:
		// Don't forward 401 as it triggers frontend logout - use 502 instead
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "Central authentication failed",
		})
	default:
		body, _ := io.ReadAll(resp.Body)
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("Central returned %d: %s", resp.StatusCode, string(body)),
		})
	}
}

// UpdateDeviceExpiryRequest is the request body for updating device expiry.
type UpdateDeviceExpiryRequest struct {
	// ExpiresAt is the new expiry time in RFC3339 format.
	// If null/omitted/empty, the device becomes permanent (no expiry).
	ExpiresAt *string `json:"expires_at"`
}

// UpdateDeviceExpiryResponse is the response for updating device expiry.
type UpdateDeviceExpiryResponse struct {
	Hostname  string `json:"hostname"`
	ExpiresAt string `json:"expires_at,omitempty"` // empty means permanent
}

// UpdateDeviceExpiry updates the expiry time for a device.
// PATCH /api/v1/devices/:hostname/expiry
func (h *Handler) UpdateDeviceExpiry(c echo.Context) error {
	hostname := c.Param("hostname")
	if hostname == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Hostname is required",
		})
	}

	if h.centralURL == "" || h.tunnelToken == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Device expiry update not configured (missing Central connection)",
		})
	}

	var req UpdateDeviceExpiryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Build request body for Central
	reqBody, err := json.Marshal(req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create request",
		})
	}

	// Call Central's HQ API to update device expiry
	url := strings.TrimSuffix(h.centralURL, "/") + "/api/v1/hq/devices/" + hostname + "/expiry"
	httpReq, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(reqBody))
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

	// Handle response based on status code
	switch resp.StatusCode {
	case http.StatusOK:
		// Success - parse and return the response
		var centralResp UpdateDeviceExpiryResponse
		if err := json.NewDecoder(resp.Body).Decode(&centralResp); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to parse response",
			})
		}
		return c.JSON(http.StatusOK, centralResp)
	case http.StatusNotFound:
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Device not found",
		})
	case http.StatusForbidden:
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Cannot modify HQ node expiry (always permanent)",
		})
	case http.StatusBadRequest:
		body, _ := io.ReadAll(resp.Body)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": string(body),
		})
	case http.StatusUnauthorized:
		// Don't forward 401 as it triggers frontend logout - use 502 instead
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "Central authentication failed",
		})
	default:
		body, _ := io.ReadAll(resp.Body)
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("Central returned %d: %s", resp.StatusCode, string(body)),
		})
	}
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
