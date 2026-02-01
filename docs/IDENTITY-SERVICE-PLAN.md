# Poindexter Identity Service (PIS)

## Overview

A managed service that provisions complete digital identities for Poindexter instances:
- Google Workspace account (email + calendar)
- Phone number + Signal account
- AI API access (proxied, metered, billed)

Users can either use the managed service OR bring their own credentials.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     POINDEXTER IDENTITY SERVICE                              │
│                        identity.poindexter.ai                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         CONTROL PLANE                                   │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────────┐   │ │
│  │  │ Admin API    │  │ User Portal  │  │ Billing Service            │   │ │
│  │  │ (internal)   │  │ (dashboard)  │  │ (Stripe)                   │   │ │
│  │  └──────────────┘  └──────────────┘  └────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                      IDENTITY PROVISIONING                              │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────────┐   │ │
│  │  │ Google       │  │ Phone/Signal │  │ Identity Database          │   │ │
│  │  │ Workspace    │  │ Manager      │  │ (PostgreSQL)               │   │ │
│  │  │ Provisioner  │  │ (Telnyx)     │  │                            │   │ │
│  │  └──────────────┘  └──────────────┘  └────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         AI PROXY LAYER                                  │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────────┐   │ │
│  │  │ Anthropic    │  │ OpenAI       │  │ Usage Tracker              │   │ │
│  │  │ Proxy        │  │ Proxy        │  │ (tokens in/out, costs)     │   │ │
│  │  └──────────────┘  └──────────────┘  └────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                      CLIENT API (for Dex instances)                     │ │
│  │                                                                          │ │
│  │  POST /v1/auth/token          - Exchange API key for JWT                │ │
│  │  GET  /v1/identity            - Get provisioned identity details        │ │
│  │  POST /v1/ai/chat             - Proxied AI requests                     │ │
│  │  GET  /v1/usage               - Get usage stats                         │ │
│  │  POST /v1/email/send          - Send email via provisioned account      │ │
│  │  GET  /v1/email/inbox         - Read inbox                              │ │
│  │  POST /v1/signal/send         - Send Signal message                     │ │
│  │  GET  /v1/signal/messages     - Receive Signal messages                 │ │
│  │  POST /v1/calendar/events     - Create calendar events                  │ │
│  │                                                                          │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DEX INSTANCES (Customers)                            │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │ Poindexter #1   │  │ Poindexter #2   │  │ Poindexter #3   │   ...       │
│  │ (managed)       │  │ (managed)       │  │ (BYOK)          │             │
│  │                 │  │                 │  │                 │             │
│  │ API Key: pk_... │  │ API Key: pk_... │  │ Own credentials │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## User Journey

### Flow 1: Managed Identity (Recommended)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        MANAGED ONBOARDING FLOW                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. USER SIGNS UP                                                            │
│     └── identity.poindexter.ai/signup                                       │
│         ├── Email + Password (or OAuth with GitHub/Google)                  │
│         ├── Payment method (Stripe)                                         │
│         └── Choose plan (Basic / Pro / Enterprise)                          │
│                                                                              │
│  2. IDENTITY PROVISIONED (automatic, ~30 seconds)                           │
│     ├── Google Workspace account created                                    │
│     │   └── dex-{user_id}@poindexter.ai                                    │
│     ├── Phone number assigned from pool                                     │
│     │   └── +1-555-xxx-xxxx (Telnyx)                                       │
│     ├── Signal account registered                                           │
│     │   └── Using assigned phone number                                     │
│     └── API key generated                                                   │
│         └── pk_live_xxxxxxxxxxxx                                            │
│                                                                              │
│  3. USER CONFIGURES DEX INSTANCE                                            │
│     └── In Dex onboarding, choose "Use Poindexter Identity Service"        │
│         ├── Enter API key: pk_live_xxxxxxxxxxxx                            │
│         ├── Dex fetches identity details from service                      │
│         └── All integrations configured automatically                       │
│                                                                              │
│  4. ONGOING USAGE                                                            │
│     ├── AI requests proxied through service (metered)                      │
│     ├── Email/Signal/Calendar via service API                              │
│     ├── Monthly billing based on usage                                      │
│     └── Dashboard shows usage, costs, logs                                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Flow 2: Bring Your Own Keys (BYOK)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BYOK ONBOARDING FLOW                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. USER SKIPS MANAGED SERVICE                                              │
│     └── In Dex onboarding, choose "I'll configure my own credentials"      │
│                                                                              │
│  2. MANUAL CONFIGURATION                                                     │
│     ├── Anthropic API Key: (user provides)                                  │
│     ├── Google Workspace: (user creates account, provides OAuth)            │
│     ├── Telnyx/Twilio: (user provides account + phone number)              │
│     └── Signal: (user registers manually)                                   │
│                                                                              │
│  3. NO DEPENDENCY ON IDENTITY SERVICE                                        │
│     └── Dex operates fully independently                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Service Components

