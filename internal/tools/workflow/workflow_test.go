package workflow

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewExecutor(t *testing.T) {
	exec := NewExecutor("task-123", "session-456")
	if exec.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %q", exec.TaskID)
	}
	if exec.SessionID != "session-456" {
		t.Errorf("expected SessionID 'session-456', got %q", exec.SessionID)
	}
}

func TestExecutor_MarkChecklistItem_Done(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	var calledItemID string
	var calledStatus ChecklistItemStatus
	exec.OnChecklistUpdate = func(itemID string, status ChecklistItemStatus, reason string) error {
		calledItemID = itemID
		calledStatus = status
		return nil
	}

	result := exec.MarkChecklistItem("citm-abc123", ChecklistStatusDone, "")

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if calledItemID != "citm-abc123" {
		t.Errorf("expected itemID 'citm-abc123', got %q", calledItemID)
	}
	if calledStatus != ChecklistStatusDone {
		t.Errorf("expected status 'done', got %q", calledStatus)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if output["item_id"] != "citm-abc123" {
		t.Errorf("expected item_id 'citm-abc123', got %v", output["item_id"])
	}
	if output["status"] != "done" {
		t.Errorf("expected status 'done', got %v", output["status"])
	}
}

func TestExecutor_MarkChecklistItem_Failed(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	var calledReason string
	exec.OnChecklistUpdate = func(itemID string, status ChecklistItemStatus, reason string) error {
		calledReason = reason
		return nil
	}

	result := exec.MarkChecklistItem("citm-abc123", ChecklistStatusFailed, "tests failing")

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if calledReason != "tests failing" {
		t.Errorf("expected reason 'tests failing', got %q", calledReason)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if output["reason"] != "tests failing" {
		t.Errorf("expected reason 'tests failing', got %v", output["reason"])
	}
}

func TestExecutor_MarkChecklistItem_FailedNoReason(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	result := exec.MarkChecklistItem("citm-abc123", ChecklistStatusFailed, "")

	if !result.IsError {
		t.Error("expected error when status is failed without reason")
	}
	if result.Output != "Reason is required when status is 'failed'" {
		t.Errorf("unexpected error message: %s", result.Output)
	}
}

func TestExecutor_MarkChecklistItem_SkippedNoReason(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	result := exec.MarkChecklistItem("citm-abc123", ChecklistStatusSkipped, "")

	if !result.IsError {
		t.Error("expected error when status is skipped without reason")
	}
}

func TestExecutor_MarkChecklistItem_InvalidStatus(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	result := exec.MarkChecklistItem("citm-abc123", ChecklistItemStatus("invalid"), "")

	if !result.IsError {
		t.Error("expected error for invalid status")
	}
}

func TestExecutor_MarkChecklistItem_HandlerError(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")
	exec.OnChecklistUpdate = func(itemID string, status ChecklistItemStatus, reason string) error {
		return errors.New("database error")
	}

	result := exec.MarkChecklistItem("citm-abc123", ChecklistStatusDone, "")

	if !result.IsError {
		t.Error("expected error when handler fails")
	}
	if result.Output != "Failed to update checklist item: database error" {
		t.Errorf("unexpected error message: %s", result.Output)
	}
}

func TestExecutor_SignalEvent(t *testing.T) {
	tests := []struct {
		event   string
		nextHat string
	}{
		{"plan.complete", "designer"},
		{"design.complete", "creator"},
		{"implementation.done", "critic"},
		{"review.approved", "editor"},
		{"review.rejected", "creator"},
		{"task.blocked", "resolver"},
		{"resolved", "creator"},
		{"task.complete", ""},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			exec := NewExecutor("task-1", "session-1")

			var calledEvent EventType
			exec.OnEvent = func(event EventType, payload map[string]any, ackFailures bool) error {
				calledEvent = event
				return nil
			}

			result := exec.SignalEvent(tt.event, nil, false)

			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Output)
			}
			if string(calledEvent) != tt.event {
				t.Errorf("expected event %q, got %q", tt.event, calledEvent)
			}

			var output EventResult
			if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
				t.Fatalf("failed to parse output: %v", err)
			}
			if output.NextHat != tt.nextHat {
				t.Errorf("expected next_hat %q, got %q", tt.nextHat, output.NextHat)
			}
		})
	}
}

