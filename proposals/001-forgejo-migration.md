# Proposal: Replace GitHub with Self-Hosted Forgejo

**Status:** Draft
**Author:** Dex
**Created:** 2026-02-03

---

## Agent Context

This proposal is designed to be implemented by an AI agent (Ralph-style loop). The implementation is broken into discrete, sequential phases. Each phase has clear acceptance criteria and can be validated before proceeding.

### Key Principles for Implementation

1. **Build incrementally** - Each phase produces working code that can be tested
2. **Abstraction first** - Create interfaces before implementations to enable swapping providers
3. **Preserve backwards compatibility** - GitHub provider should remain functional throughout
4. **Test at boundaries** - Verify each phase works before starting the next
5. **Follow existing patterns** - Match the codebase's existing style for handlers, database, etc.

### Critical Files to Understand First

Before starting implementation, read these files to understand existing patterns:

```
internal/github/app.go          # Current GitHub auth pattern
internal/github/sync.go         # How sync currently works
internal/api/handlers/github/   # API handler patterns
internal/db/models.go           # Database model patterns
internal/db/sqlite.go           # Migration patterns
internal/auth/passkey.go        # Existing WebAuthn implementation
frontend/src/components/onboarding/  # Onboarding UI patterns
```

### Dependencies to Add

```go
// go.mod additions
require (
    code.gitea.io/sdk/gitea v0.19.0  // Forgejo/Gitea SDK
)
```

---

## Summary

Replace the tight GitHub integration in Dex with a self-hosted Forgejo instance, providing full control over git hosting, issue tracking, and user management while eliminating external dependencies and reducing friction.

## Motivation

The current GitHub integration introduces unnecessary friction:

1. **External dependency** - Requires GitHub account, app setup, and internet connectivity
2. **Complex onboarding** - GitHub App manifest flow, organization selection, installation permissions
3. **Rate limits** - GitHub API rate limiting affects sync operations
4. **Privacy concerns** - All project data visible to GitHub
5. **Vendor lock-in** - Tightly coupled to GitHub's API and authentication model

A self-hosted Forgejo instance provides equivalent functionality with full control.

---

## Current State Analysis

### GitHub Features Currently Used

| Feature | Usage in Dex | Criticality |
|---------|--------------|-------------|
| Issues | Quest/objective tracking, progress comments | High |
| Pull Requests | Linked to objectives, auto-created | Medium |
| Repositories | Creation, workflow setup | Medium |
| GitHub App Auth | JWT tokens, installation management | High |
| Labels | `dex:quest`, `dex:objective` classification | Low |
| Comments | Status updates, checklist sync | Medium |

### Current Integration Points

```
internal/github/
├── app.go          # GitHub App manager, JWT generation
├── issue.go        # Issue CRUD with retry logic
├── comments.go     # Rate-limited commenting
└── sync.go         # Quest/objective sync orchestrator

internal/toolbelt/github.go    # Client wrapper
internal/api/handlers/github/  # API handlers
internal/db/github.go          # Persistence
```

### Database Fields Affected

- `Task`: `github_issue_number`, `pr_number`
- `Quest`: `github_issue_number`
- `Project`: `github_owner`, `github_repo`
- `GitHubAppConfig`: entire table
- `GitHubInstallation`: entire table

---

## Proposed Architecture

### High-Level Design

```
┌─────────────────────────────────────────────────────────────┐
│                      Dex Dashboard                          │
│                    (Primary Interface)                      │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │   Passkey   │  │   Quest/    │  │   Project           │ │
│  │   Auth      │  │   Task UI   │  │   Management        │ │
│  └──────┬──────┘  └─────────────┘  └─────────────────────┘ │
│         │                                                   │
│         │ OIDC Provider                                     │
└─────────┼───────────────────────────────────────────────────┘
          │
          │ SSO Token
          ▼
┌─────────────────────────────────────────────────────────────┐
│                    Forgejo Instance                         │
│                  (Embedded/Sidecar)                         │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │   Git       │  │   Issues    │  │   Pull Requests     │ │
│  │   Repos     │  │   Tracking  │  │   Code Review       │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │   Actions   │  │   Packages  │  │   Webhooks          │ │
│  │   (CI/CD)   │  │   Registry  │  │   (to Dex)          │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| Dex Dashboard | Primary UI, passkey auth, OIDC provider, quest management |
| Forgejo | Git hosting, issue tracking, PRs, CI/CD, code browsing |
| Dex Bot Account | Automated operations (repo creation, issue sync, PR creation) |

### Authentication Flow

```
User                    Dex                     Forgejo
  │                      │                         │
  │──── Passkey ────────►│                         │
  │                      │                         │
  │◄─── Session ─────────│                         │
  │                      │                         │
  │──── Access Forgejo ──┼────────────────────────►│
  │                      │                         │
  │                      │◄── OIDC Auth Request ───│
  │                      │                         │
  │                      │─── Token (already ──────►│
  │                      │    authenticated)       │
  │                      │                         │
  │◄─────────────────────┼─── Forgejo Session ─────│
