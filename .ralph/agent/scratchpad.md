# Scratchpad

## Current Objective
Building Poindexter (dex) - **Phase 5: Session Management**

## Phase 3 Progress ✓ COMPLETE

### Checkpoint 3.1: Task CRUD ✓
### Checkpoint 3.2: Natural Language Parsing - Deferred
### Checkpoint 3.3: Dependency Graph ✓
### Checkpoint 3.4: Task State Machine ✓
### Checkpoint 3.5: Priority Queue Scheduler ✓

## Phase 4 Progress ✓ COMPLETE

### Checkpoint 4.1: Worktree Operations ✓
### Checkpoint 4.2: Git Operations ✓
### Checkpoint 4.3: Integration ✓

## Phase 5 Progress

### Checkpoint 5.1: Claude Agent SDK Integration - ✓ COMPLETE
### Checkpoint 5.2: Hat Prompt Loading - POLISHING (Observer feedback)
### Checkpoint 5.3: Ralph Loop - PENDING
### Checkpoint 5.4: Checkpointing - PENDING
### Checkpoint 5.5: Hat Transitions - PENDING

---

## Navigator Direction (2026-01-29) - Fix Observer Issues in Checkpoint 5.2

The Observer has identified 2 moderate and 3 minor issues. We should fix the moderate issues now as they affect runtime correctness. Minor issues can be deferred.

### Issues to Fix Now

**Issue 1 (MODERATE): sql.NullString fields render incorrectly in templates**

The templates access `{{.Task.Description}}` and `{{.Task.BranchName}}` directly, but these are `sql.NullString` types. They will render as `{<string> true}` instead of just the value.

**Fix:** Add accessor methods to `db.Task` that unwrap NullString values:

In `internal/db/models.go`, add methods to the Task struct:

```go
// GetDescription returns the description string, or empty if null
func (t *Task) GetDescription() string {
    if t.Description.Valid {
        return t.Description.String
    }
    return ""
}

// GetBranchName returns the branch name string, or empty if null
func (t *Task) GetBranchName() string {
    if t.BranchName.Valid {
        return t.BranchName.String
    }
    return ""
}

// GetWorktreePath returns the worktree path string, or empty if null
func (t *Task) GetWorktreePath() string {
    if t.WorktreePath.Valid {
        return t.WorktreePath.String
    }
    return ""
}

// GetHat returns the hat string, or empty if null
func (t *Task) GetHat() string {
    if t.Hat.Valid {
        return t.Hat.String
    }
    return ""
}

// GetParentID returns the parent ID string, or empty if null
func (t *Task) GetParentID() string {
    if t.ParentID.Valid {
        return t.ParentID.String
    }
    return ""
}
```

Then update the templates in `prompts/hats/` to use these methods:
- `{{.Task.Description}}` → `{{.Task.GetDescription}}`
- `{{.Task.BranchName}}` → `{{.Task.GetBranchName}}`
- `{{.Task.WorktreePath}}` → `{{.Task.GetWorktreePath}}`

Update these files:
- `prompts/hats/implementer.md` (lines 8, 10)
- `prompts/hats/planner.md` (line 8)
- `prompts/hats/architect.md` (line 8)
- `prompts/hats/debugger.md` (line 8)
- `prompts/hats/conflict_manager.md` (lines 10, 11)

**Issue 2 (MODERATE): devops.md template produces empty output when Toolbelt is nil**

In `prompts/hats/devops.md`, the `{{range .Toolbelt}}` silently produces nothing if Toolbelt is nil or empty.

**Fix:** Update devops.md to handle empty Toolbelt:

```markdown
## Available Toolbelt Services
{{if .Toolbelt}}
{{range .Toolbelt}}
- **{{.Name}}:** {{.Status}}
{{end}}
{{else}}
No toolbelt services configured.
{{end}}
```

**Issue 3 (MINOR): SetDefaults has unused `dollarBudget *int64` parameter**

In `internal/session/manager.go:87`, remove the unused `dollarBudget *int64` parameter.

Change:
```go
func (m *Manager) SetDefaults(maxIterations int, tokenBudget, dollarBudget *int64, dollarBudgetFloat *float64)
```

To:
```go
func (m *Manager) SetDefaults(maxIterations int, tokenBudget *int64, dollarBudgetFloat *float64)
```

