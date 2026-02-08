# Proposal: Mesh-Native Git Hosting with Forgejo on HQ

**Status:** ✅ IMPLEMENTED
**Author:** Dex
**Created:** 2026-02-03
**Revised:** 2026-02-05
**Completed:** 2026-02-08

> **Implementation**: See `internal/forgejo/` for embedded Forgejo manager.

---

## Summary

Replace the GitHub integration with a Forgejo instance running directly on the HQ node, exposed to the mesh network at a reserved address (`git.<username>.enbox.id`). This eliminates all external dependencies for git hosting while making repositories accessible to every device on the Campus mesh — laptops, phones, worker nodes — without any internet dependency.

---

## Motivation

The current architecture has three external dependencies that compromise self-sovereignty:

1. **GitHub** — All code hosting, issues, PRs, and sync depend on github.com
2. **Cloudflare Tunnels** — Alternative access path depends on Cloudflare infrastructure (optional)

The mesh networking system (dexnet/Central) provides self-contained networking. This proposal completes the picture by replacing GitHub with embedded Forgejo. The result: a fully self-contained development platform where the HQ node is the authoritative source for everything.

### Why Not Just Use Bare Git?

Forgejo provides features that bare git repos cannot:

- **Web UI** for browsing code, diffs, and history from any device on the mesh
- **Issues & PRs** that the quest/objective system currently depends on
- **API** compatible with the Gitea SDK, giving us a clean programmatic interface
- **Access control** for multi-user scenarios (invitations, teams, permissions)
- **Webhooks** for triggering Dex actions on push, PR, etc.

Bare git would require reimplementing all of this. Forgejo gives it to us for free in a single binary.

---

## Architecture

### Network Topology

```
Campus Mesh (WireGuard / dexnet)
─────────────────────────────────────────────────────
│                                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │            HQ Node (dex server)             │    │
│  │                                             │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │    │
│  │  │ Dex API  │  │ Forgejo  │  │ Realtime │  │    │
│  │  │ :8080    │  │ :3000    │  │ (centri) │  │    │
│  │  └────┬─────┘  └────┬─────┘  └──────────┘  │    │
│  │       │              │                      │    │
│  │       └──────┬───────┘                      │    │
│  │              │                              │    │
│  │    ┌─────────▼──────────┐                   │    │
│  │    │  Mesh Listener(s)  │                   │    │
│  │    │                    │                   │    │
│  │    │  hq.enbox.id:443   │  ← Dex dashboard  │    │
│  │    │  git.hq.enbox.id   │  ← Forgejo web+api│    │
│  │    │  :9418 (git proto) │  ← git clone/push │    │
│  │    └────────────────────┘                   │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐      │
│  │  Laptop   │  │  Phone    │  │  Worker   │      │
│  │  (peer)   │  │  (peer)   │  │  (peer)   │      │
│  │           │  │           │  │           │      │
│  │ git clone │  │ browse    │  │ git push  │      │
│  │ git push  │  │ code/PRs  │  │ (CI jobs) │      │
│  └───────────┘  └───────────┘  └───────────┘      │
─────────────────────────────────────────────────────
```

### Reserved Mesh Addresses

The HQ node registers multiple hostnames on the mesh:

| Address | Service | Purpose |
|---------|---------|---------|
| `hq.<username>.enbox.id` | Dex API + Dashboard | Primary management interface |
| `git.<username>.enbox.id` | Forgejo (HTTP/HTTPS) | Web UI, API, HTTP git transport |

Both resolve to the same HQ node's mesh IP but are handled by a reverse proxy (or separate mesh listeners) that routes by hostname. The `<username>` portion comes from the Campus owner's identity registered with Central.

**Alternative (simpler):** A single hostname `hq.<username>.enbox.id` with path-based routing:
- `/` → Dex dashboard
- `/git/` → Forgejo (or Forgejo at a subpath)

The dedicated `git.` subdomain is preferred because it makes clone URLs clean:
```
git clone http://git.hq.enbox.id/workspace/my-project.git
```

### How Mesh Listeners Work

The mesh client already exposes `Listen()` and `Dial()` — currently unused. This proposal puts them to work:

```go
// In the HQ startup sequence, after mesh client is connected:

// 1. Create a mesh listener for Forgejo
gitListener, err := meshClient.Listen("tcp", ":3000")

// 2. Forgejo binds to this listener instead of localhost
// All traffic arrives encrypted over WireGuard from mesh peers
forgejoServer.Serve(gitListener)
```

