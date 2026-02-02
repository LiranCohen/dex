# Context & Continuity Management

**Priority**: High
**Effort**: Medium
**Impact**: High

## Problem

Dex carries full conversation history across iterations. For long-running tasks this causes:
- Context window exhaustion (approaching Claude's 200K limit)
- History noise accumulation (failed attempts, superseded plans)
- Token costs increasing linearly with task duration
- Complex resumption requiring full history replay

Currently, checkpoints store mechanical data (messages, iteration count, tokens) but lack human-readable summaries for review or efficient resumption.

## Solution Overview

Implement a three-layer context management system:

1. **Scratchpad**: Persistent thinking document updated each iteration
2. **Context Guards**: Proactive monitoring with automatic compaction
3. **Handoff Summaries**: Structured checkpoint metadata for review/resume

These work together to maintain continuity while keeping context lean.

## Implementation

### Layer 0: Progressive Tool Response Removal

Before full compaction, try removing tool responses progressively. This preserves more conversational context while reducing the bulk of verbose tool outputs.

**Inspired by Goose's context_mgmt system.**

```go
// internal/session/compaction.go

// RemovalLevels defines progressive percentages of tool responses to remove
var RemovalLevels = []int{0, 10, 20, 50, 100}

func (r *RalphLoop) compactHistoryProgressive() error {
    targetTokens := r.contextCompactAt * 80 / 100 // Target 80% of compact threshold

    for _, pct := range RemovalLevels {
        filtered := filterToolResponses(r.messages, pct)
        tokens := estimateTokens(filtered)

        if tokens < targetTokens {
            r.activity.Log(ActivityDebugLog, fmt.Sprintf(
                "compaction: removed %d%% of tool responses, %d -> %d tokens",
                pct, estimateTokens(r.messages), tokens))
            r.messages = filtered
            return r.saveCheckpoint("compaction")
        }
    }

    // All tool responses removed but still over limit - fall back to full summarization
    return r.compactHistoryWithSummary()
}

// filterToolResponses removes a percentage of tool responses from the middle outward
// Middle-out removal preserves recent context and initial task understanding
func filterToolResponses(messages []Message, removePercent int) []Message {
    if removePercent == 0 {
        return messages
    }

    // Find indices of tool response messages
    toolIndices := []int{}
    for i, msg := range messages {
        if msg.Role == "user" && hasToolResponse(msg) {
            toolIndices = append(toolIndices, i)
        }
    }

    if len(toolIndices) == 0 {
        return messages
    }

    // Calculate how many to remove
    numToRemove := (len(toolIndices) * removePercent) / 100
    if numToRemove == 0 && removePercent > 0 {
        numToRemove = 1
    }

    // Remove from middle outward
    middle := len(toolIndices) / 2
    toRemove := make(map[int]bool)

    for i := 0; i < numToRemove; i++ {
        if i%2 == 0 {
            offset := i / 2
            if middle-offset-1 >= 0 {
                toRemove[toolIndices[middle-offset-1]] = true
            }
        } else {
            offset := i / 2
            if middle+offset < len(toolIndices) {
                toRemove[toolIndices[middle+offset]] = true
            }
        }
    }

    // Filter out removed messages
    result := make([]Message, 0, len(messages)-numToRemove)
    for i, msg := range messages {
        if !toRemove[i] {
            result = append(result, msg)
        }
    }

    return result
}
```

### Layer 1: Scratchpad

A persistent markdown document that replaces the need to carry full history.

#### Structure

```markdown
# Task Scratchpad

## Current Understanding
- This is a Go project using the standard library
- Main entry point is cmd/dex/main.go
- Tests use testify for assertions

## Current Plan
1. [x] Explore codebase structure
2. [x] Identify files to modify
3. [ ] Implement the feature in internal/session/
4. [ ] Add tests
5. [ ] Run quality gates

## Key Decisions
- Using table-driven tests for consistency with existing code
- Adding new field to existing struct rather than creating new type

## Blockers
- None currently

## Last Action
Added the new field to ActiveSession struct. Next: update the database schema.
```

#### Storage

Add to `ActiveSession` in `internal/session/manager.go`:

```go
type ActiveSession struct {
    // ... existing fields (ID, TaskID, Hat, State, etc.)
    Scratchpad string // NEW
}
```

Checkpoints are stored as `map[string]any` JSON in `session_checkpoints` table. The existing checkpoint state includes:
- `iteration`, `input_tokens`, `output_tokens`, `hat`, `messages`
- `last_error`, `failed_at`, `recovery_hint` (for failure recovery)

Add scratchpad to checkpoint state:

```go
// In ralph.go saveCheckpoint():
state := map[string]any{
    // ... existing fields
    "scratchpad": r.session.Scratchpad, // NEW
}
```

#### Agent Signals

Agents update scratchpad via signals in their responses:

```
SCRATCHPAD:## Current Understanding
- This is a Go project...
[full scratchpad content]
```

Parse in Ralph loop after processing response:

```go
func parseScratchpadSignal(text string) (string, bool) {
    const prefix = "SCRATCHPAD:"
    if idx := strings.Index(text, prefix); idx != -1 {
        // Extract from prefix to end of text or next signal
        content := text[idx+len(prefix):]
        if endIdx := strings.Index(content, "\nSIGNAL:"); endIdx != -1 {
            content = content[:endIdx]
        }
        return strings.TrimSpace(content), true
    }
    return "", false
}
```

#### Prompt Instructions

Add to hat system prompts:

```yaml
## Maintaining Your Scratchpad

You have a scratchpad for maintaining continuity across iterations. After significant progress, update it with:

SCRATCHPAD:## Current Understanding
[Your understanding of the task and codebase]

## Current Plan
[Checklist of steps, mark completed with [x]]

## Key Decisions
[Important choices made and rationale]

## Blockers
[Any issues preventing progress]

## Last Action
[What you just did and what's next]

Keep it concise but complete enough to resume with only this information.
```

### Layer 2: Context Guards

Proactive monitoring to prevent context exhaustion.

#### Add to RalphLoop

```go
type RalphLoop struct {
    // ... existing fields

    // Context tracking
    contextWindowMax  int // Model limit (200000 for Claude)
    contextWarnAt     int // 80% threshold
    contextCompactAt  int // 90% threshold
}

func NewRalphLoop(...) *RalphLoop {
    return &RalphLoop{
        // ...
        contextWindowMax:  200000,
        contextWarnAt:     160000,
        contextCompactAt:  180000,
    }
}
```

#### Token Estimation

```go
func (r *RalphLoop) estimateContextTokens() int {
    total := 0

    // System prompt (estimate)
    total += len(r.systemPrompt) / 4

    // All messages
    for _, msg := range r.messages {
        // ~4 chars per token is a reasonable estimate
        total += len(msg.Content) / 4

        // Tool calls add overhead
        for _, tc := range msg.ToolUse {
            total += len(tc.Input) / 4
            total += 50 // Tool call structure overhead
        }
    }

    return total
}
```

#### Guard Check

Call before each API request:

```go
func (r *RalphLoop) checkContextGuard() error {
    tokens := r.estimateContextTokens()

    if tokens >= r.contextCompactAt {
        r.activity.Log(ActivityDebugLog, "context at 90%, triggering compaction")
        if err := r.compactHistory(); err != nil {
            return fmt.Errorf("compaction failed: %w", err)
        }
    } else if tokens >= r.contextWarnAt {
        r.activity.Log(ActivityDebugLog, fmt.Sprintf("context at %d%%, approaching limit",
            tokens*100/r.contextWindowMax))
    }

    return nil
}
```

#### History Compaction

```go
const (
    MaxRecentMessages = 10
)

func (r *RalphLoop) compactHistory() error {
    if len(r.messages) <= MaxRecentMessages {
        return nil // Nothing to compact
    }

    // Keep only recent messages
    oldMessages := r.messages[:len(r.messages)-MaxRecentMessages]
    r.messages = r.messages[len(r.messages)-MaxRecentMessages:]

    // Summarize old messages into scratchpad
    summary := r.summarizeMessages(oldMessages)
    r.session.Scratchpad = r.session.Scratchpad + "\n\n## Compacted History\n" + summary

    // Save checkpoint after compaction
    return r.saveCheckpoint("compaction")
}

func (r *RalphLoop) summarizeMessages(messages []Message) string {
    var summary strings.Builder

    for _, msg := range messages {
        // Extract significant events
        if msg.Role == "assistant" {
            // Tool calls
            for _, tc := range msg.ToolUse {
                summary.WriteString(fmt.Sprintf("- Called %s\n", tc.Name))
            }
            // Decisions (look for keywords)
            if strings.Contains(msg.Content, "decided") ||
               strings.Contains(msg.Content, "choosing") {
                summary.WriteString(fmt.Sprintf("- Decision: %s\n",
                    extractFirstSentence(msg.Content)))
            }
        }
        if msg.Role == "user" && strings.Contains(msg.Content, "QUALITY_") {
            summary.WriteString(fmt.Sprintf("- Quality gate: %s\n",
                extractQualityResult(msg.Content)))
        }
    }

    return summary.String()
}

// compactHistoryWithSummary uses LLM to create a structured summary
// Inspired by Goose's compaction prompt structure
func (r *RalphLoop) compactHistoryWithSummary() error {
    if len(r.messages) <= MaxRecentMessages {
        return nil
    }

    oldMessages := r.messages[:len(r.messages)-MaxRecentMessages]
    r.messages = r.messages[len(r.messages)-MaxRecentMessages:]

    // Use structured prompt for summarization
    summaryPrompt := buildCompactionPrompt(oldMessages)
    summary, err := r.summarizeWithLLM(summaryPrompt)
    if err != nil {
        // Fall back to simple extraction
        summary = r.summarizeMessages(oldMessages)
    }

    r.session.Scratchpad = r.session.Scratchpad + "\n\n## Compacted History\n" + summary
    return r.saveCheckpoint("compaction")
}

// Structured compaction prompt (inspired by Goose)
const compactionPromptTemplate = `Summarize this conversation history for session continuation.

**Conversation History:**
%s

Wrap your reasoning in <analysis> tags, then provide a structured summary with these sections:

1. **User Intent** - All goals and requests from the user
2. **Technical Context** - Tools, methods, patterns discovered
3. **Files & Code** - Files viewed/edited, significant code changes
4. **Errors & Fixes** - Problems encountered and how they were resolved
5. **Key Decisions** - Important choices made and rationale
6. **Pending Tasks** - Unfinished work items
7. **Current State** - Where work left off, what's next

Be thorough - this summary will be the only context for continuing the session.
Do not mention that you read a summary or that summarization occurred.`

func buildCompactionPrompt(messages []Message) string {
    var history strings.Builder
    for _, msg := range messages {
        role := "user"
        if msg.Role == "assistant" {
            role = "assistant"
        }
        history.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, truncateForSummary(msg.Content)))
    }
    return fmt.Sprintf(compactionPromptTemplate, history.String())
}

func truncateForSummary(content string) string {
    const maxLen = 2000
    if len(content) > maxLen {
        return content[:maxLen] + "... [truncated]"
    }
    return content
}
```

### Layer 3: Handoff Summaries

Structured metadata for human review and efficient resume.

#### Extend Checkpoint State

Checkpoints are `map[string]any` JSON. The existing state includes:
- `iteration` (int)
- `input_tokens`, `output_tokens` (int64)
- `hat` (string)
- `messages` ([]anthropic.Message)
- `last_error`, `failed_at`, `recovery_hint` (failure recovery context)

Add new fields to the checkpoint state map:

```go
state := map[string]any{
    // ... existing fields

    // New: Scratchpad (from Layer 1)
    "scratchpad": r.session.Scratchpad,

    // New: Handoff summary
    "handoff": r.generateHandoff(),
}

type HandoffSummary struct {
    GeneratedAt time.Time `json:"generated_at"`
    TaskTitle   string    `json:"task_title"`
    CurrentHat  string    `json:"current_hat"`

    // Git context
    Branch     string `json:"branch"`
    HeadCommit string `json:"head_commit"`

    // Progress
    CompletedItems []string `json:"completed_items"`
    RemainingItems []string `json:"remaining_items"`
    BlockingIssues []string `json:"blocking_issues,omitempty"`

    // Artifacts
    ModifiedFiles []string `json:"modified_files"`
    CreatedFiles  []string `json:"created_files"`

    // Context
    KeyDecisions []string `json:"key_decisions,omitempty"`

    // For resume
    ContinuationPrompt string `json:"continuation_prompt"`
}
```

#### Generate Handoff at Checkpoint

```go
func (r *RalphLoop) generateHandoff() *HandoffSummary {
    handoff := &HandoffSummary{
        GeneratedAt: time.Now(),
        TaskTitle:   r.task.Title,
        CurrentHat:  r.session.Hat,
    }

    // Git context
    if status, err := r.executor.GitStatus(); err == nil {
        handoff.Branch = status.Branch
        handoff.HeadCommit = status.HeadCommit
        handoff.ModifiedFiles = status.Modified
        handoff.CreatedFiles = status.Untracked
    }

    // Extract progress from checklist
    if r.task.Checklist != nil {
        for _, item := range r.task.Checklist.Items {
            if item.Status == "completed" {
                handoff.CompletedItems = append(handoff.CompletedItems, item.Description)
            } else {
                handoff.RemainingItems = append(handoff.RemainingItems, item.Description)
            }
        }
    }

    // Extract decisions from scratchpad
    handoff.KeyDecisions = extractDecisionsFromScratchpad(r.session.Scratchpad)

    // Generate continuation prompt
    handoff.ContinuationPrompt = fmt.Sprintf(
        "Continue working on: %s\nCurrent phase: %s\nNext step: %s",
        r.task.Title,
        r.session.Hat,
        firstOrDefault(handoff.RemainingItems, "Complete the task"),
    )

    return handoff
}
```

#### Use Handoff on Resume

The existing `ralph.go` already restores from checkpoints. Extend it to use handoff:

```go
func (r *RalphLoop) resumeFromCheckpoint(state map[string]any) {
    // Existing restoration (iteration, tokens, hat, messages, failure context)
    // is already implemented in ralph.go

    // Add: Restore scratchpad
    if scratchpad, ok := state["scratchpad"].(string); ok {
        r.session.Scratchpad = scratchpad
    }

    // Add: Use handoff for better resume context
    if handoffData, ok := state["handoff"].(map[string]any); ok {
        handoff := parseHandoffFromMap(handoffData)

        // Build enhanced recovery context
        var recovery strings.Builder
        recovery.WriteString("## Resuming Session\n\n")
        recovery.WriteString(fmt.Sprintf("**Task**: %s\n", handoff.TaskTitle))
        recovery.WriteString(fmt.Sprintf("**Branch**: %s\n", handoff.Branch))
        recovery.WriteString(fmt.Sprintf("**Progress**: %d/%d items completed\n\n",
            len(handoff.CompletedItems),
            len(handoff.CompletedItems)+len(handoff.RemainingItems)))

        if len(handoff.RemainingItems) > 0 {
            recovery.WriteString("**Remaining**:\n")
            for _, item := range handoff.RemainingItems {
                recovery.WriteString(fmt.Sprintf("- %s\n", item))
            }
        }

        recovery.WriteString(fmt.Sprintf("\n**Continue with**: %s\n",
            handoff.ContinuationPrompt))

        // Prepend to existing recovery message
        // (existing code already handles failure context)
    }

    if r.session.Scratchpad != "" {
        // Include scratchpad in system prompt or user context
    }
}
```

### API Endpoint

```go
// GET /api/sessions/{id}/handoff
func (s *Server) getSessionHandoff(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "id")

    checkpoint, err := s.db.GetLatestCheckpoint(sessionID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }

    if checkpoint.Handoff == nil {
        http.Error(w, "no handoff summary available", http.StatusNotFound)
        return
    }

    json.NewEncoder(w).Encode(checkpoint.Handoff)
}
```

## Configuration

```go
type ContextConfig struct {
    // Context window limits
    ContextWindowMax  int `json:"context_window_max"`  // Default: 200000
    ContextWarnPct    int `json:"context_warn_pct"`    // Default: 80
    ContextCompactPct int `json:"context_compact_pct"` // Default: 90

    // History management
    MaxRecentMessages int `json:"max_recent_messages"` // Default: 10

    // Checkpoint settings
    CheckpointInterval int  `json:"checkpoint_interval"` // Default: 5 iterations
    GenerateHandoff    bool `json:"generate_handoff"`    // Default: true
}
```

## Acceptance Criteria

### Scratchpad
- [x] Scratchpad field added to ActiveSession (`manager.go`)
- [x] SCRATCHPAD: signal parsed from responses (`ralph.go:parseScratchpadSignal()`)
- [x] Scratchpad stored in checkpoint (`ralph.go:checkpoint()`)
- [x] Hat prompts include scratchpad instructions (`prompts/components/system.yaml`)
- [x] Scratchpad sanitized on restore (`ralph.go:RestoreFromCheckpoint()`)

### Context Guards
- [x] Token estimation implemented (`compaction.go:EstimateTokens()`)
- [x] Warning logged at 80% threshold (`ContextGuard.CheckAndCompact()`)
- [x] Automatic compaction at 90% threshold (`ContextGuard.CheckAndCompact()`)
- [x] Progressive tool response removal (0%, 10%, 20%, 50%, 100%) (`compaction.go:RemovalLevels`)
- [x] Middle-out removal strategy (preserves recent and initial context) (`filterToolResponses()`)
- [x] History summarization preserves key events (`summarizeMessages()`)
- [x] Checkpoint saved after compaction (`ralph.go` calls checkpoint after compaction)
- [ ] Structured summarization with LLM fallback (deferred - basic summarization works)

### Handoff Summaries
- [x] HandoffSummary struct defined (`handoff.go`)
- [x] Git context captured (branch) (`HandoffGenerator.Generate()`)
- [x] Progress extracted from checklist (`HandoffGenerator.Generate()`)
- [x] Continuation prompt generated (`HandoffSummary.ContinuationPrompt`)
- [x] Key decisions extracted from scratchpad (`extractDecisionsFromScratchpad()`)
- [x] Resume uses handoff context (`ralph.go:RestoreFromCheckpoint()`)
- [ ] API endpoint for viewing handoff (deferred - can be added when needed)

## Migration Notes

This consolidates and replaces:
- `03-handoff-summary-checkpoint.md`
- `05-scratchpad-pattern.md`
- `08-stateless-iteration-model.md`
- `context-window-tracking.md`

The hybrid approach (short history + scratchpad + handoff) provides the benefits of stateless iteration while maintaining conversational context for recent interactions.