### 1. User Portal (identity.poindexter.ai)

**Tech Stack:** Next.js + Tailwind + Supabase Auth

**Pages:**
- `/signup` - Registration with Stripe checkout
- `/login` - Authentication
- `/dashboard` - Overview, usage stats, costs
- `/identity` - View provisioned identity details
- `/billing` - Manage subscription, view invoices
- `/api-keys` - Manage API keys
- `/logs` - View AI request logs, email logs

**Dashboard Features:**
```
┌─────────────────────────────────────────────────────────────────────────────┐
│  POINDEXTER IDENTITY SERVICE                          user@example.com ▼   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  YOUR IDENTITY                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Email:    dex-abc123@poindexter.ai                    [Copy]        │   │
│  │ Phone:    +1 (555) 123-4567                           [Copy]        │   │
│  │ Signal:   Registered ✓                                              │   │
│  │ API Key:  pk_live_xxxxxxxx...                         [Reveal]      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  THIS MONTH'S USAGE                                         Feb 2026        │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐         │
│  │ AI Tokens        │  │ Emails Sent      │  │ Signal Messages  │         │
│  │ 2.4M in / 890K out│ │ 156              │  │ 42               │         │
│  │ $34.50           │  │ $0.00            │  │ $0.00            │         │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘         │
│                                                                              │
│  ESTIMATED BILL: $34.50 + $15.00 base = $49.50                             │
│                                                                              │
│  RECENT ACTIVITY                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 2 min ago    AI Request    claude-3-opus    1,234 tokens    $0.02   │   │
│  │ 5 min ago    Email Sent    to: user@ex...   -                $0.00   │   │
│  │ 1 hour ago   AI Request    claude-3-sonnet  5,678 tokens    $0.04   │   │
│  │ ...                                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### 2. Identity Provisioning Service

**Tech Stack:** Go service + PostgreSQL + Redis

**Responsibilities:**
- Create Google Workspace accounts via Admin SDK
- Provision phone numbers from Telnyx
- Register Signal accounts via signal-cli
- Generate and manage API keys
- Store identity mappings

**Database Schema:**

```sql
-- Users (linked to Stripe customer)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    stripe_customer_id TEXT UNIQUE,
    plan TEXT NOT NULL DEFAULT 'basic',  -- basic, pro, enterprise
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Provisioned identities (one per user)
CREATE TABLE identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- Google Workspace
    workspace_email TEXT UNIQUE,
    workspace_password_encrypted TEXT,
    google_refresh_token_encrypted TEXT,

    -- Phone/Signal
    phone_number TEXT UNIQUE,
    phone_provider TEXT,  -- 'telnyx', 'twilio'
    phone_provider_id TEXT,  -- Provider's resource ID
    signal_registered BOOLEAN DEFAULT FALSE,
    signal_device_id INTEGER,

    -- Status
    status TEXT DEFAULT 'provisioning',  -- provisioning, active, suspended, deleted
    provisioned_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id)
);

-- API keys for Dex instances to authenticate
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    key_hash TEXT UNIQUE NOT NULL,  -- bcrypt hash of pk_live_xxx
    key_prefix TEXT NOT NULL,  -- pk_live_xxx (first 12 chars for display)
    name TEXT,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Usage tracking
CREATE TABLE usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    event_type TEXT NOT NULL,  -- 'ai_request', 'email_sent', 'signal_sent', 'sms_received'

    -- AI-specific
    model TEXT,
    input_tokens INTEGER,
    output_tokens INTEGER,
    cost_cents INTEGER,

    -- Request metadata
    request_id TEXT,
    latency_ms INTEGER,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Monthly usage aggregates (for billing)
