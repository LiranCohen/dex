# Poindexter Onboarding Redesign Plan

## Executive Summary

This document outlines a redesigned onboarding flow where Poindexter has its own dedicated accounts and credentials, rather than using the user's personal accounts. The goal is to give Poindexter a distinct identity while maintaining user oversight and access.

**Proposed Flow:**
1. Establish dexnet mesh connection
2. Provision Anthropic API key
3. Create/provision Gmail account for Poindexter (user has access)
4. Create/provision GitHub identity for Poindexter (user has access)

---

## Current State Analysis

### What Exists Today

| Component | Current Implementation |
|-----------|------------------------|
| **dexnet** | Fully integrated. User enrolls HQ with Central, device joins mesh network. |
| **Anthropic API** | User manually provides their personal API key during onboarding. |
| **GitHub** | User manually creates PAT from their personal account. |
| **Email/Gmail** | No integration. Resend exists for outbound transactional email only. |
| **Authentication** | WebAuthn/Passkeys (biometric, no passwords). Single-user model. |

### Current Onboarding Flow (for reference)

```
Phase 1 (Installation):
1. Run installer on VPS
2. Temporary Cloudflare tunnel with PIN
3. User chooses dexnet (recommended) or Cloudflare
4. Device joins mesh network or permanent tunnel created

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
│  │   dexnet     │  │   Gmail      │  │   GitHub     │               │
│  │   Account    │  │   Account    │  │   Account    │               │
│  │  (Central)   │  │ (Dedicated)  │  │  (App/Bot)   │               │
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

## Step 3: Poindexter's Full Digital Identity

### Vision

Each Poindexter instance should have its own complete digital identity:
- **Email** - Send/receive emails, use as account recovery
- **Calendar** - Schedule events, accept invitations
- **Signal** - Secure messaging with users
- **OAuth** - Authenticate to arbitrary third-party services

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    POINDEXTER IDENTITY STACK                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    IDENTITY FOUNDATION                           │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │    │
│  │  │ Phone #     │  │ Email       │  │ Calendar                │  │    │
│  │  │ (Twilio)    │  │ (Workspace) │  │ (Google/CalDAV)         │  │    │
│  │  │             │  │             │  │                         │  │    │
│  │  │ +1-xxx-xxxx │  │ dex-id@     │  │ CalDAV endpoint or      │  │    │
│  │  │ SMS receive │  │ poindexter  │  │ Google Calendar API     │  │    │
│  │  │ Voice calls │  │ .ai         │  │                         │  │    │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    COMMUNICATION LAYER                           │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │    │
│  │  │ Signal      │  │ SMS/Voice   │  │ Email (SMTP/IMAP)       │  │    │
│  │  │ (signal-cli)│  │ (Twilio)    │  │ (Gmail API/Resend)      │  │    │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    OAUTH INTEGRATION LAYER                       │    │
│  │  ┌─────────────────────────────────────────────────────────────┐│    │
│  │  │ OAuth Manager                                                ││    │
│  │  │ - Client Credentials (M2M)                                   ││    │
│  │  │ - Device Authorization Flow (user-delegated)                 ││    │
│  │  │ - Token storage & refresh                                    ││    │
│  │  │ - Per-service credential management                          ││    │
│  │  └─────────────────────────────────────────────────────────────┘│    │
│  │                                                                  │    │
│  │  Connected Services: Linear, Notion, Slack, Jira, etc.          │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

### Component 1: Phone Number (Foundation)

A dedicated phone number is the foundation - required for Signal and SMS-based 2FA.

**Recommended: Twilio**

| Feature | Details |
|---------|---------|
| Cost | ~$1/month per number + $0.0075/SMS received |
| Capabilities | SMS receive, voice calls, programmable |
| API | REST API with Go SDK |
| Coverage | 100+ countries |

**Implementation:**

```go
// /internal/identity/phone.go

