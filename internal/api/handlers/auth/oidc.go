package auth

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/auth"
	"github.com/lirancohen/dex/internal/auth/oidc"
)

// OIDCHandler handles OIDC provider HTTP requests.
type OIDCHandler struct {
	deps           *core.Deps
	provider       *oidc.Provider
	sessionManager *auth.SessionManager
	loginURL       string
}

// OIDCConfig holds configuration for the OIDC handler.
type OIDCConfig struct {
	Issuer   string // Base URL (e.g., https://hq.alice.enbox.id)
	DataDir  string // Directory for storing OIDC keys
	LoginURL string // URL to redirect for authentication (e.g., /login)
}

// NewOIDCHandler creates a new OIDC handler.
// Returns nil if OIDC is not configured (no issuer provided).
func NewOIDCHandler(deps *core.Deps, cfg OIDCConfig) (*OIDCHandler, error) {
	if cfg.Issuer == "" {
		return nil, nil // OIDC not configured
	}

	// Load or generate RSA keys for token signing
	keyPair, err := oidc.LoadOrGenerateKeyPair(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	// Create OIDC provider
	providerConfig := oidc.DefaultConfig(cfg.Issuer)
	provider := oidc.NewProvider(providerConfig, keyPair)

	// Create session manager for OIDC sessions
	// This is separate from the JWT-based API auth
	sessionManager := auth.NewSessionManager(auth.SessionManagerConfig{
		CookieName: "hq_session",
		Secure:     strings.HasPrefix(cfg.Issuer, "https://"),
	})

	loginURL := cfg.LoginURL
	if loginURL == "" {
		loginURL = "/login"
	}

	return &OIDCHandler{
		deps:           deps,
		provider:       provider,
		sessionManager: sessionManager,
		loginURL:       loginURL,
	}, nil
}

// RegisterClient registers an OIDC client (e.g., Forgejo).
func (h *OIDCHandler) RegisterClient(client *oidc.Client) error {
	return h.provider.RegisterClient(client)
}

// GetSessionManager returns the session manager for use by other handlers.
func (h *OIDCHandler) GetSessionManager() *auth.SessionManager {
	return h.sessionManager
}

// GetProvider returns the OIDC provider for use by other handlers.
func (h *OIDCHandler) GetProvider() *oidc.Provider {
	return h.provider
}

// RegisterRoutes registers OIDC routes on the Echo instance.
// OIDC endpoints are at root level per spec, not under /api/v1.
func (h *OIDCHandler) RegisterRoutes(e *echo.Echo) {
	// OIDC Discovery
	e.GET("/.well-known/openid-configuration", h.handleDiscovery)

	// OAuth2/OIDC endpoints
	e.GET("/oauth/authorize", h.handleAuthorize)
	e.POST("/oauth/token", h.handleToken)
	e.GET("/oauth/userinfo", h.handleUserInfo)
	e.POST("/oauth/userinfo", h.handleUserInfo) // OIDC allows both GET and POST
	e.GET("/oauth/jwks", h.handleJWKS)
}

// handleDiscovery returns the OIDC discovery document.
// GET /.well-known/openid-configuration
func (h *OIDCHandler) handleDiscovery(c echo.Context) error {
	doc := h.provider.DiscoveryDocument()
	return c.JSON(http.StatusOK, doc)
}

// handleJWKS returns the JSON Web Key Set.
// GET /oauth/jwks
func (h *OIDCHandler) handleJWKS(c echo.Context) error {
	jwks, err := h.provider.JWKS()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, oidc.ErrorResponse{
			Error:            oidc.ErrServerError,
			ErrorDescription: "failed to generate JWKS",
		})
	}

	c.Response().Header().Set("Content-Type", "application/json")
	return c.Blob(http.StatusOK, "application/json", jwks)
}

// handleAuthorize handles the authorization endpoint.
// GET /oauth/authorize
func (h *OIDCHandler) handleAuthorize(c echo.Context) error {
	// Parse parameters
	clientID := c.QueryParam("client_id")
	redirectURI := c.QueryParam("redirect_uri")
	responseType := c.QueryParam("response_type")
	scope := c.QueryParam("scope")
	state := c.QueryParam("state")
	nonce := c.QueryParam("nonce")

	// Validate client
	client, err := h.provider.GetClient(clientID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, oidc.ErrorResponse{
			Error:            oidc.ErrInvalidClient,
			ErrorDescription: "unknown client",
		})
	}

	// Validate redirect URI
	// Per RFC 6749, if redirect_uri is omitted and client has exactly one registered, use it
	if redirectURI == "" {
		redirectURI = client.GetDefaultRedirectURI()
	}
	if !client.ValidateRedirectURI(redirectURI) {
		return c.JSON(http.StatusBadRequest, oidc.ErrorResponse{
			Error:            oidc.ErrInvalidRequest,
			ErrorDescription: "invalid redirect_uri",
		})
	}

	// Validate response type
	if responseType != "code" {
		return h.redirectWithError(c, redirectURI, state, oidc.ErrUnsupportedResponseType, "only 'code' response type is supported")
	}

	// Parse and validate scopes
	scopes := strings.Fields(scope)
	hasOpenID := false
	for _, s := range scopes {
		if s == "openid" {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		return h.redirectWithError(c, redirectURI, state, oidc.ErrInvalidScope, "openid scope is required")
	}

	// Check if user is authenticated
	session, err := h.sessionManager.GetSessionFromRequest(c.Request())
	if err != nil {
		// Not authenticated - redirect to login
		// Store the full authorize URL so we can resume after login
		loginURL := h.loginURL + "?next=" + c.Request().URL.String()
		return c.Redirect(http.StatusFound, loginURL)
	}

	// User is authenticated - issue authorization code
	authCode, err := h.provider.CreateAuthorizationCode(
		clientID,
		session.UserID,
		session.Email,
		session.Email, // Use email as display name
		redirectURI,
		scopes,
		nonce,
	)
	if err != nil {
		return h.redirectWithError(c, redirectURI, state, oidc.ErrServerError, "failed to create authorization code")
	}

	// Build redirect URL with code
	redirectURL := redirectURI + "?code=" + authCode.Code
	if state != "" {
		redirectURL += "&state=" + state
	}

	return c.Redirect(http.StatusFound, redirectURL)
}

