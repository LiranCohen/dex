# Poindexter Onboarding Redesign Plan

## Executive Summary

This document outlines a redesigned onboarding flow where Poindexter has its own dedicated accounts and credentials, rather than using the user's personal accounts. The goal is to give Poindexter a distinct identity while maintaining user oversight and access.

**Proposed Flow:**
1. Establish Tailscale connection/account
2. Provision Anthropic API key
3. Create/provision Gmail account for Poindexter (user has access)
4. Create/provision GitHub identity for Poindexter (user has access)

---

## Current State Analysis

### What Exists Today

| Component | Current Implementation |
|-----------|------------------------|
| **Tailscale** | Fully integrated. User creates Tailscale account, device joins tailnet via `tailscale up`. |
| **Anthropic API** | User manually provides their personal API key during onboarding. |
| **GitHub** | User manually creates PAT from their personal account. |
| **Email/Gmail** | No integration. Resend exists for outbound transactional email only. |
| **Authentication** | WebAuthn/Passkeys (biometric, no passwords). Single-user model. |

### Current Onboarding Flow (for reference)

```
Phase 1 (Installation):
1. Run installer on VPS
2. Temporary Cloudflare tunnel with PIN
3. User chooses Tailscale (recommended) or Cloudflare
4. Device joins tailnet or permanent tunnel created

Phase 2 (Web UI Setup):
1. Passkey registration (Face ID, Touch ID, etc.)
2. GitHub PAT input (validated with API call)
3. Anthropic API key input (validated with API call)
4. Setup complete
```

---

## Proposed New Onboarding Flow

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         USER (Operator)                              │
│                                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │
│  │  Tailscale   │  │   Gmail      │  │   GitHub     │               │
│  │   Account    │  │   Account    │  │   Account    │               │
│  │  (Personal)  │  │ (Dedicated)  │  │  (App/Bot)   │               │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘               │
│         │                 │                 │                        │
│         │   ┌─────────────┴─────────────────┘                        │
│         │   │                                                        │
│         ▼   ▼                                                        │
│  ┌──────────────────────────────────────────────────────────┐       │
│  │                    POINDEXTER                             │       │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────────────┐  │       │
│  │  │ Tailscale  │  │   Gmail    │  │ GitHub App/Token   │  │       │
│  │  │   Access   │  │   OAuth    │  │    Integration     │  │       │
│  │  └────────────┘  └────────────┘  └────────────────────┘  │       │
│  │                                                           │       │
│  │  ┌────────────────────────────────────────────────────┐  │       │
│  │  │              Anthropic API Access                   │  │       │
│  │  └────────────────────────────────────────────────────┘  │       │
│  └──────────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Step 1: Tailscale Connection

### Current Implementation (Keep As-Is)

The current Tailscale integration is well-designed and should remain unchanged:

1. Installer runs `tailscale up --hostname=dex --operator=<user>`
2. User authenticates via Tailscale login URL (QR code provided)
3. Device joins user's tailnet
4. `tailscale serve --bg --https=443 http://127.0.0.1:8080` exposes Poindexter

### Why No Changes Needed

- Tailscale is for **network access**, not identity
- The Poindexter server joins the user's tailnet as a device
- This is the correct model - the user's network contains Poindexter
- No benefit to Poindexter having its "own" Tailscale account

### Enhancement Opportunity

Consider supporting **Tailscale OAuth** in future for identity verification:
- Tailscale provides identity headers that could supplement WebAuthn
- Would allow "login with Tailscale" as an alternative to passkeys

---

## Step 2: Anthropic API Key

### Options Analysis

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A: User Provides Key** | Current model - user provides their own API key | Simple, user controls billing | User's key used for AI identity |
| **B: Operator Provisions** | Dex operator/company provides pre-configured key | Centralized billing, controlled access | Requires operator infrastructure |
| **C: Per-Instance Key** | Each Poindexter instance gets its own API key | Isolated billing, clear attribution | Requires account per instance |

### Recommendation: Hybrid Approach