**Issue 4 (MINOR): ListHats returns unsorted order**

In `internal/session/prompts.go:90-96`, sort the hats slice before returning.

Add `sort.Strings(hats)` before the return statement, and import `sort`.

**Issue 5 (MINOR): No validation that all ValidHats have templates**

In `internal/session/prompts.go`, add validation in `LoadAll()` to check that all ValidHats have templates loaded.

After loading all templates, add:
```go
// Validate all required hats have templates
for _, hat := range ValidHats {
    if _, exists := p.templates[hat]; !exists {
        return fmt.Errorf("missing template for required hat: %s", hat)
    }
}
```

### Steps for Driver

1. Add accessor methods to `internal/db/models.go` for NullString fields
2. Update prompt templates to use accessor methods (5 files)
3. Fix devops.md Toolbelt handling
4. Fix SetDefaults unused parameter in manager.go
5. Add sort to ListHats in prompts.go
6. Add ValidHats validation in LoadAll()
7. Run `go build ./...` to verify
8. Emit `build.done` with result

### Success Criteria
- `go build ./...` passes
- All NullString fields properly unwrapped in templates
- devops.md handles empty Toolbelt gracefully
- SetDefaults has no unused parameters
- ListHats returns deterministic sorted order
- LoadAll validates all ValidHats have templates

---

## Observer Review (2026-01-29) - Fixes Verified

All 5 issues from the previous Observer review have been addressed correctly:

### ✓ Issue 1: NullString accessor methods
- Added `GetDescription()`, `GetBranchName()`, `GetWorktreePath()`, `GetHat()`, `GetParentID()` methods to `internal/db/models.go:167-205`
- Templates updated to use these methods (verified in implementer.md, planner.md, architect.md, debugger.md, conflict_manager.md)

### ✓ Issue 2: devops.md Toolbelt handling
- `prompts/hats/devops.md:11-17` now properly handles nil Toolbelt with `{{if .Toolbelt}}` check and fallback message

### ✓ Issue 3: SetDefaults unused parameter
- `internal/session/manager.go:87` now has correct signature: `SetDefaults(maxIterations int, tokenBudget *int64, dollarBudgetFloat *float64)`

### ✓ Issue 4: ListHats sort
- `internal/session/prompts.go:103` now calls `sort.Strings(hats)` before returning

### ✓ Issue 5: ValidHats validation
- `internal/session/prompts.go:72-77` now validates all required hats have templates after loading

### Build Status
- `go build ./...` passes ✓

### New Observation: Minor Improvement Opportunity

**slices import could replace sort** (MINOR - defer)
- In `prompts.go:9`, both `slices` and `sort` packages are imported
- Could use `slices.Sort(hats)` instead of `sort.Strings(hats)` for consistency
- Not blocking - works correctly as-is

### Checkpoint 5.2 Status: COMPLETE
All fixes verified. Ready to proceed to Checkpoint 5.3: Ralph Loop.

---

## Navigator Direction (2026-01-29) - Checkpoint 5.3: Ralph Loop

The Ralph Loop is the core iteration mechanism that runs Claude Code sessions. Based on the objective spec, the loop needs to:

1. Run iterations of Claude Code with the session's prompt
2. Detect completion (task done, hat transition needed, or failure)
3. Enforce budget limits (tokens, dollars, iterations)
4. Handle pausing/stopping gracefully

### Implementation Plan

Create a new file `internal/session/ralph.go` with the Ralph Loop implementation.

#### File: `internal/session/ralph.go`

Create a `RalphLoop` struct that encapsulates the iteration logic:

```go
// RalphLoop manages the Claude Code iteration loop for a session
type RalphLoop struct {
    session      *ActiveSession
    promptLoader *PromptLoader
    db           *db.DB

    // Callbacks for events
    onIteration    func(iteration int, tokensUsed int64)
    onCompletion   func(outcome string)
    onBudgetExceed func(budgetType string, used, limit float64)
}
```

#### Core Methods Needed:

1. **`NewRalphLoop(session, promptLoader, db)`** - Constructor

