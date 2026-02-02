# Hat System Evolution

**Priority**: Architectural (Longer term)
**Effort**: High
**Impact**: High

## Problem

The current hat system works but has limitations:

1. **Tight coupling**: Hat transitions are direct signals (`HAT_TRANSITION:editor`), making it hard to add new hats or compose workflows
2. **No tool scoping**: Currently only two tool sets exist (`ReadOnlyTools` for Quest, `ReadWriteTools` for Ralph) - all hats in Ralph get the same write tools
3. **Mixed responsibilities**: Orchestration logic knows about specific hats rather than working with contracts

**Current State:**
- Tools defined in `internal/tools/sets.go` with `ReadOnlyTools()` and `ReadWriteTools()` functions
- Hat transitions tracked in `internal/orchestrator/transitions.go` with valid transition map
- `TransitionTracker` in `internal/session/transition_tracker.go` detects transition loops
- Hats defined in `internal/session/prompts.go`: explorer, planner, designer, creator, critic, editor, resolver

## Solution Overview

Evolve the hat system in two complementary directions:

1. **Tool Profiles**: Scope tools to hats based on their role (explorer = read-only, creator = full access)
2. **Event-Driven Coordination**: Replace direct transitions with pub/sub events for loose coupling

These can be implemented independently but work well together.

## Part 1: Tool Profiles

### Current State

The existing tool system in `internal/tools/sets.go` has two sets:
- `ReadOnlyTools()` - for Quest chat: read_file, list_files, glob, grep, git_status, git_diff, git_log, web_search, web_fetch, list_runtimes
- `ReadWriteTools()` - for Ralph loop: read_file, list_files, glob, grep, git_status, git_diff, git_log, web_search, web_fetch, bash, write_file, git_init, git_commit, git_remote_add, git_push, github_create_repo, github_create_pr, run_tests, run_lint, run_build, task_complete

**Note:** `list_runtimes` should be added to `ReadWriteTools()` for consistency (see Pre-Implementation Cleanup in README).

### Semantic Tool Groups

Reorganize into semantic groups for finer-grained control:

```go
// internal/tools/groups.go

type ToolGroup string

const (
    GroupFSRead   ToolGroup = "fs_read"   // File system read operations
    GroupFSWrite  ToolGroup = "fs_write"  // File system write operations
    GroupGitRead  ToolGroup = "git_read"  // Git read operations
    GroupGitWrite ToolGroup = "git_write" // Git write operations
    GroupGitHub   ToolGroup = "github"    // GitHub API operations
    GroupWeb      ToolGroup = "web"       // Web search/fetch
    GroupRuntime  ToolGroup = "runtime"   // Command execution
    GroupQuality  ToolGroup = "quality"   // Tests, lint, build
    GroupComplete ToolGroup = "complete"  // Task completion signals
)

// Maps to existing tool function names in definitions.go
var ToolGroups = map[ToolGroup][]string{
    GroupFSRead: {
        "read_file",   // ReadFileTool()
        "list_files",  // ListFilesTool()
        "glob",        // GlobTool()
        "grep",        // GrepTool()
    },
    GroupFSWrite: {
        "write_file",  // WriteFileTool()
    },
    GroupGitRead: {
        "git_status",  // GitStatusTool()
        "git_diff",    // GitDiffTool()
        "git_log",     // GitLogTool()
    },
    GroupGitWrite: {
        "git_init",       // GitInitTool()
        "git_commit",     // GitCommitTool()
        "git_remote_add", // GitRemoteAddTool()
        "git_push",       // GitPushTool()
    },
    GroupGitHub: {
        "github_create_repo", // GitHubCreateRepoTool()
        "github_create_pr",   // GitHubCreatePRTool()
    },
    GroupWeb: {
        "web_search",  // WebSearchTool()
        "web_fetch",   // WebFetchTool()
    },
    GroupRuntime: {
        "bash",          // BashTool()
        "list_runtimes", // ListRuntimesTool()
    },
    GroupQuality: {
        "run_tests",  // RunTestsTool()
        "run_lint",   // RunLintTool()
        "run_build",  // RunBuildTool()
    },
    GroupComplete: {
        "task_complete", // TaskCompleteTool()
    },
}
```

