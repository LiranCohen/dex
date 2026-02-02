# Loop Resilience Improvements

**Priority**: Medium (Quick Wins + Medium Impact)
**Effort**: Low-Medium
**Impact**: Medium-High

## Problem

Dex sessions can get stuck or degrade in ways the current `loop_health.go` doesn't fully detect:

1. **Unbounded runtime**: Sessions run indefinitely if they don't hit iteration limits
2. **Validation failures**: Malformed Claude output (bad JSON, invalid tool calls) isn't tracked distinctly from execution failures
3. **Opaque quality gate failures**: Quality gates return pass/fail but don't provide structured feedback for recovery

## Solution Overview

Extend the existing loop health and quality gate systems with:

1. **Max Runtime Termination**: Hard time limit (default: 4 hours)
2. **Validation Failure Tracking**: Separate counter for parsing/validation errors
3. **Backpressure Quality Gates**: Transform failures into structured events with suggestions

These are additive improvements to existing infrastructure.

## 1. Max Runtime Termination

### Extend ActiveSession

`ActiveSession` in `internal/session/manager.go` already has `StartedAt` (set in `Manager.Start()`). Just add `MaxRuntime`:

```go
type ActiveSession struct {
    // ... existing fields
    // StartedAt already exists and is set when session starts

    MaxRuntime time.Duration // Default: 4 hours - NEW
}
```

**Note:** `StartedAt` is already set in `Manager.Start()`:
```go
session.StartedAt = time.Now()
```

In `Manager.CreateSession()` (or Start), set the max runtime:

```go
session := &ActiveSession{
    // ... existing initialization
    MaxRuntime: m.config.MaxRuntime, // From config or env
}
```

### Add Check in Ralph Loop

In `internal/session/ralph.go`, add to the existing budget check logic:

```go
// In the budget checking section of the main loop:

// New: runtime check
if r.session.MaxRuntime > 0 && time.Since(r.session.StartedAt) > r.session.MaxRuntime {
    return TerminationMaxRuntime, nil
}
```

### Add Termination Reason

In `internal/session/termination.go`, add to existing constants:

```go
const (
    // ... existing reasons (TerminationMaxIterations, TerminationMaxTokens, etc.)
    TerminationMaxRuntime TerminationReason = "max_runtime" // NEW
)
```

Update `IsExhaustion()` to include the new reason:

```go
func (t TerminationReason) IsExhaustion() bool {
    switch t {
    case TerminationMaxIterations, TerminationMaxTokens, TerminationMaxCost,
         TerminationQualityGateExhausted, TerminationLoopThrashing,
         TerminationConsecutiveFailures, TerminationMaxRuntime: // Add this
        return true
    default:
        return false
    }
}
```

### Configuration

```go
// Environment variable
// DEX_MAX_RUNTIME_HOURS=4

type Config struct {
    // ... existing fields
    MaxRuntime time.Duration `json:"max_runtime"` // Default: 4h
}

func DefaultConfig() Config {
    return Config{
        // ...
        MaxRuntime: 4 * time.Hour,
    }
}
```

### Activity Logging

```go
if reason == TerminationReasonMaxRuntime {
    r.activity.Log(ActivityLoopHealth, fmt.Sprintf(
        "session terminated: max runtime exceeded (%v)",
        r.session.MaxRuntime,
    ))
}
```

## 2. Tool Repetition Detection

Detect and prevent infinite loops from identical consecutive tool calls.

**Note:** Dex already has `TransitionTracker` (`transition_tracker.go`) for detecting hat transition loops. This section adds similar detection for tool calls.

**Inspired by Goose's RepetitionInspector.**

### RepetitionInspector

```go
// internal/session/repetition.go

type ToolCallSignature struct {
    Name   string
    Params string // JSON-serialized params for comparison
}

func (t ToolCallSignature) Equals(other ToolCallSignature) bool {
    return t.Name == other.Name && t.Params == other.Params
}

type RepetitionInspector struct {
    maxRepetitions int
    lastCall       *ToolCallSignature
    repeatCount    int
    callCounts     map[string]int // Track total calls per tool
}

func NewRepetitionInspector(maxRepetitions int) *RepetitionInspector {
    return &RepetitionInspector{
        maxRepetitions: maxRepetitions,
        callCounts:     make(map[string]int),
    }
}

func (ri *RepetitionInspector) Check(call ToolCallSignature) (bool, string) {
    ri.callCounts[call.Name]++

    if ri.lastCall != nil && ri.lastCall.Equals(call) {
        ri.repeatCount++
        if ri.repeatCount > ri.maxRepetitions {
            return false, fmt.Sprintf(
                "tool '%s' called %d times consecutively with identical parameters",
                call.Name, ri.repeatCount)
        }
    } else {
        ri.repeatCount = 1
    }

    ri.lastCall = &call
    return true, ""
}

func (ri *RepetitionInspector) Reset() {
    ri.lastCall = nil
    ri.repeatCount = 0
    ri.callCounts = make(map[string]int)
}
```

