#!/bin/bash
#
# Dex Installer
#
# Downloads pre-built binaries and runs enrollment.
#
# Usage:
#   curl -fsSL https://get.enbox.id | bash
#   curl -fsSL https://get.enbox.id | bash -s -- --key dexkey-xxx
#   curl -fsSL https://get.enbox.id | bash -s -- dexkey-xxx
#
# Environment variables:
#   DEX_VERSION      - Version to install (default: latest)
#   DEX_INSTALL_DIR  - Binary install location (default: /usr/local/bin)
#   DEX_DATA_DIR     - Data directory (default: /opt/dex)
#   DEX_CENTRAL_URL  - Central server URL (default: https://central.enbox.id)
#   DEX_ENROLL_KEY   - Enrollment key (for non-interactive install)
#
set -e

# Installer version - update this when making changes to the installer
INSTALLER_VERSION="0.1.26"

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
INSTALL_DIR="${DEX_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DEX_DATA_DIR:-/opt/dex}"
CENTRAL_URL="${DEX_CENTRAL_URL:-https://central.enbox.id}"
ENROLL_KEY="${DEX_ENROLL_KEY:-}"

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
    "Checking permissions"
    "Downloading Dex"
    "Installing binary"
    "Setting up directories"
    "Enrolling with Central"
    "Creating system service"
)
STEP_COUNT=${#STEPS[@]}

# Step statuses: pending, running, success, failed, skip
declare -a STEP_STATUS
for ((i=0; i<STEP_COUNT; i++)); do
    STEP_STATUS[$i]="pending"
done

# Activity log (last few messages)
declare -a ACTIVITY_LOG
MAX_LOG_LINES=4

# Current step details
CURRENT_DETAIL=""

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
    │    ██║  ██║█████╗   ╚███╔╝                       │
    │    ██║  ██║██╔══╝   ██╔██╗                       │
    │    ██████╔╝███████╗██╔╝ ██╗                      │
    │    ╚═════╝ ╚══════╝╚═╝  ╚═╝                      │
    │                                                  │
    │         AI Coding Agents on Your Terms           │
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
        local symbol color prefix

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

    # Show last N log entries
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

# Log activity message
log_activity() {
    ACTIVITY_LOG+=("$1")
    draw_ui
}

# Update step status
update_step() {
    local step_index=$1
    local new_status=$2
    STEP_STATUS[$step_index]="$new_status"
    draw_ui
}

# Run a step with status tracking
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

# Skip a step
skip_step() {
    local step_index=$1
    local reason=$2
    update_step "$step_index" "skip"
    log_activity "Skipped: $reason"
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

# Step 1: Check permissions and system time
do_check_permissions() {
    SUDO=""
    if [ "$EUID" -ne 0 ]; then
        if [ -w "$INSTALL_DIR" ] && [ -w "$(dirname "$DATA_DIR")" ]; then
            log_activity "Permissions: OK (no sudo needed)"
        else
            log_activity "Permissions: Will use sudo for some operations"
            SUDO="sudo"
        fi
    else
        log_activity "Permissions: Running as root"
    fi

    # Check system time is reasonable (for TLS certificate validity)
    CURRENT_YEAR=$(date +%Y)
    if [ "$CURRENT_YEAR" -lt 2025 ] || [ "$CURRENT_YEAR" -gt 2030 ]; then
        log_activity "WARNING: System clock may be wrong (year=$CURRENT_YEAR)"
        log_activity "This can cause TLS certificate errors"
        log_activity "Please sync your system clock (e.g., timedatectl set-ntp true)"
    fi

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

    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_DIR/dex" "$INSTALL_DIR/dex"
    else
        log_activity "Installing to $INSTALL_DIR (sudo)..."
        $SUDO mv "$TMP_DIR/dex" "$INSTALL_DIR/dex"
    fi

    chmod +x "$INSTALL_DIR/dex"
    log_activity "Installed: $INSTALL_DIR/dex"
    return 0
}

# Step 4: Setup data directory
do_setup_data_dir() {
    if [ -w "$(dirname "$DATA_DIR")" ]; then
        mkdir -p "$DATA_DIR"
    else
        $SUDO mkdir -p "$DATA_DIR"
        $SUDO chown "$(id -u):$(id -g)" "$DATA_DIR"
    fi

    log_activity "Data directory: $DATA_DIR"
    return 0
}

# Step 5: Enrollment (interactive)
do_enroll() {
    # Skip if already enrolled
    if [ -f "$DATA_DIR/config.json" ]; then
        log_activity "Already enrolled (config.json exists)"
        return 0
    fi

    local key="$ENROLL_KEY"

    # If no key provided, we need to prompt (breaks the UI flow)
    if [ -z "$key" ]; then
        if [ -t 0 ]; then
            # Show prompt UI
            draw_ui
            echo ""
            echo -e "  ${BOLD}Enrollment Required${NC}"
            echo -e "  ${DIM}Get your key from: ${CYAN}https://enbox.id${NC}"
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

    if "$INSTALL_DIR/dex" enroll --key "$key" --data-dir "$DATA_DIR" --central-url "$CENTRAL_URL" >/dev/null 2>&1; then
        log_activity "Enrollment successful!"
        return 0
    else
        log_activity "Enrollment failed - run 'dex enroll' manually"
        return 1
    fi
}

# Step 6: Create system service
do_create_service() {
    if [ "$OS" = "linux" ]; then
        do_create_systemd_service
    elif [ "$OS" = "darwin" ]; then
        do_create_launchd_service
    else
        log_activity "No service manager for this OS"
    fi
    return 0
}

do_create_systemd_service() {
    if [ ! -d "/etc/systemd/system" ]; then
        log_activity "systemd not found, skipping"
        return 0
    fi

    local service_file="/etc/systemd/system/dex.service"
    local service_content="[Unit]
Description=Dex - AI Coding Agents
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$(id -un)
WorkingDirectory=$DATA_DIR
ExecStart=$INSTALL_DIR/dex start --base-dir $DATA_DIR
Restart=always
RestartSec=10
Environment=DEX_DATA_DIR=$DATA_DIR

[Install]
WantedBy=multi-user.target"

    if [ -w "/etc/systemd/system" ]; then
        echo "$service_content" > "$service_file"
    else
        echo "$service_content" | $SUDO tee "$service_file" > /dev/null
    fi

    $SUDO systemctl daemon-reload 2>/dev/null || true
    log_activity "Created systemd service"
}

do_create_launchd_service() {
    local plist_dir="$HOME/Library/LaunchAgents"
    local plist_file="$plist_dir/id.enbox.dex.plist"

    mkdir -p "$plist_dir"

    cat > "$plist_file" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>id.enbox.dex</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/dex</string>
        <string>start</string>
        <string>--base-dir</string>
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
        <key>DEX_DATA_DIR</key>
        <string>$DATA_DIR</string>
    </dict>
    <key>StandardOutPath</key>
    <string>$DATA_DIR/dex.log</string>
    <key>StandardErrorPath</key>
    <string>$DATA_DIR/dex.error.log</string>
</dict>
</plist>
EOF

    log_activity "Created launchd service"
}

# =============================================================================
# Completion Screen
# =============================================================================

show_completion() {
    clear_screen

    # Get installed dex version
    local dex_version
    dex_version=$("$INSTALL_DIR/dex" version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")

    echo -e "${GREEN}"
    cat << BANNER
    ╭──────────────────────────────────────────────────╮
    │                                                  │
    │    ██████╗ ███████╗██╗  ██╗                      │
    │    ██╔══██╗██╔════╝╚██╗██╔╝                      │
    │    ██║  ██║█████╗   ╚███╔╝                       │
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

    if [ -f "$DATA_DIR/config.json" ]; then
        local namespace=""
        if command -v jq &>/dev/null; then
            namespace=$(jq -r '.namespace // empty' "$DATA_DIR/config.json" 2>/dev/null || true)
        fi

        echo -e "    ${BOLD}Start Dex:${NC}"
        echo -e "      ${CYAN}dex start --base-dir $DATA_DIR${NC}"
        echo ""

        if [ "$OS" = "linux" ]; then
            echo -e "    ${BOLD}Or enable the service:${NC}"
            echo -e "      ${CYAN}sudo systemctl enable --now dex${NC}"
        elif [ "$OS" = "darwin" ]; then
            echo -e "    ${BOLD}Or load the service:${NC}"
            echo -e "      ${CYAN}launchctl load ~/Library/LaunchAgents/id.enbox.dex.plist${NC}"
        fi
        echo ""

        # Show public URL from config
        local public_url
        public_url=$(jq -r '.public_url // empty' "$DATA_DIR/config.json" 2>/dev/null || true)
        if [ -n "$public_url" ]; then
            echo -e "    ${BOLD}Access your Dex:${NC}"
            echo -e "      ${CYAN}${public_url}${NC}"
            echo ""
        fi
    else
        echo -e "    ${BOLD}Complete enrollment:${NC}"
        echo -e "      ${CYAN}dex enroll --data-dir $DATA_DIR${NC}"
        echo ""
        echo -e "    ${BOLD}Then start Dex:${NC}"
        echo -e "      ${CYAN}dex start --base-dir $DATA_DIR${NC}"
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
    │    ██║  ██║█████╗   ╚███╔╝                       │
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

    # Show last few log entries
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
    # Initial draw
    draw_ui

    # Run installation steps
    local failed=0

    run_step 0 do_detect_platform || failed=1
    [ $failed -eq 0 ] && run_step 1 do_check_permissions || failed=1
    [ $failed -eq 0 ] && run_step 2 do_download_binary || failed=1
    [ $failed -eq 0 ] && run_step 3 do_install_binary || failed=1
    [ $failed -eq 0 ] && run_step 4 do_setup_data_dir || failed=1
    [ $failed -eq 0 ] && run_step 5 do_enroll || true  # Don't fail on enrollment skip
    [ $failed -eq 0 ] && run_step 6 do_create_service || true  # Don't fail on service creation

    # Brief pause to show final state
    sleep 0.5

    # Show completion or failure screen
    if [ $failed -eq 0 ]; then
        show_completion
    else
        show_failure
        exit 1
    fi
}

main "$@"