This means Forgejo is **only accessible via the mesh**. No ports are opened on the public internet. No tunnels. No external DNS. The mesh's WireGuard encryption handles transport security.

### Process Architecture on HQ

```
dex (main process, PID 1)
 ├── API server (Echo, :8080 on mesh)
 ├── Realtime (Centrifuge, WebSocket)
 ├── Mesh client (dexnet/tsnet)
 ├── Forgejo manager (subprocess lifecycle)
 │    └── forgejo (child process, :3000 on localhost)
 └── Reverse proxy / mesh router
      ├── hq.*.enbox.id → API server
      └── git.*.enbox.id → Forgejo
```

Dex manages Forgejo as a child process. Forgejo binds to `localhost:3000` (never exposed directly). A mesh-level router inside Dex accepts connections on the mesh listener and proxies to the right backend based on the SNI/Host header.

---

## Implementation Plan

### Phase 1: Forgejo Lifecycle Manager

**Goal:** Dex can start, stop, health-check, and auto-configure a Forgejo instance as a child process.

**What gets built:**
- `internal/forgejo/manager.go` — Start/stop Forgejo subprocess
- `internal/forgejo/config.go` — Generate `app.ini` from Dex configuration
- `internal/forgejo/setup.go` — First-run setup (admin user, bot account, tokens)

**Forgejo Configuration (app.ini):**

```ini
[server]
HTTP_ADDR = 127.0.0.1
HTTP_PORT = 3000
ROOT_URL = http://git.hq.enbox.id
DISABLE_SSH = true        ; SSH not needed — mesh handles auth/encryption
LFS_START_SERVER = false  ; Keep it simple initially

[database]
DB_TYPE = sqlite3
PATH = {data_dir}/forgejo/forgejo.db

[security]
INSTALL_LOCK = true
SECRET_KEY = {generated}
INTERNAL_TOKEN = {generated}

[service]
DISABLE_REGISTRATION = true   ; Users created only via Dex invitation
REQUIRE_SIGNIN_VIEW = true    ; Must be authenticated to see anything
ENABLE_NOTIFY_MAIL = false

[repository]
DEFAULT_PRIVATE = private     ; Everything private by default
ROOT = {data_dir}/forgejo/repositories

[log]
MODE = console
LEVEL = warn                  ; Dex controls logging
```

**Key Design Decisions:**
- SSH disabled — the mesh provides encrypted transport; HTTP git over mesh is sufficient
- Registration disabled — all user provisioning goes through Dex
- Private by default — this is a personal/team dev environment, not a public forge
- SQLite — matches Dex's own database approach, no external DB needed

**First-Run Bootstrap:**

```go
func (m *Manager) bootstrap(ctx context.Context) error {
    // 1. Create admin account via Forgejo CLI
    m.cli("admin", "user", "create",
        "--username", "dex-admin",
        "--password", secureRandom(),
        "--email", "admin@hq.local",
        "--admin")

    // 2. Generate admin API token
    adminToken := m.cli("admin", "user", "generate-access-token",
        "--username", "dex-admin",
        "--token-name", "dex-admin",
        "--scopes", "all",
        "--raw")

    // 3. Create bot account for automated operations
    m.api(adminToken).CreateUser(User{
        Username: "dex-bot",
        Email:    "bot@hq.local",
        Password: secureRandom(),
    })

    // 4. Generate bot token
    botToken := m.api(adminToken).CreateAccessToken("dex-bot", "automation", []string{"all"})

    // 5. Store tokens in Dex database (encrypted)
    m.db.SetSecret("forgejo_admin_token", adminToken)
    m.db.SetSecret("forgejo_bot_token", botToken)

    return nil
}
```

**Acceptance Criteria:**
- [ ] `dex --forgejo` starts Forgejo as a child process on `:3000`
- [ ] Health check polls `/api/v1/version` until ready
- [ ] First run creates admin + bot accounts automatically
- [ ] Tokens stored encrypted in Dex's SQLite database
- [ ] Graceful shutdown kills Forgejo child process
- [ ] Forgejo data lives under `{data_dir}/forgejo/`

---

### Phase 2: Mesh Service Router

**Goal:** Expose both Dex and Forgejo to the mesh through hostname-based routing.

**What gets built:**
- `internal/mesh/router.go` — Accept mesh connections, route by Host header
- Updates to `internal/mesh/client.go` — Register multiple service names
- Updates to `internal/api/server.go` — Wire mesh listener into server startup