### Integration with LoopHealth

```go
type LoopHealth struct {
    // ... existing fields

    // Repetition detection
    RepetitionInspector *RepetitionInspector
}

func NewLoopHealth(activity *ActivityRecorder) *LoopHealth {
    return &LoopHealth{
        // ...
        RepetitionInspector: NewRepetitionInspector(5), // Default: 5 consecutive identical calls
    }
}
```

### Integration in Tool Execution

```go
func (r *RalphLoop) executeToolCall(call ToolCall) error {
    sig := ToolCallSignature{
        Name:   call.Name,
        Params: string(call.ParamsJSON),
    }

    allowed, reason := r.health.RepetitionInspector.Check(sig)
    if !allowed {
        r.activity.Log(ActivityLoopHealth, fmt.Sprintf("repetition blocked: %s", reason))

        // Inject feedback to agent instead of executing
        r.addErrorMessage(fmt.Sprintf(
            "Tool call blocked: %s. Try a different approach or parameters.",
            reason))

        if r.health.RecordRepetitionBlock() != "" {
            return &TerminationError{Reason: TerminationReasonRepetitionLoop}
        }
        return nil
    }

    // Continue with normal execution...
}
```

### Termination Reason

```go
const (
    // ... existing reasons
    TerminationRepetitionLoop TerminationReason = "repetition_loop" // NEW
)
```

## 3. Validation Failure Tracking

### Extend LoopHealth

The existing `LoopHealth` in `internal/session/loop_health.go` already tracks:
- `ConsecutiveFailures` - general failures
- `ConsecutiveBlocked` - quality gate blocks
- `TotalFailures` - cumulative failures
- `QualityGateAttempts` - quality gate attempts
- `TaskBlockCounts` - per-checklist-item blocks

Add validation-specific tracking:

```go
type LoopHealth struct {
    // ... existing fields

    // New: validation tracking (distinct from tool execution failures)
    ConsecutiveValidationFailures int
    TotalValidationFailures       int
}

const (
    // ... existing defaults (DefaultMaxConsecutiveFailures = 5, etc.)
    DefaultMaxConsecutiveValidationFailures = 3 // NEW
)
```

### Define Validation Failures

Validation failures are distinct from tool execution failures:

- JSON parsing errors in tool_use blocks
- Missing required fields in tool calls
- Invalid tool names
- Malformed completion signals (e.g., `TASK_COMPLETE` with garbage)
- Truncated responses

### Add Tracking Methods

```go
func (h *LoopHealth) RecordValidationFailure(description string) TerminationReason {
    h.ConsecutiveValidationFailures++
    h.TotalValidationFailures++

    // Log it
    h.activity.Log(ActivityLoopHealth, fmt.Sprintf(
        "validation failure %d/%d: %s",
        h.ConsecutiveValidationFailures,
        MaxConsecutiveValidationFailures,
        description,
    ))

    if h.ConsecutiveValidationFailures >= MaxConsecutiveValidationFailures {
        return TerminationReasonValidationFailure
    }
    return ""
}

func (h *LoopHealth) ResetValidationFailures() {
    h.ConsecutiveValidationFailures = 0
}
```

### Add Termination Reason

```go
const (
    // ... existing reasons (TerminationConsecutiveFailures, etc.)
    TerminationValidationFailure TerminationReason = "validation_failure" // NEW
)
```

### Integrate in Ralph Loop

In `internal/session/ralph.go`, when processing Claude's response:

```go
func (r *RalphLoop) processResponse(response *anthropic.Response) error {
    // Parse tool calls
    toolCalls, err := r.parseToolCalls(response)
    if err != nil {
        // This is a validation failure, not a tool execution failure
        if reason := r.health.RecordValidationFailure(err.Error()); reason != "" {
            return &TerminationError{Reason: reason}
        }
        // Inform Claude about the parsing error and continue
        r.addErrorMessage(fmt.Sprintf("Failed to parse your response: %v. Please try again with valid JSON.", err))
        return nil
    }

    // Successful parse - reset validation counter
    r.health.ResetValidationFailures()

    // Continue with tool execution...
}
```

