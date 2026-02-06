package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/lirancohen/dex/internal/toolbelt"
)

func TestFindAllSignals(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		signal   string
		expected []string
	}{
		{
			name:     "Single CHECKLIST_DONE",
			content:  "Some text CHECKLIST_DONE:1\nmore text",
			signal:   SignalChecklistDone,
			expected: []string{"1"},
		},
		{
			name:     "Multiple CHECKLIST_DONE",
			content:  "CHECKLIST_DONE:1\nCHECKLIST_DONE:2\nCHECKLIST_DONE:3",
			signal:   SignalChecklistDone,
			expected: []string{"1", "2", "3"},
		},
		{
			name:     "CHECKLIST_FAILED with reason",
			content:  "CHECKLIST_FAILED:2:tests are failing",
			signal:   SignalChecklistFailed,
			expected: []string{"2:tests are failing"},
		},
		{
			name:     "No signals",
			content:  "Just regular text",
			signal:   SignalChecklistDone,
			expected: nil,
		},
		{
			name:     "Empty signal content",
			content:  "CHECKLIST_DONE:\nmore text",
			signal:   SignalChecklistDone,
			expected: nil, // Empty content is filtered out
		},
		{
			name:     "Signal at end of string",
			content:  "CHECKLIST_DONE:final",
			signal:   SignalChecklistDone,
			expected: []string{"final"},
		},
		{
			name:     "EVENT signal",
			content:  "EVENT:task.complete\nEVENT:plan.complete",
			signal:   SignalEvent,
			expected: []string{"task.complete", "plan.complete"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := findAllSignals(tc.content, tc.signal)
			if len(result) != len(tc.expected) {
				t.Errorf("expected %d signals, got %d: %v", len(tc.expected), len(result), result)
				return
			}
			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("expected signal %d to be %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "Short string unchanged",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "Exact length unchanged",
			input:    "exactly10!",
			maxLen:   10,
			expected: "exactly10!",
		},
		{
			name:     "Long string truncated",
			input:    "this is a very long string",
			maxLen:   10,
			expected: "this is a ...",
		},
		{
			name:     "Empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "Zero maxLen",
			input:    "test",
			maxLen:   0,
			expected: "...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := truncateOutput(tc.input, tc.maxLen)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestIsHatTransitionEvent(t *testing.T) {
	tests := []struct {
		event    string
		expected bool
	}{
		{"plan.complete", true},
		{"design.complete", true},
		{"implementation.done", true},
		{"review.approved", true},
		{"review.rejected", true},
		{"resolved", true},
		{"task.blocked", true},
		{"task.complete", false},
		{"random.event", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.event, func(t *testing.T) {
			result := isHatTransitionEvent(tc.event)
			if result != tc.expected {
				t.Errorf("isHatTransitionEvent(%q) = %v, expected %v", tc.event, result, tc.expected)
			}
		})
	}
}

func TestWorkerRalphLoop_DetectCompletion(t *testing.T) {
	loop := &WorkerRalphLoop{
		session: NewWorkerSession("test", "obj", "explorer", "/work"),
	}

	tests := []struct {
		name     string
		response string
		expected bool
	}{
		{
			name:     "Contains task.complete",
			response: "All done! EVENT:task.complete",
			expected: true,
		},
		{
			name:     "task.complete in middle",
			response: "Step 1 done. EVENT:task.complete. Finishing up.",
			expected: true,
		},
		{
			name:     "No completion signal",
			response: "Still working on it...",
			expected: false,
		},
		{
			name:     "Other event",
			response: "EVENT:plan.complete",
			expected: false,
		},
		{
			name:     "Empty response",
			response: "",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := loop.detectCompletion(tc.response)
			if result != tc.expected {
				t.Errorf("detectCompletion(%q) = %v, expected %v", tc.response, result, tc.expected)
			}
		})
	}
}

func TestWorkerRalphLoop_DetectEvent(t *testing.T) {
	loop := &WorkerRalphLoop{
		session: NewWorkerSession("test", "obj", "explorer", "/work"),
	}

	tests := []struct {
		name     string
		response string
		expected string
	}{
		{
			name:     "Simple event",
			response: "EVENT:task.complete",
			expected: "task.complete",
		},
		{
			name:     "Event with text after",
			response: "EVENT:plan.complete\nMore text",
			expected: "plan.complete",
		},
		{
			name:     "Event with space",
			response: "EVENT:design.complete ",
			expected: "design.complete",
		},
		{
			name:     "No event",
			response: "Just regular text",
			expected: "",
		},
		{
			name:     "Event in middle",
			response: "Finished work EVENT:resolved\nDone",
			expected: "resolved",
		},
		{
			name:     "Empty response",
			response: "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := loop.detectEvent(tc.response)
			if result != tc.expected {
				t.Errorf("detectEvent(%q) = %q, expected %q", tc.response, result, tc.expected)
			}
		})
	}
}

func TestWorkerRalphLoop_GetContinuationPrompt(t *testing.T) {
	tests := []struct {
		hat      string
		contains string
	}{
		{"explorer", "exploring"},
		{"planner", "planning"},
		{"designer", "designing"},
		{"creator", "implementing"},
		{"critic", "reviewing"},
		{"editor", "polishing"},
		{"resolver", "resolving"},
		{"unknown", "Continue"},
	}

	for _, tc := range tests {
		t.Run(tc.hat, func(t *testing.T) {
			session := NewWorkerSession("test", "obj", tc.hat, "/work")
			loop := &WorkerRalphLoop{session: session}

			result := loop.getContinuationPrompt()
			if result == "" {
				t.Error("expected non-empty continuation prompt")
			}
			// Just verify we get some response for each hat
			if len(result) < 10 {
				t.Errorf("continuation prompt too short: %q", result)
			}
		})
	}
}

func TestWorkerRalphLoop_ProcessChecklistSignals(t *testing.T) {
	t.Run("Marks items done", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		loop := &WorkerRalphLoop{
			session:  session,
			activity: activity,
		}

		response := "Completed first item CHECKLIST_DONE:1\nCompleted second CHECKLIST_DONE:2"
		loop.processChecklistSignals(response)

		done, _ := session.GetChecklistStatus()
		if len(done) != 2 {
			t.Errorf("expected 2 done items, got %d", len(done))
		}
	})

	t.Run("Marks items failed with reason", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		loop := &WorkerRalphLoop{
			session:  session,
			activity: activity,
		}

		response := "CHECKLIST_FAILED:3:tests not passing"
		loop.processChecklistSignals(response)

		_, failed := session.GetChecklistStatus()
		if len(failed) != 1 {
			t.Errorf("expected 1 failed item, got %d", len(failed))
		}
	})

	t.Run("Mixed done and failed", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		loop := &WorkerRalphLoop{
			session:  session,
			activity: activity,
		}

		response := "CHECKLIST_DONE:1\nCHECKLIST_FAILED:2:blocked\nCHECKLIST_DONE:3"
		loop.processChecklistSignals(response)

		done, failed := session.GetChecklistStatus()
		if len(done) != 2 {
			t.Errorf("expected 2 done items, got %d", len(done))
		}
		if len(failed) != 1 {
			t.Errorf("expected 1 failed item, got %d", len(failed))
		}
	})
}

