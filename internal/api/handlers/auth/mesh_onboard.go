package auth

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/api/middleware"
	"github.com/lirancohen/dex/internal/auth"
)

// MeshOnboardHandler handles mesh passkey onboarding HTTP requests.
type MeshOnboardHandler struct {
	deps       *core.Deps
	tokenStore *auth.OnboardingTokenStore
	namespace  string // The namespace for mesh rpID (e.g., "alice" for alice.dex)
}

// NewMeshOnboardHandler creates a new mesh onboard handler.
func NewMeshOnboardHandler(deps *core.Deps, namespace string) *MeshOnboardHandler {
	return &MeshOnboardHandler{
		deps:       deps,
		tokenStore: auth.NewOnboardingTokenStore(auth.DefaultOnboardingTokenTTL),
		namespace:  namespace,
	}
}

// RegisterRoutes registers mesh onboarding routes on the given group.
//   - POST /auth/mesh-onboard/initiate     (start onboarding from public URL)
//   - POST /auth/mesh-onboard/complete     (complete onboarding at mesh URL)
//   - GET  /auth/mesh-onboard/status       (check onboarding status)
//   - POST /auth/mesh-passkey/register/begin  (register mesh passkey)
//   - POST /auth/mesh-passkey/register/finish (complete mesh passkey registration)
//   - GET  /auth/passkeys                  (list user's passkeys)
//   - PUT  /auth/passkeys/:id              (rename a passkey)
//   - DELETE /auth/passkeys/:id            (delete a passkey)
func (h *MeshOnboardHandler) RegisterRoutes(g *echo.Group) {
	// Mesh onboarding flow (require auth)
	g.POST("/auth/mesh-onboard/initiate", h.HandleInitiate)
	g.POST("/auth/mesh-onboard/complete", h.HandleComplete)
	g.GET("/auth/mesh-onboard/status", h.HandleStatus)

	// Mesh passkey registration (require auth + onboarding complete)
	g.POST("/auth/mesh-passkey/register/begin", h.HandleMeshPasskeyRegisterBegin)
	g.POST("/auth/mesh-passkey/register/finish", h.HandleMeshPasskeyRegisterFinish)

	// Passkey management (require auth)
	g.GET("/auth/passkeys", h.HandleListPasskeys)
	g.PUT("/auth/passkeys/:id", h.HandleRenamePasskey)
	g.DELETE("/auth/passkeys/:id", h.HandleDeletePasskey)
}

// getMeshRPID returns the mesh rpID for this namespace (e.g., "alice.dex")
func (h *MeshOnboardHandler) getMeshRPID() string {
	return h.namespace + ".dex"
}

// getMeshOrigin returns the mesh origin for HQ (e.g., "https://hq.alice.dex")
func (h *MeshOnboardHandler) getMeshOrigin() string {
	return "https://hq." + h.getMeshRPID()
}

// HandleInitiate starts the mesh onboarding flow.
// Called from the public URL (hq.alice.enbox.id) when user clicks "Enable Direct Mesh Access".
// POST /api/v1/auth/mesh-onboard/initiate
func (h *MeshOnboardHandler) HandleInitiate(c echo.Context) error {
	// Require authentication
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	// Generate onboarding token
	meshRPID := h.getMeshRPID()
	meshOrigin := h.getMeshOrigin()

	token, err := h.tokenStore.GenerateToken(userID, meshRPID, meshOrigin)
	if err != nil {
		log.Printf("Mesh onboard initiate: failed to generate token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	log.Printf("Mesh onboard initiate: generated token for user %s, meshRPID=%s", userID, meshRPID)

	// Return the redirect URL with token as fragment
	// Frontend will redirect to: https://hq.alice.dex/onboard#token=xxx
	redirectURL := meshOrigin + "/onboard#token=" + token

	return c.JSON(http.StatusOK, map[string]any{
		"redirect_url": redirectURL,
		"mesh_rpid":    meshRPID,
		"mesh_origin":  meshOrigin,
	})
}

// HandleComplete validates the onboarding token and establishes a session.
// Called from the mesh URL (hq.alice.dex) after redirect.
// POST /api/v1/auth/mesh-onboard/complete
// Body: {"token": "xxx"}
func (h *MeshOnboardHandler) HandleComplete(c echo.Context) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}

	// Validate and consume token
	tokenData := h.tokenStore.ValidateAndConsumeToken(req.Token)
	if tokenData == nil {
		log.Printf("Mesh onboard complete: invalid or expired token")
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
	}

	log.Printf("Mesh onboard complete: token validated for user %s, meshRPID=%s", tokenData.UserID, tokenData.MeshRPID)

	// Mark onboarding as complete (but passkey not yet synced)
	err := h.deps.DB.CreateOrUpdateMeshOnboardingStatus(tokenData.UserID, tokenData.MeshRPID, true, false)
	if err != nil {
		log.Printf("Mesh onboard complete: failed to update status: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update onboarding status")
	}

	// Generate a JWT token for the mesh session
	if h.deps.TokenConfig == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "authentication not configured")
	}

	token, err := auth.GenerateToken(tokenData.UserID, h.deps.TokenConfig)
	if err != nil {
		log.Printf("Mesh onboard complete: failed to generate token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"token":       token,
		"user_id":     tokenData.UserID,
		"mesh_rpid":   tokenData.MeshRPID,
		"mesh_origin": tokenData.MeshOrigin,
	})
}

