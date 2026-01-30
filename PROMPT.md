# POINDEXTER (dex) — Your Nerdy AI Orchestration Genius

## Mission

Build **Poindexter** — a self-contained, single-user system for orchestrating 25+ concurrent Claude Code sessions on a local machine. Poindexter is the brilliant nerd who manages your AI workforce: decomposing tasks, assigning specialized "hats" to sessions, managing git worktrees for isolation, and building complete, deployed applications using a curated toolbelt of cloud services.

**Nickname:** `dex`  
**Personality:** Helpful, meticulous, slightly obsessive about clean code and proper git hygiene. Knows his tools inside and out.

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│   "I've got 8 sessions running, 3 in queue. Project Alpha      │
│    just deployed to Fly.io — want me to set up the custom      │
│    domain on Cloudflare?"                                       │
│                                                                 │
│                                        — Poindexter             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Two Distinct Concerns

### 1. Poindexter Himself (Self-Contained, Local)

Poindexter runs entirely on your machine. No external dependencies for his own operation.

| Component | Technology |
|-----------|------------|
| Database | **SQLite** (single file, zero config) |
| Cache/Queue | **In-memory** (Go channels + sync.Map) |
| State | **Local filesystem** |
| API | **Go + Echo** (serves API + frontend) |
| Frontend | **React + Bun + Tailwind** |
| Access | **Tailscale HTTPS** (private, secure) |
| Auth | **BIP39 passphrase → Ed25519** |

### 2. Poindexter's Toolbelt (For Building YOUR Projects)

When Poindexter builds projects for you, he has a curated set of cloud services at his disposal. You provide API keys; he provisions, deploys, and manages.

---

