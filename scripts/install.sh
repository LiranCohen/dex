#!/bin/bash
#
# Poindexter Magic Installer
#
# Usage:
#   curl -fsSL https://dex.example.com/install.sh | bash
#
# Flow:
#   1. Shows QR code → scan to join Tailscale (or login)
#   2. Shows QR code → scan to enter API keys & get passphrase
#   3. Done! Dex is running at https://dex.your-tailnet.ts.net
#
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Configuration
DEX_VERSION="${DEX_VERSION:-latest}"
DEX_HOSTNAME="${DEX_HOSTNAME:-dex}"
DEX_PORT="${DEX_PORT:-8080}"
DEX_INSTALL_DIR="${DEX_INSTALL_DIR:-/opt/dex}"
SETUP_PORT="${SETUP_PORT:-9999}"

# State
CLEANUP_PIDS=()
CLEANUP_FILES=()

cleanup() {
    for pid in "${CLEANUP_PIDS[@]:-}"; do
        kill "$pid" 2>/dev/null || true
    done
    for file in "${CLEANUP_FILES[@]:-}"; do
        rm -f "$file" 2>/dev/null || true
    done
    tailscale serve reset 2>/dev/null || true
}
trap cleanup EXIT

log() { echo -e "${BLUE}▸${NC} $1"; }
success() { echo -e "${GREEN}✓${NC} $1"; }
warn() { echo -e "${YELLOW}!${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; exit 1; }

print_banner() {
    clear
    cat << 'EOF'

    ██████╗ ███████╗██╗  ██╗
    ██╔══██╗██╔════╝╚██╗██╔╝
    ██║  ██║█████╗   ╚███╔╝
    ██║  ██║██╔══╝   ██╔██╗
    ██████╔╝███████╗██╔╝ ██╗
    ╚═════╝ ╚══════╝╚═╝  ╚═╝

    P O I N D E X T E R
    AI Orchestration Platform

EOF
    echo -e "    ${DIM}Disk is state. Git is memory.${NC}"
    echo ""
}

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH" ;;
    esac
    case "$OS" in
        linux|darwin) ;;
        *) error "Unsupported OS: $OS" ;;
    esac
    PLATFORM="${OS}_${ARCH}"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${YELLOW}This installer needs sudo access.${NC}"
        echo "Re-running with sudo..."
        exec sudo -E bash "$0" "$@"
    fi
}

install_qrencode() {
    if command -v qrencode &>/dev/null; then return; fi
    log "Installing qrencode..."
    if command -v apt-get &>/dev/null; then
        apt-get update -qq && apt-get install -y -qq qrencode >/dev/null
    elif command -v yum &>/dev/null; then
        yum install -y -q qrencode >/dev/null
    elif command -v pacman &>/dev/null; then
        pacman -Sy --noconfirm qrencode >/dev/null
    elif command -v brew &>/dev/null; then
        brew install qrencode >/dev/null
    else
        error "Cannot install qrencode. Please install it manually."
    fi
    success "qrencode installed"
}

install_jq() {
    if command -v jq &>/dev/null; then return; fi
    log "Installing jq..."
    if command -v apt-get &>/dev/null; then
        apt-get install -y -qq jq >/dev/null
    elif command -v yum &>/dev/null; then
        yum install -y -q jq >/dev/null
    elif command -v pacman &>/dev/null; then
        pacman -Sy --noconfirm jq >/dev/null
    elif command -v brew &>/dev/null; then
        brew install jq >/dev/null
    fi
}

install_go() {
    # Check common locations for Go
    export PATH="$PATH:/usr/local/go/bin"

    if command -v go &>/dev/null; then
        success "Go already installed: $(go version | awk '{print $3}')"
        return
    fi
    log "Installing Go..."

    local go_version="1.24.3"
    local go_arch="$ARCH"
    local go_os="$OS"

    # Download and install Go
    local go_tarball="go${go_version}.${go_os}-${go_arch}.tar.gz"
    curl -fsSL "https://go.dev/dl/${go_tarball}" -o "/tmp/${go_tarball}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${go_tarball}"
    rm -f "/tmp/${go_tarball}"

    # Add to PATH for this session
    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=/root/go
    export PATH=$PATH:$GOPATH/bin

    success "Go installed: $(go version | awk '{print $3}')"
}

