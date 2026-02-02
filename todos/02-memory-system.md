# Memory System for Cross-Session Learning

**Priority**: High
**Effort**: Medium-High
**Impact**: High

## Problem

Dex starts fresh every session. Valuable learnings are lost:
- Project patterns discovered during exploration
- Decisions made and their rationale
- Solutions to problems encountered
- Pitfalls discovered during critic reviews
- Project-specific conventions

This leads to repeated discovery of the same patterns, inconsistent decisions, and slower task completion over time.

When running parallel tasks, each task learns independently with no shared knowledge.

## Solution Overview

A SQLite-based memory system that:
1. Stores project learnings aligned with hat roles
2. Extracts memories automatically at key points
3. Injects relevant memories into prompts using smart scoring
4. Shares memories across parallel tasks in real-time

## Storage Design

### Why SQLite (not repo files)

Dex operates autonomously - PRs are created and merged without human review conversations. The valuable knowledge emerges from hat interactions, not GitHub activity. Therefore:

- Store in the existing Dex database (`internal/db/sqlite.go`)
- Fast indexed queries
- No pollution of user's project with `.dex/` folders
- No merge conflicts
- Easy complex queries
- Portable via CLI export/import

**Integration point:** Add new `memories` table alongside existing tables (users, projects, tasks, sessions, session_checkpoints, session_activities, quests, task_checklists, checklist_items).

### Database Schema

```sql
CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,

    -- Relevance scoring
    confidence REAL DEFAULT 0.5,
    tags TEXT,           -- JSON array: ["testing", "go"]
    file_refs TEXT,      -- JSON array: ["internal/session/*.go"]

    -- Provenance
    created_by_hat TEXT,
    created_by_task_id INTEGER,
    created_by_session_id TEXT,
    source TEXT DEFAULT 'automatic',  -- automatic, explicit, imported

    -- Lifecycle
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    use_count INTEGER DEFAULT 0,
    verified_at TIMESTAMP,

    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (created_by_task_id) REFERENCES tasks(id)
);

CREATE INDEX idx_memories_project ON memories(project_id);
CREATE INDEX idx_memories_project_type ON memories(project_id, type);
CREATE INDEX idx_memories_project_confidence ON memories(project_id, confidence DESC);
CREATE INDEX idx_memories_last_used ON memories(last_used_at DESC);
```

### Memory Types (Hat-Aligned)

| Type | Created By | Purpose | Example |
|------|-----------|---------|---------|
| `architecture` | Explorer | How code is organized | "API handlers in internal/api/, one file per resource" |
| `dependency` | Explorer | External deps and quirks | "Uses testify for assertions, not standard testing" |
| `decision` | Planner | Why something was chosen | "SQLite for simplicity; Postgres migration path exists" |
| `constraint` | Planner | Limitations, requirements | "Must support Go 1.21+, no generics in public API" |
| `pattern` | Creator | How things are done here | "Tests use table-driven pattern with t.Run" |
| `convention` | Creator/Editor | Style, naming, formatting | "Error messages lowercase, no trailing punctuation" |
| `pitfall` | Critic | Things that don't work | "session.Get() returns nil not error when not found" |
| `fix` | Creator | Solutions to recurring problems | "Nil pointer in Start(): check session.Worktree first" |

## Memory Creation

### Explicit Signals

Agents emit memory signals when they learn something worth remembering:

```
MEMORY:pattern:Tests use testify/assert, not standard library
MEMORY:pitfall:session.Worktree can be empty string if not initialized
MEMORY:decision:Using mutex over channel for simplicity in Manager
```

