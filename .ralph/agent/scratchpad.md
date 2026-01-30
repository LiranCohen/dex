# Scratchpad - Current Iteration

## Objective
Build Poindexter (dex) - AI orchestration system for Claude Code sessions

## Current Status
- `go build ./cmd/dex` - PASSES
- `go test ./...` - PASSES
- `cd frontend && bun run build` - PASSES

## Completion Promise Progress
- [x] go build ./cmd/dex && go test ./... → PASS
- [x] Frontend builds: cd frontend && bun run build → PASS
- [~] Can authenticate via BIP39 passphrase from mobile - UI exists, needs testing
- [~] Can create a task via API and it appears in UI - Works!
- [ ] Can start a task and see a session running Ralph loop
- [ ] Session completes and creates a PR on GitHub
- [~] Real-time updates flow via WebSocket - Infrastructure exists

## Priority Work (from PROMPT.md)
1. **Project CRUD endpoints** - ✅ DONE
2. **Approval endpoints** - ✅ DONE
3. **Session control endpoints** - ✅ DONE
4. **Frontend UI** - IN PROGRESS

## Frontend Analysis

**What EXISTS and WORKS:**
- `LoginPage` - Full BIP39 auth flow with passphrase generation
- `DashboardPage` - System status, task list, task creation modal
- `TaskDetailPage` - Task info, start button, session info, worktree/budget display
- `useWebSocket` hook - Full WebSocket client with reconnection
- `api.ts` - Complete API client
- `types.ts` - All TypeScript types

**What NEEDS WORK:**
1. `TaskListPage` - Currently a placeholder stub that just says "(Task list UI coming soon)"
2. `TaskDetailPage` - Pause/Resume buttons are disabled with "coming soon" - but API endpoints exist now!
3. **Approvals page** - Missing entirely (no route, no component)

## Completed: Enable Pause/Resume/Cancel in TaskDetailPage

Wired up the pause/resume/cancel buttons in `TaskDetailPage`:
- Added `isPausing`, `isResuming`, `isCancelling` state variables
- Added `handlePauseTask`, `handleResumeTask`, `handleCancelTask` handlers
- Updated action buttons: running tasks show Pause + Cancel, paused tasks show Resume + Cancel
- Cancel button includes confirmation dialog
- All buttons show loading states and refetch task after action

Verified:
- `go build ./cmd/dex` - PASSES
- `go test ./...` - PASSES
- `bun run build` - PASSES

## Observer Review: TaskDetailPage Pause/Resume/Cancel

**Code Location:** `frontend/src/App.tsx:614-1042` (TaskDetailPage component)

### Observations

**1. Good Patterns Found:**
- Loading states properly prevent double-clicks (`isPausing`, `isResuming`, `isCancelling`)
- Task refetch after each action keeps UI in sync
- Confirmation dialog on cancel prevents accidental task destruction
- Status-based button visibility logic is clean and correct

**2. Potential Issues:**

**2a. Error handling could be more specific:**
- Lines 726, 746, 766: Error messages say "Failed to pause/resume/cancel task" but the API errors may have more context (e.g., "no active session for task"). The generic catch overwrites useful info.

**2b. Race condition on rapid clicks:**
- Lines 719-730: While `isPausing` prevents button re-click during a request, there's no guard against clicking Cancel while Pause is in-flight (or vice versa). User could click Pause then immediately click Cancel before Pause completes.
- **Fix:** Disable all action buttons when any action is in progress.

**2c. Cancel doesn't handle non-session tasks:**
- If a task is paused but the session was garbage collected, cancel will fail with 404. The UI shows the error but user experience could be confusing.
- **Note:** Backend `handleCancelTask` (server.go:1027-1029) returns 404 if no session exists, but `handleCancelTask` could still update task status even without a session.

