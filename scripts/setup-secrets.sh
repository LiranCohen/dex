#!/bin/bash
# setup-secrets.sh - Interactive script to configure Poindexter secrets
set -e

echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║           Poindexter Secrets Setup                            ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""

# Determine output file
ENV_FILE="${1:-.env}"
echo "This will create/update: $ENV_FILE"
echo ""

# Check if file exists
if [ -f "$ENV_FILE" ]; then
    read -p "File exists. Overwrite? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

# Start fresh
echo "# Poindexter Environment Variables" > "$ENV_FILE"
echo "# Generated: $(date)" >> "$ENV_FILE"
echo "" >> "$ENV_FILE"

# Required keys
echo "═══════════════════════════════════════════════════════════════"
echo "REQUIRED KEYS"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Anthropic
echo "1. ANTHROPIC API KEY"
echo "   Get one at: https://console.anthropic.com/"
echo "   This powers all AI sessions."
read -p "   Enter ANTHROPIC_API_KEY: " ANTHROPIC_KEY
if [ -n "$ANTHROPIC_KEY" ]; then
    echo "ANTHROPIC_API_KEY=$ANTHROPIC_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   ⚠ Skipped (required for AI sessions)"
fi
echo ""

# GitHub
echo "2. GITHUB TOKEN"
echo "   Get one at: https://github.com/settings/tokens"
echo "   Select 'Generate new token (classic)' with 'repo' scope."
read -p "   Enter GITHUB_TOKEN: " GITHUB_KEY
if [ -n "$GITHUB_KEY" ]; then
    echo "GITHUB_TOKEN=$GITHUB_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   ⚠ Skipped (required for GitHub operations)"
fi
echo ""

# Optional keys
echo "═══════════════════════════════════════════════════════════════"
echo "OPTIONAL KEYS (press Enter to skip)"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Fly.io
echo "3. FLY.IO TOKEN (for deployments)"
echo "   Get one at: https://fly.io/user/personal_access_tokens"
read -p "   Enter FLY_API_TOKEN: " FLY_KEY
if [ -n "$FLY_KEY" ]; then
    echo "FLY_API_TOKEN=$FLY_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# Cloudflare
echo "4. CLOUDFLARE (for DNS/CDN)"
echo "   Get tokens at: https://dash.cloudflare.com/profile/api-tokens"
read -p "   Enter CLOUDFLARE_API_TOKEN: " CF_TOKEN
if [ -n "$CF_TOKEN" ]; then
    echo "CLOUDFLARE_API_TOKEN=$CF_TOKEN" >> "$ENV_FILE"
    read -p "   Enter CLOUDFLARE_ACCOUNT_ID: " CF_ACCOUNT
    if [ -n "$CF_ACCOUNT" ]; then
        echo "CLOUDFLARE_ACCOUNT_ID=$CF_ACCOUNT" >> "$ENV_FILE"
    fi
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# Neon
echo "5. NEON (serverless Postgres)"
echo "   Get key at: https://console.neon.tech/app/settings/api-keys"
read -p "   Enter NEON_API_KEY: " NEON_KEY
if [ -n "$NEON_KEY" ]; then
    echo "NEON_API_KEY=$NEON_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# Upstash
echo "6. UPSTASH (Redis & queues)"
echo "   Get keys at: https://console.upstash.com/account/api"
read -p "   Enter UPSTASH_EMAIL: " UPSTASH_EMAIL
if [ -n "$UPSTASH_EMAIL" ]; then
    echo "UPSTASH_EMAIL=$UPSTASH_EMAIL" >> "$ENV_FILE"
    read -p "   Enter UPSTASH_API_KEY: " UPSTASH_KEY
    if [ -n "$UPSTASH_KEY" ]; then
        echo "UPSTASH_API_KEY=$UPSTASH_KEY" >> "$ENV_FILE"
    fi
    read -p "   Enter UPSTASH_QSTASH_TOKEN (optional): " UPSTASH_QSTASH
    if [ -n "$UPSTASH_QSTASH" ]; then
        echo "UPSTASH_QSTASH_TOKEN=$UPSTASH_QSTASH" >> "$ENV_FILE"
    fi
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# Resend
echo "7. RESEND (email)"
echo "   Get key at: https://resend.com/api-keys"
read -p "   Enter RESEND_API_KEY: " RESEND_KEY
if [ -n "$RESEND_KEY" ]; then
    echo "RESEND_API_KEY=$RESEND_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# BetterStack
echo "8. BETTER STACK (monitoring)"
echo "   Get token at: https://betterstack.com/docs/uptime/api/getting-started/"
read -p "   Enter BETTER_STACK_API_TOKEN: " BETTERSTACK_KEY
if [ -n "$BETTERSTACK_KEY" ]; then
    echo "BETTER_STACK_API_TOKEN=$BETTERSTACK_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# Doppler
echo "9. DOPPLER (secrets management)"
echo "   Get token at: https://docs.doppler.com/docs/api"
read -p "   Enter DOPPLER_TOKEN: " DOPPLER_KEY
if [ -n "$DOPPLER_KEY" ]; then
    echo "DOPPLER_TOKEN=$DOPPLER_KEY" >> "$ENV_FILE"
    echo "   ✓ Saved"
else
    echo "   - Skipped"
fi
echo ""

# fal.ai
echo "10. FAL.AI (image/video AI)"
echo "    Get key at: https://fal.ai/dashboard/keys"
read -p "    Enter FAL_API_KEY: " FAL_KEY
if [ -n "$FAL_KEY" ]; then
    echo "FAL_API_KEY=$FAL_KEY" >> "$ENV_FILE"
    echo "    ✓ Saved"
else
    echo "    - Skipped"
fi
echo ""

# Secure the file
chmod 600 "$ENV_FILE"

echo "═══════════════════════════════════════════════════════════════"
echo "SETUP COMPLETE"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "Saved to: $ENV_FILE"
echo "File permissions set to 600 (owner read/write only)"
echo ""
echo "To use these secrets:"
echo ""
echo "  Option 1: Source before running dex"
echo "    source $ENV_FILE && ./dex"
echo ""
echo "  Option 2: Use with systemd (recommended for production)"
echo "    Add to unit file: EnvironmentFile=$PWD/$ENV_FILE"
echo ""
echo "  Option 3: Export manually"
echo "    export \$(cat $ENV_FILE | xargs)"
echo ""
echo "Next steps:"
echo "  1. Copy toolbelt.yaml.example to toolbelt.yaml"
echo "  2. Build: go build ./cmd/dex"
echo "  3. Run: source $ENV_FILE && ./dex -static ./frontend/dist"
echo ""
