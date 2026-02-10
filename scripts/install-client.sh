#!/bin/bash
#
# Dex Client Installer
#
# One-shot installer: downloads binary, enrolls device, installs mesh daemon,
# and launches the tray app. Everything you need in a single command.
#
# Usage:
#   curl -fsSL https://get.enbox.id/client | bash -s -- dexkey-xxx
#   curl -fsSL https://get.enbox.id/client | bash -s -- --key dexkey-xxx
#
# Environment variables:
#   DEX_VERSION           - Version to install (default: latest)
#   DEX_CLIENT_DATA_DIR   - Data directory (default: ~/.dex)
#   DEX_CENTRAL_URL       - Central server URL (default: https://central.enbox.id)
#   DEX_ENROLL_KEY        - Enrollment key (alternative to passing as argument)
#
set -e

# Installer version
INSTALLER_VERSION="0.2.0"

# =============================================================================
# Configuration
# =============================================================================

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --key|-k)
            DEX_ENROLL_KEY="$2"
            shift 2
            ;;
        --version|-v)
            DEX_VERSION="$2"
            shift 2
            ;;
        --hostname|-h)
            DEX_HOSTNAME="$2"
            shift 2
            ;;
        dexkey-*)
            DEX_ENROLL_KEY="$1"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

VERSION="${DEX_VERSION:-latest}"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="${DEX_CLIENT_DATA_DIR:-$HOME/.dex}"
CENTRAL_URL="${DEX_CENTRAL_URL:-https://central.enbox.id}"
ENROLL_KEY="${DEX_ENROLL_KEY:-}"
HOSTNAME_ARG="${DEX_HOSTNAME:-}"

# =============================================================================
# Terminal UI
# =============================================================================

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
GRAY='\033[0;90m'
NC='\033[0m'

# Symbols
SYM_PENDING="○"
SYM_RUNNING="●"
SYM_SUCCESS="✔"
SYM_FAILED="✖"
SYM_SKIP="◌"

# Platform-specific steps are set after detection
declare -a STEPS
declare -a STEP_STATUS

init_steps_darwin() {
    STEPS=(
        "Detecting platform"
        "Acquiring privileges"
        "Downloading Dex"
        "Installing binary"
        "Enrolling device"
        "Installing mesh daemon"
        "Installing tray app"
    )
}

init_steps_linux() {
    STEPS=(
        "Detecting platform"
        "Acquiring privileges"
        "Downloading Dex"
        "Installing binary"
        "Enrolling device"
        "Creating system service"
    )
}