### Detailed Logging

```go
type ValidationFailure struct {
    Expected string `json:"expected"`
    Received string `json:"received"`
    Error    string `json:"error"`
}

func (r *RalphLoop) logValidationFailure(vf ValidationFailure) {
    r.activity.LogWithMetadata(ActivityLoopHealth, "validation_failure", map[string]string{
        "expected": vf.Expected,
        "received": truncate(vf.Received, 200),
        "error":    vf.Error,
    })
}
```

## 4. Large Response Handling

Prevent context explosion from verbose tool outputs by writing large responses to temporary files.

**Inspired by Goose's large_response_handler.**

### Implementation

```go
// internal/tools/large_response.go

const (
    LargeResponseThreshold = 200_000 // characters
    TempDirName           = "dex_tool_responses"
)

// ProcessToolResponse handles large tool outputs by writing to temp files
func ProcessToolResponse(toolName string, result string) string {
    if len(result) <= LargeResponseThreshold {
        return result
    }

    // Write to temp file
    tempDir := filepath.Join(os.TempDir(), TempDirName)
    os.MkdirAll(tempDir, 0755)

    filename := fmt.Sprintf("%s_%s.txt", toolName, time.Now().Format("20060102_150405"))
    filePath := filepath.Join(tempDir, filename)

    if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
        // Fall back to truncation if write fails
        return truncateWithWarning(result, LargeResponseThreshold)
    }

    return fmt.Sprintf(`Tool response too large (%d characters). Full output saved to: %s

To examine the output:
- Use read_file tool to view the file
- Use grep tool to search within the file
- The file will be cleaned up when the session ends

Preview (first 1000 chars):
%s...`,
        len(result),
        filePath,
        result[:min(1000, len(result))])
}

func truncateWithWarning(content string, maxLen int) string {
    if len(content) <= maxLen {
        return content
    }
    return content[:maxLen] + fmt.Sprintf("\n\n[TRUNCATED: Response was %d characters, showing first %d]", len(content), maxLen)
}

// CleanupTempResponses removes old temp files (call on session end)
func CleanupTempResponses() {
    tempDir := filepath.Join(os.TempDir(), TempDirName)
    entries, err := os.ReadDir(tempDir)
    if err != nil {
        return
    }

    cutoff := time.Now().Add(-24 * time.Hour)
    for _, entry := range entries {
        info, err := entry.Info()
        if err != nil {
            continue
        }
        if info.ModTime().Before(cutoff) {
            os.Remove(filepath.Join(tempDir, entry.Name()))
        }
    }
}
```

### Integration with Tool Execution

```go
func (r *RalphLoop) executeToolCall(call ToolCall) (string, error) {
    result, err := r.executor.Execute(call)
    if err != nil {
        return "", err
    }

    // Process large responses
    processed := ProcessToolResponse(call.Name, result)

    if processed != result {
        r.activity.Log(ActivityDebugLog, fmt.Sprintf(
            "large response from %s (%d chars) written to temp file",
            call.Name, len(result)))
    }

    return processed, nil
}
```

### Session Cleanup

```go
func (r *RalphLoop) cleanup() {
    // ... existing cleanup
    CleanupTempResponses()
}
```

## 5. Backpressure Quality Gates

Transform quality gate failures into structured events that help the agent recover.

The existing `QualityGate` in `quality_gate.go` already returns `GateResult` with:
- `Passed bool` - overall pass/fail
- `Tests *CheckResult` - test results
- `Lint *CheckResult` - lint results
- `Build *CheckResult` - build results
- `Feedback string` - QUALITY_PASSED or QUALITY_BLOCKED message

Each `CheckResult` has: `Passed`, `Output`, `DurationMs`, `Skipped`, `SkipReason`.

### Enhanced BlockingEvent Structure

Add a new structure that wraps `GateResult` with recovery suggestions:

```go
type BlockingEvent struct {
    OriginalRequest string      `json:"original_request"` // e.g., "TASK_COMPLETE"
    BlockedBy       string      `json:"blocked_by"`       // e.g., "tests"
    Summary         string      `json:"summary"`          // One-line summary
    Details         []string    `json:"details"`          // Specific failures
    Suggestions     []string    `json:"suggestions"`      // Actionable next steps
    GateResult      *GateResult `json:"gate_result"`      // Original result
}
```

