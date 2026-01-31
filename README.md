# Poindexter (dex)

> **Disk is state. Git is memory. Fresh context each iteration.**

Poindexter is a single-user AI orchestration platform that manages up to 25 concurrent Claude Code sessions on your local machine. It decomposes complex tasks, assigns specialized "hats" to AI sessions, manages isolated git worktrees, and orchestrates deployments across multiple cloud services.

## Installation

### Remote Server Install (Recommended)

Install on a VPS or cloud server with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/lirancohen/dex/master/scripts/install.sh | sudo bash
```

This will:
1. Install Go, cloudflared, and Tailscale
2. Build dex from source
3. Create a temporary Cloudflare tunnel for setup
4. Guide you through access method selection (Tailscale or Cloudflare)
5. Set up passkey authentication and API keys
6. Configure systemd service

### Upgrade Existing Installation

Simply re-run the installer - it will detect the existing installation and upgrade while preserving your data:

```bash
curl -fsSL https://raw.githubusercontent.com/lirancohen/dex/master/scripts/install.sh | sudo bash
```

### Fresh Install (Wipe and Reinstall)

To completely wipe all data and start fresh:

```bash
# Interactive (prompts for confirmation)
curl -fsSL https://raw.githubusercontent.com/lirancohen/dex/master/scripts/install.sh | sudo bash -s -- --fresh

# Non-interactive (for scripts)
curl -fsSL https://raw.githubusercontent.com/lirancohen/dex/master/scripts/install.sh | sudo bash -s -- --fresh --yes
```

**Warning:** This permanently deletes all data including:
- Database (users, tasks, credentials)
- Registered passkeys
- API keys (GitHub, Anthropic)
- All repositories and worktrees

### Local Development

```bash
# Build
go build ./cmd/dex
cd frontend && bun install && bun run build && cd ..

# Configure secrets
./scripts/setup-secrets.sh
source .env

# Copy toolbelt config
cp toolbelt.yaml.example toolbelt.yaml

# Run
./dex -static ./frontend/dist -toolbelt toolbelt.yaml -worktree-base ./worktrees -repos-dir ./repos
```

Open http://localhost:8080 and register a passkey (Face ID, Touch ID, or security key).

## Service Management

```bash
# Check status
sudo systemctl status dex

# Restart
sudo systemctl restart dex

# View logs
sudo journalctl -u dex -f

# Stop
sudo systemctl stop dex
```

## Documentation

- **[docs/SETUP.md](docs/SETUP.md)** - Full setup and configuration guide
- **[docs/USAGE.md](docs/USAGE.md)** - How to use Poindexter

## Requirements

- Go 1.24+
- Bun (or Node.js 20+)
- Git
- API keys: Anthropic (required), GitHub (required), others optional

## Minimum Secrets

```bash
export ANTHROPIC_API_KEY="sk-ant-..."  # Powers AI sessions
export GITHUB_TOKEN="ghp_..."           # For repo operations
```

## Projects and Repositories

Dex manages git repositories for your projects. During setup, a default workspace is created at `/opt/dex/repos/dex-workspace`.

### Creating Projects via API

```bash
# Create project with new local repository
curl -X POST https://your-dex-url/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name": "My App", "create_repo": true}'

# Create project with new GitHub repository
curl -X POST https://your-dex-url/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name": "My App", "create_repo": true, "github_create": true, "github_private": true}'

# Create project by cloning existing repo
curl -X POST https://your-dex-url/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name": "My App", "clone_url": "https://github.com/user/repo.git"}'

# Create project pointing to existing local repo
curl -X POST https://your-dex-url/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name": "My App", "repo_path": "/path/to/existing/repo"}'
```

## Directory Structure

When installed via the install script:

```
/opt/dex/
├── dex              # Main binary
├── dex-setup        # Setup wizard binary
├── frontend/        # Built frontend assets
├── dex.db           # SQLite database
├── secrets.json     # API keys (GitHub, Anthropic)
├── repos/           # Git repositories
│   └── dex-workspace/  # Default workspace (created on setup)
├── worktrees/       # Git worktrees for active tasks
├── setup-complete   # Marker file (created after setup)
├── permanent-url    # Your dex URL
└── access-method    # "tailscale" or "cloudflare"
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Frontend (React)  ←──WebSocket──→  API (Go/Echo)          │
│       ↓                                    ↓                │
│  Auth (Passkey/JWT)            SQLite │ Git Worktrees      │
│                                        │       ↓            │
│                                 Orchestrator (Ralph Loop)   │
│                                        ↓                    │
│                          Toolbelt (11 Cloud Services)       │
└─────────────────────────────────────────────────────────────┘
```

## E2E Tests

```bash
# Just connection test
export GITHUB_TOKEN="ghp_..."
export DEX_E2E_ENABLED=true
go test -v ./internal/e2e -run TestGitHubConnectionOnly

# Full push + PR test (requires a test repo)
export DEX_E2E_REPO="owner/test-repo"
./scripts/run-e2e-tests.sh
```

## License

Private - All rights reserved.