func TestWorkerRalphLoop_ProcessScratchpadSignal(t *testing.T) {
	t.Run("Extracts scratchpad content", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		loop := &WorkerRalphLoop{
			session:  session,
			activity: activity,
		}

		response := "Working on task. SCRATCHPAD:Remember to check the API docs for auth flow."
		loop.processScratchpadSignal(response)

		scratchpad := session.GetScratchpad()
		if scratchpad != "Remember to check the API docs for auth flow." {
			t.Errorf("unexpected scratchpad: %q", scratchpad)
		}
	})

	t.Run("Scratchpad ends at EVENT", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		loop := &WorkerRalphLoop{
			session:  session,
			activity: activity,
		}

		response := "SCRATCHPAD:Notes here EVENT:task.complete"
		loop.processScratchpadSignal(response)

		scratchpad := session.GetScratchpad()
		if scratchpad != "Notes here" {
			t.Errorf("unexpected scratchpad: %q", scratchpad)
		}
	})

	t.Run("No scratchpad signal", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		loop := &WorkerRalphLoop{
			session:  session,
			activity: activity,
		}

		session.UpdateScratchpad("existing")
		response := "Regular response without scratchpad"
		loop.processScratchpadSignal(response)

		// Should not modify existing scratchpad
		if session.GetScratchpad() != "existing" {
			t.Error("scratchpad was modified unexpectedly")
		}
	})
}