// HandleStatus returns the mesh onboarding status for the current user.
// GET /api/v1/auth/mesh-onboard/status
func (h *MeshOnboardHandler) HandleStatus(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	status, err := h.deps.DB.GetMeshOnboardingStatus(userID)
	if err != nil {
		log.Printf("Mesh onboard status: failed to get status: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get status")
	}

	if status == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"onboarding_complete": false,
			"passkey_synced":      false,
			"mesh_rpid":           h.getMeshRPID(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"onboarding_complete": status.OnboardingComplete,
		"passkey_synced":      status.PasskeySynced,
		"mesh_rpid":           status.MeshRPID,
		"completed_at":        status.CompletedAt,
	})
}

// HandleMeshPasskeyRegisterBegin starts mesh passkey registration.
// Requires user to have completed onboarding.
// POST /api/v1/auth/mesh-passkey/register/begin
func (h *MeshOnboardHandler) HandleMeshPasskeyRegisterBegin(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	// Check onboarding status
	status, err := h.deps.DB.GetMeshOnboardingStatus(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check onboarding status")
	}
	if status == nil || !status.OnboardingComplete {
		return echo.NewHTTPError(http.StatusForbidden, "mesh onboarding not complete")
	}

	// Get user's existing credentials for this rpID to exclude them
	meshRPID := h.getMeshRPID()
	existingCreds, _ := h.deps.DB.GetCredentialsByRPID(userID, meshRPID)

	// Create WebAuthn config for mesh domain
	meshOrigin := h.getMeshOrigin()
	cfg := &auth.PasskeyConfig{
		RPDisplayName: "Poindexter (Mesh)",
		RPID:          meshRPID,
		RPOrigin:      meshOrigin,
	}

	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create user with existing credentials (to exclude them from registration)
	user := auth.NewWebAuthnUser(userID, "owner", existingCreds)

	// Generate registration options
	options, session, err := wa.BeginRegistration(user)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to begin registration: "+err.Error())
	}

	// Store session data
	sessionID := uuid.New().String()
	passkeyStore.Store(sessionID, session)

	log.Printf("Mesh passkey register begin: user=%s, meshRPID=%s", userID, meshRPID)

	return c.JSON(http.StatusOK, map[string]any{
		"session_id": sessionID,
		"user_id":    userID,
		"options":    options,
		"mesh_rpid":  meshRPID,
	})
}

// HandleMeshPasskeyRegisterFinish completes mesh passkey registration.
// POST /api/v1/auth/mesh-passkey/register/finish?session_id=...
func (h *MeshOnboardHandler) HandleMeshPasskeyRegisterFinish(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	sessionID := c.QueryParam("session_id")
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing session_id")
	}

	// Get session data
	session := passkeyStore.Get(sessionID)
	if session == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired session")
	}
	defer passkeyStore.Delete(sessionID)

	// Create WebAuthn config for mesh domain
	meshRPID := h.getMeshRPID()
	meshOrigin := h.getMeshOrigin()
	cfg := &auth.PasskeyConfig{
		RPDisplayName: "Poindexter (Mesh)",
		RPID:          meshRPID,
		RPOrigin:      meshOrigin,
	}

	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create user for verification
	user := auth.NewWebAuthnUser(userID, "owner", nil)

	// Parse the credential from request body
	parsedCredential, err := protocol.ParseCredentialCreationResponseBody(c.Request().Body)
	if err != nil {
		log.Printf("Mesh passkey registration: failed to parse credential: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse credential: "+err.Error())
	}

	// Finish registration
	credential, err := wa.CreateCredential(user, *session, parsedCredential)
	if err != nil {
		log.Printf("Mesh passkey registration: CreateCredential failed: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "registration failed: "+err.Error())
	}

	// Extract device info from request
	userAgent := c.Request().UserAgent()
	ipAddress := c.RealIP()
	deviceName := guessDeviceName(userAgent)
	location := "" // TODO: Implement IP-based geolocation

	// Store credential with metadata
	_, err = h.deps.DB.CreateWebAuthnCredentialWithMetadata(
		userID, credential, meshRPID, deviceName, userAgent, ipAddress, location,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to store credential")
	}

	// Mark passkey as synced
	if err := h.deps.DB.MarkMeshPasskeySynced(userID); err != nil {
		log.Printf("Mesh passkey registration: failed to mark synced: %v", err)
		// Non-fatal, continue
	}

	log.Printf("Mesh passkey registration: success for user %s, meshRPID=%s, device=%s", userID, meshRPID, deviceName)

	return c.JSON(http.StatusOK, map[string]any{
		"success":     true,
		"mesh_rpid":   meshRPID,
		"device_name": deviceName,
	})
}