```go
func parseMemorySignal(text string) (*Memory, bool) {
    const prefix = "MEMORY:"
    idx := strings.Index(text, prefix)
    if idx == -1 {
        return nil, false
    }

    rest := text[idx+len(prefix):]
    parts := strings.SplitN(rest, ":", 2)
    if len(parts) != 2 {
        return nil, false
    }

    memType := strings.TrimSpace(parts[0])
    content := strings.TrimSpace(parts[1])

    // Extract title (first sentence or line)
    title := content
    if idx := strings.IndexAny(content, ".\n"); idx != -1 {
        title = content[:idx]
    }
    if len(title) > 100 {
        title = title[:100] + "..."
    }

    return &Memory{
        ID:      generateMemoryID(),
        Type:    MemoryType(memType),
        Title:   title,
        Content: content,
        Source:  SourceExplicit,
    }, true
}
```

### Automatic Extraction

Extract memories at significant events without explicit signals.

#### Extraction Points

Hook into existing activity types from `internal/session/activity.go`:
- `hat_transition` - recorded via `RecordHatTransition()`
- `quality_gate` - recorded via `RecordQualityGate()`
- `completion` - recorded via `RecordCompletion()`

```go
type ExtractionPoint struct {
    Trigger   string
    Extractor func(session *ActiveSession, activities []db.SessionActivity) []Memory
}

var extractionPoints = []ExtractionPoint{
    {
        Trigger:   "hat_transition:explorer->planner",
        Extractor: extractExplorerFindings, // architecture, dependency
    },
    {
        Trigger:   "hat_transition:planner->creator",
        Extractor: extractPlannerDecisions, // decision, constraint
    },
    {
        Trigger:   "hat_transition:critic->creator",
        Extractor: extractCriticFindings, // pitfall
    },
    {
        Trigger:   "quality_gate:passed_after_failure",
        Extractor: extractQualityGateFix, // fix, pitfall
    },
    {
        Trigger:   "task:completed",
        Extractor: extractTaskSummary, // pattern, convention
    },
}
```

#### Example: Quality Gate Fail to Pass

```go
func extractQualityGateFix(session *Session, activities []Activity) []Memory {
    memories := []Memory{}

    for i, act := range activities {
        if act.Type != ActivityQualityGate || act.Metadata["result"] != "failed" {
            continue
        }

        gate := act.Metadata["gate"]

        // Look for subsequent pass of same gate
        for j := i + 1; j < len(activities); j++ {
            if activities[j].Type == ActivityQualityGate &&
                activities[j].Metadata["gate"] == gate &&
                activities[j].Metadata["result"] == "passed" {

                // Find what changed between fail and pass
                changes := getChangesBetween(activities, i, j)

                memories = append(memories, Memory{
                    ID:         generateMemoryID(),
                    Type:       MemoryFix,
                    Title:      fmt.Sprintf("Fix for %s failure", gate),
                    Content:    fmt.Sprintf("**Problem:** %s\n\n**Fix:** %s",
                        summarizeFailure(act.Content),
                        summarizeChanges(changes)),
                    Source:     SourceAutomatic,
                    Tags:       []string{gate, "quality-gate"},
                    Confidence: 0.7, // Higher - verified by gate passing
                    CreatedByHat: session.Hat,
                })
                break
            }
        }
    }

    return memories
}
```

#### Example: Critic Findings

```go
func extractCriticFindings(session *Session, activities []Activity) []Memory {
    memories := []Memory{}

    // Find critic feedback that led to creator changes
    for _, act := range activities {
        if act.Type != ActivityAssistantResponse || act.Metadata["hat"] != "critic" {
            continue
        }

        findings := extractReviewFindings(act.Content)
        for _, finding := range findings {
            memories = append(memories, Memory{
                ID:          generateMemoryID(),
                Type:        MemoryPitfall,
                Title:       finding.Title,
                Content:     finding.Description,
                Source:      SourceAutomatic,
                CreatedByHat: "critic",
                Tags:        []string{"review", "critic-finding"},
                Confidence:  0.6,
            })
        }
    }

    return memories
}
```

## Memory Injection

### Relevance Scoring

Not all memories should be injected every time. Score by relevance:

```go
type MemoryContext struct {
    ProjectID        string
    CurrentHat       string
    CurrentSessionID string   // Exclude from search to prevent self-reference
    RelevantPaths    []string // Files being worked on
    TaskKeywords     []string // From task title/description
}

func (db *DB) GetRelevantMemories(ctx MemoryContext, limit int) ([]Memory, error) {
    // Exclude current session to prevent self-referential memory injection
    // (Inspired by Goose's ChatRecall which excludes current session from search)
    candidates, err := db.Query(`
        SELECT * FROM memories
        WHERE project_id = ?
          AND confidence > 0.3
          AND (created_by_session_id IS NULL OR created_by_session_id != ?)
        ORDER BY confidence DESC
        LIMIT 50
    `, ctx.ProjectID, ctx.CurrentSessionID)
    if err != nil {
        return nil, err
    }

    scored := []scoredMemory{}

    for _, m := range candidates {
        score := m.Confidence * 0.2 // Base score from confidence

        // Hat alignment: same or related hat gets boost
        if m.CreatedByHat == ctx.CurrentHat {
            score += 0.25
        } else if isRelatedHat(m.CreatedByHat, ctx.CurrentHat) {
            score += 0.1
        }

        // Path overlap: memories about files being touched
        for _, ref := range m.FileRefs {
            if pathOverlaps(ref, ctx.RelevantPaths) {
                score += 0.3
                break
            }
        }

        // Keyword match
        for _, tag := range m.Tags {
            for _, keyword := range ctx.TaskKeywords {
                if strings.Contains(strings.ToLower(tag), strings.ToLower(keyword)) {
                    score += 0.15
                    break
                }
            }
        }

        // Recency boost
        if m.LastUsedAt != nil {
            daysSince := time.Since(*m.LastUsedAt).Hours() / 24
            score += max(0, 0.1-daysSince*0.002)
        }

        if score > 0.25 {
            scored = append(scored, scoredMemory{Memory: m, Score: score})
        }
    }

    // Sort by score descending
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].Score > scored[j].Score
    })

    // Take top N and record usage
    result := make([]Memory, 0, limit)
    for i := 0; i < len(scored) && i < limit; i++ {
        result = append(result, scored[i].Memory)
        db.RecordMemoryUsage(scored[i].Memory.ID)
    }

    return result, nil
}

// Hat relationships for relevance scoring
var hatRelations = map[string][]string{
    "explorer": {"planner"},
    "planner":  {"creator", "explorer"},
    "creator":  {"critic", "editor", "planner"},
    "critic":   {"creator", "editor"},
    "editor":   {"creator", "critic"},
}

func isRelatedHat(memoryHat, currentHat string) bool {
    for _, related := range hatRelations[currentHat] {
        if memoryHat == related {
            return true
        }
    }
    return false
}
```

### Prompt Integration

```go
func (r *RalphLoop) buildMemorySection() string {
    ctx := MemoryContext{
        ProjectID:     r.task.ProjectID,
        CurrentHat:    r.session.Hat,
        RelevantPaths: r.getRelevantPaths(),
        TaskKeywords:  extractKeywords(r.task.Title + " " + r.task.Description),
    }

    memories, err := r.db.GetRelevantMemories(ctx, 8)
    if err != nil || len(memories) == 0 {
        return ""
    }

    var sb strings.Builder
    sb.WriteString("## Project Knowledge\n\n")
    sb.WriteString("Learnings from previous work on this project:\n\n")

    // Group by type for readability
    byType := groupByType(memories)

    typeOrder := []MemoryType{
        MemoryArchitecture, MemoryPattern, MemoryPitfall,
        MemoryDecision, MemoryFix, MemoryConvention,
    }

    for _, memType := range typeOrder {
        mems := byType[memType]
        if len(mems) == 0 {
            continue
        }

        sb.WriteString(fmt.Sprintf("### %s\n", memType.Title()))
        for _, m := range mems {
            sb.WriteString(fmt.Sprintf("- **%s**: %s\n", m.Title, m.Content))
        }
        sb.WriteString("\n")
    }

    return sb.String()
}
```

### Hat Prompt Addition

Add to hat system prompts:

