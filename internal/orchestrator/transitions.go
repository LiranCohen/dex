// Package orchestrator provides task scheduling and hat transition logic
package orchestrator

import (
	"fmt"
	"slices"

	"github.com/lirancohen/dex/internal/db"
)

// HatTransitions defines the valid transitions from each hat
// An empty slice means the hat is terminal (task completes after it)
// These are general-purpose roles that apply to any domain
var HatTransitions = map[string][]string{
	"explorer": {"planner", "designer", "creator"}, // research → plan, design, or create
	"planner":  {"designer", "creator"},            // plan → design or create
	"designer": {"creator"},                        // design → create
	"creator":  {"critic", "editor", "resolver"},   // create → review, refine, or handle blockers
	"critic":   {"creator", "editor"},              // review → fix issues or polish
	"editor":   {"resolver"},                       // can recover if issues found via resolver
	"resolver": {"creator", "critic", "editor"},    // resolve issues → continue work, re-review, or finalize
}

// TerminalHats lists hats that mark task completion (no further transitions)
var TerminalHats = []string{"editor"}

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
	case "creator":
		// After creation, go to critic for review
		return "critic", false, nil

	case "critic":
		// Critic completing means work is approved, task is done
		return "", true, nil

	case "explorer":
		// Explorer completing means research is done, task is done
		return "", true, nil

	case "planner", "designer":
		// These normally transition, but if they signal complete, respect it
		return "", true, nil

	case "resolver":
		// Resolver completing means issues are handled, task is done
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
