#!/usr/bin/env bash
#
# Build DexClient.app for macOS
#
# Usage: ./scripts/build-macos-app.sh [output-dir]
#
# This script builds a macOS application bundle for the Dex Client.
# It creates a proper .app structure that can be distributed to users.
#
# Requirements:
# - Go 1.21+
# - CGO_ENABLED=1 (for systray support)
# - macOS (for proper app bundling)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Output directory
OUTPUT_DIR="${1:-$PROJECT_ROOT/dist}"

# App metadata
APP_NAME="DexClient"
APP_BUNDLE="$OUTPUT_DIR/$APP_NAME.app"
BUNDLE_ID="id.enbox.dex-client"
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo "dev")}"

echo "Building $APP_NAME.app version $VERSION..."

# Create app bundle structure
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"

# Build the binary with CGO enabled for systray
echo "Compiling binary..."
cd "$PROJECT_ROOT"
CGO_ENABLED=1 go build -ldflags="-s -w -X main.version=$VERSION" -o "$APP_BUNDLE/Contents/MacOS/$APP_NAME" ./cmd/dex

# Create Info.plist
echo "Creating Info.plist..."
cat > "$APP_BUNDLE/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>$APP_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>$BUNDLE_ID</string>
    <key>CFBundleName</key>
    <string>Dex Client</string>
    <key>CFBundleDisplayName</key>
    <string>Dex Client</string>
    <key>CFBundleVersion</key>
    <string>$VERSION</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwithcing</key>
    <true/>
    <key>LSApplicationCategoryType</key>
    <string>public.app-category.utilities</string>
</dict>
</plist>
EOF

# Create a launcher script that runs 'dex client tray'
echo "Creating launcher..."
cat > "$APP_BUNDLE/Contents/MacOS/DexClient-Launcher" << 'EOF'
#!/bin/bash
# Launcher script for Dex Client
# This ensures the app runs in tray mode

SCRIPT_DIR="$(dirname "$0")"
exec "$SCRIPT_DIR/DexClient" client tray "$@"
EOF
chmod +x "$APP_BUNDLE/Contents/MacOS/DexClient-Launcher"

# Update Info.plist to use launcher
sed -i '' "s/<string>$APP_NAME<\/string>/<string>DexClient-Launcher<\/string>/" "$APP_BUNDLE/Contents/Info.plist"

# Generate app icon (simple colored circle, same as tray icon)
echo "Generating app icon..."
# For now, we'll create a placeholder. In production, use a proper icon.
# The systray library uses the programmatically generated icons.

# Create PkgInfo
echo -n "APPL????" > "$APP_BUNDLE/Contents/PkgInfo"

# Optional: Code sign if identity is available
if command -v codesign &> /dev/null; then
    SIGNING_IDENTITY="${CODESIGN_IDENTITY:-}"
    if [ -n "$SIGNING_IDENTITY" ]; then
        echo "Signing app bundle..."
        codesign --force --deep --sign "$SIGNING_IDENTITY" "$APP_BUNDLE"
    else
        echo "Skipping code signing (no CODESIGN_IDENTITY set)"
    fi
fi

echo ""
echo "Built: $APP_BUNDLE"
echo ""
echo "To install:"
echo "  cp -R '$APP_BUNDLE' ~/Applications/"
echo ""
echo "Or distribute the app to users."
