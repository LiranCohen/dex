# E2E Integration: HQ Side Implementation Plan

**Status**: MVP Complete (HQ Side)
**Created**: 2026-02-06
**Last Updated**: 2026-02-06
**Target**: MVP Critical Path

## Implementation Status

| Item | Status | Notes |
|------|--------|-------|
| HQ-MVP-01: Enrollment Command | **COMPLETE** | `cmd/dex/enroll.go`, `cmd/dex/config.go` |
| HQ-MVP-02: Load Config on Startup | **COMPLETE** | Modified `cmd/dex/main.go` |
| HQ-MVP-03: Update Install Script | **COMPLETE** | `scripts/install.sh` simplified |
| HQ-MVP-04: GitHub Release Automation | **COMPLETE** | `.github/workflows/release.yml` |

All HQ-side MVP items are implemented. Central-side work is complete. Only remaining gap is Edge token verification with Central (tracked in `dex-saas/hq-plan/SAAS-E2E.md`).

## Overview

This document specifies the HQ-side work needed for the MVP end-to-end flow where external users can:
1. Sign up on Central (enbox.id)
2. Get an enrollment key
3. Install HQ on their server
4. Run `dex enroll` to connect to the mesh
5. Access their Dex at `https://<namespace>.enbox.id`

**Coordination**: Central-side work is tracked in `dex-saas/hq-plan/SAAS-E2E.md`

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         USER FLOW                                    │
│                                                                      │
│  1. User signs up at https://enbox.id (Central)                     │
│  2. Gets enrollment key from dashboard                               │
│  3. Runs: curl -fsSL https://get.enbox.id | sh                      │
│  4. Enters enrollment key when prompted                              │
│  5. HQ enrolls with Central, gets mesh credentials                   │
│  6. HQ starts, connects to mesh, establishes tunnel                  │
│  7. User accesses https://alice.enbox.id                            │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                      COMPONENT INTERACTION                           │
│                                                                      │
│   HQ (dex)                    Central                    Edge        │
│      │                           │                         │         │
│      │  POST /api/v1/enroll      │                         │         │
│      │  {key: "dexkey-..."}      │                         │         │
│      │ ─────────────────────────►│                         │         │
│      │                           │                         │         │
│      │  {namespace, mesh_ip,     │                         │         │
│      │   noise_key, token, ...}  │                         │         │
│      │ ◄─────────────────────────│                         │         │
│      │                           │                         │         │
│      │  Connect to mesh          │                         │         │
│      │ ─────────────────────────►│                         │         │
│      │                           │                         │         │
│      │  TCP :9443 HELLO          │                         │         │
│      │ ────────────────────────────────────────────────────►│        │
│      │                           │  Verify token           │         │
│      │                           │◄─────────────────────────│        │
│      │  HELLO_ACK                │                         │         │
│      │ ◄────────────────────────────────────────────────────│        │
│      │                           │                         │         │
│      │  ACME DNS-01              │                         │         │
│      │ ─────────────────────────►│ (sets TXT record)       │         │
│      │                           │                         │         │
│      │  ✅ Ready at https://alice.enbox.id                 │         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## MVP Work Items

### HQ-MVP-01: Enrollment Command
**Priority**: CRITICAL
**Effort**: Medium
**Depends on**: Central enrollment API (CENTRAL-MVP-02)
**Status**: COMPLETE

Create a new `dex enroll` command that:
1. Takes an enrollment key as input
2. Calls Central's enrollment API
3. Receives and saves configuration
4. Prepares HQ for first start

#### Command Interface

```bash
# Interactive mode
dex enroll
# Prompts: "Enter your enrollment key: "

# Non-interactive mode
dex enroll --key dexkey-alice-a1b2c3d4

# With custom data directory
dex enroll --key dexkey-alice-a1b2c3d4 --data-dir /opt/dex
```

#### Implementation

**File**: `cmd/dex/enroll.go` (new file)

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
)

