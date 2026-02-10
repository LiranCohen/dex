#!/usr/bin/env bash
#
# Build Dex.pkg — a standard macOS installer package
#
# Usage: ./scripts/build-macos-pkg.sh [output-dir]
#
# Prerequisites:
#   - The dex binary must already be built (at ./dex or via build-macos-app.sh)
#   - DexClient.app must already be built (at dist/DexClient.app or via build-macos-app.sh)
#   - macOS with pkgbuild and productbuild (included with Xcode CLT)
#
# This creates a .pkg installer that:
#   1. Installs /usr/local/bin/dex (the CLI + daemon binary)
#   2. Installs /Library/LaunchDaemons/com.dex.meshd.plist
#   3. Installs /Applications/DexClient.app
#   4. Stops any existing meshd daemon (preinstall)
#   5. Loads the meshd daemon and launches DexClient.app (postinstall)
#
# Optional: set CODESIGN_IDENTITY to sign the .pkg for notarization.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="${1:-$PROJECT_ROOT/dist}"

VERSION="${VERSION:-$(git -C "$PROJECT_ROOT" describe --tags --always 2>/dev/null || echo "dev")}"
VERSION="${VERSION#v}" # strip leading 'v' for pkg version

IDENTIFIER="id.enbox.dex"
APP_NAME="DexClient"
PKG_NAME="Dex-${VERSION}.pkg"

# Paths to pre-built artifacts
DEX_BINARY="${DEX_BINARY:-$PROJECT_ROOT/dex}"
APP_BUNDLE="${APP_BUNDLE:-$PROJECT_ROOT/dist/DexClient.app}"

echo "==> Building Dex.pkg version $VERSION"

# Verify prerequisites
if [ ! -f "$DEX_BINARY" ]; then
    echo "ERROR: dex binary not found at $DEX_BINARY"
    echo "  Build it first: go build -ldflags=\"-s -w\" -o dex ./cmd/dex"
    exit 1
fi

if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: DexClient.app not found at $APP_BUNDLE"
    echo "  Build it first: ./scripts/build-macos-app.sh dist"
    exit 1
fi

# Create staging directories
STAGING="$(mktemp -d)"
trap 'rm -rf "$STAGING"' EXIT

STAGE_DAEMON="$STAGING/daemon-root"
STAGE_APP="$STAGING/app-root"
STAGE_SCRIPTS_PRE="$STAGING/scripts-daemon"
STAGE_SCRIPTS_POST="$STAGING/scripts-app"
STAGE_PKGS="$STAGING/packages"
STAGE_RESOURCES="$STAGING/resources"

mkdir -p "$STAGE_DAEMON/usr/local/bin"
mkdir -p "$STAGE_DAEMON/Library/LaunchDaemons"
mkdir -p "$STAGE_APP/Applications"
mkdir -p "$STAGE_SCRIPTS_PRE"
mkdir -p "$STAGE_SCRIPTS_POST"
mkdir -p "$STAGE_PKGS"
mkdir -p "$STAGE_RESOURCES"
mkdir -p "$OUTPUT_DIR"

# --- Stage the daemon component (binary + plist) ---
echo "==> Staging daemon component..."
cp "$DEX_BINARY" "$STAGE_DAEMON/usr/local/bin/dex"
chmod 755 "$STAGE_DAEMON/usr/local/bin/dex"

# LaunchDaemon plist (matches meshd_install_darwin.go exactly)
cat > "$STAGE_DAEMON/Library/LaunchDaemons/com.dex.meshd.plist" << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>

  <key>Label</key>
  <string>com.dex.meshd</string>

  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/dex</string>
    <string>meshd</string>
  </array>

  <key>RunAtLoad</key>
  <true/>

  <key>StandardErrorPath</key>
  <string>/var/log/dex-meshd.log</string>

  <key>StandardOutPath</key>
  <string>/var/log/dex-meshd.log</string>

</dict>
</plist>
PLIST

# --- Preinstall script: stop existing daemon if upgrading ---
echo "==> Creating preinstall script..."
cat > "$STAGE_SCRIPTS_PRE/preinstall" << 'SCRIPT'
#!/bin/bash
# Stop and unload existing meshd daemon if present (upgrade path)
PLIST="/Library/LaunchDaemons/com.dex.meshd.plist"
SERVICE="com.dex.meshd"

if launchctl list "$SERVICE" &>/dev/null; then
    echo "Stopping existing meshd daemon..."
    launchctl stop "$SERVICE" 2>/dev/null || true
    launchctl unload "$PLIST" 2>/dev/null || true
fi
exit 0
SCRIPT
chmod 755 "$STAGE_SCRIPTS_PRE/preinstall"

# --- Postinstall script: load daemon + open app ---
echo "==> Creating postinstall script..."
cat > "$STAGE_SCRIPTS_POST/postinstall" << 'SCRIPT'
#!/bin/bash
# Create socket directory
mkdir -p /var/run

# Load and start the meshd daemon
PLIST="/Library/LaunchDaemons/com.dex.meshd.plist"
SERVICE="com.dex.meshd"

echo "Loading meshd daemon..."
launchctl load "$PLIST" 2>/dev/null || true
launchctl start "$SERVICE" 2>/dev/null || true

# Launch DexClient.app for the installing user (not root)
# The installer runs as root, so we need to find the console user
CONSOLE_USER=$(stat -f '%Su' /dev/console 2>/dev/null || echo "")
if [ -n "$CONSOLE_USER" ] && [ "$CONSOLE_USER" != "root" ]; then
    su "$CONSOLE_USER" -c 'open "/Applications/DexClient.app"' 2>/dev/null || true
