# Poindexter Identity Service (PIS)

## Overview

A managed service that provisions complete digital identities for Poindexter instances:
- Google Workspace account (email + calendar)
- Phone number + Signal account
- AI API access (proxied, metered, billed)

**Two User Options:**
1. **Self Setup** - User configures their own Gmail, Signal, Anthropic API key, etc.
2. **Hosted Service** - We provide everything, two tiers:
   - **Basic ($50/mo)** - Includes 5M tokens + identity
   - **Pro ($250/mo)** - Includes 30M tokens + identity + priority support

**Architecture Notes:**
- User portal is part of the main Poindexter app (not separate)
- Backend is Go (not open source, operator-only)
- AI proxy uses Bifrost (Go-based, fastest) + Stripe metered billing

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

### Option 1: Hosted Service (Recommended)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      HOSTED SERVICE ONBOARDING                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  IN POINDEXTER APP (during Dex onboarding)                                  │
│                                                                              │
│  1. USER CHOOSES HOSTED SERVICE                                             │
│     ┌─────────────────────────────────────────────────────────────────┐    │
│     │  How would you like to set up Poindexter's identity?            │    │
│     │                                                                  │    │
│     │  ○ Self Setup (bring your own API keys and accounts)            │    │
│     │  ● Hosted Service (we handle everything)     [Recommended]      │    │
│     │                                                                  │    │
│     │  ┌─────────────────┐  ┌─────────────────┐                       │    │
│     │  │ Basic - $50/mo  │  │ Pro - $250/mo   │                       │    │
│     │  │ 5M tokens       │  │ 30M tokens      │                       │    │
│     │  │ Email + Signal  │  │ Email + Signal  │                       │    │
│     │  │ Email support   │  │ Priority support│                       │    │
│     │  │ [Select]        │  │ [Select]        │                       │    │
│     │  └─────────────────┘  └─────────────────┘                       │    │
│     └─────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  2. STRIPE CHECKOUT (embedded in app)                                       │
│     └── Credit card → Subscription created                                  │
│                                                                              │
│  3. IDENTITY PROVISIONED (automatic, ~30 seconds)                           │
│     ├── Google Workspace account: dex-{id}@poindexter.ai                   │
│     ├── Phone number assigned: +1-555-xxx-xxxx                             │
│     ├── Signal registered automatically                                     │
│     └── API key generated: pk_live_xxxxxxxxxxxx                            │
│                                                                              │
│  4. DEX CONFIGURED AUTOMATICALLY                                            │
│     └── No manual steps - app stores API key, fetches identity             │
│                                                                              │
│  5. READY TO USE                                                             │
│     ├── AI requests proxied + metered                                       │
│     ├── Email/Signal/Calendar working                                       │
│     └── Usage visible in dashboard                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Option 2: Self Setup (BYOK)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SELF SETUP ONBOARDING                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  IN POINDEXTER APP (during Dex onboarding)                                  │
│                                                                              │
│  1. USER CHOOSES SELF SETUP                                                 │
│     └── "I'll configure my own credentials"                                │
│                                                                              │
│  2. STEP-BY-STEP CONFIGURATION                                              │
│     ┌─────────────────────────────────────────────────────────────────┐    │
│     │  Configure Your Identity                                         │    │
│     │                                                                  │    │
│     │  AI API Key                                                      │    │
│     │  ┌──────────────────────────────────────────────────────────┐  │    │
│     │  │ sk-ant-api03-xxxxxxxxxxxxx                               │  │    │
│     │  └──────────────────────────────────────────────────────────┘  │    │
│     │  Provider: ○ Anthropic  ○ OpenAI  ○ Other                      │    │
│     │                                                                  │    │
│     │  Email (Optional)                                                │    │
│     │  ┌──────────────────────────────────────────────────────────┐  │    │
│     │  │ [Connect Google Account]  or  [Skip]                     │  │    │
│     │  └──────────────────────────────────────────────────────────┘  │    │
│     │                                                                  │    │
│     │  Phone/Signal (Optional)                                         │    │
│     │  ┌──────────────────────────────────────────────────────────┐  │    │
│     │  │ Telnyx API Key: ___________  Phone: +1-555-xxx-xxxx     │  │    │
│     │  └──────────────────────────────────────────────────────────┘  │    │
│     │                                                                  │    │
│     │  [Complete Setup]                                                │    │
│     └─────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  3. DEX OPERATES INDEPENDENTLY                                              │
│     └── No dependency on Poindexter Identity Service                       │
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

**Tech Stack:** Bifrost (Go) + Stripe Metered Billing

**Why Bifrost:**
- Written in Go (matches our stack)
- 50x faster than LiteLLM (<100µs overhead at 5K RPS)
- Built-in failover, load balancing, caching
- OpenAI-compatible API for all providers
- Virtual keys with budget tracking

