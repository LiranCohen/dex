# Poindexter (dex) Setup Guide

This guide walks you through setting up Poindexter from scratch.

## Prerequisites

- Go 1.24+ installed
- Bun (for frontend) or Node.js 20+
- Git configured with your credentials
- Access to the services you want to use (GitHub, Anthropic, etc.)

## Quick Start

```bash
# 1. Clone and build
git clone https://github.com/lirancohen/dex.git
cd dex
go build ./cmd/dex

# 2. Build frontend
cd frontend && bun install && bun run build && cd ..

# 3. Copy and configure secrets
cp toolbelt.yaml.example toolbelt.yaml
# Edit toolbelt.yaml with your API keys (see below)

# 4. Set environment variables
export ANTHROPIC_API_KEY="your-key"
export GITHUB_TOKEN="your-token"
# ... other keys

# 5. Run
./dex -db dex.db -static ./frontend/dist -toolbelt toolbelt.yaml
```

## Step 1: API Keys Setup

Poindexter integrates with multiple services. You need at minimum:
- **Anthropic API key** (required - powers the AI sessions)
- **GitHub token** (required - for code management)

### Required Keys

#### Anthropic API Key
This powers all AI sessions. Get one at https://console.anthropic.com/

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

#### GitHub Personal Access Token
Required for repository operations, PRs, and issues.

1. Go to https://github.com/settings/tokens
2. Click "Generate new token (classic)"
3. Select scopes:
   - `repo` (full control of private repositories)
   - `workflow` (if using GitHub Actions)
4. Copy the token

```bash
export GITHUB_TOKEN="ghp_..."
```

### Optional Service Keys

These enable additional cloud services for deployments:

| Service | Purpose | Get Key At |
|---------|---------|------------|
| Fly.io | App deployment | https://fly.io/docs/reference/api-tokens/ |
| Cloudflare | DNS, CDN | https://dash.cloudflare.com/profile/api-tokens |
| Neon | Serverless Postgres | https://console.neon.tech/app/settings/api-keys |
| Upstash | Redis & queues | https://console.upstash.com/account/api |
| Resend | Email sending | https://resend.com/api-keys |
| BetterStack | Monitoring | https://betterstack.com/docs/uptime/api/getting-started/ |
| Doppler | Secrets management | https://docs.doppler.com/docs/api |
| fal.ai | Image/video AI | https://fal.ai/dashboard/keys |

## Step 2: Configure toolbelt.yaml

Copy the example and fill in your keys:

```bash
cp toolbelt.yaml.example toolbelt.yaml
```

Edit `toolbelt.yaml`:

```yaml
# Minimum required configuration
github:
  token: ${GITHUB_TOKEN}
  default_org: your-github-username  # or your org name

anthropic:
  api_key: ${ANTHROPIC_API_KEY}

# Optional - add as needed
fly:
  token: ${FLY_API_TOKEN}
  default_region: ord

cloudflare:
  api_token: ${CLOUDFLARE_API_TOKEN}
  account_id: ${CLOUDFLARE_ACCOUNT_ID}

# ... other services
```

The `${VAR_NAME}` syntax references environment variables, so you don't store secrets in the file directly.

## Step 3: Initialize the Database

The database initializes automatically on first run:

```bash
./dex -db dex.db
```

This creates `dex.db` with all required tables.

## Step 4: Generate Your BIP39 Passphrase

Poindexter uses a 24-word passphrase for authentication. This is your master credential - **store it securely**.

### Option A: Generate via UI (Recommended)
1. Open http://localhost:8080 in your browser
2. Click "Generate New Passphrase"
3. **Write down all 24 words in order**
4. Store them securely (password manager, safe, etc.)

### Option B: Generate via CLI
```bash
# Using bip39 tool (if installed)
bip39 generate 24

# Or use the dex utility
./dex generate-passphrase
```

### Security Notes
- This passphrase is your **only** authentication method
- Anyone with this phrase can access your dex instance
- There is no password reset - lose it and you're locked out
- Consider using a hardware wallet or secure vault for storage

## Step 5: First Login

1. Open http://localhost:8080
2. Enter your 24-word passphrase
3. You're now authenticated!

The passphrase derives an Ed25519 keypair client-side. Only the public key is sent to the server.

## Step 6: Verify Services

Test that your API keys are working:

```bash
curl http://localhost:8080/api/v1/toolbelt/status
```

Or use the UI to check service status on the dashboard.

## Running in Production

### With Tailscale (Recommended)

Poindexter is designed to run on your local network via Tailscale:

```bash
# Get Tailscale HTTPS certs
tailscale cert dex.your-tailnet.ts.net

# Run with TLS
./dex \
  -addr :443 \
  -cert dex.your-tailnet.ts.net.crt \
  -key dex.your-tailnet.ts.net.key \
  -db /opt/dex/dex.db \
  -static ./frontend/dist \
  -toolbelt /opt/dex/toolbelt.yaml
```

### As a systemd Service

Create `/etc/systemd/system/dex.service`:

```ini
[Unit]
Description=Poindexter AI Orchestration
After=network.target

[Service]
Type=simple
User=dex
WorkingDirectory=/opt/dex
ExecStart=/opt/dex/dex -db /opt/dex/dex.db -static /opt/dex/frontend/dist -toolbelt /opt/dex/toolbelt.yaml
Restart=always
RestartSec=5

# Environment variables for secrets
EnvironmentFile=/opt/dex/.env

[Install]
WantedBy=multi-user.target
```

Create `/opt/dex/.env`:
```bash
ANTHROPIC_API_KEY=sk-ant-...
GITHUB_TOKEN=ghp_...
# ... other keys
```

```bash
sudo systemctl enable dex
sudo systemctl start dex
```

## Directory Structure

```
/opt/dex/                    # Installation root
├── dex                      # Binary
├── dex.db                   # SQLite database
├── toolbelt.yaml           # Service configuration
├── .env                     # Environment secrets
├── frontend/
│   └── dist/               # Built frontend
└── worktrees/              # Git worktrees (created automatically)
    ├── project-a/
    │   └── task-abc123/
    └── project-b/
        └── task-def456/
```

## Troubleshooting

### "GitHub ping failed"
- Check your GITHUB_TOKEN is valid
- Ensure token has `repo` scope
- Check for typos in token

### "Anthropic API error"
- Verify ANTHROPIC_API_KEY is correct
- Check your Anthropic account has API access
- Ensure sufficient credits/quota

### "Database locked"
- Only run one instance of dex at a time
- Check for orphaned processes: `ps aux | grep dex`

### "Cannot connect from mobile"
- Ensure you're on the same Tailscale network
- Check firewall allows the port
- Verify TLS certificates are valid

## Next Steps

1. **Create a project**: Link a GitHub repository to track
2. **Create a task**: Describe what you want to build
3. **Start a session**: Let the AI work on your task
4. **Review and approve**: Check PRs and provide feedback

See [USAGE.md](./USAGE.md) for detailed usage instructions.