CREATE TABLE usage_monthly (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    month DATE NOT NULL,  -- First day of month

    ai_input_tokens BIGINT DEFAULT 0,
    ai_output_tokens BIGINT DEFAULT 0,
    ai_cost_cents INTEGER DEFAULT 0,

    emails_sent INTEGER DEFAULT 0,
    emails_received INTEGER DEFAULT 0,

    signal_messages_sent INTEGER DEFAULT 0,
    signal_messages_received INTEGER DEFAULT 0,

    sms_sent INTEGER DEFAULT 0,
    sms_received INTEGER DEFAULT 0,

    UNIQUE(user_id, month)
);
```

**Provisioning Flow:**

```go
// /internal/provisioner/provisioner.go

type Provisioner struct {
    db           *sql.DB
    googleAdmin  *admin.Service
    telnyx       *telnyx.Client
    signalCLI    *signal.Client
    stripe       *stripe.Client
}

func (p *Provisioner) ProvisionIdentity(ctx context.Context, userID string) (*Identity, error) {
    // 1. Create Google Workspace account
    email := fmt.Sprintf("dex-%s@poindexter.ai", shortID(userID))
    password := generateSecurePassword()

    if err := p.createWorkspaceAccount(ctx, email, password); err != nil {
        return nil, fmt.Errorf("workspace creation failed: %w", err)
    }

    // 2. Provision phone number from Telnyx
    phoneNumber, err := p.provisionPhoneNumber(ctx)
    if err != nil {
        // Rollback: delete workspace account
        p.deleteWorkspaceAccount(ctx, email)
        return nil, fmt.Errorf("phone provisioning failed: %w", err)
    }

    // 3. Register Signal account
    if err := p.registerSignal(ctx, phoneNumber); err != nil {
        // Signal is optional, log but don't fail
        log.Printf("Signal registration failed (will retry): %v", err)
    }

    // 4. Store identity in database
    identity := &Identity{
        UserID:         userID,
        WorkspaceEmail: email,
        PhoneNumber:    phoneNumber,
        Status:         "active",
    }

    if err := p.db.SaveIdentity(ctx, identity); err != nil {
        return nil, err
    }

    return identity, nil
}
```

---

### 3. AI Proxy Service

**Tech Stack:** Go + Redis (rate limiting) + PostgreSQL (usage tracking)

**Features:**
- Proxies requests to Anthropic/OpenAI
- Authenticates via API key
- Tracks token usage per request
- Enforces rate limits and spending caps
- Streams responses back to client

**API:**

```
POST /v1/ai/chat
Authorization: Bearer pk_live_xxxxxxxxxxxx

{
  "model": "claude-sonnet-4-20250514",
  "messages": [...],
  "max_tokens": 4096
}

Response: Proxied response from Anthropic (streaming supported)
```

**Implementation:**

```go
// /internal/proxy/ai_proxy.go

type AIProxy struct {
    anthropic    *anthropic.Client
    openai       *openai.Client
    usageTracker *UsageTracker
    rateLimiter  *RateLimiter
}

func (p *AIProxy) HandleChatRequest(w http.ResponseWriter, r *http.Request) {
    // 1. Authenticate
    userID, err := p.authenticate(r)
    if err != nil {
        http.Error(w, "Unauthorized", 401)
        return
    }

    // 2. Check rate limits / spending caps
    if err := p.rateLimiter.Check(userID); err != nil {
        http.Error(w, "Rate limit exceeded", 429)
        return
    }

    // 3. Parse request
    var req ChatRequest
    json.NewDecoder(r.Body).Decode(&req)

    // 4. Proxy to upstream provider
    startTime := time.Now()
    resp, inputTokens, outputTokens, err := p.callUpstream(r.Context(), req)
    latency := time.Since(startTime)

    // 5. Track usage
    cost := p.calculateCost(req.Model, inputTokens, outputTokens)
    p.usageTracker.Record(userID, UsageEvent{
        Type:         "ai_request",
        Model:        req.Model,
        InputTokens:  inputTokens,
        OutputTokens: outputTokens,
        CostCents:    cost,
        LatencyMs:    latency.Milliseconds(),
    })

    // 6. Return response
    json.NewEncoder(w).Encode(resp)
}

func (p *AIProxy) calculateCost(model string, input, output int) int {
    // Pricing per 1M tokens (in cents)
    pricing := map[string]struct{ input, output int }{
        "claude-sonnet-4-20250514": {300, 1500},   // $3/$15 per 1M
        "claude-opus-4-20250514":   {1500, 7500},  // $15/$75 per 1M
        "claude-3-haiku":           {25, 125},     // $0.25/$1.25 per 1M
    }

    p := pricing[model]
    inputCost := (input * p.input) / 1_000_000
    outputCost := (output * p.output) / 1_000_000
    return inputCost + outputCost
}
```

---

### 4. Email/Signal/Calendar APIs

Thin wrappers that use the provisioned identity to interact with services.

```go
// /internal/services/email.go

