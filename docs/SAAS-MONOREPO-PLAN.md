# Poindexter SaaS Platform

## Overview

A separate monorepo (`poindexter-cloud`) containing the managed service that provisions identities and proxies AI for Poindexter (Dex) instances.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ECOSYSTEM OVERVIEW                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  OPEN SOURCE (github.com/lirancohen/dex)                                    │
│  ├── Poindexter core application                                            │
│  ├── Can run fully standalone (Self Setup)                                  │
│  ├── OR connects to SaaS for managed identity                              │
│  └── MIT Licensed                                                           │
│                                                                              │
│  SAAS (github.com/lirancohen/poindexter-cloud) ← THIS PLAN                 │
│  ├── Identity provisioning (Workspace, Telnyx, Signal)                     │
│  ├── AI proxy (Bifrost + Stripe billing)                                   │
│  ├── User management + billing                                              │
│  └── Private / Proprietary                                                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Repository Structure

```
poindexter-cloud/
├── README.md
├── go.work                          # Go workspace for multi-module
├── go.work.sum
│
├── apps/
│   ├── api/                         # Main API gateway
│   │   ├── cmd/api/main.go
│   │   ├── internal/
│   │   │   ├── handlers/            # HTTP handlers
│   │   │   ├── middleware/          # Auth, rate limiting, logging
│   │   │   └── routes/              # Route definitions
│   │   ├── go.mod
│   │   └── Dockerfile
│   │
│   ├── proxy/                       # AI proxy service
│   │   ├── cmd/proxy/main.go
│   │   ├── internal/
│   │   │   ├── bifrost/             # Bifrost integration
│   │   │   ├── metering/            # Usage tracking
│   │   │   └── streaming/           # SSE/streaming support
│   │   ├── go.mod
│   │   └── Dockerfile
│   │
│   ├── provisioner/                 # Identity provisioning worker
│   │   ├── cmd/provisioner/main.go
│   │   ├── internal/
│   │   │   ├── workspace/           # Google Workspace provisioning
│   │   │   ├── phone/               # Telnyx phone provisioning
│   │   │   ├── signal/              # Signal registration
│   │   │   └── jobs/                # Background job handlers
│   │   ├── go.mod
│   │   └── Dockerfile
│   │
│   └── webhooks/                    # Webhook receiver service
│       ├── cmd/webhooks/main.go
│       ├── internal/
│       │   ├── stripe/              # Stripe webhooks
│       │   ├── telnyx/              # SMS/voice webhooks
│       │   └── github/              # GitHub App webhooks (if needed)
│       ├── go.mod
│       └── Dockerfile
│
├── pkg/                             # Shared packages
│   ├── auth/                        # API key auth, JWT
│   ├── billing/                     # Stripe client wrapper
│   ├── config/                      # Configuration loading
│   ├── db/                          # Database models, queries
│   ├── queue/                       # Job queue (Redis-based)
│   ├── telemetry/                   # Logging, metrics, tracing
│   └── clients/                     # External service clients
│       ├── workspace/               # Google Workspace Admin SDK
│       ├── telnyx/                  # Telnyx API
│       ├── signal/                  # signal-cli-rest-api client
│       └── anthropic/               # Anthropic API (for proxy)
│
├── migrations/                      # SQL migrations
│   ├── 001_initial.up.sql
│   ├── 001_initial.down.sql
│   └── ...
│
├── deploy/                          # Deployment configs
│   ├── fly/                         # Fly.io configs
│   │   ├── api.toml
│   │   ├── proxy.toml
│   │   ├── provisioner.toml
│   │   └── webhooks.toml
│   ├── docker-compose.yml           # Local development
│   └── docker-compose.prod.yml
│
├── scripts/                         # Utility scripts
│   ├── setup-dev.sh
│   ├── migrate.sh
│   └── seed-stripe-products.sh
│
├── .github/
│   └── workflows/
│       ├── ci.yml
│       ├── deploy-staging.yml
│       └── deploy-prod.yml
│
└── Makefile
```

