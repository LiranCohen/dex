# Security: Unicode Tag Sanitization

**Priority**: High
**Effort**: Low
**Impact**: High (security)

## Problem

Unicode includes invisible characters that can be used for prompt injection attacks:

1. **Unicode Tags (U+E0000-U+E007F)**: Invisible characters that can hide instructions
2. **Bidirectional Overrides**: Characters that reverse text display direction
3. **Zero-width characters**: Invisible spacers that can encode data

An attacker could embed malicious instructions in:
- Task descriptions
- Project hint files (.dexhints, AGENTS.md)
- Memory content
- File contents read by the agent

**Inspired by Goose's unicode tag sanitization in prompt_manager.**

## Attack Examples

### Unicode Tag Injection

```
Normal task description[U+E0001][U+E0049][U+E006E][U+E0073][U+E0074][U+E0065][U+E0061][U+E0064]...
```

The unicode tags spell out hidden instructions that render invisibly but may be processed.

### Bidirectional Override

```
This file is safe[U+202E]edoc suoicilam etucexe[U+202C]
```

Text after the override character appears reversed visually but may process normally.

## Solution Overview

Sanitize all untrusted input before including in prompts:

1. **Remove unicode tags**: Strip U+E0000-U+E007F range
2. **Remove bidi overrides**: Strip directional control characters
3. **Remove zero-width chars**: Strip invisible spacers
4. **Apply at boundaries**: Sanitize input from external sources

## Implementation

### Core Sanitizer

```go
// internal/security/sanitize.go

package security

import (
    "regexp"
    "strings"
)

// Unicode ranges to remove
var (
    // Unicode Tags block (U+E0000-U+E007F) - invisible tag characters
    unicodeTagsRegex = regexp.MustCompile(`[\x{E0000}-\x{E007F}]`)

    // Variation selectors that could hide content
    variationSelectorsRegex = regexp.MustCompile(`[\x{FE00}-\x{FE0F}\x{E0100}-\x{E01EF}]`)
)

// Bidirectional control characters
var bidiControlChars = []rune{
    '\u200E', // LEFT-TO-RIGHT MARK
    '\u200F', // RIGHT-TO-LEFT MARK
    '\u202A', // LEFT-TO-RIGHT EMBEDDING
    '\u202B', // RIGHT-TO-LEFT EMBEDDING
    '\u202C', // POP DIRECTIONAL FORMATTING
    '\u202D', // LEFT-TO-RIGHT OVERRIDE
    '\u202E', // RIGHT-TO-LEFT OVERRIDE
    '\u2066', // LEFT-TO-RIGHT ISOLATE
    '\u2067', // RIGHT-TO-LEFT ISOLATE
    '\u2068', // FIRST STRONG ISOLATE
    '\u2069', // POP DIRECTIONAL ISOLATE
}

// Zero-width and invisible characters
var zeroWidthChars = []rune{
    '\u200B', // ZERO WIDTH SPACE
    '\u200C', // ZERO WIDTH NON-JOINER
    '\u200D', // ZERO WIDTH JOINER
    '\u2060', // WORD JOINER
    '\uFEFF', // ZERO WIDTH NO-BREAK SPACE (BOM)
}

// SanitizeForPrompt removes potentially dangerous unicode from text
// that will be included in LLM prompts
func SanitizeForPrompt(input string) string {
    result := input

    // Remove unicode tags
    result = unicodeTagsRegex.ReplaceAllString(result, "")

    // Remove variation selectors
    result = variationSelectorsRegex.ReplaceAllString(result, "")

    // Remove bidi control characters
    for _, char := range bidiControlChars {
        result = strings.ReplaceAll(result, string(char), "")
    }

    // Remove zero-width characters
    for _, char := range zeroWidthChars {
        result = strings.ReplaceAll(result, string(char), "")
    }

    return result
}

// HasDangerousUnicode checks if input contains suspicious unicode
// Returns true and a description if found
func HasDangerousUnicode(input string) (bool, string) {
    if unicodeTagsRegex.MatchString(input) {
        return true, "contains unicode tag characters (U+E0000-U+E007F)"
    }

    for _, char := range bidiControlChars {
        if strings.ContainsRune(input, char) {
            return true, fmt.Sprintf("contains bidirectional control character (U+%04X)", char)
        }
    }

    return false, ""
}
```