**2d. WebSocket event handling could update UI:**
- Line 680 handles `task.*` events but only triggers refetch if `task_id` matches - but the event payload key check `(event.payload as { task_id?: string }).task_id === id` may miss events where the task_id is nested differently.
- The `task.paused`, `task.resumed`, `task.cancelled` events broadcast by server use `task_id` at top level, so this should work.

**3. Style/UX Suggestions (optional):**
- The "Task completed/cancelled" message when `isComplete` is true could show more context about when it happened.
- Consider disabling Cancel button with a "Stopping..." state after confirm while request is in-flight.

### Verdict

**Implementation is solid.** The main actionable issue is the potential for clicking multiple action buttons simultaneously. Everything else is minor or working correctly.

### Suggested Quick Fix for Race Condition

```tsx
const isActioning = isStarting || isPausing || isResuming || isCancelling;
// Then use isActioning to disable all buttons when any action is in progress
```

---

## Observer Review: Race Condition Fix

**Event:** `code.written` - Fixed race condition in TaskDetailPage action buttons

### Fix Verification

**Code Applied:**
- Line 625: `const isActioning = isStarting || isPausing || isResuming || isCancelling;`
- All action buttons (lines 913, 923, 930, 941, 948) now use `disabled={isActioning}`

### Observations

**✅ Fix is correct and complete:**
1. The `isActioning` guard combines all four loading states
2. All five action buttons (Start, Pause, Cancel when running, Resume, Cancel when paused) use the guard
3. Buttons show their individual loading text (e.g., "Pausing...", "Resuming...") while all are disabled
4. Frontend builds without errors

**Potential improvements (non-blocking):**
- Could add a visual indicator that "an action is in progress" to make it clearer why other buttons are disabled, but current disabled styling is adequate.

### Verdict

**Fix is solid. No issues found.** The race condition identified in the previous Observer pass has been properly addressed. Users can no longer trigger multiple concurrent actions on a task.

### Next Steps

With the TaskDetailPage actions working correctly, the remaining frontend gaps are:
1. `TaskListPage` - Still a stub placeholder
2. `ApprovalsPage` - Missing entirely (though approval endpoints exist)
3. End-to-end testing with real Anthropic API

---

## Navigator Direction: Add ApprovalsPage (Step 1 of 4)

**Goal:** Build the ApprovalsPage so users can approve/reject system requests.

**Step 1: Add Approval interface to types.ts** - ✅ DONE

---

## Observer Review: Approval Types (Step 1)

**Event:** `code.written` - Added Approval and ApprovalsResponse interfaces to frontend/src/lib/types.ts

### Verification

Compared frontend types to backend `ApprovalResponse` struct (`internal/api/server.go:771-783`):

| Frontend Field | Backend JSON | Status |
|---------------|--------------|--------|
| `id: string` | `id` | ✅ |
| `task_id?: string` | `task_id,omitempty` | ✅ |
| `session_id?: string` | `session_id,omitempty` | ✅ |
| `type: string` | `type` | ✅ |
| `title: string` | `title` | ✅ |
| `description?: string` | `description,omitempty` | ✅ |
| `data?: unknown` | `data,omitempty` | ✅ |
| `status: 'pending' \| 'approved' \| 'rejected'` | `status` | ✅ |
| `created_at: string` | `created_at` | ✅ |
| `resolved_at?: string` | `resolved_at,omitempty` | ✅ |

`ApprovalsResponse` matches backend list endpoint (`server.go:878-881`): ✅

### Verdict

**Implementation is correct.** Interfaces accurately map to backend API.

### Next Steps (for Navigator)

Step 1 complete. Remaining:
- Step 2: Add API functions for approvals in `api.ts`
- Step 3: Create `ApprovalsPage` component
- Step 4: Add routing for `/approvals`

---

## Navigator Direction: Add ApprovalsPage (Step 2 of 4)

**Goal:** Add approval API functions to `frontend/src/lib/api.ts`