fi

exit 0
SCRIPT
chmod 755 "$STAGE_SCRIPTS_POST/postinstall"

# --- Stage the app component ---
echo "==> Staging app component..."
cp -R "$APP_BUNDLE" "$STAGE_APP/Applications/$APP_NAME.app"

# --- Build component packages ---
echo "==> Building daemon component package..."
pkgbuild \
    --root "$STAGE_DAEMON" \
    --identifier "${IDENTIFIER}.daemon" \
    --version "$VERSION" \
    --scripts "$STAGE_SCRIPTS_PRE" \
    "$STAGE_PKGS/daemon.pkg"

echo "==> Building app component package..."
pkgbuild \
    --root "$STAGE_APP" \
    --identifier "${IDENTIFIER}.app" \
    --version "$VERSION" \
    --scripts "$STAGE_SCRIPTS_POST" \
    "$STAGE_PKGS/app.pkg"

# --- Create distribution XML ---
echo "==> Creating distribution descriptor..."
cat > "$STAGING/distribution.xml" << DIST
<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="2">
    <title>Dex Client</title>
    <organization>${IDENTIFIER}</organization>
    <domains enable_localSystem="true"/>
    <options customize="never" require-scripts="true" rootVolumeOnly="true"/>

    <!-- Require macOS 11+ (Big Sur) for TUN device support -->
    <volume-check>
        <allowed-os-versions>
            <os-version min="11.0"/>
        </allowed-os-versions>
    </volume-check>

    <welcome file="welcome.html" mime-type="text/html"/>
    <conclusion file="conclusion.html" mime-type="text/html"/>

    <choices-outline>
        <line choice="daemon"/>
        <line choice="app"/>
    </choices-outline>

    <choice id="daemon" title="Dex Mesh Daemon"
            description="Installs the dex binary and mesh networking daemon."
            visible="false">
        <pkg-ref id="${IDENTIFIER}.daemon"/>
    </choice>

    <choice id="app" title="DexClient Application"
            description="Installs the DexClient menu bar app."
            visible="false">
        <pkg-ref id="${IDENTIFIER}.app"/>
    </choice>

    <pkg-ref id="${IDENTIFIER}.daemon" version="${VERSION}" onConclusion="none">daemon.pkg</pkg-ref>
    <pkg-ref id="${IDENTIFIER}.app" version="${VERSION}" onConclusion="none">app.pkg</pkg-ref>
</installer-gui-script>
DIST

# --- Create welcome and conclusion HTML ---
cat > "$STAGE_RESOURCES/welcome.html" << 'HTML'
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; padding: 20px; }
        h1 { font-size: 22px; font-weight: 600; }
        p { font-size: 14px; color: #333; line-height: 1.5; }
        .highlight { color: #007AFF; font-weight: 500; }
    </style>
</head>
<body>
    <h1>Dex Client</h1>
    <p>This installer will set up your Dex mesh networking client.</p>
    <p>The following components will be installed:</p>
    <ul>
        <li><span class="highlight">dex</span> command-line tool in <code>/usr/local/bin</code></li>
        <li><span class="highlight">DexClient.app</span> menu bar application</li>
        <li><span class="highlight">Mesh daemon</span> for secure networking</li>
    </ul>
    <p>Administrator privileges are required to install the mesh networking daemon.</p>
</body>
</html>
HTML

cat > "$STAGE_RESOURCES/conclusion.html" << 'HTML'
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; padding: 20px; }
        h1 { font-size: 22px; font-weight: 600; }
        p { font-size: 14px; color: #333; line-height: 1.5; }
        .success { color: #34C759; font-weight: 600; }
        code { background: #f0f0f0; padding: 2px 6px; border-radius: 4px; font-size: 13px; }
    </style>
</head>
<body>
    <h1 class="success">Installation Complete</h1>
    <p>DexClient is now running in your menu bar. Look for the Dex icon in the top-right corner of your screen.</p>
    <p>Click the icon and choose <strong>Sign Up</strong> or <strong>Sign In</strong> to connect to the mesh network.</p>
    <p>You can also use the command line:</p>
    <ul>
        <li><code>dex client enroll --key dexkey-xxx</code> to enroll</li>
        <li><code>dex client start</code> to connect</li>
    </ul>
</body>
</html>
HTML

# --- Build the final product .pkg ---
echo "==> Building final installer package..."

if [ -n "${CODESIGN_IDENTITY:-}" ]; then
    echo "  Signing with: $CODESIGN_IDENTITY"
    productbuild \
        --distribution "$STAGING/distribution.xml" \
        --package-path "$STAGE_PKGS" \
        --resources "$STAGE_RESOURCES" \
        --sign "$CODESIGN_IDENTITY" \
        "$OUTPUT_DIR/$PKG_NAME"
else
    productbuild \
        --distribution "$STAGING/distribution.xml" \
        --package-path "$STAGE_PKGS" \
        --resources "$STAGE_RESOURCES" \
        "$OUTPUT_DIR/$PKG_NAME"
fi

echo ""
echo "==> Built: $OUTPUT_DIR/$PKG_NAME"
echo ""
echo "    Size: $(du -h "$OUTPUT_DIR/$PKG_NAME" | cut -f1)"
echo ""
echo "To install: open \"$OUTPUT_DIR/$PKG_NAME\""
echo ""
if [ -z "${CODESIGN_IDENTITY:-}" ]; then
    echo "NOTE: Package is unsigned. Users will see a Gatekeeper warning."
    echo "  To sign: CODESIGN_IDENTITY='Developer ID Installer: ...' $0"
fi