func TestSignalConstants(t *testing.T) {
	// Verify signal constants have expected values
	if SignalChecklistDone != "CHECKLIST_DONE:" {
		t.Errorf("unexpected SignalChecklistDone: %q", SignalChecklistDone)
	}
	if SignalChecklistFailed != "CHECKLIST_FAILED:" {
		t.Errorf("unexpected SignalChecklistFailed: %q", SignalChecklistFailed)
	}
	if SignalEvent != "EVENT:" {
		t.Errorf("unexpected SignalEvent: %q", SignalEvent)
	}
	if SignalScratchpad != "SCRATCHPAD:" {
		t.Errorf("unexpected SignalScratchpad: %q", SignalScratchpad)
	}
}

func TestBudgetErrors(t *testing.T) {
	// Verify error values
	if ErrBudgetExceeded == nil {
		t.Error("ErrBudgetExceeded should not be nil")
	}
	if ErrIterationLimit == nil {
		t.Error("ErrIterationLimit should not be nil")
	}
	if ErrTokenBudget == nil {
		t.Error("ErrTokenBudget should not be nil")
	}
	if ErrRuntimeLimit == nil {
		t.Error("ErrRuntimeLimit should not be nil")
	}
	if ErrNoAnthropicClient == nil {
		t.Error("ErrNoAnthropicClient should not be nil")
	}
	if ErrCancelled == nil {
		t.Error("ErrCancelled should not be nil")
	}

	// Verify error messages
	if ErrCancelled.Error() != "execution cancelled" {
		t.Errorf("unexpected error message: %s", ErrCancelled.Error())
	}
}

func TestWorkerRalphLoop_CheckBudget(t *testing.T) {
	t.Run("No budget set - passes", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		loop := &WorkerRalphLoop{session: session}

		if err := loop.checkBudget(); err != nil {
			t.Errorf("expected no error with no budget, got %v", err)
		}
	})

	t.Run("Iteration limit exceeded", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		session.SetBudgets(0, 5, 0)
		loop := &WorkerRalphLoop{session: session}

		// Simulate 5 iterations
		for i := 0; i < 5; i++ {
			session.RecordIteration(100, 50)
		}

		err := loop.checkBudget()
		if err != ErrIterationLimit {
			t.Errorf("expected ErrIterationLimit, got %v", err)
		}
	})

	t.Run("Token budget exceeded", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		session.SetBudgets(1000, 0, 0)
		loop := &WorkerRalphLoop{session: session}

		// Add tokens exceeding budget
		session.RecordIteration(600, 500)

		err := loop.checkBudget()
		if err != ErrTokenBudget {
			t.Errorf("expected ErrTokenBudget, got %v", err)
		}
	})

	t.Run("Within budget", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		session.SetBudgets(10000, 100, 0)
		loop := &WorkerRalphLoop{session: session}

		session.RecordIteration(100, 50)

		if err := loop.checkBudget(); err != nil {
			t.Errorf("expected no error within budget, got %v", err)
		}
	})
}

func TestWorkerRalphLoop_BuildToolDescriptions(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	loop := &WorkerRalphLoop{
		session: session,
		tools:   getToolDefinitionsForHat("creator"),
	}

	desc := loop.buildToolDescriptions()

	// Should start with header
	if desc[:20] != "## Available Tools\n\n" {
		t.Errorf("unexpected header: %s", desc[:20])
	}

	// Should contain tool entries
	if len(desc) < 100 {
		t.Error("tool descriptions seem too short")
	}
}

func TestGetToolDefinitionsForHat(t *testing.T) {
	hats := []string{"explorer", "planner", "designer", "creator", "critic", "editor", "resolver"}

	for _, hat := range hats {
		t.Run(hat, func(t *testing.T) {
			tools := getToolDefinitionsForHat(hat)
			if len(tools) == 0 {
				t.Errorf("expected tools for hat %s, got none", hat)
			}

			// Verify each tool has required fields
			for _, tool := range tools {
				if tool.Name == "" {
					t.Error("tool has empty name")
				}
				if tool.Description == "" {
					t.Errorf("tool %s has empty description", tool.Name)
				}
			}
		})
	}
}

