# Subagent System

**Priority**: Medium
**Effort**: High
**Impact**: High

## Problem

Some tasks benefit from delegation to isolated sub-sessions:
- Parallel exploration of multiple codebases or branches
- Focused subtasks that don't need full context
- Bounded operations that shouldn't pollute parent context
- Work that benefits from different tool sets

Currently, Dex runs a single session per task with full context accumulation.

**Inspired by Goose's subagent_tool system.**

## Solution Overview

Allow the agent to spawn isolated subagents for delegated work:

1. **Context isolation**: Each subagent has its own session and context
2. **Bounded operation**: Configurable turn limits prevent runaway subagents
3. **Tool scoping**: Subagents can have different tool profiles
4. **Summary mode**: Parent receives summary, not full execution history

## Subagent Configuration

```go
// internal/session/subagent.go

type SubagentConfig struct {
    // Task definition
    Instructions string `json:"instructions"` // What the subagent should do

    // Constraints
    Hat       string `json:"hat,omitempty"`       // Hat to use (default: explorer)
    MaxTurns  int    `json:"max_turns,omitempty"` // Max iterations (default: 10)
    Timeout   time.Duration `json:"timeout,omitempty"` // Max runtime (default: 10m)

    // Context
    InheritMemories bool     `json:"inherit_memories"` // Inject parent's memory context
    Files           []string `json:"files,omitempty"`  // Files to pre-read

    // Output
    SummaryMode bool `json:"summary_mode"` // Return only final summary (default: true)
}

type SubagentResult struct {
    Success     bool              `json:"success"`
    Summary     string            `json:"summary"`      // Final response or error
    Iterations  int               `json:"iterations"`
    TokensUsed  int               `json:"tokens_used"`
    Artifacts   map[string]string `json:"artifacts,omitempty"` // Files created, etc.
    Terminated  string            `json:"terminated,omitempty"` // Reason if early termination
}
```

## Subagent Tool

Expose subagent spawning as a tool the agent can use:

```go
// internal/tools/subagent.go

var SubagentTool = Tool{
    Name: "spawn_subagent",
    Description: `Spawn an isolated subagent for delegated work.

Use this when you need to:
- Explore a separate codebase or branch without polluting your context
- Perform a bounded subtask (search, analysis, validation)
- Parallelize independent work

The subagent runs with its own context and returns a summary.
Subagents cannot spawn additional subagents.`,
    Parameters: json.RawMessage(`{
        "type": "object",
        "properties": {
            "instructions": {
                "type": "string",
                "description": "Clear, specific task for the subagent"
            },
            "hat": {
                "type": "string",
                "enum": ["explorer", "planner", "creator", "critic"],
                "description": "Hat/role for the subagent (default: explorer)"
            },
            "max_turns": {
                "type": "integer",
                "description": "Maximum iterations (default: 10, max: 25)"
            },
            "files": {
                "type": "array",
                "items": {"type": "string"},
                "description": "Files to pre-read for context"
            }
        },
        "required": ["instructions"]
    }`),
    Annotations: ToolAnnotations{
        ReadOnlyHint:    false,
        DestructiveHint: false, // Subagent might write, but contained
        IdempotentHint:  false,
    },
}
```

## Subagent Executor

Uses existing infrastructure:
- `RalphLoop` for execution
- `ActiveSession` for session state
- `toolbelt.AnthropicClient` for LLM calls
- Tool profiles from doc 04 for scoped tools

