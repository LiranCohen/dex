#!/bin/bash
#
# Poindexter Magic Installer
#
# Two-phase setup:
#   Phase 1: Temporary Cloudflare tunnel -> PIN -> Choose access method
#            (Tailscale or Cloudflare) -> Get permanent URL
#   Phase 2: On permanent URL -> Register passkey -> Enter API keys
#
# Usage:
#   Fresh install:  curl -fsSL https://example.com/install.sh | bash
#   Upgrade:        (automatic if already installed)
#   Reset:          curl -fsSL https://example.com/install.sh | bash -s -- --fresh
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
SETUP_PORT="${SETUP_PORT:-8081}"

# Flags
FRESH_INSTALL=false
CONFIRM_FRESH=false

# State
CLEANUP_PIDS=()
CLEANUP_FILES=()
TUNNEL_PID=""
PERMANENT_URL=""
ACCESS_METHOD=""

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --fresh|--reset|-f)
                FRESH_INSTALL=true
                shift
                ;;
            --yes|-y)
                CONFIRM_FRESH=true
                shift
                ;;
            --help|-h)
                echo "Poindexter Installer"
                echo ""
                echo "Usage: install.sh [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --fresh, --reset, -f   Wipe all data and start fresh install"
                echo "  --yes, -y              Skip confirmation (required with --fresh when piped)"
                echo "  --help, -h             Show this help message"
                echo ""
                echo "Without flags, the installer will:"
                echo "  - Fresh install if no existing installation"
                echo "  - Upgrade (preserve data) if already installed"
                echo ""
                echo "Examples:"
                echo "  # Interactive fresh install (run script directly)"
                echo "  sudo ./install.sh --fresh"
                echo ""
                echo "  # Non-interactive fresh install (piped from curl)"
                echo "  curl -fsSL ... | sudo bash -s -- --fresh --yes"
                exit 0
                ;;
            *)
                shift
                ;;
        esac
    done
}

# Wipe all user data for fresh install
wipe_data() {
    echo ""
    echo -e "${RED}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${RED}${BOLD}⚠️  WARNING: FRESH INSTALL REQUESTED${NC}"
    echo ""
    echo -e "  This will ${RED}PERMANENTLY DELETE${NC} all Poindexter data:"
    echo ""
    echo -e "    • Database (users, tasks, credentials)"
    echo -e "    • Registered passkeys"
    echo -e "    • API keys (GitHub, Anthropic)"
    echo -e "    • All configuration"
    echo ""
    echo -e "  ${YELLOW}This action is IRRECOVERABLE.${NC}"
    echo ""
    echo -e "${RED}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""

    # Check if --yes was provided or if we can prompt interactively
    if [ "$CONFIRM_FRESH" = true ]; then
        echo -e "  ${YELLOW}--yes flag provided, proceeding with wipe...${NC}"
        echo ""
    elif [ -e /dev/tty ]; then
        # Try to read from the actual terminal (works even when stdin is piped)
        echo -ne "  Type ${BOLD}DELETE${NC} to confirm: "
        if read -r confirmation < /dev/tty 2>/dev/null; then
            if [ "$confirmation" != "DELETE" ]; then
                echo ""
                echo -e "  ${GREEN}Aborted.${NC} No data was deleted."
                echo ""
                exit 0
            fi
            echo ""
        else
            # Can't read from tty, require --yes
            echo ""
            echo -e "  ${RED}ERROR:${NC} Cannot read from terminal."
            echo ""
            echo -e "  To confirm fresh install, add ${BOLD}--yes${NC} flag:"
            echo ""
            echo -e "    ${CYAN}curl -fsSL ... | sudo bash -s -- --fresh --yes${NC}"
            echo ""
            exit 1
        fi
    else
        # No tty available, require --yes
        echo -e "  ${RED}ERROR:${NC} Running in non-interactive mode."
        echo ""
        echo -e "  To confirm fresh install, add ${BOLD}--yes${NC} flag:"
        echo ""
        echo -e "    ${CYAN}curl -fsSL ... | sudo bash -s -- --fresh --yes${NC}"
        echo ""
        exit 1
    fi

    log "Stopping services..."
    if command -v systemctl &>/dev/null; then
        systemctl stop dex 2>/dev/null || true
        systemctl stop dex-tunnel 2>/dev/null || true
    fi

    log "Wiping data..."

    # Remove data files but keep binaries
    rm -f "$DEX_INSTALL_DIR/dex.db" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/secrets.json" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/setup-complete" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/setup-pin" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/permanent-url" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/access-method" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/cloudflare-tunnel.json" 2>/dev/null || true
    rm -f "$DEX_INSTALL_DIR/cloudflared-creds.json" 2>/dev/null || true
    rm -rf "$DEX_INSTALL_DIR/worktrees" 2>/dev/null || true
    rm -rf "$DEX_INSTALL_DIR/repos" 2>/dev/null || true

    # Reset tailscale serve if configured
    if command -v tailscale &>/dev/null; then
        tailscale serve reset 2>/dev/null || true
    fi

    success "All data wiped"
    echo ""
}