init_step_status() {
    STEP_COUNT=${#STEPS[@]}
    STEP_STATUS=()
    for ((i=0; i<STEP_COUNT; i++)); do
        STEP_STATUS[$i]="pending"
    done
}

# Activity log
declare -a ACTIVITY_LOG
MAX_LOG_LINES=4

# Clear screen and move cursor to top
clear_screen() {
    printf '\033[2J\033[H'
}

# Draw the full UI
draw_ui() {
    clear_screen

    # Header
    echo -e "${CYAN}"
    cat << BANNER
    ╭──────────────────────────────────────────────────╮
    │                                                  │
    │    ██████╗ ███████╗██╗  ██╗                      │
    │    ██╔══██╗██╔════╝╚██╗██╔╝                      │
    │    ██║  ██║█████╗   ╚███╔╝     CLIENT            │
    │    ██║  ██║██╔══╝   ██╔██╗                       │
    │    ██████╔╝███████╗██╔╝ ██╗                      │
    │    ╚═════╝ ╚══════╝╚═╝  ╚═╝                      │
    │                                                  │
    │         Direct Mesh Access for Your HQ           │
    │                    v${INSTALLER_VERSION}                         │
    ╰──────────────────────────────────────────────────╯
BANNER
    echo -e "${NC}"

    # Progress section
    echo -e "  ${BOLD}Installation Progress${NC}"
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"
    echo ""

    # Draw each step
    for ((i=0; i<STEP_COUNT; i++)); do
        local status="${STEP_STATUS[$i]}"
        local symbol color

        case "$status" in
            pending)
                symbol="$SYM_PENDING"
                color="$GRAY"
                ;;
            running)
                symbol="$SYM_RUNNING"
                color="$YELLOW"
                ;;
            success)
                symbol="$SYM_SUCCESS"
                color="$GREEN"
                ;;
            failed)
                symbol="$SYM_FAILED"
                color="$RED"
                ;;
            skip)
                symbol="$SYM_SKIP"
                color="$GRAY"
                ;;
        esac

        echo -e "    ${color}${symbol}${NC}  ${STEPS[$i]}"
    done

    echo ""
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"

    # Activity log section
    echo -e "  ${BOLD}Activity${NC}"
    echo ""

    local log_count=${#ACTIVITY_LOG[@]}
    local start=$((log_count > MAX_LOG_LINES ? log_count - MAX_LOG_LINES : 0))

    for ((i=start; i<log_count; i++)); do
        echo -e "    ${DIM}${ACTIVITY_LOG[$i]}${NC}"
    done

    # Pad remaining lines
    local shown=$((log_count - start))
    for ((i=shown; i<MAX_LOG_LINES; i++)); do
        echo ""
    done

    echo ""
}

log_activity() {
    ACTIVITY_LOG+=("$1")
    draw_ui
}

update_step() {
    local step_index=$1
    local new_status=$2
    STEP_STATUS[$step_index]="$new_status"
    draw_ui
}

run_step() {
    local step_index=$1
    local step_func=$2

    update_step "$step_index" "running"

    if $step_func; then
        update_step "$step_index" "success"
        return 0
    else
        update_step "$step_index" "failed"
        return 1
    fi
}

skip_step() {
    local step_index=$1
    update_step "$step_index" "skip"
}

# =============================================================================
# Installation Steps
# =============================================================================

# Step 0: Detect platform
do_detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        arm64)   ARCH="arm64" ;;
        *)
            log_activity "Error: Unsupported architecture: $ARCH"
            return 1
            ;;
    esac

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)
            log_activity "Error: Unsupported OS: $OS"
            return 1
            ;;
    esac

    # Initialize platform-specific steps now that we know the OS
    if [ "$OS" = "darwin" ]; then
        init_steps_darwin
    else
        init_steps_linux
    fi
    init_step_status
    # Mark step 0 as running again since init_step_status reset it
    STEP_STATUS[0]="success"

    log_activity "Platform: ${OS}/${ARCH}"
    return 0
}

# Step 1: Acquire sudo
do_acquire_sudo() {
    log_activity "Requesting administrator privileges..."
    log_activity "  (install binary, mesh daemon, tray app)"

    # Check if already root
    if [ "$(id -u)" -eq 0 ]; then
        log_activity "Running as root"
        return 0
    fi

    # Validate sudo credentials (will prompt the user)
    if ! sudo -v 2>/dev/null; then
        log_activity "Error: Failed to acquire sudo privileges"
        return 1
    fi

    # Keep sudo alive in the background during install
    (while true; do sudo -n true 2>/dev/null; sleep 50; done) &
    SUDO_KEEPALIVE_PID=$!
    trap "kill $SUDO_KEEPALIVE_PID 2>/dev/null; rm -rf ${TMP_DIR:-/dev/null}" EXIT

    log_activity "Privileges acquired"
    return 0
}

