package oidc

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"github.com/lirancohen/dex/internal/auth"
)

// Handlers wraps the OIDC provider with HTTP handlers.
type Handlers struct {
	provider       *Provider
	sessionManager *auth.SessionManager
	loginURL       string // URL to redirect for authentication
}

// NewHandlers creates new OIDC HTTP handlers.
func NewHandlers(provider *Provider, sessionManager *auth.SessionManager, loginURL string) *Handlers {
	return &Handlers{
		provider:       provider,
		sessionManager: sessionManager,
		loginURL:       loginURL,
	}
}

// HandleDiscovery handles GET /.well-known/openid-configuration
func (h *Handlers) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}

	doc := h.provider.DiscoveryDocument()
	WriteJSON(w, http.StatusOK, doc)
}

// HandleJWKS handles GET /oauth/jwks
func (h *Handlers) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}

	jwks, err := h.provider.JWKS()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrServerError, "failed to generate JWKS")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jwks)
}

// HandleAuthorize handles GET /oauth/authorize
func (h *Handlers) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}

	// Parse required parameters
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	nonce := r.URL.Query().Get("nonce")

	// Validate client
	client, err := h.provider.GetClient(clientID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrInvalidClient, "unknown client")
		return
	}

	// Validate redirect URI
	if !client.ValidateRedirectURI(redirectURI) {
		WriteError(w, http.StatusBadRequest, ErrInvalidRequest, "invalid redirect_uri")
		return
	}

	// Validate response type
	if responseType != "code" {
		redirectWithError(w, r, redirectURI, state, ErrUnsupportedResponseType, "only 'code' response type is supported")
		return
	}

	// Parse scopes
	scopes := parseScopes(scope)
	if !hasScope(scopes, "openid") {
		redirectWithError(w, r, redirectURI, state, ErrInvalidScope, "openid scope is required")
		return
	}

	// Check if user is authenticated
	session, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		// Not authenticated - redirect to login
		loginURL := h.loginURL + "?next=" + url.QueryEscape(r.URL.String())
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
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
		redirectWithError(w, r, redirectURI, state, ErrServerError, "failed to create authorization code")
		return
	}

	// Redirect back with code
	redirectURL, _ := url.Parse(redirectURI)
	q := redirectURL.Query()
	q.Set("code", authCode.Code)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// HandleToken handles POST /oauth/token
func (h *Handlers) HandleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		WriteError(w, http.StatusBadRequest, ErrInvalidRequest, "invalid request body")
		return
	}

	grantType := r.PostFormValue("grant_type")
	if grantType != "authorization_code" {
		WriteError(w, http.StatusBadRequest, ErrUnsupportedGrantType, "only authorization_code grant is supported")
		return
	}

	code := r.PostFormValue("code")
	redirectURI := r.PostFormValue("redirect_uri")

	// Get client credentials (from Basic auth or form)
	clientID, clientSecret, err := getClientCredentials(r)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, ErrInvalidClient, "invalid client authentication")
		return
	}

	// Validate client
	_, err = h.provider.ValidateClient(clientID, clientSecret)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, ErrInvalidClient, "invalid client credentials")
		return
	}

	// Exchange code for tokens
	tokenResponse, err := h.provider.ExchangeAuthorizationCode(code, clientID, redirectURI)
	if err != nil {
		if err.Error() == ErrInvalidGrant {
			WriteError(w, http.StatusBadRequest, ErrInvalidGrant, "invalid or expired authorization code")
		} else {
			WriteError(w, http.StatusInternalServerError, ErrServerError, "failed to exchange authorization code")
		}
		return
	}

	// Prevent caching of token response
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	WriteJSON(w, http.StatusOK, tokenResponse)
}

// HandleUserInfo handles GET /oauth/userinfo
func (h *Handlers) HandleUserInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}

	// Get access token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		w.Header().Set("WWW-Authenticate", "Bearer")
		WriteError(w, http.StatusUnauthorized, ErrInvalidRequest, "missing or invalid authorization header")
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	userInfo, err := h.provider.GetUserInfo(token)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer error=\"invalid_token\"")
		WriteError(w, http.StatusUnauthorized, ErrInvalidRequest, "invalid or expired access token")
		return
	}

	WriteJSON(w, http.StatusOK, userInfo)
}

// Helper functions

func getClientCredentials(r *http.Request) (clientID, clientSecret string, err error) {
	// Try Basic authentication first
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Basic ") {
		encoded := strings.TrimPrefix(authHeader, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", "", err
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	// Fall back to form parameters
	clientID = r.PostFormValue("client_id")
	clientSecret = r.PostFormValue("client_secret")

	if clientID == "" || clientSecret == "" {
		return "", "", http.ErrNoCookie // Use as generic "not found" error
	}

	return clientID, clientSecret, nil
}

func parseScopes(scope string) []string {
	if scope == "" {
		return nil
	}
	return strings.Fields(scope)
}

func hasScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

func redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, state, errorCode, description string) {
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrInvalidRequest, "invalid redirect_uri")
		return
	}

	q := redirectURL.Query()
	q.Set("error", errorCode)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}
