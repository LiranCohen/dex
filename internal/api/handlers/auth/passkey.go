package auth

import (
	"log"
	"net/http"
	"sync"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/auth"
)

// passkeySessionStore holds temporary WebAuthn session data.
type passkeySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*webauthn.SessionData // sessionID -> session data
}

var passkeyStore = &passkeySessionStore{
	sessions: make(map[string]*webauthn.SessionData),
}

func (s *passkeySessionStore) Store(sessionID string, data *webauthn.SessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = data
}

func (s *passkeySessionStore) Get(sessionID string) *webauthn.SessionData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

func (s *passkeySessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// PasskeyHandler handles passkey/WebAuthn-related HTTP requests.
type PasskeyHandler struct {
	deps *core.Deps
}

// NewPasskeyHandler creates a new passkey handler.
func NewPasskeyHandler(deps *core.Deps) *PasskeyHandler {
	return &PasskeyHandler{deps: deps}
}

// RegisterRoutes registers all passkey routes on the given group.
// These are all public routes (WebAuthn requires specific flow).
//   - GET /auth/passkey/status
//   - POST /auth/passkey/register/begin
//   - POST /auth/passkey/register/finish
//   - POST /auth/passkey/login/begin
//   - POST /auth/passkey/login/finish
func (h *PasskeyHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/auth/passkey/status", h.HandleStatus)
	g.POST("/auth/passkey/register/begin", h.HandleRegisterBegin)
	g.POST("/auth/passkey/register/finish", h.HandleRegisterFinish)
	g.POST("/auth/passkey/login/begin", h.HandleLoginBegin)
	g.POST("/auth/passkey/login/finish", h.HandleLoginFinish)
}

// getWebAuthnConfig creates a WebAuthn config from the request origin.
func (h *PasskeyHandler) getWebAuthnConfig(c echo.Context) *auth.PasskeyConfig {
	// Get origin from request
	origin := c.Request().Header.Get("Origin")
	if origin == "" {
		// Fallback: construct from Host header
		scheme := "https"
		if c.Request().TLS == nil {
			scheme = "http"
		}
		origin = scheme + "://" + c.Request().Host
	}

	// Extract hostname for RPID (domain without port)
	host := c.Request().Host
	if colonIdx := len(host) - 1; colonIdx > 0 {
		for i := len(host) - 1; i >= 0; i-- {
			if host[i] == ':' {
				host = host[:i]
				break
			}
			if host[i] == ']' {
				// IPv6, stop looking for port
				break
			}
		}
	}

	return &auth.PasskeyConfig{
		RPDisplayName: "Poindexter",
		RPID:          host,
		RPOrigin:      origin,
	}
}

// HandleStatus returns whether passkeys are configured.
// GET /api/v1/auth/passkey/status
func (h *PasskeyHandler) HandleStatus(c echo.Context) error {
	hasCredentials, err := h.deps.DB.HasAnyCredentials()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check credentials")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"configured": hasCredentials,
	})
}

// HandleRegisterBegin starts passkey registration.
// POST /api/v1/auth/passkey/register/begin
func (h *PasskeyHandler) HandleRegisterBegin(c echo.Context) error {
	// Check if this is first-time setup (no users exist)
	hasUsers, err := h.deps.DB.HasAnyUsers()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check users")
	}

	// For now, only allow registration if no users exist (single-user mode)
	if hasUsers {
		return echo.NewHTTPError(http.StatusForbidden, "registration not allowed - user already exists")
	}

	// Create WebAuthn instance
	cfg := h.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create a new user ID for registration
	userID := uuid.New().String()
	user := auth.NewWebAuthnUser(userID, "owner", nil)

	// Generate registration options
	options, session, err := wa.BeginRegistration(user)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to begin registration: "+err.Error())
	}

	// Store session data
	sessionID := uuid.New().String()
	passkeyStore.Store(sessionID, session)

	// Return options to client
	return c.JSON(http.StatusOK, map[string]any{
		"session_id": sessionID,
		"user_id":    userID,
		"options":    options,
	})
}