### Tool Profiles with Allow/Deny

```go
type ToolProfile string

const (
    ProfileExplorer ToolProfile = "explorer"
    ProfilePlanner  ToolProfile = "planner"
    ProfileCreator  ToolProfile = "creator"
    ProfileCritic   ToolProfile = "critic"
    ProfileEditor   ToolProfile = "editor"
)

type ProfilePolicy struct {
    Allow []ToolGroup // Groups to include
    Deny  []string    // Specific tools to exclude (overrides Allow)
}

var ToolProfiles = map[ToolProfile]ProfilePolicy{
    ProfileExplorer: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupWeb},
        Deny:  []string{"write_file", "git_commit", "git_push"}, // Read-only
    },
    ProfilePlanner: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupWeb},
        Deny:  []string{"write_file"}, // Can read, not write
    },
    ProfileCreator: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupGitHub, GroupRuntime, GroupQuality},
        // Full implementation access
    },
    ProfileCritic: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupQuality},
        Deny:  []string{"write_file", "git_commit"}, // Review only
    },
    ProfileEditor: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupGitHub, GroupRuntime, GroupQuality, GroupComplete},
        // Full access including completion
    },
}
```

### Hat to Profile Mapping

Maps to hats defined in `internal/session/prompts.go`:

```go
var HatProfiles = map[string]ToolProfile{
    "explorer": ProfileExplorer,  // Research only
    "planner":  ProfilePlanner,   // Can read, not write
    "designer": ProfilePlanner,   // Same as planner (architecture)
    "creator":  ProfileCreator,   // Full implementation access
    "critic":   ProfileCritic,    // Review only (read + quality gates)
    "editor":   ProfileEditor,    // Full access including completion
    "resolver": ProfileCreator,   // Needs full access to resolve blockers
}
```

### Profile Resolution

```go
func ResolveProfileTools(profile ToolProfile) []Tool {
    policy, exists := ToolProfiles[profile]
    if !exists {
        return []Tool{} // Empty for unknown profile
    }

    // Collect tools from allowed groups
    allowed := make(map[string]bool)
    for _, group := range policy.Allow {
        for _, toolName := range ToolGroups[group] {
            allowed[toolName] = true
        }
    }

    // Remove denied tools
    for _, toolName := range policy.Deny {
        delete(allowed, toolName)
    }

    // Build tool list from registry
    tools := []Tool{}
    for toolName := range allowed {
        if tool := GetToolByName(toolName); tool != nil {
            tools = append(tools, tool)
        }
    }

    return tools
}

func GetToolsForHat(hat string) []Tool {
    profile, exists := HatProfiles[hat]
    if !exists {
        return ResolveProfileTools(ProfileExplorer) // Safe default
    }
    return ResolveProfileTools(profile)
}
```

### Tool Annotations

Add safety annotations to tools themselves for automatic policy enforcement.

**Inspired by Goose's ToolAnnotations in the MCP protocol.**

```go
// internal/tools/annotations.go

type ToolAnnotations struct {
    ReadOnlyHint    bool `json:"read_only_hint"`    // Tool doesn't modify state
    DestructiveHint bool `json:"destructive_hint"`  // Tool can cause data loss (delete, overwrite)
    IdempotentHint  bool `json:"idempotent_hint"`   // Multiple calls have same effect as one
    OpenWorldHint   bool `json:"open_world_hint"`   // Tool interacts with external systems
}

// Tool with annotations
type Tool struct {
    Name        string
    Description string
    Parameters  json.RawMessage
    Execute     func(params json.RawMessage) (string, error)
    Annotations ToolAnnotations
}

// Annotation presets for common patterns
var (
    AnnotationsReadOnly = ToolAnnotations{
        ReadOnlyHint:    true,
        DestructiveHint: false,
        IdempotentHint:  true,
    }

    AnnotationsWriteFile = ToolAnnotations{
        ReadOnlyHint:    false,
        DestructiveHint: true,  // Overwrites existing
        IdempotentHint:  true,
    }

    AnnotationsGitPush = ToolAnnotations{
        ReadOnlyHint:    false,
        DestructiveHint: false,
        IdempotentHint:  false, // Pushing twice creates different states
        OpenWorldHint:   true,
    }

    AnnotationsWebFetch = ToolAnnotations{
        ReadOnlyHint:    true,
        DestructiveHint: false,
        IdempotentHint:  false, // Response may change
        OpenWorldHint:   true,
    }
)
```