```
┌─────────────────────────────────────────────────────────────┐
│                    Anthropic API Key Sources                 │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Priority 1: Operator-Provisioned Key                        │
│  ├── Pre-configured in image/deployment                     │
│  ├── Managed via Doppler or environment                     │
│  └── Billing to operator                                     │
│                                                              │
│  Priority 2: User-Provided Key (Fallback)                    │
│  ├── Traditional onboarding flow                            │
│  ├── User controls their own billing                        │
│  └── Validates on entry                                      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Implementation Changes

1. Check for pre-configured key at startup (environment variable or secrets file)
2. Skip API key step in onboarding if already configured
3. Display "API Key: Provided by Operator" in settings
4. Allow user to override if desired

**Files to modify:**
- `/internal/api/setup.go` - Add pre-configured key detection
- `/frontend/src/components/Onboarding.tsx` - Conditional skip of API key step
- `/cmd/dex/main.go` - Priority loading (operator key > user key)

---

## Step 3: Gmail Account for Poindexter

### Critical Constraints

**Google does NOT allow programmatic creation of consumer Gmail accounts.**

From research:
- No API exists for creating `@gmail.com` accounts
- Browser automation violates Google ToS
- Only Google Workspace Admin SDK can create accounts (within owned domains)

### Viable Options

#### Option A: Dedicated Gmail (Manual Creation + OAuth)

User creates a dedicated Gmail account manually, then authorizes Poindexter.

```
Onboarding Flow:
1. User creates poindexter-<unique>@gmail.com manually
2. User adds account to their password manager
3. Poindexter initiates Google OAuth flow
4. User authorizes Gmail read/send scopes
5. OAuth refresh token stored in secrets.json
```

**Pros:**
- User has full access to the account
- Standard OAuth flow
- No ToS violations

**Cons:**
- Manual account creation step
- User must remember another password
- Account could be flagged if used unusually

#### Option B: Google Workspace Domain (Recommended for Production)

Operator provisions accounts on a dedicated domain (e.g., `poindexter.ai`).

```
Setup Flow:
1. Operator owns Google Workspace domain
2. Admin SDK creates user@poindexter.ai
3. Service account has domain-wide delegation
4. OAuth tokens managed automatically
```

**Pros:**
- Fully automated after initial setup
- Professional appearance
- Centralized management
- Clear separation from user's personal email

**Cons:**
- Requires Google Workspace subscription (~$6/user/month)
- Operator must manage the domain
- More complex infrastructure

#### Option C: No Gmail (Use Existing Email Infrastructure)

Don't create a Gmail account. Use Resend for sending, webhooks for receiving.

```
Architecture:
- Outbound: Resend API (already integrated)
- Inbound: Webhook endpoint for receiving emails
- Address: poindexter-<id>@mail.poindexter.ai (Resend domain)
```

**Pros:**
- No Google dependency
- Already have Resend integration
- Full control over email identity

**Cons:**
- No access to existing Gmail features (Drive, Calendar, etc.)
- Limited ecosystem integration
- Custom infrastructure required

### Recommendation: Phased Approach

**Phase 1 (MVP):** Option C - Use existing email infrastructure
- Leverage Resend for outbound
- Add inbound webhook support
- Custom domain for Poindexter email addresses

**Phase 2 (Enhancement):** Option A - Add Google OAuth
- For users who want Gmail integration
- Optional feature, not required
- User creates account manually, Poindexter gets OAuth access

**Phase 3 (Enterprise):** Option B - Google Workspace
- For operators running multiple instances
- Admin SDK provisioning
- Centralized account management

### Implementation Plan (Phase 1)

```go
// New file: /internal/toolbelt/email.go

type EmailService interface {
    Send(ctx context.Context, email Email) error
    // Future: Receive via webhook
}

// Uses Resend as backend
type ResendEmailService struct {
    client *ResendClient
    fromAddress string // e.g., poindexter@mail.dex.local
}
```

**Files to create/modify:**
- `/internal/email/service.go` - Unified email service interface
- `/internal/api/webhooks.go` - Inbound email webhook handler
- `/docs/EMAIL.md` - Documentation for email setup

---

## Step 4: GitHub Identity for Poindexter

### Critical Constraints

**GitHub does NOT allow programmatic user account creation.**

From research:
- No API for creating user accounts
- ToS prohibits bot-created accounts
- Machine accounts allowed but require human setup

### Viable Options

#### Option A: GitHub App (Strongly Recommended)

A centrally-owned GitHub App that acts as Poindexter's identity across all users.

```
Benefits:
- No user account needed
- No license seat consumed
- Higher rate limits (15,000 req/hr vs 5,000)
- Fine-grained permissions
- Survives employee turnover
- Better security model
- Consistent branding across all installations
```

**How it works:**
1. Operator creates and owns the "Poindexter" GitHub App (one time)
2. Users install the App on their repositories during onboarding
3. Each installation gets its own installation ID and scoped access
4. Commits show as "Poindexter[bot]" with consistent avatar/branding

**Implementation:**

```go
// /internal/toolbelt/github_app.go

