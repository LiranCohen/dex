# POINDEXTER (dex) — Your Nerdy AI Orchestration Genius

> **Disk is state. Git is memory. Fresh context each iteration.**

## Completion Promise

This task is **COMPLETE** when all of these are true:

```
[ ] go build ./cmd/dex && go test ./... → PASS
[ ] Frontend builds: cd frontend && bun run build → PASS
[ ] Can authenticate via passkey (WebAuthn) from mobile
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
- Never store secrets in code — use passkeys for auth, env vars for API keys
- Never auto-merge conflicts — always require user approval
- Never ignore budget limits — always pause and ask
- Never expose to public internet — Tailscale only
- Never hardcode API keys — use toolbelt.yaml

---

## Current State Summary

**Overall Completion: ~60-70%**

**What EXISTS and WORKS (✅):**
- Go module initialized with all dependencies
- SQLite database with 7 migrations (users, projects, tasks, dependencies, sessions, checkpoints, approvals)
- WebAuthn/Passkey + JWT authentication system (core logic)
- All 11 toolbelt clients (GitHub, Fly, Cloudflare, Neon, Upstash, Resend, BetterStack, Doppler, MoneyDevKit, Anthropic, fal.ai)
- Task CRUD with state machine and dependency graph
- Git worktree management (create, remove, list, status)
- Priority queue scheduler (heap-based, max 25 parallel)
- Echo API server with 13 endpoints
- Session manager with budget tracking
- Ralph loop core logic (budget enforcement, completion detection, hat transitions)
- 9 hat prompt templates
- WebSocket hub infrastructure (`internal/api/websocket/`)
- Hat transition validation (`internal/orchestrator/transitions.go`)
- Frontend login page with passkey authentication

**What is PARTIALLY DONE (⚠️):**
- Auth API endpoints (exist but need verification)
- Ralph loop execution (logic ready, never tested with real Anthropic API)
- Session manager (structure exists, real execution untested)
- Git PR creation (code exists, untested)

**What is MISSING (❌):**
- Project CRUD endpoints (DB ready, no API)
- Approval endpoints (DB ready, no API)
- Session control endpoints (pause/resume/cancel/logs)
- GitHub bi-directional sync (`internal/github/sync.go`)
- GitHub webhooks (`internal/github/webhooks.go`)
- Natural language task parsing (`internal/task/parser.go`)
- Frontend UI beyond login (dashboard, tasks, approvals, WebSocket client)
- Integration tests
- Real API testing with Anthropic

---

## E2E Gaps — PRIORITIZED ACTION LIST

> **Ralph: Work through these in order. Each gap blocks end-to-end functionality.**

### GAP 1: Frontend UI (CRITICAL — Blocks User Interaction)
**Status:** 5% complete
**Impact:** Users cannot interact with the system beyond login

**What exists:**
- Login page with passphrase input/generation
- BIP39 crypto functions in `frontend/src/lib/crypto.ts`
- Auth flow (challenge → sign → verify)
- Zustand auth store in `frontend/src/stores/auth.ts`

**What's missing:**
```
frontend/src/pages/Dashboard.tsx       — System status, active tasks, quick create
frontend/src/pages/Tasks.tsx           — Task list with filters, real-time status
frontend/src/pages/TaskDetail.tsx      — Full task info, logs, actions
frontend/src/pages/Approvals.tsx       — Pending approvals, approve/reject
frontend/src/pages/Projects.tsx        — Project management
frontend/src/hooks/useWebSocket.ts     — Connect to /api/v1/ws, handle events
frontend/src/components/TaskCard.tsx   — Task display component
frontend/src/components/LogViewer.tsx  — Streaming session logs
```

**Done when:**
- [ ] Dashboard shows running sessions and queue depth
- [ ] Can create, view, and manage tasks
- [ ] Real-time updates via WebSocket work
- [ ] Can approve/reject from UI
- [ ] `bun run build` succeeds

---

### GAP 2: Project CRUD Endpoints (HIGH — Blocks Project Setup)
**Status:** 0% API, 100% DB
**Impact:** No way to create/manage projects via API

**What exists:**
- `internal/db/projects.go` — Full CRUD operations
- `internal/db/models.go` — Project struct defined

**What's missing:**
```
internal/api/server.go — Add routes:
  GET    /api/v1/projects              — List projects
  POST   /api/v1/projects              — Create project
  GET    /api/v1/projects/:id          — Get project
  PUT    /api/v1/projects/:id          — Update project
  DELETE /api/v1/projects/:id          — Delete project
  POST   /api/v1/projects/:id/provision — Provision toolbelt services
