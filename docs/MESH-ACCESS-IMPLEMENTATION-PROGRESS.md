# Mesh Access Gap - Implementation Progress

**Status**: IN PROGRESS
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

## In Progress 🔄

### 3. Enrollment API Logic (dex-saas)
- [ ] Update `CreateEnrollmentKey` to:
  - Force `is_public=false` for HQ (hostname="hq")
  - Accept and validate `is_public` for Outposts
  - Store `is_public` on enrollment key
- [ ] Update `EnrollNodeDirectly` to accept `isPublic` parameter
- [ ] Update `Enroll` endpoint to:
  - Pass `is_public` from enrollment key to node creation
  - Conditionally generate tunnel token only if `is_public=true`
  - Conditionally include Tunnel/ACME config in response only if `is_public=true`
  - Only include Owner info for HQ enrollments

## Remaining Tasks 📋

### 4. HQ Startup Changes (dex)
- [ ] Update config structure to make `Tunnel *TunnelConfig` optional
- [ ] Modify `internal/api/server.go` startup to skip tunnel if config.Tunnel is nil
- [ ] Update enrollment command to handle missing tunnel config gracefully

### 5. Central Dashboard (dex-saas)
- [ ] Add "Make Public" checkbox to Add Outpost UI
- [ ] Show public URL only for public Outposts
- [ ] Indicate mesh-only vs public status in node list

### 6. Testing
- [ ] Test HQ enrollment → verify no tunnel config in response
- [ ] Test Outpost enrollment (default) → verify no tunnel config
- [ ] Test Outpost enrollment (is_public=true) → verify tunnel config present
- [ ] Test HQ startup → verify no Edge connection attempt
- [ ] Test public Outpost startup → verify Edge tunnel works
- [ ] Test public access to `hq.{namespace}.enbox.id` → should fail
- [ ] Test public access to `public-outpost.{namespace}.enbox.id` → should work

## Files Modified

### dex-saas
- `hscontrol/types/node.go` - Added IsPublic field
- `hscontrol/types/enrollment_key.go` - Added IsPublic field
- `hscontrol/api/enrollment.go` - Updated request/response types
- `hscontrol/db/migrations/000003_*` - Node is_public column
- `hscontrol/db/migrations/000004_*` - EnrollmentKey is_public column

### dex
- (None yet - pending)

## Next Steps

1. **Complete enrollment API logic** (highest priority):
   - Update `CreateEnrollmentKey` handler (lines 149-280 in enrollment.go)
   - Update `EnrollNodeDirectly` signature (enrollment_key.go)
   - Update `Enroll` handler logic (lines 558-752 in enrollment.go)

2. **Test database migrations**:
   ```bash
   cd ~/src/dex-saas
   # Run migrations up
   # Verify columns exist
   # Run migrations down
   # Verify clean rollback
   ```

3. **Update dex binary**:
   - Handle optional tunnel config
   - Skip tunnel connection if not configured

## Design Decisions

### Why two is_public fields?
- `EnrollmentKey.IsPublic`: Captures user's intent during key creation
- `Node.IsPublic`: Actual node configuration (must match enrollment key)

### Why optional pointers in EnrollResponse?
- Allows clean omission in JSON (no empty objects)
- Makes it explicit when tunnel/ACME config is not provided
- Simplifies HQ code - can check `if config.Tunnel != nil`

### Why force is_public=false for HQ?
- Security: HQ should never be exposed to public internet
- Architecture: Users access HQ via dex-client on mesh
- Prevents accidental public exposure of sensitive coordinator

## Testing Strategy

### Unit Tests
- [ ] Test `CreateEnrollmentKey` with is_public flag
- [ ] Test `Enroll` response structure for each scenario
- [ ] Test node creation with is_public flag

### Integration Tests
- [ ] End-to-end enrollment flow for HQ
- [ ] End-to-end enrollment flow for mesh-only Outpost
- [ ] End-to-end enrollment flow for public Outpost

### Manual Testing
- [ ] Enroll HQ, verify no tunnel connection in logs
- [ ] Try to access `hq.{namespace}.enbox.id`, verify failure
- [ ] Enroll public Outpost, verify tunnel works
- [ ] Access public Outpost URL, verify success