**Current State:**
- `api.ts` has a clean `ApiClient` class with `get`, `post`, `put`, `delete` methods
- `types.ts` has `Approval` and `ApprovalsResponse` interfaces

**Instructions for Driver:**

Add the following 4 approval API functions at the end of `api.ts`, before the final export lines:

```typescript
// Approval API functions
export async function fetchApprovals(): Promise<import('./types').ApprovalsResponse> {
  return api.get('/approvals');
}

export async function fetchApproval(id: string): Promise<import('./types').Approval> {
  return api.get(`/approvals/${id}`);
}

export async function approveApproval(id: string): Promise<void> {
  return api.post(`/approvals/${id}/approve`);
}

export async function rejectApproval(id: string): Promise<void> {
  return api.post(`/approvals/${id}/reject`);
}
```

**Why these names:**
- `fetchApprovals` - matches existing pattern (e.g., `fetchTasks`)
- `fetchApproval` - get single approval details
- `approveApproval`/`rejectApproval` - matches backend endpoints `/approvals/:id/approve` and `/approvals/:id/reject`

**Verification:**
- Run `cd frontend && bun run build` to confirm no TypeScript errors

---

## Observer Review: Approval API Functions (Step 2)

**Event:** `code.written` - Added 4 approval API functions to frontend/src/lib/api.ts

### Verification

Checked implementation against backend endpoints (`internal/api/server.go`):

| Function | Endpoint | Return Type | Backend Response | Status |
|----------|----------|-------------|------------------|--------|
| `fetchApprovals()` | `GET /approvals` | `ApprovalsResponse` | `{approvals: [], count: N}` | ✅ |
| `fetchApproval(id)` | `GET /approvals/:id` | `Approval` | `ApprovalResponse` | ✅ |
| `approveApproval(id)` | `POST /approvals/:id/approve` | `void` | 204 No Content | ✅ |
| `rejectApproval(id)` | `POST /approvals/:id/reject` | `void` | 204 No Content | ✅ |

### Observations

**✅ Implementation is correct:**
1. All 4 functions map correctly to backend endpoints
2. Return types match backend responses
3. Using dynamic `import('./types')` avoids circular import issues
4. `void` return type is correct - backend returns 204 for approve/reject

**Minor note (non-blocking):**
- The naming `approveApproval` and `rejectApproval` is slightly redundant ("approve an approval"), but matches the backend route naming convention (`/approvals/:id/approve`) and is unambiguous. Alternative naming like `approve(id)` could conflict with other approve patterns later. Current naming is fine.

**Build verification:** Per scratchpad, build passes.

### Verdict

**Implementation is solid.** API functions correctly map to backend, types are accurate.

### Next Steps (for Navigator)

Step 2 complete. Remaining:
- Step 3: Create `ApprovalsPage` component in `App.tsx`
- Step 4: Add routing for `/approvals`

---

## Observer Review: ApprovalsPage Component (Step 3 of 4)

**Event:** `code.written` - Added ApprovalsPage component with approve/reject functionality, WebSocket subscription, and route at /approvals

### Code Location
- `frontend/src/App.tsx:1048-1063` - `ApprovalTypeBadge` component
- `frontend/src/App.tsx:1065-1257` - `ApprovalsPage` component
- `frontend/src/App.tsx:1298-1305` - Route `/approvals`

### Verification

**Build status:** ✅ `bun run build` passes (668ms)

**Checklist against Navigator instructions:**

| Requirement | Status |
|-------------|--------|
| State: `approvals`, `loading`, `error`, `actioning` | ✅ Lines 1066-1069 |
| Fetch on mount via `fetchApprovals()` | ✅ Lines 1085-1094 |
| WebSocket subscription to `approval.*` events | ✅ Lines 1097-1107 |
| `handleApprove(id)` handler | ✅ Lines 1110-1127 |
| `handleReject(id)` handler | ✅ Lines 1130-1147 |
| Header with back link + WS status | ✅ Lines 1168-1181 |
| Loading spinner | ✅ Lines 1154-1163 |
| Error display | ✅ Lines 1186-1190 |
| Empty state message | ✅ Lines 1192-1209 |
| Approval cards with title, badge, description, created_at | ✅ Lines 1217-1230 |
| Approve/Reject buttons with loading states | ✅ Lines 1233-1248 |
| Buttons disabled during action | ✅ Line 1236, 1243 (`isActioning`) |
| Route at `/approvals` | ✅ Lines 1298-1305 |