---

## Services Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SAAS SERVICES ARCHITECTURE                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  INTERNET                                                                    │
│      │                                                                       │
│      ▼                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         CLOUDFLARE                                    │   │
│  │  ├── api.poindexter.ai      → API Gateway                            │   │
│  │  ├── proxy.poindexter.ai    → AI Proxy                               │   │
│  │  └── webhooks.poindexter.ai → Webhook Receiver                       │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│      │                                                                       │
│      ▼                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         FLY.IO (Compute)                              │   │
│  │                                                                        │   │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐          │   │
│  │  │ API Gateway    │  │ AI Proxy       │  │ Webhooks       │          │   │
│  │  │ (apps/api)     │  │ (apps/proxy)   │  │ (apps/webhooks)│          │   │
│  │  │                │  │                │  │                │          │   │
│  │  │ • User mgmt    │  │ • Bifrost      │  │ • Stripe       │          │   │
│  │  │ • Identity API │  │ • Streaming    │  │ • Telnyx SMS   │          │   │
│  │  │ • Billing API  │  │ • Metering     │  │ • GitHub       │          │   │
│  │  │ • Admin API    │  │                │  │                │          │   │
│  │  └───────┬────────┘  └───────┬────────┘  └───────┬────────┘          │   │
│  │          │                   │                   │                    │   │
│  │          └─────────────┬─────┴─────────────┬─────┘                    │   │
│  │                        │                   │                          │   │
│  │                        ▼                   ▼                          │   │
│  │  ┌────────────────────────────────────────────────────────────────┐  │   │
│  │  │                    PROVISIONER WORKER                          │  │   │
│  │  │                    (apps/provisioner)                          │  │   │
│  │  │                                                                │  │   │
│  │  │  Consumes jobs from Redis queue:                               │  │   │
│  │  │  • provision_identity   → Create Workspace + Phone + Signal   │  │   │
│  │  │  • deprovision_identity → Clean up on cancellation            │  │   │
│  │  │  • register_signal      → Complete Signal registration        │  │   │
│  │  │  • sync_usage           → Reconcile usage with Stripe         │  │   │
│  │  └────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│      │                                                                       │
│      ▼                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         DATA LAYER                                    │   │
│  │                                                                        │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                │   │
│  │  │ PostgreSQL   │  │ Redis        │  │ Doppler      │                │   │
│  │  │ (Neon)       │  │ (Upstash)    │  │ (Secrets)    │                │   │
│  │  │              │  │              │  │              │                │   │
│  │  │ • Users      │  │ • Job queue  │  │ • API keys   │                │   │
│  │  │ • Identities │  │ • Rate limit │  │ • Workspace  │                │   │
│  │  │ • Usage      │  │ • Token cache│  │ • Telnyx     │                │   │
│  │  │ • API keys   │  │              │  │ • Stripe     │                │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘                │   │
│  │                                                                        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│      │                                                                       │
│      ▼                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                      EXTERNAL SERVICES                                │   │
│  │                                                                        │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                │   │
│  │  │ Stripe       │  │ Google       │  │ Telnyx       │                │   │
│  │  │              │  │ Workspace    │  │              │                │   │
│  │  │ • Billing    │  │ • Admin SDK  │  │ • Numbers    │                │   │
│  │  │ • Metering   │  │ • Gmail API  │  │ • SMS        │                │   │
│  │  │ • Invoices   │  │ • Calendar   │  │ • Webhooks   │                │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘                │   │
│  │                                                                        │   │
│  │  ┌──────────────┐  ┌──────────────┐                                   │   │
│  │  │ Anthropic    │  │ Signal       │                                   │   │
│  │  │              │  │ (signal-cli) │                                   │   │
│  │  │ • Claude API │  │              │                                   │   │
│  │  │ • Upstream   │  │ • Sidecar    │                                   │   │
│  │  └──────────────┘  └──────────────┘                                   │   │
│  │                                                                        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Service Details

