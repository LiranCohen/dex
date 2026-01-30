// Package orchestrator provides task scheduling and hat transition logic
package orchestrator

import (
	"fmt"
	"slices"

	"github.com/liranmauda/dex/internal/db"
)

// HatTransitions defines the valid transitions from each hat
// An empty slice means the hat is terminal (task completes after it)
var HatTransitions = map[string][]string{
	"planner":          {"architect", "implementer"},  // planner can spawn to architect or implementer
	"architect":        {"implementer"},               // architect → implementer
	"implementer":      {"reviewer", "tester"},        // implementer → reviewer or tester
	"reviewer":         {"implementer"},               // reviewer → implementer (if changes needed)
	"tester":           {"implementer", "debugger"},   // tester → implementer or debugger
	"debugger":         {"implementer", "tester"},     // debugger → implementer or tester
	"documenter":       {},                            // terminal: task completes
	"devops":           {},                            // terminal: task completes
	"conflict_manager": {},                            // terminal: task completes
}

// TerminalHats lists hats that mark task completion (no further transitions)
var TerminalHats = []string{"documenter", "devops", "conflict_manager"}

// TransitionHandler handles hat transitions and task completion
type TransitionHandler struct {
	db *db.DB
}

// NewTransitionHandler creates a new transition handler
func NewTransitionHandler(database *db.DB) *TransitionHandler {
	return &TransitionHandler{
		db: database,
	}
}

// ValidateTransition checks if a hat transition is allowed
func (h *TransitionHandler) ValidateTransition(fromHat, toHat string) bool {
	allowedTargets, exists := HatTransitions[fromHat]
	if !exists {
		return false
	}

	return slices.Contains(allowedTargets, toHat)
}

// IsTerminalHat checks if a hat is terminal (task completes after it)
func (h *TransitionHandler) IsTerminalHat(hat string) bool {
	return slices.Contains(TerminalHats, hat)
}

// OnHatComplete is called when a hat signals HAT_COMPLETE (not transitioning)
// It determines the appropriate next hat or marks task as complete
// Returns:
//   - nextHat: the next hat to transition to (empty if task should complete)
//   - taskComplete: true if the task should be marked as complete
//   - err: any error that occurred
func (h *TransitionHandler) OnHatComplete(taskID, currentHat string) (nextHat string, taskComplete bool, err error) {
	// If current hat is terminal, task is complete
	if h.IsTerminalHat(currentHat) {
		return "", true, nil
	}

	// For non-terminal hats that signal HAT_COMPLETE without transition:
	// - implementer completing → reviewer
	// - reviewer completing with no issues → task complete
	// - tester completing with no issues → task complete
	// - planner/architect completing → task complete (unusual but valid)

	switch currentHat {
	case "implementer":
		// After implementation, go to reviewer
		return "reviewer", false, nil

	case "reviewer":
		// Reviewer completing means code is approved, task is done
		return "", true, nil

	case "tester":
		// Tester completing means tests pass, task is done
		return "", true, nil

	case "debugger":
		// Debugger completing means bug is fixed, task is done
		return "", true, nil

	case "planner", "architect":
		// These normally transition, but if they signal complete, respect it
		return "", true, nil
	}

	// Unknown hat, mark as complete
	return "", true, nil
}

// GetAllowedTransitions returns the list of valid next hats for a given hat
func (h *TransitionHandler) GetAllowedTransitions(fromHat string) []string {
	if targets, exists := HatTransitions[fromHat]; exists {
		return targets
	}
	return nil
}

// UpdateTaskStatus updates the task status in the database
func (h *TransitionHandler) UpdateTaskStatus(taskID string, status string) error {
	if err := h.db.UpdateTaskStatus(taskID, status); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}
	return nil
}
