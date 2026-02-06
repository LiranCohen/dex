# Passkey SSO: HQ Implementation Plan

**Status**: Planning
**Created**: 2026-02-06
**Last Updated**: 2026-02-06
**Depends On**: E2E Integration (002), Forgejo Migration (001)
**Coordination**: Central work tracked in `dex-saas/hq-plan/PASSKEY-SSO.md`

---

## Overview

This document specifies HQ's role in the Passkey SSO system. The goal is unified authentication across the Enbox ecosystem using a single passkey registered at `enbox.id`.

### Architecture Summary

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              INTERNET                                    │
│                                                                          │
│    ┌──────────────────────────────────────────────────────────────┐     │
│    │                    central.enbox.id                          │     │
│    │                                                              │     │
│    │  - User signup/billing                                       │     │
│    │  - Passkey registration (rpId: enbox.id)                     │     │
│    │  - Enrollment key generation                                 │     │
│    │  - Stores user public keys                                   │     │
│    │  - Provides public key to HQ during enrollment               │     │
│    └──────────────────────────────────────────────────────────────┘     │
│                                    │                                     │
│                                    │ Enrollment (one-time)               │
│                                    │ - namespace, tokens, PUBLIC KEY     │
│                                    ▼                                     │
└────────────────────────────────────┼─────────────────────────────────────┘
                                     │
┌────────────────────────────────────┼─────────────────────────────────────┐
│                              USER'S MESH                                 │
│                                    │                                     │
│    ┌───────────────────────────────▼──────────────────────────────┐     │
│    │                     hq.alice.enbox.id                        │     │
│    │                                                              │     │
│    │  ┌─────────────────────────────────────────────────────┐    │     │
│    │  │                    HQ Server                        │    │     │
│    │  │                                                     │    │     │
│    │  │  - Stores owner's public key (from enrollment)      │    │     │
│    │  │  - Passkey verification endpoint                    │    │     │
│    │  │  - OIDC Provider for services                       │    │     │
│    │  │  - Session management                               │    │     │
│    │  └─────────────────────────────────────────────────────┘    │     │
│    │                          │                                   │     │
│    │              ┌───────────┴───────────┐                      │     │
│    │              │ OIDC                  │ OIDC                  │     │
│    │              ▼                       ▼                       │     │
│    │  ┌─────────────────┐     ┌─────────────────┐                │     │
│    │  │    Forgejo      │     │  Future Service │                │     │
│    │  │                 │     │                 │                │     │
│    │  │ git.alice.enbox │     │ svc.alice.enbox │                │     │
│    │  └─────────────────┘     └─────────────────┘                │     │
│    └──────────────────────────────────────────────────────────────┘     │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### Authentication Flows

**Flow 1: Direct Passkey (for HQ UI)**
```
1. User visits hq.alice.enbox.id
2. HQ: "Authenticate" → WebAuthn challenge (rpId: enbox.id)
3. User taps passkey
4. HQ verifies signature against stored public key
5. HQ creates session
```

**Flow 2: OIDC (for Forgejo and other services)**
```
1. User visits git.alice.enbox.id
2. Forgejo: "Who are you?" → Redirect to hq.alice.enbox.id/oauth/authorize
3. HQ: Not logged in? → Passkey challenge
4. User taps passkey
5. HQ: Issues authorization code → Redirect to Forgejo
6. Forgejo: Exchanges code for tokens
7. Forgejo: Creates session for user
```

---

## Current Status

| Component | Status | Notes |
|-----------|--------|-------|
| Enrollment receives public key | ❌ Pending | Central must include in response |
| Public key storage | ❌ Pending | Add to HQ config/database |
| Passkey verification | ❌ Pending | WebAuthn assertion verification |
| OIDC Provider | ❌ Pending | Authorization + token endpoints |
| Forgejo OIDC config | ❌ Pending | Configure Forgejo to use HQ |
| Session management | ❌ Pending | Cookie-based sessions |

---

## Phase 1: Enrollment Enhancement

**Goal**: Receive and store user's passkey public key during enrollment.

### 1.1 Updated Enrollment Response

Central will include the user's passkey credential in the enrollment response:

```go
// EnrollmentResponse additions (cmd/dex/enroll.go)
type EnrollmentResponse struct {
    // ... existing fields ...

    // Owner identity for authentication
    Owner struct {
        UserID      string `json:"user_id"`       // Central's user ID
        Email       string `json:"email"`         // User's email
        DisplayName string `json:"display_name"`  // For UI

        // Passkey credential (WebAuthn)
        Passkey struct {
            CredentialID     string `json:"credential_id"`      // Base64 encoded
            PublicKey        string `json:"public_key"`         // Base64 encoded COSE key
            PublicKeyAlg     int    `json:"public_key_alg"`     // COSE algorithm (-7 for ES256)
            AttestationType  string `json:"attestation_type"`   // "none", "direct", etc.
            AAGUID          string `json:"aaguid,omitempty"`    // Authenticator ID
            SignCount       uint32 `json:"sign_count"`          // For replay protection
        } `json:"passkey"`
    } `json:"owner"`
}
```

### 1.2 Config Storage

Store owner identity in HQ config:

```go
// Config additions (cmd/dex/config.go)
type Config struct {
    // ... existing fields ...

    Owner OwnerConfig `json:"owner"`
}

type OwnerConfig struct {
    UserID      string        `json:"user_id"`
    Email       string        `json:"email"`
    DisplayName string        `json:"display_name"`
    Passkey     PasskeyConfig `json:"passkey"`
}

type PasskeyConfig struct {
    CredentialID    []byte `json:"credential_id"`
    PublicKey       []byte `json:"public_key"`
    PublicKeyAlg    int    `json:"public_key_alg"`
    SignCount       uint32 `json:"sign_count"`
}
```

### 1.3 Implementation Tasks

- [ ] Update `EnrollmentResponse` struct to include `owner` field
- [ ] Update `Config` struct with `OwnerConfig`
- [ ] Update `buildConfigFromResponse()` to populate owner config
- [ ] Add migration for existing configs (owner field optional initially)

---

## Phase 2: Passkey Verification

**Goal**: Implement WebAuthn assertion verification for direct passkey auth.

### 2.1 Dependencies

```go
// go.mod addition
require github.com/go-webauthn/webauthn v0.10.0
```

### 2.2 WebAuthn Verifier

```go
// internal/auth/passkey.go
package auth

import (
    "github.com/go-webauthn/webauthn/protocol"
    "github.com/go-webauthn/webauthn/webauthn"
)

type PasskeyVerifier struct {
    webauthn *webauthn.WebAuthn
    owner    *OwnerCredential
}

type OwnerCredential struct {
    ID        []byte
    PublicKey []byte
    Algorithm int
    SignCount uint32
}

// NewPasskeyVerifier creates a verifier for the HQ owner
func NewPasskeyVerifier(rpID, rpOrigin string, owner *OwnerCredential) (*PasskeyVerifier, error) {
    wconfig := &webauthn.Config{
        RPDisplayName: "Enbox HQ",
        RPID:          rpID,      // "enbox.id"
        RPOrigins:     []string{rpOrigin}, // "https://hq.alice.enbox.id"
    }

    w, err := webauthn.New(wconfig)
    if err != nil {
        return nil, err
    }

    return &PasskeyVerifier{webauthn: w, owner: owner}, nil
}

// BeginAuthentication generates a challenge for passkey auth
func (v *PasskeyVerifier) BeginAuthentication() (*protocol.CredentialAssertion, string, error) {
    // Generate challenge, store session
}

// FinishAuthentication verifies the passkey response
func (v *PasskeyVerifier) FinishAuthentication(sessionData string, response *protocol.ParsedCredentialAssertionData) error {
    // Verify signature against stored public key
    // Update sign count for replay protection
}
```

### 2.3 Auth Endpoints

```go
// internal/server/auth.go

// GET /auth/passkey/begin
// Returns: { challenge, rpId, allowCredentials, timeout }
func (s *Server) handlePasskeyBegin(w http.ResponseWriter, r *http.Request) {
    options, session, err := s.passkey.BeginAuthentication()
    // Store session in cookie or memory
    // Return options as JSON
}

// POST /auth/passkey/finish
// Body: { id, rawId, response: { authenticatorData, clientDataJSON, signature } }
// Returns: { success, session_token }
func (s *Server) handlePasskeyFinish(w http.ResponseWriter, r *http.Request) {
    // Verify assertion
    // Create session
    // Set session cookie
}
```

### 2.4 Session Management

```go
// internal/auth/session.go
package auth

type SessionManager struct {
    store  SessionStore
    maxAge time.Duration
}

type Session struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
}

// CreateSession creates a new authenticated session
func (m *SessionManager) CreateSession(userID, email string) (*Session, error)

// ValidateSession checks if a session is valid
func (m *SessionManager) ValidateSession(sessionID string) (*Session, error)

// Middleware returns HTTP middleware that validates sessions
func (m *SessionManager) Middleware(next http.Handler) http.Handler
```