// EnrollmentRequest is sent to Central
type EnrollmentRequest struct {
    Key      string `json:"key"`
    Hostname string `json:"hostname"` // optional, defaults to OS hostname
}

// EnrollmentResponse is returned by Central
type EnrollmentResponse struct {
    // Account/Network info
    AccountID   string `json:"account_id"`
    NetworkID   string `json:"network_id"`
    Namespace   string `json:"namespace"`    // e.g., "alice"
    PublicURL   string `json:"public_url"`   // e.g., "https://alice.enbox.id"
    ControlURL  string `json:"control_url"`  // Central URL for dexnet mesh
    TunnelToken string `json:"tunnel_token"` // JWT for tunnel authentication

    // Mesh configuration
    Mesh struct {
        ControlURL string `json:"control_url"` // https://central.enbox.id
        AuthKey    string `json:"auth_key"`    // Headscale pre-auth key
    } `json:"mesh"`

    // Tunnel configuration
    Tunnel struct {
        IngressAddr string `json:"ingress_addr"` // ingress.enbox.id:9443
        Token       string `json:"token"`        // JWT for Edge authentication
    } `json:"tunnel"`

    // ACME configuration
    ACME struct {
        Email  string `json:"email"`
        DNSAPI string `json:"dns_api"` // https://central.enbox.id/api/v1/dns/acme-challenge
    } `json:"acme"`
}

const (
    DefaultCentralURL = "https://central.enbox.id"
    DefaultDataDir    = "/opt/dex"
)

func runEnroll(keyFlag string, dataDirFlag string, centralURL string) error {
    // 1. Get enrollment key
    key := keyFlag
    if key == "" {
        fmt.Print("Enter your enrollment key: ")
        reader := bufio.NewReader(os.Stdin)
        input, err := reader.ReadString('\n')
        if err != nil {
            return fmt.Errorf("failed to read input: %w", err)
        }
        key = strings.TrimSpace(input)
    }

    if key == "" {
        return fmt.Errorf("enrollment key is required")
    }

    // 2. Determine data directory
    dataDir := dataDirFlag
    if dataDir == "" {
        dataDir = DefaultDataDir
    }

    // 3. Create data directory if needed
    if err := os.MkdirAll(dataDir, 0755); err != nil {
        return fmt.Errorf("failed to create data directory: %w", err)
    }

    // 4. Get hostname
    hostname, _ := os.Hostname()

    // 5. Call Central enrollment API
    fmt.Println("Enrolling with Central...")

    if centralURL == "" {
        centralURL = DefaultCentralURL
    }

    reqBody := EnrollmentRequest{
        Key:      key,
        Hostname: hostname,
    }

    resp, err := callEnrollmentAPI(centralURL, reqBody)
    if err != nil {
        return fmt.Errorf("enrollment failed: %w", err)
    }

    // 6. Save configuration
    config := buildConfig(resp)
    configPath := filepath.Join(dataDir, "config.json")

    configBytes, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal config: %w", err)
    }

    if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
        return fmt.Errorf("failed to save config: %w", err)
    }

    // 7. Print success
    fmt.Println()
    fmt.Println("✅ Enrollment successful!")
    fmt.Println()
    fmt.Printf("   Namespace:  %s\n", resp.Namespace)
    fmt.Printf("   Public URL: %s\n", resp.PublicURL)
    fmt.Printf("   Config:     %s\n", configPath)
    fmt.Println()
    fmt.Println("Start Dex with:")
    fmt.Printf("   dex start --data-dir %s\n", dataDir)
    fmt.Println()

    return nil
}

func callEnrollmentAPI(centralURL string, req EnrollmentRequest) (*EnrollmentResponse, error) {
    // Implementation: HTTP POST to centralURL/api/v1/enroll
    // See full implementation below
}

