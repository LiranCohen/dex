// Package session provides session lifecycle management for Poindexter
package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
)

// HandoffSummary provides structured checkpoint metadata for human review and efficient resume
type HandoffSummary struct {
	GeneratedAt time.Time `json:"generated_at"`
	TaskTitle   string    `json:"task_title"`
	CurrentHat  string    `json:"current_hat"`

	// Git context
	Branch     string `json:"branch,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`

	// Progress
	CompletedItems []string `json:"completed_items,omitempty"`
	RemainingItems []string `json:"remaining_items,omitempty"`
	BlockingIssues []string `json:"blocking_issues,omitempty"`

	// Artifacts
	ModifiedFiles []string `json:"modified_files,omitempty"`
	CreatedFiles  []string `json:"created_files,omitempty"`

	// Context
	KeyDecisions []string `json:"key_decisions,omitempty"`

	// For resume
	ContinuationPrompt string `json:"continuation_prompt"`
}

// HandoffGenerator creates handoff summaries from session state
type HandoffGenerator struct {
	db     *db.DB
	gitOps *git.Operations
}

// NewHandoffGenerator creates a new handoff generator
func NewHandoffGenerator(database *db.DB, gitOps *git.Operations) *HandoffGenerator {
	return &HandoffGenerator{
		db:     database,
		gitOps: gitOps,
	}
}

// Generate creates a handoff summary for the current session state
func (g *HandoffGenerator) Generate(session *ActiveSession, scratchpad string, worktreePath string) *HandoffSummary {
	handoff := &HandoffSummary{
		GeneratedAt: time.Now(),
		CurrentHat:  session.Hat,
	}

	// Get task title
	if g.db != nil {
		if task, err := g.db.GetTaskByID(session.TaskID); err == nil && task != nil {
			handoff.TaskTitle = task.Title
		}
	}

	// Git context
	if g.gitOps != nil && worktreePath != "" {
		if branch, err := g.gitOps.GetCurrentBranch(worktreePath); err == nil {
			handoff.Branch = branch
		}
		// Note: ModifiedFiles and CreatedFiles would need git status parsing
		// For now we rely on the scratchpad to track file changes
	}

	// Extract progress from checklist
	if g.db != nil {
		if checklist, err := g.db.GetChecklistByTaskID(session.TaskID); err == nil && checklist != nil {
			if items, err := g.db.GetChecklistItems(checklist.ID); err == nil {
				for _, item := range items {
					switch item.Status {
					case db.ChecklistItemStatusDone:
						handoff.CompletedItems = append(handoff.CompletedItems, item.Description)
					case db.ChecklistItemStatusFailed:
						handoff.BlockingIssues = append(handoff.BlockingIssues, item.Description)
					default:
						handoff.RemainingItems = append(handoff.RemainingItems, item.Description)
					}
				}
			}
		}
	}

	// Extract decisions from scratchpad
	handoff.KeyDecisions = extractDecisionsFromScratchpad(scratchpad)

	// Generate continuation prompt
	nextStep := "Complete the task"
	if len(handoff.RemainingItems) > 0 {
		nextStep = handoff.RemainingItems[0]
	}
	handoff.ContinuationPrompt = fmt.Sprintf(
		"Continue working on: %s\nCurrent phase: %s\nNext step: %s",
		handoff.TaskTitle,
		session.Hat,
		nextStep,
	)

	return handoff
}

// extractDecisionsFromScratchpad parses decisions from the scratchpad content
func extractDecisionsFromScratchpad(scratchpad string) []string {
	if scratchpad == "" {
		return nil
	}

	var decisions []string

	// Look for "## Key Decisions" section
	lines := strings.Split(scratchpad, "\n")
	inDecisions := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for section headers
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
			inDecisions = strings.Contains(strings.ToLower(trimmed), "decision")
			continue
		}

		// Extract bullet points from decisions section
		if inDecisions && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
			decision := strings.TrimPrefix(trimmed, "- ")
			decision = strings.TrimPrefix(decision, "* ")
			if decision != "" {
				decisions = append(decisions, decision)
			}
		}
	}

	return decisions
}

// FormatForResume creates a formatted message for session resumption
func (h *HandoffSummary) FormatForResume() string {
	var sb strings.Builder

	sb.WriteString("## Resuming Session\n\n")
	sb.WriteString(fmt.Sprintf("**Task**: %s\n", h.TaskTitle))

	if h.Branch != "" {
		sb.WriteString(fmt.Sprintf("**Branch**: %s\n", h.Branch))
	}

	totalItems := len(h.CompletedItems) + len(h.RemainingItems)
	if totalItems > 0 {
		sb.WriteString(fmt.Sprintf("**Progress**: %d/%d items completed\n\n",
			len(h.CompletedItems), totalItems))
	}

	if len(h.RemainingItems) > 0 {
		sb.WriteString("**Remaining**:\n")
		for _, item := range h.RemainingItems {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
		sb.WriteString("\n")
	}

	if len(h.BlockingIssues) > 0 {
		sb.WriteString("**Blockers**:\n")
		for _, issue := range h.BlockingIssues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		sb.WriteString("\n")
	}

	if len(h.KeyDecisions) > 0 {
		sb.WriteString("**Key Decisions**:\n")
		for _, decision := range h.KeyDecisions {
			sb.WriteString(fmt.Sprintf("- %s\n", decision))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Continue with**: %s\n", h.ContinuationPrompt))

	return sb.String()
}

// FormatForAPI creates a JSON-friendly structure for the API
func (h *HandoffSummary) FormatForAPI() map[string]any {
	return map[string]any{
		"generated_at":        h.GeneratedAt,
		"task_title":          h.TaskTitle,
		"current_hat":         h.CurrentHat,
		"branch":              h.Branch,
		"head_commit":         h.HeadCommit,
		"completed_items":     h.CompletedItems,
		"remaining_items":     h.RemainingItems,
		"blocking_issues":     h.BlockingIssues,
		"modified_files":      h.ModifiedFiles,
		"created_files":       h.CreatedFiles,
		"key_decisions":       h.KeyDecisions,
		"continuation_prompt": h.ContinuationPrompt,
	}
}