type EmailService struct {
    db     *sql.DB
    gmail  *gmail.Service
}

func (e *EmailService) SendEmail(ctx context.Context, userID, to, subject, body string) error {
    // 1. Get user's provisioned identity
    identity, err := e.db.GetIdentity(ctx, userID)
    if err != nil {
        return err
    }

    // 2. Get Gmail client for this identity
    client, err := e.getGmailClient(ctx, identity)
    if err != nil {
        return err
    }

    // 3. Send email
    message := createMessage(identity.WorkspaceEmail, to, subject, body)
    _, err = client.Users.Messages.Send("me", message).Do()
    return err
}
```

---

## Pricing Model

### Plans

| Feature | Basic | Pro | Enterprise |
|---------|-------|-----|------------|
| **Base Price** | $15/mo | $35/mo | Custom |
| **Identity** | Full | Full | Full |
| **AI Tokens (included)** | 1M | 5M | Custom |
| **AI Overage** | $3/1M in, $15/1M out | $2.50/1M in, $12/1M out | Volume pricing |
| **Email** | 500/mo | Unlimited | Unlimited |
| **Signal** | 100 msgs/mo | 500 msgs/mo | Unlimited |
| **Support** | Community | Email | Dedicated |
| **SLA** | None | 99.9% | 99.99% |

### Cost Structure (Our Costs)

| Component | Our Cost | User Price | Margin |
|-----------|----------|------------|--------|
| Google Workspace | ~$3.50/user | Included | -$3.50 |
| Telnyx Number | $1/mo | Included | -$1.00 |
| Telnyx SMS | $0.004/msg | Included | ~-$0.20/mo |
| Anthropic API | Wholesale | +20% markup | +20% |
| **Net per Basic user** | ~$5/mo | $15/mo | **+$10/mo** |

---

## Infrastructure

### Deployment Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLOUD INFRASTRUCTURE                            │
│                                   (Fly.io)                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  REGION: US-EAST (Primary)              REGION: US-WEST (Replica)           │
│  ┌─────────────────────────────┐        ┌─────────────────────────────┐    │
│  │                             │        │                             │    │
│  │  ┌─────────────────────┐   │        │  ┌─────────────────────┐   │    │
│  │  │ API Gateway         │   │        │  │ API Gateway         │   │    │
│  │  │ (identity-api)      │   │◄──────►│  │ (identity-api)      │   │    │
│  │  │ 3 instances         │   │        │  │ 2 instances         │   │    │
│  │  └─────────────────────┘   │        │  └─────────────────────┘   │    │
│  │                             │        │                             │    │
│  │  ┌─────────────────────┐   │        │                             │    │
│  │  │ AI Proxy            │   │        │                             │    │
│  │  │ (ai-proxy)          │   │        │                             │    │
│  │  │ 5 instances         │   │        │                             │    │
│  │  └─────────────────────┘   │        │                             │    │
│  │                             │        │                             │    │
│  │  ┌─────────────────────┐   │        │                             │    │
│  │  │ Signal Worker       │   │        │                             │    │
│  │  │ (signal-worker)     │   │        │                             │    │
│  │  │ 1 instance          │   │        │                             │    │
│  │  └─────────────────────┘   │        │                             │    │
│  │                             │        │                             │    │
│  └─────────────────────────────┘        └─────────────────────────────┘    │
│                                                                              │
│  DATABASES                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ PostgreSQL (Neon)        │ Redis (Upstash)       │ Secrets (Doppler) │  │
│  │ - Users, Identities      │ - Rate limiting       │ - API keys        │  │
│  │ - Usage tracking         │ - Session cache       │ - Provider creds  │  │
│  │ - Billing data           │ - Token cache         │                   │  │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  EXTERNAL SERVICES                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Stripe        │ Google Workspace │ Telnyx      │ Anthropic/OpenAI   │  │
│  │ (billing)     │ (email/calendar) │ (phone/SMS) │ (AI upstream)      │  │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Domain Structure

| Domain | Purpose |
|--------|---------|
| `identity.poindexter.ai` | User portal / dashboard |
| `api.poindexter.ai` | Client API for Dex instances |
| `*.poindexter.ai` | Google Workspace email domain |

---

## Dex Integration

### Configuration in Dex

```yaml
# /opt/dex/config.yaml

