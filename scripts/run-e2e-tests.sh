#!/bin/bash
# run-e2e-tests.sh - Run end-to-end tests for Poindexter
set -e

echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║           Poindexter E2E Tests                                ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""

# Check for required environment variables
MISSING=""

if [ -z "$GITHUB_TOKEN" ]; then
    MISSING="$MISSING GITHUB_TOKEN"
fi

if [ -n "$MISSING" ]; then
    echo "ERROR: Missing required environment variables:$MISSING"
    echo ""
    echo "Set them before running:"
    echo "  export GITHUB_TOKEN=ghp_..."
    echo ""
    echo "Or source your .env file:"
    echo "  source .env"
    echo ""
    exit 1
fi

# Default test repo (user can override)
if [ -z "$DEX_E2E_REPO" ]; then
    echo "NOTE: DEX_E2E_REPO not set."
    echo "Set it to run the full push test:"
    echo "  export DEX_E2E_REPO=owner/test-repo"
    echo ""
    echo "Running connection test only..."
    echo ""
fi

# Enable e2e tests
export DEX_E2E_ENABLED=true

echo "Running E2E tests..."
echo ""

# Run tests
if [ -n "$DEX_E2E_REPO" ]; then
    echo "Test repo: $DEX_E2E_REPO"
    echo ""
    go test -v ./internal/e2e -run TestGitHubPushE2E -timeout 5m
else
    go test -v ./internal/e2e -run TestGitHubConnectionOnly -timeout 1m
fi

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "E2E tests completed successfully!"
echo "═══════════════════════════════════════════════════════════════"
