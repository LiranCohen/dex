package session

import (
	"testing"

	"github.com/liranmauda/dex/internal/toolbelt"
)

func TestNewRalphLoop(t *testing.T) {
	// Create minimal ActiveSession
	session := &ActiveSession{
		ID:            "test-session-id",
		TaskID:        "test-task-id",
		Hat:           "implementer",
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
		TokensUsed:    1000,
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
		TokensUsed:    500,
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
		DollarsUsed:   5.0,
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
		DollarsUsed:   2.5,
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
		TokensUsed:     999999,
		TokensBudget:   nil,
		DollarsUsed:    999.99,
		DollarsBudget:  nil,
	}

	loop := &RalphLoop{session: session}

	err := loop.checkBudget()
	if err != nil {
		t.Errorf("expected no error when no budgets set, got %v", err)
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
		{"Code is done. HAT_TRANSITION:reviewer", "reviewer"},
		{"HAT_TRANSITION:implementer", "implementer"},
		{"Let's move on. HAT_TRANSITION:tester now", "tester"},
		{"HAT_TRANSITION:architect\nmore text", "architect"},
		{"HAT_TRANSITION:planner", "planner"},
		{"HAT_TRANSITION:debugger", "debugger"},
		{"HAT_TRANSITION:documenter", "documenter"},
		{"HAT_TRANSITION:devops", "devops"},
		{"HAT_TRANSITION:conflict_manager", "conflict_manager"},
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
		{"HAT_TRANSITION: implementer"}, // space before hat name
		{"hat_transition:implementer"},  // lowercase
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
	result := loop.detectHatTransition("HAT_TRANSITION:reviewer")
	if result != "reviewer" {
		t.Errorf("expected 'reviewer' for end-of-string case, got %q", result)
	}

	// Hat name followed by newline
	result = loop.detectHatTransition("HAT_TRANSITION:implementer\n")
	if result != "implementer" {
		t.Errorf("expected 'implementer' for newline case, got %q", result)
	}

	// Hat name followed by tab
	result = loop.detectHatTransition("HAT_TRANSITION:tester\textra")
	if result != "tester" {
		t.Errorf("expected 'tester' for tab case, got %q", result)
	}

	// Hat name followed by carriage return
	result = loop.detectHatTransition("HAT_TRANSITION:architect\r\n")
	if result != "architect" {
		t.Errorf("expected 'architect' for CR case, got %q", result)
	}
}

func TestEstimateCost(t *testing.T) {
	loop := &RalphLoop{}

	tests := []struct {
		name         string
		inputTokens  int
		outputTokens int
		expectedCost float64
	}{
		{
			name:         "zero tokens",
			inputTokens:  0,
			outputTokens: 0,
			expectedCost: 0.0,
		},
		{
			name:         "1M input tokens only",
			inputTokens:  1_000_000,
			outputTokens: 0,
			expectedCost: 3.0,
		},
		{
			name:         "1M output tokens only",
			inputTokens:  0,
			outputTokens: 1_000_000,
			expectedCost: 15.0,
		},
		{
			name:         "1M each",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			expectedCost: 18.0,
		},
		{
			name:         "small usage",
			inputTokens:  1000,
			outputTokens: 500,
			expectedCost: 0.0105, // (1000*3 + 500*15) / 1M = 10500 / 1M
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := toolbelt.AnthropicUsage{
				InputTokens:  tt.inputTokens,
				OutputTokens: tt.outputTokens,
			}

			result := loop.estimateCost(usage)

			// Allow small floating point differences
			diff := result - tt.expectedCost
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 {
				t.Errorf("estimateCost(%d input, %d output) = %f, want %f",
					tt.inputTokens, tt.outputTokens, result, tt.expectedCost)
			}
		})
	}
}

func TestBuildPrompt_NilManager(t *testing.T) {
	session := &ActiveSession{
		ID:     "test-session",
		TaskID: "test-task",
		Hat:    "implementer",
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
		"planner",
		"architect",
		"implementer",
		"reviewer",
		"tester",
		"debugger",
		"documenter",
		"devops",
		"conflict_manager",
	}

	for _, hat := range validHats {
		if !IsValidHat(hat) {
			t.Errorf("IsValidHat(%q) = false, want true", hat)
		}
	}

	invalidHats := []string{
		"invalid",
		"Implementer", // case sensitive
		"REVIEWER",
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
