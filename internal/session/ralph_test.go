package session

import (
	"testing"
	"time"
)

func TestNewRalphLoop(t *testing.T) {
	// Create minimal ActiveSession
	session := &ActiveSession{
		ID:            "test-session-id",
		TaskID:        "test-task-id",
		Hat:           "creator",
		State:         StateCreated,
		WorktreePath:  "/tmp/test-worktree",
		MaxIterations: 10,
	}

	// Test with nil dependencies (loop should still create)
	loop := NewRalphLoop(nil, session, nil, nil, nil)

	if loop == nil {
		t.Fatal("NewRalphLoop returned nil")
	}

	if loop.session != session {
		t.Error("session not set correctly")
	}

	if loop.checkpointInterval != 5 {
		t.Errorf("expected checkpointInterval=5, got %d", loop.checkpointInterval)
	}

	if len(loop.messages) != 0 {
		t.Errorf("expected empty messages slice, got %d messages", len(loop.messages))
	}
}

func TestCheckBudget_IterationLimit(t *testing.T) {
	session := &ActiveSession{
		IterationCount: 10,
		MaxIterations:  10,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != ErrIterationLimit {
		t.Errorf("expected ErrIterationLimit, got %v", err)
	}
}

func TestCheckBudget_IterationBelowLimit(t *testing.T) {
	session := &ActiveSession{
		IterationCount: 5,
		MaxIterations:  10,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckBudget_TokenBudget(t *testing.T) {
	tokenBudget := int64(1000)
	session := &ActiveSession{
		InputTokens:   600,
		OutputTokens:  400, // Total: 1000, at budget
		TokensBudget:  &tokenBudget,
		MaxIterations: 100,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != ErrTokenBudget {
		t.Errorf("expected ErrTokenBudget, got %v", err)
	}
}

func TestCheckBudget_TokenBelowBudget(t *testing.T) {
	tokenBudget := int64(1000)
	session := &ActiveSession{
		InputTokens:   300,
		OutputTokens:  200, // Total: 500, below budget
		TokensBudget:  &tokenBudget,
		MaxIterations: 100,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckBudget_DollarBudget(t *testing.T) {
	dollarBudget := 5.0
	session := &ActiveSession{
		// At $3/MTok input, $15/MTok output (Sonnet rates):
		// Need cost of $5: e.g., 1M input ($3) + 133K output ($2)
		InputTokens:   1000000,
		OutputTokens:  133334,
		InputRate:     3.0,
		OutputRate:    15.0,
		DollarsBudget: &dollarBudget,
		MaxIterations: 100,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != ErrDollarBudget {
		t.Errorf("expected ErrDollarBudget, got %v", err)
	}
}

func TestCheckBudget_DollarBelowBudget(t *testing.T) {
	dollarBudget := 5.0
	session := &ActiveSession{
		// At $3/MTok input, $15/MTok output: 500K input = $1.5, 66K output = $1
		InputTokens:   500000,
		OutputTokens:  66666,
		InputRate:     3.0,
		OutputRate:    15.0,
		DollarsBudget: &dollarBudget,
		MaxIterations: 100,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckBudget_NoBudgetsSet(t *testing.T) {
	session := &ActiveSession{
		IterationCount: 50,
		MaxIterations:  0, // 0 means no limit
		InputTokens:    500000,
		OutputTokens:   499999,
		InputRate:      3.0,
		OutputRate:     15.0,
		TokensBudget:   nil,
		DollarsBudget:  nil,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error when no budgets set, got %v", err)
	}
}

func TestCheckBudget_RuntimeExceeded(t *testing.T) {
	session := &ActiveSession{
		MaxIterations: 100,
		MaxRuntime:    1 * time.Hour,
		StartedAt:     time.Now().Add(-2 * time.Hour), // Started 2 hours ago
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != ErrRuntimeLimit {
		t.Errorf("expected ErrRuntimeLimit, got %v", err)
	}
}

func TestCheckBudget_RuntimeBelowLimit(t *testing.T) {
	session := &ActiveSession{
		MaxIterations: 100,
		MaxRuntime:    4 * time.Hour,
		StartedAt:     time.Now().Add(-1 * time.Hour), // Started 1 hour ago
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckBudget_RuntimeZeroMeansNoLimit(t *testing.T) {
	session := &ActiveSession{
		MaxIterations: 100,
		MaxRuntime:    0, // No limit
		StartedAt:     time.Now().Add(-100 * time.Hour), // Started 100 hours ago
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error when MaxRuntime is 0, got %v", err)
	}
}

func TestDetectCompletion_TaskComplete(t *testing.T) {
	loop := &RalphLoop{}

	tests := []struct {
		response string
		expected bool
	}{
		{"The task is done. TASK_COMPLETE", true},
		{"TASK_COMPLETE - all tests pass", true},
		{"some output\nTASK_COMPLETE\nmore output", true},
		{"HAT_COMPLETE", true},
		{"Task done HAT_COMPLETE successfully", true},
		{"task complete", false}, // case sensitive
		{"TASK COMPLETE", false}, // no underscore
		{"nothing special here", false},
		{"", false},
	}

	for _, tt := range tests {
		result := loop.detectCompletion(tt.response)
		if result != tt.expected {
			t.Errorf("detectCompletion(%q) = %v, want %v", tt.response, result, tt.expected)
		}
	}
}

func TestDetectHatTransition_ValidHats(t *testing.T) {
	loop := &RalphLoop{}

	tests := []struct {
		response    string
		expectedHat string
	}{
		{"Work is done. HAT_TRANSITION:critic", "critic"},
		{"HAT_TRANSITION:creator", "creator"},
		{"Let's move on. HAT_TRANSITION:editor now", "editor"},
		{"HAT_TRANSITION:designer\nmore text", "designer"},
		{"HAT_TRANSITION:planner", "planner"},
		{"HAT_TRANSITION:explorer", "explorer"},
		{"HAT_TRANSITION:resolver", "resolver"},
	}

	for _, tt := range tests {
		result := loop.detectHatTransition(tt.response)
		if result != tt.expectedHat {
			t.Errorf("detectHatTransition(%q) = %q, want %q", tt.response, result, tt.expectedHat)
		}
	}
}

func TestDetectHatTransition_InvalidHats(t *testing.T) {
	loop := &RalphLoop{}

	tests := []struct {
		response string
	}{
		{"HAT_TRANSITION:invalid_hat"},
		{"HAT_TRANSITION:foobar"},
		{"HAT_TRANSITION:"},
		{"HAT_TRANSITION: creator"}, // space before hat name
		{"hat_transition:creator"},  // lowercase
		{"no transition here"},
		{""},
	}

	for _, tt := range tests {
		result := loop.detectHatTransition(tt.response)
		if result != "" {
			t.Errorf("detectHatTransition(%q) = %q, want empty string", tt.response, result)
		}
	}
}

func TestDetectHatTransition_EdgeCases(t *testing.T) {
	loop := &RalphLoop{}

	// Hat name at end of string
	result := loop.detectHatTransition("HAT_TRANSITION:critic")
	if result != "critic" {
		t.Errorf("expected 'critic' for end-of-string case, got %q", result)
	}

	// Hat name followed by newline
	result = loop.detectHatTransition("HAT_TRANSITION:creator\n")
	if result != "creator" {
		t.Errorf("expected 'creator' for newline case, got %q", result)
	}

	// Hat name followed by tab
	result = loop.detectHatTransition("HAT_TRANSITION:editor\textra")
	if result != "editor" {
		t.Errorf("expected 'editor' for tab case, got %q", result)
	}

	// Hat name followed by carriage return
	result = loop.detectHatTransition("HAT_TRANSITION:designer\r\n")
	if result != "designer" {
		t.Errorf("expected 'designer' for CR case, got %q", result)
	}
}

func TestSessionCost(t *testing.T) {
	tests := []struct {
		name         string
		inputTokens  int64
		outputTokens int64
		inputRate    float64
		outputRate   float64
		expectedCost float64
	}{
		{
			name:         "zero tokens",
			inputTokens:  0,
			outputTokens: 0,
			inputRate:    3.0,
			outputRate:   15.0,
			expectedCost: 0.0,
		},
		{
			name:         "1M input tokens only (Sonnet)",
			inputTokens:  1_000_000,
			outputTokens: 0,
			inputRate:    3.0,
			outputRate:   15.0,
			expectedCost: 3.0,
		},
		{
			name:         "1M output tokens only (Sonnet)",
			inputTokens:  0,
			outputTokens: 1_000_000,
			inputRate:    3.0,
			outputRate:   15.0,
			expectedCost: 15.0,
		},
		{
			name:         "1M each (Sonnet)",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			inputRate:    3.0,
			outputRate:   15.0,
			expectedCost: 18.0,
		},
		{
			name:         "1M each (Opus)",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			inputRate:    5.0,
			outputRate:   25.0,
			expectedCost: 30.0,
		},
		{
			name:         "small usage (Sonnet)",
			inputTokens:  1000,
			outputTokens: 500,
			inputRate:    3.0,
			outputRate:   15.0,
			expectedCost: 0.0105, // (1000*3 + 500*15) / 1M = 10500 / 1M
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &ActiveSession{
				InputTokens:  tt.inputTokens,
				OutputTokens: tt.outputTokens,
				InputRate:    tt.inputRate,
				OutputRate:   tt.outputRate,
			}

			result := session.Cost()

			// Allow small floating point differences
			diff := result - tt.expectedCost
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 {
				t.Errorf("Cost() = %f, want %f", result, tt.expectedCost)
			}
		})
	}
}

func TestBuildPrompt_NilManager(t *testing.T) {
	session := &ActiveSession{
		ID:     "test-session",
		TaskID: "test-task",
		Hat:    "creator",
	}

	// Test with nil manager
	loop := &RalphLoop{
		session: session,
		manager: nil,
	}

	_, err := loop.buildPrompt()
	if err == nil {
		t.Error("expected error when manager is nil, got nil")
	}
	if err.Error() != "manager or prompt loader not initialized" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// Test with manager but nil promptLoader
	loop.manager = &Manager{
		promptLoader: nil,
	}

	_, err = loop.buildPrompt()
	if err == nil {
		t.Error("expected error when promptLoader is nil, got nil")
	}
	if err.Error() != "manager or prompt loader not initialized" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestIsValidHat(t *testing.T) {
	validHats := []string{
		"explorer",
		"planner",
		"designer",
		"creator",
		"critic",
		"editor",
		"resolver",
	}

	for _, hat := range validHats {
		if !IsValidHat(hat) {
			t.Errorf("IsValidHat(%q) = false, want true", hat)
		}
	}

	invalidHats := []string{
		"invalid",
		"Creator", // case sensitive
		"EXPLORER",
		"",
		"planner ",
		" planner",
	}

	for _, hat := range invalidHats {
		if IsValidHat(hat) {
			t.Errorf("IsValidHat(%q) = true, want false", hat)
		}
	}
}