# Step 2: Download binary
do_download_binary() {
    local base_url="https://github.com/lirancohen/dex/releases"

    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="${base_url}/latest/download/dex-${OS}-${ARCH}.tar.gz"
    else
        DOWNLOAD_URL="${base_url}/download/${VERSION}/dex-${OS}-${ARCH}.tar.gz"
    fi

    TMP_DIR=$(mktemp -d)

    log_activity "Downloading from GitHub releases..."

    if command -v curl &> /dev/null; then
        if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/dex.tar.gz" 2>/dev/null; then
            log_activity "Error: Download failed: $DOWNLOAD_URL"
            return 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/dex.tar.gz" 2>/dev/null; then
            log_activity "Error: Download failed: $DOWNLOAD_URL"
            return 1
        fi
    else
        log_activity "Error: Neither curl nor wget found"
        return 1
    fi

    # Also download the macOS app bundle if on darwin
    if [ "$OS" = "darwin" ]; then
        if [ "$VERSION" = "latest" ]; then
            APP_DOWNLOAD_URL="${base_url}/latest/download/DexClient-${ARCH}.app.zip"
        else
            APP_DOWNLOAD_URL="${base_url}/download/${VERSION}/DexClient-${ARCH}.app.zip"
        fi

        if command -v curl &> /dev/null; then
            if ! curl -fsSL "$APP_DOWNLOAD_URL" -o "$TMP_DIR/DexClient.app.zip" 2>/dev/null; then
                log_activity "Warning: Failed to download DexClient.app"
                APP_DOWNLOAD_FAILED=1
            fi
        elif command -v wget &> /dev/null; then
            if ! wget -q "$APP_DOWNLOAD_URL" -O "$TMP_DIR/DexClient.app.zip" 2>/dev/null; then
                log_activity "Warning: Failed to download DexClient.app"
                APP_DOWNLOAD_FAILED=1
            fi
        fi
    fi

    log_activity "Download complete"
    return 0
}

# Step 3: Install binary
do_install_binary() {
    log_activity "Extracting archive..."
    tar -xzf "$TMP_DIR/dex.tar.gz" -C "$TMP_DIR"

    log_activity "Installing to ${INSTALL_DIR}/dex..."
    sudo install -m 755 "$TMP_DIR/dex" "$INSTALL_DIR/dex"

    # Create data directory (as current user, not root)
    if [ ! -d "$DATA_DIR" ]; then
        mkdir -p "$DATA_DIR"
    fi

    log_activity "Installed: ${INSTALL_DIR}/dex"
    return 0
}

# Step 4: Enrollment
do_enroll() {
    # Skip if already enrolled
    if [ -f "$DATA_DIR/config.json" ]; then
        log_activity "Already enrolled (config.json exists)"
        return 0
    fi

    local key="$ENROLL_KEY"

    # If no key provided, prompt interactively
    if [ -z "$key" ]; then
        if [ -t 0 ]; then
            draw_ui
            echo ""
            echo -e "  ${BOLD}Enrollment Required${NC}"
            echo -e "  ${DIM}Get your key from HQ Settings → Devices${NC}"
            echo ""
            read -p "  Enter enrollment key: " key

            if [ -z "$key" ]; then
                log_activity "Error: Enrollment key is required"
                return 1
            fi
        else
            log_activity "Error: No enrollment key provided"
            log_activity "Pass key: curl ... | bash -s -- dexkey-xxx"
            return 1
        fi
    fi

    log_activity "Enrolling with Central..."

    local hostname_arg=""
    if [ -n "$HOSTNAME_ARG" ]; then
        hostname_arg="--hostname $HOSTNAME_ARG"
    fi

    local enroll_output
    if enroll_output=$("$INSTALL_DIR/dex" client enroll --key "$key" --data-dir "$DATA_DIR" --central-url "$CENTRAL_URL" $hostname_arg 2>&1); then
        log_activity "Enrollment successful!"
        return 0
    else
        log_activity "Error: Enrollment failed"
        log_activity "$enroll_output"
        return 1
    fi
}