```yaml
## Recording Learnings

When you discover something important about this project, record it:

MEMORY:pattern:How this codebase does something specific
MEMORY:pitfall:Something that doesn't work or is a gotcha
MEMORY:decision:Why a choice was made
MEMORY:fix:Solution to a problem

Only record genuinely useful learnings that would help future tasks.
Keep memories concise (1-2 sentences) and actionable.
```

## Memory Lifecycle

### Confidence Evolution

```go
const (
    InitialConfidenceExplicit  = 0.6
    InitialConfidenceAutomatic = 0.5
    InitialConfidenceImported  = 0.7

    UsageBoost      = 0.02
    MaxConfidence   = 0.95
    DecayPerWeek    = 0.02
    MinConfidence   = 0.1
)

func (db *DB) RecordMemoryUsage(memoryID string) error {
    return db.Exec(`
        UPDATE memories
        SET use_count = use_count + 1,
            last_used_at = CURRENT_TIMESTAMP,
            confidence = MIN(?, confidence + ?)
        WHERE id = ?
    `, MaxConfidence, UsageBoost, memoryID)
}

func (db *DB) DecayUnusedMemories() error {
    return db.Exec(`
        UPDATE memories
        SET confidence = MAX(?, confidence - ?)
        WHERE last_used_at < datetime('now', '-7 days')
          AND confidence > ?
    `, MinConfidence, DecayPerWeek, MinConfidence)
}
```

### Cleanup

Run periodically (e.g., weekly):

```go
func (db *DB) CleanupMemories() error {
    // Decay unused memories
    db.DecayUnusedMemories()

    // Remove very low confidence memories that haven't been used
    return db.Exec(`
        DELETE FROM memories
        WHERE confidence < 0.15
          AND use_count = 0
          AND created_at < datetime('now', '-30 days')
    `)
}
```

## Parallel Task Sharing

When multiple tasks run concurrently, memories should propagate between them.

### Shared Memory Notifications

```go
type MemoryStore struct {
    db        *sql.DB
    projectID string
    updates   chan Memory
    mu        sync.RWMutex
}

func (s *MemoryStore) Add(memory Memory) error {
    memory.ProjectID = s.projectID

    if err := s.db.InsertMemory(memory); err != nil {
        return err
    }

    // Notify listeners (non-blocking)
    select {
    case s.updates <- memory:
    default:
    }

    return nil
}

func (s *MemoryStore) Subscribe() <-chan Memory {
    return s.updates
}
```

### Integration with RalphLoop

```go
func (r *RalphLoop) watchMemoryUpdates(ctx context.Context) {
    updates := r.memoryStore.Subscribe()

    for {
        select {
        case <-ctx.Done():
            return
        case memory := <-updates:
            // Skip our own memories
            if memory.CreatedBySessionID == r.session.ID {
                continue
            }

            // Add to pending for next iteration
            r.pendingMemories = append(r.pendingMemories, memory)
            r.activity.Log(ActivityDebugLog,
                fmt.Sprintf("received memory from sibling task: %s", memory.Title))
        }
    }
}
```

## CLI Commands

```bash
# List memories for a project
dex memories list --project myproject
dex memories list --project myproject --type pattern --min-confidence 0.7

# Search memories
dex memories search --project myproject "session"

# Search with date filters
dex memories search --project myproject "session" --after 2024-01-01 --before 2024-06-01

# Add manually
dex memories add --project myproject --type pattern \
    --title "Error handling style" \
    --content "Always wrap errors with fmt.Errorf and %w"

# Export/import for portability
dex memories export --project myproject > memories.json
dex memories import --project myproject < memories.json

# Delete
dex memories delete mem-abc123

# Cleanup low-confidence memories
dex memories cleanup --project myproject
```

### Export Format

```json
{
  "version": 1,
  "project": "myproject",
  "exported_at": "2024-01-15T10:30:00Z",
  "memories": [
    {
      "id": "mem-abc123",
      "type": "pattern",
      "title": "Tests use table-driven pattern",
      "content": "Tests use table-driven patterns with t.Run subtests...",
      "confidence": 0.85,
      "tags": ["testing", "go"],
      "file_refs": ["internal/session/*_test.go"],
      "created_by_hat": "creator",
      "source": "automatic",
      "use_count": 5
    }
  ]
}
```