type PhoneService struct {
    twilioClient *twilio.RestClient
    phoneNumber  string // Poindexter's dedicated number
}

// Receive SMS (webhook from Twilio)
func (p *PhoneService) HandleIncomingSMS(from, body string) error {
    // Parse verification codes, forward to appropriate handler
}

// For 2FA during account creation
func (p *PhoneService) WaitForVerificationCode(ctx context.Context, timeout time.Duration) (string, error) {
    // Poll or webhook-wait for incoming SMS with code pattern
}
```

**Onboarding Flow:**
1. Operator pre-provisions Twilio account with phone numbers
2. During Dex setup, assign a number to this instance
3. Configure Twilio webhook → Dex endpoint
4. Phone number stored in `identity_config` table

---

### Component 2: Email (Google Workspace)

**Recommended: Google Workspace with Admin SDK**

| Feature | Details |
|---------|---------|
| Cost | ~$6/user/month (Workspace Starter) |
| Domain | `poindexter.ai` or similar |
| API | Admin SDK for account creation, Gmail API for email |
| Benefits | Full Google ecosystem (Calendar, Drive included) |

**Why Workspace over alternatives:**
- Programmatic account creation via Admin SDK
- Gmail API for full email management
- Calendar included (no separate service needed)
- Professional appearance (`dex-xxx@poindexter.ai`)
- OAuth tokens for Google services come free

**Implementation:**

```go
// /internal/identity/email.go

type EmailIdentity struct {
    address      string // dex-xxx@poindexter.ai
    gmailService *gmail.Service
    adminService *admin.Service // For account creation (operator-level)
}

// Create new email account (operator-level, during provisioning)
func (e *EmailIdentity) CreateAccount(ctx context.Context, instanceID string) error {
    user := &admin.User{
        PrimaryEmail: fmt.Sprintf("dex-%s@poindexter.ai", instanceID),
        Name: &admin.UserName{
            GivenName:  "Poindexter",
            FamilyName: instanceID,
        },
        Password: generateSecurePassword(),
    }
    _, err := e.adminService.Users.Insert(user).Do()
    return err
}

// Send email
func (e *EmailIdentity) Send(ctx context.Context, to, subject, body string) error

// Read inbox
func (e *EmailIdentity) GetUnread(ctx context.Context) ([]*gmail.Message, error)
```

**Alternative: Self-Hosted (Postal/Mailcow)**
- Full control, no per-user cost
- More operational overhead
- Good for privacy-focused deployments

---

### Component 3: Calendar (Google Calendar or CalDAV)

**Option A: Google Calendar (comes with Workspace)**

```go
// /internal/identity/calendar.go

type CalendarService struct {
    service *calendar.Service
}

func (c *CalendarService) CreateEvent(ctx context.Context, event Event) error
func (c *CalendarService) GetUpcoming(ctx context.Context, days int) ([]Event, error)
func (c *CalendarService) AcceptInvitation(ctx context.Context, eventID string) error
```

**Option B: Self-Hosted CalDAV (Baïkal or Nextcloud)**

For users who want to avoid Google:

```go
// /internal/identity/caldav.go

type CalDAVService struct {
    client   *caldav.Client
    calendar string // calendar URL
}

// Standard CalDAV operations
func (c *CalDAVService) CreateEvent(ctx context.Context, event Event) error
func (c *CalDAVService) GetEvents(ctx context.Context, start, end time.Time) ([]Event, error)
```

---

### Component 4: Signal (Secure Messaging)

**Implementation: signal-cli + REST wrapper**

| Component | Purpose |
|-----------|---------|
| signal-cli | Core Signal protocol implementation |
| signal-cli-rest-api | Docker container exposing REST API |
| Twilio number | Phone number for Signal registration |

**Setup Flow:**
1. Provision Twilio number for this Poindexter
2. Register Signal account using signal-cli
3. Receive verification SMS via Twilio webhook
4. Complete registration
5. Run signal-cli-rest-api as sidecar container

**Implementation:**

```go
// /internal/identity/signal.go