func buildConfig(resp *EnrollmentResponse) map[string]interface{} {
    return map[string]interface{}{
        "namespace":  resp.Namespace,
        "public_url": resp.PublicURL,
        "mesh": map[string]interface{}{
            "enabled":     true,
            "control_url": resp.Mesh.ControlURL,
            "auth_key":    resp.Mesh.AuthKey,
        },
        "tunnel": map[string]interface{}{
            "enabled":      true,
            "ingress_addr": resp.Tunnel.IngressAddr,
            "token":        resp.Tunnel.Token,
            "endpoints": []map[string]interface{}{
                {
                    "hostname":   resp.Namespace + ".enbox.id",
                    "local_port": 8080,
                },
            },
        },
        "acme": map[string]interface{}{
            "enabled": true,
            "email":   resp.ACME.Email,
            "dns_api": resp.ACME.DNSAPI,
        },
    }
}
```

**File**: `cmd/dex/main.go` (add enroll subcommand)

```go
// In main(), use standard library flag package with subcommands:

func main() {
    if len(os.Args) < 2 {
        printUsage()
        os.Exit(1)
    }

    switch os.Args[1] {
    case "enroll":
        if err := runEnroll(os.Args[2:]); err != nil {
            log.Fatal().Err(err).Msg("Enrollment failed")
        }
    case "start":
        runStart(os.Args[2:])
    case "version":
        fmt.Println("dex version", version)
    default:
        printUsage()
        os.Exit(1)
    }
}
```

#### Testing

```bash
# Unit test
go test ./cmd/dex -run TestEnroll

# Integration test (requires Central running)
dex enroll --key test-key-123 --data-dir /tmp/dex-test --central-url http://localhost:8080

# Verify config saved
cat /tmp/dex-test/config.json
```

---

### HQ-MVP-02: Load Config on Startup
**Priority**: CRITICAL
**Effort**: Small
**Depends on**: HQ-MVP-01
**Status**: COMPLETE

The main `dex start` command must load `config.json` saved by `dex enroll`.

#### Current Behavior
- Only reads CLI flags (`-mesh-control-url`, `-mesh-auth-key`, etc.)
- User must pass all flags manually

#### Required Behavior
- Check for `config.json` in data directory
- Load and apply configuration
- CLI flags override config file values
- Start mesh and tunnel automatically if configured

#### Implementation

**File**: `cmd/dex/main.go` (modify start command)

```go
func runStart(dataDir string, cliFlags StartFlags) error {
    // 1. Try to load config file
    configPath := filepath.Join(dataDir, "config.json")
    var config Config

    if data, err := os.ReadFile(configPath); err == nil {
        if err := json.Unmarshal(data, &config); err != nil {
            return fmt.Errorf("invalid config.json: %w", err)
        }
        log.Info().Str("path", configPath).Msg("Loaded configuration")
    } else if !os.IsNotExist(err) {
        return fmt.Errorf("failed to read config: %w", err)
    }

    // 2. CLI flags override config
    if cliFlags.MeshControlURL != "" {
        config.Mesh.ControlURL = cliFlags.MeshControlURL
    }
    // ... other overrides

    // 3. Validate required config
    if config.Mesh.Enabled && config.Mesh.ControlURL == "" {
        return fmt.Errorf("mesh enabled but control_url not set - run 'dex enroll' first")
    }

    // 4. Initialize components
    app := NewApp(dataDir)

    // 5. Start mesh if enabled
    if config.Mesh.Enabled {
        meshClient, err := mesh.NewClient(mesh.Config{
            ControlURL: config.Mesh.ControlURL,
            AuthKey:    config.Mesh.AuthKey,
            StateDir:   filepath.Join(dataDir, "mesh"),
            Hostname:   config.Hostname,
        })
        if err != nil {
            return fmt.Errorf("failed to create mesh client: %w", err)
        }
        app.meshClient = meshClient
    }

    // 6. Start tunnel if enabled
    if config.Tunnel.Enabled {
        tunnelClient, err := mesh.NewTunnelClient(mesh.TunnelConfig{
            IngressAddr: config.Tunnel.IngressAddr,
            Token:       config.Tunnel.Token,
            Endpoints:   config.Tunnel.Endpoints,
            ACMEEmail:   config.ACME.Email,
            ACMEDNSAPI:  config.ACME.DNSAPI,
        })
        if err != nil {
            return fmt.Errorf("failed to create tunnel client: %w", err)
        }
        app.tunnelClient = tunnelClient
    }

    // 7. Start the application
    return app.Run()
}
```

**File**: `cmd/dex/config.go` (new file)

```go
package main

