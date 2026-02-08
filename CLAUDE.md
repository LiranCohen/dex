# Dex (Poindexter) - Claude Code Guide

## Overview

Dex is a single-user AI orchestration platform that manages concurrent Claude Code sessions. It includes:
- **HQ**: Main server with API, frontend, session management
- **Forgejo**: Embedded git server for code hosting
- **Mesh**: Tailscale-based networking via dex-saas Central
- **Tunnel**: Public ingress via Edge server

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

- **dex-saas** (`~/src/dex-saas`): Central coordination server and Edge ingress
  - Central: User accounts, enrollment, mesh coordination
  - Edge: SNI router, tunnels to HQs
