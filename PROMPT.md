# POINDEXTER (dex) — Your Nerdy AI Orchestration Genius

> **Disk is state. Git is memory. Fresh context each iteration.**

## Completion Promise

This task is **COMPLETE** when all of these are true:

```
[ ] go build ./cmd/dex && go test ./... → PASS
[ ] Frontend builds: cd frontend && bun run build → PASS
[ ] Can authenticate via BIP39 passphrase from mobile
[ ] Can create a task via API and it appears in UI
[ ] Can start a task and see a session running Ralph loop
[ ] Session completes and creates a PR on GitHub
[ ] Real-time updates flow via WebSocket
```

**Evidence required before declaring LOOP_COMPLETE:**
- `go test ./...` passes
- `go build ./cmd/dex` succeeds
- Frontend loads at configured URL
- At least one end-to-end task execution works

---

## Mission

Build **Poindexter** — a self-contained, single-user system for orchestrating 25+ concurrent Claude Code sessions on a local machine. Poindexter manages your AI workforce: decomposing tasks, assigning specialized "hats" to sessions, managing git worktrees for isolation, and building complete, deployed applications using a curated toolbelt of cloud services.

**Nickname:** `dex`

```
"I've got 8 sessions running, 3 in queue. Project Alpha just deployed
 to Fly.io — want me to set up the custom domain on Cloudflare?"
                                                    — Poindexter
```

---

## Backpressure Gates

Before any phase can be marked complete:

| Gate | Command | Must Pass |
|------|---------|-----------|
| Build | `go build ./cmd/dex` | No errors |
| Tests | `go test ./...` | All pass |
| Lint | `golangci-lint run` | No errors |
| Frontend | `cd frontend && bun run build` | No errors |

---

## Guardrails

- Never use external services for Dex's own state — SQLite only
- Never modify main repos directly — always use worktrees
- Never store private keys — derive from passphrase
- Never auto-merge conflicts — always require user approval
- Never ignore budget limits — always pause and ask
- Never expose to public internet — Tailscale only
- Never hardcode API keys — use toolbelt.yaml

---

## Current State Summary

**What EXISTS and WORKS:**
- Go module initialized with all dependencies
- SQLite database with 7 migrations (users, projects, tasks, dependencies, sessions, checkpoints, approvals)
- BIP39 + Ed25519 + JWT authentication system
- All 11 toolbelt clients (GitHub, Fly, Cloudflare, Neon, Upstash, Resend, BetterStack, Doppler, MoneyDevKit, Anthropic, fal.ai)
- Task CRUD with state machine and dependency graph
- Git worktree management (create, remove, list, status)
- Priority queue scheduler (heap-based, max 25 parallel)
- Echo API server with 13 endpoints
- Session manager structure with budget tracking
- 9 hat prompt templates

**What is MISSING:**
- Auth API endpoints (challenge/verify/refresh flow)
- Project CRUD endpoints
- Ralph loop (Claude SDK integration for actual task execution)
- Hat transitions (logic to move between hats)
- WebSocket server for real-time updates
- GitHub bi-directional sync (task ↔ issue)
- Frontend UI (only placeholder exists)

---

## Architecture

| Component | Technology | Status |
|-----------|------------|--------|
| Database | SQLite (single file) | DONE |
| Cache/Queue | In-memory (Go channels) | DONE |
| State | Local filesystem | DONE |
| API | Go + Echo | PARTIAL |
| Frontend | React + Bun + Tailwind | SCAFFOLD ONLY |
| Access | Tailscale HTTPS | CONFIG READY |
| Auth | BIP39 → Ed25519 → JWT | CORE DONE, API MISSING |

---

## Phase 1: Foundation [~80% COMPLETE]

### Checkpoint 1.1: Project Setup
- [x] Initialize Go module
- [x] Create directory structure
- [x] Setup Bun + React frontend scaffold
- [x] Add Tailwind configuration
- [x] Create config.yaml and toolbelt.yaml examples
- [x] Setup .gitignore