```go
// internal/session/subagent_executor.go

type SubagentExecutor struct {
    db              *db.DB
    anthropic       *toolbelt.AnthropicClient
    parentSessionID string
    parentTaskID    string // string, not int64 (matches db.Task.ID)

func (e *SubagentExecutor) Spawn(ctx context.Context, config SubagentConfig) (*SubagentResult, error) {
    // Validate config
    if config.MaxTurns <= 0 {
        config.MaxTurns = 10
    }
    if config.MaxTurns > 25 {
        config.MaxTurns = 25
    }
    if config.Hat == "" {
        config.Hat = "explorer"
    }
    if config.Timeout <= 0 {
        config.Timeout = 10 * time.Minute
    }

    // Create subagent session
    subSession := &ActiveSession{
        ID:          fmt.Sprintf("sub_%s_%s", e.parentSessionID, generateID()),
        ParentID:    e.parentSessionID,
        Hat:         config.Hat,
        IsSubagent:  true,
        StartedAt:   time.Now(),
        MaxRuntime:  config.Timeout,
    }

    // Build initial context
    initialContext := e.buildSubagentContext(config)

    // Create subagent loop (no subagent spawning allowed)
    // Uses existing RalphLoop infrastructure
    loop := NewRalphLoop(RalphLoopConfig{
        Session:          subSession,
        AnthropicClient:  e.anthropic,
        DB:               e.db,
        MaxIterations:    config.MaxTurns,
        Tools:            tools.GetToolsForHat(config.Hat), // From doc 04
        DisallowSubagent: true, // Prevent recursion
    })

    // Run with timeout
    ctx, cancel := context.WithTimeout(ctx, config.Timeout)
    defer cancel()

    err := loop.RunWithContext(ctx, initialContext)

    // Build result
    result := &SubagentResult{
        Success:    err == nil,
        Iterations: loop.Iteration(),
        TokensUsed: loop.TotalTokens(),
    }

    if config.SummaryMode {
        result.Summary = loop.GetFinalSummary()
    } else {
        result.Summary = loop.GetFullResponse()
    }

    if err != nil {
        result.Terminated = err.Error()
    }

    // Collect artifacts
    result.Artifacts = loop.CollectArtifacts()

    return result, nil
}

func (e *SubagentExecutor) buildSubagentContext(config SubagentConfig) string {
    var ctx strings.Builder

    ctx.WriteString("# Subagent Task\n\n")
    ctx.WriteString(config.Instructions)
    ctx.WriteString("\n\n")

    // Pre-read requested files
    if len(config.Files) > 0 {
        ctx.WriteString("## Pre-loaded Files\n\n")
        for _, path := range config.Files {
            content, err := os.ReadFile(path)
            if err != nil {
                ctx.WriteString(fmt.Sprintf("### %s\n(failed to read: %v)\n\n", path, err))
            } else {
                ctx.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", path, truncate(string(content), 10000)))
            }
        }
    }

    // Inject memories if requested
    if config.InheritMemories {
        memories := e.getParentMemories()
        if len(memories) > 0 {
            ctx.WriteString("## Project Knowledge\n\n")
            for _, m := range memories {
                ctx.WriteString(fmt.Sprintf("- **%s**: %s\n", m.Title, m.Content))
            }
            ctx.WriteString("\n")
        }
    }

    return ctx.String()
}
```

## Subagent System Prompt

```go
const SubagentSystemPromptAddition = `
# Subagent Context

You are a specialized subagent spawned for a specific task. Key constraints:

1. **Focus**: Complete only the task assigned to you
2. **Bounded**: You have limited iterations - be efficient
3. **No recursion**: You cannot spawn additional subagents
4. **Summary**: Your final message should summarize what you accomplished

When done, provide a clear summary of:
- What you found/accomplished
- Any important decisions made
- Files read/modified
- Recommendations for the parent agent
`
```

## Integration with RalphLoop

