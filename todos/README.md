# Dex Improvement Todos

Ideas inspired by [ralph-orchestrator](https://github.com/mikeyobrien/ralph-orchestrator), [block/goose](https://github.com/block/goose), and [assistant-ui](https://github.com/assistant-ui/assistant-ui), adapted for Dex's architecture.

## Pre-Implementation Cleanup

Before starting the TODOs, address these inconsistencies in the existing codebase:

### 1. Fix `ListRuntimesTool` in `ReadWriteTools()` (`internal/tools/sets.go`)

`ListRuntimesTool()` is included in `ReadOnlyTools()` but **missing** from `ReadWriteTools()`. This means agents executing tasks (Ralph loop) can't discover available runtimes.

```go
// Add to ReadWriteTools() after the read-only tools:
ListRuntimesTool(),
```

### 2. Add Missing Termination Reasons (`internal/session/termination.go`)

The following termination reasons are referenced in TODO 03 but don't exist yet:

```go
const (
    // ... existing reasons ...
    TerminationMaxRuntime       TerminationReason = "max_runtime"        // NEW
    TerminationValidationFailure TerminationReason = "validation_failure" // NEW
    TerminationRepetitionLoop   TerminationReason = "repetition_loop"    // NEW
)
```

Also update `IsExhaustion()` to include these.

### 3. Document Quest Tools Separation

The quest-specific tools (`list_objectives`, `get_objective_details`, `cancel_objective` from `internal/quest/tools.go`) are intentionally separate from the standard tool sets. This is correct - they're only available during Quest conversations, not Ralph execution.

---

## Documents

| # | Document | Priority | Effort | Impact | Summary |
|---|----------|----------|--------|--------|---------|
| 1 | [Context & Continuity](./01-context-continuity.md) | High | Medium | High | Scratchpad, context guards, handoff summaries, progressive compaction |
| 2 | [Memory System](./02-memory-system.md) | High | Medium-High | High | Cross-session learning with smart injection, date filtering |
| 3 | [Loop Resilience](./03-loop-resilience.md) | Medium | Low-Medium | Medium-High | Max runtime, validation tracking, repetition detection, large responses |
| 4 | [Hat System](./04-hat-system.md) | Architectural | High | High | Tool profiles with annotations, event-driven coordination |
| 5 | [Issue Activity Sync](./05-issue-activity-sync.md) | Medium | Low-Medium | Medium-High | Post progress comments to linked GitHub Issues |
| 6 | [Subagent System](./06-subagent-system.md) | Medium | High | High | Isolated sub-sessions for delegated tasks |
| 7 | [Structured Output](./07-structured-output-validation.md) | Low-Medium | Low | Medium | JSON Schema validation for task outputs |
| 8 | [Project Hints](./08-project-hints-system.md) | Medium | Low | Medium | Load .dexhints, AGENTS.md from project hierarchy |
| 9 | [Unicode Sanitization](./09-security-unicode-sanitization.md) | High | Low | High | Security: remove dangerous unicode from inputs |
| 10 | [Quest Chat UI](./10-quest-chat-ui.md) | High | Medium | High | Chat components, tool visualization, decision UI |
| 11 | [Real-Time Activity System](./11-realtime-activity-system.md) | High | Medium | High | WebSocket activity feed fixes: tool pairing, performance, security |

## Recommended Implementation Order

### Pre-Phase: Codebase Cleanup

Before starting the TODOs, fix these inconsistencies (see "Pre-Implementation Cleanup" section above):

1. **Add `ListRuntimesTool()` to `ReadWriteTools()`** - 1 line fix
2. **Add missing termination reasons** - Add 3 constants + update `IsExhaustion()`

These are quick fixes that ensure the foundation is consistent.

### Phase 0: Security Foundation
Start with [09-security-unicode-sanitization.md](./09-security-unicode-sanitization.md):
- Quick win with high security impact
- Apply to all input boundaries before other features

### Phase 1: Quick Wins from Loop Resilience
Continue with the low-effort improvements in [03-loop-resilience.md](./03-loop-resilience.md):
- Max runtime termination (extends existing budget checks)
- Validation failure tracking (extends existing `loop_health.go`)
- **NEW**: Tool repetition detection (prevent infinite loops)
- **NEW**: Large response handling (write >200k to temp files)

These build directly on existing infrastructure with minimal risk.

### Phase 2: Context Continuity
Implement [01-context-continuity.md](./01-context-continuity.md):
- Scratchpad pattern for thinking persistence
- Context guards to prevent window exhaustion
- **NEW**: Progressive tool response removal (10%, 20%, 50%, 100%)
- Handoff summaries for better resume

This reduces token costs and improves long-running task reliability.

### Phase 3: Project Hints
Implement [08-project-hints-system.md](./08-project-hints-system.md):
- Load .dexhints, AGENTS.md, CLAUDE.md from project
- Directory walking from cwd to git root
- @filename imports

Low effort, immediate value for project-specific context.

### Phase 4: Tool Profiles
Implement the first half of [04-hat-system.md](./04-hat-system.md):
- Semantic tool groups
- **NEW**: Tool annotations (ReadOnlyHint, DestructiveHint)
- Hat-specific tool profiles
- Tool registry

This provides immediate safety benefits (explorer can't write, critic can't commit).

### Phase 5: Memory System
Implement [02-memory-system.md](./02-memory-system.md):
- Start with explicit MEMORY: signals
- Add relevance-scored injection
- **NEW**: Exclude current session from injection
- **NEW**: Date filtering for searches
- Later: automatic extraction at transitions

This is high impact but benefits from having scratchpad/context work done first.

### Phase 6: Backpressure Quality Gates
Complete [03-loop-resilience.md](./03-loop-resilience.md):
- Transform quality gate failures to structured events
- Provide actionable suggestions for recovery

### Phase 7: Issue Activity Sync
Implement [05-issue-activity-sync.md](./05-issue-activity-sync.md):
- Post progress comments to linked GitHub Issues
- Debounced updates on hat transitions
- Quality gate results visible externally

Low effort, high visibility improvement for stakeholders.

### Phase 8: Structured Output (Optional)
Implement [07-structured-output-validation.md](./07-structured-output-validation.md):
- JSON Schema validation for task outputs
- Useful for tasks generating configs, specs, etc.

### Phase 9: Event-Driven Coordination ✅ COMPLETED
Complete [04-hat-system.md](./04-hat-system.md):
- ✅ Event routing for hat coordination (`EVENT:topic` signals)
- ✅ Contract-driven hat registration (`HatContracts` in contracts.go)
- ✅ SQLite event persistence
- ✅ Removed legacy signals (`HAT_TRANSITION`, `TASK_COMPLETE`, `HAT_COMPLETE`)

### Phase 10: Subagent System
Implement [06-subagent-system.md](./06-subagent-system.md):
- Isolated sub-sessions for delegated tasks
- Context isolation, bounded operations
- Requires most other systems to be stable first

## Dependencies

```
┌─────────────────────┐
│ Pre-Implementation  │ ← Fix inconsistencies first
│   Cleanup           │   (see section above)
└─────────────────────┘
          │
          ▼
┌─────────────────────┐
│ 09: Unicode         │ ← Then security foundation
│   Sanitization      │
└─────────────────────┘
          │
          ▼
┌─────────────────────┐
│ 03: Loop Resilience │ ← Quick wins
│   (max runtime,     │
│    validation,      │
│    repetition,      │
│    large response)  │
└─────────────────────┘
          │
          ▼
┌─────────────────────┐
│ 01: Context         │
│   (scratchpad,      │
│    progressive      │
│    compaction)      │
└─────────────────────┘
          │
          ├──────────────────────┬──────────────────────┬──────────────────────┐
          ▼                      ▼                      ▼                      ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│ 08: Project Hints   │  │ 04: Tool Profiles   │  │ 02: Memory System   │  │ 05: Issue Sync      │
│   (low effort)      │  │   (annotations)     │  │   (date filter,     │  │                     │
└─────────────────────┘  └─────────────────────┘  │    exclude self)    │  └─────────────────────┘
                                  │                └─────────────────────┘
                                  ▼                          │
                         ┌─────────────────────┐            │
                         │ 03: Backpressure    │◄───────────┘
                         │   (quality gates)   │   (memory extraction
                         └─────────────────────┘    uses fail→pass)
                                  │
                                  ▼
                         ┌─────────────────────┐  ┌─────────────────────┐
                         │ 04: Events ✅       │  │ 07: Structured      │
                         │   (COMPLETED)       │  │   Output (optional) │
                         └─────────────────────┘  └─────────────────────┘
                                  │
                                  ▼
                         ┌─────────────────────┐
                         │ 06: Subagent        │ ← Requires stable foundation
                         │   System            │
                         └─────────────────────┘
```

## Key Design Principles

From ralph-orchestrator's philosophy:

> "The orchestrator is a thin coordination layer, not a platform. Agents are smart; let them do the work."

From block/goose's architecture:

> Metadata-based persistence, progressive degradation, and defense in depth.

Applied to Dex:

1. **Stateless-ish**: Scratchpad + short history, not full conversation replay
2. **Persistent Artifacts**: Memories, checkpoints, handoffs survive across sessions
3. **Smart Injection**: Relevance-scored context, not everything every time
4. **Quality Backpressure**: Transform failures to feedback, don't just block
5. **Scoped Capabilities**: Each hat gets exactly the tools it needs
6. **Progressive Degradation**: Try lighter approaches first (Goose: 10%→20%→50%→100% tool response removal)
7. **Metadata Persistence**: Store scratchpad/TODO in session metadata, not messages (survives compaction)
8. **Context Isolation**: Subagents get their own context, don't pollute parent
9. **Security at Boundaries**: Sanitize all external input (unicode tags, bidi overrides)

## Signal Format Reference

All agent signals use consistent format:

```
SIGNAL:content

# Examples:
SCRATCHPAD:## Current Understanding...
MEMORY:pattern:Tests use table-driven patterns
EVENT:implementation.done:{"files": "manager.go"}
```

Signals are parsed from agent responses after each iteration.

## Existing Infrastructure

These improvements build on existing Dex components:

| Component | File | What Exists | What's Missing (for TODOs) |
|-----------|------|-------------|---------------------------|
| Loop health | `internal/session/loop_health.go` | ConsecutiveFailures, TotalFailures, QualityGateAttempts, ConsecutiveBlocked, TaskBlockCounts, health status (ok/degraded/thrashing/exhausted), ShouldTerminate() | ConsecutiveValidationFailures (TODO 03), RepetitionInspector (TODO 03) |
| Quality gates | `internal/session/quality_gate.go` | GateResult/CheckResult structs, test/lint/build validation, project type detection, basic feedback strings | BlockingEvent with structured suggestions (TODO 03) |
| Checkpoints | `internal/session/ralph.go` | Periodic state saves as map[string]any JSON (iteration, tokens, hat, messages, failure context) | Scratchpad field (TODO 01), HandoffSummary (TODO 01) |
| Termination | `internal/session/termination.go` | TerminationCompleted, TerminationMaxIterations, TerminationMaxTokens, TerminationMaxCost, TerminationQualityGateExhausted, TerminationLoopThrashing, TerminationConsecutiveFailures, TerminationUserStopped, TerminationError | TerminationMaxRuntime, TerminationValidationFailure, TerminationRepetitionLoop (TODO 03) |
| Transition tracker | `internal/session/transition_tracker.go` | Loop detection for hat transitions (oscillation, repetition tracking) | Tool call repetition detection is separate (TODO 03) |
| Event system | `internal/session/event.go`, `contracts.go`, `router.go` | ✅ EVENT:topic signals, HatContracts, EventRouter, SQLite persistence | — |
| Activity log | `internal/session/activity.go` | Event recording, WebSocket broadcast | GitHub Issue comments (TODO 05) |
| Tool sets | `internal/tools/sets.go` | ReadOnlyTools(), ReadWriteTools() | Per-hat tool profiles, tool annotations (TODO 04) |
| Prompts | `internal/session/prompts.go` | ValidHats list (explorer, planner, designer, creator, critic, editor, resolver), PromptLoader | Scratchpad instructions (TODO 01), memory instructions (TODO 02) |
| ActiveSession | `internal/session/manager.go` | ID, TaskID, Hat, State, WorktreePath, IterationCount, MaxIterations, InputTokens, OutputTokens, rates, TokensBudget, DollarsBudget, **StartedAt**, LastActivity | MaxRuntime (TODO 03), Scratchpad (TODO 01) |
| Session (DB) | `internal/db/models.go` | Persistent session state, checkpoints | — |
| Project detection | `internal/tools/project_detect.go` | ProjectConfig with GetTestCommand(), GetLintCommand(), GetBuildCommand() | — |
| GitHub integration | `internal/toolbelt/github.go` | GitHubClient, CreatePR, etc. | IssueCommenter for activity sync (TODO 05) |

### Key Observations

1. **StartedAt already exists** in ActiveSession - adding MaxRuntime check is trivial (TODO 03)
2. **TransitionTracker exists for hats** but tool call repetition needs separate RepetitionInspector (TODO 03)
3. **QualityGate has GateResult** but feedback is simple strings, not structured BlockingEvent (TODO 03)
4. **Checkpoints save state** but no scratchpad or handoff summary fields (TODO 01)
5. **Tool sets are binary** (ReadOnly vs ReadWrite) - no per-hat profiles yet (TODO 04)

## Implementation Status

Summary of what exists vs what needs to be built for each TODO:

| TODO | Status | Notes |
|------|--------|-------|
| **Pre-Phase: Cleanup** | ✅ Complete | `ListRuntimesTool` added to `ReadWriteTools()`, termination reasons added |
| **Phase 0: Security** | ✅ Complete | `internal/security/sanitize.go` with unicode sanitization, integrated in executor and ralph |
| **Phase 1: Loop Resilience** | ✅ Complete | MaxRuntime, RepetitionInspector, large response handling, validation tracking |
| **Phase 2: Context Continuity** | ✅ Complete | Scratchpad, ContextGuard, progressive compaction, HandoffSummary |
| **Phase 3: Project Hints** | ✅ Complete | HintsLoader, @import processing, directory walking, sanitization |
| **Phase 4: Tool Profiles** | ✅ Complete | Tool groups, hat-to-profile mapping, GetToolsForHat(), tool registry |
| **Phase 5: Memory System** | ✅ Complete | `memories` table, MEMORY: signal parsing, relevance scoring, API endpoints, activity logging |
| **Phase 7: Issue Activity Sync** | ✅ Complete | IssueCommenter, CommentBuilder, rate limiting, debounced hat transition comments |
| **04: Hat System (Events)** | ✅ Complete | EVENT:topic signals, HatContracts, EventRouter, SQLite persistence |
| **06: Subagent System** | Pending | spawn_subagent tool, SubagentExecutor, context isolation |
| **07: Structured Output** | Pending | OutputSchema field, task_output tool, JSON Schema validation |
| **10: Quest Chat UI** | ✅ Complete | All 5 phases done - Core chat, tool activity, decision UI, objective management, polish |
| **11: Real-Time Activity** | Pending | Tool pairing fix, performance optimizations, security improvements |

### Phase 1 Completion Details

Implemented in `internal/session/` and `internal/tools/`:

- **Max Runtime** (`ralph.go:559-563`): `ErrRuntimeLimit` check in budget validation
- **RepetitionInspector** (`repetition.go`): Tracks consecutive identical tool calls, blocks after 5 repeats
- **Validation Tracking** (`loop_health.go`): `RecordValidationFailure()`, `MaxValidationFailures`
- **Large Response Handling** (`tools/large_response.go`): >200k chars written to temp files with preview
- **Termination Reasons** (`termination.go`): Added `TerminationMaxRuntime`, `TerminationValidationFailure`, `TerminationRepetitionLoop`
- **Ralph Integration**: Tool repetition check before execution (`ralph.go:322-334`), temp file cleanup on exit (`ralph.go:131-135`)

### Phase 2 Completion Details

Implemented in `internal/session/`:

- **Scratchpad** (`manager.go`, `ralph.go`): `Scratchpad` field in ActiveSession, SCRATCHPAD: signal parsing, stored in checkpoints
- **ContextGuard** (`compaction.go`): Token estimation, 80%/90% thresholds, progressive compaction
- **Progressive Compaction** (`compaction.go`): Tool response removal at 0%/10%/20%/50%/100% levels, middle-out strategy
- **HandoffSummary** (`handoff.go`): Structured checkpoint metadata with branch, checklist progress, key decisions
- **Prompt Instructions** (`prompts/components/system.yaml`): Scratchpad usage instructions for all hats

### Phase 3 Completion Details

Implemented in `internal/hints/`:

- **HintsLoader** (`loader.go`): Loads hints from directory chain (cwd to git root)
- **Multiple Filenames**: Supports `.dexhints`, `DEX.md`, `AGENTS.md`, `CLAUDE.md`
- **Global Hints**: Loads from `~/.config/dex/hints.md`
- **Import Processing**: `@filename` imports with depth limiting and security boundaries
- **Size Limits**: 50KB max total size, prevents huge hint files
- **Integration**: `ProjectHints` field in `PromptContext`, hints loaded in `ralph.go:buildPrompt()`

### Phase 4 Completion Details

Implemented in `internal/tools/` and `internal/session/`:

- **Tool Groups** (`groups.go`): 9 semantic groups (FSRead, FSWrite, GitRead, GitWrite, GitHub, Web, Runtime, Quality, Complete)
- **Tool Profiles** (`groups.go`): 5 profiles (Explorer, Planner, Creator, Critic, Editor) with Allow/Deny policies
- **Hat Mapping** (`groups.go`): `HatProfiles` maps hats to profiles, `GetToolsForHat()` returns appropriate tool set
- **Tool Registry** (`registry.go`): `GetToolByName()`, `RegisterTool()`, `ListRegisteredTools()`
- **Integration** (`session/tools.go`): `GetToolDefinitionsForHat()`, updated RalphLoop to use hat-based tools

### Phase 5 Completion Details

Implemented in `internal/db/`, `internal/session/`, and `internal/api/`:

- **Database Schema** (`sqlite.go`): `memories` table with confidence, tags, file_refs, provenance tracking
- **Memory Model** (`memory.go`): 8 memory types (architecture, dependency, decision, constraint, pattern, convention, pitfall, fix)
- **CRUD Operations** (`memory.go`): Create, Get, Update, Delete, List, Search with date filtering
- **Relevance Scoring** (`memory.go`): Hat-based, path-based, keyword-based scoring, self-reference exclusion
- **Signal Parsing** (`ralph.go`): `MEMORY:type:content` signal parsing, automatic storage
- **Prompt Injection** (`ralph.go`, `prompts.go`): `buildMemorySection()`, grouped by type for readability
- **API Endpoints** (`memory_handlers.go`): Full CRUD plus search and cleanup endpoints
- **Activity Logging** (`activity.go`): `RecordMemoryCreated()`, `ActivityTypeMemoryCreated`
- **Lifecycle** (`memory.go`): Usage tracking, confidence boost/decay, cleanup of low-value memories
- **Prompt Instructions** (`system.yaml`): Memory recording instructions for all hats

### Phase 9 Completion Details (Event-Driven Coordination)

Implemented in `internal/session/` and `internal/db/`:

- **Event Struct** (`event.go`): ID, SessionID, Topic, Payload (JSON), SourceHat, CreatedAt
- **Topic Constants** (`event.go`): 9 topics (task.started, plan.complete, design.complete, implementation.done, review.approved, review.rejected, task.blocked, resolved, task.complete)
- **ParseEvent()** (`event.go`): Parses `EVENT:topic` and `EVENT:topic:{"json"}` from agent responses
- **IsTerminalEvent()** (`event.go`): Identifies task.complete as terminal
- **HatContracts** (`contracts.go`): Pub/sub contracts for all 7 hats (explorer, planner, designer, creator, critic, editor, resolver)
- **CanPublish()** (`contracts.go`): Validates hat can publish a topic
- **GetNextHatForTopic()** (`contracts.go`): Routes topics to subscribing hats with priority
- **EventRouter** (`router.go`): Routes events, validates contracts, integrates with TransitionTracker for loop detection
- **SQLite Persistence** (`db/events.go`): `events` table with session_id, topic, payload, source_hat; CreateEvent(), ListEventsBySession(), GetEventsByTopic()
- **RalphLoop Integration** (`ralph.go`): `detectEvent()`, event routing in main loop, hat-specific continuation prompts updated
- **Prompt Updates**: All `hat_*.yaml` and `system.yaml` updated with EVENT:topic instructions

**Removed (dead code cleanup):**
- `internal/orchestrator/transitions.go` - Entire file deleted (HatTransitions map, TransitionHandler, ValidateTransition, OnHatComplete)
- `TASK_COMPLETE` / `HAT_COMPLETE` signals - Replaced by `EVENT:task.complete`
- `HAT_TRANSITION:x` signals - Replaced by `EVENT:topic`
- `transitionHandler` field in Manager - No longer needed

### Phase 7 Completion Details

Implemented in `internal/github/` and `internal/session/`:

- **IssueCommenter** (`comments.go`): Posts comments to linked GitHub Issues with rate limiting (3s minimum interval)
- **CommentBuilder** (`comments.go`): Builds formatted comments for started, hat transitions, checkpoint, completed events
- **Debouncing** (`comments.go`): Hat transition comments debounced (minimum 5 iterations between comments)
- **Comment Formats**: Well-formatted markdown with emojis, progress tracking, token counts
- **RalphLoop Integration** (`ralph.go`):
  - `initIssueCommenter()`: Initializes commenter when task has linked GitHub issue
  - `postIssueComment()`: Posts comments with error handling (failures don't break session)
  - `buildCommentData()`: Builds comment context from session state
- **Event Hooks**: Comments posted on session start, hat transitions (debounced), task completion

### Phase 10 Completion Details (Quest Chat UI)

Implemented in `frontend/src/components/QuestChat/`:

- **MarkdownContent** (`MarkdownContent.tsx`): Full markdown rendering with react-markdown, remark-gfm, syntax highlighting via react-syntax-highlighter with oneDark theme
- **CodeBlock**: Memoized code blocks with language detection, copy button (disabled while streaming), large code block warnings (>10KB)
- **ChatInput** (`ChatInput.tsx`): Auto-resize textarea (react-textarea-autosize), Enter to submit, Shift+Enter for newline, Escape to clear, auto-focus management
- **MessageBubble** (`MessageBubble.tsx`): User/assistant message display, signal stripping (OBJECTIVE_DRAFT, QUESTION), tool call display, hover actions (copy, retry), timestamps
- **MessageList** (`MessageList.tsx`): Scrollable message container, auto-scroll on new messages, scroll-to-bottom button, streaming indicator, empty state
- **ToolActivity** (`ToolActivity.tsx`): Tool-specific visualizations for 13 tools (read_file, glob, grep, web_search, etc.), running/complete/error states, collapsible output
- **QuestionPrompt** (`QuestionPrompt.tsx`): Inline decision UI with clickable options, "Other" input, answer confirmation state
- **utils.ts**: Shared utilities - parseObjectiveDrafts, parseQuestions, formatMessageContent, stripSignals

**App.tsx Integration:**
- QuestDetailPage refactored to use new QuestChat components
- MessageList replaces inline message rendering
- ChatInput replaces inline textarea
- QuestionPrompt replaces inline question buttons
- ObjectiveDraftCard kept in sidebar (existing component)

**Dependencies added:**
- `react-markdown` ^9.0.0
- `remark-gfm` ^4.0.0
- `react-syntax-highlighter` ^15.5.0
- `react-textarea-autosize` ^8.5.0
- `@types/react-syntax-highlighter` (dev)

**Cleanup:**
- Deleted `ToolCallList.tsx` (replaced by ToolActivity in MessageBubble)
- Deleted `ToolCallDisplay.tsx` (orphaned after ToolCallList removal)
- Removed duplicate helper functions from App.tsx (now in QuestChat/utils.ts)

## Document History

Documents 01-04 consolidate the original 12 documents. Documents 05-09 are new additions.

### Original Consolidation
- `01-max-runtime-termination.md` → `03-loop-resilience.md`
- `02-validation-failure-tracking.md` → `03-loop-resilience.md`
- `03-handoff-summary-checkpoint.md` → `01-context-continuity.md`
- `04-memory-system.md` → `02-memory-system.md`
- `05-scratchpad-pattern.md` → `01-context-continuity.md`
- `06-backpressure-quality-gates.md` → `03-loop-resilience.md`
- `07-pubsub-hat-coordination.md` → `04-hat-system.md`
- `08-stateless-iteration-model.md` → `01-context-continuity.md`
- `09-parallel-worktree-learning.md` → `02-memory-system.md`
- `context-window-tracking.md` → `01-context-continuity.md`
- `tool-profiles-groups.md` → `04-hat-system.md`

### Goose-Inspired Additions
Techniques from [block/goose](https://github.com/block/goose) analysis:

**Updates to existing docs:**
- `01-context-continuity.md`: Progressive tool response removal, structured compaction prompt
- `02-memory-system.md`: Session exclusion from injection, date filtering
- `03-loop-resilience.md`: Tool repetition detection, large response handling
- `04-hat-system.md`: Tool annotations (ReadOnlyHint, DestructiveHint, IdempotentHint)

**New documents:**
- `06-subagent-system.md`: Isolated sub-sessions (from Goose's subagent_tool)
- `07-structured-output-validation.md`: JSON Schema validation (from Goose's FinalOutputTool)
- `08-project-hints-system.md`: Hint file loading (from Goose's hints system)
- `09-security-unicode-sanitization.md`: Unicode sanitization (from Goose's prompt_manager)
- `10-quest-chat-ui.md`: Chat UI/UX patterns (from Goose + assistant-ui + OpenClaw research)
