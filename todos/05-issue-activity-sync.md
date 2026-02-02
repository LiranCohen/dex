# Issue Activity Sync

**Priority**: Medium
**Effort**: Low-Medium
**Impact**: Medium-High

## Problem

When Dex works on a task linked to a GitHub Issue, the Issue goes silent until a PR is created. Stakeholders watching the Issue have no visibility into:
- Whether work has started
- What phase it's in (planning, implementing, reviewing)
- What progress has been made
- Why it might be taking longer than expected

This creates a disconnect between Dex's rich internal activity log and the external GitHub visibility.

## Solution

Post structured comments to the GitHub Issue at significant points during task execution. Comments provide a digestible summary of progress without overwhelming detail.

## When to Post Comments

Not every iteration - that would be noisy and hit rate limits. Post on **significant events**:

| Event | Trigger | Why |
|-------|---------|-----|
| Work started | Session begins | Confirms task is being worked on |
| Hat transition | Hat changes | Shows phase progression |
| Quality gate result | Tests/lint/build completes | Shows validation status |
| Checkpoint | Manual or long-running pause | Explains work is paused |
| Blocked | Loop health degraded | Explains delays |
| Completed | Task done, PR created | Links to the PR |

## Comment Format

### Work Started

```markdown
### 🚀 Started

**Branch:** `dex/issue-42-add-auth`
**Approach:** Planning phase beginning

---
<sub>🤖 Dex</sub>
```

### Hat Transition (Progress Update)

```markdown
### 🎨 Creator - Iteration 12

**Changes this phase:**
- `internal/session/manager.go` - Added StartedAt tracking
- `internal/db/models.go` - Updated Session struct

**Progress:**
- [x] Add field to ActiveSession
- [x] Update database model
- [ ] Add termination check in loop
- [ ] Add tests

**Next:** Implementing termination check

---
<sub>🤖 Dex • 3,450 tokens used</sub>
```

### Quality Gate Result

```markdown
### ✅ Tests Passing

All quality gates passed:
- [x] Build
- [x] Tests (42 passed)
- [x] Lint

Moving to final review.

---
<sub>🤖 Dex • Iteration 18</sub>
```

Or on failure:

```markdown
### ⚠️ Tests Failing

Quality gate blocked completion:
- [x] Build
- [ ] Tests (2 failing)
  - `TestSessionManager_Start`: expected "running", got "created"
  - `TestRalphLoop_Checkpoint`: file not found
- [~] Lint (skipped)

Working on fixes...

---
<sub>🤖 Dex • Iteration 15</sub>
```

### Blocked/Paused

```markdown
### ⏸️ Paused

Session checkpointed after 25 iterations.

**Completed:**
- Core implementation done
- Tests passing

**Remaining:**
- Documentation updates
- Final review

**Resume:** Will continue automatically or can be manually resumed.

---
<sub>🤖 Dex • 45,000 tokens used</sub>
```

### Completed

```markdown
### ✅ Completed

**Pull Request:** #127

**Summary:**
- Added max runtime termination to session management
- Extended LoopHealth with validation failure tracking
- Added 12 new tests

**Files changed:** 5 files, +245 -12 lines

---
<sub>🤖 Dex • 28 iterations • 52,000 tokens</sub>
```

## Implementation

### Integration with Existing GitHub Infrastructure

Dex already has GitHub integration:
- `toolbelt.GitHubClient` in `internal/toolbelt/github.go`
- GitHub client fetcher in `Manager` for GitHub App installations
- Tasks can have `GitHubIssueNumber` field

### IssueCommenter Interface

```go
// internal/github/comments.go

type IssueCommenter struct {
    client    *toolbelt.GitHubClient // Use existing GitHubClient
    owner     string
    repo      string
    issueNum  int

    // Rate limiting
    lastComment time.Time
    minInterval time.Duration // Default: 3 seconds
}

func NewIssueCommenter(client *toolbelt.GitHubClient, owner, repo string, issueNum int) *IssueCommenter {
    return &IssueCommenter{
        client:      client,
        owner:       owner,
        repo:        repo,
        issueNum:    issueNum,
        minInterval: 3 * time.Second,
    }
}

func (ic *IssueCommenter) Post(ctx context.Context, comment string) error {
    // Rate limiting
    if time.Since(ic.lastComment) < ic.minInterval {
        return nil // Skip, too soon
    }

    _, _, err := ic.client.Issues.CreateComment(ctx, ic.owner, ic.repo, ic.issueNum,
        &github.IssueComment{Body: github.String(comment)})

    if err == nil {
        ic.lastComment = time.Now()
    }

    return err
}
```