## API Endpoints

```
GET    /api/projects/{id}/memories           - List memories (filters: type, min_confidence)
POST   /api/projects/{id}/memories           - Add memory manually
GET    /api/memories/{id}                    - Get single memory
PUT    /api/memories/{id}                    - Update memory
DELETE /api/memories/{id}                    - Delete memory
GET    /api/projects/{id}/memories/search?q= - Search memories (supports after_date, before_date)
POST   /api/projects/{id}/memories/cleanup   - Run cleanup
```

### Search with Date Filters

```go
// SearchMemories supports date filtering (inspired by Goose's ChatRecall)
type MemorySearchParams struct {
    Query           string
    Limit           int
    AfterDate       *time.Time // ISO 8601
    BeforeDate      *time.Time // ISO 8601
    ExcludeSessionID string    // Prevent self-reference
}

func (db *DB) SearchMemories(projectID string, params MemorySearchParams) ([]Memory, error) {
    query := `
        SELECT * FROM memories
        WHERE project_id = ?
          AND (title LIKE ? OR content LIKE ?)
    `
    args := []interface{}{projectID, "%" + params.Query + "%", "%" + params.Query + "%"}

    if params.AfterDate != nil {
        query += " AND created_at >= ?"
        args = append(args, params.AfterDate)
    }
    if params.BeforeDate != nil {
        query += " AND created_at <= ?"
        args = append(args, params.BeforeDate)
    }
    if params.ExcludeSessionID != "" {
        query += " AND (created_by_session_id IS NULL OR created_by_session_id != ?)"
        args = append(args, params.ExcludeSessionID)
    }

    query += " ORDER BY confidence DESC, created_at DESC"

    if params.Limit > 0 {
        query += fmt.Sprintf(" LIMIT %d", params.Limit)
    }

    return db.Query(query, args...)
}
```

## Implementation Phases

### Phase 1: Foundation
- [x] Database schema and migrations
- [x] Memory model and basic CRUD in `internal/db/`
- [x] Explicit MEMORY: signal parsing in ralph.go
- [x] Basic injection (most recent N memories)
- [x] API endpoints

### Phase 2: Smart Relevance
- [x] Hat-based relevance scoring
- [x] Path-based relevance scoring
- [x] Confidence tracking and usage stats
- [x] Improved injection with scoring

### Phase 3: Automatic Extraction
- [ ] Hook into hat transitions
- [ ] Quality gate fail->pass extraction
- [ ] Critic finding extraction
- [ ] Task completion summary extraction

### Phase 4: Lifecycle & Polish
- [x] Confidence decay for unused memories
- [x] Cleanup of low-value memories
- [ ] CLI export/import commands
- [ ] Parallel task memory sharing

## Acceptance Criteria

- [x] Memories table created in database
- [x] MEMORY: signals parsed and stored
- [x] Memories injected into hat prompts
- [x] Relevance scoring considers hat, paths, confidence
- [x] Current session excluded from memory injection (prevent self-reference)
- [x] Date filtering supported in search (after_date, before_date)
- [x] Usage tracking (last_used_at, use_count)
- [x] Confidence evolution (boost on use, decay on neglect)
- [ ] Automatic extraction at hat transitions
- [ ] Automatic extraction from quality gate recovery
- [ ] CLI for list/search/add/export/import
- [x] API endpoints for CRUD
- [x] Activity log shows memory creation/usage
- [ ] Parallel tasks share memories

## Migration Notes

This consolidates and replaces:
- `04-memory-system.md`
- `09-parallel-worktree-learning.md`

The merge queue and conflict resolution from `09-parallel-worktree-learning.md` are out of scope here - they relate to git worktree management, not memory. Memory sharing is the cross-cutting concern that benefits from consolidation.