```

**Done when:**
- [ ] All 6 endpoints implemented
- [ ] Can create project and see it in list
- [ ] `go test ./internal/api/...` passes

---

### GAP 3: Approval Endpoints (HIGH — Blocks User Decision Points)
**Status:** 0% API, 100% DB
**Impact:** Users cannot act on approval requests

**What exists:**
- `internal/db/approvals.go` — Full CRUD operations
- `internal/db/models.go` — Approval struct defined

**What's missing:**
```
internal/api/server.go — Add routes:
  GET    /api/v1/approvals             — List pending approvals
  GET    /api/v1/approvals/:id         — Get approval details
  POST   /api/v1/approvals/:id/approve — Approve request
  POST   /api/v1/approvals/:id/reject  — Reject request
```

**Done when:**
- [ ] All 4 endpoints implemented
- [ ] Approval status updates in DB
- [ ] WebSocket broadcasts approval events

---

### GAP 4: Session Control Endpoints (HIGH — Blocks Task Management)
**Status:** 0%
**Impact:** Cannot pause, resume, cancel tasks or view logs

**What's missing:**
```
internal/api/server.go — Add routes:
  POST   /api/v1/tasks/:id/pause       — Pause running session
  POST   /api/v1/tasks/:id/resume      — Resume paused session
  POST   /api/v1/tasks/:id/cancel      — Cancel task entirely
  GET    /api/v1/tasks/:id/logs        — Stream session output (SSE or WS)

  GET    /api/v1/sessions              — List all sessions
  GET    /api/v1/sessions/:id          — Get session details
  POST   /api/v1/sessions/:id/kill     — Force kill session
```

**Done when:**
- [ ] Can pause/resume running tasks
- [ ] Can view streaming logs
- [ ] Session state persists across pause/resume

---

### GAP 5: GitHub Bi-directional Sync (MEDIUM — Blocks Issue Tracking)
**Status:** 0%
**Impact:** Tasks don't sync with GitHub issues

**What exists:**
- `internal/toolbelt/github.go` — GitHub API client with issue/PR operations

**What's missing:**
```
internal/github/sync.go — Implement:
  - SyncTaskToIssue(task) — Create/update GitHub issue from task
  - SyncIssueToTask(issue) — Create/update task from GitHub issue
  - OnTaskCreate → CreateIssue with label "dex:task"
  - OnTaskStart → AddLabel "dex:in-progress"
  - OnTaskComplete → CreatePR, CloseIssue
  - OnIssueCreate → CreateTask with label "dex:external"
  - OnIssueClose → MarkTaskComplete
```

**Done when:**
- [ ] Creating task creates GitHub issue
- [ ] Completing task creates PR
- [ ] Task status reflected in issue labels

---

### GAP 6: GitHub Webhooks (MEDIUM — Blocks External Triggers)
**Status:** 0%
**Impact:** GitHub events don't trigger Dex actions

**What's missing:**
```
internal/github/webhooks.go — Implement:
  - WebhookHandler(c echo.Context) — Main handler
  - VerifySignature(payload, sig) — HMAC verification
  - HandleIssueEvent(event) — issues.opened, closed, edited
  - HandlePREvent(event) — pull_request.opened, merged, closed
  - HandleCheckEvent(event) — check_suite.completed

internal/api/server.go — Add route:
  POST   /api/v1/webhooks/github
```

**Done when:**
- [ ] Webhook endpoint accepts GitHub events
- [ ] Signature verification works
- [ ] Issue events update task status
- [ ] PR merge updates task status

---

### GAP 7: Natural Language Task Parsing (LOW — Nice to Have)
**Status:** 0%
**Impact:** Can only create structured tasks, not via conversation

**What's missing:**
```
internal/task/parser.go — Implement:
  - ParseNaturalLanguage(input string) (*TaskProposal, error)
  - Use Anthropic API to extract:
    - Title, description
    - Type (epic, feature, bug, task, chore)
    - Priority (1-5)
    - Suggested autonomy level
    - Required toolbelt services
  - Return proposal for user confirmation
