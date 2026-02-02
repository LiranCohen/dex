package session

import (
	"strings"
	"testing"
)

func TestExtractDecisionsFromScratchpad_Empty(t *testing.T) {
	decisions := extractDecisionsFromScratchpad("")
	if len(decisions) != 0 {
		t.Errorf("Expected 0 decisions from empty scratchpad, got %d", len(decisions))
	}
}

func TestExtractDecisionsFromScratchpad_WithDecisions(t *testing.T) {
	scratchpad := `## Current Understanding
This is a Go project

## Key Decisions
- Using table-driven tests
- Adding new field to existing struct
- Keeping backwards compatibility

## Blockers
None
`
	decisions := extractDecisionsFromScratchpad(scratchpad)

	if len(decisions) != 3 {
		t.Errorf("Expected 3 decisions, got %d", len(decisions))
	}

	expected := []string{
		"Using table-driven tests",
		"Adding new field to existing struct",
		"Keeping backwards compatibility",
	}

	for i, exp := range expected {
		if i >= len(decisions) {
			t.Errorf("Missing decision %d: %s", i, exp)
			continue
		}
		if decisions[i] != exp {
			t.Errorf("Decision %d: expected %q, got %q", i, exp, decisions[i])
		}
	}
}

func TestExtractDecisionsFromScratchpad_WithAsterisks(t *testing.T) {
	scratchpad := `## Key Decisions
* First decision
* Second decision
`
	decisions := extractDecisionsFromScratchpad(scratchpad)

	if len(decisions) != 2 {
		t.Errorf("Expected 2 decisions, got %d", len(decisions))
	}
}

func TestExtractDecisionsFromScratchpad_NoDecisionSection(t *testing.T) {
	scratchpad := `## Current Understanding
This is a Go project

## Current Plan
1. Explore codebase
2. Make changes
`
	decisions := extractDecisionsFromScratchpad(scratchpad)

	if len(decisions) != 0 {
		t.Errorf("Expected 0 decisions when no decision section, got %d", len(decisions))
	}
}

func TestHandoffSummary_FormatForResume(t *testing.T) {
	handoff := &HandoffSummary{
		TaskTitle:          "Add feature X",
		CurrentHat:         "creator",
		Branch:             "feature/add-x",
		CompletedItems:     []string{"Explore codebase", "Create initial implementation"},
		RemainingItems:     []string{"Add tests", "Create PR"},
		BlockingIssues:     []string{"CI pipeline broken"},
		KeyDecisions:       []string{"Using table-driven tests"},
		ContinuationPrompt: "Continue with: Add tests",
	}

	output := handoff.FormatForResume()

	// Check all sections are present
	expectedParts := []string{
		"Resuming Session",
		"Add feature X",
		"feature/add-x",
		"2/4 items completed",
		"Add tests",
		"Create PR",
		"CI pipeline broken",
		"Using table-driven tests",
		"Continue with",
	}

	for _, part := range expectedParts {
		if !strings.Contains(output, part) {
			t.Errorf("Expected output to contain %q", part)
		}
	}
}

func TestHandoffSummary_FormatForResume_Minimal(t *testing.T) {
	handoff := &HandoffSummary{
		TaskTitle:          "Simple task",
		CurrentHat:         "explorer",
		ContinuationPrompt: "Explore the codebase",
	}

	output := handoff.FormatForResume()

	if !strings.Contains(output, "Simple task") {
		t.Error("Expected output to contain task title")
	}
	if !strings.Contains(output, "Explore the codebase") {
		t.Error("Expected output to contain continuation prompt")
	}
	// Should not have sections for empty fields
	if strings.Contains(output, "**Remaining") {
		t.Error("Should not have Remaining section when empty")
	}
	if strings.Contains(output, "**Blockers") {
		t.Error("Should not have Blockers section when empty")
	}
}

func TestHandoffSummary_FormatForAPI(t *testing.T) {
	handoff := &HandoffSummary{
		TaskTitle:      "Test task",
		CurrentHat:     "creator",
		CompletedItems: []string{"item1"},
	}

	apiFormat := handoff.FormatForAPI()

	if apiFormat["task_title"] != "Test task" {
		t.Errorf("Expected task_title to be 'Test task', got %v", apiFormat["task_title"])
	}
	if apiFormat["current_hat"] != "creator" {
		t.Errorf("Expected current_hat to be 'creator', got %v", apiFormat["current_hat"])
	}

	completedItems, ok := apiFormat["completed_items"].([]string)
	if !ok {
		t.Error("Expected completed_items to be []string")
	} else if len(completedItems) != 1 || completedItems[0] != "item1" {
		t.Errorf("Expected completed_items to be ['item1'], got %v", completedItems)
	}
}

func TestHandoffGenerator_Generate_Minimal(t *testing.T) {
	gen := NewHandoffGenerator(nil, nil)

	session := &ActiveSession{
		TaskID:   "task-123",
		Hat:      "creator",
		Scratchpad: "## Key Decisions\n- Decision 1\n",
	}

	handoff := gen.Generate(session, session.Scratchpad, "")

	if handoff.CurrentHat != "creator" {
		t.Errorf("Expected current_hat to be 'creator', got %s", handoff.CurrentHat)
	}
	if len(handoff.KeyDecisions) != 1 {
		t.Errorf("Expected 1 key decision, got %d", len(handoff.KeyDecisions))
	}
	if handoff.GeneratedAt.IsZero() {
		t.Error("Expected GeneratedAt to be set")
	}
}