func TestGetTargetHatForEvent(t *testing.T) {
	tests := []struct {
		event      string
		currentHat string
		expected   string
	}{
		{"plan.complete", "planner", "designer"},
		{"design.complete", "designer", "creator"},
		{"implementation.done", "creator", "critic"},
		{"review.approved", "critic", "editor"},
		{"review.rejected", "critic", "creator"},
		{"task.blocked", "creator", "resolver"},
		{"resolved", "resolver", "creator"},
		{"task.complete", "editor", ""}, // Not a transition event
		{"unknown.event", "explorer", ""},
		{"", "explorer", ""},
	}

	for _, tc := range tests {
		t.Run(tc.event, func(t *testing.T) {
			result := getTargetHatForEvent(tc.event, tc.currentHat)
			if result != tc.expected {
				t.Errorf("getTargetHatForEvent(%q, %q) = %q, expected %q",
					tc.event, tc.currentHat, result, tc.expected)
			}
		})
	}
}

func TestWorkerRalphLoop_CanTransitionTo(t *testing.T) {
	t.Run("Allows first transition", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "explorer", "/work")
		loop := &WorkerRalphLoop{session: session}

		if !loop.canTransitionTo("planner") {
			t.Error("expected first transition to be allowed")
		}
	})

	t.Run("Blocks after max visits to same hat", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "explorer", "/work")
		loop := &WorkerRalphLoop{session: session}

		// Simulate visiting creator MaxHatVisits times
		for i := 0; i < MaxHatVisits; i++ {
			session.RecordHatTransition("other", "creator", "test")
			session.RecordHatTransition("creator", "critic", "implementation.done")
		}

		if loop.canTransitionTo("creator") {
			t.Errorf("expected transition to creator to be blocked after %d visits", MaxHatVisits)
		}
	})

	t.Run("Blocks after max total transitions", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "explorer", "/work")
		loop := &WorkerRalphLoop{session: session}

		// Simulate MaxTotalTransitions transitions
		hats := []string{"planner", "creator", "critic", "editor"}
		for i := 0; i < MaxTotalTransitions; i++ {
			from := hats[i%len(hats)]
			to := hats[(i+1)%len(hats)]
			session.RecordHatTransition(from, to, "test")
		}

		if loop.canTransitionTo("resolver") {
			t.Errorf("expected transition to be blocked after %d total transitions", MaxTotalTransitions)
		}
	})

	t.Run("Allows transition within limits", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "explorer", "/work")
		loop := &WorkerRalphLoop{session: session}

		// Just one transition
		session.RecordHatTransition("explorer", "planner", "plan.complete")

		if !loop.canTransitionTo("creator") {
			t.Error("expected transition to creator to be allowed")
		}
	})
}

func TestLoopDetectionConstants(t *testing.T) {
	// Verify constants have reasonable values
	if MaxHatVisits < 2 {
		t.Errorf("MaxHatVisits too low: %d", MaxHatVisits)
	}
	if MaxTotalTransitions < MaxHatVisits {
		t.Errorf("MaxTotalTransitions (%d) should be >= MaxHatVisits (%d)",
			MaxTotalTransitions, MaxHatVisits)
	}
}

func TestWorkerRalphLoop_GetHatInstructions(t *testing.T) {
	session := NewWorkerSession("test", "obj", "explorer", "/work")
	loop := &WorkerRalphLoop{session: session}

	hats := []string{"explorer", "planner", "designer", "creator", "critic", "editor", "resolver"}

	for _, hat := range hats {
		t.Run(hat, func(t *testing.T) {
			instructions := loop.getHatInstructions(hat)
			if instructions == "" {
				t.Errorf("expected non-empty instructions for hat %s", hat)
			}
			if len(instructions) < 10 {
				t.Errorf("instructions too short for hat %s: %q", hat, instructions)
			}
		})
	}

	t.Run("Unknown hat", func(t *testing.T) {
		instructions := loop.getHatInstructions("unknown")
		if instructions == "" {
			t.Error("expected fallback instructions for unknown hat")
		}
	})
}

// MockChatClient is a test double for ChatClient.
type MockChatClient struct {
	responses []*toolbelt.AnthropicChatResponse
	errors    []error
	calls     int
}