### Checkpoint 1.2: Tailscale HTTPS
- [x] Server supports TLS configuration
- [ ] Document Tailscale cert setup (manual step)
- [ ] Test HTTPS access from mobile

### Checkpoint 1.3: BIP39 Authentication
- [x] `internal/auth/bip39.go`: GeneratePassphrase(), GenerateRecoveryPhrase(), ValidateMnemonic()
- [x] `internal/auth/ed25519.go`: DeriveKeypair(), Sign(), Verify()
- [x] `internal/auth/jwt.go`: GenerateToken(), ValidateToken(), RefreshToken()

### Checkpoint 1.4: SQLite Database
- [x] `internal/db/sqlite.go` with modernc.org/sqlite (pure Go, WAL mode)
- [x] 7 schema migrations: users, projects, tasks, task_dependencies, sessions, session_checkpoints, approvals
- [x] CRUD operations for all tables
- [x] Foreign keys enabled

### Checkpoint 1.5: Basic API
- [x] Echo server with TLS support
- [x] JWT auth middleware
- [x] Health check: `GET /api/v1/system/status`
- [x] Static frontend serving
- [ ] **MISSING: Auth endpoints** — Need to implement:
  - `POST /api/v1/auth/challenge` — Return random challenge
  - `POST /api/v1/auth/verify` — Verify signature, return JWT
  - `POST /api/v1/auth/refresh` — Refresh JWT token

**Phase 1 DONE when:**
```
[ ] Auth endpoints work (challenge → sign → verify → JWT)
[ ] Can authenticate from mobile browser
[ ] go test ./internal/auth/... passes
```

---

## Phase 2: Toolbelt Clients [100% COMPLETE]

### Checkpoint 2.1: Toolbelt Configuration
- [x] Load toolbelt.yaml on startup
- [x] Config validation
- [x] `GET /api/v1/toolbelt/status` returns configured services

### Checkpoint 2.2-2.7: All Clients Implemented
- [x] GitHub: CreateRepo, ListRepos, CreateIssue, UpdateIssue, CloseIssue, CreatePR, MergePR
- [x] Fly.io: CreateApp, DeleteApp, Deploy, SetSecrets, GetStatus, GetLogs, Scale
- [x] Cloudflare: DNS records, R2 buckets, KV namespace, Pages projects
- [x] Neon: CreateProject, CreateDatabase, CreateBranch, GetConnectionString
- [x] Upstash: CreateRedis, DeleteRedis, GetCredentials, CreateQStash
- [x] Resend: SendEmail, VerifyDomain
- [x] BetterStack: CreateMonitor, CreateLogSource
- [x] Doppler: CreateProject, SetSecrets, GetSecrets, SyncSecrets
- [x] MoneyDevKit: CreateProduct, CreatePrice, CreateCheckoutLink
- [x] Anthropic: Chat, Complete
- [x] fal.ai: GenerateImage, GenerateVideo

### Checkpoint 2.8: Test All Connections
- [x] `POST /api/v1/toolbelt/test` tests each configured service
- [x] Returns status with latency for each

**Phase 2 DONE:** All toolbelt clients implemented with Ping() methods.

---

## Phase 3: Core Task System [~85% COMPLETE]

### Checkpoint 3.1: Task CRUD
- [x] `internal/task/service.go`: Create, Read, Update, Delete, List with filters
- [x] API handlers: GET/POST/PUT/DELETE `/api/v1/tasks`
- [x] Status filtering, project filtering

### Checkpoint 3.2: Natural Language Parsing
- [ ] **MISSING: `internal/task/parser.go`** — Need to implement:
  - Parse natural language input with Claude API
  - Extract: title, description, type, priority
  - Suggest autonomy level based on task complexity
  - Suggest toolbelt services needed
  - Return structured task proposal for user confirmation