// Config represents the HQ configuration saved by 'dex enroll'
type Config struct {
    Namespace string `json:"namespace"`
    PublicURL string `json:"public_url"`
    Hostname  string `json:"hostname,omitempty"`

    Mesh   MeshConfig   `json:"mesh"`
    Tunnel TunnelConfig `json:"tunnel"`
    ACME   ACMEConfig   `json:"acme"`
}

type MeshConfig struct {
    Enabled    bool   `json:"enabled"`
    ControlURL string `json:"control_url"`
    AuthKey    string `json:"auth_key"`
}

type TunnelConfig struct {
    Enabled     bool              `json:"enabled"`
    IngressAddr string            `json:"ingress_addr"`
    Token       string            `json:"token"`
    Endpoints   []EndpointConfig  `json:"endpoints"`
}

type EndpointConfig struct {
    Hostname  string `json:"hostname"`
    LocalPort int    `json:"local_port"`
}

type ACMEConfig struct {
    Enabled bool   `json:"enabled"`
    Email   string `json:"email"`
    DNSAPI  string `json:"dns_api"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var config Config
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

---

### HQ-MVP-03: Update Install Script
**Priority**: HIGH
**Effort**: Small
**Depends on**: HQ-MVP-01, HQ-MVP-02
**Status**: COMPLETE

Replace the current install script that installs Cloudflared/Tailscale with a simplified version.

#### Current Script Issues
- Installs Cloudflared (not needed)
- Installs Tailscale (not needed - we use dexnet)
- Builds from source (slow)
- Complex setup wizard

#### New Script Requirements
- Download pre-built binary
- Create data directory
- Prompt for enrollment key
- Run `dex enroll`
- Start `dex`

#### Implementation

**File**: `scripts/install.sh` (replace entirely)

```bash
#!/bin/bash
set -e

# Dex Installer
# Usage: curl -fsSL https://get.enbox.id | sh

VERSION="${DEX_VERSION:-latest}"
INSTALL_DIR="${DEX_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DEX_DATA_DIR:-/opt/dex}"
CENTRAL_URL="${DEX_CENTRAL_URL:-https://central.enbox.id}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
echo_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
echo_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Banner
cat << 'EOF'

  ██████╗ ███████╗██╗  ██╗
  ██╔══██╗██╔════╝╚██╗██╔╝
  ██║  ██║█████╗   ╚███╔╝
  ██║  ██║██╔══╝   ██╔██╗
  ██████╔╝███████╗██╔╝ ██╗
  ╚═════╝ ╚══════╝╚═╝  ╚═╝

  AI Coding Agents on Your Terms

EOF

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        arm64)   ARCH="arm64" ;;
        *)       echo_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      echo_error "Unsupported OS: $OS"; exit 1 ;;
    esac

    echo_info "Detected platform: ${OS}/${ARCH}"
}

# Download and install binary
install_binary() {
    echo_info "Downloading Dex ${VERSION}..."

    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="https://releases.enbox.id/dex-latest-${OS}-${ARCH}.tar.gz"
    else
        DOWNLOAD_URL="https://releases.enbox.id/dex-${VERSION}-${OS}-${ARCH}.tar.gz"
    fi

    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT

    if command -v curl &> /dev/null; then
        curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/dex.tar.gz"
    elif command -v wget &> /dev/null; then
        wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/dex.tar.gz"
    else
        echo_error "Neither curl nor wget found. Please install one."
        exit 1
    fi

    tar -xzf "$TMP_DIR/dex.tar.gz" -C "$TMP_DIR"

    # Install binary (may need sudo)
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_DIR/dex" "$INSTALL_DIR/dex"
    else
        echo_info "Installing to $INSTALL_DIR (requires sudo)..."
        sudo mv "$TMP_DIR/dex" "$INSTALL_DIR/dex"
    fi

    chmod +x "$INSTALL_DIR/dex"
    echo_info "Installed dex to $INSTALL_DIR/dex"
}

