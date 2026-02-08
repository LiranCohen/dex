# Mesh Access Implementation Gap

This document outlines the gap between the current implementation and the desired architecture for mesh access control in Poindexter.

## Desired Architecture

### HQ (Headquarters)
- **Exactly 1 per user** (required)
- **Always mesh-only** - NOT accessible from public internet
- Users access HQ via:
  - dex-client running on their local machine (connects to mesh)
  - Future: WASM Tailscale client in Central dashboard (browser-based mesh access)

### Outposts
- **Optional** additional nodes
- **Default: mesh-only** - accessible only via mesh
- **Can be marked "public"** - accessible from public internet via Edge tunnel
- Public Outposts get a URL: `{hostname}.{namespace}.enbox.id`

### Access Paths

| Node Type | Mesh Access | Public Access (via Edge) |
|-----------|-------------|--------------------------|
| HQ | Always | Never |
| Outpost (default) | Always | Never |
| Outpost (public) | Always | Yes |

## Current Implementation

### What Works
1. **Mesh networking** - HQ and clients can connect via Central coordination
2. **Edge tunnel** - SNI routing and yamux multiplexing work correctly
3. **dex-client** - Local machine mesh access works (LaunchAgent on macOS)
4. **Enrollment flow** - Keys, machine registration, config generation all work

### What's Wrong
1. **HQ has public tunnel enabled** - `hq.{namespace}.enbox.id` is publicly accessible via Edge
2. **No `is_public` flag on nodes** - All nodes with tunnel config get public access
3. **Tunnel is always-on** - No way to disable tunnel per-node

### Current Traffic Flow (Incorrect)

```
Public Internet
      │
      ▼
Edge (168.220.92.190:443)
      │ SNI: hq.liran.enbox.id
      ▼
HQ Tunnel Client ◄── This should NOT exist for HQ
      │
      ▼
HQ HTTP Server (127.0.0.1:8080)
```

## Implementation Tasks

### 1. Database Schema Changes (dex-saas)

Add `is_public` column to nodes table:

```sql
ALTER TABLE nodes ADD COLUMN is_public BOOLEAN DEFAULT FALSE;
```

- HQ nodes: `is_public` is always `FALSE` (enforced in code)
- Outpost nodes: `is_public` defaults to `FALSE`, can be set to `TRUE`

### 2. Enrollment API Changes (dex-saas)

Update `/api/v1/enroll` response:

```go
type EnrollResponse struct {
    // ... existing fields
    Tunnel *TunnelConfig `json:"tunnel,omitempty"` // Only present if is_public=true
}
```

- If `is_public=false`: Do NOT include tunnel config in enrollment response
- If `is_public=true`: Include tunnel config (ingress_addr, token, endpoints)

### 3. Central Dashboard Changes (dex-saas)

Update "Add Outpost" UI:

```
┌─────────────────────────────────────────────────────────────────┐
│                    ADD AN OUTPOST                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Hostname: [worker1                   ]                         │
│                                                                  │
│  □ Make this Outpost publicly accessible                        │
│     If checked: worker1.alice.enbox.id (via Edge tunnel)        │
│     If unchecked: mesh-only access (default)                    │
│                                                                  │
│  [Generate Enrollment Key]                                      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 4. HQ Startup Changes (dex)

Modify `internal/api/server.go` startup sequence:

```go
// In startMeshAndTunnel() or equivalent:

// Start mesh (always)
if err := s.startMesh(); err != nil {
    return err
}

// Start tunnel ONLY if config includes tunnel settings
// (which only happens for public Outposts)
if s.config.Tunnel != nil && s.config.Tunnel.Enabled {
    if err := s.startTunnel(); err != nil {
        return err
    }
}
```

### 5. Config Schema Changes (dex)

Update `cmd/dex/config.go`:

```go
type Config struct {
    Namespace string      `json:"namespace"`
    Hostname  string      `json:"hostname"`
    IsHQ      bool        `json:"is_hq"`       // True for HQ, false for Outposts
    IsPublic  bool        `json:"is_public"`   // Only relevant for Outposts
    Mesh      MeshConfig  `json:"mesh"`
    Tunnel    *TunnelConfig `json:"tunnel,omitempty"` // nil for mesh-only nodes
    // ...
}
```

### 6. Edge Token Verification (dex-saas)

Update Edge to verify `is_public` status:

- When HQ connects with tunnel token, Edge should reject (or Central shouldn't issue tunnel tokens for HQ)
- Alternatively: Central simply doesn't include tunnel config for non-public nodes

## Migration Path

### For Existing Installations

1. Add `is_public` column with default `FALSE`
2. Existing HQs continue to work (tunnel will be disabled on next enrollment/restart)
3. Users who want public Outposts must explicitly enable via dashboard

### Backwards Compatibility

- Old dex binaries with tunnel config will still try to connect
- Edge can reject based on node type, or
- Central can revoke/regenerate enrollment for existing HQs without tunnel config

## Future: WASM Mesh Client

For browser-based access to mesh-only HQs:

1. Central serves WASM Tailscale client (from `tsconnect`)
2. User authenticates with Central (passkey)
3. Central issues temporary node key for WASM client
4. WASM client joins mesh, can access HQ directly
5. Toggle on/off per session (ephemeral node)

This is a larger undertaking and should be tracked separately.

## Files to Modify

### dex-saas repo
- `hscontrol/db/node.go` - Add `is_public` field
- `hscontrol/api/enrollment.go` - Conditionally include tunnel config
- `hscontrol/dashboard/src/pages/OnboardingPage.tsx` - Add public checkbox for Outposts
- `hscontrol/types/node.go` - Add `IsPublic` field

### dex repo
- `cmd/dex/config.go` - Update config structure
- `internal/api/server.go` - Conditional tunnel startup
- `internal/mesh/config.go` - Update TunnelSettings

## Testing

1. Enroll new HQ → verify no tunnel config in response
2. Enroll new Outpost (default) → verify no tunnel config
3. Enroll new Outpost (public=true) → verify tunnel config present
4. Start HQ → verify no connection to Edge
5. Start public Outpost → verify Edge tunnel works
6. Access `hq.{namespace}.enbox.id` from public internet → should fail (connection refused)
7. Access `public-outpost.{namespace}.enbox.id` → should work