```

---

## Implementation Phases

### Phase 1: Git Provider Abstraction Layer

**Goal:** Create an interface that abstracts git hosting operations, allowing multiple backends.

**Tasks:**
1. Create `internal/gitprovider/provider.go` with the `Provider` interface
2. Create `internal/gitprovider/types.go` with shared types (User, Org, Repo, Issue, PR)
3. Implement `internal/gitprovider/github/client.go` wrapping existing GitHub code
4. Add provider selection to configuration (`DEX_GIT_PROVIDER=github|forgejo`)
5. Update `internal/github/sync.go` to use the interface instead of direct GitHub calls

**Acceptance Criteria:**
- [ ] Existing GitHub functionality works unchanged
- [ ] `Provider` interface covers all current GitHub operations
- [ ] Can instantiate either provider based on config
- [ ] All tests pass

**Interface Definition:**

```go
// internal/gitprovider/provider.go

package gitprovider

import "context"

// Provider abstracts git hosting operations
type Provider interface {
    // Health
    Health(ctx context.Context) error

    // Users
    CreateUser(ctx context.Context, user *User) error
    UpdateUser(ctx context.Context, username string, settings *UserSettings) error
    DeleteUser(ctx context.Context, username string) error
    CreateAccessToken(ctx context.Context, username, tokenName string, scopes []string) (string, error)

    // Organizations
    CreateOrg(ctx context.Context, org *Organization) error
    AddOrgMember(ctx context.Context, org, username string, role OrgRole) error
    RemoveOrgMember(ctx context.Context, org, username string) error

    // Repositories
    CreateRepo(ctx context.Context, owner string, repo *Repository) error
    DeleteRepo(ctx context.Context, owner, repo string) error
    GetRepo(ctx context.Context, owner, repo string) (*Repository, error)

    // Issues
    CreateIssue(ctx context.Context, owner, repo string, issue *Issue) (*Issue, error)
    UpdateIssue(ctx context.Context, owner, repo string, number int, issue *IssueUpdate) error
    CloseIssue(ctx context.Context, owner, repo string, number int) error
    AddIssueComment(ctx context.Context, owner, repo string, number int, body string) error
    AddIssueLabels(ctx context.Context, owner, repo string, number int, labels []string) error

    // Pull Requests
    CreatePR(ctx context.Context, owner, repo string, pr *PullRequest) (*PullRequest, error)
    MergePR(ctx context.Context, owner, repo string, number int, method MergeMethod) error

    // Webhooks
    CreateWebhook(ctx context.Context, owner, repo string, hook *Webhook) error
}
```

**Types Definition:**

```go
// internal/gitprovider/types.go

package gitprovider

type User struct {
    Username    string
    Email       string
    FullName    string
    Password    string // Only for creation
    IsAdmin     bool
    MustChange  bool
}

type UserSettings struct {
    FullName  *string
    Bio       *string
    Location  *string
    Website   *string
    AvatarB64 *string // Base64 encoded image
}

type Organization struct {
    Name        string
    FullName    string
    Description string
    Visibility  Visibility
}

type Visibility string

const (
    VisibilityPublic  Visibility = "public"
    VisibilityPrivate Visibility = "private"
)

type OrgRole string

const (
    OrgRoleOwner  OrgRole = "owner"
    OrgRoleMember OrgRole = "member"
)

type Repository struct {
    Name          string
    Description   string
    Private       bool
    DefaultBranch string
    AutoInit      bool
}

type Issue struct {
    Number int64
    Title  string
    Body   string
    Labels []string
    State  IssueState
}

type IssueState string

const (
    IssueStateOpen   IssueState = "open"
    IssueStateClosed IssueState = "closed"
)

type IssueUpdate struct {
    Title  *string
    Body   *string
    State  *IssueState
}

type PullRequest struct {
    Number int64
    Title  string
    Body   string
    Head   string // Source branch
    Base   string // Target branch
}

type MergeMethod string

const (
    MergeMethodMerge  MergeMethod = "merge"
    MergeMethodSquash MergeMethod = "squash"
    MergeMethodRebase MergeMethod = "rebase"
)