**Architecture:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         AI PROXY ARCHITECTURE                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Dex Instance                                                                │
│       │                                                                      │
│       │ POST /v1/chat/completions                                           │
│       │ Authorization: Bearer pk_live_xxx                                   │
│       ▼                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                     AUTH + METERING LAYER (Go)                        │   │
│  │  1. Validate API key → get user_id, plan                             │   │
│  │  2. Check if user has remaining included tokens                       │   │
│  │  3. Forward to Bifrost                                                │   │
│  │  4. On response: extract token counts                                 │   │
│  │  5. Report usage to Stripe Metering API                              │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│       │                                                                      │
│       ▼                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         BIFROST PROXY                                 │   │
│  │  - Routes to Anthropic/OpenAI/etc.                                   │   │
│  │  - Handles failover, retries, caching                                │   │
│  │  - Returns OpenAI-compatible response                                │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│       │                                                                      │
│       ▼                                                                      │
│  Anthropic / OpenAI / etc.                                                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Stripe Metered Billing Integration:**

```go
// Stripe Products Setup (one-time)
//
// Product: "Poindexter Basic"
//   Price: $50/month (subscription)
//   Metered Price: $0.000003/input_token (overage)
//   Metered Price: $0.000015/output_token (overage)
//   Free tier: 5M tokens included
//
// Product: "Poindexter Pro"
//   Price: $250/month (subscription)
//   Metered Price: $0.0000025/input_token (overage)
//   Metered Price: $0.000012/output_token (overage)
//   Free tier: 30M tokens included

// /internal/billing/stripe.go

type StripeBilling struct {
    client *stripe.Client
}

// Report usage after each AI request
func (s *StripeBilling) ReportUsage(ctx context.Context, subscriptionItemID string, tokens int64) error {
    _, err := s.client.UsageRecords.New(&stripe.UsageRecordParams{
        SubscriptionItem: stripe.String(subscriptionItemID),
        Quantity:         stripe.Int64(tokens),
        Timestamp:        stripe.Int64(time.Now().Unix()),
        Action:           stripe.String("increment"),
    })
    return err
}

// Check included tokens remaining
func (s *StripeBilling) GetIncludedTokensRemaining(ctx context.Context, userID string) (int64, error) {
    // Query usage this billing period
    // Subtract from plan's included amount
    // Return remaining (can be negative = overage)
}
```

**Metering Flow:**

```
Request comes in
    │
    ▼
┌─────────────────────────────────────────┐
│ 1. Auth: Validate pk_live_xxx           │
│    → Get user_id, stripe_subscription   │
└─────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────┐
│ 2. Proxy request through Bifrost        │
│    → Get response + token counts        │
└─────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────┐
│ 3. Report to Stripe (async)             │
│    → UsageRecords.New(input_tokens)     │
│    → UsageRecords.New(output_tokens)    │
└─────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────┐
│ 4. Stripe handles billing:              │
│    - Tracks cumulative usage            │
│    - Subtracts included tier amount     │
│    - Bills overage at invoice time      │
└─────────────────────────────────────────┘
```

**API (OpenAI-compatible):**

```
POST /v1/chat/completions
Authorization: Bearer pk_live_xxxxxxxxxxxx

{
  "model": "claude-sonnet-4-20250514",
  "messages": [...],
  "max_tokens": 4096
}

Response: Standard OpenAI-format response (streaming supported)
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

| Feature | Basic | Pro |
|---------|-------|-----|
| **Price** | $50/mo | $250/mo |
| **Included Tokens** | 5M | 30M |
| **Overage (input)** | $3/1M | $2.50/1M |
| **Overage (output)** | $15/1M | $12/1M |
| **Identity** | Full (email, phone, Signal) | Full |
| **Email** | Unlimited | Unlimited |
| **Signal** | Unlimited | Unlimited |
| **Support** | Email | Priority |
| **SLA** | 99.9% | 99.99% |

### Token Economics

```
Claude Sonnet pricing (our cost): $3 input / $15 output per 1M tokens

Basic Plan ($50/mo):
├── 5M tokens included ≈ $15-75 in AI costs (depending on in/out ratio)
├── Identity costs: ~$5/mo (Workspace + Telnyx)
├── Margin on base: $50 - $20 = ~$30
└── Overage: Pass-through at cost (no margin, but increases stickiness)

Pro Plan ($250/mo):
├── 30M tokens included ≈ $90-450 in AI costs
├── Identity costs: ~$5/mo
├── Margin on base: $250 - $100 = ~$150 (assuming 50/50 in/out)
└── Overage: Slight discount (customer loyalty)
```

### Cost Structure (Per User)

| Component | Our Cost | Included In |
|-----------|----------|-------------|
| Google Workspace | ~$3.50/user | Base price |
| Telnyx Number | $1/mo | Base price |
| Telnyx SMS (~100 msgs) | ~$0.40/mo | Base price |
| Anthropic (5M tokens) | ~$45/mo* | Basic tier |
| Anthropic (30M tokens) | ~$135/mo* | Pro tier |

*Assuming 50/50 input/output ratio

### Margin Analysis

| Plan | Revenue | Costs | Margin |
|------|---------|-------|--------|
| Basic (light user) | $50 | ~$20 | **+$30** |
| Basic (heavy user) | $50 + overage | ~$50 | **+$0** (but overage profit) |
| Pro (light user) | $250 | ~$50 | **+$200** |
| Pro (heavy user) | $250 + overage | ~$150 | **+$100** |

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