cleanup() {
    for pid in "${CLEANUP_PIDS[@]:-}"; do
        kill "$pid" 2>/dev/null || true
    done
    if [ -n "${TUNNEL_PID:-}" ]; then
        kill "$TUNNEL_PID" 2>/dev/null || true
    fi
    for file in "${CLEANUP_FILES[@]:-}"; do
        rm -f "$file" 2>/dev/null || true
    done
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
    export PATH="$PATH:/usr/local/go/bin"

    if command -v go &>/dev/null; then
        success "Go already installed: $(go version | awk '{print $3}')"
        return
    fi
    log "Installing Go..."

    local go_version="1.24.3"
    local go_arch="$ARCH"
    local go_os="$OS"

    local go_tarball="go${go_version}.${go_os}-${go_arch}.tar.gz"
    curl -fsSL "https://go.dev/dl/${go_tarball}" -o "/tmp/${go_tarball}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${go_tarball}"
    rm -f "/tmp/${go_tarball}"

    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=/root/go
    export PATH=$PATH:$GOPATH/bin

    success "Go installed: $(go version | awk '{print $3}')"
}

install_cloudflared() {
    if command -v cloudflared &>/dev/null; then
        success "cloudflared already installed"
        return
    fi
    log "Installing cloudflared..."

    case "$OS" in
        darwin)
            if command -v brew &>/dev/null; then
                brew install cloudflare/cloudflare/cloudflared >/dev/null 2>&1 || {
                    # Fallback to manual install
                    local url="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-${ARCH}.tgz"
                    curl -fsSL "$url" -o /tmp/cloudflared.tgz
                    tar -xzf /tmp/cloudflared.tgz -C /tmp
                    mv /tmp/cloudflared /usr/local/bin/
                    chmod +x /usr/local/bin/cloudflared
                    rm -f /tmp/cloudflared.tgz
                }
            else
                local url="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-${ARCH}.tgz"
                curl -fsSL "$url" -o /tmp/cloudflared.tgz
                tar -xzf /tmp/cloudflared.tgz -C /tmp
                mv /tmp/cloudflared /usr/local/bin/
                chmod +x /usr/local/bin/cloudflared
                rm -f /tmp/cloudflared.tgz
            fi
            ;;
        linux)
            local url="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${ARCH}"
            curl -fsSL "$url" -o /usr/local/bin/cloudflared
            chmod +x /usr/local/bin/cloudflared
            ;;
    esac

    success "cloudflared installed"
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

# ============================================================================
# Development Runtime Installation
# ============================================================================