### Checkpoint 3.3: Dependency Graph
- [x] `internal/task/graph.go`: AddDependency, RemoveDependency, GetBlockers, GetBlocked, IsReady
- [x] Cycle detection (wouldCreateCycle)
- [x] GetReadyTasks() returns unblocked tasks

### Checkpoint 3.4: Task State Machine
- [x] `internal/task/state.go`: Valid transitions defined
- [x] CanTransition(), Transition() with validation
- [ ] **MISSING: Event emission** — Need WebSocket to emit state change events

### Checkpoint 3.5: Priority Queue Scheduler
- [x] `internal/orchestrator/scheduler.go`: Heap-based priority queue
- [x] Max 25 parallel tasks (configurable)
- [x] Enqueue, Dequeue, Pause, Resume

**Phase 3 DONE when:**
```
[ ] POST /api/v1/tasks accepts natural language, returns structured proposal
[ ] User can confirm/modify proposal before task creation
[ ] State transitions emit events (requires WebSocket from Phase 7)
```

---

## Phase 4: Git Worktree Management [~90% COMPLETE]

### Checkpoint 4.1: Worktree Operations
- [x] `internal/git/worktree.go`: Create, Remove, List
- [x] Creates worktrees at `~/src/worktrees/{project}-{taskID}/`
- [x] Branch naming: `task/{taskID}`

### Checkpoint 4.2: Git Operations
- [x] `internal/git/operations.go`: Commit, Push, Pull, GetCurrentBranch, GetDiff, GetStatus

### Checkpoint 4.3: Integration
- [x] `POST /api/v1/tasks/:id/start` creates worktree
- [x] `GET /api/v1/tasks/:id/worktree/status` returns git status
- [x] `DELETE /api/v1/worktrees/:task_id` removes worktree
- [ ] **MISSING: Conflict detection** — Need to detect merge conflicts when PR created

**Phase 4 DONE when:**
```
[ ] Conflict detection implemented
[ ] go test ./internal/git/... passes with conflict scenarios
```

---

## Phase 5: Session Management [~40% COMPLETE — CRITICAL GAP]

This is the **core orchestration layer** that makes Poindexter actually work.

### Checkpoint 5.1: Claude SDK Integration
- [ ] **MISSING: `internal/session/sdk.go`** — Need to implement:
  - Wrapper around Claude Agent SDK (TypeScript) or direct API
  - StartSession(worktreePath, hatPrompt) → sessionHandle
  - SendMessage(sessionHandle, message) → response
  - StopSession(sessionHandle)
  - GetSessionState(sessionHandle) → (running/stopped/error)

### Checkpoint 5.2: Hat Prompt Loading
- [x] `internal/session/prompts.go`: LoadAll(), GetPrompt(hat)
- [x] 9 hat templates exist in `prompts/hats/`
- [ ] **MISSING: Template variable injection** — Need to inject:
  - Task context (title, description, dependencies)
  - Worktree path
  - Toolbelt available services
  - Completion criteria

### Checkpoint 5.3: Ralph Loop — THE CORE
- [ ] **MISSING: `internal/session/ralph.go`** — This is the heart of Poindexter:

```go
// The Ralph loop: iterate until complete or budget exceeded
func (m *Manager) RunRalphLoop(ctx context.Context, session *Session) error {
    for {
        // 1. Check budget (tokens, time, dollars)
        if session.ExceedsBudget() {
            return m.PauseForApproval(session, "budget_exceeded")
        }

        // 2. Load fresh context (disk is state)
        prompt := m.buildPrompt(session)

        // 3. Send to Claude
        response, err := m.sdk.SendMessage(session.Handle, prompt)
        if err != nil {
            return m.handleError(session, err)
        }

        // 4. Check for completion signal
        if m.detectCompletion(response) {
            return m.completeSession(session)
        }

        // 5. Check for hat transition
        if nextHat := m.detectHatTransition(response); nextHat != "" {
            return m.transitionHat(session, nextHat)
        }

        // 6. Increment iteration, checkpoint if needed
        session.IterationCount++
        if session.IterationCount % 5 == 0 {
            m.checkpoint(session)
        }

        // 7. Check iteration limit
        if session.IterationCount >= session.MaxIterations {
            return m.PauseForApproval(session, "iteration_limit")
        }
    }
}
```