### Profile Resolution with Annotations

Profiles can use annotations for automatic filtering:

```go
type ProfilePolicy struct {
    Allow             []ToolGroup // Groups to include
    Deny              []string    // Specific tools to exclude
    RequireReadOnly   bool        // Only include tools with ReadOnlyHint=true
    DenyDestructive   bool        // Exclude tools with DestructiveHint=true
}

var ToolProfiles = map[ToolProfile]ProfilePolicy{
    ProfileExplorer: {
        Allow:           []ToolGroup{GroupFS, GroupGit, GroupWeb},
        RequireReadOnly: true, // Automatically excludes write_file, git_commit, etc.
    },
    ProfileCritic: {
        Allow:           []ToolGroup{GroupFS, GroupGit, GroupQuality},
        RequireReadOnly: true,
    },
    ProfileCreator: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupGitHub, GroupRuntime, GroupQuality},
        // Full access - no restrictions
    },
    ProfileEditor: {
        Allow: []ToolGroup{GroupFS, GroupGit, GroupGitHub, GroupRuntime, GroupQuality, GroupComplete},
        // Full access including completion
    },
}

func ResolveProfileTools(profile ToolProfile) []Tool {
    policy, exists := ToolProfiles[profile]
    if !exists {
        return []Tool{}
    }

    tools := []Tool{}
    for _, group := range policy.Allow {
        for _, toolName := range ToolGroups[group] {
            tool := GetToolByName(toolName)
            if tool == nil {
                continue
            }

            // Check deny list
            if slices.Contains(policy.Deny, toolName) {
                continue
            }

            // Check annotation requirements
            if policy.RequireReadOnly && !tool.Annotations.ReadOnlyHint {
                continue
            }
            if policy.DenyDestructive && tool.Annotations.DestructiveHint {
                continue
            }

            tools = append(tools, tool)
        }
    }

    return tools
}
```

### Tool Registry

Add a registry to look up tools by name:

```go
// internal/tools/registry.go

var toolRegistry = map[string]func() Tool{
    "read_file":          ReadFileTool,
    "write_file":         WriteFileTool,
    "list_files":         ListFilesTool,
    "glob":               GlobTool,
    "grep":               GrepTool,
    "git_status":         GitStatusTool,
    "git_diff":           GitDiffTool,
    "git_log":            GitLogTool,
    "git_init":           GitInitTool,
    "git_commit":         GitCommitTool,
    "git_remote_add":     GitRemoteAddTool,
    "git_push":           GitPushTool,
    "github_create_repo": GitHubCreateRepoTool,
    "github_create_pr":   GitHubCreatePRTool,
    "web_search":         WebSearchTool,
    "web_fetch":          WebFetchTool,
    "bash":               BashTool,
    "list_runtimes":      ListRuntimesTool,
    "run_tests":          RunTestsTool,
    "run_lint":           RunLintTool,
    "run_build":          RunBuildTool,
    "task_complete":      TaskCompleteTool,
}

func GetToolByName(name string) Tool {
    if factory, exists := toolRegistry[name]; exists {
        return factory()
    }
    return nil
}

func RegisterTool(name string, factory func() Tool) {
    toolRegistry[name] = factory
}
```

### Integration with RalphLoop

Update tool selection on hat change:

```go
func (r *RalphLoop) SetHat(hat string) {
    r.session.Hat = hat
    r.tools = tools.GetToolsForHat(hat)

    r.activity.Log(ActivityHatTransition, fmt.Sprintf(
        "switched to %s (%d tools available)",
        hat, len(r.tools),
    ))
}
```