type SignalService struct {
    apiURL      string // signal-cli-rest-api endpoint
    phoneNumber string // Poindexter's Signal number
}

func (s *SignalService) Register(ctx context.Context, verificationCode string) error {
    // Complete Signal registration with code from Twilio SMS
}

func (s *SignalService) Send(ctx context.Context, to, message string) error {
    // POST /v2/send
}

func (s *SignalService) Receive(ctx context.Context) ([]Message, error) {
    // GET /v1/receive/{number}
}
```

**Maintenance Requirements:**
- Signal accounts expire after 120 days of inactivity
- signal-cli needs updates when Signal protocol changes (~quarterly)
- Consider Matrix/Element as more bot-friendly alternative

---

### Component 5: OAuth Integration Layer

This is the key to Poindexter creating/managing accounts on arbitrary services.

**Two OAuth Flows:**

1. **Client Credentials** (Machine-to-Machine)
   - For services with API access (no user delegation)
   - Poindexter authenticates as itself

2. **Device Authorization Flow** (User-Delegated)
   - For services requiring user consent
   - User authorizes on separate device
   - Best for headless/CLI scenarios

**Implementation:**

```go
// /internal/identity/oauth.go

type OAuthManager struct {
    db           *db.DB
    tokenStore   *TokenStore
    httpClient   *http.Client
}

// Service registration
type OAuthService struct {
    ID           string
    Name         string
    AuthURL      string
    TokenURL     string
    ClientID     string
    ClientSecret string
    Scopes       []string
    FlowType     string // "client_credentials", "device", "authorization_code"
}

// Device Authorization Flow (RFC 8628)
func (o *OAuthManager) StartDeviceFlow(ctx context.Context, service *OAuthService) (*DeviceCode, error) {
    // POST to device_authorization_endpoint
    // Returns: device_code, user_code, verification_uri
}

func (o *OAuthManager) PollForToken(ctx context.Context, service *OAuthService, deviceCode string) (*Token, error) {
    // Poll token endpoint until user completes authorization
}

// Client Credentials Flow
func (o *OAuthManager) GetClientCredentialsToken(ctx context.Context, service *OAuthService) (*Token, error) {
    // Direct token request with client_id + client_secret
}

// Token management
func (o *OAuthManager) GetToken(ctx context.Context, serviceID string) (*Token, error) {
    // Get cached token, refresh if expired
}

func (o *OAuthManager) RefreshToken(ctx context.Context, serviceID string) (*Token, error) {
    // Use refresh_token to get new access_token
}
```

**Database Schema:**

```sql
CREATE TABLE oauth_services (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    auth_url TEXT,
    token_url TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT,
    scopes TEXT,  -- JSON array
    flow_type TEXT NOT NULL,
    created_at TIMESTAMP
);