### 2.5 Implementation Tasks

- [ ] Add `go-webauthn/webauthn` dependency
- [ ] Create `internal/auth/passkey.go` with verifier
- [ ] Create `internal/auth/session.go` with session management
- [ ] Add `/auth/passkey/begin` endpoint
- [ ] Add `/auth/passkey/finish` endpoint
- [ ] Add session cookie handling
- [ ] Add auth middleware for protected routes

---

## Phase 3: OIDC Provider

**Goal**: Implement OIDC provider so services like Forgejo can authenticate via HQ.

### 3.1 OIDC Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /.well-known/openid-configuration` | OIDC discovery document |
| `GET /oauth/authorize` | Authorization endpoint |
| `POST /oauth/token` | Token endpoint |
| `GET /oauth/userinfo` | User info endpoint |
| `GET /oauth/jwks` | JSON Web Key Set |

### 3.2 Discovery Document

```json
// GET /.well-known/openid-configuration
{
  "issuer": "https://hq.alice.enbox.id",
  "authorization_endpoint": "https://hq.alice.enbox.id/oauth/authorize",
  "token_endpoint": "https://hq.alice.enbox.id/oauth/token",
  "userinfo_endpoint": "https://hq.alice.enbox.id/oauth/userinfo",
  "jwks_uri": "https://hq.alice.enbox.id/oauth/jwks",
  "response_types_supported": ["code"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"],
  "scopes_supported": ["openid", "profile", "email"],
  "token_endpoint_auth_methods_supported": ["client_secret_basic", "client_secret_post"],
  "claims_supported": ["sub", "email", "name", "preferred_username"]
}
```

### 3.3 OIDC Provider Implementation

```go
// internal/auth/oidc/provider.go
package oidc

import (
    "crypto/rsa"
    "time"
)

type Provider struct {
    issuer     string          // https://hq.alice.enbox.id
    privateKey *rsa.PrivateKey // For signing tokens
    clients    map[string]*Client
    sessions   SessionStore
}

type Client struct {
    ID          string   `json:"client_id"`
    Secret      string   `json:"client_secret"`
    RedirectURI string   `json:"redirect_uri"`
    Name        string   `json:"name"`
}

// RegisterClient adds a new OIDC client (e.g., Forgejo)
func (p *Provider) RegisterClient(client *Client) error

// Authorize handles the authorization request
// If user not authenticated, triggers passkey flow
// If authenticated, issues authorization code
func (p *Provider) Authorize(w http.ResponseWriter, r *http.Request)

// Token exchanges authorization code for tokens
func (p *Provider) Token(w http.ResponseWriter, r *http.Request)

// UserInfo returns user claims
func (p *Provider) UserInfo(w http.ResponseWriter, r *http.Request)
```

### 3.4 Token Structure

```go
// ID Token claims
type IDTokenClaims struct {
    jwt.RegisteredClaims
    Email             string `json:"email"`
    EmailVerified     bool   `json:"email_verified"`
    Name              string `json:"name"`
    PreferredUsername string `json:"preferred_username"`
}

// Access Token (opaque, stored server-side)
type AccessToken struct {
    Token     string    `json:"token"`
    UserID    string    `json:"user_id"`
    ClientID  string    `json:"client_id"`
    Scopes    []string  `json:"scopes"`
    ExpiresAt time.Time `json:"expires_at"`
}
```

### 3.5 Key Management

```go
// internal/auth/oidc/keys.go

// GenerateKeyPair creates RSA key pair for token signing
// Keys stored in HQ's data directory
func GenerateKeyPair(dataDir string) (*rsa.PrivateKey, error)

// LoadKeyPair loads existing keys or generates new ones
func LoadKeyPair(dataDir string) (*rsa.PrivateKey, error)

// JWKS returns the public key in JWKS format
func JWKS(publicKey *rsa.PublicKey) ([]byte, error)
```

### 3.6 Implementation Tasks

- [ ] Create `internal/auth/oidc/provider.go`
- [ ] Create `internal/auth/oidc/keys.go` for JWT signing keys
- [ ] Implement discovery endpoint
- [ ] Implement authorization endpoint with passkey trigger
- [ ] Implement token endpoint
- [ ] Implement userinfo endpoint
- [ ] Implement JWKS endpoint
- [ ] Add client registration (for Forgejo)

---

## Phase 4: Forgejo Integration

**Goal**: Configure Forgejo to use HQ as OIDC provider.