install_podman() {
    if command -v podman &>/dev/null; then
        success "Podman already installed: $(podman --version)"
        # Ensure docker alias exists
        if ! command -v docker &>/dev/null; then
            ln -sf "$(which podman)" /usr/local/bin/docker
            success "Created docker -> podman alias"
        fi
        return
    fi
    log "Installing Podman..."

    case "$OS" in
        darwin)
            if command -v brew &>/dev/null; then
                brew install podman >/dev/null 2>&1
            else
                error "Homebrew required to install Podman on macOS"
            fi
            ;;
        linux)
            if command -v apt-get &>/dev/null; then
                apt-get update -qq && apt-get install -y -qq podman >/dev/null
            elif command -v dnf &>/dev/null; then
                dnf install -y -q podman >/dev/null
            elif command -v yum &>/dev/null; then
                yum install -y -q podman >/dev/null
            elif command -v pacman &>/dev/null; then
                pacman -Sy --noconfirm podman >/dev/null
            else
                warn "Could not install Podman - unknown package manager"
                return
            fi
            ;;
    esac

    # Create docker -> podman alias
    ln -sf "$(which podman)" /usr/local/bin/docker

    success "Podman installed: $(podman --version)"
    success "Created docker -> podman alias"
}

install_nodejs() {
    if command -v node &>/dev/null; then
        success "Node.js already installed: $(node --version)"
        return
    fi
    log "Installing Node.js..."

    case "$OS" in
        darwin)
            if command -v brew &>/dev/null; then
                brew install node >/dev/null 2>&1
            else
                # Use official installer
                curl -fsSL https://nodejs.org/dist/v22.0.0/node-v22.0.0-darwin-${ARCH}.tar.gz -o /tmp/node.tar.gz
                tar -xzf /tmp/node.tar.gz -C /usr/local --strip-components=1
                rm -f /tmp/node.tar.gz
            fi
            ;;
        linux)
            # Use NodeSource for latest LTS
            if command -v apt-get &>/dev/null; then
                curl -fsSL https://deb.nodesource.com/setup_22.x | bash - >/dev/null 2>&1
                apt-get install -y -qq nodejs >/dev/null
            elif command -v dnf &>/dev/null; then
                dnf install -y -q nodejs npm >/dev/null
            elif command -v yum &>/dev/null; then
                curl -fsSL https://rpm.nodesource.com/setup_22.x | bash - >/dev/null 2>&1
                yum install -y -q nodejs >/dev/null
            elif command -v pacman &>/dev/null; then
                pacman -Sy --noconfirm nodejs npm >/dev/null
            else
                warn "Could not install Node.js - unknown package manager"
                return
            fi
            ;;
    esac

    success "Node.js installed: $(node --version)"
}

install_python() {
    if command -v python3 &>/dev/null; then
        success "Python already installed: $(python3 --version)"
        return
    fi
    log "Installing Python..."

    case "$OS" in
        darwin)
            if command -v brew &>/dev/null; then
                brew install python@3.12 >/dev/null 2>&1
            fi
            ;;
        linux)
            if command -v apt-get &>/dev/null; then
                apt-get update -qq && apt-get install -y -qq python3 python3-pip python3-venv >/dev/null
            elif command -v dnf &>/dev/null; then
                dnf install -y -q python3 python3-pip >/dev/null
            elif command -v yum &>/dev/null; then
                yum install -y -q python3 python3-pip >/dev/null
            elif command -v pacman &>/dev/null; then
                pacman -Sy --noconfirm python python-pip >/dev/null
            fi
            ;;
    esac

    if command -v python3 &>/dev/null; then
        success "Python installed: $(python3 --version)"
    else
        warn "Could not install Python"
    fi
}

install_rust() {
    if command -v rustc &>/dev/null; then
        success "Rust already installed: $(rustc --version)"
        return
    fi
    log "Installing Rust..."

    # Use rustup for all platforms
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y >/dev/null 2>&1
    export PATH="$HOME/.cargo/bin:$PATH"

    if command -v rustc &>/dev/null; then
        success "Rust installed: $(rustc --version)"
    else
        warn "Could not install Rust"
    fi
}

