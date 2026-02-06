package auth

import (
	"net/http"
	"sync"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/auth"
)

// OIDCLoginHandler handles passkey-based login for OIDC flows.
// This creates cookie-based sessions for OIDC authorization.
type OIDCLoginHandler struct {
	oidcHandler *OIDCHandler
	sessions    *oidcLoginSessionStore
}

// oidcLoginSessionStore stores WebAuthn sessions for OIDC login.
type oidcLoginSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*webauthn.SessionData
}

// NewOIDCLoginHandler creates a new OIDC login handler.
func NewOIDCLoginHandler(oidcHandler *OIDCHandler) *OIDCLoginHandler {
	return &OIDCLoginHandler{
		oidcHandler: oidcHandler,
		sessions: &oidcLoginSessionStore{
			sessions: make(map[string]*webauthn.SessionData),
		},
	}
}

// RegisterRoutes registers OIDC login routes.
func (h *OIDCLoginHandler) RegisterRoutes(e *echo.Echo) {
	// Login page and API endpoints
	e.GET("/login", h.handleLoginPage)
	e.GET("/auth/oidc/passkey/begin", h.handlePasskeyBegin)
	e.POST("/auth/oidc/passkey/finish", h.handlePasskeyFinish)
	e.POST("/auth/oidc/logout", h.handleLogout)
	e.GET("/auth/oidc/session", h.handleSessionStatus)
}

// handleLoginPage serves the login page.
// GET /login
func (h *OIDCLoginHandler) handleLoginPage(c echo.Context) error {
	// Check if already logged in
	session, err := h.oidcHandler.sessionManager.GetSessionFromRequest(c.Request())
	if err == nil && session != nil {
		// Already logged in, redirect to next or home
		next := c.QueryParam("next")
		if next != "" {
			return c.Redirect(http.StatusFound, next)
		}
		return c.Redirect(http.StatusFound, "/")
	}

	// Return a simple login page
	// In production, this would be served from the frontend
	next := c.QueryParam("next")
	return c.HTML(http.StatusOK, renderLoginPage(next))
}

// handlePasskeyBegin starts the passkey authentication flow.
// GET /auth/oidc/passkey/begin
func (h *OIDCLoginHandler) handlePasskeyBegin(c echo.Context) error {
	// Get user credentials from database
	user, err := h.oidcHandler.deps.DB.GetFirstUser()
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "no user configured",
		})
	}

	// Get passkey credentials
	webauthnCreds, err := h.oidcHandler.deps.DB.GetWebAuthnCredentialsByUserID(user.ID)
	if err != nil || len(webauthnCreds) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "no passkey credentials configured",
		})
	}

	// Create WebAuthn config
	cfg := h.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to initialize WebAuthn",
		})
	}

	// Create WebAuthn user
	webauthnUser := auth.NewWebAuthnUser(user.ID, user.Email, webauthnCreds)

	// Begin login
	options, session, err := wa.BeginLogin(webauthnUser)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to begin authentication",
		})
	}

	// Store session
	sessionID := uuid.New().String()
	h.sessions.mu.Lock()
	h.sessions.sessions[sessionID] = session
	h.sessions.mu.Unlock()

	return c.JSON(http.StatusOK, map[string]any{
		"session_id": sessionID,
		"options":    options,
	})
}

// handlePasskeyFinish completes the passkey authentication flow.
// POST /auth/oidc/passkey/finish
func (h *OIDCLoginHandler) handlePasskeyFinish(c echo.Context) error {
	sessionID := c.QueryParam("session_id")
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "session_id is required",
		})
	}

	// Get session
	h.sessions.mu.Lock()
	session, ok := h.sessions.sessions[sessionID]
	if ok {
		delete(h.sessions.sessions, sessionID)
	}
	h.sessions.mu.Unlock()

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid or expired session",
		})
	}

	// Parse credential assertion
	response, err := protocol.ParseCredentialRequestResponseBody(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "failed to parse credential response",
		})
	}

	// Get user
	user, err := h.oidcHandler.deps.DB.GetFirstUser()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get user",
		})
	}

	// Get credentials
	webauthnCreds, err := h.oidcHandler.deps.DB.GetWebAuthnCredentialsByUserID(user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get credentials",
		})
	}

	// Create WebAuthn config and user
	cfg := h.getWebAuthnConfig(c)
	wa, err := auth.NewWebAuthn(cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to initialize WebAuthn",
		})
	}

	webauthnUser := auth.NewWebAuthnUser(user.ID, user.Email, webauthnCreds)

	// Validate login
	credential, err := wa.ValidateLogin(webauthnUser, *session, response)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication failed",
		})
	}

	// Update sign count in database
	if err := h.oidcHandler.deps.DB.UpdateWebAuthnCredentialCounter(credential.ID, credential.Authenticator.SignCount); err != nil {
		// Log but don't fail - sign count is for replay protection
		_ = err
	}

	// Create OIDC session
	authSession, err := h.oidcHandler.sessionManager.CreateSession(user.ID, user.Email)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to create session",
		})
	}

	// Set session cookie
	h.oidcHandler.sessionManager.SetSessionCookie(c.Response(), authSession)

	// Get redirect URL
	next := c.QueryParam("next")
	if next == "" {
		next = "/"
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":     true,
		"redirect_to": next,
		"user": map[string]string{
			"id":    user.ID,
			"email": user.Email,
		},
	})
}

// handleLogout logs out the user.
// POST /auth/oidc/logout
func (h *OIDCLoginHandler) handleLogout(c echo.Context) error {
	session, err := h.oidcHandler.sessionManager.GetSessionFromRequest(c.Request())
	if err == nil && session != nil {
		_ = h.oidcHandler.sessionManager.DeleteSession(session.ID)
	}

	h.oidcHandler.sessionManager.ClearSessionCookie(c.Response())

	return c.JSON(http.StatusOK, map[string]string{
		"status": "logged out",
	})
}