**Router Design:**

```go
// internal/mesh/router.go

type ServiceRouter struct {
    meshClient *Client
    routes     map[string]string // hostname pattern → backend addr
    listener   net.Listener
}

func (r *ServiceRouter) Start(ctx context.Context) error {
    // Listen on mesh port 443 (HTTPS) or 80 (HTTP)
    var err error
    r.listener, err = r.meshClient.Listen("tcp", ":80")
    if err != nil {
        return fmt.Errorf("mesh listen: %w", err)
    }

    // Accept connections and route based on Host header
    for {
        conn, err := r.listener.Accept()
        if err != nil {
            return err
        }
        go r.handleConn(conn)
    }
}

func (r *ServiceRouter) handleConn(conn net.Conn) {
    // Peek at HTTP Host header, proxy to correct backend
    // hq.*.enbox.id → localhost:8080 (Dex API)
    // git.*.enbox.id → localhost:3000 (Forgejo)
}
```

**TLS Consideration:** Traffic on the mesh is already encrypted by WireGuard. We can run HTTP (not HTTPS) internally and let the mesh handle encryption. This avoids certificate management complexity. If we want HTTPS for defense-in-depth, the mesh client's `ListenTLS()` can use auto-generated certs from the control plane.

**Hostname Registration:** The dexnet control plane (Central) needs to know that the HQ node should be reachable at both `hq.<user>.enbox.id` and `git.<user>.enbox.id`. Two approaches:

1. **Multiple tsnet.Server instances** — Each service gets its own mesh identity. Simple but uses more mesh IPs.
2. **Single mesh identity + DNS aliases** — HQ registers one IP, Central adds CNAME-like records for `git.*` → same IP. Preferred, requires Central support.
3. **Single hostname, port-based** — HQ is `hq.<user>.enbox.id`, Forgejo at `:3000`, Dex at `:8080`. Simplest. No Central changes needed.

**Recommended: Option 3 for Phase 2, evolve to Option 2 later.**

This means initial clone URLs are:
```
git clone http://hq.<user>.enbox.id:3000/workspace/project.git
```

And later, with DNS alias support from Central:
```
git clone http://git.<user>.enbox.id/workspace/project.git
```

**Acceptance Criteria:**
- [ ] Forgejo web UI accessible from any mesh peer at `hq:3000`
- [ ] Dex dashboard accessible from any mesh peer at `hq:8080`
- [ ] No ports exposed on public network
- [ ] Connection from non-mesh device is refused

---

### Phase 3: Git Provider Abstraction

**Goal:** Decouple the quest/objective sync system from GitHub-specific code.

**What gets built:**
- `internal/gitprovider/provider.go` — Interface definition
- `internal/gitprovider/forgejo/client.go` — Forgejo implementation using Gitea SDK
- `internal/gitprovider/github/client.go` — Wrapper around existing GitHub code (keep working)
- `internal/gitprovider/types.go` — Shared types

**Provider Interface (focused on what Dex actually uses):**

```go
package gitprovider

type Provider interface {
    // Repos
    CreateRepo(ctx context.Context, owner string, repo *Repository) error
    GetRepo(ctx context.Context, owner, repo string) (*Repository, error)
    DeleteRepo(ctx context.Context, owner, repo string) error

    // Issues (used by quest/objective sync)
    CreateIssue(ctx context.Context, owner, repo string, issue *Issue) (*Issue, error)
    UpdateIssue(ctx context.Context, owner, repo string, number int, update *IssueUpdate) error
    CloseIssue(ctx context.Context, owner, repo string, number int) error
    AddComment(ctx context.Context, owner, repo string, number int, body string) error
    SetLabels(ctx context.Context, owner, repo string, number int, labels []string) error

    // Pull Requests
    CreatePR(ctx context.Context, owner, repo string, pr *PullRequest) (*PullRequest, error)
    MergePR(ctx context.Context, owner, repo string, number int, method MergeMethod) error

    // Webhooks
    CreateWebhook(ctx context.Context, owner, repo string, hook *Webhook) error

    // Health
    Ping(ctx context.Context) error
}
```

**Forgejo Implementation:**

