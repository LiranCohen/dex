# Proposal: Replace GitHub with Self-Hosted Forgejo

**Status:** Draft
**Author:** Dex
**Created:** 2026-02-03

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

## Detailed Onboarding Flow

### Phase 1: Initial Setup (First Run)

```
┌─────────────────────────────────────────────────────────────┐
│                    Step 1: Welcome                          │
│                                                             │
│  "Welcome to Dex! Let's get you set up."                   │
│                                                             │
│  Dex will configure:                                        │
│  • Local git server (Forgejo)                              │
│  • Your personal workspace                                  │
│  • Secure passkey authentication                           │
│                                                             │
│                    [ Get Started ]                          │
└─────────────────────────────────────────────────────────────┘
```

### Phase 2: Passkey Registration

```
┌─────────────────────────────────────────────────────────────┐
│                 Step 2: Create Your Passkey                 │
│                                                             │
│  Your passkey secures access to Dex and all connected      │
│  services. Use your device's biometrics or security key.   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                 🔐                                   │   │
│  │         Touch ID / Face ID / PIN                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│                 [ Register Passkey ]                        │
└─────────────────────────────────────────────────────────────┘
```

### Phase 3: Forgejo Auto-Configuration

```
┌─────────────────────────────────────────────────────────────┐
│              Step 3: Setting Up Git Server                  │
│                                                             │
│  ✓ Starting Forgejo instance                               │
│  ✓ Creating admin account                                   │
│  ✓ Configuring SSO with Dex                                │
│  ◐ Creating your workspace...                              │
│  ○ Setting up Dex bot account                              │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ████████████████████░░░░░░░░░░  65%                 │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Behind the scenes:**

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

### Phase 4: User Account Linking

```
┌─────────────────────────────────────────────────────────────┐
│              Step 4: Your Git Account                       │
│                                                             │
│  Your Forgejo account has been created and linked to       │
│  your Dex passkey.                                          │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  👤 Username: alice                                 │   │
│  │  📧 Email: alice@localhost                          │   │
│  │  🔗 Auth: Passkey (via Dex SSO)                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  You can access Forgejo directly at:                       │
│  http://localhost:3000                                      │
│                                                             │
│                      [ Continue ]                           │
└─────────────────────────────────────────────────────────────┘
```

### Phase 5: Workspace Creation

```
┌─────────────────────────────────────────────────────────────┐
│              Step 5: Create Workspace                       │
│                                                             │
│  A workspace organizes your projects. You can create       │
│  multiple workspaces for different contexts.               │
│                                                             │
│  Workspace Name:                                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ my-workspace                                        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  This will create:                                          │
│  • Organization: my-workspace                              │
│  • Team: maintainers (you + dex-bot)                       │
│  • Default repository settings                             │
│                                                             │
│                [ Create Workspace ]                         │
└─────────────────────────────────────────────────────────────┘
```

**Behind the scenes:**

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

### Phase 6: First Project

```
┌─────────────────────────────────────────────────────────────┐
│              Step 6: Add Your First Project                 │
│                                                             │
│  ○ Import existing repository                              │
│     Clone from URL or local path                           │
│                                                             │
│  ● Create new project                                      │
│     Start fresh with a new repository                      │
│                                                             │
│  ○ Skip for now                                            │
│     You can add projects later                             │
│                                                             │
│  ─────────────────────────────────────────────────────────  │
│                                                             │
│  Project Name:                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ my-awesome-project                                  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│                 [ Create Project ]                          │
└─────────────────────────────────────────────────────────────┘
```

### Phase 7: Complete

```
┌─────────────────────────────────────────────────────────────┐
│                   🎉 You're All Set!                        │
│                                                             │
│  Dex is ready to help you build.                           │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  ✓ Passkey configured                               │   │
│  │  ✓ Git server running                               │   │
│  │  ✓ Workspace created: my-workspace                  │   │
│  │  ✓ Project ready: my-awesome-project                │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Quick Links:                                               │
│  • Dashboard: http://localhost:8080                        │
│  • Git Server: http://localhost:3000                       │
│  • Clone URL: http://localhost:3000/my-workspace/my-project│
│                                                             │
│               [ Open Dashboard ]                            │
└─────────────────────────────────────────────────────────────┘
```

## Inviting Additional Users

When a user wants to invite collaborators:

```
┌─────────────────────────────────────────────────────────────┐
│                   Invite Collaborator                       │
│                                                             │
│  Email:                                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ bob@example.com                                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Access Level:                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ▼ Contributor (can push to assigned branches)       │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Workspaces:                                                │
│  ☑ my-workspace                                            │
│  ☐ another-workspace                                       │
│                                                             │
│              [ Send Invitation ]                            │
└─────────────────────────────────────────────────────────────┘
```

**Behind the scenes:**

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

// 3. Send invitation email
email.Send(InviteTemplate{
    To:         email,
    InviteURL:  fmt.Sprintf("http://localhost:8080/invite/%s", token),
    InvitedBy:  currentUser.Name,
    Workspaces: selectedWorkspaces,
})
```