identity:
  # Option 1: Managed service
  provider: "poindexter"
  api_key: "pk_live_xxxxxxxxxxxx"
  api_url: "https://api.poindexter.ai"

  # Option 2: BYOK (bring your own keys)
  # provider: "custom"
  # anthropic_key: "sk-ant-..."
  # google_credentials: "/path/to/credentials.json"
  # telnyx_api_key: "KEY..."
  # telnyx_phone_number: "+15551234567"
```

### Dex Identity Client

```go
// /internal/identity/client.go

type IdentityClient struct {
    apiKey  string
    baseURL string
    http    *http.Client
}

// Called during Dex startup
func (c *IdentityClient) GetIdentity(ctx context.Context) (*Identity, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/identity", nil)
    req.Header.Set("Authorization", "Bearer "+c.apiKey)

    resp, err := c.http.Do(req)
    // ...

    var identity Identity
    json.NewDecoder(resp.Body).Decode(&identity)
    return &identity, nil
}

// Proxy AI requests through service
func (c *IdentityClient) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
    body, _ := json.Marshal(req)
    httpReq, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/ai/chat", bytes.NewReader(body))
    httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := c.http.Do(httpReq)
    // ...
}

// Send email via service
func (c *IdentityClient) SendEmail(ctx context.Context, to, subject, body string) error {
    // POST /v1/email/send
}

// Send Signal message via service
func (c *IdentityClient) SendSignal(ctx context.Context, to, message string) error {
    // POST /v1/signal/send
}
```

---

## Implementation Phases

### Phase 1: Core Infrastructure (MVP)

- [ ] User portal (signup, login, dashboard)
- [ ] Stripe integration (subscriptions, billing)
- [ ] API key generation and authentication
- [ ] AI proxy with usage tracking
- [ ] Basic usage dashboard

**Deliverable:** Users can sign up, get API key, proxy AI requests

### Phase 2: Identity Provisioning

- [ ] Google Workspace Admin SDK integration
- [ ] Automatic account creation on signup
- [ ] Email send/receive via Gmail API
- [ ] Calendar integration

**Deliverable:** Full email/calendar identity provisioned automatically

### Phase 3: Phone/Signal

- [ ] Telnyx integration for phone numbers
- [ ] SMS receive webhooks
- [ ] Signal registration automation
- [ ] Signal send/receive API

**Deliverable:** Full communication stack (email + phone + Signal)

### Phase 4: Dex Integration

- [ ] Identity client library in Dex
- [ ] Config option for managed vs BYOK
- [ ] Seamless onboarding flow
- [ ] Usage display in Dex dashboard

**Deliverable:** One-click identity setup in Dex onboarding

### Phase 5: Polish & Scale

- [ ] Multi-region deployment
- [ ] Usage alerts and spending caps
- [ ] Admin dashboard for operators
- [ ] Enterprise features (SSO, audit logs)

---

## Security Considerations

### Data Protection

- All credentials encrypted at rest (AES-256-GCM)
- API keys hashed (bcrypt), only prefix stored in plaintext
- TLS everywhere
- Secrets stored in Doppler, not in database

### Access Control

- API keys scoped to single user
- Rate limiting per key
- Spending caps enforced
- Suspicious activity alerts

### Compliance

- SOC 2 Type II (future)
- GDPR compliant (EU data residency option)
- Data retention policies

---

## Repository Structure

```
poindexter-identity/
├── cmd/
│   ├── api/              # Main API server
│   ├── worker/           # Background jobs (provisioning, Signal)
│   └── portal/           # Next.js user portal
├── internal/
│   ├── api/              # HTTP handlers
│   ├── auth/             # API key authentication
│   ├── billing/          # Stripe integration
│   ├── db/               # Database operations
│   ├── provisioner/      # Identity provisioning
│   ├── proxy/            # AI proxy
│   └── services/         # Email, Signal, Calendar wrappers
├── migrations/           # SQL migrations
├── deploy/               # Fly.io configs
└── portal/               # Next.js frontend
```

---

## Open Questions

1. **Domain:** Use `poindexter.ai` for everything or separate domains?

2. **Signal reliability:** Signal-cli requires maintenance. Worth the complexity or skip Signal initially?

3. **AI proxy latency:** Should we use Cloudflare Workers for lower latency, or keep it simple with Fly.io?

4. **Free tier:** Should there be a free tier for evaluation? (Risk: abuse)

5. **Multi-tenancy:** Should Enterprise customers get dedicated infrastructure?