```go
package forgejo

import (
    "code.gitea.io/sdk/gitea"
)

type Client struct {
    api     *gitea.Client
    baseURL string
}

func New(baseURL, token string) (*Client, error) {
    api, err := gitea.NewClient(baseURL, gitea.SetToken(token))
    if err != nil {
        return nil, err
    }
    return &Client{api: api, baseURL: baseURL}, nil
}

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, issue *gitprovider.Issue) (*gitprovider.Issue, error) {
    created, _, err := c.api.CreateIssue(owner, repo, gitea.CreateIssueOption{
        Title: issue.Title,
        Body:  issue.Body,
    })
    if err != nil {
        return nil, err
    }
    issue.Number = int(created.Index)
    return issue, nil
}
// ... remaining methods follow the same pattern
```

**Migration Path:**
- Add `DEX_GIT_PROVIDER=forgejo|github` config flag
- Default to `forgejo` for new installs
- Existing installs with GitHub configured continue to use GitHub
- `internal/github/sync.go` refactored to call `gitprovider.Provider` methods

**Acceptance Criteria:**
- [ ] Existing GitHub sync works unchanged through the abstraction
- [ ] New Forgejo provider passes all integration tests against local instance
- [ ] Provider selected at startup via configuration
- [ ] Quest/objective CRUD operations work with Forgejo backend

---

### Phase 4: Database & Project Model Updates

**Goal:** Generalize the database schema from GitHub-specific to provider-agnostic.

**Schema Changes:**

```sql
-- New: Forgejo instance configuration
CREATE TABLE IF NOT EXISTS forgejo_config (
    id INTEGER PRIMARY KEY,
    base_url TEXT NOT NULL DEFAULT 'http://127.0.0.1:3000',
    admin_token_ref TEXT NOT NULL,   -- Reference to secrets table
    bot_token_ref TEXT NOT NULL,     -- Reference to secrets table
    bot_username TEXT NOT NULL DEFAULT 'dex-bot',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Add provider-agnostic columns to projects
ALTER TABLE projects ADD COLUMN git_provider TEXT DEFAULT 'forgejo';
ALTER TABLE projects ADD COLUMN git_owner TEXT;     -- Org/user on the provider
ALTER TABLE projects ADD COLUMN git_repo TEXT;      -- Repo name on the provider

-- Migrate existing data
UPDATE projects SET
    git_provider = 'github',
    git_owner = github_owner,
    git_repo = github_repo
WHERE github_owner IS NOT NULL;

-- Update projects that don't have GitHub to use Forgejo
UPDATE projects SET
    git_provider = 'forgejo'
WHERE github_owner IS NULL;
```

**Model Updates:**

```go
type Project struct {
    ID             string
    Name           string
    RepoPath       string
    GitProvider    string         // "forgejo" or "github"
    GitOwner       sql.NullString // Owner on the git provider
    GitRepo        sql.NullString // Repo name on the git provider
    // Keep old fields for backwards compat during transition
    GitHubOwner    sql.NullString
    GitHubRepo     sql.NullString
    RemoteOrigin   sql.NullString
    RemoteUpstream sql.NullString
    DefaultBranch  string
    Services       ProjectServices
    CreatedAt      time.Time
}
```

**Acceptance Criteria:**
- [ ] Migration runs cleanly on existing databases
- [ ] New installs create `forgejo_config` table
- [ ] Projects can be associated with either provider
- [ ] Old `github_*` columns still readable (backwards compat)

---

### Phase 5: Onboarding Flow Replacement

**Goal:** New installs set up Forgejo instead of GitHub. Zero external accounts needed.

**New Onboarding Steps:**

```
Step 1: Welcome
    "Dex runs your own private git server. No accounts needed."

Step 2: Create Passkey
    (Unchanged — device biometric / security key)

Step 3: Git Server Setup (automatic)
    ████████████████░░░░░░  75%
    [x] Starting Forgejo
    [x] Creating admin account
    [x] Setting up automation bot
    [ ] Creating your workspace...

Step 4: Workspace
    Name: [ my-workspace ]
    → Creates Forgejo org + default repo

Step 5: Anthropic API Key
    (Unchanged — needed for Claude sessions)

Step 6: Done
    "Your git server is running at git.hq.enbox.id"
    "Clone URL: http://hq:3000/my-workspace/my-project.git"
```

**What Gets Removed:**
- GitHub App manifest flow (complex multi-step OAuth dance)
- Organization selection step
- Installation permissions step
- GitHub token management
- All `setup/steps/github-*` API endpoints