# Create data directory
setup_data_dir() {
    echo_info "Setting up data directory at $DATA_DIR..."

    if [ -w "$(dirname $DATA_DIR)" ]; then
        mkdir -p "$DATA_DIR"
    else
        sudo mkdir -p "$DATA_DIR"
        sudo chown "$(id -u):$(id -g)" "$DATA_DIR"
    fi
}

# Enroll with Central
enroll() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  To complete installation, you need an enrollment key."
    echo "  Get one from your dashboard at: https://enbox.id"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    read -p "Enter your enrollment key (or press Enter to skip): " ENROLL_KEY

    if [ -n "$ENROLL_KEY" ]; then
        echo_info "Enrolling with Central..."
        "$INSTALL_DIR/dex" enroll --key "$ENROLL_KEY" --data-dir "$DATA_DIR" --central-url "$CENTRAL_URL"
    else
        echo_warn "Skipping enrollment. Run 'dex enroll' later to complete setup."
    fi
}

# Create systemd service (Linux only)
create_systemd_service() {
    if [ "$OS" != "linux" ]; then
        return
    fi

    if [ ! -d "/etc/systemd/system" ]; then
        echo_warn "systemd not found, skipping service creation"
        return
    fi

    echo_info "Creating systemd service..."

    sudo tee /etc/systemd/system/dex.service > /dev/null << EOF
[Unit]
Description=Dex - AI Coding Agents
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$(id -un)
ExecStart=$INSTALL_DIR/dex start --data-dir $DATA_DIR
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    echo_info "Systemd service created. Enable with: sudo systemctl enable --now dex"
}

# Print next steps
print_next_steps() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  ✅ Dex installed successfully!"
    echo ""

    if [ -f "$DATA_DIR/config.json" ]; then
        echo "  Start Dex with:"
        echo "    dex start --data-dir $DATA_DIR"
        echo ""
        echo "  Or enable the systemd service:"
        echo "    sudo systemctl enable --now dex"
    else
        echo "  Complete setup by enrolling:"
        echo "    dex enroll --data-dir $DATA_DIR"
        echo ""
        echo "  Then start Dex:"
        echo "    dex start --data-dir $DATA_DIR"
    fi
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

# Main
main() {
    detect_platform
    install_binary
    setup_data_dir
    enroll
    create_systemd_service
    print_next_steps
}

main "$@"
```

---

### HQ-MVP-04: GitHub Release Automation
**Priority**: HIGH
**Effort**: Medium
**Status**: COMPLETE

Set up automated binary releases so the install script can download pre-built binaries.

#### Implementation

**File**: `.github/workflows/release.yml` (new file)

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build:
    strategy:
      matrix:
        include:
          - os: linux
            arch: amd64
            runner: ubuntu-latest
          - os: linux
            arch: arm64
            runner: ubuntu-latest
          - os: darwin
            arch: amd64
            runner: macos-latest
          - os: darwin
            arch: arm64
            runner: macos-latest

    runs-on: ${{ matrix.runner }}

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Build
        env:
          GOOS: ${{ matrix.os }}
          GOARCH: ${{ matrix.arch }}
          CGO_ENABLED: 0
        run: |
          go build -ldflags="-s -w -X main.Version=${{ github.ref_name }}" \
            -o dex ./cmd/dex
          tar -czvf dex-${{ github.ref_name }}-${{ matrix.os }}-${{ matrix.arch }}.tar.gz dex

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: dex-${{ matrix.os }}-${{ matrix.arch }}
          path: dex-*.tar.gz

  release:
    needs: build
    runs-on: ubuntu-latest

    steps:
      - name: Download artifacts
        uses: actions/download-artifact@v4
        with:
          path: artifacts
          merge-multiple: true

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: artifacts/*.tar.gz
          generate_release_notes: true
```