build_dex() {
    log "Building dex from source..."

    # Ensure Go is in PATH
    export PATH="$PATH:/usr/local/go/bin"
    export GOPATH="${GOPATH:-/root/go}"
    export PATH="$PATH:$GOPATH/bin"
    mkdir -p "$GOPATH/bin"

    # Use GOPROXY=direct to bypass proxy cache (case sensitivity fix)
    GOPROXY=direct go install github.com/lirancohen/dex/cmd/dex@latest
    GOPROXY=direct go install github.com/lirancohen/dex/cmd/dex-setup@latest

    # Copy to install dir
    mkdir -p "$DEX_INSTALL_DIR"
    cp "$GOPATH/bin/dex" "$DEX_INSTALL_DIR/dex"
    cp "$GOPATH/bin/dex-setup" "$DEX_INSTALL_DIR/dex-setup"

    success "Built dex and dex-setup"
}

install_tailscale() {
    if command -v tailscale &>/dev/null; then
        success "Tailscale already installed"
        return
    fi
    log "Installing Tailscale..."
    curl -fsSL https://tailscale.com/install.sh | sh
    if command -v systemctl &>/dev/null; then
        systemctl enable --now tailscaled >/dev/null 2>&1 || true
    fi
    success "Tailscale installed"
}

show_qr() {
    local url="$1"
    local title="$2"

    echo ""
    echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${BOLD}📱 $title${NC}"
    echo ""
    qrencode -t ANSI256 -m 2 "$url"
    echo ""
    echo -e "  ${DIM}Or open:${NC} ${CYAN}$url${NC}"
    echo ""
    echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

authenticate_tailscale() {
    # Check if already connected
    if tailscale status --json 2>/dev/null | jq -e '.BackendState == "Running"' >/dev/null 2>&1; then
        success "Already connected to Tailscale"
        return 0
    fi

    log "Connecting to Tailscale..."

    # Create a temp file to capture the auth URL
    local auth_log
    auth_log=$(mktemp)
    CLEANUP_FILES+=("$auth_log")

    # Start tailscale up in background
    tailscale up --hostname="$DEX_HOSTNAME" --ssh 2>&1 | tee "$auth_log" &
    local ts_pid=$!
    CLEANUP_PIDS+=($ts_pid)

    # Wait for auth URL
    local auth_url=""
    for _ in {1..30}; do
        if grep -q "https://login.tailscale.com" "$auth_log" 2>/dev/null; then
            auth_url=$(grep -o 'https://login.tailscale.com[^ ]*' "$auth_log" | head -1)
            break
        fi
        # Check if already authenticated
        if tailscale status --json 2>/dev/null | jq -e '.BackendState == "Running"' >/dev/null 2>&1; then
            success "Connected to Tailscale"
            return 0
        fi
        sleep 1
    done

    if [ -z "$auth_url" ]; then
        # One more check
        if tailscale status --json 2>/dev/null | jq -e '.BackendState == "Running"' >/dev/null 2>&1; then
            success "Connected to Tailscale"
            return 0
        fi
        error "Could not get Tailscale auth URL"
    fi

    show_qr "$auth_url" "SCAN TO JOIN TAILSCALE"
    echo -e "  ${YELLOW}Waiting for authentication...${NC}"

    # Wait for connection
    while ! tailscale status --json 2>/dev/null | jq -e '.BackendState == "Running"' >/dev/null 2>&1; do
        sleep 2
    done

    local dns_name
    dns_name=$(tailscale status --json | jq -r '.Self.DNSName' | sed 's/\.$//')
    echo ""
    success "Connected as ${BOLD}$dns_name${NC}"
}

run_setup_wizard() {
    log "Starting setup wizard..."

    # Get our DNS name
    local dns_name
    dns_name=$(tailscale status --json | jq -r '.Self.DNSName' | sed 's/\.$//')
    local setup_url="https://${dns_name}"

    # Start the setup wizard
    local secrets_file="/tmp/dex-setup-secrets.json"
    local done_file="/tmp/dex-setup-complete"
    CLEANUP_FILES+=("$secrets_file" "$done_file")

    rm -f "$secrets_file" "$done_file"

    # Run dex-setup
    local setup_bin="$DEX_INSTALL_DIR/dex-setup"
    if [ ! -f "$setup_bin" ]; then
        error "Setup wizard not found at $setup_bin"
    fi

    "$setup_bin" \
        -addr "127.0.0.1:$SETUP_PORT" \
        -output "$secrets_file" \
        -done "$done_file" \
        -url "$setup_url" &
    local wizard_pid=$!
    CLEANUP_PIDS+=($wizard_pid)

    sleep 1

    # Expose via tailscale serve
    tailscale serve --bg --https=443 "http://127.0.0.1:$SETUP_PORT"

    show_qr "$setup_url" "SCAN TO COMPLETE SETUP"
    echo -e "  ${YELLOW}Enter your API keys and save your passphrase...${NC}"

    # Wait for completion
    while [ ! -f "$done_file" ]; do
        # Check if wizard is still running
        if ! kill -0 "$wizard_pid" 2>/dev/null; then
            if [ -f "$done_file" ]; then
                break
            fi
            error "Setup wizard exited unexpectedly"
        fi
        sleep 2
    done

    # Stop the wizard serve
    tailscale serve reset

    # Read secrets
    if [ ! -f "$secrets_file" ]; then
        error "Secrets file not found"
    fi

    ANTHROPIC_API_KEY=$(jq -r '.anthropic' "$secrets_file")
    GITHUB_TOKEN=$(jq -r '.github' "$secrets_file")

    echo ""
    success "Configuration received!"
}

install_frontend() {
    log "Installing frontend..."

    # Check if bun is available
    if ! command -v bun &>/dev/null; then
        log "Installing bun..."
        curl -fsSL https://bun.sh/install | bash
        export PATH="$HOME/.bun/bin:$PATH"
    fi

    # Clone repo to get frontend source
    local tmp_repo="/tmp/dex-repo"
    rm -rf "$tmp_repo"
    git clone --depth=1 https://github.com/lirancohen/dex.git "$tmp_repo"

    # Build frontend
    cd "$tmp_repo/frontend"
    bun install
    bun run build
    cd - >/dev/null

    # Copy to install dir
    mkdir -p "$DEX_INSTALL_DIR"
    cp -r "$tmp_repo/frontend/dist" "$DEX_INSTALL_DIR/frontend"
    rm -rf "$tmp_repo"

    success "Frontend installed"
}

create_config() {
    log "Creating configuration..."

    # Create .env
    cat > "$DEX_INSTALL_DIR/.env" << EOF
ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
GITHUB_TOKEN=${GITHUB_TOKEN}
EOF
    chmod 600 "$DEX_INSTALL_DIR/.env"

    # Create toolbelt.yaml
    cat > "$DEX_INSTALL_DIR/toolbelt.yaml" << 'EOF'
github:
  token: ${GITHUB_TOKEN}

anthropic:
  api_key: ${ANTHROPIC_API_KEY}
EOF

    success "Configuration saved"
}

create_systemd_service() {
    if ! command -v systemctl &>/dev/null; then
        warn "systemd not found, skipping service creation"
        return
    fi

    log "Creating systemd service..."

    cat > /etc/systemd/system/dex.service << EOF
[Unit]
Description=Poindexter AI Orchestration
After=network.target tailscaled.service
Wants=tailscaled.service

[Service]
Type=simple
User=root
WorkingDirectory=${DEX_INSTALL_DIR}
EnvironmentFile=${DEX_INSTALL_DIR}/.env
ExecStart=${DEX_INSTALL_DIR}/dex \\
    -db ${DEX_INSTALL_DIR}/dex.db \\
    -static ${DEX_INSTALL_DIR}/frontend \\
    -toolbelt ${DEX_INSTALL_DIR}/toolbelt.yaml \\
    -addr 127.0.0.1:${DEX_PORT}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable dex >/dev/null 2>&1
    systemctl start dex

    success "Service created and started"
}

configure_tailscale_serve() {
    log "Configuring Tailscale Serve..."

    # Wait for dex to be ready
    local attempts=0
    while ! curl -s "http://127.0.0.1:${DEX_PORT}/api/v1/system/status" >/dev/null 2>&1; do
        sleep 1
        attempts=$((attempts + 1))
        if [ $attempts -gt 30 ]; then
            warn "Dex not responding, continuing anyway"
            break
        fi
    done

    # Configure permanent serve
    tailscale serve --bg --https=443 "http://127.0.0.1:${DEX_PORT}"

    success "HTTPS configured via Tailscale Serve"
}

print_success() {
    local dns_name
    dns_name=$(tailscale status --json | jq -r '.Self.DNSName' | sed 's/\.$//')

    echo ""
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${GREEN}${BOLD}✓ POINDEXTER IS READY${NC}"
    echo ""
    echo -e "  ${BOLD}Access from any device on your Tailscale network:${NC}"
    echo ""
    echo -e "    ${CYAN}https://${dns_name}${NC}"
    echo ""
    echo -e "  ${BOLD}SSH access (no keys needed):${NC}"
    echo ""
    echo -e "    ${CYAN}tailscale ssh ${DEX_HOSTNAME}${NC}"
    echo ""
    echo -e "  ${BOLD}Service management:${NC}"
    echo ""
    echo -e "    sudo systemctl status dex"
    echo -e "    sudo systemctl restart dex"
    echo -e "    sudo journalctl -u dex -f"
    echo ""
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

main() {
    print_banner
    detect_platform
    check_root
    install_qrencode
    install_jq
    install_go
    build_dex
    install_tailscale
    authenticate_tailscale
    run_setup_wizard
    install_frontend
    create_config
    create_systemd_service
    configure_tailscale_serve
    print_success
}

main "$@"