**What Gets Added:**
- `POST /api/v1/setup/forgejo` — Trigger Forgejo bootstrap
- `GET /api/v1/setup/forgejo/status` — Poll bootstrap progress
- `POST /api/v1/setup/workspace` — Create Forgejo org (already exists, needs rewiring)

**Acceptance Criteria:**
- [ ] Fresh install completes without any external account
- [ ] Forgejo running and healthy at end of onboarding
- [ ] Workspace org created with bot account as member
- [ ] User can clone the default repo from a mesh peer

---

### Phase 6: Quest/Objective Sync Migration

**Goal:** Wire the quest and objective system to use Forgejo instead of GitHub for issue tracking.

**Current Flow (GitHub):**
1. Quest created → GitHub Issue created in project repo
2. Objectives added → Sub-issues or checklist items on the issue
3. Task completes → Comment posted, checklist updated
4. PR created → Linked to the objective issue

**New Flow (Forgejo):**
Same logic, same UX, different backend. The `gitprovider.Provider` abstraction makes this a configuration change, not a code rewrite.

**Files to Update:**
- `internal/github/sync.go` → Refactor to use `gitprovider.Provider`
- `internal/api/handlers/github/sync.go` → Rename to `internal/api/handlers/gitsync/`
- `internal/api/handlers/github/handlers.go` → Move provider-agnostic parts out
- `internal/quest/handler.go` → Use provider interface instead of `toolbelt.GitHubClient`

**Acceptance Criteria:**
- [ ] Creating a quest creates a Forgejo issue
- [ ] Objective progress updates Forgejo issue comments
- [ ] Task completion posts to Forgejo
- [ ] PR creation works against Forgejo repos

---

### Phase 7: Git Remote Wiring

**Goal:** When Dex creates worktrees and pushes branches, it pushes to Forgejo instead of GitHub.

**Current git flow:**
1. Project has `RemoteOrigin` pointing to `github.com/org/repo`
2. Worktree created, branch pushed to origin
3. PR created via GitHub API

**New git flow:**
1. Project has `RemoteOrigin` pointing to `http://127.0.0.1:3000/workspace/repo.git`
2. Worktree created, branch pushed to origin (Forgejo)
3. PR created via Forgejo API (gitprovider.Provider)

**Auth for git push:** Forgejo is on localhost. We use the bot token for HTTP basic auth:

```
http://dex-bot:{token}@127.0.0.1:3000/workspace/repo.git
```

This is already the pattern used for GitHub (`x-access-token:{token}@github.com`), so the existing `git/operations.go` auth injection works with minimal changes.

**For mesh peers pushing directly:** They authenticate to Forgejo via OIDC (see Phase 8) and push over HTTP through the mesh:

```
git clone http://hq:3000/workspace/my-project.git
# Auth handled by git credential helper that gets OIDC tokens from Dex
```

**Acceptance Criteria:**
- [ ] `git push` from HQ to Forgejo works with bot token
- [ ] `git clone` from mesh peer works
- [ ] `git push` from mesh peer works with user credentials
- [ ] Existing worktree workflow unchanged for the end user

---

### Phase 8: Authentication (OIDC SSO)

**Goal:** Users authenticate to Forgejo using their Dex passkey — single sign-on across the mesh.

**Flow:**

```
Mesh Peer (laptop)          HQ (Dex)              HQ (Forgejo)
      │                       │                       │
      │──── Open Forgejo ─────┼──────────────────────►│
      │                       │                       │
      │                       │◄── OIDC redirect ─────│
      │                       │                       │
      │◄── Passkey prompt ────│                       │
      │                       │                       │
      │──── Biometric ───────►│                       │
      │                       │                       │
      │                       │─── OIDC token ───────►│
      │                       │                       │
      │◄──────────────────────┼─── Logged in ─────────│
```

**What gets built:**
- `internal/oidc/provider.go` — Minimal OIDC provider (Dex as identity provider)
- Endpoints: `/.well-known/openid-configuration`, `/oauth2/authorize`, `/oauth2/token`, `/oauth2/userinfo`, `/.well-known/jwks.json`
- Forgejo configured with OIDC auth source pointing to Dex

**This is the most complex phase** but it's also optional for single-user setups where the HQ owner is the only user. In that case, Forgejo's admin token handles everything and the web UI is accessed directly through Dex's dashboard (embedded iframe or proxy).

**Acceptance Criteria:**
- [ ] OIDC discovery endpoint works
- [ ] Forgejo redirects to Dex for login
- [ ] Passkey auth grants Forgejo session
- [ ] User identity flows through correctly (username, email)

