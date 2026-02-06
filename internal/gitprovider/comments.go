// Package gitprovider provides issue commenting with rate limiting.
package gitprovider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// IssueCommenter posts structured comments to issues with rate limiting.
// Works with any Provider implementation (Forgejo, etc).
type IssueCommenter struct {
	provider Provider
	owner    string
	repo     string
	issueNum int

	// Rate limiting
	mu          sync.Mutex
	lastComment time.Time
	minInterval time.Duration

	// Debouncing for hat transitions
	lastHatIteration int
	hatDebounce      int // minimum iterations between hat transition comments
}

// IssueCommenterConfig configures the IssueCommenter
type IssueCommenterConfig struct {
	MinInterval time.Duration // Default: 3s
	HatDebounce int           // Default: 5 iterations
}

// DefaultIssueCommenterConfig returns the default configuration
func DefaultIssueCommenterConfig() IssueCommenterConfig {
	return IssueCommenterConfig{
		MinInterval: 3 * time.Second,
		HatDebounce: 5,
	}
}

// NewIssueCommenter creates a new IssueCommenter
func NewIssueCommenter(provider Provider, owner, repo string, issueNum int, cfg IssueCommenterConfig) *IssueCommenter {
	if cfg.MinInterval == 0 {
		cfg.MinInterval = 3 * time.Second
	}
	if cfg.HatDebounce == 0 {
		cfg.HatDebounce = 5
	}

	return &IssueCommenter{
		provider:    provider,
		owner:       owner,
		repo:        repo,
		issueNum:    issueNum,
		minInterval: cfg.MinInterval,
		hatDebounce: cfg.HatDebounce,
	}
}

// Post posts a comment to the issue with rate limiting
func (ic *IssueCommenter) Post(ctx context.Context, comment string) error {
	if ic.provider == nil {
		return nil // No provider configured, skip silently
	}

	ic.mu.Lock()
	defer ic.mu.Unlock()

	// Rate limiting
	if time.Since(ic.lastComment) < ic.minInterval {
		return nil // Skip, too soon
	}

	_, err := ic.provider.AddComment(ctx, ic.owner, ic.repo, ic.issueNum, comment)
	if err == nil {
		ic.lastComment = time.Now()
	}

	return err
}

// ShouldPostHatTransition checks if enough iterations have passed for a hat transition comment
func (ic *IssueCommenter) ShouldPostHatTransition(currentIteration int) bool {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if currentIteration-ic.lastHatIteration >= ic.hatDebounce {
		ic.lastHatIteration = currentIteration
		return true
	}
	return false
}

// CommentData holds information for building comments
type CommentData struct {
	// Session info
	SessionID   string
	Iteration   int
	TotalTokens int64
	Branch      string

	// Hat info
	Hat         string
	PreviousHat string

	// Progress
	ChecklistItems []ChecklistItemStatus
	FilesChanged   []FileChange

	// Quality gates
	QualityResult *QualityGateResult

	// Completion
	PRNumber int
	PRURL    string
	Stats    *CommitStats
}

// ChecklistItemStatus represents a checklist item with status
type ChecklistItemStatus struct {
	Description string
	Status      string // pending, done, failed, skipped
}

// FileChange represents a changed file
type FileChange struct {
	Path    string
	Summary string
}

// QualityGateResult holds quality gate results
type QualityGateResult struct {
	Passed bool
	Tests  *CheckResultSummary
	Lint   *CheckResultSummary
	Build  *CheckResultSummary
}

// CheckResultSummary is a simplified check result for comments
type CheckResultSummary struct {
	Passed   bool
	Skipped  bool
	Details  []string // Individual failure details
	Duration time.Duration
}

// CommitStats holds git commit statistics
type CommitStats struct {
	FilesChanged int
	Additions    int
	Deletions    int
}

// Hat emojis for visual distinction
var hatEmojis = map[string]string{
	"explorer": "🔍",
	"planner":  "📋",
	"designer": "📐",
	"creator":  "🎨",
	"critic":   "🔎",
	"editor":   "✨",
	"resolver": "🔧",
}

func getHatEmoji(hat string) string {
	if emoji, ok := hatEmojis[hat]; ok {
		return emoji
	}
	return "🤖"
}

// BuildStartedComment builds the "work started" comment
func BuildStartedComment(data *CommentData) string {
	var sb strings.Builder

	sb.WriteString("### 🚀 Started\n\n")
	if data.Branch != "" {
		sb.WriteString(fmt.Sprintf("**Branch:** `%s`\n", data.Branch))
	}
	sb.WriteString(fmt.Sprintf("**Phase:** %s\n", data.Hat))
	sb.WriteString("\n---\n")
	sb.WriteString("<sub>🤖 Dex</sub>")

	return sb.String()
}

