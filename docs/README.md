# Poindexter Documentation

## For Users

| Document | Description |
|----------|-------------|
| [SETUP.md](SETUP.md) | Installation, API keys, first-time configuration |
| [USAGE.md](USAGE.md) | How to use tasks, hats, workflows, and the API |

## Quick Links

### Getting API Keys

| Service | Required | URL |
|---------|----------|-----|
| Anthropic | Yes | https://console.anthropic.com/ |
| GitHub | Yes | https://github.com/settings/tokens |
| Fly.io | No | https://fly.io/user/personal_access_tokens |
| Cloudflare | No | https://dash.cloudflare.com/profile/api-tokens |
| Neon | No | https://console.neon.tech/app/settings/api-keys |
| Upstash | No | https://console.upstash.com/account/api |
| Resend | No | https://resend.com/api-keys |
| BetterStack | No | https://betterstack.com/docs/uptime/api/getting-started/ |
| fal.ai | No | https://fal.ai/dashboard/keys |

### Commands Cheatsheet

```bash
# Build everything
go build ./cmd/dex
cd frontend && bun run build && cd ..

# Run server
./dex -static ./frontend/dist -toolbelt toolbelt.yaml

# Run tests
go test ./...

# E2E tests
export DEX_E2E_ENABLED=true
go test -v ./internal/e2e

# Interactive secrets setup
./scripts/setup-secrets.sh
```

### Important Files

```
dex                      # Main binary (after build)
dex.db                   # SQLite database (auto-created)
toolbelt.yaml            # Service API keys
.env                     # Environment secrets
frontend/dist/           # Built frontend
```

### Hat Quick Reference

| Hat | Use For |
|-----|---------|
| planner | Breaking down epics into tasks |
| architect | Designing interfaces before coding |
| implementer | Writing code |
| reviewer | Code review |
| tester | Writing and running tests |
| debugger | Fixing bugs |
| documenter | Writing docs (terminal) |
| devops | Deploying (terminal) |
| conflict_manager | Resolving merge conflicts (terminal) |