### Benefits

1. **Safety**: Explorer/Critic can't accidentally modify files
2. **Clarity**: Each hat has exactly the tools it needs
3. **Extensibility**: Add tools to groups, not individual profiles
4. **Easy policy changes**: Modify ProfilePolicy without touching tool code

## Part 2: Event-Driven Coordination

Replace direct `HAT_TRANSITION:x` signals with a pub/sub event system.

**Existing Infrastructure:**
- `HAT_TRANSITION:editor` signals parsed in `ralph.go`
- `TransitionTracker` (`transition_tracker.go`) detects loops (oscillation, excessive repetition)
- `HatTransitions` map in `orchestrator/transitions.go` defines valid transitions
- `TransitionHandler` in `orchestrator/transitions.go` validates and executes transitions

### Event Structure

```go
type Event struct {
    ID        string            `json:"id"`
    Topic     string            `json:"topic"`     // e.g., "task.ready", "implementation.done"
    Payload   map[string]string `json:"payload"`   // Contextual data
    Source    string            `json:"source"`    // Which hat/system emitted this
    Timestamp time.Time         `json:"timestamp"`
}
```

### Hat Contracts

Define what triggers each hat and what it can emit. Aligns with valid transitions in `orchestrator/transitions.go`:

```go
type HatContract struct {
    Name       string   // Hat name
    Triggers   []string // Topics that activate this hat
    Publishes  []string // Topics this hat can emit
}

// Covers all hats from prompts.go: explorer, planner, designer, creator, critic, editor, resolver
var HatContracts = map[string]HatContract{
    "explorer": {
        Name:     "explorer",
        Triggers: []string{"task.needs_exploration"},
        Publishes: []string{"exploration.complete"}, // Can transition to: planner, designer, creator
    },
    "planner": {
        Name:     "planner",
        Triggers: []string{"task.ready", "plan.requested"},
        Publishes: []string{"plan.complete", "implementation.ready"}, // Can transition to: designer, creator
    },
    "designer": {
        Name:     "designer",
        Triggers: []string{"design.requested", "architecture.needed"},
        Publishes: []string{"design.complete"}, // Can transition to: creator
    },
    "creator": {
        Name:     "creator",
        Triggers: []string{"implementation.ready", "fix.requested"},
        Publishes: []string{"implementation.done", "review.requested"}, // Can transition to: critic, editor, resolver
    },
    "critic": {
        Name:     "critic",
        Triggers: []string{"review.requested", "implementation.done"},
        Publishes: []string{"review.approved", "review.changes_requested"}, // Can transition to: creator, editor
    },
    "editor": {
        Name:     "editor",
        Triggers: []string{"review.approved", "polish.requested"},
        Publishes: []string{"task.complete"}, // Terminal hat - can only transition to resolver
    },
    "resolver": {
        Name:     "resolver",
        Triggers: []string{"blocked", "conflict.detected"},
        Publishes: []string{"resolved"}, // Can transition to: creator, critic, editor
    },
}
```

### Event Bus

```go
type EventBus struct {
    subscriptions map[string][]string // topic pattern -> hat names
    history       []Event
    mu            sync.RWMutex
}

func NewEventBus() *EventBus {
    eb := &EventBus{
        subscriptions: make(map[string][]string),
        history:       []Event{},
    }

    // Register all hat subscriptions
    for hat, contract := range HatContracts {
        for _, topic := range contract.Triggers {
            eb.subscriptions[topic] = append(eb.subscriptions[topic], hat)
        }
    }

    return eb
}

func (eb *EventBus) Publish(event Event) {
    eb.mu.Lock()
    event.ID = generateEventID()
    event.Timestamp = time.Now()
    eb.history = append(eb.history, event)
    eb.mu.Unlock()
}

func (eb *EventBus) GetNextHat(topic string) string {
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    // Exact match first
    if hats, ok := eb.subscriptions[topic]; ok && len(hats) > 0 {
        return hats[0]
    }

    // Wildcard match (e.g., "implementation.*" matches "implementation.done")
    for pattern, hats := range eb.subscriptions {
        if matchesTopic(pattern, topic) && len(hats) > 0 {
            return hats[0]
        }
    }

    return ""
}

func matchesTopic(pattern, topic string) bool {
    if pattern == "*" {
        return true
    }
    if strings.HasSuffix(pattern, ".*") {
        prefix := strings.TrimSuffix(pattern, ".*")
        return strings.HasPrefix(topic, prefix+".")
    }
    return pattern == topic
}
```