CREATE TABLE oauth_tokens (
    service_id TEXT PRIMARY KEY REFERENCES oauth_services(id),
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    token_type TEXT,
    expires_at TIMESTAMP,
    scopes TEXT,  -- JSON array of granted scopes
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

**Pre-configured Services:**

```go
var KnownServices = map[string]*OAuthService{
    "linear": {
        Name:     "Linear",
        AuthURL:  "https://linear.app/oauth/authorize",
        TokenURL: "https://api.linear.app/oauth/token",
        FlowType: "authorization_code",
        Scopes:   []string{"read", "write"},
    },
    "notion": {
        Name:     "Notion",
        AuthURL:  "https://api.notion.com/v1/oauth/authorize",
        TokenURL: "https://api.notion.com/v1/oauth/token",
        FlowType: "authorization_code",
    },
    "slack": {
        Name:     "Slack",
        AuthURL:  "https://slack.com/oauth/v2/authorize",
        TokenURL: "https://slack.com/api/oauth.v2.access",
        FlowType: "authorization_code",
        Scopes:   []string{"chat:write", "channels:read"},
    },
    // ... more services
}
```

---

### Account Creation Flow (Poindexter Creates Its Own Accounts)

For services that allow email-based signup:

```
┌─────────────────────────────────────────────────────────────────────┐
│              Poindexter Self-Service Account Creation                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. User requests: "Sign up for Linear"                             │
│                                                                      │
│  2. Poindexter navigates to signup page                             │
│     └── Uses headless browser (Playwright/Rod)                      │
│                                                                      │
│  3. Fills signup form:                                               │
│     ├── Email: dex-xxx@poindexter.ai                                │
│     ├── Name: Poindexter                                             │
│     └── Password: (generated, stored in vault)                      │
│                                                                      │
│  4. Handles verification:                                            │
│     ├── Email verification → Gmail API reads code                   │
│     ├── SMS verification → Twilio receives code                     │
│     └── CAPTCHA → (manual intervention or solving service)          │
│                                                                      │
│  5. Completes OAuth authorization:                                   │
│     └── Stores tokens in oauth_tokens table                         │
│                                                                      │
│  6. Account ready for use                                            │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

**Implementation:**

```go
// /internal/identity/accounts.go

type AccountManager struct {
    email    *EmailIdentity
    phone    *PhoneService
    oauth    *OAuthManager
    browser  *rod.Browser // Headless browser for signup flows
}

func (a *AccountManager) CreateAccount(ctx context.Context, service string) error {
    switch service {
    case "linear":
        return a.createLinearAccount(ctx)
    case "notion":
        return a.createNotionAccount(ctx)
    // ...
    }
}

func (a *AccountManager) createLinearAccount(ctx context.Context) error {
    // 1. Navigate to linear.app/signup
    // 2. Fill form with Poindexter's email
    // 3. Wait for verification email
    // 4. Extract verification link from email
    // 5. Complete verification
    // 6. Store credentials
}
```

---

### Onboarding Flow (Revised with Full Identity)

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Identity-Aware Onboarding                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  PHASE 1: Core Setup (unchanged)                                     │
│  ├── Tailscale connection                                           │
│  ├── Passkey registration                                           │
│  └── Anthropic API key                                              │
│                                                                      │
│  PHASE 2: Identity Provisioning (NEW)                                │
│  │                                                                   │
│  ├── 2a. Phone Number Assignment                                    │
│  │   ├── Operator has pool of Twilio numbers                        │
│  │   ├── Assign one to this instance                                │
│  │   ├── Configure webhook → Dex                                    │
│  │   └── Test SMS reception                                         │
│  │                                                                   │
│  ├── 2b. Email Account Creation                                     │
│  │   ├── Admin SDK creates dex-xxx@poindexter.ai                   │
│  │   ├── Generate app password or OAuth token                       │
│  │   ├── Verify email works (send test)                             │
│  │   └── Calendar auto-provisioned with Workspace                   │
│  │                                                                   │
│  ├── 2c. Signal Registration (Optional)                             │
│  │   ├── Start signal-cli registration                              │
│  │   ├── Receive verification code via Twilio                       │
│  │   ├── Complete registration                                      │
│  │   └── Start signal-cli-rest-api sidecar                          │
│  │                                                                   │
│  PHASE 3: GitHub App (existing)                                      │
│  └── Create/install GitHub App                                      │
│                                                                      │
│  PHASE 4: Additional Services (Optional)                             │
│  ├── "Connect Linear" → OAuth flow                                  │
│  ├── "Connect Slack" → OAuth flow                                   │
│  └── "Create account on X" → Automated signup                       │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

### Database Schema for Identity

```sql
-- Core identity configuration
CREATE TABLE identity_config (
    id INTEGER PRIMARY KEY DEFAULT 1,

    -- Phone
    phone_number TEXT,
    twilio_account_sid TEXT,
    twilio_auth_token TEXT,

    -- Email (Google Workspace)
    email_address TEXT,
    google_service_account_json TEXT,  -- For API access

    -- Signal
    signal_registered BOOLEAN DEFAULT FALSE,
    signal_device_id INTEGER,

    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- OAuth services and tokens (from above)
CREATE TABLE oauth_services (...);
CREATE TABLE oauth_tokens (...);

-- Account credentials for services where Poindexter has its own account
CREATE TABLE service_accounts (
    id TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    email TEXT,
    password_encrypted TEXT,  -- For non-OAuth services
    api_key_encrypted TEXT,   -- For API key services
    notes TEXT,
    created_at TIMESTAMP
);
```

---

### Cost Estimate (Per Poindexter Instance)

| Component | Monthly Cost |
|-----------|-------------|
| Twilio Phone Number | ~$1 |
| Twilio SMS (est. 100 msgs) | ~$0.75 |
| Google Workspace | ~$6 |
| Signal | Free |
| **Total** | **~$8/month** |

Self-hosted alternative (email + calendar):
| Component | Monthly Cost |
|-----------|-------------|
| Twilio Phone + SMS | ~$1.75 |
| Self-hosted email (Postal) | ~$0 (server costs) |
| Self-hosted CalDAV (Baïkal) | ~$0 (server costs) |
| **Total** | **~$2/month** + server |

---

## Step 4: GitHub Identity for Poindexter

### Critical Constraints

**GitHub does NOT allow programmatic user account creation.**

From research:
- No API for creating user accounts
- ToS prohibits bot-created accounts
- Machine accounts allowed but require human setup

### Viable Options

#### Option A: GitHub App via Manifest Flow ✅ IMPLEMENTED

Each Dex instance creates its own GitHub App via the manifest flow during onboarding.

```
Benefits:
- No user account needed
- No license seat consumed
- Higher rate limits (15,000 req/hr vs 5,000)
- Fine-grained permissions
- User owns their App (can customize name/logo)
- Better security model (short-lived tokens)
- Self-hosted friendly (no central dependency)
```

**How it works:**
1. User clicks "Create GitHub App" in onboarding
2. Dex generates a manifest and redirects to `github.com/settings/apps/new`
3. GitHub creates the App (named `dex-<random>`) and redirects back with credentials
4. User installs the App on their account/repos
5. Commits show as "dex-xxxx[bot]" (user can rename in GitHub settings)

**Implementation (Completed):**

```go
// /internal/github/app.go - AppManager handles JWT generation and token caching
type AppManager struct {
    config     *AppConfig
    privateKey *rsa.PrivateKey
    // Token cache with 5-minute buffer before expiry
    installTokens   map[int64]*cachedToken
}

// /internal/db/github.go - Database storage
// - github_app_config: singleton table for App credentials
// - github_installations: tracks installed accounts/orgs

// /internal/api/github.go - API endpoints
// - GET  /api/v1/setup/github/app/status    - Check if App configured
// - GET  /api/v1/setup/github/app/manifest  - Get manifest for creation
// - GET  /api/v1/setup/github/app/callback  - Handle App creation callback
// - GET  /api/v1/setup/github/install/callback - Handle installation callback
// - POST /api/v1/setup/github/sync          - Sync installations from GitHub
```

**Permissions Requested:**
- `contents: write` - Read/write repo files
- `pull_requests: write` - Create PRs
- `issues: write` - Create issues
- `administration: write` - Create repos
- `metadata: read` - Repo metadata
- `workflows: write` - Trigger GitHub Actions

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

### Final Decision: Per-Instance GitHub App via Manifest Flow ✅

```
┌────────────────────────────────────────────────────────────────────┐
│                 Per-Instance GitHub App Architecture                │
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  DEX INSTANCE (User's Server)                                       │
│  └── Creates own GitHub App during onboarding                      │
│      ├── App Name: "dex-a1b2c3d4" (unique per instance)            │
│      ├── Logo: Poindexter headshot (from manifest)                 │
│      ├── Credentials: Stored in github_app_config table            │
│      └── User can rename/rebrand in GitHub settings                │
│                                                                     │
│  USER's GitHub Account                                              │
│  └── Installs dex-xxxx App                                         │
│      ├── Installation ID: stored in github_installations           │
│      ├── Scoped to: Selected repos OR all repos                    │
│      └── Can install on personal account + organizations           │
│                                                                     │
│  Token Flow:                                                        │
│  ├── JWT: Generated from private key (10min expiry)                │
│  ├── Installation Token: Exchanged from JWT (1hr expiry)           │
│  └── Cache: In-memory with 5-minute refresh buffer                 │
│                                                                     │
│  Actions Appear As:                                                 │
│  ├── Commits: "dex-xxxx[bot] <id+dex-xxxx[bot]@users...>"         │
│  └── PRs/Issues: Created by "dex-xxxx[bot]"                        │
│                                                                     │
└────────────────────────────────────────────────────────────────────┘
```

### Implementation Status ✅ COMPLETE

**Key Files Created:**

| File | Purpose |
|------|---------|
| `/internal/github/app.go` | AppManager - JWT generation, token caching, installation management |
| `/internal/db/github.go` | Database operations for app config & installations |
| `/internal/api/github.go` | API endpoints for manifest flow & callbacks |

**Database Schema (Implemented):**

```sql
-- Singleton table for App credentials
CREATE TABLE github_app_config (
    id INTEGER PRIMARY KEY DEFAULT 1,
    app_id INTEGER NOT NULL,
    app_slug TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    private_key TEXT NOT NULL,
    webhook_secret TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Tracks installed accounts/orgs
CREATE TABLE github_installations (
    id INTEGER PRIMARY KEY,  -- GitHub's installation ID
    account_id INTEGER NOT NULL,
    account_type TEXT NOT NULL,  -- 'User' or 'Organization'
    login TEXT NOT NULL,         -- Username or org name
    created_at TIMESTAMP
);
```

**API Endpoints (Implemented):**

```
GET  /api/v1/setup/github/app/status     - Check if App is configured
GET  /api/v1/setup/github/app/manifest   - Get manifest for App creation
GET  /api/v1/setup/github/app/callback   - Handle App creation callback from GitHub
GET  /api/v1/setup/github/install/callback - Handle installation callback
GET  /api/v1/setup/github/installations  - List all installations
POST /api/v1/setup/github/sync           - Sync installations from GitHub
DELETE /api/v1/setup/github/app          - Remove App configuration
```

**Onboarding Flow:**

```
1. User clicks "Create GitHub App" button
2. Frontend fetches manifest from /api/v1/setup/github/app/manifest
3. User redirected to: github.com/settings/apps/new?manifest=<json>
4. User approves App creation on GitHub
5. GitHub redirects to callback with code
6. Backend exchanges code for credentials (app_id, private_key, etc.)
7. Credentials saved to github_app_config table
8. User redirected to install the App on their repos
9. Installation callback saves installation_id to github_installations
10. Setup complete - App can now authenticate to GitHub
```

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

### Phase 1: GitHub App Foundation ✅ COMPLETE

- [x] Implement GitHub App authentication (`internal/github/app.go`)
- [x] Create App manifest and creation flow (`internal/api/github.go`)
- [x] Add installation management (`internal/db/github.go`)
- [x] Update onboarding UI with App option
- [x] Maintain PAT as fallback (legacy token support)

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
   - **Decision**: Per-instance GitHub Apps via manifest flow
   - Each instance creates its own App (named `dex-<random>`)
   - User can customize name/logo in GitHub settings after creation
   - Self-hosted friendly - no central App dependency

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