### 4.1 Forgejo OIDC Configuration

Update `app.ini` generation in `internal/forgejo/config.go`:

```ini
[oauth2]
ENABLED = true

[oauth2_client]
OPENID_CONNECT_SCOPES = openid profile email
ENABLE_AUTO_REGISTRATION = true
USERNAME = preferred_username

; HQ as OIDC provider - configured at runtime via API
```

### 4.2 Auto-Register OIDC Provider

During Forgejo bootstrap, register HQ as OIDC provider:

```go
// internal/forgejo/setup.go

func (m *Manager) configureOIDC(ctx context.Context, hqURL string) error {
    adminToken, err := m.AdminToken()
    if err != nil {
        return err
    }

    // Register HQ as OAuth2 provider
    body := map[string]interface{}{
        "name":               "HQ",
        "provider":           "openidConnect",
        "client_id":          "forgejo",
        "client_secret":      m.generateClientSecret(),
        "open_id_connect_auto_discovery_url": hqURL + "/.well-known/openid-configuration",
        "skip_local_2fa":     true,
        "scopes":             []string{"openid", "profile", "email"},
    }

    _, err = m.apiRequest(ctx, adminToken, "POST", "/api/v1/admin/oauth2", body)
    return err
}
```

### 4.3 Client Registration on HQ

When Forgejo starts, HQ needs to register it as OIDC client:

```go
// internal/server/services.go

func (s *Server) registerForgejoClient() error {
    client := &oidc.Client{
        ID:          "forgejo",
        Secret:      s.generateForgejoSecret(),
        RedirectURI: s.forgejoURL + "/user/oauth2/HQ/callback",
        Name:        "Forgejo",
    }

    return s.oidcProvider.RegisterClient(client)
}
```

### 4.4 Implementation Tasks

- [ ] Update Forgejo `app.ini` template with OAuth2 settings
- [ ] Add `configureOIDC()` to Forgejo bootstrap
- [ ] Generate and store Forgejo client credentials
- [ ] Register Forgejo as OIDC client on HQ startup
- [ ] Test end-to-end flow

---

## Phase 5: UI Integration

**Goal**: Add login UI to HQ for passkey authentication.

### 5.1 Login Page

```
hq.alice.enbox.id/login

┌─────────────────────────────────────────┐
│                                         │
│              Enbox HQ                   │
│                                         │
│    ┌─────────────────────────────┐      │
│    │                             │      │
│    │      [Passkey Icon]         │      │
│    │                             │      │
│    │   Sign in with Passkey      │      │
│    │                             │      │
│    └─────────────────────────────┘      │
│                                         │
│         alice@example.com               │
│                                         │
└─────────────────────────────────────────┘
```

### 5.2 Login Flow

```javascript
// Static JS in HQ binary
async function login() {
    // 1. Get challenge from HQ
    const beginResp = await fetch('/auth/passkey/begin');
    const options = await beginResp.json();

    // 2. Prompt passkey
    const credential = await navigator.credentials.get({
        publicKey: {
            ...options,
            rpId: 'enbox.id',  // Parent domain
        }
    });

    // 3. Verify with HQ
    const finishResp = await fetch('/auth/passkey/finish', {
        method: 'POST',
        body: JSON.stringify(credential),
    });

    if (finishResp.ok) {
        window.location.href = '/dashboard';
    }
}
```

### 5.3 Implementation Tasks

- [ ] Create login page HTML/CSS/JS
- [ ] Embed in HQ binary (embed.FS)
- [ ] Add login route
- [ ] Add redirect-after-login logic
- [ ] Add logout endpoint

---

## Security Considerations

### 6.1 Passkey Security

| Concern | Mitigation |
|---------|------------|
| Replay attacks | Verify and increment `signCount` |
| Origin validation | Validate `origin` in clientDataJSON matches expected |
| Challenge freshness | Generate cryptographically random challenges, expire after 5 min |

### 6.2 OIDC Security

| Concern | Mitigation |
|---------|------------|
| Token theft | Short-lived access tokens (1 hour), refresh tokens with rotation |
| CSRF | `state` parameter validation in authorization flow |
| Code injection | PKCE required for public clients |
| Token signing | RSA-256 with keys stored securely |

### 6.3 Mesh Security

| Concern | Mitigation |
|---------|------------|
| Non-mesh access | HQ only accessible via mesh (WireGuard) |
| Rogue mesh nodes | Passkey required, mesh alone isn't sufficient |

---

## API Reference

### Authentication Endpoints