### Comment Builder

Reuses concepts from HandoffSummary (see `01-context-continuity.md`):

```go
// internal/github/comment_builder.go

type CommentBuilder struct {
    session  *ActiveSession       // From manager.go
    task     *db.Task             // From db/models.go
    activity *ActivityRecorder   // From activity.go
}

func (cb *CommentBuilder) BuildHatTransitionComment(newHat string, iteration int) string {
    var sb strings.Builder

    // Header with emoji
    emoji := hatEmoji(newHat)
    sb.WriteString(fmt.Sprintf("### %s %s - Iteration %d\n\n",
        emoji, strings.Title(newHat), iteration))

    // Changes this phase (from git status)
    if changes := cb.getRecentChanges(); len(changes) > 0 {
        sb.WriteString("**Changes this phase:**\n")
        for _, change := range changes {
            sb.WriteString(fmt.Sprintf("- `%s` - %s\n", change.File, change.Summary))
        }
        sb.WriteString("\n")
    }

    // Progress from checklist
    if cb.task.Checklist != nil {
        sb.WriteString("**Progress:**\n")
        for _, item := range cb.task.Checklist.Items {
            checkbox := "[ ]"
            if item.Status == "completed" {
                checkbox = "[x]"
            }
            sb.WriteString(fmt.Sprintf("- %s %s\n", checkbox, item.Description))
        }
        sb.WriteString("\n")
    }

    // Next action
    if next := cb.getNextAction(); next != "" {
        sb.WriteString(fmt.Sprintf("**Next:** %s\n\n", next))
    }

    // Footer
    sb.WriteString("---\n")
    sb.WriteString(fmt.Sprintf("<sub>🤖 Dex • %s tokens used</sub>",
        formatTokens(cb.session.InputTokens + cb.session.OutputTokens)))

    return sb.String()
}

// Uses GateResult from quality_gate.go which has Tests, Lint, Build as *CheckResult
func (cb *CommentBuilder) BuildQualityGateComment(result *GateResult) string {
    var sb strings.Builder

    if result.Passed {
        sb.WriteString("### ✅ Tests Passing\n\n")
        sb.WriteString("All quality gates passed:\n")
    } else {
        sb.WriteString("### ⚠️ Tests Failing\n\n")
        sb.WriteString("Quality gate blocked completion:\n")
    }

    formatCheck := func(name string, check *CheckResult) {
        if check == nil {
            return
        }
        icon := "[ ]"
        if check.Passed {
            icon = "[x]"
        } else if check.Skipped {
            icon = "[~]"
        }

        line := fmt.Sprintf("- %s %s", icon, name)
        if !check.Passed && !check.Skipped && check.Output != "" {
            line += "\n"
            for _, detail := range parseFailureDetails(check.Output) {
                line += fmt.Sprintf("  - %s\n", detail)
            }
        }
        sb.WriteString(line + "\n")
    }

    formatCheck("tests", result.Tests)
    formatCheck("lint", result.Lint)
    formatCheck("build", result.Build)

    if !result.Passed {
        sb.WriteString("\nWorking on fixes...\n")
    } else {
        sb.WriteString("\nMoving to final review.\n")
    }

    sb.WriteString("\n---\n")
    sb.WriteString(fmt.Sprintf("<sub>🤖 Dex • Iteration %d</sub>", cb.session.IterationCount))

    return sb.String()
}

func (cb *CommentBuilder) BuildCompletedComment(prURL string, stats CommitStats) string {
    var sb strings.Builder

    sb.WriteString("### ✅ Completed\n\n")
    sb.WriteString(fmt.Sprintf("**Pull Request:** %s\n\n", prURL))

    sb.WriteString("**Summary:**\n")
    for _, item := range cb.getCompletedItems() {
        sb.WriteString(fmt.Sprintf("- %s\n", item))
    }
    sb.WriteString("\n")

    sb.WriteString(fmt.Sprintf("**Files changed:** %d files, +%d -%d lines\n\n",
        stats.FilesChanged, stats.Additions, stats.Deletions))

    sb.WriteString("---\n")
    sb.WriteString(fmt.Sprintf("<sub>🤖 Dex • %d iterations • %s tokens</sub>",
        cb.session.Iteration,
        formatTokens(cb.session.InputTokens + cb.session.OutputTokens)))

    return sb.String()
}

var hatEmojis = map[string]string{
    "explorer": "🔍",
    "planner":  "📋",
    "designer": "📐",
    "creator":  "🎨",
    "critic":   "🔎",
    "editor":   "✨",
    "resolver": "🔧",
}

func hatEmoji(hat string) string {
    if e, ok := hatEmojis[hat]; ok {
        return e
    }
    return "🤖"
}
```