2. **`Run(ctx context.Context) (Outcome, error)`** - Main loop:
   - Get rendered prompt from PromptLoader
   - For each iteration (up to MaxIterations):
     - Check if context cancelled → return Paused/Stopped
     - Check budgets (tokens, dollars) → if exceeded, pause and wait for user
     - Execute Claude Code iteration (placeholder for now - just simulate)
     - Parse output for completion signals:
       - `LOOP_COMPLETE` → return Completed
       - `HAT_TRANSITION:<hat>` → return TransitionNeeded
       - Error patterns → return Failed
     - Update session metrics (tokens, iteration count)
     - Save checkpoint if iteration % 5 == 0
   - If max iterations reached → return MaxIterationsReached

3. **`executeIteration(ctx, prompt string) (IterationResult, error)`** - Execute single iteration:
   - For now, create a placeholder that simulates Claude Code execution
   - Later (Checkpoint 5.1 was SDK integration) will call actual Claude Code
   - Return: output text, tokens used, any errors

4. **`checkBudgets() error`** - Check if any budget exceeded:
   - Compare TokensUsed vs TokensBudget
   - Compare DollarsUsed vs DollarsBudget
   - Return error describing which budget exceeded (if any)

5. **`parseOutcome(output string) Outcome`** - Parse Claude's output:
   - Look for `LOOP_COMPLETE` keyword
   - Look for `HAT_TRANSITION:` prefix
   - Look for error patterns
   - Return appropriate Outcome enum

#### Outcome Enum:

```go
type Outcome string

const (
    OutcomeContinue           Outcome = "continue"
    OutcomeCompleted          Outcome = "completed"
    OutcomeTransitionNeeded   Outcome = "transition_needed"
    OutcomeBudgetExceeded     Outcome = "budget_exceeded"
    OutcomeMaxIterations      Outcome = "max_iterations"
    OutcomeFailed             Outcome = "failed"
    OutcomePaused             Outcome = "paused"
    OutcomeStopped            Outcome = "stopped"
)
```

#### IterationResult struct:

```go
type IterationResult struct {
    Output     string
    TokensUsed int64
    Duration   time.Duration
    Error      error
}
```

### Update `manager.go`

Update `runSession` method to use the Ralph Loop:

```go
func (m *Manager) runSession(ctx context.Context, session *ActiveSession) {
    defer close(session.done)

    m.mu.Lock()
    session.State = StateRunning
    m.mu.Unlock()

    // Create and run the Ralph loop
    loop := NewRalphLoop(session, m.promptLoader, m.db)
    loop.onIteration = func(iteration int, tokens int64) {
        m.mu.Lock()
        session.IterationCount = iteration
        session.TokensUsed = tokens
        session.LastActivity = time.Now()
        m.mu.Unlock()
    }

    outcome, err := loop.Run(ctx)

    // Map outcome to final state
    m.mu.Lock()
    switch outcome {
    case OutcomeCompleted:
        session.State = StateCompleted
    case OutcomeFailed:
        session.State = StateFailed
    case OutcomePaused, OutcomeBudgetExceeded:
        session.State = StatePaused
    case OutcomeStopped:
        session.State = StateStopped
    default:
        if session.State == StateStopping {
            session.State = StateStopped
        } else if session.State == StatePaused {
            // Keep paused
        } else {
            session.State = StateCompleted
        }
    }
    // ... rest of cleanup
}
```

### Steps for Driver

1. Create `internal/session/ralph.go` with:
   - `Outcome` enum constants
   - `IterationResult` struct
   - `RalphLoop` struct with fields
   - `NewRalphLoop()` constructor
   - `Run()` method with the main loop logic
   - `executeIteration()` placeholder (simulates execution)
   - `checkBudgets()` method
   - `parseOutcome()` method

2. Update `internal/session/manager.go`:
   - Modify `runSession()` to use `RalphLoop`
   - Wire up the callbacks

3. Run `go build ./...` to verify compilation

### Success Criteria
- `go build ./...` passes
- `RalphLoop` struct exists with all required methods
- `runSession` uses the Ralph loop instead of placeholder
- Budgets are checked each iteration
- Outcomes are properly detected from output

### Notes
- `executeIteration` is a placeholder for now - actual Claude Code invocation comes from Checkpoint 5.1
- Checkpointing (5.4) and hat transitions (5.5) are separate checkpoints
- Focus on the loop structure and budget enforcement first