#### Begin Passkey Auth
```
GET /auth/passkey/begin

Response:
{
    "challenge": "base64...",
    "timeout": 60000,
    "rpId": "enbox.id",
    "allowCredentials": [{
        "id": "base64...",
        "type": "public-key"
    }],
    "userVerification": "preferred"
}
```

#### Finish Passkey Auth
```
POST /auth/passkey/finish
Content-Type: application/json

{
    "id": "base64...",
    "rawId": "base64...",
    "type": "public-key",
    "response": {
        "authenticatorData": "base64...",
        "clientDataJSON": "base64...",
        "signature": "base64..."
    }
}

Response:
{
    "success": true,
    "user": {
        "id": "user-123",
        "email": "alice@example.com"
    }
}

Set-Cookie: hq_session=...; HttpOnly; Secure; SameSite=Lax
```

### OIDC Endpoints

#### Authorization
```
GET /oauth/authorize?
    client_id=forgejo&
    redirect_uri=https://git.alice.enbox.id/user/oauth2/HQ/callback&
    response_type=code&
    scope=openid%20profile%20email&
    state=random-state

Response (if authenticated):
302 Redirect to redirect_uri?code=auth-code&state=random-state

Response (if not authenticated):
302 Redirect to /login?next=/oauth/authorize?...
```

#### Token
```
POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&
code=auth-code&
redirect_uri=https://git.alice.enbox.id/user/oauth2/HQ/callback&
client_id=forgejo&
client_secret=secret

Response:
{
    "access_token": "...",
    "token_type": "Bearer",
    "expires_in": 3600,
    "id_token": "eyJ...",
    "scope": "openid profile email"
}
```

---

## Testing Checklist

### Unit Tests
- [ ] Passkey challenge generation
- [ ] Passkey assertion verification
- [ ] JWT generation and validation
- [ ] OIDC authorization code flow
- [ ] Session management

### Integration Tests
- [ ] End-to-end passkey login
- [ ] OIDC flow with test client
- [ ] Forgejo OIDC integration
- [ ] Session persistence across requests

### Manual Tests
- [ ] Fresh enrollment with passkey
- [ ] Login to HQ with passkey
- [ ] Access Forgejo, redirected to HQ, passkey, back to Forgejo
- [ ] Multiple browser sessions
- [ ] Session expiry and re-auth

---

## File Structure

```
internal/
├── auth/
│   ├── passkey.go       # WebAuthn verification
│   ├── session.go       # Session management
│   └── oidc/
│       ├── provider.go  # OIDC provider implementation
│       ├── keys.go      # JWT signing keys
│       ├── authorize.go # Authorization endpoint
│       ├── token.go     # Token endpoint
│       └── userinfo.go  # UserInfo endpoint
├── server/
│   ├── auth.go          # Auth route handlers
│   └── oidc.go          # OIDC route handlers
└── forgejo/
    └── setup.go         # OIDC client registration (update)

cmd/dex/
├── config.go            # Owner config (update)
└── enroll.go            # Parse owner from response (update)

web/
├── login.html           # Login page
└── static/
    └── auth.js          # Passkey JS
```

---

## Timeline & Dependencies

```
Phase 1: Enrollment Enhancement
    ├── Depends on: Central including public key in enrollment
    └── Duration: 1-2 days

Phase 2: Passkey Verification
    ├── Depends on: Phase 1
    └── Duration: 2-3 days

Phase 3: OIDC Provider
    ├── Depends on: Phase 2 (shares session management)
    └── Duration: 3-4 days

Phase 4: Forgejo Integration
    ├── Depends on: Phase 3
    └── Duration: 1-2 days

Phase 5: UI Integration
    ├── Depends on: Phase 2
    └── Duration: 1-2 days
```

---

## Open Questions

1. **Multi-device support**: Should HQ support multiple passkeys (e.g., phone + laptop)?
   - Recommendation: Start with single passkey, add multi-device later

2. **Passkey rotation**: What if user loses their passkey?
   - Recommendation: Re-enroll from Central (which would have recovery options)

3. **Multiple users**: Should HQ support team members?
   - Recommendation: Defer to future phase, single-owner for MVP

4. **Offline access**: What if Central is down during passkey verification?
   - Answer: HQ verifies locally using stored public key, no Central dependency

---

## Coordination with Central

Central is responsible for:
1. Registering passkeys with `rpId: enbox.id`
2. Including passkey public key in enrollment response
3. Storing passkeys securely for potential recovery flows

See `dex-saas/hq-plan/PASSKEY-SSO.md` for Central's implementation plan.