**Completion Detection:**
- Look for `TASK_COMPLETE` or `HAT_COMPLETE` in response
- Verify backpressure gates pass (tests, lint, build)
- Only then transition or complete

### Checkpoint 5.4: Checkpointing
- [x] Database table exists (session_checkpoints)
- [ ] **MISSING: Checkpoint creation/restore logic**
  - Save: iteration count, Claude session ID, last response
  - Restore: Resume from checkpoint on restart

### Checkpoint 5.5: Hat Transitions
- [ ] **MISSING: `internal/orchestrator/transitions.go`** — Need to implement:
  - Planner → spawns child tasks with appropriate hats
  - Architect → Implementer (when design complete)
  - Implementer → Reviewer (when code + tests written)
  - Reviewer → Implementer (if changes requested) OR Complete
  - Respect autonomy levels for approvals

**Phase 5 DONE when:**
```
[ ] Can start a session and watch it iterate
[ ] Session detects completion and stops
[ ] Hat transitions work (Implementer → Reviewer → Complete)
[ ] Budget limits pause and ask for approval
[ ] go test ./internal/session/... passes
```

---

## Phase 6: GitHub Integration [~30% COMPLETE]

### Checkpoint 6.1: Bi-directional Sync
- [x] GitHub client exists with issue/PR operations
- [ ] **MISSING: `internal/github/sync.go`** — Need to implement:
  - On task create → Create GitHub issue (label: `dex:task`)
  - On task start → Add label `dex:in-progress`
  - On task complete → Create PR, close issue
  - On GitHub issue create → Create Dex task (label: `dex:external`)
  - On GitHub issue close → Mark Dex task complete

### Checkpoint 6.2: Webhook Handler
- [ ] **MISSING: `internal/github/webhooks.go`** — Need to implement:
  - `POST /api/v1/webhooks/github` endpoint
  - Handle: issues.opened, issues.closed, issues.edited
  - Handle: pull_request.opened, pull_request.merged, pull_request.closed
  - Handle: check_suite.completed (CI results)
  - Verify webhook signature

### Checkpoint 6.3: PR Workflow
- [ ] **MISSING: Auto-PR creation** — On session complete:
  - Push branch to origin
  - Create PR with description from task
  - Link to GitHub issue
  - Request review based on autonomy level

### Checkpoint 6.4: Conflict Resolution
- [ ] **MISSING: Conflict Manager flow**:
  - Detect conflict via GitHub API or local git
  - Create conflict worktree
  - Spawn Conflict Manager hat
  - ALWAYS require user approval for conflict resolution

**Phase 6 DONE when:**
```
[ ] Creating a task creates a GitHub issue
[ ] Completing a task creates a PR
[ ] GitHub webhook updates task status
[ ] Conflicts detected and escalated to user
```

---

## Phase 7: Mobile-First Frontend [~5% COMPLETE — NEEDS REBUILD]

The frontend is currently just a placeholder. Needs complete implementation.

### Checkpoint 7.1: Core Setup
- [x] Vite + React + TypeScript scaffold
- [x] Tailwind configured
- [ ] **MISSING: State management** — Add Zustand
- [ ] **MISSING: API client** — Add TanStack Query
- [ ] **MISSING: WebSocket client** — Add Socket.io or native WS

### Checkpoint 7.2: WebSocket Server (Backend)
- [ ] **MISSING: `internal/api/websocket/hub.go`** — Need to implement:
  - WebSocket endpoint: `WS /api/v1/ws`
  - Broadcast events: task.created, task.updated, task.completed
  - Broadcast events: session.started, session.iteration, session.completed
  - Broadcast events: approval.required
  - Client subscription by task/project

