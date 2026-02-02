package content

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ChecklistStatus represents the status of a checklist item
type ChecklistStatus string

const (
	ChecklistPending    ChecklistStatus = "pending"
	ChecklistInProgress ChecklistStatus = "in_progress"
	ChecklistDone       ChecklistStatus = "done"
	ChecklistSkipped    ChecklistStatus = "skipped"
)

// ChecklistItem represents a single checklist item
type ChecklistItem struct {
	ID          string
	Description string
	Status      ChecklistStatus
	Category    string // "must_have" or "optional"
}

// TaskSpec represents the parsed content of a task specification
type TaskSpec struct {
	Title        string
	Description  string
	Context      string
	Requirements string
	CreatedAt    time.Time
	QuestID      string
	ProjectName  string
}

// FormatTaskSpec formats a task specification as markdown
func FormatTaskSpec(spec TaskSpec) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", spec.Title))

	sb.WriteString("## Description\n\n")
	if spec.Description != "" {
		sb.WriteString(spec.Description)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Context\n\n")
	if spec.ProjectName != "" {
		sb.WriteString(fmt.Sprintf("- **Project:** %s\n", spec.ProjectName))
	}
	if !spec.CreatedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("- **Created:** %s\n", spec.CreatedAt.Format("2006-01-02 15:04")))
	}
	if spec.QuestID != "" {
		sb.WriteString(fmt.Sprintf("- **Quest:** #%s\n", spec.QuestID))
	}
	sb.WriteString("\n")

	if spec.Requirements != "" {
		sb.WriteString("## Requirements\n\n")
		sb.WriteString(spec.Requirements)
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatChecklist formats checklist items as markdown
func FormatChecklist(items []ChecklistItem) string {
	var sb strings.Builder

	sb.WriteString("# Acceptance Criteria\n\n")

	// Group by category
	mustHave := filterByCategory(items, "must_have")
	optional := filterByCategory(items, "optional")

	if len(mustHave) > 0 {
		sb.WriteString("## Must Have\n\n")
		for _, item := range mustHave {
			sb.WriteString(formatChecklistItem(item))
		}
		sb.WriteString("\n")
	}

	if len(optional) > 0 {
		sb.WriteString("## Optional\n\n")
		for _, item := range optional {
			sb.WriteString(formatChecklistItem(item))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("Last updated: %s\n", time.Now().Format("2006-01-02 15:04")))

	return sb.String()
}

// formatChecklistItem formats a single checklist item
func formatChecklistItem(item ChecklistItem) string {
	checkbox := "[ ]"
	suffix := ""

	switch item.Status {
	case ChecklistDone:
		checkbox = "[x]"
	case ChecklistInProgress:
		suffix = " *(in progress)*"
	case ChecklistSkipped:
		checkbox = "[-]"
		suffix = " *(skipped)*"
	}

	return fmt.Sprintf("- %s %s%s\n", checkbox, item.Description, suffix)
}

// filterByCategory filters checklist items by category
func filterByCategory(items []ChecklistItem, category string) []ChecklistItem {
	var result []ChecklistItem
	for _, item := range items {
		if item.Category == category {
			result = append(result, item)
		}
	}
	return result
}

// ParseChecklist parses a markdown checklist into ChecklistItems
func ParseChecklist(content string) []ChecklistItem {
	var items []ChecklistItem
	lines := strings.Split(content, "\n")

	currentCategory := "must_have"
	checkboxRegex := regexp.MustCompile(`^-\s*\[([ xX\-])\]\s*(.+?)(\s*\*\([^)]+\)\*)?$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for category headers
		if strings.HasPrefix(line, "## Must Have") {
			currentCategory = "must_have"
			continue
		}
		if strings.HasPrefix(line, "## Optional") {
			currentCategory = "optional"
			continue
		}

		// Parse checkbox items
		matches := checkboxRegex.FindStringSubmatch(line)
		if matches != nil {
			status := ChecklistPending
			switch strings.ToLower(matches[1]) {
			case "x":
				status = ChecklistDone
			case "-":
				status = ChecklistSkipped
			}

			// Check for in-progress marker
			if strings.Contains(matches[0], "*(in progress)*") {
				status = ChecklistInProgress
			}

			items = append(items, ChecklistItem{
				Description: strings.TrimSpace(matches[2]),
				Status:      status,
				Category:    currentCategory,
			})
		}
	}

	return items
}

// ChecklistFromPlanningItems converts planning checklist items (string slices) to ChecklistItems
// This is used when a planning session completes and we need to write the checklist to git files
func ChecklistFromPlanningItems(mustHave, optional []string) []ChecklistItem {
	var items []ChecklistItem

	for i, desc := range mustHave {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("must-%d", i+1),
			Description: desc,
			Status:      ChecklistPending,
			Category:    "must_have",
		})
	}

	for i, desc := range optional {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("opt-%d", i+1),
			Description: desc,
			Status:      ChecklistPending,
			Category:    "optional",
		})
	}

	return items
}

// UpdateChecklistItemStatus updates a specific item's status in the checklist content
func UpdateChecklistItemStatus(content string, description string, newStatus ChecklistStatus) string {
	lines := strings.Split(content, "\n")
	var result []string

	checkboxRegex := regexp.MustCompile(`^(-\s*\[)[ xX\-](\]\s*)(.+?)(\s*\*\([^)]+\)\*)?$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		matches := checkboxRegex.FindStringSubmatch(trimmed)

		if matches != nil && strings.TrimSpace(matches[3]) == description {
			// Found the item, update it
			newCheckbox := " "
			suffix := ""

			switch newStatus {
			case ChecklistDone:
				newCheckbox = "x"
			case ChecklistSkipped:
				newCheckbox = "-"
			case ChecklistInProgress:
				suffix = " *(in progress)*"
			}

			line = fmt.Sprintf("%s%s%s%s%s", matches[1], newCheckbox, matches[2], matches[3], suffix)
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