### Integration with RalphLoop

Hook into existing activity recording. RalphLoop already has access to:
- `manager` - which has `githubClient` and `githubClientFetcher`
- `task` - which has `GitHubIssueNumber` field
- `activity` - ActivityRecorder for logging

```go
// internal/session/ralph.go

type RalphLoop struct {
    // ... existing fields

    issueCommenter *IssueCommenter // nil if no linked issue
}

func (r *RalphLoop) onHatTransition(oldHat, newHat string) {
    // Existing transition handling...

    // Post to issue if linked
    if r.issueCommenter != nil && r.shouldPostComment("hat_transition") {
        comment := r.commentBuilder.BuildHatTransitionComment(newHat, r.session.IterationCount)
        if err := r.issueCommenter.Post(r.ctx, comment); err != nil {
            r.activity.Debug(r.session.IterationCount, fmt.Sprintf("failed to post issue comment: %v", err))
        }
    }
}

func (r *RalphLoop) onQualityGateComplete(result *GateResult) {
    // Existing quality gate handling...

    // Post to issue if linked
    if r.issueCommenter != nil && r.shouldPostComment("quality_gate") {
        comment := r.commentBuilder.BuildQualityGateComment(result)
        if err := r.issueCommenter.Post(r.ctx, comment); err != nil {
            r.activity.Debug(r.session.IterationCount, fmt.Sprintf("failed to post issue comment: %v", err))
        }
    }
}

func (r *RalphLoop) onTaskComplete(prURL string) {
    // Post completion to issue
    if r.issueCommenter != nil {
        stats := r.getCommitStats()
        comment := r.commentBuilder.BuildCompletedComment(prURL, stats)
        r.issueCommenter.Post(r.ctx, comment) // Best effort
    }
}

// Debounce logic - don't post too frequently
func (r *RalphLoop) shouldPostComment(eventType string) bool {
    // Always post quality gate results and completion
    if eventType == "quality_gate" || eventType == "completed" {
        return true
    }

    // For hat transitions, post at most every 5 iterations
    if eventType == "hat_transition" {
        return r.session.IterationCount - r.lastCommentIteration >= 5
    }

    return false
}
```

### Configuration

```go
type IssueSyncConfig struct {
    Enabled           bool          `json:"enabled"`            // Default: true
    MinCommentInterval time.Duration `json:"min_comment_interval"` // Default: 3s
    PostOnHatTransition bool         `json:"post_on_hat_transition"` // Default: true
    PostOnQualityGate   bool         `json:"post_on_quality_gate"`   // Default: true
    PostOnCheckpoint    bool         `json:"post_on_checkpoint"`     // Default: false
    HatTransitionDebounce int        `json:"hat_transition_debounce"` // Default: 5 iterations
}
```

Environment variables:
```bash
DEX_ISSUE_SYNC_ENABLED=true
DEX_ISSUE_SYNC_MIN_INTERVAL=3s
```

## What NOT to Include

Keep comments digestible. Exclude:

- **Full debug logs** - Too verbose, use Activity Log link instead
- **Raw assistant responses** - Internal detail
- **Tool call parameters** - Too technical
- **Token breakdowns** - Just show total
- **Every file read** - Only show files changed
- **Failed attempts** - Only show current state

## Relationship to Other Docs

| Doc | Relationship |
|-----|-------------|
| **01-context-continuity** | Reuses HandoffSummary concepts for content generation |
| **03-loop-resilience** | Quality gate results feed into comments |
| **04-hat-system** | Hat transitions trigger comments |

The CommentBuilder can share code with HandoffSummary generation - both need:
- Git status (modified/created files)
- Checklist progress
- Current hat/phase

## Acceptance Criteria

- [x] IssueCommenter posts to linked GitHub Issue
- [x] Rate limiting prevents comment spam (min 3s interval)
- [x] Comment posted when work starts
- [x] Comment posted on significant hat transitions (debounced)
- [ ] Comment posted on quality gate pass/fail
- [x] Comment posted when task completes (with PR link)
- [x] Comments are concise and well-formatted
- [x] Failures to post don't break the session
- [x] Configuration allows disabling/tuning
- [x] Activity log records comment posting

## Future Enhancements

- **Edit instead of new comment**: Update a single "status" comment instead of posting many
- **Reactions**: Add 👀 when starting, ✅ when done
- **Link to Activity UI**: Include URL to Dex activity log for details
- **Collapse old updates**: Use `<details>` for older progress updates