```go
// In RalphLoop tool execution
func (r *RalphLoop) executeToolCall(call ToolCall) (string, error) {
    if call.Name == "spawn_subagent" {
        if r.session.IsSubagent {
            return "", fmt.Errorf("subagents cannot spawn additional subagents")
        }
        return r.executeSubagent(call)
    }
    // ... other tools
}

func (r *RalphLoop) executeSubagent(call ToolCall) (string, error) {
    var config SubagentConfig
    if err := json.Unmarshal(call.ParamsJSON, &config); err != nil {
        return "", fmt.Errorf("invalid subagent config: %w", err)
    }

    executor := &SubagentExecutor{
        db:              r.db,
        llm:             r.llm,
        parentSessionID: r.session.ID,
        parentTaskID:    r.task.ID,
    }

    r.activity.Log(ActivityDebugLog, fmt.Sprintf(
        "spawning subagent: hat=%s, max_turns=%d",
        config.Hat, config.MaxTurns))

    result, err := executor.Spawn(r.ctx, config)
    if err != nil {
        return "", err
    }

    r.activity.Log(ActivityDebugLog, fmt.Sprintf(
        "subagent completed: success=%v, iterations=%d, tokens=%d",
        result.Success, result.Iterations, result.TokensUsed))

    return formatSubagentResult(result), nil
}

func formatSubagentResult(result *SubagentResult) string {
    var sb strings.Builder

    if result.Success {
        sb.WriteString("## Subagent Result (Success)\n\n")
    } else {
        sb.WriteString("## Subagent Result (Terminated)\n\n")
        if result.Terminated != "" {
            sb.WriteString(fmt.Sprintf("**Reason**: %s\n\n", result.Terminated))
        }
    }

    sb.WriteString(result.Summary)

    if len(result.Artifacts) > 0 {
        sb.WriteString("\n\n**Artifacts:**\n")
        for key, value := range result.Artifacts {
            sb.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
        }
    }

    sb.WriteString(fmt.Sprintf("\n\n*(%d iterations, %d tokens)*",
        result.Iterations, result.TokensUsed))

    return sb.String()
}
```

## Parallel Subagents

For independent work, the agent can spawn multiple subagents:

```go
// Agent makes multiple spawn_subagent calls in one response
// Each runs independently, results returned as separate tool responses
```

The orchestrator handles this naturally since tool calls are processed in order, but a future enhancement could parallelize independent subagent executions.

## Activity Logging

```go
const (
    ActivitySubagentSpawn    ActivityType = "subagent_spawn"
    ActivitySubagentComplete ActivityType = "subagent_complete"
)

// Log subagent lifecycle
r.activity.LogWithMetadata(ActivitySubagentSpawn, "spawned", map[string]string{
    "subagent_id": subSession.ID,
    "hat":         config.Hat,
    "max_turns":   strconv.Itoa(config.MaxTurns),
})

r.activity.LogWithMetadata(ActivitySubagentComplete, "completed", map[string]string{
    "subagent_id": subSession.ID,
    "success":     strconv.FormatBool(result.Success),
    "iterations":  strconv.Itoa(result.Iterations),
})
```

## Configuration

```go
type SubagentConfig struct {
    Enabled           bool          `json:"enabled"`             // Default: true
    MaxConcurrent     int           `json:"max_concurrent"`      // Default: 3
    DefaultMaxTurns   int           `json:"default_max_turns"`   // Default: 10
    MaxMaxTurns       int           `json:"max_max_turns"`       // Default: 25
    DefaultTimeout    time.Duration `json:"default_timeout"`     // Default: 10m
    AllowedHats       []string      `json:"allowed_hats"`        // Default: all except editor
}
```

Environment variables:
```bash
DEX_SUBAGENT_ENABLED=true
DEX_SUBAGENT_MAX_CONCURRENT=3
DEX_SUBAGENT_DEFAULT_MAX_TURNS=10
DEX_SUBAGENT_DEFAULT_TIMEOUT=10m
```

## Acceptance Criteria

- [ ] SubagentConfig struct defined with validation
- [ ] spawn_subagent tool available to non-subagent sessions
- [ ] Subagents run with isolated context
- [ ] Subagents cannot spawn additional subagents
- [ ] Max turns enforced
- [ ] Timeout enforced
- [ ] Hat-based tool profiles applied to subagents
- [ ] Memory inheritance optional
- [ ] File pre-reading works
- [ ] Summary mode returns condensed result
- [ ] Activity log tracks subagent lifecycle
- [ ] Parent receives formatted result
- [ ] Artifacts (created files) tracked

## Future Enhancements

- **Parallel execution**: Run multiple subagents concurrently
- **Shared scratchpad**: Allow subagents to write to a shared scratchpad
- **Subagent recipes**: Pre-defined subagent configurations for common tasks
- **Cost tracking**: Track token costs per subagent for budgeting

## Relationship to Other Docs

| Doc | Relationship |
|-----|-------------|
| **04-hat-system** | Subagents use hat-based tool profiles |
| **02-memory-system** | Subagents can inherit parent memories |
| **01-context-continuity** | Subagent isolation prevents context pollution |