type GitHubAppClient struct {
    appID          int64
    installationID int64
    privateKey     *rsa.PrivateKey
}

func (c *GitHubAppClient) GetInstallationToken() (string, error) {
    // Generate JWT, exchange for installation token
    // Tokens are short-lived (1 hour)
}
```

**Onboarding Flow:**
1. User clicks "Create GitHub App" link (pre-filled manifest)
2. GitHub redirects with App credentials
3. User installs App on repositories
4. Poindexter stores App ID + private key + installation ID

#### Option B: Dedicated Machine Account (Manual)

User creates a dedicated GitHub account for Poindexter.

```
Onboarding Flow:
1. User creates poindexter-<unique> GitHub account (manual)
2. User generates PAT with required scopes
3. User adds Poindexter account as collaborator on repos
4. Poindexter stores PAT in secrets.json
```

**Pros:**
- Simple implementation (current model)
- Full user account capabilities

**Cons:**
- Consumes a paid seat
- Manual account creation
- Password management burden
- PAT expiration management

#### Option C: OAuth App (User Impersonation)

Poindexter acts as an OAuth App, performing actions on behalf of the user.

```
Flow:
1. User authorizes OAuth App
2. Actions appear as user's identity
3. Token refresh handled automatically
```

**Pros:**
- No separate account needed
- Actions attributed to user

**Cons:**
- Poindexter doesn't have its own identity
- All actions show as user
- Not suitable if Poindexter identity is desired

### Recommendation: GitHub App (Option A)

```
┌────────────────────────────────────────────────────────────────────┐
│                     GitHub App Architecture                         │
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  OPERATOR (One-Time Setup)                                          │
│  └── Creates "Poindexter" GitHub App                               │
│      ├── App ID: 123456                                             │
│      ├── Custom Avatar: [Poindexter Logo]                          │
│      ├── Description: "AI development assistant"                   │
│      └── Private Key: (stored securely by operator)                │
│                                                                     │
│  USER A's GitHub                     USER B's GitHub                │
│  └── Installs Poindexter App         └── Installs Poindexter App   │
│      ├── Installation ID: 111            ├── Installation ID: 222  │
│      ├── Repos: user-a/proj-1            ├── Repos: user-b/app     │
│      └── Repos: user-a/proj-2            └── Org: user-b-org       │
│                                                                     │
│  All Actions Appear As:                                             │
│  ├── Commits: "Poindexter[bot] <123456+poindexter[bot]@...>"       │
│  ├── PRs: "poindexter[bot]"                                        │
│  └── Same avatar everywhere (brand recognition)                    │
│                                                                     │
└────────────────────────────────────────────────────────────────────┘
```

### Implementation Plan

**Step 1: Operator Creates GitHub App (One-Time)**

The operator (you) creates the Poindexter GitHub App once:

1. Go to https://github.com/settings/apps/new
2. Configure the App:
   ```
   Name:        Poindexter
   Description: AI-powered development assistant
   Homepage:    https://poindexter.ai
   Logo:        [Upload Poindexter avatar]
   Public:      Yes (so users can install it)
   ```
3. Set permissions:
   ```
   Repository permissions:
   - Contents: Read & Write
   - Issues: Read & Write
   - Pull requests: Read & Write
   - Workflows: Read & Write
   - Metadata: Read-only
   ```
4. Generate and securely store the private key
5. Note the App ID and publish the App

**Step 2: Store App Credentials (Operator-Side)**

The App's private key is stored centrally or distributed to instances:

```go
// /internal/toolbelt/github_app.go

type GitHubAppConfig struct {
    AppID         int64  `json:"app_id"`
    PrivateKeyPEM string `json:"private_key"` // Or path to key file
}

// Loaded from operator-provisioned config (like Anthropic key)
func LoadGitHubAppConfig(path string) (*GitHubAppConfig, error)
```

**Step 3: User Installation Flow (Onboarding)**

During onboarding, the user installs the pre-existing App:

```go
// /internal/api/github_app.go

func (s *Server) handleGitHubAppInstallRedirect(w http.ResponseWriter, r *http.Request) {
    // Redirect user to install the Poindexter App
    // https://github.com/apps/poindexter/installations/new
    http.Redirect(w, r, "https://github.com/apps/poindexter/installations/new", http.StatusFound)
}

