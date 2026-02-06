package oidc

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lirancohen/dex/internal/auth"
)

func setupTestHandlers(t *testing.T) (*Handlers, *Provider, *auth.SessionManager) {
	t.Helper()

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	config := DefaultConfig("https://hq.test.enbox.id")
	provider := NewProvider(config, kp)

	sessionManager := auth.NewSessionManager(auth.SessionManagerConfig{
		CookieName: "hq_session",
		MaxAge:     time.Hour,
	})

	handlers := NewHandlers(provider, sessionManager, "/login")

	return handlers, provider, sessionManager
}

func TestHandleDiscovery(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	t.Run("GET returns discovery document", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
		rec := httptest.NewRecorder()

		handlers.HandleDiscovery(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		var doc DiscoveryDocument
		if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if doc.Issuer != "https://hq.test.enbox.id" {
			t.Errorf("unexpected issuer: %s", doc.Issuer)
		}
	})

	t.Run("POST returns method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/.well-known/openid-configuration", nil)
		rec := httptest.NewRecorder()

		handlers.HandleDiscovery(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleJWKS(t *testing.T) {
	handlers, _, _ := setupTestHandlers(t)

	t.Run("GET returns JWKS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/jwks", nil)
		rec := httptest.NewRecorder()

		handlers.HandleJWKS(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", rec.Header().Get("Content-Type"))
		}

		var jwks JWKS
		if err := json.NewDecoder(rec.Body).Decode(&jwks); err != nil {
			t.Fatalf("failed to decode JWKS: %v", err)
		}

		if len(jwks.Keys) != 1 {
			t.Errorf("expected 1 key, got %d", len(jwks.Keys))
		}
	})
}

func TestHandleAuthorize(t *testing.T) {
	handlers, provider, sessionManager := setupTestHandlers(t)

	// Register a test client
	client := &Client{
		ID:           "test-client",
		Secret:       "test-secret",
		RedirectURIs: []string{"https://app.test/callback"},
		Name:         "Test App",
	}
	_ = provider.RegisterClient(client)

	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
			"client_id":     {"test-client"},
			"redirect_uri":  {"https://app.test/callback"},
			"response_type": {"code"},
			"scope":         {"openid profile"},
			"state":         {"test-state"},
		}.Encode(), nil)
		rec := httptest.NewRecorder()

		handlers.HandleAuthorize(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected 302, got %d", rec.Code)
		}

		location := rec.Header().Get("Location")
		if !strings.HasPrefix(location, "/login?next=") {
			t.Errorf("should redirect to login, got: %s", location)
		}
	})

	t.Run("authenticated issues auth code", func(t *testing.T) {
		// Create a session
		session, _ := sessionManager.CreateSession("user-123", "alice@example.com")

		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
			"client_id":     {"test-client"},
			"redirect_uri":  {"https://app.test/callback"},
			"response_type": {"code"},
			"scope":         {"openid profile"},
			"state":         {"test-state"},
		}.Encode(), nil)
		req.AddCookie(&http.Cookie{
			Name:  "hq_session",
			Value: session.ID,
		})
		rec := httptest.NewRecorder()

		handlers.HandleAuthorize(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected 302, got %d", rec.Code)
		}

		location := rec.Header().Get("Location")
		if !strings.HasPrefix(location, "https://app.test/callback?") {
			t.Errorf("should redirect to callback, got: %s", location)
		}

		// Parse redirect URL
		redirectURL, _ := url.Parse(location)
		if redirectURL.Query().Get("code") == "" {
			t.Error("redirect should include auth code")
		}
		if redirectURL.Query().Get("state") != "test-state" {
			t.Errorf("state should be preserved: %s", redirectURL.Query().Get("state"))
		}
	})

	t.Run("unknown client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
			"client_id":     {"unknown-client"},
			"redirect_uri":  {"https://app.test/callback"},
			"response_type": {"code"},
			"scope":         {"openid"},
		}.Encode(), nil)
		rec := httptest.NewRecorder()

		handlers.HandleAuthorize(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("invalid redirect URI", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
			"client_id":     {"test-client"},
			"redirect_uri":  {"https://evil.test/callback"},
			"response_type": {"code"},
			"scope":         {"openid"},
		}.Encode(), nil)
		rec := httptest.NewRecorder()

		handlers.HandleAuthorize(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("unsupported response type", func(t *testing.T) {
		session, _ := sessionManager.CreateSession("user-123", "alice@example.com")

		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
			"client_id":     {"test-client"},
			"redirect_uri":  {"https://app.test/callback"},
			"response_type": {"token"}, // Implicit flow not supported
			"scope":         {"openid"},
			"state":         {"test-state"},
		}.Encode(), nil)
		req.AddCookie(&http.Cookie{
			Name:  "hq_session",
			Value: session.ID,
		})
		rec := httptest.NewRecorder()

		handlers.HandleAuthorize(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected 302 redirect with error, got %d", rec.Code)
		}

		location := rec.Header().Get("Location")
		redirectURL, _ := url.Parse(location)
		if redirectURL.Query().Get("error") != ErrUnsupportedResponseType {
			t.Errorf("should return unsupported_response_type error: %s", location)
		}
	})

	t.Run("missing openid scope", func(t *testing.T) {
		session, _ := sessionManager.CreateSession("user-123", "alice@example.com")

		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
			"client_id":     {"test-client"},
			"redirect_uri":  {"https://app.test/callback"},
			"response_type": {"code"},
			"scope":         {"profile"}, // Missing openid
			"state":         {"test-state"},
		}.Encode(), nil)
		req.AddCookie(&http.Cookie{
			Name:  "hq_session",
			Value: session.ID,
		})
		rec := httptest.NewRecorder()

		handlers.HandleAuthorize(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected 302 redirect with error, got %d", rec.Code)
		}

		location := rec.Header().Get("Location")
		redirectURL, _ := url.Parse(location)
		if redirectURL.Query().Get("error") != ErrInvalidScope {
			t.Errorf("should return invalid_scope error: %s", location)
		}
	})
}

