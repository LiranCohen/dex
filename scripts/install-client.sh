#!/bin/bash
#
# Dex Client Installer
#
# Downloads pre-built binaries and runs client enrollment for local mesh access.
# Installs to user-local directories (no root/sudo required).
#
# Usage:
#   curl -fsSL https://get.enbox.id/client | sh
#   curl -fsSL https://get.enbox.id/client | sh -s -- --key dexkey-xxx
#   curl -fsSL https://get.enbox.id/client | sh -s -- dexkey-xxx
#
# Environment variables:
#   DEX_VERSION           - Version to install (default: latest)
#   DEX_CLIENT_INSTALL_DIR - Binary install location (default: ~/.local/bin)
#   DEX_CLIENT_DATA_DIR   - Data directory (default: ~/.dex)
#   DEX_CENTRAL_URL       - Central server URL (default: https://central.enbox.id)
#   DEX_ENROLL_KEY        - Enrollment key (for non-interactive install)
#
set -e

# Installer version
INSTALLER_VERSION="0.1.0"

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
INSTALL_DIR="${DEX_CLIENT_INSTALL_DIR:-$HOME/.local/bin}"
DATA_DIR="${DEX_CLIENT_DATA_DIR:-$HOME/.dex}"
CENTRAL_URL="${DEX_CENTRAL_URL:-https://central.enbox.id}"
ENROLL_KEY="${DEX_ENROLL_KEY:-}"
HOSTNAME_ARG="${DEX_HOSTNAME:-}"

# =============================================================================
# Terminal UI
# =============================================================================