# Step 5 (darwin): Install mesh daemon
do_install_meshd() {
    log_activity "Installing mesh daemon (LaunchDaemon)..."

    local meshd_output
    if meshd_output=$(sudo "$INSTALL_DIR/dex" meshd install 2>&1); then
        log_activity "Mesh daemon installed and started"
        return 0
    else
        log_activity "Error: Failed to install mesh daemon"
        log_activity "$meshd_output"
        return 1
    fi
}

# Step 6 (darwin): Install and launch tray app
do_install_tray_app() {
    if [ "${APP_DOWNLOAD_FAILED:-0}" = "1" ]; then
        log_activity "Skipped: DexClient.app download failed earlier"
        return 1
    fi

    # Remove existing app if present
    if [ -d "/Applications/DexClient.app" ]; then
        sudo rm -rf "/Applications/DexClient.app"
    fi

    log_activity "Installing DexClient.app..."
    if ! unzip -q "$TMP_DIR/DexClient.app.zip" -d "$TMP_DIR/app" 2>/dev/null; then
        log_activity "Error: Failed to extract DexClient.app"
        return 1
    fi

    sudo cp -R "$TMP_DIR/app/DexClient.app" "/Applications/DexClient.app"

    log_activity "Launching DexClient..."
    open "/Applications/DexClient.app"

    log_activity "DexClient.app installed and launched"
    return 0
}

# Step 5 (linux): Create systemd user service
do_create_linux_service() {
    local service_dir="$HOME/.config/systemd/user"
    local service_file="$service_dir/dex-client.service"

    if [ ! -d "$service_dir" ]; then
        mkdir -p "$service_dir"
    fi

    cat > "$service_file" << EOF
[Unit]
Description=Dex Client - Mesh Network Access
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/dex client start --data-dir $DATA_DIR
Restart=always
RestartSec=10
Environment=DEX_CLIENT_DATA_DIR=$DATA_DIR

[Install]
WantedBy=default.target
EOF

    # Reload and enable the service
    systemctl --user daemon-reload 2>/dev/null || true
    systemctl --user enable dex-client 2>/dev/null || true

    log_activity "Created systemd user service"
    return 0
}

# =============================================================================
# Completion Screen
# =============================================================================

