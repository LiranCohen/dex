# Dex Thin Client

Cross-platform thin client for connecting to HQ over the Dex Campus mesh network.

## Platforms

- macOS (ARM64 + x86_64)
- Linux (x86_64)
- Windows (x86_64)
- iOS (planned)
- Android (planned)

## Architecture

The thin client uses WebAssembly (WASM) for mesh networking instead of native VPN APIs:

```
┌─────────────────────────────────────────┐
│           Tauri 2.0 App                 │
├─────────────────────────────────────────┤
│  React UI (TypeScript)                  │
│  - Dashboard, tasks, approvals          │
├─────────────────────────────────────────┤
│  Mesh Client (tsconnect WASM)           │
│  - Go compiled to WebAssembly           │
│  - Connects to Central                  │
│  - Dials HQ over mesh                   │
├─────────────────────────────────────────┤
│  Native Shell (Rust)                    │
│  - System tray, notifications           │
│  - Keychain credential storage          │
└─────────────────────────────────────────┘
```

## Prerequisites

1. Bun 1.0+
2. Rust 1.87+
3. Go 1.24+ (for building WASM)
4. Xcode (macOS only)
5. Android Studio (Android only)

## Setup

1. Install dependencies:
   ```bash
   bun install
   ```

2. Build the WASM mesh client (from dexnet repo):
   ```bash
   cd /path/to/dexnet
   GOOS=js GOARCH=wasm go build -o /path/to/apps/client/public/main.wasm ./cmd/tsconnect/wasm/
   ```

3. Copy Go WASM support:
   ```bash
   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" public/
   ```

## Development

```bash
# Start development server
bun tauri dev

# Build for production
bun tauri build
```

## Mobile Development (Future)

```bash
# iOS
bun tauri ios init
bun tauri ios dev

# Android
bun tauri android init
bun tauri android dev
```

## Project Structure

```
apps/client/
├── src/                    # React frontend
│   ├── components/         # UI components
│   ├── mesh/               # WASM client wrapper
│   ├── stores/             # Zustand state
│   └── App.tsx
├── src-tauri/              # Rust backend
│   ├── src/lib.rs          # Native commands
│   └── tauri.conf.json     # Tauri config
├── public/                 # Static assets
│   ├── main.wasm           # Mesh client (not in git)
│   └── wasm_exec.js        # Go WASM support
└── package.json
```

## How It Works

1. **Connect**: User enters auth key or logs in via browser
2. **Register**: WASM client connects to Central, gets mesh IP
3. **Discover**: Receives peer list including HQ
4. **Dial**: Connects to HQ's mesh IP over encrypted tunnel
5. **Interact**: HTTP/WebSocket API calls over mesh