### 1. API Gateway (`apps/api`)

The main entry point for all client requests.

**Responsibilities:**
- User authentication (email/password, OAuth)
- API key management
- Identity CRUD operations
- Billing/subscription management
- Usage reporting endpoints

**Key Endpoints:**

```
Authentication
├── POST   /v1/auth/register           # Create account
├── POST   /v1/auth/login              # Login, get JWT
├── POST   /v1/auth/refresh            # Refresh JWT
└── POST   /v1/auth/logout             # Invalidate session

API Keys (for Dex instances)
├── GET    /v1/api-keys                # List API keys
├── POST   /v1/api-keys                # Create new key
├── DELETE /v1/api-keys/:id            # Revoke key
└── POST   /v1/api-keys/:id/rotate     # Rotate key

Identity (what Dex instances call)
├── GET    /v1/identity                # Get provisioned identity
├── GET    /v1/identity/email          # Get email config
├── GET    /v1/identity/phone          # Get phone config
└── GET    /v1/identity/signal         # Get Signal status

Email Operations (proxied to Gmail API)
├── POST   /v1/email/send              # Send email
├── GET    /v1/email/inbox             # List inbox
├── GET    /v1/email/messages/:id      # Get message
└── DELETE /v1/email/messages/:id      # Delete message

Signal Operations (proxied to signal-cli)
├── POST   /v1/signal/send             # Send message
├── GET    /v1/signal/messages         # Get received messages
└── GET    /v1/signal/contacts         # List contacts

Calendar Operations (proxied to Google Calendar)
├── GET    /v1/calendar/events         # List events
├── POST   /v1/calendar/events         # Create event
├── PUT    /v1/calendar/events/:id     # Update event
└── DELETE /v1/calendar/events/:id     # Delete event

Billing
├── GET    /v1/billing/subscription    # Get current subscription
├── POST   /v1/billing/subscribe       # Create subscription
├── POST   /v1/billing/portal          # Get Stripe portal URL
└── GET    /v1/billing/usage           # Get current usage

Admin (internal)
├── GET    /v1/admin/users             # List all users
├── GET    /v1/admin/usage             # Aggregate usage stats
└── POST   /v1/admin/provision/:id     # Manually trigger provisioning
```

---

### 2. AI Proxy (`apps/proxy`)

High-performance proxy for AI requests with metering.

**Responsibilities:**
- Authenticate incoming requests via API key
- Route to Bifrost for upstream handling
- Extract token counts from responses
- Report usage to Stripe asynchronously
- Stream responses back to client

**Architecture:**

```go
// apps/proxy/internal/proxy/handler.go

type ProxyHandler struct {
    bifrost   *bifrost.Client
    auth      *auth.Validator
    metering  *metering.Reporter
    rateLimit *ratelimit.Limiter
}

func (h *ProxyHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
    // 1. Authenticate
    identity, err := h.auth.ValidateAPIKey(r.Header.Get("Authorization"))
    if err != nil {
        http.Error(w, "Unauthorized", 401)
        return
    }

    // 2. Rate limit check
    if err := h.rateLimit.Allow(identity.UserID); err != nil {
        http.Error(w, "Rate limit exceeded", 429)
        return
    }

    // 3. Parse request
    var req openai.ChatCompletionRequest
    json.NewDecoder(r.Body).Decode(&req)

    // 4. Check if streaming
    if req.Stream {
        h.handleStreaming(w, r, identity, req)
        return
    }

    // 5. Forward to Bifrost
    resp, err := h.bifrost.ChatCompletion(r.Context(), req)
    if err != nil {
        http.Error(w, err.Error(), 502)
        return
    }

    // 6. Report usage (async)
    go h.metering.Report(identity.UserID, metering.Event{
        Model:        req.Model,
        InputTokens:  resp.Usage.PromptTokens,
        OutputTokens: resp.Usage.CompletionTokens,
    })

    // 7. Return response
    json.NewEncoder(w).Encode(resp)
}
```