# Colors
BLACK='\033[0;30m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GRAY='\033[0;90m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Symbols
SYM_PENDING="○"
SYM_RUNNING="●"
SYM_SUCCESS="✔"
SYM_FAILED="✖"
SYM_SKIP="◌"

# Steps definition
STEPS=(
    "Detecting platform"
    "Checking directories"
    "Downloading Dex"
    "Installing binary"
    "Enrolling device"
    "Creating user service"
)
STEP_COUNT=${#STEPS[@]}

# Step statuses: pending, running, success, failed, skip
declare -a STEP_STATUS
for ((i=0; i<STEP_COUNT; i++)); do
    STEP_STATUS[$i]="pending"
done

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

    log_activity "Platform: ${OS}/${ARCH}"
    return 0
}

# Step 1: Check directories
do_check_directories() {
    # Create install directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        mkdir -p "$INSTALL_DIR"
        log_activity "Created: $INSTALL_DIR"
    fi

    # Create data directory if needed
    if [ ! -d "$DATA_DIR" ]; then
        mkdir -p "$DATA_DIR"
        log_activity "Created: $DATA_DIR"
    fi

    # Ensure install dir is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *)
            log_activity "Note: Add $INSTALL_DIR to your PATH"
            ;;
    esac

    log_activity "Directories ready"
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
    trap "rm -rf $TMP_DIR" EXIT

    log_activity "Downloading from GitHub releases..."

    if command -v curl &> /dev/null; then
        if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/dex.tar.gz" 2>/dev/null; then
            log_activity "Error: Download failed"
            return 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/dex.tar.gz" 2>/dev/null; then
            log_activity "Error: Download failed"
            return 1
        fi
    else
        log_activity "Error: Neither curl nor wget found"
        return 1
    fi

    log_activity "Download complete"
    return 0
}

# Step 3: Install binary
do_install_binary() {
    log_activity "Extracting archive..."
    tar -xzf "$TMP_DIR/dex.tar.gz" -C "$TMP_DIR"

    mv "$TMP_DIR/dex" "$INSTALL_DIR/dex"
    chmod +x "$INSTALL_DIR/dex"

    log_activity "Installed: $INSTALL_DIR/dex"
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
            read -p "  Enter enrollment key (or press Enter to skip): " key

            if [ -z "$key" ]; then
                log_activity "Enrollment skipped (no key provided)"
                return 0
            fi
        else
            log_activity "No enrollment key provided (non-interactive)"
            return 0
        fi
    fi

    log_activity "Enrolling with Central..."

    local hostname_arg=""
    if [ -n "$HOSTNAME_ARG" ]; then
        hostname_arg="--hostname $HOSTNAME_ARG"
    fi

    if "$INSTALL_DIR/dex" client enroll --key "$key" --data-dir "$DATA_DIR" --central-url "$CENTRAL_URL" $hostname_arg >/dev/null 2>&1; then
        log_activity "Enrollment successful!"
        return 0
    else
        log_activity "Enrollment failed - run 'dex client enroll' manually"
        return 1
    fi
}

# Step 5: Create user service
do_create_service() {
    if [ "$OS" = "linux" ]; then
        do_create_systemd_user_service
    elif [ "$OS" = "darwin" ]; then
        do_create_launchd_user_service
    else
        log_activity "No service manager for this OS"
    fi
    return 0
}

do_create_systemd_user_service() {
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
}

do_create_launchd_user_service() {
    local plist_dir="$HOME/Library/LaunchAgents"
    local plist_file="$plist_dir/id.enbox.dex-client.plist"

    mkdir -p "$plist_dir"

    cat > "$plist_file" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>id.enbox.dex-client</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/dex</string>
        <string>client</string>
        <string>start</string>
        <string>--data-dir</string>
        <string>$DATA_DIR</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>$DATA_DIR</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>DEX_CLIENT_DATA_DIR</key>
        <string>$DATA_DIR</string>
    </dict>
    <key>StandardOutPath</key>
    <string>$DATA_DIR/dex-client.log</string>
    <key>StandardErrorPath</key>
    <string>$DATA_DIR/dex-client.error.log</string>
</dict>
</plist>
EOF

    log_activity "Created launchd user service"
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

    echo -e "  ${BOLD}Next Steps${NC}"
    echo -e "  ${DIM}─────────────────────────────────────────────────${NC}"
    echo ""

    # Check if ~/.local/bin is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *)
            echo -e "    ${BOLD}Add to your PATH:${NC}"
            echo -e "      ${CYAN}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
            echo ""
            ;;
    esac

    if [ -f "$DATA_DIR/config.json" ]; then
        local namespace
        namespace=$(grep -o '"namespace"[[:space:]]*:[[:space:]]*"[^"]*"' "$DATA_DIR/config.json" 2>/dev/null | cut -d'"' -f4 || true)

        echo -e "    ${BOLD}Start the mesh client:${NC}"
        echo -e "      ${CYAN}dex client start${NC}"
        echo ""

        if [ "$OS" = "linux" ]; then
            echo -e "    ${BOLD}Or enable auto-start on login:${NC}"
            echo -e "      ${CYAN}systemctl --user start dex-client${NC}"
        elif [ "$OS" = "darwin" ]; then
            echo -e "    ${BOLD}Or enable auto-start on login:${NC}"
            echo -e "      ${CYAN}launchctl load ~/Library/LaunchAgents/id.enbox.dex-client.plist${NC}"
        fi
        echo ""

        if [ -n "$namespace" ]; then
            echo -e "    ${BOLD}Connected to namespace:${NC} ${CYAN}${namespace}${NC}"
            echo ""
        fi
    else
        echo -e "    ${BOLD}Complete enrollment:${NC}"
        echo -e "      ${CYAN}dex client enroll --key YOUR_KEY${NC}"
        echo ""
        echo -e "    ${BOLD}Then start the client:${NC}"
        echo -e "      ${CYAN}dex client start${NC}"
        echo ""
    fi

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
    draw_ui

    local failed=0

    run_step 0 do_detect_platform || failed=1
    [ $failed -eq 0 ] && run_step 1 do_check_directories || failed=1
    [ $failed -eq 0 ] && run_step 2 do_download_binary || failed=1
    [ $failed -eq 0 ] && run_step 3 do_install_binary || failed=1
    [ $failed -eq 0 ] && run_step 4 do_enroll || true  # Don't fail on enrollment skip
    [ $failed -eq 0 ] && run_step 5 do_create_service || true  # Don't fail on service creation

    sleep 0.5

    if [ $failed -eq 0 ]; then
        show_completion
    else
        show_failure
        exit 1
    fi
}

main "$@"