```

**Done when:**
- [ ] Can POST natural language to `/api/v1/tasks/parse`
- [ ] Returns structured proposal
- [ ] User can confirm/modify before creation

---

### GAP 8: Real Anthropic API Testing (CRITICAL — Blocks Core Functionality)
**Status:** 0%
**Impact:** Ralph loop logic untested with real API

**What exists:**
- `internal/session/ralph.go` — Full loop logic (323 lines)
- `internal/session/ralph_test.go` — Unit tests for budget/completion/transitions
- `internal/toolbelt/anthropic.go` — API client

**What's missing:**
- Integration test with real API key
- Verify response parsing works
- Test hat transition detection in real responses
- Test completion signal detection
- Test budget enforcement with real token counts

**Done when:**
- [ ] Run one real session end-to-end
- [ ] Session completes or transitions correctly
- [ ] Logs captured and stored
- [ ] Tokens/cost tracked accurately

---

### GAP 9: Test Coverage (LOW — Quality Improvement)
**Status:** ~5% coverage
**Impact:** Regressions possible, confidence low

**What exists:**
- `internal/session/ralph_test.go` — Only test file (385 lines, ~20 tests)

**What's missing:**
```
internal/api/server_test.go      — API endpoint tests
internal/db/sqlite_test.go       — Database operation tests
internal/auth/auth_test.go       — Auth system tests
internal/git/git_test.go         — Git operation tests
internal/task/task_test.go       — Task service tests
frontend/src/**/*.test.tsx       — Frontend component tests
```

**Done when:**
- [ ] `go test ./...` covers major paths
- [ ] Critical paths have tests (auth, task state, session lifecycle)

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

## Phase 5: Session Management [~80% COMPLETE — NEEDS REAL TESTING]

This is the **core orchestration layer** that makes Poindexter actually work.

### Checkpoint 5.1: Claude SDK Integration
- [x] `internal/toolbelt/anthropic.go`: Chat() method implemented
- [x] Direct API integration (no SDK wrapper needed)
- [ ] **NEEDS: Real API testing** — Logic exists, never tested with live API

### Checkpoint 5.2: Hat Prompt Loading
- [x] `internal/session/prompts.go`: LoadAll(), GetPrompt(hat)
- [x] 9 hat templates exist in `prompts/hats/`
- [x] Template variable injection via buildPrompt() in ralph.go

### Checkpoint 5.3: Ralph Loop — THE CORE
- [x] **IMPLEMENTED: `internal/session/ralph.go`** (323 lines) — Contains:

```go
// ALREADY IMPLEMENTED in internal/session/ralph.go
// - Budget checking (tokens, time, dollars, iterations)
// - Prompt building with task context
// - Completion detection (TASK_COMPLETE, HAT_COMPLETE)
// - Hat transition detection (HAT_TRANSITION:next_hat)
// - Checkpointing every 5 iterations
// - WebSocket event broadcasting
```

**Completion Detection (IMPLEMENTED):**
- [x] Detects `TASK_COMPLETE` or `HAT_COMPLETE` in response
- [x] Detects `HAT_TRANSITION:hat_name` format
- [x] Budget enforcement (tokens, dollars, iterations, time)

### Checkpoint 5.4: Checkpointing
- [x] Database table exists (session_checkpoints)
- [x] Checkpoint creation in ralph.go (every 5 iterations)
- [x] Manager loads checkpoints on restart

### Checkpoint 5.5: Hat Transitions
- [x] **IMPLEMENTED: `internal/orchestrator/transitions.go`** — Contains:
  - HatTransitions map with valid transitions
  - ValidateTransition() function
  - All 9 hats defined with allowed transitions
  - Terminal hats identified (documenter, devops, conflict_manager)

**Phase 5 Status:**
```
[x] Ralph loop core logic implemented
[x] Budget enforcement works
[x] Completion detection works
[x] Hat transition validation works
[x] Checkpointing infrastructure exists
[ ] NEEDS: Real API testing with Anthropic
[ ] NEEDS: Integration test end-to-end
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

## Phase 7: Mobile-First Frontend [~15% COMPLETE — NEEDS UI PAGES]

Login flow works. Dashboard and task management pages needed.

### Checkpoint 7.1: Core Setup
- [x] Vite + React + TypeScript scaffold
- [x] Tailwind configured
- [x] Zustand installed and used for auth store
- [x] API client in `frontend/src/lib/api.ts`
- [ ] **NEEDS: WebSocket client hook** — `useWebSocket.ts` exists but incomplete

### Checkpoint 7.2: WebSocket Server (Backend)
- [x] **IMPLEMENTED: `internal/api/websocket/`** — Contains:
  - `hub.go` — Client management, broadcasting
  - `handler.go` — WebSocket upgrade, message handling
  - Endpoint: `WS /api/v1/ws`
  - Events: task.*, session.*, approval.*
  - Client subscription support

### Checkpoint 7.3: Auth Flow (Frontend)
- [x] **IMPLEMENTED: Login page**
  - Generate or input passphrase
  - BIP39 validation in `frontend/src/lib/crypto.ts`
  - Ed25519 signing
  - JWT stored in localStorage via Zustand

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

> **Ralph: This is your work queue. Complete in order.**

### TIER 1: Critical Path (Must complete for MVP)