**Endpoints:**

```
POST /v1/chat/completions      # OpenAI-compatible chat
POST /v1/completions           # Legacy completions
POST /v1/embeddings            # Embeddings
GET  /v1/models                # List available models
```

---

### 3. Provisioner (`apps/provisioner`)

Background worker that handles identity provisioning.

**Job Types:**

```go
// pkg/queue/jobs.go

type ProvisionIdentityJob struct {
    UserID string
    Plan   string // "basic" or "pro"
}

type DeprovisionIdentityJob struct {
    UserID   string
    Identity IdentityConfig
}

type RegisterSignalJob struct {
    UserID      string
    PhoneNumber string
    RetryCount  int
}

type SyncUsageJob struct {
    UserID string
    Month  string // "2026-02"
}
```

**Provisioning Flow:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      IDENTITY PROVISIONING FLOW                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. User subscribes (Stripe webhook)                                        │
│     └── Webhook service enqueues: ProvisionIdentityJob                      │
│                                                                              │
│  2. Provisioner picks up job                                                │
│     │                                                                        │
│     ├── 2a. Create Google Workspace account                                │
│     │   ├── Generate unique email: dex-{short_id}@poindexter.ai            │
│     │   ├── Call Admin SDK: users.insert()                                  │
│     │   ├── Store credentials in DB (encrypted)                            │
│     │   └── Generate OAuth tokens for Gmail/Calendar access                │
│     │                                                                        │
│     ├── 2b. Provision phone number                                          │
│     │   ├── Call Telnyx: phone_numbers.create()                            │
│     │   ├── Configure SMS webhook → webhooks.poindexter.ai                 │
│     │   └── Store phone number in DB                                        │
│     │                                                                        │
│     ├── 2c. Enqueue Signal registration (separate job)                     │
│     │   └── RegisterSignalJob { phone_number }                             │
│     │                                                                        │
│     └── 2d. Update identity status to "active"                             │
│                                                                              │
│  3. Signal registration (separate job, may retry)                           │
│     ├── Start signal-cli registration                                      │
│     ├── Wait for SMS verification code (via Telnyx webhook)               │
│     ├── Complete registration                                               │
│     └── Update Signal status in DB                                          │
│                                                                              │
│  4. Identity ready                                                           │
│     └── Dex instance can now call /v1/identity to get config               │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### 4. Webhooks (`apps/webhooks`)

Receives and processes webhooks from external services.

**Handlers:**

```go
// apps/webhooks/internal/handlers/stripe.go

func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
    event := stripe.ConstructEvent(r.Body, r.Header.Get("Stripe-Signature"), webhookSecret)

    switch event.Type {
    case "customer.subscription.created":
        // Enqueue provisioning job
        queue.Enqueue(ProvisionIdentityJob{...})

    case "customer.subscription.deleted":
        // Enqueue deprovisioning job
        queue.Enqueue(DeprovisionIdentityJob{...})

    case "invoice.payment_failed":
        // Suspend identity, notify user
        suspendIdentity(event.Data.Object)
    }
}

// apps/webhooks/internal/handlers/telnyx.go

func HandleTelnyxWebhook(w http.ResponseWriter, r *http.Request) {
    event := telnyx.ParseWebhook(r)

    switch event.Type {
    case "message.received":
        // Check if this is a Signal verification code
        if isVerificationCode(event.Body) {
            // Complete pending Signal registration
            completeSignalRegistration(event.From, event.Body)
        } else {
            // Store for user retrieval via API
            storeIncomingSMS(event)
        }
    }
}
```

---

## Database Schema