## The Toolbelt

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         POINDEXTER'S TOOLBELT                                    │
│                                                                                  │
│   You provide the API keys. Poindexter does the rest.                           │
│                                                                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│   CODE & CI/CD                                                                  │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  GitHub                                                                   │ │
│   │  • Repositories, Issues, Pull Requests                                   │ │
│   │  • Actions for CI/CD pipelines                                           │ │
│   │  • Packages for container registry                                       │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   COMPUTE                                                                        │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Fly.io                                                                   │ │
│   │  • Deploy containers globally                                            │ │
│   │  • Auto-scaling, zero-downtime deploys                                   │ │
│   │  • Machines API for programmatic control                                 │ │
│   │  • Volumes for persistent storage                                        │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   EDGE / CDN / DNS / STORAGE                                                    │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Cloudflare                                                               │ │
│   │  • DNS management                                                        │ │
│   │  • CDN and caching                                                       │ │
│   │  • WAF and DDoS protection                                               │ │
│   │  • R2 for S3-compatible object storage (no egress fees)                  │ │
│   │  • Workers for edge functions                                            │ │
│   │  • KV for simple key-value storage                                       │ │
│   │  • Pages for static site hosting                                         │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   DATABASE                                                                       │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Neon                                                                     │ │
│   │  • Serverless PostgreSQL                                                 │ │
│   │  • Database branching (like git for databases)                           │ │
│   │  • Scale-to-zero (cost efficient)                                        │ │
│   │  • Point-in-time recovery                                                │ │
│   │  • Connection pooling built-in                                           │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   CACHE / QUEUE / RATE LIMITING                                                 │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Upstash                                                                  │ │
│   │  • Serverless Redis (REST API — works everywhere)                        │ │
│   │  • Queues (QStash) for background jobs                                   │ │
│   │  • Rate limiting                                                         │ │
│   │  • Kafka for event streaming                                             │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   EMAIL                                                                          │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Resend                                                                   │ │
│   │  • Transactional email API                                               │ │
│   │  • React Email for templates                                             │ │
│   │  • Webhooks for delivery tracking                                        │ │
│   │  • Custom domains                                                        │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   MONITORING / LOGGING                                                          │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Better Stack                                                             │ │
│   │  • Log aggregation and search                                            │ │
│   │  • Uptime monitoring                                                     │ │
│   │  • Incident management                                                   │ │
│   │  • Status pages                                                          │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   SECRETS MANAGEMENT                                                            │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Doppler                                                                  │ │
│   │  • Centralized secrets for all environments                              │ │
│   │  • Sync to Fly.io, Vercel, etc.                                          │ │
│   │  • Automatic rotation                                                    │ │
│   │  • Audit logs                                                            │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   PAYMENTS                                                                       │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  MoneyDevKit                                                              │ │
│   │  • Payment processing                                                    │ │
│   │  • Subscriptions and billing                                             │ │
│   │  • Invoicing                                                             │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   AI / LLM                                                                       │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Anthropic                                                                │ │
│   │  • Claude API for LLM features                                           │ │
│   │  • Chat, completion, embeddings                                          │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   AI / MEDIA GENERATION                                                         │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  fal.ai                                                                   │ │
│   │  • Image generation (Flux, SD, etc.)                                     │ │
│   │  • Video generation                                                      │ │
│   │  • Audio processing                                                      │ │
│   │  • Real-time inference                                                   │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│   AUTH (Self-Hosted Libraries)                                                  │
│   ┌──────────────────────────────────────────────────────────────────────────┐ │
│   │  Options Poindexter can implement:                                        │ │
│   │  • better-auth — Full-featured, Next.js/SvelteKit/etc.                   │ │
│   │  • lucia — Lightweight, flexible sessions                                │ │
│   │  • Custom JWT — When you need full control                               │ │
│   │  • OAuth integrations — Google, GitHub, etc.                             │ │
│   └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              YOUR DEVICE                                         │
│                     https://dex.{tailnet}.ts.net                                │
│                     Mobile-first React UI                                        │
└─────────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ Tailscale (WireGuard TLS)
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                     HOST MACHINE (Poindexter's Home)                             │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         Dex API (Go + Echo)                              │   │
│  │         HTTPS via Tailscale certs • BIP39 Auth • WebSocket Hub          │   │
│  │                    Serves React frontend                                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                       │                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      Dex Orchestrator (Go)                               │   │
│  │   Task Scheduler • Hat Manager • GitHub Sync • Resource Monitor          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                       │                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                 Session Manager (25 parallel max)                        │   │
│  │                                                                          │   │
│  │   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐              │   │
│  │   │Session 1 │  │Session 2 │  │Session 3 │  │Session N │              │   │
│  │   │task-a1b2 │  │task-c3d4 │  │task-e5f6 │  │task-xxxx │              │   │
│  │   │[Architect│  │[Implement│  │[Reviewer]│  │[DevOps]  │              │   │
│  │   │Ralph Loop│  │Ralph Loop│  │Ralph Loop│  │Ralph Loop│              │   │
│  │   └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘              │   │
│  │        └─────────────┴─────────────┴─────────────┘                     │   │
│  │                          │                                              │   │
│  │                          ▼                                              │   │
│  │              Claude Agent SDK (TypeScript)                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    LOCAL STATE (Self-Contained)                          │   │
│  │                                                                          │   │
│  │  /opt/dex/                                                               │   │
│  │  ├── dex.db              # SQLite database (all state)                  │   │
│  │  ├── config.yaml         # Configuration                                │   │
│  │  ├── toolbelt.yaml       # API keys for external services               │   │
│  │  └── prompts/hats/       # Hat prompt templates                         │   │
│  │                                                                          │   │
│  │  ~/src/                   # Git repos (protected)                       │   │
│  │  ~/src/worktrees/         # Task worktrees (isolated)                   │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    TOOLBELT INTEGRATIONS (Go Clients)                    │   │
│  │                                                                          │   │
│  │   ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐         │   │
│  │   │ GitHub  │ │ Fly.io  │ │Cloudflare │ │  Neon   │ │ Upstash │         │   │
│  │   └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘         │   │
│  │   ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐         │   │
│  │   │ Resend  │ │Better   │ │ Doppler │ │MoneyDev │ │Anthropic│         │   │
│  │   │         │ │ Stack   │ │         │ │  Kit    │ │         │         │   │
│  │   └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘         │   │
│  │   ┌─────────┐                                                          │   │
│  │   │ fal.ai  │                                                          │   │
│  │   └─────────┘                                                          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ Poindexter provisions & deploys
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         YOUR PROJECTS (Deployed)                                 │
│                                                                                  │
│   ┌─────────────────────────────────────────────────────────────────────────┐  │
│   │  Project Alpha (SaaS App)                                                │  │
│   │  • Fly.io: api.alpha.com (Go backend)                                   │  │
│   │  • Cloudflare Pages: alpha.com (React frontend)                         │  │
│   │  • Cloudflare: DNS, CDN, R2 for uploads                                 │  │
│   │  • Neon: PostgreSQL database                                            │  │
│   │  • Upstash: Redis cache + job queue                                     │  │
│   │  • Resend: Transactional emails                                         │  │
│   │  • Better Stack: Logs + monitoring                                      │  │
│   │  • Doppler: Secrets management                                          │  │
│   │  • MoneyDevKit: Subscriptions                                           │  │
│   │  • Anthropic: AI features                                               │  │
│   └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
│   ┌─────────────────────────────────────────────────────────────────────────┐  │
│   │  Project Beta (API Service)                                              │  │
│   │  • Fly.io: api.beta.com                                                 │  │
│   │  • Neon: Database                                                       │  │
│   │  • Better Stack: Monitoring                                             │  │
│   └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Toolbelt Configuration

```yaml
# /opt/dex/toolbelt.yaml
# Poindexter's API keys for building your projects

github:
  token: ${GITHUB_TOKEN}
  default_org: your-org

fly:
  token: ${FLY_API_TOKEN}
  default_region: ord

cloudflare:
  api_token: ${CLOUDFLARE_API_TOKEN}
  account_id: ${CLOUDFLARE_ACCOUNT_ID}

neon:
  api_key: ${NEON_API_KEY}
  default_region: aws-us-east-1

upstash:
  email: ${UPSTASH_EMAIL}
  api_key: ${UPSTASH_API_KEY}

resend:
  api_key: ${RESEND_API_KEY}

better_stack:
  api_token: ${BETTER_STACK_API_TOKEN}

doppler:
  token: ${DOPPLER_TOKEN}

moneydevkit:
  api_key: ${MONEYDEVKIT_API_KEY}
  webhook_secret: ${MONEYDEVKIT_WEBHOOK_SECRET}

anthropic:
  api_key: ${ANTHROPIC_API_KEY}

fal:
  api_key: ${FAL_API_KEY}
```

---

## Toolbelt Client Interfaces

Each service gets a Go client that Poindexter can use:

```go
// internal/toolbelt/toolbelt.go

type Toolbelt struct {
    GitHub      *github.Client
    Fly         *fly.Client
    Cloudflare  *cloudflare.Client
    Neon        *neon.Client
    Upstash     *upstash.Client
    Resend      *resend.Client
    BetterStack *betterstack.Client
    Doppler     *doppler.Client
    MoneyDevKit *moneydevkit.Client
    Anthropic   *anthropic.Client
    Fal         *fal.Client
}

// Example: Provision complete infrastructure for a project
func (t *Toolbelt) ProvisionProject(ctx context.Context, spec ProjectSpec) error {
    // 1. Create GitHub repo
    repo, _ := t.GitHub.CreateRepo(spec.Name)
    
    // 2. Create Neon database
    db, _ := t.Neon.CreateDatabase(spec.Name)
    
    // 3. Create Upstash Redis
    redis, _ := t.Upstash.CreateRedis(spec.Name)
    
    // 4. Setup Doppler project and sync secrets
    t.Doppler.CreateProject(spec.Name)
    t.Doppler.SetSecrets(spec.Name, map[string]string{
        "DATABASE_URL": db.ConnectionString,
        "REDIS_URL":    redis.URL,
    })
    
    // 5. Create Fly.io app
    app, _ := t.Fly.CreateApp(spec.Name)
    t.Fly.SetSecrets(app.Name, t.Doppler.GetSecrets(spec.Name))
    
    // 6. Setup Cloudflare DNS
    t.Cloudflare.AddDNSRecord(spec.Domain, app.Hostname)
    
    // 7. Setup Better Stack monitoring
    t.BetterStack.CreateMonitor(spec.Name, spec.Domain)
    
    return nil
}
```

---

## How Poindexter Uses the Toolbelt

### Suggesting Tools

When you describe a project, Poindexter suggests which tools to use:

```
┌─────────────────────────────────────────────────────────────────┐
│  You: "Build me a SaaS app for tracking fitness goals"         │
│                                                                 │
│  Poindexter: "Got it! Here's what I'm thinking:                │
│                                                                 │
│    📦 Code: GitHub (repo + CI/CD)                              │
│    🚀 Backend: Fly.io (Go API)                                 │
│    🌐 Frontend: Cloudflare Pages (React)                       │
│    🗄️ Database: Neon (PostgreSQL)                              │
│    ⚡ Cache: Upstash Redis                                      │
│    🔐 Auth: better-auth (self-hosted)                          │
│    📧 Email: Resend (welcome emails, notifications)            │
│    📊 Monitoring: Better Stack                                  │
│    🔑 Secrets: Doppler                                         │
│                                                                 │
│    Payments and AI aren't needed yet, but I can add them       │
│    later if you want subscriptions or AI features.             │
│                                                                 │
│    Sound good?"                                                 │
│                                                                 │
│  [Approve]  [Modify]  [Tell me more]                           │
└─────────────────────────────────────────────────────────────────┘
```

### Or You Specify

```
You: "Build a landing page with waitlist. Use Cloudflare Pages, 
      Neon for the waitlist DB, and Resend for confirmation emails."

Poindexter: "Perfect, I'll use exactly those. Let me get started..."
```

---

## Core Concepts

### 1. Git Worktrees (Task Isolation)

Every task runs in its own worktree. No stepping on each other's toes.

```bash
# Main repos (NEVER touched directly by Dex)
~/src/project-alpha/

# Task worktrees (isolated sandboxes)
~/src/worktrees/project-alpha-task-a1b2/   # Task A's playground
~/src/worktrees/project-alpha-task-c3d4/   # Task B's playground

# Create worktree for new task
git worktree add ~/src/worktrees/project-alpha-task-a1b2 -b task/task-a1b2 main

# Cleanup after merge (user-initiated)
git worktree remove ~/src/worktrees/project-alpha-task-a1b2
```

### 2. Task Lifecycle

```
User: "Add OAuth with Google and GitHub"
                    │
                    ▼
            ┌──────────────┐
            │   PLANNER    │  Decomposes into subtask graph
            │     Hat      │
            └──────┬───────┘
                   │
    ┌──────────────┼──────────────┐
    ▼              ▼              ▼
┌────────┐   ┌────────────┐   ┌────────┐
│task-a1 │──▶│  task-b2   │──▶│task-c3 │
│Architect│   │Implementer │   │Reviewer│
└────────┘   └────────────┘   └────────┘
    │              │              │
    ▼              ▼              ▼
[Design]     [Code+Tests]    [Review]
    │              │              │
    └──────────────┴──────────────┘
                   │
                   ▼
              Create PR
                   │
                   ▼
            Merge (based on autonomy)
```

### 3. Hats (Specialized AI Roles)

| Hat | Purpose | Transitions To |
|-----|---------|----------------|
| 🎯 Planner | Decompose tasks into subtask graph | (spawns others) |
| 🏗️ Architect | Design, interfaces, structure | Implementer |
| ⚙️ Implementer | Write code and tests | Reviewer |
| 🔍 Reviewer | Code review, quality checks | Implementer or Complete |
| 🧪 Tester | Integration tests, coverage | Reviewer |
| 🐛 Debugger | Fix bugs, analyze failures | Reviewer |
| 📚 Documenter | Docs, comments, READMEs | Reviewer |
| 🚀 DevOps | CI/CD, deployment, infrastructure | Reviewer |
| ⚔️ Conflict Manager | Resolve merge conflicts | User Approval |

### 4. Autonomy Levels

| Level | Name | Behavior |
|-------|------|----------|
| 0 | SUPERVISED | Approve every commit, hat transition, PR, merge |
| 1 | SEMI-AUTO | Auto-commit; approve hat transitions, PR, merge |
| 2 | AUTONOMOUS | Auto-commit, auto-transitions, auto-PR; approve merge |
| 3 | FULL-AUTO | Everything automatic; auto-merge if CI passes |

### 5. GitHub Sync (Bi-directional)

```
Dex Action                │  GitHub Result
──────────────────────────┼─────────────────────────
Task created              │  Issue created (label: dex:task)
Task started              │  Issue labeled dex:in-progress
Hat assigned              │  Issue labeled dex:hat:{name}
Task completed            │  PR created, Issue closed
Task quarantined          │  Issue labeled dex:quarantined

GitHub Action             │  Dex Result
──────────────────────────┼─────────────────────────
Issue created (no label)  │  Task created (label: dex:external)
Issue edited              │  Task updated
Issue closed              │  Task marked complete
```

### 6. Conflict Resolution

```
1. PR B has conflicts (PR A merged first)
2. Dex detects via GitHub webhook
3. Creates conflict worktree: project-task-b-conflict
4. Spawns Conflict Manager hat
5. Resolves conflicts, commits
6. USER APPROVAL REQUIRED (all autonomy levels)
7. Original worktree updated
8. PR B ready for merge
```

### 7. Resource Budgets ("Are You Still Watching?")

```
┌─────────────────────────────────────────┐
│  ⏸️  Task Paused                        │
│                                         │
│  Token budget exceeded (150%)           │
│  Time: 45min / 30min budget             │
│  Cost: $4.50 / $3.00 budget             │
│                                         │
│  Progress: ~70% complete                │
│                                         │
│  [Continue +50%]  [Pause]  [Stop]       │
└─────────────────────────────────────────┘
```

---

## Data Models

### Task

```go
type Task struct {
    ID                string    // "task-a1b2c3d4"
    ProjectID         string
    GitHubIssueNumber int
    
    Title             string
    Description       string
    
    ParentID          *string   // Epic parent
    Type              string    // epic, feature, bug, task, chore
    Hat               string    // Current hat
    Priority          int       // 1-5 (1 highest)
    AutonomyLevel     int       // 0-3
    
    Status            string    // pending, blocked, ready, running, etc.
    
    BaseBranch        string
    WorktreePath      *string
    BranchName        *string
    PRNumber          *int
    
    TokenBudget       *int
    TokenUsed         int
    TimeBudgetMin     *int
    TimeUsedMin       int
    DollarBudget      *float64
    DollarUsed        float64
    
    BlockedBy         []string  // Task IDs
    Blocks            []string  // Task IDs
    
    CreatedAt         time.Time
    StartedAt         *time.Time
    CompletedAt       *time.Time
}
```

### Project

```go
type Project struct {
    ID            string
    Name          string
    RepoPath      string    // Local path: ~/src/project-alpha
    GitHubOwner   string
    GitHubRepo    string
    DefaultBranch string
    
    // Toolbelt services used by this project
    Services      ProjectServices
    
    CreatedAt     time.Time
}

type ProjectServices struct {
    // Which services this project uses (provisioned by Dex)
    FlyApp          *string  // Fly.io app name
    NeonProject     *string  // Neon project ID
    NeonDatabase    *string  // Neon database name
    UpstashRedis    *string  // Upstash Redis database ID
    CloudflareDomain *string // Cloudflare managed domain
    DopplerProject  *string  // Doppler project name
    BetterStackMonitor *string // Better Stack monitor ID
    ResendDomain    *string  // Resend verified domain
}
```

### Session

```go
type Session struct {
    ID                string
    TaskID            string
    Hat               string
    
    ClaudeSessionID   *string   // For resume
    Status            string
    WorktreePath      string
    
    IterationCount    int
    MaxIterations     int
    CompletionPromise *string
    
    TokensUsed        int
    TokensBudget      *int
    DollarsUsed       float64
    DollarsBudget     *float64
    
    Checkpoints       []Checkpoint
    
    CreatedAt         time.Time
    StartedAt         *time.Time
    EndedAt           *time.Time
    Outcome           *string
}
```

---

## Directory Structure

```
poindexter/
├── cmd/
│   ├── dex/
│   │   └── main.go              # Single binary (API + Orchestrator)
│   └── dex-cli/
│       └── main.go              # CLI tool
├── internal/
│   ├── api/
│   │   ├── handlers/            # HTTP handlers
│   │   ├── middleware/          # Auth, logging
│   │   └── websocket/           # Real-time updates
│   ├── auth/
│   │   ├── bip39.go             # Passphrase generation
│   │   ├── ed25519.go           # Key derivation
│   │   └── jwt.go               # Token management
│   ├── db/
│   │   ├── sqlite.go            # SQLite connection
│   │   ├── migrations/          # SQL migrations
│   │   └── models/              # DB models
│   ├── git/
│   │   ├── worktree.go          # Worktree management
│   │   ├── operations.go        # Git commands
│   │   └── conflict.go          # Conflict detection
│   ├── github/
│   │   ├── client.go            # API client
│   │   ├── sync.go              # Bi-directional sync
│   │   └── webhooks.go          # Webhook handlers
│   ├── orchestrator/
│   │   ├── scheduler.go         # Priority queue
│   │   ├── hats.go              # Hat management
│   │   ├── transitions.go       # Hat transitions
│   │   └── monitor.go           # Resource monitoring
│   ├── session/
│   │   ├── manager.go           # Session lifecycle
│   │   ├── ralph.go             # Ralph loop
│   │   ├── checkpoint.go        # Checkpointing
│   │   └── sdk.go               # Claude SDK wrapper
│   ├── task/
│   │   ├── service.go           # Task CRUD
│   │   ├── parser.go            # NL parsing
│   │   └── graph.go             # Dependency graph
│   └── toolbelt/
│       ├── toolbelt.go          # Main toolbelt struct
│       ├── github.go            # GitHub client wrapper
│       ├── fly.go               # Fly.io client
│       ├── cloudflare.go        # Cloudflare client
│       ├── neon.go              # Neon client
│       ├── upstash.go           # Upstash client
│       ├── resend.go            # Resend client
│       ├── betterstack.go       # Better Stack client
│       ├── doppler.go           # Doppler client
│       ├── moneydevkit.go       # MoneyDevKit client
│       ├── anthropic.go         # Anthropic client
│       └── fal.go               # fal.ai client
├── prompts/
│   └── hats/
│       ├── planner.md
│       ├── architect.md
│       ├── implementer.md
│       ├── reviewer.md
│       ├── tester.md
│       ├── debugger.md
│       ├── documenter.md
│       ├── devops.md
│       └── conflict_manager.md
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── hooks/
│   │   ├── stores/              # Zustand
│   │   └── api/                 # TanStack Query
│   ├── package.json
│   └── tailwind.config.js
├── deploy/
│   ├── systemd/
│   │   └── dex.service          # Single service
│   └── install.sh
├── scripts/
│   ├── backup.sh
│   └── setup-tailscale.sh
├── go.mod
├── go.sum
└── README.md
```

---

## API Endpoints

```
# Auth
POST   /api/v1/auth/challenge
POST   /api/v1/auth/verify
POST   /api/v1/auth/refresh

# Projects
GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/:id
DELETE /api/v1/projects/:id
POST   /api/v1/projects/:id/sync
POST   /api/v1/projects/:id/provision    # Provision toolbelt services

# Tasks
GET    /api/v1/tasks
POST   /api/v1/tasks                      # Natural language input
GET    /api/v1/tasks/:id
PUT    /api/v1/tasks/:id
POST   /api/v1/tasks/:id/start
POST   /api/v1/tasks/:id/pause
POST   /api/v1/tasks/:id/resume
POST   /api/v1/tasks/:id/cancel
GET    /api/v1/tasks/:id/logs

# Sessions
GET    /api/v1/sessions
GET    /api/v1/sessions/:id
POST   /api/v1/sessions/:id/kill

# Worktrees
GET    /api/v1/worktrees
DELETE /api/v1/worktrees/:path

# Approvals
GET    /api/v1/approvals
POST   /api/v1/approvals/:id/approve
POST   /api/v1/approvals/:id/reject

# Toolbelt
GET    /api/v1/toolbelt/status            # Which services are configured
POST   /api/v1/toolbelt/test              # Test all connections

# System
GET    /api/v1/system/status
GET    /api/v1/system/metrics

# WebSocket
WS     /api/v1/ws
```

---

## Phase 1: Foundation

### Checkpoint 1.1: Project Setup
- [ ] Initialize Go module: `go mod init github.com/you/poindexter`
- [ ] Create directory structure as shown above
- [ ] Setup Bun + React frontend: `cd frontend && bun create vite . --template react-ts`
- [ ] Add Tailwind: `bun add -d tailwindcss postcss autoprefixer`
- [ ] Create basic config.yaml and toolbelt.yaml
- [ ] Setup .gitignore

### Checkpoint 1.2: Tailscale HTTPS (question! Can tailscale https be set up through code/library without installing tailscale?)
- [ ] Install Tailscale on host
- [ ] Configure with hostname: `sudo tailscale up --hostname=dex`
- [ ] Generate HTTPS cert: `sudo tailscale cert dex.{tailnet}.ts.net`
- [ ] Store cert paths in config
- [ ] Test HTTPS access from phone

### Checkpoint 1.3: BIP39 Authentication
- [ ] Implement `internal/auth/bip39.go`:
  - GeneratePassphrase() (24 words)
  - GenerateRecoveryPhrase() (12 words)
  - ValidateMnemonic()
- [ ] Implement `internal/auth/ed25519.go`:
  - DeriveKeypair(passphrase) → (pubkey, privkey)
  - Sign(message, privkey) → signature
  - Verify(message, signature, pubkey) → bool
- [ ] Implement `internal/auth/jwt.go`:
  - GenerateToken(userID) → JWT
  - ValidateToken(token) → claims
  - RefreshToken(token) → newJWT

### Checkpoint 1.4: SQLite Database
- [ ] Create `internal/db/sqlite.go` with modernc.org/sqlite (pure Go)
- [ ] Create schema migrations:
  - users table
  - projects table (with services JSON)
  - tasks table
  - task_dependencies table
  - sessions table
  - session_checkpoints table
  - approvals table
- [ ] Run migrations on startup
- [ ] Test basic CRUD

### Checkpoint 1.5: Basic API
- [ ] Setup Echo server with HTTPS (Tailscale certs)
- [ ] Implement auth middleware
- [ ] Implement health check: `GET /api/v1/system/status`
- [ ] Serve static frontend files
- [ ] Test from mobile via Tailscale

**Phase 1 Complete When:**
- [ ] `https://dex.{tailnet}.ts.net` loads in browser
- [ ] Can generate passphrase and authenticate
- [ ] SQLite database working
- [ ] All running from single binary

---

## Phase 2: Toolbelt Clients

### Checkpoint 2.1: Toolbelt Configuration
- [ ] Load toolbelt.yaml on startup
- [ ] Validate required credentials
- [ ] `GET /api/v1/toolbelt/status` returns configured services

### Checkpoint 2.2: GitHub Client
- [ ] Implement `internal/toolbelt/github.go`:
  - CreateRepo, ListRepos
  - CreateIssue, UpdateIssue, CloseIssue
  - CreatePR, MergePR
  - SetupActions (push workflow file)

### Checkpoint 2.3: Fly.io Client
- [ ] Implement `internal/toolbelt/fly.go`:
  - CreateApp, DeleteApp
  - Deploy (from Dockerfile or image)
  - SetSecrets
  - GetStatus, GetLogs
  - Scale

### Checkpoint 2.4: Cloudflare Client
- [ ] Implement `internal/toolbelt/cloudflare.go`:
  - AddDNSRecord, UpdateDNSRecord, DeleteDNSRecord
  - CreateR2Bucket, UploadToR2
  - CreateKVNamespace, KVPut, KVGet
  - CreatePagesProject, DeployPages

### Checkpoint 2.5: Neon Client
- [ ] Implement `internal/toolbelt/neon.go`:
  - CreateProject, DeleteProject
  - CreateDatabase, DeleteDatabase
  - CreateBranch (for testing/staging)
  - GetConnectionString

### Checkpoint 2.6: Upstash Client
- [ ] Implement `internal/toolbelt/upstash.go`:
  - CreateRedis, DeleteRedis
  - GetCredentials
  - CreateQStash queue

### Checkpoint 2.7: Remaining Clients
- [ ] Implement `internal/toolbelt/resend.go`:
  - SendEmail, VerifyDomain
- [ ] Implement `internal/toolbelt/betterstack.go`:
  - CreateMonitor, CreateLogSource
- [ ] Implement `internal/toolbelt/doppler.go`:
  - CreateProject, SetSecrets, GetSecrets, SyncToFly
- [ ] Implement `internal/toolbelt/moneydevkit.go`:
  - CreateProduct, CreatePrice, CreateCheckoutLink
- [ ] Implement `internal/toolbelt/anthropic.go`:
  - Chat, Complete (for project's AI features, not Dex's brain)
- [ ] Implement `internal/toolbelt/fal.go`:
  - GenerateImage, GenerateVideo

### Checkpoint 2.8: Test All Connections
- [ ] `POST /api/v1/toolbelt/test` tests each configured service
- [ ] Returns status for each

**Phase 2 Complete When:**
- [ ] All toolbelt clients implemented
- [ ] Can test all connections from UI
- [ ] Can provision a simple project (repo + database + deploy)

---

## Phase 3: Core Task System

### Checkpoint 3.1: Task CRUD
- [ ] Implement `internal/task/service.go`:
  - Create, Read, Update, Delete
  - List with filters (status, project, priority)
- [ ] Implement API handlers
- [ ] Test via curl/Postman

### Checkpoint 3.2: Natural Language Parsing
- [ ] Implement `internal/task/parser.go`:
  - Parse user input with Claude
  - Extract: title, description, type, priority
  - Suggest autonomy level
  - Suggest toolbelt services needed
- [ ] Create `/api/v1/tasks` POST that:
  - Accepts natural language
  - Returns suggested structure + services
  - Confirms with user
  - Creates task

### Checkpoint 3.3: Dependency Graph
- [ ] Implement `internal/task/graph.go`:
  - AddDependency(blocker, blocked)
  - RemoveDependency()
  - GetBlockers(taskID) → []Task
  - GetBlocked(taskID) → []Task
  - IsReady(taskID) → bool
  - GetReadyTasks() → []Task

### Checkpoint 3.4: Task State Machine
- [ ] Implement status transitions with validation
- [ ] Emit events on transition (for WebSocket)

### Checkpoint 3.5: Priority Queue Scheduler
- [ ] Implement `internal/orchestrator/scheduler.go`:
  - In-memory priority queue (heap)
  - Max 25 parallel
  - Preemption for high-priority tasks

**Phase 3 Complete When:**
- [ ] Can create task via natural language from mobile
- [ ] Tasks show correct status based on dependencies
- [ ] Priority queue correctly orders tasks

---

## Phase 4: Git Worktree Management

### Checkpoint 4.1: Worktree Operations
- [ ] Implement `internal/git/worktree.go`:
  - Create(project, taskID, baseBranch) → path
  - Remove(path)
  - List() → []Worktree
  - GetStatus(path) → GitStatus

### Checkpoint 4.2: Git Operations
- [ ] Implement `internal/git/operations.go`:
  - Commit, Push, Pull
  - GetCurrentBranch, GetDiff

### Checkpoint 4.3: Integration
- [ ] On task start: create worktree
- [ ] On task complete: keep worktree (user deletes later)
- [ ] API endpoint to delete worktree

**Phase 4 Complete When:**
- [ ] Starting a task creates isolated worktree
- [ ] Can manage worktrees via API

---

## Phase 5: Session Management

### Checkpoint 5.1: Claude Agent SDK Integration
- [ ] Create TypeScript wrapper called from Go
- [ ] Start, Resume, Stop sessions

### Checkpoint 5.2: Hat Prompt Loading
- [ ] Load and template hat prompts
- [ ] Include toolbelt context in prompts

### Checkpoint 5.3: Ralph Loop
- [ ] Implement iteration loop
- [ ] Completion detection
- [ ] Budget checking
- [ ] Failure detection

### Checkpoint 5.4: Checkpointing
- [ ] Checkpoint every 5 iterations
- [ ] Resume from checkpoint

### Checkpoint 5.5: Hat Transitions
- [ ] Detect completion, transition to next hat
- [ ] Respect autonomy levels

**Phase 5 Complete When:**
- [ ] Sessions run with Ralph loop
- [ ] Hat transitions work
- [ ] Can view logs in real-time

---

## Phase 6: GitHub Integration

### Checkpoint 6.1: Bi-directional Sync
- [ ] Sync tasks ↔ issues
- [ ] Handle webhooks

### Checkpoint 6.2: PR Workflow
- [ ] Create PR on completion
- [ ] Auto-merge based on autonomy

### Checkpoint 6.3: Conflict Resolution
- [ ] Detect conflicts
- [ ] Spawn Conflict Manager
- [ ] User approval flow

**Phase 6 Complete When:**
- [ ] Issues sync both ways
- [ ] PRs created and merged
- [ ] Conflicts resolved with approval

---

## Phase 7: Mobile-First Frontend

### Checkpoint 7.1: Core Setup
- [ ] Tailwind, TanStack Query, Zustand, Socket.io
- [ ] Auth flow

### Checkpoint 7.2: Dashboard
- [ ] System status
- [ ] Needs attention
- [ ] Active tasks
- [ ] Quick task creation

### Checkpoint 7.3: Task Management
- [ ] List, detail, creation flow
- [ ] Toolbelt service selection

### Checkpoint 7.4: Toolbelt UI
- [ ] View configured services
- [ ] Test connections
- [ ] View project infrastructure

### Checkpoint 7.5: Approvals
- [ ] Pending list
- [ ] Review and approve/reject

**Phase 7 Complete When:**
- [ ] Everything works from phone
- [ ] Real-time updates
- [ ] Can provision and deploy projects

---

## Completion Promise

This task is **COMPLETE** when:

1. **All 7 phases have green checkpoints**

2. **Self-contained operation:**
   - Poindexter runs from single binary
   - SQLite database, no external dependencies
   - Survives machine reboot

3. **Toolbelt works end-to-end:**
   - User says: "Build me a SaaS for tracking habits"
   - Poindexter suggests services (Fly, Neon, etc.)
   - User approves
   - Poindexter provisions infrastructure
   - Poindexter builds the app
   - Poindexter deploys to Fly.io
   - App is live at custom domain

4. **Mobile UX is smooth:**
   - Dashboard loads in <2s
   - Real-time updates work
   - Can manage everything from phone

5. **Autonomy levels work:**
   - Level 0: Approve every step
   - Level 3: Runs to completion unattended

---

## Anti-Patterns to Avoid

- ❌ Don't use external services for Dex's own state — SQLite only
- ❌ Don't modify main repos directly — always use worktrees
- ❌ Don't store private keys — derive from passphrase
- ❌ Don't auto-merge conflicts — always require approval
- ❌ Don't ignore budget limits — always pause and ask
- ❌ Don't expose to public internet — Tailscale only
- ❌ Don't hardcode API keys — use toolbelt.yaml

---

## Quick Reference

```bash
# Start Poindexter
sudo systemctl start dex

# View logs
journalctl -u dex -f

# Access
https://dex.{tailnet}.ts.net

# CLI
dex status
dex task new "Build a habit tracker SaaS"
dex toolbelt test
dex project provision my-saas
```

---

## Notes for Claude

- **Poindexter is self-contained**: SQLite, in-memory queues, local filesystem
- **Toolbelt is for YOUR projects**: Provision infrastructure, deploy apps
- **Be methodical**: Complete each checkpoint before moving on
- **Test on mobile**: After each UI change, test on phone
- **Commit often**: Atomic commits with clear messages

You are Poindexter's creator. Build your nerdy genius well. 🤓