---

### Phase 9: GitHub Removal & Cleanup

**Goal:** Remove GitHub as a required dependency. Keep it as an optional provider for users who want it.

**What gets removed from the critical path:**
- `internal/github/app.go` — GitHub App manager (JWT generation, installation management)
- `internal/api/handlers/github/` — GitHub-specific API handlers (keep as optional)
- All `setup/steps/github-*` endpoints from onboarding
- GitHub App config from database (mark as optional/legacy)
- `SecretKeyGitHubToken` from default secrets

**What stays (optional):**
- `internal/gitprovider/github/` — GitHub as an alternative provider
- Import from GitHub functionality (clone existing repos into Forgejo)
- `toolbelt.GitHubClient` — Available if user configures a token

**Acceptance Criteria:**
- [ ] Fresh install works with zero GitHub references
- [ ] `internal/github/` code compiles but is not imported by default
- [ ] Existing installs with GitHub continue to function
- [ ] Migration guide documents how to move repos from GitHub to Forgejo

---

## Forgejo Binary Distribution

Forgejo is a single static binary (~100MB). Downloaded on first run from a **Dex-controlled mirror** to ensure version stability.

**Decision:** Download on first run, but from our own mirror — not directly from Codeberg. This prevents upstream releases from breaking installs. The Dex build pins a specific Forgejo version + SHA256 checksum.

```go
const (
    forgejoVersion  = "9.0.3"
    forgejoChecksum = "sha256:abc123..." // Pinned checksum
    forgejeMirror   = "https://dl.enbox.id/forgejo"  // Our mirror
    forgejoUpstream = "https://codeberg.org/forgejo/forgejo/releases/download"  // Fallback
)

func (m *Manager) ensureBinary() error {
    binaryPath := filepath.Join(m.dataDir, "bin", "forgejo")
    if fileExists(binaryPath) && checksumMatches(binaryPath, forgejoChecksum) {
        return nil
    }

    // Try mirror first, fall back to upstream
    urls := []string{
        fmt.Sprintf("%s/v%s/forgejo-%s-linux-amd64", forgejeMirror, forgejoVersion, forgejoVersion),
        fmt.Sprintf("%s/v%s/forgejo-%s-linux-amd64", forgejoUpstream, forgejoVersion, forgejoVersion),
    }

    return downloadAndVerify(urls, binaryPath, forgejoChecksum)
}
```

Cached in `{data_dir}/forgejo/bin/forgejo`. A system-installed Forgejo binary can be used instead via `--forgejo-binary=/usr/bin/forgejo` flag.

---

## Data Layout

```
/opt/dex/                          (or {data_dir})
├── dex.db                         # Dex SQLite database
├── worktrees/                     # Task worktrees (created from bare repos)
│   └── my-project/
│       └── abc123/
├── mesh/                          # Mesh networking state
└── forgejo/                       # Forgejo data directory
    ├── bin/
    │   └── forgejo                # Forgejo binary
    ├── app.ini                    # Forgejo configuration
    ├── forgejo.db                 # Forgejo SQLite database
    ├── repositories/              # Bare git repositories (Forgejo-managed)
    │   └── workspace/
    │       └── my-project.git/    # ← Dex creates worktrees directly from here
    └── log/                       # Forgejo logs
```

**Decision: No separate `repos/` directory.** Dex creates worktrees directly from Forgejo's bare repositories. Since Forgejo stores bare repos and Dex uses `git worktree add`, they share the same object store with zero duplication:

```bash
# Dex creates a task worktree directly from Forgejo's bare repo:
git -C /opt/dex/forgejo/repositories/workspace/my-project.git \
    worktree add /opt/dex/worktrees/my-project/abc123 -b task/task-abc123

# Commits land directly in the bare repo's object store.
# Forgejo sees new branches immediately — no push needed.
# PRs created via Forgejo API referencing the branch.
```

This means the `Project.RepoPath` field points to the Forgejo bare repo path, and the existing `WorktreeManager` creates worktrees from there. The key insight: since Forgejo is local, there's no "remote" — the bare repo IS the server.

---

## Git Workflow (How It All Connects)

### Creating a Project

