# Mesh Access Gap - Implementation Progress

**Status**: COMPLETED (Core Implementation)
**Start Date**: 2026-02-08
**Issue**: HQ is currently publicly accessible via Edge tunnel - it should be mesh-only

## Completed ✅

### 1. Database Schema (dex-saas)
- [x] Created migration `000003_add_node_is_public.up.sql` 
- [x] Created migration `000003_add_node_is_public.down.sql`
- [x] Created migration `000004_add_enrollment_key_is_public.up.sql`
- [x] Created migration `000004_add_enrollment_key_is_public.down.sql`

### 2. Type Definitions (dex-saas)
- [x] Added `IsPublic bool` field to `types.Node` struct
- [x] Added `IsPublic bool` field to `types.EnrollmentKey` struct
- [x] Added `IsPublic bool` field to `CreateEnrollmentKeyRequest` API type
- [x] Updated `EnrollResponse` to make `Tunnel`, `ACME`, `Owner` optional (pointers)

### 3. Enrollment API Logic (dex-saas)
- [x] Updated `GetOrCreateEnrollmentKey` to accept `isPublic` parameter
- [x] Updated `EnrollNodeDirectly` to accept `isPublic` parameter
- [x] Updated `CreateEnrollmentKey` handler to:
  - Force `is_public=false` for HQ (hostname="hq")
  - Force `is_public=false` for clients
  - Accept `is_public` for Outposts
- [x] Updated `Enroll` endpoint to conditionally include tunnel config
- [x] **Commit**: `9b3164a` - "feat: add is_public flag for mesh-only vs public node access"

### 4. Dex Binary Updates (dex)
- [x] Updated `EnrollmentResponse` struct to have optional `Tunnel`, `ACME`, `Owner` (pointers)
- [x] Updated `buildConfigFromResponse` to only enable tunnel/ACME when provided
- [x] Updated enrollment success message to show access mode (mesh-only vs public)
- [x] Added `TestEnrollmentMeshOnly` test case
- [x] **Commit**: `889cbce` - "feat: support mesh-only enrollment (no tunnel/ACME)"

## Remaining Tasks 📋

### 5. Central Dashboard (dex-saas)
- [ ] Add "Make Public" checkbox to Add Outpost UI
- [ ] Show public URL only for public Outposts
- [ ] Indicate mesh-only vs public status in node list

### 6. End-to-End Testing
- [ ] Test HQ enrollment → verify no tunnel config in response
- [ ] Test Outpost enrollment (default) → verify no tunnel config
- [ ] Test Outpost enrollment (is_public=true) → verify tunnel config present
- [ ] Test HQ startup → verify no Edge connection attempt
- [ ] Test public access to `hq.{namespace}.enbox.id` → should fail

## Related: Enrollment Flow Assessment

A comprehensive assessment of all enrollment flows (HQ, Outpost, Client) was conducted and documented in:
- **dex-saas**: `docs/ENROLLMENT-FLOW-ASSESSMENT.md`

### Key Issues Found
See the assessment document for full details. Summary:

| Issue | Priority | Status |
|-------|----------|--------|
| Outpost install command bug (`--outpost` flag) | High | Open |
| Device revocation missing | High | Open |
| HQ public URL generated but unused | Medium | Open |
| Node type distinction (tags) | Medium | Open |

## Files Modified

### dex-saas (Commit: 9b3164a)
- `hscontrol/types/node.go` - Added IsPublic field
- `hscontrol/types/enrollment_key.go` - Added IsPublic field
- `hscontrol/types/types_clone.go` - Regenerated
- `hscontrol/types/types_view.go` - Regenerated
- `hscontrol/api/enrollment.go` - Updated request/response types and logic
- `hscontrol/api/hq.go` - Updated GetOrCreateEnrollmentKey calls
- `hscontrol/api/onboarding.go` - Updated GetOrCreateEnrollmentKey calls
- `hscontrol/db/enrollment_key.go` - Updated function signatures
- `hscontrol/db/migrations/000003_*` - Node is_public column
- `hscontrol/db/migrations/000004_*` - EnrollmentKey is_public column

### dex (Commit: 889cbce)
- `cmd/dex/enroll.go` - Updated EnrollmentResponse and buildConfigFromResponse
- `cmd/dex/enroll_test.go` - Added TestEnrollmentMeshOnly

## Design Decisions

### Why two is_public fields?
- `EnrollmentKey.IsPublic`: Captures user's intent during key creation
- `Node.IsPublic`: Actual node configuration (must match enrollment key)

### Why optional pointers in EnrollResponse?
- Allows clean omission in JSON (no empty objects)
- Makes it explicit when tunnel/ACME config is not provided
- Simplifies dex code - can check `if resp.Tunnel != nil`

### Why force is_public=false for HQ?
- Security: HQ should never be exposed to public internet
- Architecture: Users access HQ via dex-client on mesh
- Prevents accidental public exposure of sensitive coordinator