// handleSessionStatus returns the current session status.
// GET /auth/oidc/session
func (h *OIDCLoginHandler) handleSessionStatus(c echo.Context) error {
	session, err := h.oidcHandler.sessionManager.GetSessionFromRequest(c.Request())
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"authenticated": false,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"authenticated": true,
		"user": map[string]string{
			"id":    session.UserID,
			"email": session.Email,
		},
	})
}

// getWebAuthnConfig creates WebAuthn config from the request.
func (h *OIDCLoginHandler) getWebAuthnConfig(c echo.Context) *auth.PasskeyConfig {
	origin := c.Request().Header.Get("Origin")
	if origin == "" {
		scheme := "https"
		if c.Request().TLS == nil {
			scheme = "http"
		}
		origin = scheme + "://" + c.Request().Host
	}

	// For OIDC SSO, we use the top-level domain (enbox.id) as RPID
	// This allows passkeys registered at Central to work at HQ
	host := c.Request().Host
	if colonIdx := len(host) - 1; colonIdx > 0 {
		for i := len(host) - 1; i >= 0; i-- {
			if host[i] == ':' {
				host = host[:i]
				break
			}
			if host[i] == ']' {
				break
			}
		}
	}

	// Use parent domain for cross-subdomain passkeys
	// e.g., hq.alice.enbox.id -> enbox.id
	rpID := getParentDomain(host)
	if rpID == "" {
		rpID = host
	}

	return &auth.PasskeyConfig{
		RPDisplayName: "Enbox HQ",
		RPID:          rpID,
		RPOrigin:      origin,
	}
}

// getParentDomain extracts the parent domain for cross-subdomain passkeys.
// e.g., "hq.alice.enbox.id" -> "enbox.id"
func getParentDomain(host string) string {
	parts := splitDomain(host)
	if len(parts) >= 2 {
		// Return last two parts (e.g., enbox.id)
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return host
}

func splitDomain(host string) []string {
	var parts []string
	current := ""
	for _, c := range host {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// renderLoginPage returns a simple login page HTML.
func renderLoginPage(next string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - Enbox HQ</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .login-container {
            background: white;
            border-radius: 16px;
            padding: 48px;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
            text-align: center;
            max-width: 400px;
            width: 90%;
        }
        h1 { color: #1a202c; margin-bottom: 8px; }
        p { color: #718096; margin-bottom: 32px; }
        .passkey-btn {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 16px 32px;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            width: 100%;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 12px;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .passkey-btn:hover { transform: translateY(-2px); box-shadow: 0 10px 20px -10px rgba(102, 126, 234, 0.5); }
        .passkey-btn:disabled { opacity: 0.7; cursor: not-allowed; transform: none; }
        .error { color: #e53e3e; margin-top: 16px; display: none; }
        .spinner { display: none; }
        .loading .spinner { display: inline-block; }
        .loading .btn-text { display: none; }
    </style>
</head>
<body>
    <div class="login-container">
        <h1>Enbox HQ</h1>
        <p>Sign in with your passkey</p>
        <button class="passkey-btn" id="loginBtn" onclick="login()">
            <span class="btn-text">🔐 Sign in with Passkey</span>
            <span class="spinner">Authenticating...</span>
        </button>
        <p class="error" id="error"></p>
    </div>
    <script>
        const next = '` + next + `' || '/';

        async function login() {
            const btn = document.getElementById('loginBtn');
            const error = document.getElementById('error');

            btn.disabled = true;
            btn.classList.add('loading');
            error.style.display = 'none';

            try {
                // Begin authentication
                const beginResp = await fetch('/auth/oidc/passkey/begin');
                if (!beginResp.ok) {
                    const err = await beginResp.json();
                    throw new Error(err.error || 'Failed to start authentication');
                }
                const { session_id, options } = await beginResp.json();

                // Convert base64url to ArrayBuffer
                options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
                if (options.publicKey.allowCredentials) {
                    options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(c => ({
                        ...c,
                        id: base64urlToBuffer(c.id)
                    }));
                }

                // Perform WebAuthn assertion
                const credential = await navigator.credentials.get(options);

                // Prepare response
                const body = {
                    id: credential.id,
                    rawId: bufferToBase64url(credential.rawId),
                    type: credential.type,
                    response: {
                        authenticatorData: bufferToBase64url(credential.response.authenticatorData),
                        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                        signature: bufferToBase64url(credential.response.signature),
                        userHandle: credential.response.userHandle ? bufferToBase64url(credential.response.userHandle) : null
                    }
                };

                // Finish authentication
                const finishResp = await fetch('/auth/oidc/passkey/finish?session_id=' + session_id + '&next=' + encodeURIComponent(next), {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body)
                });

                if (!finishResp.ok) {
                    const err = await finishResp.json();
                    throw new Error(err.error || 'Authentication failed');
                }

                const result = await finishResp.json();
                window.location.href = result.redirect_to || next;

            } catch (err) {
                console.error('Login error:', err);
                error.textContent = err.message || 'Authentication failed';
                error.style.display = 'block';
                btn.disabled = false;
                btn.classList.remove('loading');
            }
        }

        function base64urlToBuffer(base64url) {
            const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
            const pad = base64.length % 4;
            const padded = pad ? base64 + '='.repeat(4 - pad) : base64;
            const binary = atob(padded);
            const bytes = new Uint8Array(binary.length);
            for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
            return bytes.buffer;
        }

        function bufferToBase64url(buffer) {
            const bytes = new Uint8Array(buffer);
            let binary = '';
            for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
            return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
        }
    </script>
</body>
</html>`
}