### Transform Quality Gate Results

Add a new method to `internal/session/quality_gate.go` that wraps `Validate()`:

```go
func (qg *QualityGate) ValidateWithBackpressure(ctx context.Context, opts TaskCompleteOpts) (*BlockingEvent, error) {
    result := qg.Validate(ctx, opts)

    if result.Passed {
        return nil, nil // All passed
    }

    // Determine which gate blocked
    blockedBy := "unknown"
    var failedCheck *CheckResult
    if result.Tests != nil && !result.Tests.Passed && !result.Tests.Skipped {
        blockedBy = "tests"
        failedCheck = result.Tests
    } else if result.Lint != nil && !result.Lint.Passed && !result.Lint.Skipped {
        blockedBy = "lint"
        failedCheck = result.Lint
    } else if result.Build != nil && !result.Build.Passed && !result.Build.Skipped {
        blockedBy = "build"
        failedCheck = result.Build
    }

    return &BlockingEvent{
        OriginalRequest: "TASK_COMPLETE",
        BlockedBy:       blockedBy,
        Summary:         fmt.Sprintf("%s failed", blockedBy),
        Details:         extractFailureDetails(blockedBy, failedCheck),
        Suggestions:     generateSuggestions(blockedBy),
        GateResult:      result,
    }, nil
}

func extractFailureDetails(gateName string, check *CheckResult) []string {
    if check == nil {
        return nil
    }
    details := []string{}

    switch gateName {
    case "tests":
        // Parse test output for individual failures
        details = parseTestFailures(check.Output)
    case "lint":
        details = parseLintErrors(check.Output)
    case "build":
        details = parseBuildErrors(check.Output)
    }

    return details
}

func generateSuggestions(gateName string) []string {
    switch gateName {
    case "tests":
        return []string{
            "Review the failing test assertions",
            "Check if recent changes broke existing behavior",
            "Run the specific failing test locally for more details",
        }
    case "lint":
        return []string{
            "Fix the reported style/lint issues",
            "Run the linter to see all issues",
        }
    case "build":
        return []string{
            "Fix compilation errors before proceeding",
            "Check for missing imports or typos",
        }
    default:
        return nil
    }
}
```

### Format for Agent

```go
func (be *BlockingEvent) Format() string {
    var sb strings.Builder

    sb.WriteString(fmt.Sprintf("## Completion Blocked: %s\n\n", be.Summary))
    sb.WriteString(fmt.Sprintf("Your `%s` request was blocked.\n\n", be.OriginalRequest))

    if len(be.Details) > 0 {
        sb.WriteString("### Issues Found\n")
        for _, detail := range be.Details {
            sb.WriteString(fmt.Sprintf("- %s\n", detail))
        }
        sb.WriteString("\n")
    }

    if len(be.Suggestions) > 0 {
        sb.WriteString("### Suggested Actions\n")
        for i, suggestion := range be.Suggestions {
            sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
        }
        sb.WriteString("\n")
    }

    // Format gate status from GateResult
    sb.WriteString("### Gate Status\n")
    if be.GateResult != nil {
        formatCheck := func(name string, check *CheckResult) {
            if check == nil {
                return
            }
            icon := "[ ]"
            status := "failed"
            if check.Passed {
                icon = "[x]"
                status = "passed"
            } else if check.Skipped {
                icon = "[~]"
                status = "skipped"
            }
            sb.WriteString(fmt.Sprintf("%s %s: %s\n", icon, name, status))
        }
        formatCheck("tests", be.GateResult.Tests)
        formatCheck("lint", be.GateResult.Lint)
        formatCheck("build", be.GateResult.Build)
    }

    return sb.String()
}
```

### Integration

Replace current quality gate handling in ralph.go:

```go
func (r *RalphLoop) handleCompletionRequest() error {
    blocking, err := r.qualityGate.ValidateWithBackpressure("TASK_COMPLETE")
    if err != nil {
        return err
    }

    if blocking != nil {
        // Don't fail - inject structured feedback and continue
        r.messages = append(r.messages, Message{
            Role:    "user",
            Content: blocking.Format(),
        })

        // Record for activity log
        r.activity.Log(ActivityQualityGate, fmt.Sprintf(
            "completion blocked by %s", blocking.BlockedBy,
        ))

        // Track for loop health
        r.health.RecordQualityGateFailure(blocking.BlockedBy)

        return nil // Continue loop
    }

    // All gates passed - complete the task
    return r.completeTask()
}
```