### Sanitization Points

Apply sanitization at trust boundaries:

```go
// internal/security/boundaries.go

// SanitizationPoints defines where sanitization should occur
var SanitizationPoints = []string{
    "task.description",      // User-provided task description
    "task.title",            // User-provided task title
    "hints.content",         // Project hint files
    "memory.content",        // Stored memories
    "memory.title",          // Memory titles
    "file.read.content",     // Files read from disk
    "tool.result",           // Results from tool execution
    "checkpoint.scratchpad", // Restored scratchpad content
}
```

### Integration Points

#### Task Creation

```go
// internal/api/tasks.go

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
    var req CreateTaskRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Sanitize user input
    req.Title = security.SanitizeForPrompt(req.Title)
    req.Description = security.SanitizeForPrompt(req.Description)

    // Log if dangerous content was detected
    if dangerous, reason := security.HasDangerousUnicode(req.Title + req.Description); dangerous {
        s.logger.Warn("sanitized dangerous unicode from task",
            "task_title", req.Title,
            "reason", reason)
    }

    // ... create task
}
```

#### Hints Loading

```go
// internal/hints/loader.go

func (l *HintsLoader) loadFile(path string) (string, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return "", err
    }

    // Sanitize loaded content
    sanitized := security.SanitizeForPrompt(string(content))

    if sanitized != string(content) {
        log.Warn("sanitized dangerous unicode from hints file",
            "path", path)
    }

    return sanitized, nil
}
```

#### Memory Storage

```go
// internal/db/memory.go

func (db *DB) InsertMemory(m *Memory) error {
    // Sanitize before storage
    m.Title = security.SanitizeForPrompt(m.Title)
    m.Content = security.SanitizeForPrompt(m.Content)

    // ... insert
}
```

#### File Reading Tool

```go
// internal/tools/read_file.go

func executeReadFile(params ReadFileParams) (string, error) {
    content, err := os.ReadFile(params.Path)
    if err != nil {
        return "", err
    }

    // Sanitize file content before returning to agent
    return security.SanitizeForPrompt(string(content)), nil
}
```

#### Checkpoint Restoration

```go
// internal/session/ralph.go

func (r *RalphLoop) resumeFromCheckpoint(cp *Checkpoint) {
    // Sanitize restored scratchpad
    r.session.Scratchpad = security.SanitizeForPrompt(cp.Scratchpad)

    // Sanitize restored messages
    for i := range cp.Messages {
        cp.Messages[i].Content = security.SanitizeForPrompt(cp.Messages[i].Content)
    }

    // ...
}
```

## Testing

```go
// internal/security/sanitize_test.go

func TestSanitizeForPrompt(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "unicode tags removed",
            input:    "hello\U000E0001\U000E0049world",
            expected: "helloworld",
        },
        {
            name:     "bidi override removed",
            input:    "safe\u202Eevil\u202Ctext",
            expected: "safeeviltext",
        },
        {
            name:     "zero width removed",
            input:    "no\u200Bspace",
            expected: "nospace",
        },
        {
            name:     "normal text unchanged",
            input:    "Hello, world! This is normal text.",
            expected: "Hello, world! This is normal text.",
        },
        {
            name:     "emoji preserved",
            input:    "Hello! Test",
            expected: "Hello! Test",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := SanitizeForPrompt(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestHasDangerousUnicode(t *testing.T) {
    dangerous, _ := HasDangerousUnicode("normal text")
    assert.False(t, dangerous)

    dangerous, reason := HasDangerousUnicode("text\U000E0001with\U000E0049tags")
    assert.True(t, dangerous)
    assert.Contains(t, reason, "unicode tag")

    dangerous, reason = HasDangerousUnicode("text\u202Ewith\u202Cbidi")
    assert.True(t, dangerous)
    assert.Contains(t, reason, "bidirectional")
}
```