### Checkpoint 7.3: Auth Flow (Frontend)
- [ ] **MISSING: Login page**
  - Display passphrase (first time) or input passphrase (returning)
  - Sign challenge with Ed25519
  - Store JWT in localStorage
  - Auto-refresh before expiry

### Checkpoint 7.4: Dashboard
- [ ] **MISSING: Dashboard page**
  - System status (sessions running, queue depth)
  - Needs attention (approvals pending)
  - Active tasks with real-time status
  - Quick task creation input

### Checkpoint 7.5: Task Management
- [ ] **MISSING: Task list page**
  - Filter by status, project
  - Show dependencies
  - Real-time status updates via WebSocket

- [ ] **MISSING: Task detail page**
  - Full task info
  - Session logs (streaming)
  - Worktree status
  - Actions: start, pause, resume, cancel

- [ ] **MISSING: Task creation flow**
  - Natural language input
  - Structured proposal review
  - Toolbelt service selection
  - Confirm and create

### Checkpoint 7.6: Approvals
- [ ] **MISSING: Approvals page**
  - List pending approvals
  - Show context (what triggered approval)
  - Approve/Reject actions
  - Push notifications (if possible)

### Checkpoint 7.7: Toolbelt UI
- [ ] **MISSING: Toolbelt page**
  - Show configured services
  - Test connections button
  - View project infrastructure

**Phase 7 DONE when:**
```
[ ] Can login with passphrase on mobile
[ ] Dashboard shows real-time task status
[ ] Can create task via natural language
[ ] Can approve/reject from phone
[ ] WebSocket delivers instant updates
[ ] bun run build succeeds
```

---

## API Endpoints Status

### Implemented
```
GET    /api/v1/system/status         ✓
GET    /api/v1/toolbelt/status       ✓
POST   /api/v1/toolbelt/test         ✓
GET    /api/v1/tasks                 ✓
POST   /api/v1/tasks                 ✓ (basic, no NL parsing)
GET    /api/v1/tasks/:id             ✓
PUT    /api/v1/tasks/:id             ✓
DELETE /api/v1/tasks/:id             ✓
POST   /api/v1/tasks/:id/start       ✓
GET    /api/v1/tasks/:id/worktree/status  ✓
GET    /api/v1/worktrees             ✓
DELETE /api/v1/worktrees/:task_id    ✓
GET    /api/v1/me                    ✓ (protected)
```

### Missing
```
POST   /api/v1/auth/challenge        ✗ — Return random challenge
POST   /api/v1/auth/verify           ✗ — Verify signature, return JWT
POST   /api/v1/auth/refresh          ✗ — Refresh JWT

GET    /api/v1/projects              ✗
POST   /api/v1/projects              ✗
GET    /api/v1/projects/:id          ✗
DELETE /api/v1/projects/:id          ✗
POST   /api/v1/projects/:id/provision ✗ — Provision toolbelt services

POST   /api/v1/tasks/:id/pause       ✗
POST   /api/v1/tasks/:id/resume      ✗
POST   /api/v1/tasks/:id/cancel      ✗
GET    /api/v1/tasks/:id/logs        ✗ — Stream session logs

GET    /api/v1/sessions              ✗
GET    /api/v1/sessions/:id          ✗
POST   /api/v1/sessions/:id/kill     ✗

GET    /api/v1/approvals             ✗
POST   /api/v1/approvals/:id/approve ✗
POST   /api/v1/approvals/:id/reject  ✗

POST   /api/v1/webhooks/github       ✗ — GitHub webhook handler

WS     /api/v1/ws                    ✗ — WebSocket for real-time
```

---

## File Locations