install_zig() {
    if command -v zig &>/dev/null; then
        success "Zig already installed: $(zig version)"
        return
    fi
    log "Installing Zig..."

    local zig_version="0.13.0"

    case "$OS" in
        darwin)
            if command -v brew &>/dev/null; then
                brew install zig >/dev/null 2>&1
            else
                local zig_arch="$ARCH"
                [ "$zig_arch" = "amd64" ] && zig_arch="x86_64"
                curl -fsSL "https://ziglang.org/download/${zig_version}/zig-macos-${zig_arch}-${zig_version}.tar.xz" -o /tmp/zig.tar.xz
                tar -xf /tmp/zig.tar.xz -C /usr/local
                ln -sf /usr/local/zig-macos-${zig_arch}-${zig_version}/zig /usr/local/bin/zig
                rm -f /tmp/zig.tar.xz
            fi
            ;;
        linux)
            local zig_arch="$ARCH"
            [ "$zig_arch" = "amd64" ] && zig_arch="x86_64"
            [ "$zig_arch" = "arm64" ] && zig_arch="aarch64"
            curl -fsSL "https://ziglang.org/download/${zig_version}/zig-linux-${zig_arch}-${zig_version}.tar.xz" -o /tmp/zig.tar.xz
            tar -xf /tmp/zig.tar.xz -C /usr/local
            ln -sf /usr/local/zig-linux-${zig_arch}-${zig_version}/zig /usr/local/bin/zig
            rm -f /tmp/zig.tar.xz
            ;;
    esac

    if command -v zig &>/dev/null; then
        success "Zig installed: $(zig version)"
    else
        warn "Could not install Zig"
    fi
}

install_build_essentials() {
    log "Installing build essentials (gcc, make, etc.)..."

    case "$OS" in
        darwin)
            # Xcode command line tools provide clang, make, etc.
            if ! xcode-select -p &>/dev/null; then
                xcode-select --install 2>/dev/null || true
                # Wait for installation
                until xcode-select -p &>/dev/null; do
                    sleep 5
                done
            fi
            success "Xcode command line tools available"
            ;;
        linux)
            if command -v apt-get &>/dev/null; then
                apt-get update -qq && apt-get install -y -qq build-essential >/dev/null
            elif command -v dnf &>/dev/null; then
                dnf groupinstall -y -q "Development Tools" >/dev/null 2>&1 || dnf install -y -q gcc gcc-c++ make >/dev/null
            elif command -v yum &>/dev/null; then
                yum groupinstall -y -q "Development Tools" >/dev/null 2>&1 || yum install -y -q gcc gcc-c++ make >/dev/null
            elif command -v pacman &>/dev/null; then
                pacman -Sy --noconfirm base-devel >/dev/null
            fi
            success "Build essentials installed"
            ;;
    esac
}

install_dev_runtimes() {
    log "Installing development runtimes..."
    echo ""

    install_build_essentials
    install_nodejs
    install_python
    install_rust
    install_zig
    install_podman

    echo ""
    success "Development runtimes installed"
}

build_dex() {
    log "Building dex from source..."

    export PATH="$PATH:/usr/local/go/bin"
    export GOPROXY="https://proxy.golang.org,direct"
    mkdir -p "$DEX_INSTALL_DIR"

    go clean -modcache 2>/dev/null || true

    local src_dir="/tmp/dex-build"
    rm -rf "$src_dir"
    git clone --depth=1 https://github.com/LiranCohen/dex.git "$src_dir"

    cd "$src_dir"
    go build -o "$DEX_INSTALL_DIR/dex" ./cmd/dex
    go build -o "$DEX_INSTALL_DIR/dex-setup" ./cmd/dex-setup

    # Copy prompts directory (required for Ralph loop)
    rm -rf "$DEX_INSTALL_DIR/prompts"
    cp -r prompts "$DEX_INSTALL_DIR/prompts"

    cd - >/dev/null

    rm -rf "$src_dir"
    success "Built dex and dex-setup"
}

install_frontend() {
    log "Building frontend..."

    if ! command -v bun &>/dev/null; then
        log "Installing bun..."
        curl -fsSL https://bun.sh/install | bash
        export PATH="$HOME/.bun/bin:$PATH"
    fi

    local tmp_repo="/tmp/dex-repo"
    rm -rf "$tmp_repo"
    git clone --depth=1 https://github.com/lirancohen/dex.git "$tmp_repo"

    cd "$tmp_repo/frontend"
    bun install
    bun run build
    cd - >/dev/null

    mkdir -p "$DEX_INSTALL_DIR"
    # Remove old frontend but preserve all other files (db, config, etc)
    rm -rf "$DEX_INSTALL_DIR/frontend"
    cp -r "$tmp_repo/frontend/dist" "$DEX_INSTALL_DIR/frontend"
    rm -rf "$tmp_repo"

    success "Frontend built and installed"
}