## Logging and Monitoring

Track sanitization events for security monitoring:

```go
// internal/security/logging.go

type SanitizationEvent struct {
    Timestamp  time.Time `json:"timestamp"`
    Source     string    `json:"source"`     // e.g., "task.description", "hints.file"
    Location   string    `json:"location"`   // e.g., file path, task ID
    Reason     string    `json:"reason"`     // What was found
    Original   string    `json:"original"`   // First 100 chars of original
    Sanitized  string    `json:"sanitized"`  // First 100 chars after
}

func LogSanitization(source, location, original, sanitized string) {
    if original == sanitized {
        return // Nothing was sanitized
    }

    dangerous, reason := HasDangerousUnicode(original)
    if !dangerous {
        reason = "unknown dangerous content"
    }

    event := SanitizationEvent{
        Timestamp: time.Now(),
        Source:    source,
        Location:  location,
        Reason:    reason,
        Original:  truncate(original, 100),
        Sanitized: truncate(sanitized, 100),
    }

    // Log to security log
    securityLog.Warn("unicode sanitization applied", event)
}
```

## Configuration

```go
type SecurityConfig struct {
    SanitizeUnicodeTags bool `json:"sanitize_unicode_tags"` // Default: true
    SanitizeBidi        bool `json:"sanitize_bidi"`         // Default: true
    SanitizeZeroWidth   bool `json:"sanitize_zero_width"`   // Default: true
    LogSanitization     bool `json:"log_sanitization"`      // Default: true
}
```

## CLI Commands

```bash
# Check a file for dangerous unicode
dex security check path/to/file.md

# Sanitize a file in place (with backup)
dex security sanitize path/to/file.md

# Scan project for dangerous unicode
dex security scan .
```

## Acceptance Criteria

- [x] Unicode tags (U+E0000-U+E007F) removed
- [x] Bidirectional control characters removed
- [x] Zero-width characters removed
- [x] Sanitization at task creation (API layer) - `server.go:handleCreateTask`, `handleCreateObjective`, `handleCreateObjectivesBatch`
- [x] Sanitization at hints loading - `hints/loader.go:Load()` calls `security.SanitizeForPrompt`
- [x] Sanitization at memory storage (API layer) - `memory_handlers.go:handleCreateMemory`, `handleUpdateMemory`
- [x] Sanitization at file reading (`internal/tools/executor.go`)
- [x] Sanitization at checkpoint restoration (`internal/session/ralph.go`)
- [ ] Logging of sanitization events
- [x] Detection function for warnings (`HasDangerousUnicode`)
- [x] Unit tests for all character types
- [ ] CLI commands for scanning/checking

## Security Considerations

1. **Defense in depth**: Sanitization is one layer; also use content security policies
2. **Logging**: Track sanitization events to detect attack attempts
3. **Updates**: Monitor for new unicode-based attacks
4. **False positives**: Some legitimate uses exist (e.g., emoji variation selectors)

## References

- [Unicode Security Considerations](https://unicode.org/reports/tr36/)
- [Trojan Source: Invisible Vulnerabilities](https://trojansource.codes/)
- [OWASP: Unicode Security](https://owasp.org/www-community/attacks/Unicode_Encoding)

## Relationship to Other Docs

| Doc | Relationship |
|-----|-------------|
| **08-project-hints** | Hints files must be sanitized |
| **02-memory-system** | Memory content must be sanitized |
| **01-context-continuity** | Scratchpad content must be sanitized on restore |