### Observations

**✅ Solid implementation:**
1. Per-approval action tracking with `actioning` map prevents double-clicks on same approval
2. Buttons show correct loading text ("Approving..." / "Rejecting...")
3. WebSocket subscription refetches on any `approval.*` event
4. Proper cleanup of `actioning` state in `finally` block
5. Filters to only pending approvals (line 1078)
6. Nice empty state with checkmark icon

**⚠️ Minor issues (non-blocking):**

1. **ApprovalTypeBadge `replace('_', ' ')` only replaces first underscore:**
   - Line 1060: `type.replace('_', ' ')` → for type `"hat_transition"` shows "hat transition" ✓
   - But if a type had multiple underscores like `"some_complex_type"`, it would show "some complex_type"
   - **Fix:** Use `type.replaceAll('_', ' ')` or `type.replace(/_/g, ' ')`
   - **Impact:** Very low - current types only have single underscores

2. **No confirmation on reject:**
   - Approve happens immediately on click, which is expected
   - Reject also happens immediately - consider adding a confirm dialog for destructive action
   - **Impact:** Low - rejecting an approval isn't data loss, just workflow decision

3. **`formatDate` could be extracted:**
   - Same date formatting logic exists in TaskDetailPage
   - Could be a shared utility, but duplication is minor and self-contained
   - **Impact:** None - code works correctly

**🎯 Potential UX improvements (future work):**
- Link to associated task from approval card (using `approval.task_id`)
- Show more context in `approval.data` field (currently unused)
- Add pagination for long approval lists

### Verdict

**Implementation is correct and complete.** The ApprovalsPage follows all Navigator instructions, matches existing patterns, and builds successfully. The minor issues noted are non-blocking polish items.

### Next Steps (for Navigator)

Step 3 complete (ApprovalsPage). Remaining:
- Step 4: Add link to Approvals from Dashboard (navigation)
- Then: TaskListPage still needs work (stub placeholder)
- Then: End-to-end testing with real Anthropic API

---

## Navigator Direction: Add Approvals Link to Dashboard (Step 4 of 4)

**Goal:** Make the Approvals page discoverable from the Dashboard.

**Current State:**
- ApprovalsPage exists at `/approvals` route
- No navigation to it from Dashboard
- Dashboard header has: "Poindexter" title, WebSocket status, Logout button

**Instructions for Driver:**

In `frontend/src/App.tsx`, modify the Dashboard header navigation (around line 371-386) to add an "Approvals" link.

Change from:
```tsx
<div className="flex items-center gap-4">
  <h1 className="text-xl font-bold">Poindexter</h1>
  <div className="flex items-center gap-2 text-sm">
```

To:
```tsx
<div className="flex items-center gap-4">
  <h1 className="text-xl font-bold">Poindexter</h1>
  <Link
    to="/approvals"
    className="text-gray-400 hover:text-white text-sm transition-colors"
  >
    Approvals
  </Link>
  <div className="flex items-center gap-2 text-sm">
```

**Why this location:**
- Places link near WebSocket status indicator
- Follows existing styling pattern (text-gray-400, text-sm)
- Simple navigation in header rather than sidebar

**Verification:**
- Run `cd frontend && bun run build` to confirm no errors
- Link should appear in header between title and WebSocket status

---

## Observer Review: Approvals Link in Dashboard (Step 4 of 4)

**Event:** `code.written` - Added Approvals link to Dashboard header navigation between title and WebSocket status indicator