func TestExecutor_SignalEvent_InvalidEvent(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	result := exec.SignalEvent("invalid.event", nil, false)

	if !result.IsError {
		t.Error("expected error for invalid event")
	}
}

func TestExecutor_SignalEvent_WithPayload(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	var calledPayload map[string]any
	exec.OnEvent = func(event EventType, payload map[string]any, ackFailures bool) error {
		calledPayload = payload
		return nil
	}

	payload := map[string]any{"reason": "waiting for API response"}
	result := exec.SignalEvent("task.blocked", payload, false)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if calledPayload["reason"] != "waiting for API response" {
		t.Errorf("expected payload reason, got %v", calledPayload)
	}
}

func TestExecutor_SignalEvent_AckFailures(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	var calledAck bool
	exec.OnEvent = func(event EventType, payload map[string]any, ackFailures bool) error {
		calledAck = ackFailures
		return nil
	}

	result := exec.SignalEvent("task.complete", nil, true)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !calledAck {
		t.Error("expected acknowledge_failures to be true")
	}
}

func TestExecutor_UpdateScratchpad(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	var calledScratchpad Scratchpad
	exec.OnScratchpadUpdate = func(scratchpad Scratchpad) error {
		calledScratchpad = scratchpad
		return nil
	}

	result := exec.UpdateScratchpad(
		"Building a Hugo blog",
		"1. [x] Create site\n2. [ ] Add theme",
		"Using Blowfish theme",
		"",
		"Created site structure, next: configure theme",
	)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if calledScratchpad.Understanding != "Building a Hugo blog" {
		t.Errorf("unexpected understanding: %q", calledScratchpad.Understanding)
	}
	if calledScratchpad.Plan != "1. [x] Create site\n2. [ ] Add theme" {
		t.Errorf("unexpected plan: %q", calledScratchpad.Plan)
	}
	if calledScratchpad.Decisions != "Using Blowfish theme" {
		t.Errorf("unexpected decisions: %q", calledScratchpad.Decisions)
	}
	if calledScratchpad.LastAction != "Created site structure, next: configure theme" {
		t.Errorf("unexpected last_action: %q", calledScratchpad.LastAction)
	}

	if result.Output != `{"updated": true}` {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestExecutor_UpdateScratchpad_MissingRequired(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	tests := []struct {
		name          string
		understanding string
		plan          string
		lastAction    string
		expectedErr   string
	}{
		{"missing understanding", "", "plan", "action", "understanding is required"},
		{"missing plan", "understanding", "", "action", "plan is required"},
		{"missing last_action", "understanding", "plan", "", "last_action is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := exec.UpdateScratchpad(tt.understanding, tt.plan, "", "", tt.lastAction)
			if !result.IsError {
				t.Error("expected error")
			}
			if result.Output != tt.expectedErr {
				t.Errorf("expected %q, got %q", tt.expectedErr, result.Output)
			}
		})
	}
}

func TestExecutor_StoreMemory(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	var calledMemory Memory
	exec.OnMemoryStore = func(memory Memory) (string, error) {
		calledMemory = memory
		return "mem-12345", nil
	}

	result := exec.StoreMemory("pattern", "Tests use table-driven pattern", "internal/tools/workflow_test.go")

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if calledMemory.Category != "pattern" {
		t.Errorf("expected category 'pattern', got %q", calledMemory.Category)
	}
	if calledMemory.Content != "Tests use table-driven pattern" {
		t.Errorf("unexpected content: %q", calledMemory.Content)
	}
	if calledMemory.Source != "internal/tools/workflow_test.go" {
		t.Errorf("unexpected source: %q", calledMemory.Source)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if output["memory_id"] != "mem-12345" {
		t.Errorf("expected memory_id 'mem-12345', got %v", output["memory_id"])
	}
	if output["category"] != "pattern" {
		t.Errorf("expected category 'pattern', got %v", output["category"])
	}
}

func TestExecutor_StoreMemory_InvalidCategory(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	result := exec.StoreMemory("invalid", "content", "")

	if !result.IsError {
		t.Error("expected error for invalid category")
	}
}

func TestExecutor_StoreMemory_EmptyContent(t *testing.T) {
	exec := NewExecutor("task-1", "session-1")

	result := exec.StoreMemory("pattern", "", "")

	if !result.IsError {
		t.Error("expected error for empty content")
	}
	if result.Output != "content is required" {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestValidEventTypes(t *testing.T) {
	events := ValidEventTypes()
	if len(events) != 8 {
		t.Errorf("expected 8 event types, got %d", len(events))
	}

	expected := []EventType{
		EventPlanComplete,
		EventDesignComplete,
		EventImplementationDone,
		EventReviewApproved,
		EventReviewRejected,
		EventTaskBlocked,
		EventResolved,
		EventTaskComplete,
	}

	for _, e := range expected {
		found := false
		for _, actual := range events {
			if actual == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing event type: %s", e)
		}
	}
}

func TestIsValidEventType(t *testing.T) {
	validEvents := []string{
		"plan.complete",
		"design.complete",
		"implementation.done",
		"review.approved",
		"review.rejected",
		"task.blocked",
		"resolved",
		"task.complete",
	}

	for _, e := range validEvents {
		if !IsValidEventType(e) {
			t.Errorf("expected %q to be valid", e)
		}
	}

	invalidEvents := []string{
		"invalid",
		"task.started",
		"plan.started",
		"",
	}

	for _, e := range invalidEvents {
		if IsValidEventType(e) {
			t.Errorf("expected %q to be invalid", e)
		}
	}
}

func TestGetNextHat(t *testing.T) {
	tests := []struct {
		event   EventType
		nextHat string
	}{
		{EventPlanComplete, "designer"},
		{EventDesignComplete, "creator"},
		{EventImplementationDone, "critic"},
		{EventReviewApproved, "editor"},
		{EventReviewRejected, "creator"},
		{EventTaskBlocked, "resolver"},
		{EventResolved, "creator"},
		{EventTaskComplete, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := GetNextHat(tt.event)
			if got != tt.nextHat {
				t.Errorf("expected %q, got %q", tt.nextHat, got)
			}
		})
	}
}

func TestFormatScratchpad(t *testing.T) {
	s := Scratchpad{
		Understanding: "Building a blog",
		Plan:          "1. Create site\n2. Add theme",
		Decisions:     "Using Hugo",
		Blockers:      "",
		LastAction:    "Initialized project",
	}

	result := FormatScratchpad(s)

	if !contains(result, "## Current Understanding") {
		t.Error("expected '## Current Understanding' header")
	}
	if !contains(result, "Building a blog") {
		t.Error("expected understanding content")
	}
	if !contains(result, "## Current Plan") {
		t.Error("expected '## Current Plan' header")
	}
	if !contains(result, "## Key Decisions") {
		t.Error("expected '## Key Decisions' header")
	}
	if !contains(result, "## Last Action") {
		t.Error("expected '## Last Action' header")
	}
	// Blockers should be omitted when empty
	if contains(result, "## Blockers") {
		t.Error("expected no '## Blockers' header when empty")
	}
}

func TestFormatScratchpad_WithBlockers(t *testing.T) {
	s := Scratchpad{
		Understanding: "Building a blog",
		Plan:          "1. Create site",
		Decisions:     "",
		Blockers:      "Waiting for API key",
		LastAction:    "Paused",
	}

	result := FormatScratchpad(s)

	if !contains(result, "## Blockers") {
		t.Error("expected '## Blockers' header when blockers present")
	}
	if !contains(result, "Waiting for API key") {
		t.Error("expected blockers content")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