// BuildHatTransitionComment builds a hat transition comment
func BuildHatTransitionComment(data *CommentData) string {
	var sb strings.Builder

	// Header with emoji
	emoji := getHatEmoji(data.Hat)
	hatTitle := data.Hat
	if len(hatTitle) > 0 {
		hatTitle = strings.ToUpper(hatTitle[:1]) + hatTitle[1:]
	}
	sb.WriteString(fmt.Sprintf("### %s %s - Iteration %d\n\n",
		emoji, hatTitle, data.Iteration))

	// Files changed this phase
	if len(data.FilesChanged) > 0 {
		sb.WriteString("**Changes this phase:**\n")
		for _, change := range data.FilesChanged[:minInt(5, len(data.FilesChanged))] {
			if change.Summary != "" {
				sb.WriteString(fmt.Sprintf("- `%s` - %s\n", change.Path, change.Summary))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s`\n", change.Path))
			}
		}
		if len(data.FilesChanged) > 5 {
			sb.WriteString(fmt.Sprintf("- ...and %d more files\n", len(data.FilesChanged)-5))
		}
		sb.WriteString("\n")
	}

	// Checklist progress
	if len(data.ChecklistItems) > 0 {
		sb.WriteString("**Progress:**\n")
		for _, item := range data.ChecklistItems {
			checkbox := "[ ]"
			switch item.Status {
			case "done":
				checkbox = "[x]"
			case "failed":
				checkbox = "[x]" // Show as checked but will have (failed) suffix
			case "skipped":
				checkbox = "[~]"
			}
			line := fmt.Sprintf("- %s %s", checkbox, item.Description)
			if item.Status == "failed" {
				line += " *(failed)*"
			} else if item.Status == "skipped" {
				line += " *(skipped)*"
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("<sub>🤖 Dex • %s tokens used</sub>", formatTokens(data.TotalTokens)))

	return sb.String()
}

// BuildQualityGateComment builds a quality gate result comment
func BuildQualityGateComment(data *CommentData) string {
	var sb strings.Builder

	if data.QualityResult == nil {
		return ""
	}

	if data.QualityResult.Passed {
		sb.WriteString("### ✅ Tests Passing\n\n")
		sb.WriteString("All quality gates passed:\n")
	} else {
		sb.WriteString("### ⚠️ Tests Failing\n\n")
		sb.WriteString("Quality gate blocked completion:\n")
	}

	formatCheck := func(name string, check *CheckResultSummary) {
		if check == nil {
			return
		}
		icon := "[ ]"
		status := ""
		if check.Passed {
			icon = "[x]"
		} else if check.Skipped {
			icon = "[~]"
			status = " (skipped)"
		}

		sb.WriteString(fmt.Sprintf("- %s %s%s\n", icon, name, status))

		// Show failure details (max 3)
		if !check.Passed && !check.Skipped && len(check.Details) > 0 {
			for i, detail := range check.Details {
				if i >= 3 {
					sb.WriteString(fmt.Sprintf("  - ...and %d more\n", len(check.Details)-3))
					break
				}
				sb.WriteString(fmt.Sprintf("  - %s\n", detail))
			}
		}
	}

	formatCheck("Build", data.QualityResult.Build)
	formatCheck("Tests", data.QualityResult.Tests)
	formatCheck("Lint", data.QualityResult.Lint)

	if !data.QualityResult.Passed {
		sb.WriteString("\nWorking on fixes...\n")
	} else {
		sb.WriteString("\nMoving to final review.\n")
	}

	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("<sub>🤖 Dex • Iteration %d</sub>", data.Iteration))

	return sb.String()
}

// BuildCompletedComment builds the task completion comment
func BuildCompletedComment(data *CommentData, summary []string) string {
	var sb strings.Builder

	sb.WriteString("### ✅ Completed\n\n")

	if data.PRURL != "" {
		sb.WriteString(fmt.Sprintf("**Pull Request:** %s\n\n", data.PRURL))
	} else if data.PRNumber > 0 {
		sb.WriteString(fmt.Sprintf("**Pull Request:** #%d\n\n", data.PRNumber))
	}

	if len(summary) > 0 {
		sb.WriteString("**Summary:**\n")
		for _, item := range summary {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
		sb.WriteString("\n")
	}

	if data.Stats != nil {
		sb.WriteString(fmt.Sprintf("**Files changed:** %d files, +%d -%d lines\n\n",
			data.Stats.FilesChanged, data.Stats.Additions, data.Stats.Deletions))
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("<sub>🤖 Dex • %d iterations • %s tokens</sub>",
		data.Iteration, formatTokens(data.TotalTokens)))

	return sb.String()
}

// formatTokens formats a token count for display
func formatTokens(tokens int64) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