```sql
-- migrations/001_initial.up.sql

-- Users (account holders)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT,

    -- Stripe
    stripe_customer_id TEXT UNIQUE,

    -- Status
    status TEXT NOT NULL DEFAULT 'pending', -- pending, active, suspended

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Subscriptions
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- Stripe
    stripe_subscription_id TEXT UNIQUE NOT NULL,
    stripe_price_id TEXT NOT NULL,

    -- Plan info
    plan TEXT NOT NULL, -- 'basic', 'pro'
    status TEXT NOT NULL, -- 'active', 'past_due', 'canceled'

    -- Billing period
    current_period_start TIMESTAMPTZ,
    current_period_end TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id) -- One subscription per user
);

-- Provisioned identities
CREATE TABLE identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- Google Workspace
    workspace_email TEXT UNIQUE,
    workspace_user_id TEXT,
    workspace_refresh_token_encrypted TEXT,

    -- Phone (Telnyx)
    phone_number TEXT UNIQUE,
    phone_telnyx_id TEXT,

    -- Signal
    signal_registered BOOLEAN DEFAULT FALSE,
    signal_device_id INTEGER,

    -- Status
    status TEXT NOT NULL DEFAULT 'provisioning',
    -- provisioning, active, suspended, deprovisioning, deleted

    provisioned_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id) -- One identity per user
);

-- API keys (for Dex instances to authenticate)
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- Key info
    key_hash TEXT UNIQUE NOT NULL,        -- bcrypt hash
    key_prefix TEXT NOT NULL,             -- First 8 chars for display: pk_live_
    name TEXT,                            -- User-provided name

    -- Metadata
    last_used_at TIMESTAMPTZ,
    last_used_ip TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Allow multiple keys per user
    INDEX idx_api_keys_user_id (user_id)
);

-- Usage events (append-only, high volume)
CREATE TABLE usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL, -- No FK for performance
    api_key_id UUID,

    -- Event type
    event_type TEXT NOT NULL, -- 'ai_request', 'email_sent', 'signal_sent', etc.

    -- AI-specific fields
    model TEXT,
    input_tokens INTEGER,
    output_tokens INTEGER,

    -- Cost (calculated at insert time, in microdollars for precision)
    cost_microdollars INTEGER,

    -- Request metadata
    request_id TEXT,
    latency_ms INTEGER,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Partition by month for efficient queries
    INDEX idx_usage_events_user_month (user_id, created_at)
) PARTITION BY RANGE (created_at);

-- Create monthly partitions
CREATE TABLE usage_events_2026_01 PARTITION OF usage_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE usage_events_2026_02 PARTITION OF usage_events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
-- ... create partitions for each month

-- Monthly usage aggregates (materialized for billing)
CREATE TABLE usage_monthly (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    month DATE NOT NULL, -- First day of month

    -- Token counts
    ai_input_tokens BIGINT DEFAULT 0,
    ai_output_tokens BIGINT DEFAULT 0,

    -- Costs (microdollars)
    ai_cost_microdollars BIGINT DEFAULT 0,

    -- Counts
    emails_sent INTEGER DEFAULT 0,
    emails_received INTEGER DEFAULT 0,
    signal_messages_sent INTEGER DEFAULT 0,
    signal_messages_received INTEGER DEFAULT 0,
    sms_sent INTEGER DEFAULT 0,
    sms_received INTEGER DEFAULT 0,

    -- Stripe sync
    synced_to_stripe BOOLEAN DEFAULT FALSE,
    synced_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id, month)
);

-- Incoming SMS storage (for Signal verification and general SMS)
CREATE TABLE incoming_sms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number TEXT NOT NULL, -- Destination (our number)
    from_number TEXT NOT NULL,
    body TEXT NOT NULL,

    -- Matching
    identity_id UUID REFERENCES identities(id),
    processed BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    INDEX idx_incoming_sms_phone (phone_number, created_at DESC)
);

-- Background jobs (for visibility, actual queue is Redis)
CREATE TABLE job_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL,
    payload JSONB NOT NULL,

    status TEXT NOT NULL, -- 'pending', 'running', 'completed', 'failed'
    attempts INTEGER DEFAULT 0,
    last_error TEXT,

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Configuration

```go
// pkg/config/config.go