// handleToken handles the token endpoint.
// POST /oauth/token
func (h *OIDCHandler) handleToken(c echo.Context) error {
	// Parse form data
	grantType := c.FormValue("grant_type")
	if grantType != "authorization_code" {
		return c.JSON(http.StatusBadRequest, oidc.ErrorResponse{
			Error:            oidc.ErrUnsupportedGrantType,
			ErrorDescription: "only authorization_code grant is supported",
		})
	}

	code := c.FormValue("code")
	redirectURI := c.FormValue("redirect_uri")

	// Get client credentials
	clientID, clientSecret, err := h.getClientCredentials(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, oidc.ErrorResponse{
			Error:            oidc.ErrInvalidClient,
			ErrorDescription: "invalid client authentication",
		})
	}

	// Validate client
	client, err := h.provider.ValidateClient(clientID, clientSecret)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, oidc.ErrorResponse{
			Error:            oidc.ErrInvalidClient,
			ErrorDescription: "invalid client credentials",
		})
	}

	// Per RFC 6749, if redirect_uri was omitted in authorization request
	// (and client has only one registered), it can be omitted in token request too
	if redirectURI == "" {
		redirectURI = client.GetDefaultRedirectURI()
	}

	// Exchange code for tokens
	tokenResponse, err := h.provider.ExchangeAuthorizationCode(code, clientID, redirectURI)
	if err != nil {
		if err.Error() == oidc.ErrInvalidGrant {
			return c.JSON(http.StatusBadRequest, oidc.ErrorResponse{
				Error:            oidc.ErrInvalidGrant,
				ErrorDescription: "invalid or expired authorization code",
			})
		}
		return c.JSON(http.StatusInternalServerError, oidc.ErrorResponse{
			Error:            oidc.ErrServerError,
			ErrorDescription: "failed to exchange authorization code",
		})
	}

	// Prevent caching
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Pragma", "no-cache")

	return c.JSON(http.StatusOK, tokenResponse)
}

// handleUserInfo returns user information for a valid access token.
// GET/POST /oauth/userinfo
func (h *OIDCHandler) handleUserInfo(c echo.Context) error {
	// Get access token from Authorization header
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		c.Response().Header().Set("WWW-Authenticate", "Bearer")
		return c.JSON(http.StatusUnauthorized, oidc.ErrorResponse{
			Error:            oidc.ErrInvalidRequest,
			ErrorDescription: "missing or invalid authorization header",
		})
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	userInfo, err := h.provider.GetUserInfo(token)
	if err != nil {
		c.Response().Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		return c.JSON(http.StatusUnauthorized, oidc.ErrorResponse{
			Error:            oidc.ErrInvalidRequest,
			ErrorDescription: "invalid or expired access token",
		})
	}

	return c.JSON(http.StatusOK, userInfo)
}

// getClientCredentials extracts client credentials from the request.
// Supports both Basic auth and form parameters.
func (h *OIDCHandler) getClientCredentials(c echo.Context) (clientID, clientSecret string, err error) {
	// Try Basic authentication first
	clientID, clientSecret, ok := c.Request().BasicAuth()
	if ok && clientID != "" && clientSecret != "" {
		return clientID, clientSecret, nil
	}

	// Fall back to form parameters
	clientID = c.FormValue("client_id")
	clientSecret = c.FormValue("client_secret")

	if clientID == "" || clientSecret == "" {
		return "", "", echo.ErrUnauthorized
	}

	return clientID, clientSecret, nil
}

// redirectWithError redirects to the client with an error.
func (h *OIDCHandler) redirectWithError(c echo.Context, redirectURI, state, errorCode, description string) error {
	redirectURL := redirectURI + "?error=" + errorCode + "&error_description=" + description
	if state != "" {
		redirectURL += "&state=" + state
	}
	return c.Redirect(http.StatusFound, redirectURL)
}

// Cleanup removes expired tokens and sessions.
// Should be called periodically.
func (h *OIDCHandler) Cleanup() {
	h.provider.Cleanup()
}

// CreateOIDCHandlerFromEnrollment creates an OIDC handler from enrollment config.
// This is a convenience function for the main server setup.
func CreateOIDCHandlerFromEnrollment(deps *core.Deps, publicURL, baseDir string) (*OIDCHandler, error) {
	if publicURL == "" {
		return nil, nil // No enrollment, no OIDC
	}

	return NewOIDCHandler(deps, OIDCConfig{
		Issuer:   publicURL,
		DataDir:  filepath.Join(baseDir, "oidc"),
		LoginURL: "/login",
	})
}