```
User clicks "New Project"
    → Dex API: POST /api/v1/projects
        → Forgejo API: Create repo in workspace org
        → DB: Store project with git_provider=forgejo, git_owner=workspace, git_repo=project
        → Project.RepoPath = {data_dir}/forgejo/repositories/workspace/project.git
        (No clone needed — the bare repo IS the project repo)
```

### Running a Task

```
Task started
    → Git Service: git worktree add from bare repo on new branch
    → Session: Claude Code runs in worktree
    → On completion: commits are already in the bare repo (no push needed)
    → Forgejo API: Create PR referencing the branch
    → Quest sync: Update issue with status
```

### Cloning from a Mesh Peer (e.g., laptop)

```
$ git clone http://hq:3000/workspace/my-project.git
# Mesh resolves "hq" to HQ's mesh IP
# WireGuard encrypts the connection
# Forgejo serves the repo over HTTP
```

---

## Security Model

| Layer | Protection |
|-------|-----------|
| **Network** | WireGuard encryption on all mesh traffic. Forgejo never exposed to public internet. |
| **Authentication** | Passkey (WebAuthn) via Dex OIDC → Forgejo. No passwords. |
| **Authorization** | Forgejo's org/team/repo permissions. Bot account has scoped tokens. |
| **Data at rest** | SQLite databases on HQ's filesystem. Standard Linux file permissions. |
| **Tokens** | Stored encrypted in Dex's database. Never transmitted outside the mesh. |
| **Git transport** | HTTP over WireGuard. Equivalent security to SSH without key management. |

---

## Resolved Decisions

1. **Repo storage** — Dex creates worktrees directly from Forgejo's bare repos. No separate `repos/` directory. No duplication. See Data Layout section.

2. **Binary distribution** — Download on first run from a Dex-controlled mirror (`dl.enbox.id`), falling back to upstream Codeberg. Version + checksum pinned in Dex binary. See Binary Distribution section.

3. **DNS aliases (`git.<user>.enbox.id`)** — Deferred. Requires Central to support multiple DNS names for a single mesh node. Phase 2 uses port-based routing (`hq:3000` for Forgejo, `hq:8080` for Dex). A separate effort will add DNS alias support to Central, at which point Phase 2 can upgrade to hostname-based routing. Not blocking for implementation.

## Open Questions

1. **CI/CD (Forgejo Actions)** — Enable or keep disabled?
   - Recommendation: Disabled initially. Dex's session system IS the CI/CD. Forgejo Actions can be added later for standard workflows.

2. **Backup strategy** — How to back up Forgejo data?
   - Recommendation: `forgejo dump` command, integrated into Dex's backup system. Single command backs up both Dex and Forgejo databases + repositories.

3. **Multi-HQ / Federation** — What if someone runs multiple HQ nodes?
   - Out of scope for now. One HQ per Campus. Federation is a future proposal.

---

## Implementation Priority

The phases are ordered by dependency and value:

| Phase | Depends On | Value | Effort |
|-------|-----------|-------|--------|
| 1. Forgejo Lifecycle | Nothing | Foundation | Medium |
| 2. Mesh Router | Phase 1 + mesh client | Mesh accessibility | Medium |
| 3. Git Provider Abstraction | Nothing (parallel with 1) | Decoupling | Medium |
| 4. Database Updates | Phase 3 | Data model | Low |
| 5. Onboarding | Phases 1, 4 | UX | Medium |
| 6. Quest Sync | Phases 3, 4 | Feature parity | Medium |
| 7. Git Remote Wiring | Phases 1, 4 | Core workflow | Low |
| 8. OIDC SSO | Phases 1, 5 | Multi-user | High |
| 9. GitHub Removal | All above | Cleanup | Low |

**Phases 1 and 3 can be built in parallel.** Phase 2 can start as soon as Phase 1 is working. Phases 5-7 can be done in any order once their dependencies are met.

**Minimum viable: Phases 1, 3, 4, 5, 7** — This gives us a working Forgejo instance that Dex uses for all git operations, with the new onboarding flow. Mesh exposure (Phase 2) and SSO (Phase 8) can follow.

---

## References

- [Forgejo Documentation](https://forgejo.org/docs/)
- [Forgejo API Reference](https://forgejo.org/docs/latest/user/api-usage/)
- [Gitea Go SDK](https://code.gitea.io/sdk/gitea)
- [Forgejo Binary Releases](https://codeberg.org/forgejo/forgejo/releases)
- [dexnet Documentation](https://pkg.go.dev/github.com/WebP2P/dexnet) (Tailscale fork for mesh networking)
