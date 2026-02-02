# Project Hints System

**Priority**: Medium
**Effort**: Low
**Impact**: Medium

## Problem

Every project has conventions, patterns, and context that the agent needs to know:
- Preferred testing frameworks
- Code style guidelines
- Architecture decisions
- Deployment processes
- Team conventions

Currently, this information must be included in task descriptions or discovered anew each session.

**Inspired by Goose's hints system (.goosehints, AGENTS.md loading).**

## Solution Overview

Load project-specific context files from the working directory hierarchy:

1. **Multiple filenames**: Support `.dexhints`, `AGENTS.md`, `CLAUDE.md`, `DEX.md`
2. **Directory walking**: Load from cwd up to git root
3. **Import support**: Allow `@filename` imports for modular hints
4. **Global hints**: User-level hints from `~/.config/dex/hints.md`

## Supported Hint Files

In priority order (later overrides earlier):

| Filename | Purpose |
|----------|---------|
| `~/.config/dex/hints.md` | Global user preferences |
| `.dexhints` | Project-specific Dex configuration |
| `DEX.md` | Project instructions for Dex |
| `AGENTS.md` | Generic agent instructions |
| `CLAUDE.md` | Claude-specific instructions |

All files are optional. Content is concatenated with section headers.

## Implementation

### HintsLoader

```go
// internal/hints/loader.go

var HintFilenames = []string{
    ".dexhints",
    "DEX.md",
    "AGENTS.md",
    "CLAUDE.md",
}

type HintsLoader struct {
    workDir string
    gitRoot string
}

func NewHintsLoader(workDir string) *HintsLoader {
    return &HintsLoader{
        workDir: workDir,
        gitRoot: findGitRoot(workDir),
    }
}

func (l *HintsLoader) Load() (string, error) {
    var sections []HintSection

    // 1. Load global hints
    globalPath := filepath.Join(os.Getenv("HOME"), ".config", "dex", "hints.md")
    if content, err := l.loadFile(globalPath); err == nil && content != "" {
        sections = append(sections, HintSection{
            Source:  "global",
            Path:    globalPath,
            Content: content,
        })
    }

    // 2. Walk from git root to workDir, loading hints
    dirsToCheck := l.getDirectoryChain()

    for _, dir := range dirsToCheck {
        for _, filename := range HintFilenames {
            path := filepath.Join(dir, filename)
            content, err := l.loadFile(path)
            if err != nil || content == "" {
                continue
            }

            // Process imports
            content, err = l.processImports(content, dir)
            if err != nil {
                return "", fmt.Errorf("error processing imports in %s: %w", path, err)
            }

            sections = append(sections, HintSection{
                Source:  filename,
                Path:    path,
                Content: content,
            })
        }
    }

    return formatHintSections(sections), nil
}

type HintSection struct {
    Source  string
    Path    string
    Content string
}

func formatHintSections(sections []HintSection) string {
    if len(sections) == 0 {
        return ""
    }

    var sb strings.Builder
    sb.WriteString("# Project Context\n\n")

    for _, section := range sections {
        sb.WriteString(fmt.Sprintf("<!-- From: %s -->\n", section.Path))
        sb.WriteString(section.Content)
        sb.WriteString("\n\n")
    }

    return sb.String()
}
```

### Directory Chain

```go
func (l *HintsLoader) getDirectoryChain() []string {
    dirs := []string{}

    current := l.workDir
    for {
        dirs = append([]string{current}, dirs...) // Prepend for root-first order

        if current == l.gitRoot {
            break
        }

        parent := filepath.Dir(current)
        if parent == current {
            break // Reached filesystem root
        }
        current = parent
    }

    return dirs
}
```

### Import Processing

Support `@filename` imports for modular hints:

```markdown
# Project Hints

@docs/architecture.md
@docs/testing-guide.md

## Additional Notes
...
```

```go
// internal/hints/imports.go

var importRegex = regexp.MustCompile(`^@([^\s]+)$`)

func (l *HintsLoader) processImports(content string, baseDir string) (string, error) {
    lines := strings.Split(content, "\n")
    var result strings.Builder

    for _, line := range lines {
        trimmed := strings.TrimSpace(line)

        if match := importRegex.FindStringSubmatch(trimmed); match != nil {
            importPath := match[1]

            // Security: prevent escaping git root
            fullPath := filepath.Join(baseDir, importPath)
            fullPath, err := filepath.Abs(fullPath)
            if err != nil {
                return "", fmt.Errorf("invalid import path: %s", importPath)
            }

            if !strings.HasPrefix(fullPath, l.gitRoot) {
                return "", fmt.Errorf("import escapes repository: %s", importPath)
            }

            imported, err := l.loadFile(fullPath)
            if err != nil {
                // Warn but don't fail on missing imports
                result.WriteString(fmt.Sprintf("<!-- Import not found: %s -->\n", importPath))
                continue
            }

            // Recursively process imports in imported file
            imported, err = l.processImports(imported, filepath.Dir(fullPath))
            if err != nil {
                return "", err
            }

            result.WriteString(fmt.Sprintf("<!-- Imported: %s -->\n", importPath))
            result.WriteString(imported)
            result.WriteString("\n")
        } else {
            result.WriteString(line)
            result.WriteString("\n")
        }
    }

    return result.String(), nil
}
```