type Webhook struct {
    URL         string
    ContentType string
    Secret      string
    Events      []string
    Active      bool
}
```

---

### Phase 2: Forgejo Provider Implementation

**Goal:** Implement the `Provider` interface for Forgejo using the Gitea SDK.

**Tasks:**
1. Add `code.gitea.io/sdk/gitea` to go.mod
2. Create `internal/gitprovider/forgejo/client.go` implementing `Provider`
3. Implement all interface methods using Gitea SDK
4. Add retry logic matching existing GitHub patterns
5. Write unit tests for Forgejo provider

**Acceptance Criteria:**
- [ ] All `Provider` interface methods implemented
- [ ] Can create users, orgs, repos, issues, PRs via Forgejo API
- [ ] Retry logic handles rate limits and transient failures
- [ ] Unit tests cover happy path and error cases

**Implementation Pattern:**

```go
// internal/gitprovider/forgejo/client.go

package forgejo

import (
    "context"
    "code.gitea.io/sdk/gitea"
    "github.com/lirancohen/dex/internal/gitprovider"
)

type Client struct {
    client   *gitea.Client
    baseURL  string
    botToken string
}

func New(baseURL, token string) (*Client, error) {
    client, err := gitea.NewClient(baseURL, gitea.SetToken(token))
    if err != nil {
        return nil, err
    }
    return &Client{
        client:   client,
        baseURL:  baseURL,
        botToken: token,
    }, nil
}

func (c *Client) Health(ctx context.Context) error {
    _, _, err := c.client.ServerVersion()
    return err
}

func (c *Client) CreateUser(ctx context.Context, user *gitprovider.User) error {
    _, _, err := c.client.AdminCreateUser(gitea.CreateUserOption{
        Username:           user.Username,
        Email:              user.Email,
        FullName:           user.FullName,
        Password:           user.Password,
        MustChangePassword: &user.MustChange,
    })
    return err
}

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, issue *gitprovider.Issue) (*gitprovider.Issue, error) {
    created, _, err := c.client.CreateIssue(owner, repo, gitea.CreateIssueOption{
        Title: issue.Title,
        Body:  issue.Body,
    })
    if err != nil {
        return nil, err
    }
    issue.Number = created.Index
    return issue, nil
}

// ... implement remaining methods
```

---

### Phase 3: Forgejo Manager (Lifecycle Management)

**Goal:** Create a manager that can start, stop, and configure a Forgejo instance.

**Tasks:**
1. Create `internal/forgejo/manager.go` for process management
2. Create `internal/forgejo/config.go` for app.ini generation
3. Implement startup with health check polling
4. Implement graceful shutdown
5. Add auto-configuration (admin user, bot account creation via CLI)

**Acceptance Criteria:**
- [ ] Can start Forgejo as subprocess
- [ ] Health check waits for Forgejo to be ready
- [ ] Can create admin user via CLI on first run
- [ ] Can generate bot token via CLI
- [ ] Graceful shutdown on Dex exit

**Manager Implementation:**

```go
// internal/forgejo/manager.go

package forgejo

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

type Manager struct {
    cmd        *exec.Cmd
    dataDir    string
    port       int
    adminToken string
    botToken   string
}

type Config struct {
    DataDir  string
    Port     int
    RootURL  string
}

func NewManager(cfg Config) *Manager {
    return &Manager{
        dataDir: cfg.DataDir,
        port:    cfg.Port,
    }
}

func (m *Manager) Start(ctx context.Context) error {
    // Ensure data directory exists
    if err := os.MkdirAll(m.dataDir, 0755); err != nil {
        return fmt.Errorf("create data dir: %w", err)
    }

    // Generate app.ini if not exists
    if err := m.ensureConfig(); err != nil {
        return fmt.Errorf("ensure config: %w", err)
    }

    // Start Forgejo process
    m.cmd = exec.CommandContext(ctx, "forgejo", "web",
        "--config", filepath.Join(m.dataDir, "app.ini"),
    )
    m.cmd.Dir = m.dataDir
    m.cmd.Env = append(os.Environ(),
        fmt.Sprintf("FORGEJO_WORK_DIR=%s", m.dataDir),
    )

    if err := m.cmd.Start(); err != nil {
        return fmt.Errorf("start forgejo: %w", err)
    }

    // Wait for health
    if err := m.waitReady(ctx); err != nil {
        m.Stop()
        return fmt.Errorf("wait ready: %w", err)
    }

    // Run first-time setup if needed
    if err := m.ensureSetup(ctx); err != nil {
        return fmt.Errorf("ensure setup: %w", err)
    }

    return nil
}