### Event Signal Parsing

Agents emit events instead of direct transitions:

```
EVENT:implementation.done:{"files_changed": "internal/session/manager.go"}
EVENT:review.approved
```

```go
func parseEventSignal(text string) (*Event, bool) {
    const prefix = "EVENT:"
    idx := strings.Index(text, prefix)
    if idx == -1 {
        return nil, false
    }

    rest := text[idx+len(prefix):]
    // Find end of line or next signal
    if endIdx := strings.Index(rest, "\n"); endIdx != -1 {
        rest = rest[:endIdx]
    }

    parts := strings.SplitN(rest, ":", 2)
    topic := strings.TrimSpace(parts[0])

    payload := map[string]string{}
    if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
        json.Unmarshal([]byte(parts[1]), &payload)
    }

    return &Event{
        Topic:   topic,
        Payload: payload,
    }, true
}
```

### Integration with RalphLoop

```go
func (r *RalphLoop) processResponse(response *anthropic.Response) error {
    // ... existing processing ...

    // Check for event signals
    if event, ok := parseEventSignal(response.Text); ok {
        event.Source = r.session.Hat
        r.eventBus.Publish(event)

        // Log for activity
        r.activity.LogWithMetadata(ActivityHatTransition, "event_published", map[string]string{
            "topic":  event.Topic,
            "source": event.Source,
        })

        // Handle terminal events
        if event.Topic == "task.complete" {
            return r.completeTask()
        }

        // Determine next hat from event
        if nextHat := r.eventBus.GetNextHat(event.Topic); nextHat != "" {
            r.SetHat(nextHat) // This also updates tools via Part 1
        }
    }

    // ... continue processing ...
}
```

### Prompt Instructions

Add to hat prompts:

```yaml
## Coordination

When you complete your work, emit an event to signal the next phase:

EVENT:implementation.done
EVENT:review.approved
EVENT:fix.requested:{"issue": "tests failing"}

Available events you can emit:
{{range .Contract.Publishes}}
- {{.}}
{{end}}
```

### Event Flow Example

```
1. Task created
   → System publishes "task.ready"

2. Planner triggered (subscribes to task.ready)
   → Analyzes task
   → Emits "implementation.ready"

3. Creator triggered (subscribes to implementation.ready)
   → Implements feature
   → Emits "implementation.done"

4. Critic triggered (subscribes to implementation.done)
   → Reviews code
   → Emits "review.changes_requested"

5. Creator triggered (subscribes to fix.requested)
   → Fixes issues
   → Emits "implementation.done"

6. Critic triggered
   → Reviews again
   → Emits "review.approved"

7. Editor triggered (subscribes to review.approved)
   → Polishes code
   → Emits "task.complete"

8. System receives "task.complete"
   → Marks task done
```

### Backward Compatibility

Support both old and new signals during migration:

```go
func (r *RalphLoop) parseSignals(text string) {
    // Try new event format first
    if event, ok := parseEventSignal(text); ok {
        r.handleEvent(event)
        return
    }

    // Fall back to legacy HAT_TRANSITION
    if hat, ok := parseHatTransition(text); ok {
        r.SetHat(hat)
        // Emit equivalent event for logging
        r.eventBus.Publish(Event{
            Topic:  fmt.Sprintf("legacy.hat_transition.%s", hat),
            Source: r.session.Hat,
        })
    }
}
```

### Quality Gates as Events

Integrate with backpressure (from 03-loop-resilience.md):