func (s *Server) handleGitHubAppInstallCallback(w http.ResponseWriter, r *http.Request) {
    installationID := r.URL.Query().Get("installation_id")

    // Store this user's installation ID
    // This scopes Poindexter's access to just their repos
}
```

**Step 4: Authentication**

```go
// /internal/toolbelt/github_app.go

func (c *GitHubAppClient) getInstallationToken(ctx context.Context) (string, error) {
    // Create JWT signed with App private key
    jwt := createAppJWT(c.appID, c.privateKey)

    // Exchange for installation access token
    // POST /app/installations/{installation_id}/access_tokens
    token, err := exchangeForToken(jwt, c.installationID)

    // Cache token until near expiration
    return token, err
}
```

**Files to create/modify:**
- `/internal/toolbelt/github_app.go` - GitHub App client
- `/internal/api/github_app.go` - App creation/installation handlers
- `/frontend/src/components/GitHubAppSetup.tsx` - UI for App creation
- `/internal/api/setup.go` - Integrate into onboarding flow

---

## Complete Onboarding Flow (Revised)

```
┌─────────────────────────────────────────────────────────────────────┐
│                    New Onboarding Flow                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  PHASE 1: Installation (unchanged)                                   │
│  ├── 1. Run installer script                                        │
│  ├── 2. Temporary Cloudflare tunnel with PIN                        │
│  ├── 3. Choose Tailscale (recommended) or Cloudflare               │
│  └── 4. Device joins tailnet / tunnel configured                    │
│                                                                      │
│  PHASE 2: Identity Setup                                             │
│  ├── 1. Passkey Registration (unchanged)                            │
│  │                                                                   │
│  ├── 2. Anthropic API Key                                           │
│  │   ├── If pre-configured: Skip (show "Provided by Operator")     │
│  │   └── If not: User provides key (current flow)                  │
│  │                                                                   │
│  ├── 3. GitHub App Installation (NEW)                               │
│  │   ├── Click "Connect GitHub"                                     │
│  │   ├── Redirected to GitHub App installation page                │
│  │   ├── User selects repos to grant access                        │
│  │   ├── Redirected back with installation ID                      │
│  │   └── Poindexter now has scoped access to those repos           │
│  │                                                                   │
│  │   (Fallback: Use existing PAT for legacy/self-hosted)            │
│  │                                                                   │
│  ├── 4. Email Setup (NEW - Optional)                                │
│  │   ├── Default: Use built-in email (Resend)                      │
│  │   └── Optional: Connect Gmail via OAuth                          │
│  │                                                                   │
│  └── 5. Setup Complete                                               │
│      ├── Verify all integrations                                    │
│      ├── Create workspace repository (as GitHub App)               │
│      └── Ready to use                                                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Database Schema Changes

```sql
-- New table for GitHub App credentials
CREATE TABLE github_app_config (
    id INTEGER PRIMARY KEY,
    app_id INTEGER NOT NULL,
    app_slug TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret_encrypted TEXT NOT NULL,
    private_key_encrypted TEXT NOT NULL,
    webhook_secret_encrypted TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- New table for GitHub App installations
CREATE TABLE github_app_installations (
    id INTEGER PRIMARY KEY,
    installation_id INTEGER NOT NULL UNIQUE,
    account_login TEXT NOT NULL,       -- User or org name
    account_type TEXT NOT NULL,        -- 'User' or 'Organization'
    repository_selection TEXT NOT NULL, -- 'all' or 'selected'
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- New table for email configuration
CREATE TABLE email_config (
    id INTEGER PRIMARY KEY,
    provider TEXT NOT NULL,             -- 'resend', 'gmail', 'workspace'
    email_address TEXT NOT NULL,
    oauth_refresh_token_encrypted TEXT, -- For Gmail OAuth
    config_json TEXT,                   -- Provider-specific config
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## Security Considerations

### Credential Storage

All sensitive credentials should be encrypted at rest:

```go
// /internal/crypto/secrets.go

type SecretStore struct {
    masterKey []byte // Derived from passkey or hardware key
}

func (s *SecretStore) Encrypt(plaintext []byte) ([]byte, error) {
    // AES-256-GCM encryption
}