### Code Location
- `frontend/src/App.tsx:373-375` - Approvals link in Dashboard header

### Verification

**Build status:** ✅ `bun run build` passes (889ms)

**Change review:**

```tsx
<Link to="/approvals" className="text-gray-400 hover:text-white text-sm transition-colors">
  Approvals
</Link>
```

### Observations

**✅ Implementation is correct and complete:**
1. Link uses correct route `/approvals` matching the route defined at line 1301-1306
2. Styling matches existing header elements (`text-gray-400 hover:text-white text-sm transition-colors`)
3. Position is appropriate: between title and WebSocket status indicator
4. Uses react-router `Link` component correctly (already imported at line 2)

**Minor style considerations (non-blocking):**

1. **Visual hierarchy:** The "Approvals" link uses the same styling as "Logout" button on the right side. This is fine - both are navigation actions. However, "Approvals" could arguably have a badge showing pending count, but that's a future enhancement.

2. **Consistency check:** Looking at the header layout:
   - Left side: "Poindexter" (bold) → "Approvals" (subtle link) → WebSocket indicator
   - Right side: "Logout" (subtle link)

   This layout is clean and balanced.

3. **Mobile consideration:** On small screens, the header might become crowded with title + Approvals + WebSocket status + Logout. Current approach is acceptable but may need responsive design refinement later.

### Verdict

**Implementation is solid.** The Approvals link is correctly placed, styled consistently with other header elements, and routes to the existing ApprovalsPage. No issues found.

### ApprovalsPage Feature Complete Summary

All 4 steps for the ApprovalsPage feature are now complete:
- [x] Step 1: Added Approval interfaces to types.ts
- [x] Step 2: Added API functions to api.ts
- [x] Step 3: Created ApprovalsPage component with approve/reject functionality
- [x] Step 4: Added navigation link from Dashboard

### Next Steps (for Navigator)

With the ApprovalsPage feature complete, the remaining frontend gaps are:
1. **TaskListPage** - Still a stub placeholder (line 608-615)
2. **End-to-end testing** with real Anthropic API

---

## Navigator Direction: Implement TaskListPage

**Goal:** Replace the stub TaskListPage with a full task list page that supports filtering and provides access to all tasks.

**Current State:**
- `TaskListPage` is a placeholder stub (lines 608-615 in App.tsx)
- DashboardPage already has task display logic that can be referenced
- Backend supports `?status=<status>&project_id=<id>` query params for filtering
- Route already exists at `/tasks`

**Design Decisions:**
1. Show ALL tasks (not limited to 10 like dashboard)
2. Add status filter dropdown (All, Pending, Ready, Running, Paused, Completed, Failed, Cancelled)
3. Include header with back link and WebSocket status (matches ApprovalsPage pattern)
4. Reuse existing `StatusBadge` and `PriorityDot` components
5. Add "Create Task" button (reuse create modal from DashboardPage? or link to dashboard for now)

**Instructions for Driver:**

Replace the `TaskListPage` function (lines 608-615) with a full implementation:

**Step 1 - State and hooks:**
```typescript
function TaskListPage() {
  const navigate = useNavigate();
  const { isAuthenticated, logout } = useAuthStore();
  const { connected, subscribe, unsubscribe } = useWebSocket();

  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>('all');
```

**Step 2 - Fetch tasks function:**
```typescript
  const fetchTasksData = useCallback(async () => {
    try {
      const params = statusFilter !== 'all' ? `?status=${statusFilter}` : '';
      const data = await api.get<TasksResponse>(`/tasks${params}`);
      setTasks(data.tasks || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load tasks');
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);
```

**Step 3 - Effects for fetch and WebSocket:**
```typescript
  useEffect(() => {
    if (!isAuthenticated) {
      navigate('/login');
      return;
    }
    fetchTasksData();
  }, [isAuthenticated, navigate, fetchTasksData]);

  useEffect(() => {
    const handleTaskEvent = () => {
      fetchTasksData();
    };
    subscribe('task.*', handleTaskEvent);
    return () => unsubscribe('task.*', handleTaskEvent);
  }, [subscribe, unsubscribe, fetchTasksData]);
```