// HandleRegisterFinish completes passkey registration.
// POST /api/v1/auth/passkey/register/finish?session_id=...&user_id=...
func (h *PasskeyHandler) HandleRegisterFinish(c echo.Context) error {
	// Get session_id and user_id from query params (body is reserved for credential)
	sessionID := c.QueryParam("session_id")
	userID := c.QueryParam("user_id")

	if sessionID == "" || userID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing session_id or user_id")
	}

	// Get session data
	session := passkeyStore.Get(sessionID)
	if session == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired session")
	}
	defer passkeyStore.Delete(sessionID)

	// Create WebAuthn instance
	cfg := h.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create user for verification
	user := auth.NewWebAuthnUser(userID, "owner", nil)

	// Parse the credential from request body
	parsedCredential, err := protocol.ParseCredentialCreationResponseBody(c.Request().Body)
	if err != nil {
		log.Printf("Passkey registration: failed to parse credential: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse credential: "+err.Error())
	}

	log.Printf("Passkey registration: parsed credential for user %s, RPID=%s, Origin=%s", userID, cfg.RPID, cfg.RPOrigin)

	// Finish registration using CreateCredential
	credential, err := wa.CreateCredential(user, *session, parsedCredential)
	if err != nil {
		log.Printf("Passkey registration: CreateCredential failed: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "registration failed: "+err.Error())
	}

	// Create user in database with the SAME ID used for WebAuthn registration
	dbUser, err := h.deps.DB.CreateUserWithID(userID, "") // Empty public key for passkey-only user
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create user")
	}

	// Store credential
	_, err = h.deps.DB.CreateWebAuthnCredential(dbUser.ID, credential)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to store credential")
	}

	// Generate JWT token
	if h.deps.TokenConfig == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "authentication not configured")
	}

	token, err := auth.GenerateToken(dbUser.ID, h.deps.TokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	// Update last login
	_ = h.deps.DB.UpdateUserLastLogin(dbUser.ID)

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"user_id": dbUser.ID,
	})
}

// HandleLoginBegin starts passkey authentication.
// POST /api/v1/auth/passkey/login/begin
func (h *PasskeyHandler) HandleLoginBegin(c echo.Context) error {
	// Get the first user (single-user mode)
	user, err := h.deps.DB.GetFirstUser()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get user")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no user registered")
	}

	// Get user's credentials
	credentials, err := h.deps.DB.GetWebAuthnCredentialsByUserID(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get credentials")
	}
	if len(credentials) == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "no passkeys registered")
	}

	// Create WebAuthn instance
	cfg := h.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create WebAuthn user with credentials
	waUser := auth.NewWebAuthnUser(user.ID, "owner", credentials)

	// Generate assertion options
	options, session, err := wa.BeginLogin(waUser)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to begin login: "+err.Error())
	}

	// Store session data
	sessionID := uuid.New().String()
	passkeyStore.Store(sessionID, session)

	return c.JSON(http.StatusOK, map[string]any{
		"session_id": sessionID,
		"user_id":    user.ID,
		"options":    options,
	})
}

// HandleLoginFinish completes passkey authentication.
// POST /api/v1/auth/passkey/login/finish?session_id=...&user_id=...
func (h *PasskeyHandler) HandleLoginFinish(c echo.Context) error {
	// Get session_id and user_id from query params (body is reserved for credential)
	sessionID := c.QueryParam("session_id")
	userID := c.QueryParam("user_id")

	if sessionID == "" || userID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing session_id or user_id")
	}

	// Get session data
	session := passkeyStore.Get(sessionID)
	if session == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired session")
	}
	defer passkeyStore.Delete(sessionID)

	// Get user
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Get user's credentials
	credentials, err := h.deps.DB.GetWebAuthnCredentialsByUserID(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get credentials")
	}

	// Create WebAuthn instance
	cfg := h.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create WebAuthn user
	waUser := auth.NewWebAuthnUser(user.ID, "owner", credentials)

	// Parse the credential assertion from request body
	parsedAssertion, err := protocol.ParseCredentialRequestResponseBody(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse credential: "+err.Error())
	}

	// Finish login using ValidateLogin
	credential, err := wa.ValidateLogin(waUser, *session, parsedAssertion)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication failed: "+err.Error())
	}

	// Update credential counter (replay protection)
	_ = h.deps.DB.UpdateWebAuthnCredentialCounter(credential.ID, credential.Authenticator.SignCount)

	// Generate JWT token
	if h.deps.TokenConfig == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "authentication not configured")
	}

	token, err := auth.GenerateToken(user.ID, h.deps.TokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	// Update last login
	_ = h.deps.DB.UpdateUserLastLogin(user.ID)

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"user_id": user.ID,
	})
}