**Invite acceptance flow:**

```
┌─────────────────────────────────────────────────────────────┐
│              You've Been Invited to Dex                     │
│                                                             │
│  Alice invited you to collaborate on:                      │
│  • my-workspace                                             │
│                                                             │
│  To get started, create your passkey:                      │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                 🔐                                   │   │
│  │         Touch ID / Face ID / PIN                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│              [ Accept & Create Passkey ]                    │
└─────────────────────────────────────────────────────────────┘
```

## API Abstraction Layer

### Interface Definition

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

// User represents a git provider user
type User struct {
    Username    string
    Email       string
    FullName    string
    Password    string // Only for creation
    IsAdmin     bool
    MustChange  bool
}

// UserSettings for profile updates
type UserSettings struct {
    FullName  *string
    Bio       *string
    Location  *string
    Website   *string
    AvatarB64 *string // Base64 encoded image
}

// Organization represents a workspace/org
type Organization struct {
    Name        string
    FullName    string
    Description string
    Visibility  Visibility
}

// Repository represents a git repository
type Repository struct {
    Name          string
    Description   string
    Private       bool
    DefaultBranch string
    AutoInit      bool
}

// Issue represents an issue/task
type Issue struct {
    Number int64  // Set after creation
    Title  string
    Body   string
    Labels []string
    State  IssueState
}

// PullRequest represents a PR/MR
type PullRequest struct {
    Number int64  // Set after creation
    Title  string
    Body   string
    Head   string // Source branch
    Base   string // Target branch
}
```

### Forgejo Implementation

```go
// internal/gitprovider/forgejo/client.go

package forgejo

import (
    "code.gitea.io/sdk/gitea"
    "github.com/yourusername/dex/internal/gitprovider"
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

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, issue *gitprovider.Issue) (*gitprovider.Issue, error) {
    created, _, err := c.client.CreateIssue(owner, repo, gitea.CreateIssueOption{
        Title:  issue.Title,
        Body:   issue.Body,
        Labels: issue.Labels,
    })
    if err != nil {
        return nil, err
    }

    issue.Number = created.Index
    return issue, nil
}

// ... implement other methods
```

### Migration: GitHub to Provider Interface

```go
// Before (direct GitHub)
func (s *SyncService) SyncQuestToIssue(quest *db.Quest) error {
    client := s.github.GetClient()
    issue, _, err := client.Issues.Create(ctx, owner, repo, &github.IssueRequest{
        Title: &quest.Title,
        Body:  &quest.Description,
    })
    // ...
}