func (m *MockChatClient) ChatWithStreaming(ctx context.Context, req *toolbelt.AnthropicChatRequest, onDelta toolbelt.StreamCallback) (*toolbelt.AnthropicChatResponse, error) {
	if m.calls >= len(m.responses) && m.calls >= len(m.errors) {
		return nil, fmt.Errorf("no more mock responses configured")
	}

	call := m.calls
	m.calls++

	if call < len(m.errors) && m.errors[call] != nil {
		return nil, m.errors[call]
	}

	if call < len(m.responses) {
		return m.responses[call], nil
	}

	return nil, fmt.Errorf("no response at index %d", call)
}

// setupPromptLoader creates and initializes a prompt loader for tests.
func setupPromptLoader(t *testing.T) *WorkerPromptLoader {
	t.Helper()
	loader := NewWorkerPromptLoader()
	if err := loader.LoadAll(); err != nil {
		t.Fatalf("failed to load prompts: %v", err)
	}
	return loader
}

func TestWorkerRalphLoop_Run_NoClient(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	loop := &WorkerRalphLoop{
		session: session,
		client:  nil, // No client
	}

	_, err := loop.Run(context.Background())
	if err != ErrNoAnthropicClient {
		t.Errorf("expected ErrNoAnthropicClient, got %v", err)
	}
}

func TestWorkerRalphLoop_Run_Cancellation(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	activity := NewWorkerActivityRecorder(nil, nil, session, 30)
	promptLoader := setupPromptLoader(t)

	mockClient := &MockChatClient{
		// No responses - context will be cancelled before any API calls
	}

	loop := NewWorkerRalphLoop(
		session,
		mockClient,
		activity,
		nil, // No conn
		promptLoader,
		nil, // No executor
		&Objective{ID: "obj-1", Title: "Test"},
		&Project{},
		"",
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	report, err := loop.Run(ctx)
	if err != ErrCancelled {
		t.Errorf("expected ErrCancelled, got %v", err)
	}
	if report.Status != "cancelled" {
		t.Errorf("expected status cancelled, got %s", report.Status)
	}
}

func TestWorkerRalphLoop_Run_TaskComplete(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	activity := NewWorkerActivityRecorder(nil, nil, session, 30)
	promptLoader := setupPromptLoader(t)

	mockClient := &MockChatClient{
		responses: []*toolbelt.AnthropicChatResponse{
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "Task done! EVENT:task.complete"},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}

	loop := NewWorkerRalphLoop(
		session,
		mockClient,
		activity,
		nil,
		promptLoader,
		nil,
		&Objective{ID: "obj-1", Title: "Test"},
		&Project{},
		"",
	)

	report, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Status != "completed" {
		t.Errorf("expected status completed, got %s", report.Status)
	}
	if mockClient.calls != 1 {
		t.Errorf("expected 1 API call, got %d", mockClient.calls)
	}
}

func TestWorkerRalphLoop_Run_HatTransition(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	activity := NewWorkerActivityRecorder(nil, nil, session, 30)
	promptLoader := setupPromptLoader(t)

	mockClient := &MockChatClient{
		responses: []*toolbelt.AnthropicChatResponse{
			// First response triggers transition to critic
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "Implementation done. EVENT:implementation.done"},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
			// Second response (as critic) completes task
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "Review complete. EVENT:task.complete"},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}

	loop := NewWorkerRalphLoop(
		session,
		mockClient,
		activity,
		nil,
		promptLoader,
		nil,
		&Objective{ID: "obj-1", Title: "Test"},
		&Project{},
		"",
	)

	report, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Status != "completed" {
		t.Errorf("expected status completed, got %s", report.Status)
	}

	// Should have transitioned from creator to critic
	if session.GetHat() != "critic" {
		t.Errorf("expected hat critic, got %s", session.GetHat())
	}
	if session.GetTransitionCount() != 1 {
		t.Errorf("expected 1 transition, got %d", session.GetTransitionCount())
	}
}

func TestWorkerRalphLoop_Run_IterationLimit(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	session.SetBudgets(0, 2, 0) // Max 2 iterations
	activity := NewWorkerActivityRecorder(nil, nil, session, 30)
	promptLoader := setupPromptLoader(t)

	mockClient := &MockChatClient{
		responses: []*toolbelt.AnthropicChatResponse{
			// First response - no completion signal
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "Working on it..."},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
			// Second response - still no completion
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "Still working..."},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
			// Third would exceed limit
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "More work..."},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}

	loop := NewWorkerRalphLoop(
		session,
		mockClient,
		activity,
		nil,
		promptLoader,
		nil,
		&Objective{ID: "obj-1", Title: "Test"},
		&Project{},
		"",
	)

	report, err := loop.Run(context.Background())
	if err != ErrIterationLimit {
		t.Errorf("expected ErrIterationLimit, got %v", err)
	}
	if report.Status != "budget_exceeded" {
		t.Errorf("expected status budget_exceeded, got %s", report.Status)
	}
}