type Config struct {
    // Server
    Environment string `env:"ENVIRONMENT" default:"development"`
    Port        int    `env:"PORT" default:"8080"`

    // Database
    DatabaseURL string `env:"DATABASE_URL" required:"true"`

    // Redis
    RedisURL string `env:"REDIS_URL" required:"true"`

    // Stripe
    StripeSecretKey      string `env:"STRIPE_SECRET_KEY" required:"true"`
    StripeWebhookSecret  string `env:"STRIPE_WEBHOOK_SECRET" required:"true"`
    StripePriceBasic     string `env:"STRIPE_PRICE_BASIC" required:"true"`
    StripePricePro       string `env:"STRIPE_PRICE_PRO" required:"true"`

    // Google Workspace
    GoogleServiceAccountJSON string `env:"GOOGLE_SERVICE_ACCOUNT_JSON" required:"true"`
    GoogleDomain             string `env:"GOOGLE_DOMAIN" default:"poindexter.ai"`
    GoogleAdminEmail         string `env:"GOOGLE_ADMIN_EMAIL" required:"true"`

    // Telnyx
    TelnyxAPIKey        string `env:"TELNYX_API_KEY" required:"true"`
    TelnyxWebhookSecret string `env:"TELNYX_WEBHOOK_SECRET" required:"true"`
    TelnyxMessagingProfileID string `env:"TELNYX_MESSAGING_PROFILE_ID" required:"true"`

    // Signal
    SignalCLIURL string `env:"SIGNAL_CLI_URL" default:"http://signal:8080"`

    // Anthropic (for proxy)
    AnthropicAPIKey string `env:"ANTHROPIC_API_KEY" required:"true"`

    // Encryption
    EncryptionKey string `env:"ENCRYPTION_KEY" required:"true"` // 32-byte hex
}
```

---

## Stripe Products Setup

```bash
#!/bin/bash
# scripts/seed-stripe-products.sh

# Create products
BASIC_PRODUCT=$(stripe products create \
    --name="Poindexter Basic" \
    --description="5M tokens included, email, phone, Signal" \
    | jq -r '.id')

PRO_PRODUCT=$(stripe products create \
    --name="Poindexter Pro" \
    --description="30M tokens included, priority support" \
    | jq -r '.id')

# Create base subscription prices
stripe prices create \
    --product=$BASIC_PRODUCT \
    --unit-amount=5000 \
    --currency=usd \
    --recurring[interval]=month \
    --nickname="basic_monthly"

stripe prices create \
    --product=$PRO_PRODUCT \
    --unit-amount=25000 \
    --currency=usd \
    --recurring[interval]=month \
    --nickname="pro_monthly"

# Create metered prices for overages
# Basic overage: $3/1M input, $15/1M output
stripe prices create \
    --product=$BASIC_PRODUCT \
    --billing-scheme=per_unit \
    --unit-amount-decimal=0.3 \
    --currency=usd \
    --recurring[interval]=month \
    --recurring[usage_type]=metered \
    --nickname="basic_input_tokens"

stripe prices create \
    --product=$BASIC_PRODUCT \
    --billing-scheme=per_unit \
    --unit-amount-decimal=1.5 \
    --currency=usd \
    --recurring[interval]=month \
    --recurring[usage_type]=metered \
    --nickname="basic_output_tokens"

# Pro overage: $2.50/1M input, $12/1M output
stripe prices create \
    --product=$PRO_PRODUCT \
    --billing-scheme=per_unit \
    --unit-amount-decimal=0.25 \
    --currency=usd \
    --recurring[interval]=month \
    --recurring[usage_type]=metered \
    --nickname="pro_input_tokens"

stripe prices create \
    --product=$PRO_PRODUCT \
    --billing-scheme=per_unit \
    --unit-amount-decimal=1.2 \
    --currency=usd \
    --recurring[interval]=month \
    --recurring[usage_type]=metered \
    --nickname="pro_output_tokens"
```

---

## Deployment

### Fly.io Configuration

```toml
# deploy/fly/api.toml

app = "poindexter-api"
primary_region = "iad"