show_completion() {
    clear_screen

    local dex_version
    dex_version=$("$INSTALL_DIR/dex" version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")

    echo -e "${GREEN}"
    cat << BANNER
    ╭──────────────────────────────────────────────────╮
    │                                                  │
    │    ██████╗ ███████╗██╗  ██╗                      │
    │    ██╔══██╗██╔════╝╚██╗██╔╝                      │
    │    ██║  ██║█████╗   ╚███╔╝     CLIENT            │
    │    ██║  ██║██╔══╝   ██╔██╗                       │
    │    ██████╔╝███████╗██╔╝ ██╗                      │
    │    ╚═════╝ ╚══════╝╚═╝  ╚═╝                      │
    │                                                  │
    │      ✔  Installation Complete!                   │
    │         Dex ${dex_version}                                │
    ╰──────────────────────────────────────────────────╯
BANNER
    echo -e "${NC}"

    echo -e "  ${BOLD}What's Running${NC}"
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"
    echo ""

    if [ -f "$DATA_DIR/config.json" ]; then
        local namespace
        namespace=$(grep -o '"namespace"[[:space:]]*:[[:space:]]*"[^"]*"' "$DATA_DIR/config.json" 2>/dev/null | cut -d'"' -f4 || true)

        if [ -n "$namespace" ]; then
            echo -e "    ${BOLD}Namespace:${NC}  ${CYAN}${namespace}${NC}"
        fi

        if [ "$OS" = "darwin" ]; then
            echo -e "    ${BOLD}Daemon:${NC}     ${GREEN}✔${NC}  Mesh daemon running (com.dex.meshd)"
            echo -e "    ${BOLD}Tray:${NC}       ${GREEN}✔${NC}  DexClient.app in menu bar"
            echo ""
            echo -e "    ${BOLD}Open HQ:${NC}"
            if [ -n "$namespace" ]; then
                echo -e "      ${CYAN}https://hq.${namespace}.enbox.id${NC}"
            else
                echo -e "      Click ${BOLD}Open HQ${NC} in the menu bar tray"
            fi
        elif [ "$OS" = "linux" ]; then
            echo -e "    ${BOLD}Service:${NC}    systemd user service created"
            echo ""
            echo -e "    ${BOLD}Start the client:${NC}"
            echo -e "      ${CYAN}systemctl --user start dex-client${NC}"
        fi
    else
        echo -e "    ${BOLD}Binary installed at:${NC} ${CYAN}${INSTALL_DIR}/dex${NC}"
        echo -e "    ${YELLOW}Enrollment was skipped — run manually:${NC}"
        echo -e "      ${CYAN}dex client enroll --key YOUR_KEY${NC}"
    fi

    echo ""
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"
    echo ""
}

show_failure() {
    clear_screen

    echo -e "${RED}"
    cat << BANNER
    ╭──────────────────────────────────────────────────╮
    │                                                  │
    │    ██████╗ ███████╗██╗  ██╗                      │
    │    ██╔══██╗██╔════╝╚██╗██╔╝                      │
    │    ██║  ██║█████╗   ╚███╔╝     CLIENT            │
    │    ██║  ██║██╔══╝   ██╔██╗                       │
    │    ██████╔╝███████╗██╔╝ ██╗                      │
    │    ╚═════╝ ╚══════╝╚═╝  ╚═╝                      │
    │                                                  │
    │      ✖  Installation Failed                      │
    │         Installer v${INSTALLER_VERSION}                       │
    ╰──────────────────────────────────────────────────╯
BANNER
    echo -e "${NC}"

    echo -e "  ${BOLD}What went wrong${NC}"
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"
    echo ""

    local log_count=${#ACTIVITY_LOG[@]}
    local start=$((log_count > 6 ? log_count - 6 : 0))
    for ((i=start; i<log_count; i++)); do
        echo -e "    ${ACTIVITY_LOG[$i]}"
    done

    echo ""
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"
    echo ""
    echo -e "  ${BOLD}Need help?${NC}"
    echo -e "    ${CYAN}https://github.com/lirancohen/dex/issues${NC}"
    echo ""
}

# =============================================================================
# Main
# =============================================================================

main() {
    # Minimal init for step 0 (will be re-initialized after platform detection)
    STEPS=("Detecting platform")
    STEP_COUNT=1
    STEP_STATUS=("pending")

    draw_ui

    local failed=0

    # Step 0: Detect platform (also initializes platform-specific steps)
    run_step 0 do_detect_platform || failed=1

    if [ $failed -eq 0 ]; then
        # Step 1: Acquire sudo
        run_step 1 do_acquire_sudo || failed=1
    fi

    if [ $failed -eq 0 ]; then
        # Step 2: Download binary (and app bundle on macOS)
        run_step 2 do_download_binary || failed=1
    fi

    if [ $failed -eq 0 ]; then
        # Step 3: Install binary to /usr/local/bin
        run_step 3 do_install_binary || failed=1
    fi

    if [ $failed -eq 0 ]; then
        # Step 4: Enroll device
        run_step 4 do_enroll || failed=1
    fi

    if [ $failed -eq 0 ]; then
        if [ "$OS" = "darwin" ]; then
            # Step 5: Install mesh daemon
            run_step 5 do_install_meshd || failed=1

            # Step 6: Install and launch tray app (don't fail on this)
            if [ $failed -eq 0 ]; then
                run_step 6 do_install_tray_app || true
            fi
        elif [ "$OS" = "linux" ]; then
            # Step 5: Create systemd user service
            run_step 5 do_create_linux_service || true
        fi
    fi

    sleep 0.5

    if [ $failed -eq 0 ]; then
        show_completion
    else
        show_failure
        exit 1
    fi
}

main "$@"