**Step 4 - Render:**
- Header with "Back to Dashboard" link, "Tasks" title, WebSocket status
- Status filter dropdown with options: All, Pending, Ready, Running, Paused, Completed, Failed, Cancelled
- Loading spinner (reuse pattern from other pages)
- Error display
- Empty state when no tasks match filter
- Task list (reuse the task card pattern from DashboardPage, but show all tasks)
- Each task links to `/tasks/${task.ID}`

**Key styling to match existing patterns:**
- `min-h-screen bg-gray-900 text-white`
- Header: `bg-gray-800 border-b border-gray-700 px-4 py-3`
- Filter select: `bg-gray-700 text-white rounded-lg px-3 py-2 text-sm`
- Task cards: `bg-gray-700 hover:bg-gray-650 rounded-lg p-3`

**Verification:**
- `cd frontend && bun run build` must pass
- Page should load at `/tasks` route
- Filter dropdown should filter tasks by status

---

## Observer Review: TaskListPage Implementation

**Event:** `code.written` - Implemented TaskListPage with status filter dropdown, WebSocket subscription, and full task list display

### Code Location
- `frontend/src/App.tsx:608-795` - TaskListPage component

### Verification

**Build status:** ✅ `bun run build` passes (704ms)

**Checklist against Navigator instructions:**

| Requirement | Status |
|-------------|--------|
| State: `tasks`, `loading`, `error`, `statusFilter` | ✅ Lines 613-616 |
| `fetchTasksData` with status param | ✅ Lines 619-630 |
| Auth redirect on mount | ✅ Lines 633-640 |
| WebSocket subscription to task events | ✅ Lines 643-653 |
| Status filter dropdown with all status options | ✅ Lines 656-666, 712-723 |
| Loading spinner | ✅ Lines 668-677 |
| Error display | ✅ Lines 699-703 |
| Empty state with context-aware message | ✅ Lines 733-754 |
| Task cards with PriorityDot, StatusBadge, details | ✅ Lines 756-789 |
| Each task links to `/tasks/${task.ID}` | ✅ Line 760 |
| Header with back link and WebSocket status | ✅ Lines 682-696 |

### Observations

**✅ Solid implementation:**
1. Status filter is wired correctly - changing dropdown triggers refetch via useEffect dependency on `fetchTasksData` which depends on `statusFilter`
2. Task count display updates correctly as filter changes
3. Empty state message adapts to current filter
4. WebSocket subscription uses correct pattern from existing pages
5. Uses existing `StatusBadge` and `PriorityDot` components for consistency
6. Task card shows type, hat (if present), and description with nice truncation

**⚠️ Minor issues (non-blocking):**

1. **`setLoading(true)` on filter change may cause flicker:**
   - Line 638: `setLoading(true)` is called in useEffect that depends on `fetchTasksData`
   - When user changes filter, the loading state resets, potentially showing the full-page spinner briefly
   - Better UX: Keep existing list visible while refetching, maybe add subtle loading indicator
   - **Impact:** Low - current behavior is acceptable, just not ideal UX

2. **WebSocket subscription pattern differs from Navigator spec:**
   - Navigator suggested: `subscribe('task.*', handleTaskEvent)` then `unsubscribe('task.*', handleTaskEvent)`
   - Implementation uses: `subscribe(handleWebSocketEvent)` which returns unsubscribe function
   - This matches how the `subscribe` function actually works in `useWebSocket` hook
   - **Impact:** None - implementation correctly matches the actual API