### Core Go Packages
```
internal/
├── api/
│   ├── server.go          ✓ Echo server, routes
│   └── middleware/auth.go ✓ JWT middleware
├── auth/
│   ├── bip39.go           ✓ Mnemonic generation
│   ├── ed25519.go         ✓ Key derivation
│   └── jwt.go             ✓ Token management
├── db/
│   ├── sqlite.go          ✓ DB connection, migrations
│   ├── models.go          ✓ Type definitions
│   ├── users.go           ✓ User CRUD
│   ├── projects.go        ✓ Project CRUD
│   ├── tasks.go           ✓ Task CRUD
│   ├── sessions.go        ✓ Session CRUD
│   └── approvals.go       ✓ Approval CRUD
├── git/
│   ├── worktree.go        ✓ Worktree management
│   ├── operations.go      ✓ Git commands
│   └── service.go         ✓ Coordinates git + DB
├── github/
│   └── (empty)            ✗ Needs sync.go, webhooks.go
├── orchestrator/
│   └── scheduler.go       ✓ Priority queue
├── session/
│   ├── manager.go         ✓ Session lifecycle (partial)
│   └── prompts.go         ✓ Hat prompt loading
├── task/
│   ├── service.go         ✓ Task business logic
│   ├── state.go           ✓ State machine
│   └── graph.go           ✓ Dependency graph
└── toolbelt/
    ├── toolbelt.go        ✓ Main struct
    ├── config.go          ✓ YAML loading
    ├── github.go          ✓
    ├── fly.go             ✓
    ├── cloudflare.go      ✓
    ├── neon.go            ✓
    ├── upstash.go         ✓
    ├── resend.go          ✓
    ├── betterstack.go     ✓
    ├── doppler.go         ✓
    ├── moneydevkit.go     ✓
    ├── anthropic.go       ✓
    └── fal.go             ✓
```

### Files That Need Creation
```
internal/api/websocket/hub.go       — WebSocket server
internal/api/handlers/auth.go       — Auth endpoints
internal/api/handlers/projects.go   — Project endpoints
internal/api/handlers/approvals.go  — Approval endpoints
internal/github/sync.go             — Bi-directional sync
internal/github/webhooks.go         — Webhook handler
internal/session/sdk.go             — Claude SDK wrapper
internal/session/ralph.go           — Ralph loop
internal/orchestrator/transitions.go — Hat transitions
internal/task/parser.go             — NL parsing
```

---

## Priority Order for Remaining Work

**Critical Path (enables core functionality):**

1. **Auth endpoints** — Can't use the app without login
2. **WebSocket server** — Can't have real-time updates without it
3. **Ralph loop + SDK** — Can't execute tasks without this
4. **Hat transitions** — Can't complete multi-step workflows without this
5. **Frontend rebuild** — Can't interact without UI

**Important but not blocking:**

6. GitHub sync — Nice for issue tracking
7. Project CRUD — Can work with tasks directly for now
8. NL parsing — Can create structured tasks manually
9. Conflict detection — Manual merge for now

---

## Toolbelt Reference

```yaml
# toolbelt.yaml — API keys for building user projects
github:
  token: ${GITHUB_TOKEN}
fly:
  token: ${FLY_API_TOKEN}
cloudflare:
  api_token: ${CLOUDFLARE_API_TOKEN}
  account_id: ${CLOUDFLARE_ACCOUNT_ID}
neon:
  api_key: ${NEON_API_KEY}
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
anthropic:
  api_key: ${ANTHROPIC_API_KEY}
fal:
  api_key: ${FAL_API_KEY}
```

---

## Notes for Claude

**Fresh context each iteration:** Re-read this file. Don't rely on conversation memory.

**Disk is state:** The codebase IS the source of truth. Read files to understand current state.

**Backpressure over prescription:** Focus on making tests pass, not following exact steps.

**The plan is disposable:** If something isn't working, try a different approach.

**Evidence required:** Before marking anything complete, show that `go test` and `go build` pass.

**Priority:** Phase 5 (Session Management / Ralph Loop) is the critical missing piece. The infrastructure exists; the orchestration doesn't.