func TestWorkerRalphLoop_Run_LoopDetection(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	activity := NewWorkerActivityRecorder(nil, nil, session, 30)
	promptLoader := setupPromptLoader(t)

	// Create responses that would cause infinite loop
	var responses []*toolbelt.AnthropicChatResponse
	for i := 0; i < MaxTotalTransitions+5; i++ {
		// Alternate between implementation.done (creator->critic) and review.rejected (critic->creator)
		var text string
		if i%2 == 0 {
			text = "Done implementing. EVENT:implementation.done"
		} else {
			text = "Needs fixes. EVENT:review.rejected"
		}
		responses = append(responses, &toolbelt.AnthropicChatResponse{
			Content: []toolbelt.AnthropicContentBlock{
				{Type: "text", Text: text},
			},
			StopReason: "end_turn",
			Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
		})
	}

	mockClient := &MockChatClient{responses: responses}

	loop := NewWorkerRalphLoop(
		session,
		mockClient,
		activity,
		nil,
		promptLoader,
		nil,
		&Objective{ID: "obj-1", Title: "Test"},
		&Project{},
		"",
	)

	report, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Status != "loop_limit" {
		t.Errorf("expected status loop_limit, got %s", report.Status)
	}

	// Should have stopped due to per-hat visit limit (creator or critic visited 3 times)
	// The loop alternates creator->critic->creator, so we hit the limit after ~6 transitions
	if session.GetTransitionCount() < MaxHatVisits*2 {
		t.Errorf("expected at least %d transitions (hitting per-hat limit), got %d", MaxHatVisits*2, session.GetTransitionCount())
	}
}