generate_pin() {
    local pin
    if command -v shuf &>/dev/null; then
        pin=$(shuf -i 100000-999999 -n 1)
    else
        # Fallback for macOS
        pin=$(jot -r 1 100000 999999)
    fi
    echo "$pin"
}

show_qr() {
    local url="$1"
    local title="$2"

    echo ""
    echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${BOLD}📱 $title${NC}"
    echo ""
    qrencode -t ANSIUTF8 -m 2 "$url"
    echo ""
    echo -e "  ${DIM}Or open:${NC} ${CYAN}$url${NC}"
    echo ""
    echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

start_quick_tunnel() {
    log "Starting temporary tunnel..." >&2

    local tunnel_log="/tmp/cloudflared-tunnel.log"
    CLEANUP_FILES+=("$tunnel_log")
    rm -f "$tunnel_log"

    # Start cloudflared quick tunnel
    cloudflared tunnel --url "http://127.0.0.1:$SETUP_PORT" > "$tunnel_log" 2>&1 &
    TUNNEL_PID=$!

    # Wait for tunnel URL
    local temp_url=""
    local attempts=0
    while [ -z "$temp_url" ] && [ $attempts -lt 30 ]; do
        sleep 1
        attempts=$((attempts + 1))
        if [ -f "$tunnel_log" ]; then
            # Extract URL, stripping any ANSI codes that might be in the log
            temp_url=$(grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' "$tunnel_log" 2>/dev/null | head -1 | sed 's/\x1b\[[0-9;]*m//g' | tr -d '[:space:]' || true)
        fi
    done

    if [ -z "$temp_url" ]; then
        cat "$tunnel_log" 2>/dev/null || true
        error "Failed to get temporary tunnel URL"
    fi

    echo "$temp_url"
}

run_setup_phase1() {
    log "Starting setup wizard (Phase 1: Choose access method)..."

    # Generate PIN
    local pin
    pin=$(generate_pin)
    local pin_file="$DEX_INSTALL_DIR/setup-pin"
    mkdir -p "$DEX_INSTALL_DIR"
    echo -n "$pin" > "$pin_file"
    chmod 600 "$pin_file"

    echo ""
    echo -e "  ${BOLD}${YELLOW}Setup PIN: $pin${NC}"
    echo ""

    # Kill anything using the setup port
    if command -v lsof &>/dev/null; then
        lsof -ti ":$SETUP_PORT" | xargs -r kill -9 2>/dev/null || true
    elif command -v fuser &>/dev/null; then
        fuser -k "$SETUP_PORT/tcp" 2>/dev/null || true
    fi
    pkill -9 -f "dex-setup" 2>/dev/null || true
    sleep 1

    # Run setup wizard
    local setup_bin="$DEX_INSTALL_DIR/dex-setup"
    if [ ! -f "$setup_bin" ]; then
        error "Setup wizard not found at $setup_bin"
    fi

    "$setup_bin" \
        -addr "127.0.0.1:$SETUP_PORT" \
        -pin-file "$pin_file" \
        -data-dir "$DEX_INSTALL_DIR" \
        -dex-port "$DEX_PORT" &
    local wizard_pid=$!
    CLEANUP_PIDS+=($wizard_pid)

    sleep 2

    # Start temporary cloudflare tunnel
    local temp_url
    temp_url=$(start_quick_tunnel)

    # Include PIN in QR URL as hash anchor for auto-fill
    show_qr "${temp_url}#${pin}" "SCAN TO SETUP POINDEXTER"
    echo -e "  ${BOLD}PIN: ${YELLOW}$pin${NC}"
    echo ""
    echo -e "  ${YELLOW}Waiting for you to choose an access method...${NC}"
    echo ""

    # Wait for permanent URL to be written (phase 1 complete)
    while [ ! -f "$DEX_INSTALL_DIR/permanent-url" ]; do
        if ! kill -0 "$wizard_pid" 2>/dev/null; then
            if [ -f "$DEX_INSTALL_DIR/permanent-url" ]; then
                break
            fi
            error "Setup wizard exited unexpectedly"
        fi
        sleep 2
    done

    # Read permanent URL and access method
    PERMANENT_URL=$(cat "$DEX_INSTALL_DIR/permanent-url")
    ACCESS_METHOD=$(cat "$DEX_INSTALL_DIR/access-method" 2>/dev/null || echo "unknown")

    echo ""
    success "Permanent access established!"
    echo ""
    echo -e "  ${BOLD}Access Method:${NC} $ACCESS_METHOD"
    echo -e "  ${BOLD}Permanent URL:${NC} ${CYAN}$PERMANENT_URL${NC}"
    echo ""

    # Remove PIN file
    rm -f "$pin_file"
}

wait_for_full_setup() {
    log "Waiting for full setup completion (Phase 2)..."
    echo ""
    echo -e "  ${YELLOW}Complete the remaining setup steps on your permanent URL:${NC}"
    echo -e "  ${CYAN}$PERMANENT_URL${NC}"
    echo ""
    echo -e "  - Register a passkey (Face ID / Touch ID / Security Key)"
    echo -e "  - Enter your GitHub token"
    echo -e "  - Enter your Anthropic API key"
    echo ""
    echo -e "  ${DIM}Temporary tunnel remains active for debugging...${NC}"
    echo ""

    # Wait for full setup completion
    while [ ! -f "$DEX_INSTALL_DIR/setup-complete" ]; do
        sleep 2
    done

    success "Setup complete!"

    # Now we can shut down the temporary tunnel
    if [ -n "${TUNNEL_PID:-}" ]; then
        log "Shutting down temporary tunnel..."
        kill "$TUNNEL_PID" 2>/dev/null || true
        TUNNEL_PID=""
    fi
}

create_systemd_service() {
    if ! command -v systemctl &>/dev/null; then
        warn "systemd not found, skipping service creation"
        return
    fi

    log "Creating systemd service..."

    # Read access method to determine how to configure
    local access_method
    access_method=$(cat "$DEX_INSTALL_DIR/access-method" 2>/dev/null || echo "tailscale")

    # Create worktree and repos directories
    mkdir -p ${DEX_INSTALL_DIR}/worktrees
    mkdir -p ${DEX_INSTALL_DIR}/repos

    # Create the main service
    cat > /etc/systemd/system/dex.service << EOF
[Unit]
Description=Poindexter AI Orchestration
After=network.target tailscaled.service
Wants=tailscaled.service

[Service]
Type=simple
User=root
WorkingDirectory=${DEX_INSTALL_DIR}
ExecStart=${DEX_INSTALL_DIR}/dex \\
    -db ${DEX_INSTALL_DIR}/dex.db \\
    -static ${DEX_INSTALL_DIR}/frontend \\
    -addr 127.0.0.1:${DEX_PORT} \\
    -base-dir ${DEX_INSTALL_DIR}
Restart=always
RestartSec=5
Environment=DEX_DATA_DIR=${DEX_INSTALL_DIR}

[Install]
WantedBy=multi-user.target
EOF

    # If using Cloudflare, create a tunnel service
    if [ "$access_method" = "cloudflare" ] && [ -f "$DEX_INSTALL_DIR/cloudflare-tunnel.json" ]; then
        local tunnel_id
        tunnel_id=$(jq -r '.id' "$DEX_INSTALL_DIR/cloudflare-tunnel.json")
        local cred_path="$DEX_INSTALL_DIR/cloudflared-creds.json"

        cat > /etc/systemd/system/dex-tunnel.service << EOF
[Unit]
Description=Poindexter Cloudflare Tunnel
After=network.target
Wants=dex.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/cloudflared tunnel --credentials-file ${cred_path} run ${tunnel_id}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable dex-tunnel >/dev/null 2>&1
        systemctl start dex-tunnel
    fi

    systemctl daemon-reload
    systemctl enable dex >/dev/null 2>&1
    systemctl start dex

    success "Service created and started"
}

configure_tailscale_serve() {
    local access_method
    access_method=$(cat "$DEX_INSTALL_DIR/access-method" 2>/dev/null || echo "")

    if [ "$access_method" != "tailscale" ]; then
        return
    fi

    log "Configuring Tailscale Serve..."

    local attempts=0
    while ! curl -s "http://127.0.0.1:${DEX_PORT}/" >/dev/null 2>&1; do
        sleep 1
        attempts=$((attempts + 1))
        if [ $attempts -gt 30 ]; then
            warn "Dex not responding on port ${DEX_PORT}, continuing anyway"
            break
        fi
    done

    if ! setsid tailscale serve --bg --https=443 "http://127.0.0.1:${DEX_PORT}" 2>/dev/null; then
        warn "Failed to configure Tailscale Serve"
        echo ""
        echo -e "  ${YELLOW}Run manually:${NC}"
        echo -e "  ${CYAN}tailscale serve --bg --https=443 http://127.0.0.1:${DEX_PORT}${NC}"
        echo ""
        return 1
    fi

    success "HTTPS configured via Tailscale Serve"
}

print_success() {
    local permanent_url
    permanent_url=$(cat "$DEX_INSTALL_DIR/permanent-url" 2>/dev/null || echo "")
    local is_upgrade="${1:-false}"

    echo ""
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    if [ "$is_upgrade" = "true" ]; then
        echo -e "  ${GREEN}${BOLD}✓ POINDEXTER UPGRADED SUCCESSFULLY${NC}"
    else
        echo -e "  ${GREEN}${BOLD}✓ POINDEXTER IS READY${NC}"
    fi
    echo ""

    if [ -n "$permanent_url" ]; then
        echo -e "  ${BOLD}📱 Scan to access:${NC}"
        echo ""
        qrencode -t ANSIUTF8 -m 2 "$permanent_url"
        echo ""
        echo -e "  ${CYAN}$permanent_url${NC}"
        echo ""
    fi

    if command -v systemctl &>/dev/null; then
        echo -e "  ${BOLD}Service management:${NC}"
        echo ""
        echo -e "    sudo systemctl status dex"
        echo -e "    sudo systemctl restart dex"
        echo -e "    sudo journalctl -u dex -f"
        echo ""
    fi
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

is_configured() {
    # Check if dex is already fully configured (has completed setup)
    [ -f "$DEX_INSTALL_DIR/setup-complete" ] && \
    [ -f "$DEX_INSTALL_DIR/dex.db" ]
}

upgrade_dex() {
    log "Upgrading Poindexter..."
    echo ""

    # Stop the service before upgrading
    if command -v systemctl &>/dev/null; then
        log "Stopping dex service..."
        systemctl stop dex 2>/dev/null || true
    fi

    # Rebuild binaries and frontend (state files are preserved)
    build_dex
    install_frontend

    # Recreate/update systemd service
    create_systemd_service

    # Configure tailscale serve if using tailscale
    configure_tailscale_serve

    success "Upgrade complete!"
}

main() {
    # Parse arguments first (before print_banner clears screen)
    parse_args "$@"

    print_banner
    detect_platform
    check_root

    # Handle fresh install request
    if [ "$FRESH_INSTALL" = true ]; then
        wipe_data
    fi

    install_qrencode
    install_jq
    install_go
    install_cloudflared
    install_tailscale
    install_dev_runtimes

    if [ "$FRESH_INSTALL" = false ] && is_configured; then
        echo -e "${CYAN}Existing installation detected. Running upgrade...${NC}"
        echo ""
        upgrade_dex
        print_success true
    else
        # Fresh install
        build_dex
        install_frontend
        run_setup_phase1
        create_systemd_service
        configure_tailscale_serve
        wait_for_full_setup
        print_success false
    fi
}

main "$@"