3. **Status options list doesn't include 'failed' status:**
   - Lines 656-666 list: all, pending, ready, running, paused, completed, cancelled, blocked, quarantined
   - The Navigator spec mentioned 'Failed' as a status option, but it's not in the list
   - **Fix:** Add `{ value: 'failed', label: 'Failed' }` to statusOptions
   - **Impact:** Low - if backend has failed tasks, users can't filter to them

4. **`hover:bg-gray-650` class may not exist:**
   - Line 761: `className="block bg-gray-700 hover:bg-gray-650 rounded-lg p-3 transition-colors"`
   - Tailwind doesn't have `gray-650` by default (gray scale: 600, 700, 800)
   - Either needs custom config or should use `hover:bg-gray-600`
   - **Impact:** Low - hover effect may not work, but no build error

5. **No "Create Task" button mentioned:**
   - Navigator suggested potentially adding a Create Task button
   - Current implementation doesn't include one - users must go to Dashboard
   - **Impact:** Low - navigation back to dashboard is available

### Code Quality

**Good patterns:**
- Clean separation of fetch logic into `fetchTasksData` callback
- Proper use of `useCallback` with dependencies
- Accessible form control with `htmlFor` label
- Consistent styling with other pages

**Minor style notes:**
- Task type is uppercased inline (`<span className="uppercase">{task.Type}</span>`) - works but could be `text-transform: uppercase` in class or component
- Description truncation uses `truncate max-w-xs` which may be too narrow on larger screens

### Verdict

**Implementation is correct and complete.** The TaskListPage provides full task listing with status filtering, WebSocket real-time updates, and links to task details. The minor issues noted are non-blocking polish items.

### Recommended Quick Fixes (optional)

1. Add missing 'failed' status to options:
   ```typescript
   { value: 'failed', label: 'Failed' },
   ```

2. Fix hover class:
   ```typescript
   className="block bg-gray-700 hover:bg-gray-600 rounded-lg p-3 transition-colors"
   ```

### Next Steps (for Navigator)

TaskListPage is now functional. Remaining frontend gaps:
1. The two minor fixes above (optional)
2. End-to-end testing with real Anthropic API
3. Verify full user flow: Login → Dashboard → Create Task → View in TaskList → Start → Monitor → Complete

---

## Navigator Direction: Apply TaskListPage Observer Fixes

**Event Handled:** `observation.noted` - TaskListPage review: Build passes. Implementation correct - filter, WebSocket, task cards all work. Two minor fixes suggested.

**Goal:** Apply the two minor fixes identified by the Observer to polish the TaskListPage implementation.

**Current State:**
- TaskListPage is functional at `/tasks` route
- Build passes
- Two minor issues need fixing

**Instructions for Driver:**

**Fix 1: Add 'failed' status to filter options**

Location: `frontend/src/App.tsx` line 665 (inside statusOptions array)

Add this line after `{ value: 'cancelled', label: 'Cancelled' },`:
```typescript
{ value: 'failed', label: 'Failed' },
```

The full statusOptions array should become:
```typescript
const statusOptions: { value: string; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'pending', label: 'Pending' },
  { value: 'ready', label: 'Ready' },
  { value: 'running', label: 'Running' },
  { value: 'paused', label: 'Paused' },
  { value: 'completed', label: 'Completed' },
  { value: 'cancelled', label: 'Cancelled' },
  { value: 'failed', label: 'Failed' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'quarantined', label: 'Quarantined' },
];
```

**Fix 2: Fix invalid Tailwind class `hover:bg-gray-650`**

Location: `frontend/src/App.tsx` line 761

Change from:
```typescript
className="block bg-gray-700 hover:bg-gray-650 rounded-lg p-3 transition-colors"
```

To:
```typescript
className="block bg-gray-700 hover:bg-gray-600 rounded-lg p-3 transition-colors"
```

**Why:** Tailwind CSS doesn't have a `gray-650` class by default. The gray scale goes 600, 700, 800. Using `hover:bg-gray-600` provides a lighter hover state which is the intended UX.

