package api

import (
	"net/http"
	"sync"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/auth"
)

// passkeySessionStore holds temporary WebAuthn session data
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

// getWebAuthnConfig creates a WebAuthn config from the request origin
func (s *Server) getWebAuthnConfig(c echo.Context) *auth.PasskeyConfig {
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

// handlePasskeyStatus returns whether passkeys are configured
func (s *Server) handlePasskeyStatus(c echo.Context) error {
	hasCredentials, err := s.db.HasAnyCredentials()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check credentials")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"configured": hasCredentials,
	})
}

// handlePasskeyRegisterBegin starts passkey registration
func (s *Server) handlePasskeyRegisterBegin(c echo.Context) error {
	// Check if this is first-time setup (no users exist)
	hasUsers, err := s.db.HasAnyUsers()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check users")
	}

	// For now, only allow registration if no users exist (single-user mode)
	// In the future, could allow authenticated users to add more passkeys
	if hasUsers {
		return echo.NewHTTPError(http.StatusForbidden, "registration not allowed - user already exists")
	}

	// Create WebAuthn instance
	cfg := s.getWebAuthnConfig(c)
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

// handlePasskeyRegisterFinish completes passkey registration
func (s *Server) handlePasskeyRegisterFinish(c echo.Context) error {
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
	cfg := s.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create user for verification
	user := auth.NewWebAuthnUser(userID, "owner", nil)

	// Finish registration - body contains the credential response
	credential, err := wa.FinishRegistration(user, *session, c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "registration failed: "+err.Error())
	}

	// Create user in database
	dbUser, err := s.db.CreateUser("") // Empty public key for passkey-only user
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create user")
	}

	// Store credential
	_, err = s.db.CreateWebAuthnCredential(dbUser.ID, credential)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to store credential")
	}

	// Generate JWT token
	if s.tokenConfig == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "authentication not configured")
	}

	token, err := auth.GenerateToken(dbUser.ID, s.tokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	// Update last login
	s.db.UpdateUserLastLogin(dbUser.ID)

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"user_id": dbUser.ID,
	})
}

// handlePasskeyLoginBegin starts passkey authentication
func (s *Server) handlePasskeyLoginBegin(c echo.Context) error {
	// Get the first user (single-user mode)
	user, err := s.db.GetFirstUser()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get user")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no user registered")
	}

	// Get user's credentials
	credentials, err := s.db.GetWebAuthnCredentialsByUserID(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get credentials")
	}
	if len(credentials) == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "no passkeys registered")
	}

	// Create WebAuthn instance
	cfg := s.getWebAuthnConfig(c)
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

// handlePasskeyLoginFinish completes passkey authentication
func (s *Server) handlePasskeyLoginFinish(c echo.Context) error {
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
	user, err := s.db.GetUserByID(userID)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Get user's credentials
	credentials, err := s.db.GetWebAuthnCredentialsByUserID(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get credentials")
	}

	// Create WebAuthn instance
	cfg := s.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize WebAuthn")
	}

	// Create WebAuthn user
	waUser := auth.NewWebAuthnUser(user.ID, "owner", credentials)

	// Finish login
	credential, err := wa.FinishLogin(waUser, *session, c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication failed: "+err.Error())
	}

	// Update credential counter (replay protection)
	s.db.UpdateWebAuthnCredentialCounter(credential.ID, credential.Authenticator.SignCount)

	// Generate JWT token
	if s.tokenConfig == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "authentication not configured")
	}

	token, err := auth.GenerateToken(user.ID, s.tokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	// Update last login
	s.db.UpdateUserLastLogin(user.ID)

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"user_id": user.ID,
	})
}