[build]
  dockerfile = "apps/api/Dockerfile"

[env]
  ENVIRONMENT = "production"
  PORT = "8080"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = false
  auto_start_machines = true
  min_machines_running = 2

[[services]]
  internal_port = 8080
  protocol = "tcp"

  [[services.ports]]
    handlers = ["http"]
    port = 80
    force_https = true

  [[services.ports]]
    handlers = ["tls", "http"]
    port = 443

[metrics]
  port = 9090
  path = "/metrics"
```

```toml
# deploy/fly/proxy.toml

app = "poindexter-proxy"
primary_region = "iad"

[build]
  dockerfile = "apps/proxy/Dockerfile"

[env]
  ENVIRONMENT = "production"
  PORT = "8080"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = false
  auto_start_machines = true
  min_machines_running = 3  # More instances for proxy

# Larger machines for AI proxy (memory for streaming)
[[vm]]
  memory = "1gb"
  cpu_kind = "shared"
  cpus = 2
```

### Local Development

```yaml
# deploy/docker-compose.yml

version: '3.8'

services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: poindexter
      POSTGRES_USER: poindexter
      POSTGRES_PASSWORD: dev_password
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  signal:
    image: bbernhard/signal-cli-rest-api
    environment:
      MODE: "json-rpc"
    ports:
      - "8081:8080"
    volumes:
      - signal_data:/home/.local/share/signal-cli

  api:
    build:
      context: .
      dockerfile: apps/api/Dockerfile
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://poindexter:dev_password@postgres:5432/poindexter
      REDIS_URL: redis://redis:6379
      SIGNAL_CLI_URL: http://signal:8080
    depends_on:
      - postgres
      - redis

  proxy:
    build:
      context: .
      dockerfile: apps/proxy/Dockerfile
    ports:
      - "8082:8080"
    environment:
      DATABASE_URL: postgres://poindexter:dev_password@postgres:5432/poindexter
      REDIS_URL: redis://redis:6379
    depends_on:
      - postgres
      - redis

  provisioner:
    build:
      context: .
      dockerfile: apps/provisioner/Dockerfile
    environment:
      DATABASE_URL: postgres://poindexter:dev_password@postgres:5432/poindexter
      REDIS_URL: redis://redis:6379
      SIGNAL_CLI_URL: http://signal:8080
    depends_on:
      - postgres
      - redis
      - signal

  webhooks:
    build:
      context: .
      dockerfile: apps/webhooks/Dockerfile
    ports:
      - "8083:8080"
    environment:
      DATABASE_URL: postgres://poindexter:dev_password@postgres:5432/poindexter
      REDIS_URL: redis://redis:6379
    depends_on:
      - postgres
      - redis

volumes:
  postgres_data:
  signal_data:
```

---

## Integration with Open Source Dex

### Dex Configuration

```yaml
# In the open-source Dex repo: /opt/dex/config.yaml

identity:
  # Option 1: Use Poindexter Cloud (managed)
  provider: "poindexter"
  api_key: "${POINDEXTER_API_KEY}"
  api_url: "https://api.poindexter.ai"
  proxy_url: "https://proxy.poindexter.ai"

  # Option 2: Self-managed (BYOK)
  # provider: "custom"
  # anthropic_key: "${ANTHROPIC_API_KEY}"
  # google_credentials_file: "/opt/dex/google-credentials.json"
  # telnyx_api_key: "${TELNYX_API_KEY}"
  # telnyx_phone_number: "+15551234567"
```

### Dex Identity Client

```go
// In open-source Dex: /internal/identity/cloud_client.go

type CloudClient struct {
    apiKey   string
    apiURL   string
    proxyURL string
    http     *http.Client
}

// Fetch identity on startup
func (c *CloudClient) GetIdentity(ctx context.Context) (*Identity, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", c.apiURL+"/v1/identity", nil)
    req.Header.Set("Authorization", "Bearer "+c.apiKey)

    resp, err := c.http.Do(req)
    // ... parse response
}