```go
// Quality gate failure transforms the event
func (r *RalphLoop) handleCompletionEvent(event Event) error {
    blocking, err := r.qualityGate.ValidateWithBackpressure("task.complete")
    if err != nil {
        return err
    }

    if blocking != nil {
        // Transform event: task.complete -> task.blocked
        r.eventBus.Publish(Event{
            Topic:   "task.blocked",
            Source:  "quality_gate",
            Payload: map[string]string{
                "blocked_by": blocking.BlockedBy,
                "original":   event.Topic,
            },
        })

        // Inject feedback
        r.messages = append(r.messages, Message{
            Role:    "user",
            Content: blocking.Format(),
        })
        return nil
    }

    // Actually complete
    return r.completeTask()
}
```

## Configuration

### YAML-based Hat Configuration

```yaml
# config/hats.yaml
hats:
  explorer:
    profile: explorer
    triggers:
      - task.needs_exploration
    publishes:
      - exploration.complete

  planner:
    profile: planner
    triggers:
      - task.ready
      - plan.requested
    publishes:
      - plan.complete
      - implementation.ready

  creator:
    profile: creator
    triggers:
      - implementation.ready
      - fix.requested
    publishes:
      - implementation.done
      - review.requested

  critic:
    profile: critic
    triggers:
      - review.requested
      - implementation.done
    publishes:
      - review.approved
      - review.changes_requested

  editor:
    profile: editor
    triggers:
      - review.approved
      - polish.requested
    publishes:
      - task.complete
```

This makes adding new hats purely configuration-driven.

## Implementation Phases

### Phase 1: Tool Profiles (Can do first)
- [x] Define tool groups in `internal/tools/groups.go` (9 semantic groups)
- [x] Define profiles with allow/deny policies (`ProfilePolicy` with Allow, Deny, RequireReadOnly)
- [x] Create tool registry (`internal/tools/registry.go`)
- [x] Map hats to profiles (`HatProfiles` map)
- [x] Update RalphLoop to use hat-based tools (`NewRalphLoop`, `RestoreFromCheckpoint`)
- [x] Add tests for profile resolution (`groups_test.go`)

### Phase 2: Event Foundation
- [ ] Define Event struct
- [ ] Implement EventBus with subscriptions
- [ ] Parse EVENT: signals
- [ ] Log events to activity
- [ ] Backward compat with HAT_TRANSITION

### Phase 3: Full Event-Driven
- [ ] Define HatContracts in config
- [ ] Route events to hats via contracts
- [ ] Update prompts with event instructions
- [ ] Integrate quality gates as event transformers
- [ ] Remove legacy transition handling

## Acceptance Criteria

### Tool Profiles
- [x] Tools grouped by semantic purpose (9 groups: FSRead, FSWrite, GitRead, GitWrite, GitHub, Web, Runtime, Quality, Complete)
- [x] Tools use ReadOnly field for safety filtering
- [x] Profiles define allow/deny policies (`ProfilePolicy` struct)
- [x] Profiles can require ReadOnly via `RequireReadOnly` flag
- [x] Each hat maps to a profile (`HatProfiles` map)
- [x] Explorer cannot write files (via RequireReadOnly)
- [x] Critic cannot commit changes (via RequireReadOnly)
- [x] Tools update on checkpoint restore
- [x] New tools added via registry (`RegisterTool()`)
- [ ] Tool annotations (DestructiveHint, IdempotentHint, OpenWorldHint) - deferred, ReadOnly sufficient for now

### Event Coordination
- [ ] Events parsed from agent responses
- [ ] EventBus routes events to hats
- [ ] Event history tracked for debugging
- [ ] Terminal events (task.complete) handled
- [ ] Quality gate failures transform events
- [ ] Configuration-driven hat registration
- [ ] Backward compatibility with HAT_TRANSITION

## Migration Notes

This consolidates and replaces:
- `07-pubsub-hat-coordination.md`
- `tool-profiles-groups.md`

The two features are complementary:
- Tool profiles ensure hats have appropriate capabilities
- Event coordination ensures hats activate at appropriate times

Implement tool profiles first as it's lower risk and provides immediate safety benefits. Event coordination is a larger architectural change that can follow.