### Security: Unicode Tag Sanitization

Hint files must be sanitized to prevent prompt injection attacks via invisible unicode characters. See [09-security-unicode-sanitization.md](./09-security-unicode-sanitization.md) for the full sanitization implementation.

The hints loader uses `security.SanitizeForPrompt()` from the security package.

### Integration with RalphLoop

```go
func (r *RalphLoop) buildSystemPrompt() string {
    var sb strings.Builder

    // Base prompt
    sb.WriteString(r.baseSystemPrompt())

    // Hat-specific instructions
    sb.WriteString(r.hatPrompt())

    // Project hints
    hints, err := r.hintsLoader.Load()
    if err != nil {
        r.activity.Log(ActivityDebugLog, fmt.Sprintf("failed to load hints: %v", err))
    } else if hints != "" {
        sanitized := security.SanitizeForPrompt(hints)
        sb.WriteString("\n\n")
        sb.WriteString(sanitized)
    }

    // Memory injection
    sb.WriteString(r.buildMemorySection())

    return sb.String()
}
```

## Example Hint Files

### `.dexhints` (Project-specific)

```markdown
# Project Configuration

## Testing
- Use table-driven tests with t.Run subtests
- Mocks go in internal/mocks/
- Integration tests tagged with //go:build integration

## Code Style
- Error messages: lowercase, no trailing punctuation
- Log levels: debug for routine, info for state changes, error for failures

## Architecture
- Handlers in internal/api/
- Business logic in internal/service/
- Database access in internal/db/
```

### `AGENTS.md` (Repository root)

```markdown
# Agent Instructions

This is a Go project using standard library conventions.

## Key Files
- cmd/dex/main.go - Entry point
- internal/ - Private packages
- api/ - OpenAPI specs

## Dependencies
- SQLite for persistence
- Chi for HTTP routing
- Testify for test assertions

## Before Committing
1. Run `go fmt ./...`
2. Run `go vet ./...`
3. Run `go test ./...`
```

### Global hints (`~/.config/dex/hints.md`)

```markdown
# Global Preferences

## Personal
- Author: Liran Cohen
- GitHub: lirancohen

## Preferences
- Prefer simple solutions over clever ones
- Add comments only when the "why" isn't obvious
- Use meaningful variable names over abbreviations
```

## CLI Commands

```bash
# Show loaded hints for current directory
dex hints show

# Validate hints (check for import errors, syntax)
dex hints validate

# Create a .dexhints template
dex hints init
```

## Configuration

```go
type HintsConfig struct {
    Enabled          bool     `json:"enabled"`            // Default: true
    MaxTotalSize     int      `json:"max_total_size"`     // Default: 50KB
    MaxImportDepth   int      `json:"max_import_depth"`   // Default: 3
    AllowedFilenames []string `json:"allowed_filenames"`  // Override defaults
    // Note: Unicode sanitization is handled by the security module (see doc 09)
}
```

Environment variables:
```bash
DEX_HINTS_ENABLED=true
DEX_HINTS_MAX_SIZE=51200
```

## Acceptance Criteria

- [x] HintsLoader scans directory chain from cwd to git root (`internal/hints/loader.go`)
- [x] Multiple filenames supported (.dexhints, DEX.md, AGENTS.md, CLAUDE.md) (`HintFilenames`)
- [x] Global hints loaded from ~/.config/dex/hints.md (`Loader.Load()`)
- [x] @filename imports work with security boundaries (`processImports()`)
- [x] Import depth limited to prevent cycles (`MaxImportDepth = 3`)
- [x] Hints sanitized via `security.SanitizeForPrompt()` (`Loader.Load()`)
- [x] Hints injected into system prompt (`ralph.go:buildPrompt()`, `prompts.go:ProjectHints`)
- [ ] CLI commands for viewing/validating hints (deferred - can add when needed)
- [x] Max size limit prevents huge hints files (`MaxTotalSize = 50KB`)
- [x] Missing files silently skipped (`loadFile()` returns empty for missing)

## Future Enhancements

- **Hint caching**: Cache parsed hints per directory
- **YAML format**: Support .dexhints.yaml for structured config
- **Schema validation**: Validate hint file structure
- **IDE integration**: VS Code extension to edit hints

## Relationship to Other Docs

| Doc | Relationship |
|-----|-------------|
| **09-security-unicode-sanitization** | Hints content must be sanitized before use |
| **02-memory-system** | Hints provide static context, memories provide learned context |
| **01-context-continuity** | Hints are part of system prompt, count toward context budget |
