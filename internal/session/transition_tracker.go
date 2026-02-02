// Package session provides session lifecycle management for Poindexter
package session

import (
	"fmt"
	"strings"
)

// TransitionTracker tracks hat transitions to detect loops
type TransitionTracker struct {
	history     []string       // Sequence of hats
	transitions map[string]int // Count per transition type (from→to)
	maxRepeats  int            // Maximum allowed repetitions of same transition
}

// NewTransitionTracker creates a new transition tracker
func NewTransitionTracker() *TransitionTracker {
	return &TransitionTracker{
		history:     make([]string, 0),
		transitions: make(map[string]int),
		maxRepeats:  3, // Default: allow same transition 3 times max
	}
}

// SetMaxRepeats configures the maximum allowed repetitions
func (t *TransitionTracker) SetMaxRepeats(max int) {
	t.maxRepeats = max
}

// RecordTransition records a hat transition and checks for loops
// Returns an error if a loop is detected
func (t *TransitionTracker) RecordTransition(from, to string) error {
	key := from + "→" + to
	t.transitions[key]++
	t.history = append(t.history, to)

	// Check for excessive repetition of same transition
	if t.transitions[key] > t.maxRepeats {
		return fmt.Errorf("hat transition loop detected: %s occurred %d times (max: %d)",
			key, t.transitions[key], t.maxRepeats)
	}

	// Check for A→B→A→B pattern
	if pattern, detected := t.detectPattern(); detected {
		return fmt.Errorf("hat transition pattern detected: %s", pattern)
	}

	return nil
}

// detectPattern checks for oscillating patterns like A→B→A→B
func (t *TransitionTracker) detectPattern() (string, bool) {
	if len(t.history) < 4 {
		return "", false
	}

	// Check last 4 items for A→B→A→B pattern
	last4 := t.history[len(t.history)-4:]
	if last4[0] == last4[2] && last4[1] == last4[3] && last4[0] != last4[1] {
		return fmt.Sprintf("%s↔%s oscillation (4+ transitions)", last4[0], last4[1]), true
	}

	// Check for A→A→A pattern (same hat 3+ times)
	if len(t.history) >= 3 {
		last3 := t.history[len(t.history)-3:]
		if last3[0] == last3[1] && last3[1] == last3[2] {
			return fmt.Sprintf("%s repeated 3+ times", last3[0]), true
		}
	}

	return "", false
}

// History returns the transition history as a formatted string
func (t *TransitionTracker) History() string {
	return strings.Join(t.history, " → ")
}

// TransitionCounts returns a copy of the transition counts
func (t *TransitionTracker) TransitionCounts() map[string]int {
	result := make(map[string]int)
	for k, v := range t.transitions {
		result[k] = v
	}
	return result
}

// Reset clears the transition history
func (t *TransitionTracker) Reset() {
	t.history = make([]string, 0)
	t.transitions = make(map[string]int)
}

// TotalTransitions returns the total number of transitions recorded
func (t *TransitionTracker) TotalTransitions() int {
	return len(t.history)
}