**Verification:**
- Run `cd frontend && bun run build` to confirm no errors
- Both changes are one-line modifications

---

## Navigator Assessment: MVP Feature Complete

**Event Handled:** `observation.noted` - TaskListPage fixes verified: Both 'failed' status and hover:bg-gray-600 correctly applied.

### Build Verification (Fresh)

```
✅ go build ./cmd/dex - PASSES
✅ go test ./... - PASSES (internal/session tests pass)
✅ cd frontend && bun run build - PASSES (668ms)
```

### Completion Promise Progress

| Promise Item | Status |
|-------------|--------|
| `go build ./cmd/dex && go test ./...` → PASS | ✅ DONE |
| Frontend builds: `cd frontend && bun run build` → PASS | ✅ DONE |
| Can authenticate via BIP39 passphrase from mobile | ✅ UI Complete (needs live test) |
| Can create a task via API and it appears in UI | ✅ Works |
| Can start a task and see a session running Ralph loop | ⚠️ Code complete, needs live test |
| Session completes and creates a PR on GitHub | ⚠️ Code complete, needs live test |
| Real-time updates flow via WebSocket | ✅ Infrastructure complete |

### What's Complete

**Frontend (100% MVP):**
- ✅ LoginPage - BIP39 auth flow with generate/input
- ✅ DashboardPage - System status, task list (10), task creation modal
- ✅ TaskListPage - Full list with 10 status filters, WebSocket updates
- ✅ TaskDetailPage - Task info, start/pause/resume/cancel buttons, session info
- ✅ ApprovalsPage - List pending approvals, approve/reject actions

**Backend (100% MVP):**
- ✅ Auth endpoints - challenge/verify/refresh
- ✅ Task CRUD - full lifecycle
- ✅ Project CRUD - complete
- ✅ Approval endpoints - list/approve/reject
- ✅ Session control - pause/resume/cancel/kill
- ✅ WebSocket hub - real-time events
- ✅ Ralph loop - complete with budget enforcement, completion detection, hat transitions

**Integration Code (untested with real API):**
- ✅ Anthropic client configured
- ✅ GitHub client for PR creation
- ✅ Git worktree management

### Remaining: End-to-End Validation

The MVP is **code-complete**. What remains is **validation**:

1. **Start the server and test full flow:**
   - Generate passphrase and login
   - Create a task
   - Start the task (triggers Ralph loop)
   - Observe real-time updates via WebSocket
   - See session complete or hit budget limit

2. **Verify with real Anthropic API:**
   - The Ralph loop calls `r.client.Chat(ctx, systemPrompt, r.messages)`
   - Need to test with actual API key in toolbelt.yaml
   - Verify response parsing, completion detection, token tracking

### LOOP_COMPLETE Assessment

The Completion Promise states "LOOP_COMPLETE when all are true":
- [x] Build passes
- [x] Tests pass
- [x] Frontend builds
- [~] Auth from mobile - UI exists
- [~] Create task appears in UI - Works
- [ ] Start task, see Ralph loop - Needs real test
- [ ] Session creates PR - Needs real test
- [~] WebSocket updates - Infrastructure exists

**Verdict:** The codebase is feature-complete for MVP. All endpoints implemented, all UI pages built, all builds pass. However, **real end-to-end testing with Anthropic API has not been performed**.

### Next Step Decision

**Option A:** Declare LOOP_COMPLETE - All code is written, builds pass, tests pass. E2E testing is a deployment/ops concern.

**Option B:** Perform real API test - Start the server, create a task, attempt to run Ralph loop with real Anthropic API.

Given the Completion Promise specifically mentions "Can start a task and see a session running Ralph loop" and "Session completes and creates a PR on GitHub", I recommend **Option B** - but this requires:
1. Valid Anthropic API key in toolbelt.yaml
2. A test project/repo to create worktrees in
3. Manual server startup and testing

This is beyond what can be done in this automated loop - it requires user interaction with the running system.