func (s *SecretStore) Decrypt(ciphertext []byte) ([]byte, error) {
    // AES-256-GCM decryption
}
```

### Current Gap

Currently, `secrets.json` stores credentials in **plaintext**. The redesign should:
1. Encrypt secrets at rest using a key derived from the passkey
2. Store encrypted blobs in SQLite (more manageable than files)
3. Decrypt in memory only when needed

### GitHub App Security Benefits

- Short-lived tokens (1 hour) vs long-lived PATs
- Tokens automatically scoped to installation
- No password to manage
- Private key never leaves the server

---

## Migration Path

### For Existing Users

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Migration Strategy                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Existing Installation Detected:                                     │
│  ├── Current credentials continue to work                          │
│  ├── Settings page shows "Upgrade to GitHub App" option            │
│  └── Migration is optional, not forced                              │
│                                                                      │
│  Migration Flow (if user opts in):                                  │
│  ├── 1. Create GitHub App (same flow as new onboarding)            │
│  ├── 2. Install on same repositories                                │
│  ├── 3. Verify App works                                            │
│  ├── 4. Deactivate old PAT                                          │
│  └── 5. Delete PAT from secrets.json                                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Implementation Phases

### Phase 1: Foundation (2-3 weeks)

- [ ] Implement GitHub App authentication
- [ ] Create App manifest and creation flow
- [ ] Add installation management
- [ ] Update onboarding UI with App option
- [ ] Maintain PAT as fallback

### Phase 2: Email Infrastructure (1-2 weeks)

- [ ] Implement unified email service interface
- [ ] Add inbound email webhook support
- [ ] Configure Resend for Poindexter identity
- [ ] Add email to onboarding (optional)

### Phase 3: Enhanced Security (1-2 weeks)

- [ ] Implement secret encryption at rest
- [ ] Migrate from secrets.json to encrypted SQLite storage
- [ ] Add key rotation support
- [ ] Security audit

### Phase 4: Gmail Integration (Optional, 1-2 weeks)

- [ ] Implement Google OAuth flow
- [ ] Gmail API integration
- [ ] Calendar/Drive integration (if desired)
- [ ] User documentation

### Phase 5: Enterprise Features (Future)

- [ ] Google Workspace Admin SDK integration
- [ ] GitHub EMU support
- [ ] Centralized credential management
- [ ] Multi-instance coordination

---

## Open Questions

1. **Gmail Necessity**: Is Gmail specifically needed, or is email capability sufficient?
   - If just email: Resend + custom domain is simpler
   - If Google ecosystem: OAuth integration needed

2. **Poindexter Identity Visibility**: Should commits appear as "Poindexter[bot]" or configurable?
   - GitHub App: Always appears as bot
   - User might want custom naming

3. **Multi-Instance Scenarios**: Will users run multiple Poindexter instances?
   - **Decision**: One central GitHub App with consistent branding
   - Each instance gets its own installation ID (scoped access)
   - Same "Poindexter[bot]" identity across all users (brand recognition)

4. **Operator Model**: Is there a central operator provisioning instances?
   - If yes: Consider centralized credential management
   - If no: Current user-driven model is fine

---

## Appendix: API Reference

### Google Workspace Admin SDK (Future Reference)

```python
# Create user in Workspace domain
from googleapiclient.discovery import build

service = build('admin', 'directory_v1', credentials=creds)
user = {
    'primaryEmail': 'poindexter@example.com',
    'name': {'givenName': 'Poindexter', 'familyName': 'AI'},
    'password': generate_secure_password(),
}
service.users().insert(body=user).execute()
```

### GitHub App Manifest Flow

```bash
# Step 1: Redirect user to create App
GET https://github.com/settings/apps/new?manifest=<url-encoded-manifest>

# Step 2: Handle callback with code
GET https://dex.example.com/api/github/callback?code=<code>

# Step 3: Exchange code for credentials
POST https://api.github.com/app-manifests/{code}/conversions
# Returns: id, slug, client_id, client_secret, pem, webhook_secret

# Step 4: Generate JWT for API access
jwt = JWT.encode({
    iss: app_id,
    iat: now,
    exp: now + 10.minutes
}, private_key, 'RS256')

# Step 5: Get installation token
POST /app/installations/{installation_id}/access_tokens
Authorization: Bearer <jwt>
# Returns: token (valid 1 hour)
```

---

## Conclusion

This redesigned onboarding flow provides Poindexter with its own identity while maintaining:
- **User Control**: User owns/manages all accounts
- **Security**: Better token management with GitHub Apps
- **Flexibility**: Supports both simple and enterprise deployments
- **Backwards Compatibility**: Existing installations continue to work

The recommended path is:
1. **GitHub**: Implement GitHub App (high value, clear benefits)
2. **Anthropic**: Add operator-provisioned key support (simple)
3. **Email**: Start with Resend, add Gmail OAuth as optional
4. **Tailscale**: Keep current implementation (already correct)