### Cascading Gates (Future Enhancement)

The existing `QualityGate.Validate()` runs checks independently. A future enhancement could add dependency ordering (e.g., skip lint if build fails):

```go
// Future: Add to TaskCompleteOpts
type TaskCompleteOpts struct {
    // ... existing fields
    CascadeFailures bool // If true, skip subsequent gates after first failure
}
```

For now, all gates run independently which provides complete feedback to the agent.

## Configuration

Combined configuration for all resilience features:

```go
type ResilienceConfig struct {
    // Max runtime
    MaxRuntime time.Duration `json:"max_runtime"` // Default: 4h

    // Repetition detection
    MaxConsecutiveRepetitions int `json:"max_consecutive_repetitions"` // Default: 5

    // Validation failures
    MaxConsecutiveValidationFailures int `json:"max_consecutive_validation_failures"` // Default: 3

    // Large response handling
    LargeResponseThreshold int `json:"large_response_threshold"` // Default: 200000 chars

    // Quality gates
    EnableBackpressure bool     `json:"enable_backpressure"` // Default: true
    GateDependencies   []string `json:"gate_dependencies"`   // e.g., ["lint:build"]
}
```

Environment variables:
```bash
DEX_MAX_RUNTIME_HOURS=4
DEX_MAX_REPETITIONS=5
DEX_MAX_VALIDATION_FAILURES=3
DEX_LARGE_RESPONSE_THRESHOLD=200000
DEX_QUALITY_BACKPRESSURE=true
```

## Acceptance Criteria

### Max Runtime
- [x] StartedAt tracked on session start (already existed)
- [x] MaxRuntime configurable (`Manager.SetMaxRuntime()`, defaults to 4h)
- [x] Sessions terminate after max runtime (`ErrRuntimeLimit` check in `checkBudget()`)
- [x] TerminationMaxRuntime reason added
- [x] Clean state save on runtime termination (defer checkpoint in `ralph.go:158-166`)

### Tool Repetition Detection
- [x] RepetitionInspector tracks consecutive identical calls (`internal/session/repetition.go`)
- [x] Identical = same tool name + same parameters (`ToolCallSignature.Equals()`)
- [x] Block after N consecutive repetitions (default: 5, configurable)
- [x] LoopHealth integration (`CheckToolCall()`, `ResetRepetition()`)
- [x] TerminationRepetitionLoop reason added
- [x] ShouldTerminate checks repetition blocks
- [x] Integration in ralph.go tool execution (`ralph.go:322-334`, injects feedback message)

### Validation Failures
- [x] ConsecutiveValidationFailures tracked in LoopHealth
- [x] Validation failures distinguished from execution failures (`RecordValidationFailure()`)
- [x] Counter reset on successful parse (`RecordSuccess()` resets it)
- [x] Termination after N consecutive failures (DefaultMaxValidationFailures = 3)
- [x] TerminationValidationFailure reason added
- [x] Detailed logging via `LoopHealth.RecordValidationFailure()` includes description

### Large Response Handling
- [x] Responses >200k chars written to temp files (`internal/tools/large_response.go`)
- [x] Agent receives file path + preview (`ProcessLargeResponse()`)
- [x] Temp files cleanup functions (`CleanupTempResponses()`, `CleanupOldTempResponses()`)
- [x] Fallback to truncation if write fails (`truncateWithWarning()`)
- [x] Integrated in tool executor (`executor.go`)
- [x] Call cleanup on session end (`ralph.go:131-135` defer cleanup)

### Backpressure Quality Gates (Deferred)
- [ ] BlockingEvent struct defined
- [ ] Quality gates return structured failures
- [ ] Failure details extracted from output
- [ ] Suggestions generated per gate type
- [ ] Gate status shows passed/failed/skipped
- [ ] Cascading dependencies respected
- [ ] Formatted message injected for agent recovery

**Note:** Backpressure Quality Gates deferred - existing quality gate infrastructure works well for current needs.

## Migration Notes

This consolidates and replaces:
- `01-max-runtime-termination.md`
- `02-validation-failure-tracking.md`
- `06-backpressure-quality-gates.md`

All three improvements extend existing infrastructure (`loop_health.go`, `quality_gate.go`, `termination.go`) rather than introducing new systems.
