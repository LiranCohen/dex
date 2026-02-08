# Dex (Poindexter) - Claude Code Guide

## Overview

**Poindexter** (nicknamed "Dex") is your smart and organized AI orchestration assistant.

This repository contains the **Dex Client** - the main application binary used for both HQ (headquarters) and Outpost nodes.

### Architecture

For complete architecture details, see **[dex-saas/docs/ARCHITECTURE.md](~/src/dex-saas/docs/ARCHITECTURE.md)**

Quick summary:
- **HQ**: Your main Poindexter node (exactly 1 per user, runs API/frontend/sessions)
- **Outposts**: Additional nodes for services/workers (optional, can be public or mesh-only)
- **Central**: SaaS coordination service in ~/src/dex-saas (account management, mesh networking)
- **Edge**: Public ingress servers for public Outposts (part of dex-saas)

## Project Structure

```
cmd/dex/           # Main binary entry point
internal/
├── api/           # HTTP API (Echo framework)
├── auth/          # Authentication (passkeys, JWT)
├── db/            # SQLite database (GORM)
├── forgejo/       # Embedded Forgejo manager
├── mesh/          # Mesh networking (tsnet)
├── session/       # AI session orchestration
├── worker/        # Distributed worker management
└── oidc/          # OIDC provider for SSO
frontend/          # React SPA (Vite + TypeScript)
scripts/           # Install and setup scripts
.github/workflows/ # CI/CD pipelines
```

## Building

```bash
# Build Go binary
go build ./cmd/dex

# Build frontend (required for embedded assets)
cd frontend && bun install && bun run build
```

## Releasing

**IMPORTANT: Releases are automated via GitHub Actions.**

To release a new version:
1. Commit and push changes to master
2. Create and push a version tag: `git tag -a v0.1.X -m "message" && git push origin v0.1.X`
3. CI automatically builds binaries (linux/darwin, amd64/arm64) and creates the GitHub release

**Do NOT manually build release binaries** - the CI workflow handles this on tag push.

The install script (`scripts/install.sh`) downloads binaries from GitHub releases.

## Testing

```bash
# Run all tests
go test ./...

# Run with short flag (skips slow tests)
go test ./... -short

# E2E tests (requires GITHUB_TOKEN and DEX_E2E_ENABLED=true)
./scripts/run-e2e-tests.sh
```

## Key Components

### Enrollment Flow
1. User gets enrollment key from Central (dex-saas)
2. `dex enroll --key dexkey-xxx` registers with Central
3. Creates `/opt/dex/config.json` with namespace, public URL, tunnel config
4. `dex start` loads config and connects to mesh/tunnel

### Forgejo SSO
- Forgejo authenticates users via HQ's OIDC provider
- OAuth secret generated during bootstrap
- `SetupSSOProvider()` configures Forgejo after HTTP server starts
- Uses passkey authentication (no username/password)

### Configuration
- Enrollment config: `/opt/dex/config.json`
- Database: `dex.db` (SQLite)
- Secrets stored encrypted in DB when master key is configured

## Common Issues

### "dex-worker binary not found"
Workers are disabled by default (`MaxLocalWorkers: 0`). The dex-worker binary is not distributed with the install script.

### Forgejo SSO not working
Ensure `public_url` is set in config.json. The SSO setup requires the OIDC discovery endpoint to be reachable.

## Related Projects

- **dex-saas** (`~/src/dex-saas`): SaaS service containing:
  - **Central**: User account management, enrollment, mesh coordination
  - **Edge**: Public ingress servers (SNI router, tunnels to public Outposts)