func TestHandleToken(t *testing.T) {
	handlers, provider, sessionManager := setupTestHandlers(t)

	client := &Client{
		ID:           "token-client",
		Secret:       "token-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	// Create a valid auth code
	session, _ := sessionManager.CreateSession("user-456", "bob@example.com")
	authCode, _ := provider.CreateAuthorizationCode(
		"token-client",
		session.UserID,
		session.Email,
		session.Email,
		"https://app.test/callback",
		[]string{"openid", "profile"},
		"nonce-123",
	)

	t.Run("exchange code with Basic auth", func(t *testing.T) {
		form := url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {authCode.Code},
			"redirect_uri": {"https://app.test/callback"},
		}

		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("token-client:token-secret")))
		rec := httptest.NewRecorder()

		handlers.HandleToken(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Check cache headers
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("should set Cache-Control: no-store")
		}

		var tokenResp TokenResponse
		if err := json.NewDecoder(rec.Body).Decode(&tokenResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if tokenResp.AccessToken == "" {
			t.Error("access token should not be empty")
		}
		if tokenResp.IDToken == "" {
			t.Error("ID token should not be empty")
		}
		if tokenResp.TokenType != "Bearer" {
			t.Errorf("unexpected token type: %s", tokenResp.TokenType)
		}
	})

	t.Run("exchange code with form credentials", func(t *testing.T) {
		// Create another auth code
		authCode2, _ := provider.CreateAuthorizationCode(
			"token-client",
			"user-789",
			"charlie@example.com",
			"Charlie",
			"https://app.test/callback",
			[]string{"openid"},
			"",
		)

		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {authCode2.Code},
			"redirect_uri":  {"https://app.test/callback"},
			"client_id":     {"token-client"},
			"client_secret": {"token-secret"},
		}

		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handlers.HandleToken(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid grant type", func(t *testing.T) {
		form := url.Values{
			"grant_type": {"password"},
		}

		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handlers.HandleToken(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("invalid client credentials", func(t *testing.T) {
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {"some-code"},
			"redirect_uri":  {"https://app.test/callback"},
			"client_id":     {"token-client"},
			"client_secret": {"wrong-secret"},
		}

		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handlers.HandleToken(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("invalid code", func(t *testing.T) {
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {"invalid-code"},
			"redirect_uri":  {"https://app.test/callback"},
			"client_id":     {"token-client"},
			"client_secret": {"token-secret"},
		}

		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handlers.HandleToken(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
}

func TestHandleUserInfo(t *testing.T) {
	handlers, provider, sessionManager := setupTestHandlers(t)

	client := &Client{
		ID:           "userinfo-client",
		Secret:       "userinfo-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	// Get an access token
	session, _ := sessionManager.CreateSession("user-info", "userinfo@example.com")
	authCode, _ := provider.CreateAuthorizationCode(
		"userinfo-client",
		session.UserID,
		session.Email,
		session.Email,
		"https://app.test/callback",
		[]string{"openid", "email"},
		"",
	)
	tokenResp, _ := provider.ExchangeAuthorizationCode(
		authCode.Code,
		"userinfo-client",
		"https://app.test/callback",
	)

	t.Run("GET with valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		rec := httptest.NewRecorder()

		handlers.HandleUserInfo(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var userInfo UserInfo
		if err := json.NewDecoder(rec.Body).Decode(&userInfo); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if userInfo.Sub != "user-info" {
			t.Errorf("unexpected sub: %s", userInfo.Sub)
		}
		if userInfo.Email != "userinfo@example.com" {
			t.Errorf("unexpected email: %s", userInfo.Email)
		}
	})

	t.Run("POST with valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/oauth/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		rec := httptest.NewRecorder()

		handlers.HandleUserInfo(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
		rec := httptest.NewRecorder()

		handlers.HandleUserInfo(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}

		if rec.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Error("should set WWW-Authenticate header")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		handlers.HandleUserInfo(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}

		if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "invalid_token") {
			t.Error("should indicate invalid_token in WWW-Authenticate")
		}
	})

	t.Run("wrong authorization type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()

		handlers.HandleUserInfo(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("parseScopes", func(t *testing.T) {
		tests := []struct {
			input    string
			expected []string
		}{
			{"", nil},
			{"openid", []string{"openid"}},
			{"openid profile email", []string{"openid", "profile", "email"}},
			{"openid  profile", []string{"openid", "profile"}}, // Extra space
		}

		for _, tt := range tests {
			got := parseScopes(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseScopes(%q) = %v, want %v", tt.input, got, tt.expected)
				continue
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("parseScopes(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		}
	})

	t.Run("hasScope", func(t *testing.T) {
		scopes := []string{"openid", "profile", "email"}

		if !hasScope(scopes, "openid") {
			t.Error("should find 'openid'")
		}
		if !hasScope(scopes, "email") {
			t.Error("should find 'email'")
		}
		if hasScope(scopes, "offline_access") {
			t.Error("should not find 'offline_access'")
		}
		if hasScope(nil, "openid") {
			t.Error("should not find in nil slice")
		}
	})
}