| # | Gap | Effort | Why Critical |
|---|-----|--------|--------------|
| 1 | **Frontend UI** | Large | Users can't interact without it |
| 2 | **Project CRUD endpoints** | Small | Can't set up projects |
| 3 | **Approval endpoints** | Small | Can't respond to system requests |
| 4 | **Session control endpoints** | Medium | Can't manage running tasks |
| 5 | **Real Anthropic API test** | Small | Must verify Ralph loop works |

### TIER 2: Important (Needed for full functionality)

| # | Gap | Effort | Why Important |
|---|-----|--------|---------------|
| 6 | **GitHub sync** | Medium | Enables issue tracking integration |
| 7 | **GitHub webhooks** | Medium | Enables external triggers |

### TIER 3: Nice to Have (Polish)

| # | Gap | Effort | Why Nice |
|---|-----|--------|----------|
| 8 | **NL task parsing** | Medium | Convenience, can create structured tasks manually |
| 9 | **Test coverage** | Large | Quality, can ship without |

### Recommended Attack Order

```
1. Project CRUD endpoints      (2 hours) → Unlocks project management
2. Approval endpoints          (2 hours) → Unlocks user decisions
3. Session control endpoints   (3 hours) → Unlocks task control
4. Real Anthropic API test     (1 hour)  → Validates core works
5. Frontend Dashboard          (4 hours) → Basic visibility
6. Frontend Task Management    (4 hours) → Full task control
7. Frontend WebSocket client   (2 hours) → Real-time updates
8. Frontend Approvals page     (2 hours) → Complete user flow
9. GitHub sync                 (4 hours) → Issue integration
10. GitHub webhooks            (3 hours) → External triggers
```

**Total estimated: ~27 hours of focused work**

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

## Notes for Ralph

**Fresh context each iteration:** Re-read this file. Don't rely on conversation memory.

**Disk is state:** The codebase IS the source of truth. Read files to understand current state.

**Backpressure over prescription:** Focus on making tests pass, not following exact steps.

**The plan is disposable:** If something isn't working, try a different approach.

**Evidence required:** Before marking anything complete, show that `go test` and `go build` pass.

---

## Ralph Quick Reference

### What's Actually Working (Don't Rebuild)
```
✅ Auth system (internal/auth/) — BIP39, Ed25519, JWT all working
✅ Database (internal/db/) — All tables, migrations, CRUD working
✅ Toolbelt (internal/toolbelt/) — All 11 clients implemented
✅ Task system (internal/task/) — State machine, graph, service working
✅ Git (internal/git/) — Worktrees, operations working
✅ Scheduler (internal/orchestrator/scheduler.go) — Priority queue working
✅ Ralph loop (internal/session/ralph.go) — Core logic done, needs real testing
✅ WebSocket hub (internal/api/websocket/) — Infrastructure exists
✅ Hat transitions (internal/orchestrator/transitions.go) — Validation exists
```

### What Needs API Wiring (DB Ready, Just Add Routes)
```
⚠️ Projects — internal/db/projects.go exists, add routes to server.go
⚠️ Approvals — internal/db/approvals.go exists, add routes to server.go
⚠️ Sessions — internal/db/sessions.go exists, add routes to server.go
```

### What Needs Implementation
```
❌ internal/github/sync.go — Task ↔ Issue sync
❌ internal/github/webhooks.go — GitHub webhook handler
❌ internal/task/parser.go — Natural language parsing
❌ frontend/src/pages/*.tsx — All UI pages beyond login
❌ frontend/src/hooks/useWebSocket.ts — WebSocket client
```

### Quick Validation Commands
```bash
go build ./cmd/dex                    # Must pass
go test ./...                         # Must pass
cd frontend && bun run build          # Must pass
```

### Starting the Server
```bash
go run ./cmd/dex -db dex.db -addr :8080 -static ./frontend/dist -toolbelt toolbelt.yaml
```

### Current API That Works
```
GET  /api/v1/system/status
GET  /api/v1/toolbelt/status
POST /api/v1/toolbelt/test
GET  /api/v1/tasks
POST /api/v1/tasks
GET  /api/v1/tasks/:id
PUT  /api/v1/tasks/:id
DELETE /api/v1/tasks/:id
POST /api/v1/tasks/:id/start
GET  /api/v1/tasks/:id/worktree/status
GET  /api/v1/worktrees
DELETE /api/v1/worktrees/:task_id
POST /api/v1/auth/challenge
POST /api/v1/auth/verify
WS   /api/v1/ws
```

---

**Priority Update (January 2025):** The Ralph loop core is implemented but untested with real API. Frontend is the biggest gap — without it, users cannot interact with the system. Focus on: (1) wiring up missing endpoints, (2) building frontend UI, (3) testing with real Anthropic API.