#### Release Hosting

For MVP, use GitHub Releases. The install script URL pattern:
- `https://github.com/lirancohen/dex/releases/latest/download/dex-linux-amd64.tar.gz`

Or set up a redirect at `https://releases.enbox.id/` that points to GitHub releases.

---

## Existing Items (Updated Priority)

### HQ-01: Load Saved Config on Startup
**Status**: Merged into HQ-MVP-02

### HQ-02: Register Endpoints with Central
**Priority**: MEDIUM (post-MVP)
**Reason**: Edge currently accepts endpoints in HELLO message. Central registration is nice-to-have for the admin dashboard but not blocking.

### HQ-03: Namespace Configuration
**Status**: Merged into HQ-MVP-01 (enrollment returns namespace)

### HQ-04: Auth Key Provisioning UX
**Status**: Merged into HQ-MVP-01 (enrollment key flow)

### HQ-05: Configuration Hot-Reload
**Priority**: LOW (post-MVP)

### HQ-06: Health & Observability
**Priority**: LOW (post-MVP)

---

## Testing Checklist

### Unit Tests
- [x] `TestEnrollmentKeyValidation` - validates key format and error handling
- [x] `TestEnrollmentSuccess` - validates successful enrollment flow
- [x] `TestAlreadyEnrolled` - validates already enrolled error
- [x] `TestLoadConfig` - validates config loading and saving

### Integration Tests (requires Central)
- [ ] Enroll with valid key → config saved
- [ ] Enroll with invalid key → error message
- [ ] Start with config → mesh connects
- [ ] Start with config → tunnel connects
- [ ] Start without config → helpful error

### E2E Test
```bash
# 1. Get enrollment key from Central dashboard (or API)
# 2. Run installer
curl -fsSL https://get.enbox.id | sh
# 3. Enter enrollment key when prompted
# 4. Verify dex starts and connects
dex start --data-dir /opt/dex
# 5. Access public URL
curl https://alice.enbox.id
```

---

## File Summary

| File | Action | Description |
|------|--------|-------------|
| `cmd/dex/enroll.go` | CREATE | New enrollment command |
| `cmd/dex/config.go` | CREATE | Config types and loading |
| `cmd/dex/main.go` | MODIFY | Add enroll subcommand, load config in start |
| `scripts/install.sh` | REPLACE | Simplified installer |
| `.github/workflows/release.yml` | CREATE | Automated releases |

---

## Coordination with Central

The HQ enrollment command depends on Central's enrollment API:

**Endpoint**: `POST https://central.enbox.id/api/v1/enroll`

**Request**:
```json
{
  "key": "dexkey-alice-a1b2c3d4",
  "hostname": "alice-server"
}
```

Note: Central accepts both `key` and `enrollment_key` field names for compatibility.

**Response** (HTTP 200 on success):
```json
{
  "account_id": "uuid",
  "network_id": "uuid",
  "namespace": "alice",
  "public_url": "https://alice.enbox.id",
  "control_url": "https://central.enbox.id",
  "tunnel_token": "eyJhbGciOiJIUzI1NiIs...",
  "mesh": {
    "control_url": "https://central.enbox.id",
    "auth_key": "tskey-auth-xxxxx"
  },
  "tunnel": {
    "ingress_addr": "ingress.enbox.id:9443",
    "token": "eyJhbGciOiJIUzI1NiIs..."
  },
  "acme": {
    "email": "alice@example.com",
    "dns_api": "https://central.enbox.id/api/v1/dns/acme-challenge"
  }
}
```

**Error Response** (HTTP 4xx/5xx):
Central returns plain text error messages in the response body (not JSON).

See `dex-saas/hq-plan/SAAS-E2E.md` for Central implementation details.
