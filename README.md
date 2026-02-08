# Poindexter (dex)

> **Disk is state. Git is memory. Fresh context each iteration.**

**Poindexter** (nicknamed "Dex") is your smart and organized AI orchestration assistant.

This repository contains the **Dex Client** - the application binary used for both HQ (headquarters) and Outpost nodes. Poindexter manages up to 25 concurrent Claude Code sessions, decomposes complex tasks, assigns specialized "hats" to AI sessions, manages isolated git worktrees, and orchestrates deployments across multiple cloud services.

> **Architecture**: See [dex-saas/docs/ARCHITECTURE.md](https://github.com/lirancohen/dex-saas/blob/master/docs/ARCHITECTURE.md) for complete system architecture.

**Quick summary:**
- **HQ**: Main node (1 per user) - runs API, frontend, AI sessions
- **Outposts**: Optional worker nodes - can be public or mesh-only
- **Central**: Coordination service ([dex-saas](https://github.com/lirancohen/dex-saas)) - account/mesh management

## Installation

### Remote Server Install (Recommended)

Install on a VPS or cloud server with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/lirancohen/dex/master/scripts/install.sh | sudo bash
```

This will:
1. Install Go and required dependencies
2. Build dex from source
3. Enroll with Central and connect to mesh (dexnet)
4. Set up passkey authentication and API keys
5. Configure systemd service

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
./dex -static ./frontend/dist -toolbelt toolbelt.yaml -base-dir .
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
├── config.json      # Enrollment config (namespace, mesh settings)
└── mesh/            # Mesh state directory
```

## HQ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Frontend (React)  ←──WebSocket──→  API (Go/Echo)          │
│       ↓                                    ↓                │
│  Auth (Passkey/JWT)            SQLite │ Git Worktrees      │
│                                        │       ↓            │
│                                 Orchestrator (Ralph Loop)   │
│                                        ↓                    │
│                          Toolbelt (11 Cloud Services)       │
│                                        ↓                    │
│                    Mesh (dexnet) ← Outposts                  │
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