// HandleListPasskeys returns all passkeys for the current user.
// GET /api/v1/auth/passkeys
func (h *MeshOnboardHandler) HandleListPasskeys(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	passkeys, err := h.deps.DB.ListUserPasskeys(userID)
	if err != nil {
		log.Printf("List passkeys: failed: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list passkeys")
	}

	// Convert to API response format
	result := make([]map[string]any, 0, len(passkeys))
	for _, pk := range passkeys {
		item := map[string]any{
			"id":          pk.ID,
			"device_name": pk.DeviceName,
			"rp_id":       pk.RPID,
			"user_agent":  pk.UserAgent,
			"ip_address":  pk.IPAddress,
			"location":    pk.Location,
			"created_at":  pk.CreatedAt,
		}
		if pk.LastUsedAt.Valid {
			item["last_used_at"] = pk.LastUsedAt.Time
		}
		if pk.LastUsedIP.Valid {
			item["last_used_ip"] = pk.LastUsedIP.String
		}
		result = append(result, item)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"passkeys": result,
	})
}

// HandleRenamePasskey renames a passkey.
// PUT /api/v1/auth/passkeys/:id
func (h *MeshOnboardHandler) HandleRenamePasskey(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	passkeyID := c.Param("id")
	if passkeyID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "passkey ID required")
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Verify passkey belongs to user
	passkey, err := h.deps.DB.GetPasskeyByID(passkeyID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get passkey")
	}
	if passkey == nil || passkey.UserID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "passkey not found")
	}

	// Update name
	if err := h.deps.DB.UpdatePasskeyName(passkeyID, req.Name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rename passkey")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
	})
}

// HandleDeletePasskey deletes a passkey.
// DELETE /api/v1/auth/passkeys/:id
func (h *MeshOnboardHandler) HandleDeletePasskey(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	passkeyID := c.Param("id")
	if passkeyID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "passkey ID required")
	}

	// Verify passkey belongs to user
	passkey, err := h.deps.DB.GetPasskeyByID(passkeyID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get passkey")
	}
	if passkey == nil || passkey.UserID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "passkey not found")
	}

	// Check if this is the user's last passkey - don't allow deletion
	passkeys, err := h.deps.DB.ListUserPasskeys(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check passkeys")
	}
	if len(passkeys) <= 1 {
		return echo.NewHTTPError(http.StatusForbidden, "cannot delete last passkey")
	}

	// Delete passkey
	if err := h.deps.DB.DeletePasskey(passkeyID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete passkey")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
	})
}

// guessDeviceName attempts to extract a friendly device name from the user agent.
func guessDeviceName(userAgent string) string {
	ua := strings.ToLower(userAgent)

	// Check for common devices
	switch {
	case strings.Contains(ua, "iphone"):
		return "iPhone"
	case strings.Contains(ua, "ipad"):
		return "iPad"
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os"):
		if strings.Contains(ua, "chrome") {
			return "Mac (Chrome)"
		}
		if strings.Contains(ua, "safari") {
			return "Mac (Safari)"
		}
		if strings.Contains(ua, "firefox") {
			return "Mac (Firefox)"
		}
		return "Mac"
	case strings.Contains(ua, "windows"):
		if strings.Contains(ua, "chrome") {
			return "Windows (Chrome)"
		}
		if strings.Contains(ua, "edge") {
			return "Windows (Edge)"
		}
		if strings.Contains(ua, "firefox") {
			return "Windows (Firefox)"
		}
		return "Windows PC"
	case strings.Contains(ua, "android"):
		if strings.Contains(ua, "chrome") {
			return "Android (Chrome)"
		}
		return "Android Device"
	case strings.Contains(ua, "linux"):
		if strings.Contains(ua, "chrome") {
			return "Linux (Chrome)"
		}
		if strings.Contains(ua, "firefox") {
			return "Linux (Firefox)"
		}
		return "Linux"
	default:
		return fmt.Sprintf("Unknown Device (%s)", truncate(userAgent, 30))
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