// After (abstracted)
func (s *SyncService) SyncQuestToIssue(quest *db.Quest) error {
    issue, err := s.provider.CreateIssue(ctx, owner, repo, &gitprovider.Issue{
        Title: quest.Title,
        Body:  quest.Description,
    })
    // ...
}
```

## Database Schema Changes

### New Tables

```sql
-- Forgejo instance configuration
CREATE TABLE forgejo_config (
    id INTEGER PRIMARY KEY,
    base_url TEXT NOT NULL,
    admin_token TEXT NOT NULL,  -- Encrypted
    bot_token TEXT NOT NULL,    -- Encrypted
    bot_username TEXT NOT NULL DEFAULT 'dex-bot',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- User invitations
CREATE TABLE invitations (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    invited_by INTEGER REFERENCES users(id),
    access_level TEXT NOT NULL DEFAULT 'contributor',
    workspaces TEXT,  -- JSON array of workspace names
    accepted_at DATETIME,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- OIDC configuration for Dex as provider
CREATE TABLE oidc_clients (
    id INTEGER PRIMARY KEY,
    client_id TEXT NOT NULL UNIQUE,
    client_secret TEXT NOT NULL,  -- Encrypted
    name TEXT NOT NULL,
    redirect_uris TEXT NOT NULL,  -- JSON array
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Schema Migrations

```sql
-- Rename GitHub-specific columns to generic names
ALTER TABLE projects RENAME COLUMN github_owner TO git_owner;
ALTER TABLE projects RENAME COLUMN github_repo TO git_repo;

-- Add provider type column
ALTER TABLE projects ADD COLUMN git_provider TEXT DEFAULT 'forgejo';

-- Keep issue/PR number columns (same concept)
-- github_issue_number -> issue_number (optional rename)
```

## Deployment Options

### Option A: Embedded Binary (Recommended)

Bundle Forgejo as a subprocess managed by Dex:

```go
type ForgejoManager struct {
    cmd      *exec.Cmd
    dataDir  string
    port     int
}

func (m *ForgejoManager) Start() error {
    m.cmd = exec.Command("forgejo", "web",
        "--config", filepath.Join(m.dataDir, "app.ini"),
        "--port", strconv.Itoa(m.port),
    )
    m.cmd.Dir = m.dataDir
    return m.cmd.Start()
}
```

**Pros:**
- Single binary distribution (bundle Forgejo)
- Automatic lifecycle management
- Consistent versioning

**Cons:**
- Larger binary size (~100MB)
- Resource overhead

### Option B: Docker Sidecar

Run Forgejo in a container alongside Dex:

```yaml
# docker-compose.yml
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
    environment:
      - USER_UID=1000
      - USER_GID=1000
```

**Pros:**
- Clean separation
- Standard Forgejo updates
- Better resource isolation

**Cons:**
- Requires Docker
- More complex setup

### Option C: External Instance

Point to existing Forgejo/Gitea instance:

```
┌─────────────────────────────────────────────────────────────┐
│              Step 3: Connect Git Server                     │
│                                                             │
│  ○ Set up local server (recommended)                       │
│  ● Connect to existing Forgejo/Gitea                       │
│                                                             │
│  Server URL:                                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ https://git.example.com                             │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Admin Token:                                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ••••••••••••••••••••••••••••••••                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│                   [ Test Connection ]                       │
└─────────────────────────────────────────────────────────────┘
```

## Implementation Phases

### Phase 1: Abstraction Layer (Week 1-2)

1. Create `gitprovider` interface
2. Implement `forgejo` provider using `code.gitea.io/sdk/gitea`
3. Implement `github` provider wrapping existing code
4. Add provider selection to configuration
5. Migrate `internal/github/sync.go` to use interface

### Phase 2: Forgejo Integration (Week 2-3)

1. Add Forgejo startup/management code
2. Implement auto-configuration (admin user, bot account, OIDC)
3. Add Forgejo health checks to API
4. Database schema migrations

### Phase 3: OIDC Provider (Week 3-4)

1. Add OIDC provider endpoints to Dex API
2. Implement token generation/validation
3. Configure Forgejo to use Dex as auth source
4. Test SSO flow end-to-end

### Phase 4: Onboarding Flow (Week 4-5)

1. Update frontend onboarding steps
2. Remove GitHub App setup steps
3. Add Forgejo setup progress UI
4. Add workspace/project creation flow

### Phase 5: Invitation System (Week 5-6)

1. Implement invitation database schema
2. Add invitation API endpoints
3. Build invitation UI
4. Email integration (optional, can use local display)

### Phase 6: Migration & Cleanup (Week 6-7)

1. Migrate existing GitHub-based installations (optional)
2. Remove GitHub-specific code (or keep as alternative provider)
3. Update documentation
4. Testing and bug fixes

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

### app.ini Template for Forgejo

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

## Security Considerations

1. **Token Storage**: All tokens stored encrypted in database
2. **OIDC Security**: Use secure signing keys, short token lifetimes
3. **Network Isolation**: Forgejo can bind to localhost only
4. **Bot Permissions**: Bot account has minimal required permissions
5. **Audit Logging**: Log all administrative actions

## Rollback Plan

If issues arise:

1. GitHub provider remains available as alternative
2. Database migrations are reversible
3. Configuration switch between providers
4. Data export from Forgejo to GitHub possible via git + API

## Success Metrics

1. **Setup Time**: < 2 minutes for complete onboarding (vs ~10 min with GitHub)
2. **Reliability**: No external API failures or rate limits
3. **Privacy**: All data remains local
4. **User Experience**: Single sign-on, no context switching

## Open Questions

1. **Forgejo Actions**: Should we enable CI/CD or keep it simple?
2. **Email**: Required for invitations, or use alternative notification?
3. **Backup**: Include Forgejo data in Dex backup strategy?
4. **Multi-user default**: Always set up for multi-user, or have single-user mode?

## References

- [Forgejo Documentation](https://forgejo.org/docs/)
- [Forgejo API](https://forgejo.org/docs/latest/user/api-usage/)
- [Gitea Go SDK](https://code.gitea.io/sdk/gitea)
- [WebAuthn Spec](https://www.w3.org/TR/webauthn/)
- [OpenID Connect](https://openid.net/connect/)