func (m *Manager) Stop() error {
    if m.cmd != nil && m.cmd.Process != nil {
        return m.cmd.Process.Signal(os.Interrupt)
    }
    return nil
}

func (m *Manager) waitReady(ctx context.Context) error {
    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) {
        // Try health endpoint
        // ...
        time.Sleep(500 * time.Millisecond)
    }
    return fmt.Errorf("timeout waiting for forgejo")
}

func (m *Manager) CLI(args ...string) (string, error) {
    cmd := exec.Command("forgejo", args...)
    cmd.Dir = m.dataDir
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("FORGEJO_WORK_DIR=%s", m.dataDir),
    )
    out, err := cmd.Output()
    return string(out), err
}

func (m *Manager) ensureSetup(ctx context.Context) error {
    // Check if admin exists, create if not
    // Generate tokens
    // Create bot account
    return nil
}
```

**Config Template:**

```go
// internal/forgejo/config.go

package forgejo

const appIniTemplate = `
[server]
ROOT_URL = {{.RootURL}}
HTTP_PORT = {{.Port}}
DISABLE_SSH = true
LFS_START_SERVER = false

[database]
DB_TYPE = sqlite3
PATH = {{.DataDir}}/forgejo.db

[security]
INSTALL_LOCK = true
SECRET_KEY = {{.SecretKey}}
INTERNAL_TOKEN = {{.InternalToken}}

[service]
DISABLE_REGISTRATION = true
REQUIRE_SIGNIN_VIEW = true
ENABLE_NOTIFY_MAIL = false

[oauth2]
ENABLE = true

[openid]
ENABLE_OPENID_SIGNIN = true
ENABLE_OPENID_SIGNUP = false

[mailer]
ENABLED = false

[log]
MODE = console
LEVEL = info

[repository]
DEFAULT_PRIVATE = private
`
```

---

### Phase 4: Database Schema Updates

**Goal:** Update database schema to support Forgejo configuration and generic git provider fields.

**Tasks:**
1. Create migration to add `forgejo_config` table
2. Create migration to add `invitations` table
3. Create migration to add `oidc_clients` table
4. Create migration to rename `github_*` columns to `git_*` (with aliases for backwards compat)
5. Add `git_provider` column to projects table
6. Update model structs in `internal/db/models.go`

**Acceptance Criteria:**
- [ ] Migrations run successfully on fresh DB
- [ ] Migrations run successfully on existing DB with GitHub data
- [ ] Old GitHub column names still work (aliases)
- [ ] New Forgejo config can be stored and retrieved

**New Tables:**

```sql
-- Forgejo instance configuration
CREATE TABLE IF NOT EXISTS forgejo_config (
    id INTEGER PRIMARY KEY,
    base_url TEXT NOT NULL,
    admin_token TEXT NOT NULL,
    bot_token TEXT NOT NULL,
    bot_username TEXT NOT NULL DEFAULT 'dex-bot',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- User invitations
CREATE TABLE IF NOT EXISTS invitations (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    invited_by INTEGER REFERENCES users(id),
    access_level TEXT NOT NULL DEFAULT 'contributor',
    workspaces TEXT,
    accepted_at DATETIME,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- OIDC configuration for Dex as provider
CREATE TABLE IF NOT EXISTS oidc_clients (
    id INTEGER PRIMARY KEY,
    client_id TEXT NOT NULL UNIQUE,
    client_secret TEXT NOT NULL,
    name TEXT NOT NULL,
    redirect_uris TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Column Renames (with backwards compat):**

```sql
-- Add new columns
ALTER TABLE projects ADD COLUMN git_owner TEXT;
ALTER TABLE projects ADD COLUMN git_repo TEXT;
ALTER TABLE projects ADD COLUMN git_provider TEXT DEFAULT 'forgejo';

-- Copy data
UPDATE projects SET git_owner = github_owner WHERE github_owner IS NOT NULL;
UPDATE projects SET git_repo = github_repo WHERE github_repo IS NOT NULL;

-- Note: Keep old columns for backwards compat, remove in future version
```

---

### Phase 5: OIDC Provider Implementation

**Goal:** Make Dex act as an OIDC provider so Forgejo can use it for SSO.

**Tasks:**
1. Create `internal/oidc/provider.go` with OIDC endpoints
2. Implement `/.well-known/openid-configuration` endpoint
3. Implement `/oauth2/authorize` endpoint
4. Implement `/oauth2/token` endpoint
5. Implement `/oauth2/userinfo` endpoint
6. Add OIDC routes to API server
7. Store OIDC client credentials for Forgejo

**Acceptance Criteria:**
- [ ] OIDC discovery endpoint returns valid configuration
- [ ] Authorization flow works with Forgejo as client
- [ ] Tokens are properly signed and validated
- [ ] User info endpoint returns correct user data
- [ ] SSO login from Forgejo works end-to-end

**OIDC Endpoints:**

```go
// internal/oidc/provider.go

package oidc

import (
    "crypto/rsa"
    "time"
    "github.com/golang-jwt/jwt/v5"
)

type Provider struct {
    issuer     string
    signingKey *rsa.PrivateKey
    db         *db.DB
}

type DiscoveryDocument struct {
    Issuer                string   `json:"issuer"`
    AuthorizationEndpoint string   `json:"authorization_endpoint"`
    TokenEndpoint         string   `json:"token_endpoint"`
    UserInfoEndpoint      string   `json:"userinfo_endpoint"`
    JwksURI               string   `json:"jwks_uri"`
    ResponseTypesSupported []string `json:"response_types_supported"`
    SubjectTypesSupported  []string `json:"subject_types_supported"`
    IDTokenSigningAlgValues []string `json:"id_token_signing_alg_values_supported"`
}

func (p *Provider) Discovery() *DiscoveryDocument {
    return &DiscoveryDocument{
        Issuer:                p.issuer,
        AuthorizationEndpoint: p.issuer + "/oauth2/authorize",
        TokenEndpoint:         p.issuer + "/oauth2/token",
        UserInfoEndpoint:      p.issuer + "/oauth2/userinfo",
        JwksURI:               p.issuer + "/.well-known/jwks.json",
        ResponseTypesSupported: []string{"code"},
        SubjectTypesSupported:  []string{"public"},
        IDTokenSigningAlgValues: []string{"RS256"},
    }
}

// HandleAuthorize - GET /oauth2/authorize
// HandleToken - POST /oauth2/token
// HandleUserInfo - GET /oauth2/userinfo
// HandleJWKS - GET /.well-known/jwks.json
```

---

### Phase 6: API Handlers for Forgejo Setup

**Goal:** Create API endpoints for Forgejo configuration and status.

**Tasks:**
1. Create `internal/api/handlers/forgejo/handlers.go`
2. Implement `GET /api/forgejo/status` - health and config status
3. Implement `POST /api/forgejo/setup` - trigger initial setup
4. Implement `GET /api/forgejo/config` - get current config
5. Add routes to API server
6. Update setup status to include Forgejo state

**Acceptance Criteria:**
- [ ] Status endpoint returns Forgejo health
- [ ] Setup endpoint triggers Forgejo initialization
- [ ] Config endpoint returns connection details
- [ ] Frontend can check Forgejo status during onboarding

**Handler Pattern:**

```go
// internal/api/handlers/forgejo/handlers.go

package forgejo

import (
    "net/http"
    "github.com/lirancohen/dex/internal/forgejo"
)

type Handler struct {
    manager *forgejo.Manager
    db      *db.DB
}

type StatusResponse struct {
    Running   bool   `json:"running"`
    Healthy   bool   `json:"healthy"`
    URL       string `json:"url,omitempty"`
    BotUser   string `json:"bot_user,omitempty"`
}

func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
    // Check manager status
    // Return JSON response
}

func (h *Handler) TriggerSetup(w http.ResponseWriter, r *http.Request) {
    // Start Forgejo if not running
    // Run setup if needed
    // Return status
}
```

---

### Phase 7: Frontend Onboarding Updates

**Goal:** Update the onboarding flow to set up Forgejo instead of GitHub.

**Tasks:**
1. Remove GitHub App setup steps from onboarding
2. Add Forgejo setup progress step
3. Add workspace creation step
4. Update onboarding state machine
5. Add Forgejo status polling during setup
6. Update completion screen with Forgejo links

**Acceptance Criteria:**
- [ ] Onboarding no longer mentions GitHub
- [ ] Forgejo setup shows progress indicators
- [ ] Workspace creation works via Forgejo API
- [ ] User can access Forgejo after onboarding

**New Onboarding Steps:**

```typescript
// frontend/src/components/onboarding/steps/ForgejoSetupStep.tsx

interface SetupStatus {
  step: 'starting' | 'creating_admin' | 'creating_bot' | 'configuring_sso' | 'complete';
  progress: number;
  error?: string;
}

export function ForgejoSetupStep() {
  const [status, setStatus] = useState<SetupStatus>({ step: 'starting', progress: 0 });

  useEffect(() => {
    // Poll /api/forgejo/status
    // Update progress
  }, []);

  return (
    <div>
      <h2>Setting Up Git Server</h2>
      <ProgressBar value={status.progress} />
      <StatusList>
        <StatusItem done={status.progress > 20}>Starting Forgejo instance</StatusItem>
        <StatusItem done={status.progress > 40}>Creating admin account</StatusItem>
        <StatusItem done={status.progress > 60}>Setting up Dex bot</StatusItem>
        <StatusItem done={status.progress > 80}>Configuring SSO</StatusItem>
        <StatusItem done={status.progress === 100}>Ready!</StatusItem>
      </StatusList>
    </div>
  );
}
```

**Updated Onboarding Flow:**

```
Step 1: Welcome
Step 2: Create Passkey (existing)
Step 3: Forgejo Setup (new - replaces GitHub steps)
Step 4: Create Workspace (new)
Step 5: First Project (optional)
Step 6: Complete
```

---

### Phase 8: Invitation System

**Goal:** Allow users to invite collaborators without requiring GitHub.

**Tasks:**
1. Create `internal/api/handlers/invitations/handlers.go`
2. Implement `POST /api/invitations` - create invitation
3. Implement `GET /api/invitations` - list pending invitations
4. Implement `DELETE /api/invitations/:id` - revoke invitation
5. Implement `POST /api/invitations/:token/accept` - accept invitation
6. Create invitation email/link generation
7. Add invitation acceptance frontend flow

**Acceptance Criteria:**
- [ ] Can create invitation for email address
- [ ] Invitation creates Forgejo user account
- [ ] Invitation link allows passkey registration
- [ ] Accepted user has access to specified workspaces

**Invitation Flow:**

```go
// POST /api/invitations
type CreateInvitationRequest struct {
    Email       string   `json:"email"`
    AccessLevel string   `json:"access_level"` // "contributor", "maintainer", "admin"
    Workspaces  []string `json:"workspaces"`
}

// Response includes invitation link
type CreateInvitationResponse struct {
    ID        int64  `json:"id"`
    Email     string `json:"email"`
    InviteURL string `json:"invite_url"`
    ExpiresAt string `json:"expires_at"`
}
```

---

### Phase 9: Migration & Cleanup

**Goal:** Support migration from GitHub-based installations and clean up old code.

**Tasks:**
1. Create migration tool for existing GitHub data
2. Document migration process
3. Add deprecation warnings to GitHub-specific code paths
4. Update all documentation to reference Forgejo
5. Add configuration to disable GitHub provider entirely

**Acceptance Criteria:**
- [ ] Existing GitHub installations can migrate to Forgejo
- [ ] Documentation is updated
- [ ] GitHub code paths show deprecation warnings
- [ ] Fresh installs use Forgejo by default

---

## Onboarding Flow Details

### Step 1: Welcome

```
┌─────────────────────────────────────────────────────────────┐
│                    Step 1: Welcome                          │
│                                                             │
│  "Welcome to Dex! Let's get you set up."                   │
│                                                             │
│  Dex will configure:                                        │
│  - Local git server (Forgejo)                              │
│  - Your personal workspace                                  │
│  - Secure passkey authentication                           │
│                                                             │
│                    [ Get Started ]                          │
└─────────────────────────────────────────────────────────────┘
```

### Step 2: Passkey Registration

```
┌─────────────────────────────────────────────────────────────┐
│                 Step 2: Create Your Passkey                 │
│                                                             │
│  Your passkey secures access to Dex and all connected      │
│  services. Use your device's biometrics or security key.   │
│                                                             │
│                 [ Register Passkey ]                        │
└─────────────────────────────────────────────────────────────┘
```

### Step 3: Forgejo Auto-Configuration

```
┌─────────────────────────────────────────────────────────────┐
│              Step 3: Setting Up Git Server                  │
│                                                             │
│  [x] Starting Forgejo instance                             │
│  [x] Creating admin account                                 │
│  [x] Configuring SSO with Dex                              │
│  [ ] Creating your workspace...                            │
│  [ ] Setting up Dex bot account                            │
│                                                             │
│  ████████████████████░░░░░░░░░░  65%                       │
└─────────────────────────────────────────────────────────────┘
```

**Backend Operations:**

```go
// 1. Start Forgejo (embedded or container)
forgejo.Start(config)

// 2. Wait for health check
forgejo.WaitReady()

// 3. Create admin account via CLI
forgejo.CLI("admin", "user", "create",
    "--username", "dex-admin",
    "--password", generateSecurePassword(),
    "--email", "admin@localhost",
    "--admin")

// 4. Generate admin token
adminToken := forgejo.CLI("admin", "user", "generate-access-token",
    "--username", "dex-admin",
    "--token-name", "dex-setup",
    "--scopes", "all",
    "--raw")

// 5. Configure OIDC auth source via API
forgejo.API.CreateAuthSource(AuthSource{
    Type:         "oauth2",
    Name:         "dex",
    Provider:     "openidConnect",
    ClientID:     dexOIDCClientID,
    ClientSecret: dexOIDCClientSecret,
    OpenIDURL:    "http://localhost:PORT/.well-known/openid-configuration",
})

// 6. Create dex-bot account
forgejo.API.AdminCreateUser(User{
    Username: "dex-bot",
    Email:    "bot@localhost",
    Password: generateSecurePassword(),
})

// 7. Generate bot token for API operations
botToken := forgejo.API.CreateAccessToken("dex-bot", "automation", []string{"all"})

// 8. Set bot profile
forgejo.API.UpdateUserSettings("dex-bot", Settings{
    FullName: "Dex",
    Bio:      "Your AI development assistant",
})

// 9. Upload bot avatar
forgejo.API.UploadAvatar("dex-bot", dexAvatarBase64)
```

### Step 4: User Account Linking

```
┌─────────────────────────────────────────────────────────────┐
│              Step 4: Your Git Account                       │
│                                                             │
│  Your Forgejo account has been created and linked to       │
│  your Dex passkey.                                          │
│                                                             │
│    Username: alice                                          │
│    Email: alice@localhost                                   │
│    Auth: Passkey (via Dex SSO)                             │
│                                                             │
│  You can access Forgejo directly at:                       │
│  http://localhost:3000                                      │
│                                                             │
│                      [ Continue ]                           │
└─────────────────────────────────────────────────────────────┘
```

### Step 5: Workspace Creation

```
┌─────────────────────────────────────────────────────────────┐
│              Step 5: Create Workspace                       │
│                                                             │
│  A workspace organizes your projects. You can create       │
│  multiple workspaces for different contexts.               │
│                                                             │
│  Workspace Name: [ my-workspace                         ]  │
│                                                             │
│  This will create:                                          │
│  - Organization: my-workspace                              │
│  - Team: maintainers (you + dex-bot)                       │
│  - Default repository settings                             │
│                                                             │
│                [ Create Workspace ]                         │
└─────────────────────────────────────────────────────────────┘
```

**Backend Operations:**

```go
// 1. Create organization
forgejo.API.AdminCreateOrg(user, Org{
    Username:   workspaceName,
    FullName:   workspaceName,
    Visibility: "private",
})

// 2. Add dex-bot to organization with write access
forgejo.API.AddOrgMember(workspaceName, "dex-bot", "owner")

// 3. Create maintainers team
forgejo.API.CreateTeam(workspaceName, Team{
    Name:       "maintainers",
    Permission: "write",
    Units:      []string{"repo.code", "repo.issues", "repo.pulls"},
})

// 4. Add user to maintainers team
forgejo.API.AddTeamMember(teamID, username)
```

### Step 6: First Project

```
┌─────────────────────────────────────────────────────────────┐
│              Step 6: Add Your First Project                 │
│                                                             │
│  ( ) Import existing repository                            │
│      Clone from URL or local path                          │
│                                                             │
│  (x) Create new project                                    │
│      Start fresh with a new repository                     │
│                                                             │
│  ( ) Skip for now                                          │
│      You can add projects later                            │
│                                                             │
│  Project Name: [ my-awesome-project                     ]  │
│                                                             │
│                 [ Create Project ]                          │
└─────────────────────────────────────────────────────────────┘
```

### Step 7: Complete

```
┌─────────────────────────────────────────────────────────────┐
│                    You're All Set!                          │
│                                                             │
│  Dex is ready to help you build.                           │
│                                                             │
│  [x] Passkey configured                                    │
│  [x] Git server running                                    │
│  [x] Workspace created: my-workspace                       │
│  [x] Project ready: my-awesome-project                     │
│                                                             │
│  Quick Links:                                               │
│  - Dashboard: http://localhost:8080                        │
│  - Git Server: http://localhost:3000                       │
│  - Clone URL: http://localhost:3000/my-workspace/my-project│
│                                                             │
│               [ Open Dashboard ]                            │
└─────────────────────────────────────────────────────────────┘
```

---

## Inviting Additional Users

```
┌─────────────────────────────────────────────────────────────┐
│                   Invite Collaborator                       │
│                                                             │
│  Email: [ bob@example.com                               ]  │
│                                                             │
│  Access Level:                                              │
│  [ Contributor (can push to assigned branches)          v] │
│                                                             │
│  Workspaces:                                                │
│  [x] my-workspace                                          │
│  [ ] another-workspace                                     │
│                                                             │
│              [ Send Invitation ]                            │
└─────────────────────────────────────────────────────────────┘
```

**Backend Operations:**

```go
// 1. Create user account in Forgejo
forgejo.API.AdminCreateUser(User{
    Username:           generateUsername(email),
    Email:              email,
    MustChangePassword: false, // Will use SSO
})

// 2. Create pending invitation in Dex
dex.DB.CreateInvitation(Invitation{
    Email:       email,
    InvitedBy:   currentUser,
    Workspaces:  selectedWorkspaces,
    AccessLevel: accessLevel,
    Token:       generateInviteToken(),
    ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
})

// 3. Send invitation email (or display link)
inviteURL := fmt.Sprintf("http://localhost:8080/invite/%s", token)
```

**Invite Acceptance:**

```
┌─────────────────────────────────────────────────────────────┐
│              You've Been Invited to Dex                     │
│                                                             │
│  Alice invited you to collaborate on:                      │
│  - my-workspace                                             │
│                                                             │
│  To get started, create your passkey:                      │
│                                                             │
│              [ Accept & Create Passkey ]                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration

### Environment Variables

```bash
# Git Provider Selection
DEX_GIT_PROVIDER=forgejo  # or "github" for backward compat

# Forgejo Configuration (embedded mode)
DEX_FORGEJO_ENABLED=true
DEX_FORGEJO_PORT=3000
DEX_FORGEJO_DATA_DIR=/var/lib/dex/forgejo

# Forgejo Configuration (external mode)
DEX_FORGEJO_EXTERNAL_URL=https://git.example.com
DEX_FORGEJO_ADMIN_TOKEN=<token>

# OIDC Provider
DEX_OIDC_ISSUER=http://localhost:8080
DEX_OIDC_SIGNING_KEY=<key>
```

### Forgejo app.ini Template

```ini
[server]
ROOT_URL = http://localhost:3000
HTTP_PORT = 3000
DISABLE_SSH = true

[database]
DB_TYPE = sqlite3
PATH = /data/forgejo.db

[security]
INSTALL_LOCK = true
SECRET_KEY = <generated>

[service]
DISABLE_REGISTRATION = true
REQUIRE_SIGNIN_VIEW = true
ENABLE_NOTIFY_MAIL = false

[oauth2]
ENABLE = true

[openid]
ENABLE_OPENID_SIGNIN = true
ENABLE_OPENID_SIGNUP = false

[mailer]
ENABLED = false

[log]
MODE = console
LEVEL = info
```

---

## Deployment Options

### Option A: Embedded Binary (Recommended)

Bundle Forgejo as a subprocess managed by Dex. Single binary distribution with automatic lifecycle management.

### Option B: Docker Sidecar

Run Forgejo in a container alongside Dex:

```yaml
services:
  dex:
    image: dex:latest
    ports:
      - "8080:8080"
    environment:
      - FORGEJO_URL=http://forgejo:3000
    depends_on:
      - forgejo

  forgejo:
    image: codeberg.org/forgejo/forgejo:9
    volumes:
      - forgejo-data:/data
```

### Option C: External Instance

Point to existing Forgejo/Gitea instance with admin token.

---

## Security Considerations

1. **Token Storage**: All tokens stored encrypted in database
2. **OIDC Security**: Use secure signing keys, short token lifetimes
3. **Network Isolation**: Forgejo can bind to localhost only
4. **Bot Permissions**: Bot account has minimal required permissions
5. **Audit Logging**: Log all administrative actions

---

## Rollback Plan

If issues arise:

1. GitHub provider remains available as alternative
2. Database migrations are reversible
3. Configuration switch between providers
4. Data export from Forgejo to GitHub possible via git + API

---

## Success Metrics

1. **Setup Time**: < 2 minutes for complete onboarding (vs ~10 min with GitHub)
2. **Reliability**: No external API failures or rate limits
3. **Privacy**: All data remains local
4. **User Experience**: Single sign-on, no context switching

---

## Open Questions

1. **Forgejo Actions**: Should we enable CI/CD or keep it simple?
2. **Email**: Required for invitations, or use alternative notification?
3. **Backup**: Include Forgejo data in Dex backup strategy?
4. **Multi-user default**: Always set up for multi-user, or have single-user mode?

---

## References

- [Forgejo Documentation](https://forgejo.org/docs/)
- [Forgejo API](https://forgejo.org/docs/latest/user/api-usage/)
- [Gitea Go SDK](https://code.gitea.io/sdk/gitea)
- [WebAuthn Spec](https://www.w3.org/TR/webauthn/)
- [OpenID Connect](https://openid.net/connect/)
