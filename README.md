# Poindexter (dex)

> **Disk is state. Git is memory. Fresh context each iteration.**

Poindexter is a single-user AI orchestration platform that manages up to 25 concurrent Claude Code sessions on your local machine. It decomposes complex tasks, assigns specialized "hats" to AI sessions, manages isolated git worktrees, and orchestrates deployments across multiple cloud services.

## Quick Start

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
./dex -static ./frontend/dist -toolbelt toolbelt.yaml
```

Open http://localhost:8080 and register a passkey (Face ID, Touch ID, or security key).

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