func TestWorkerRalphLoop_Run_ChecklistSignals(t *testing.T) {
	session := NewWorkerSession("test", "obj", "creator", "/work")
	activity := NewWorkerActivityRecorder(nil, nil, session, 30)
	promptLoader := setupPromptLoader(t)

	mockClient := &MockChatClient{
		responses: []*toolbelt.AnthropicChatResponse{
			{
				Content: []toolbelt.AnthropicContentBlock{
					{Type: "text", Text: "Completed items:\nCHECKLIST_DONE:1\nCHECKLIST_DONE:2\nCHECKLIST_FAILED:3:blocked by dependency\nEVENT:task.complete"},
				},
				StopReason: "end_turn",
				Usage:      toolbelt.AnthropicUsage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}

	loop := NewWorkerRalphLoop(
		session,
		mockClient,
		activity,
		nil,
		promptLoader,
		nil,
		&Objective{ID: "obj-1", Title: "Test", Checklist: []string{"Item 1", "Item 2", "Item 3"}},
		&Project{},
		"",
	)

	report, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	done, failed := session.GetChecklistStatus()
	if len(done) != 2 {
		t.Errorf("expected 2 done items, got %d", len(done))
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(failed))
	}

	if len(report.ChecklistDone) != 2 {
		t.Errorf("expected 2 done in report, got %d", len(report.ChecklistDone))
	}
}

// ====================
// Checkpoint/Restore Tests
// ====================

func TestWorkerRalphLoop_RestoreFromCheckpoint(t *testing.T) {
	t.Run("Restores conversation", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		promptLoader := setupPromptLoader(t)

		loop := NewWorkerRalphLoop(
			session,
			nil, // No client needed for restore test
			activity,
			nil,
			promptLoader,
			nil,
			&Objective{ID: "obj-1", Title: "Test"},
			&Project{},
			"",
		)

		state := &SessionState{
			SessionID:       "test",
			ObjectiveID:     "obj",
			Hat:             "critic",
			Iteration:       5,
			TokensInput:     1000,
			TokensOutput:    500,
			Conversation:    `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there"}]`,
			Scratchpad:      "My notes",
			ChecklistDone:   []string{"1", "2"},
			ChecklistFailed: []string{"3"},
			HatHistory:      `[{"Hat":"explorer","StartedAt":"2024-01-01T00:00:00Z","EndedAt":"2024-01-01T00:01:00Z","Event":"plan.complete"}]`,
			TransitionCount: 3,
			PreviousHat:     "creator",
			Status:          "running",
			WorkDir:         "/work",
		}

		err := loop.RestoreFromCheckpoint(state)
		if err != nil {
			t.Fatalf("failed to restore: %v", err)
		}

		// Verify session state was restored
		if session.GetHat() != "critic" {
			t.Errorf("expected hat 'critic', got '%s'", session.GetHat())
		}
		if session.GetIteration() != 5 {
			t.Errorf("expected iteration 5, got %d", session.GetIteration())
		}
		if session.GetScratchpad() != "My notes" {
			t.Errorf("expected scratchpad 'My notes', got '%s'", session.GetScratchpad())
		}
		if session.GetTransitionCount() != 3 {
			t.Errorf("expected transition count 3, got %d", session.GetTransitionCount())
		}
		if session.PreviousHat != "creator" {
			t.Errorf("expected previous hat 'creator', got '%s'", session.PreviousHat)
		}

		done, failed := session.GetChecklistStatus()
		if len(done) != 2 {
			t.Errorf("expected 2 done items, got %d", len(done))
		}
		if len(failed) != 1 {
			t.Errorf("expected 1 failed item, got %d", len(failed))
		}

		// Verify conversation was restored
		if len(loop.messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(loop.messages))
		}

		// Verify tools were updated for new hat
		if len(loop.tools) == 0 {
			t.Error("expected tools to be set for critic hat")
		}
	})

	t.Run("Handles empty conversation", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		promptLoader := setupPromptLoader(t)

		loop := NewWorkerRalphLoop(
			session,
			nil,
			activity,
			nil,
			promptLoader,
			nil,
			&Objective{ID: "obj-1", Title: "Test"},
			&Project{},
			"",
		)

		state := &SessionState{
			SessionID:    "test",
			ObjectiveID:  "obj",
			Hat:          "explorer",
			Conversation: "",
			Status:       "running",
		}

		err := loop.RestoreFromCheckpoint(state)
		if err != nil {
			t.Fatalf("failed to restore: %v", err)
		}

		// Should have no messages
		if len(loop.messages) != 0 {
			t.Errorf("expected 0 messages, got %d", len(loop.messages))
		}
	})

	t.Run("Handles empty checklist arrays", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		promptLoader := setupPromptLoader(t)

		loop := NewWorkerRalphLoop(
			session,
			nil,
			activity,
			nil,
			promptLoader,
			nil,
			&Objective{ID: "obj-1", Title: "Test"},
			&Project{},
			"",
		)

		state := &SessionState{
			SessionID:       "test",
			ObjectiveID:     "obj",
			Hat:             "explorer",
			Conversation:    "[]",
			ChecklistDone:   nil,
			ChecklistFailed: nil,
			HatHistory:      "[]",
			Status:          "running",
		}

		err := loop.RestoreFromCheckpoint(state)
		if err != nil {
			t.Fatalf("failed to restore: %v", err)
		}

		done, failed := session.GetChecklistStatus()
		if len(done) != 0 {
			t.Errorf("expected 0 done items, got %d", len(done))
		}
		if len(failed) != 0 {
			t.Errorf("expected 0 failed items, got %d", len(failed))
		}
	})

	t.Run("Restores token usage", func(t *testing.T) {
		session := NewWorkerSession("test", "obj", "creator", "/work")
		activity := NewWorkerActivityRecorder(nil, nil, session, 30)
		promptLoader := setupPromptLoader(t)

		loop := NewWorkerRalphLoop(
			session,
			nil,
			activity,
			nil,
			promptLoader,
			nil,
			&Objective{ID: "obj-1", Title: "Test"},
			&Project{},
			"",
		)

		state := &SessionState{
			SessionID:    "test",
			ObjectiveID:  "obj",
			Hat:          "creator",
			TokensInput:  5000,
			TokensOutput: 2500,
			Status:       "running",
		}

		err := loop.RestoreFromCheckpoint(state)
		if err != nil {
			t.Fatalf("failed to restore: %v", err)
		}

		input, output := session.GetTokenUsage()
		if input != 5000 {
			t.Errorf("expected input tokens 5000, got %d", input)
		}
		if output != 2500 {
			t.Errorf("expected output tokens 2500, got %d", output)
		}
	})
}