// Create AI client that routes through proxy
func (c *CloudClient) NewAnthropicClient() *anthropic.Client {
    return anthropic.NewClient(
        anthropic.WithAPIKey(c.apiKey),  // Uses our key, not Anthropic's
        anthropic.WithBaseURL(c.proxyURL),
    )
}

// Email operations
func (c *CloudClient) SendEmail(ctx context.Context, to, subject, body string) error {
    req := EmailRequest{To: to, Subject: subject, Body: body}
    // POST to c.apiURL + "/v1/email/send"
}

// Signal operations
func (c *CloudClient) SendSignal(ctx context.Context, to, message string) error {
    // POST to c.apiURL + "/v1/signal/send"
}
```

---

## Implementation Phases

### Phase 1: Core Infrastructure (Week 1-2)

- [ ] Set up monorepo structure
- [ ] Database schema + migrations
- [ ] Basic API service with auth
- [ ] API key generation
- [ ] User registration/login
- [ ] Stripe integration (subscriptions)
- [ ] Deploy to Fly.io (staging)

### Phase 2: AI Proxy (Week 2-3)

- [ ] Bifrost integration
- [ ] Request authentication
- [ ] Usage metering
- [ ] Stripe usage reporting
- [ ] Streaming support
- [ ] Rate limiting

### Phase 3: Identity Provisioning (Week 3-4)

- [ ] Google Workspace Admin SDK integration
- [ ] Account creation flow
- [ ] Gmail/Calendar API proxying
- [ ] Telnyx phone provisioning
- [ ] SMS webhook handling
- [ ] Provisioner worker

### Phase 4: Signal Integration (Week 4-5)

- [ ] signal-cli-rest-api deployment
- [ ] Signal registration flow
- [ ] Verification code handling
- [ ] Send/receive messaging
- [ ] Account maintenance (keep-alive)

### Phase 5: Dex Integration (Week 5-6)

- [ ] Identity client in Dex
- [ ] Onboarding flow (choose managed vs self-setup)
- [ ] Embedded Stripe checkout
- [ ] Seamless configuration

### Phase 6: Production Hardening (Week 6-7)

- [ ] Security audit
- [ ] Rate limiting tuning
- [ ] Monitoring + alerting
- [ ] Documentation
- [ ] Production deployment

---

## Security Considerations

### Secrets Management

- All secrets in Doppler
- Database credentials encrypted
- API keys hashed (bcrypt)
- Refresh tokens encrypted (AES-256-GCM)

### API Security

- HTTPS everywhere (Cloudflare)
- API key authentication
- Rate limiting per key
- Request signing for webhooks
- Input validation

### Data Protection

- PII encrypted at rest
- Minimal data retention
- GDPR compliance (deletion on request)
- Audit logging

---

## Monitoring

### Metrics (Prometheus)

- Request latency (p50, p95, p99)
- Request rate by endpoint
- Error rate by type
- AI proxy token throughput
- Provisioning success/failure rate
- Queue depth and processing time

### Alerting

- Error rate > 1%
- Latency p99 > 500ms
- Provisioning failures
- Payment failures
- Disk/memory pressure

### Logging

- Structured JSON logs
- Request ID correlation
- Log aggregation (BetterStack)

---

## Estimated Costs (at 100 users)

| Service | Cost/month |
|---------|-----------|
| Fly.io (4 services, 2-3 instances each) | ~$50 |
| Neon (PostgreSQL) | ~$19 (Pro) |
| Upstash (Redis) | ~$10 |
| Cloudflare | Free |
| Doppler | Free (starter) |
| Google Workspace (100 users) | ~$350 |
| Telnyx (100 numbers + SMS) | ~$150 |
| **Total Infrastructure** | **~$580/month** |

**Revenue at 100 users:**
- 80 Basic ($50) + 20 Pro ($250) = $9,000/month
- Minus AI costs (~$3,000 estimated)
- Minus infrastructure (~$600)
- **Net: ~$5,400/month**
