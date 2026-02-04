package realtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewBroadcaster(t *testing.T) {
	t.Run("creates broadcaster with node", func(t *testing.T) {
		node, err := NewNode(Config{})
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}
		if err := node.Run(); err != nil {
			t.Fatalf("Failed to run node: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			node.Shutdown(ctx)
		}()

		b := NewBroadcaster(node)
		if b == nil {
			t.Fatal("Expected broadcaster to be non-nil")
		}
		if b.node != node {
			t.Error("Expected broadcaster node to match")
		}
	})

	t.Run("creates broadcaster with nil node", func(t *testing.T) {
		b := NewBroadcaster(nil)
		if b == nil {
			t.Fatal("Expected broadcaster to be non-nil")
		}
		if b.node != nil {
			t.Error("Expected broadcaster node to be nil")
		}
	})
}

func TestBroadcasterPublish(t *testing.T) {
	t.Run("adds timestamp if not present", func(t *testing.T) {
		b := NewBroadcaster(nil) // nil node so we don't actually publish

		payload := map[string]any{"key": "value"}
		before := time.Now().UTC()
		b.Publish("test.event", payload)
		after := time.Now().UTC()

		ts, ok := payload["timestamp"].(string)
		if !ok {
			t.Fatal("Expected timestamp to be added")
		}

		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			t.Fatalf("Failed to parse timestamp: %v", err)
		}

		if parsed.Before(before) || parsed.After(after) {
			t.Error("Timestamp out of expected range")
		}
	})

	t.Run("preserves existing timestamp", func(t *testing.T) {
		b := NewBroadcaster(nil)

		existingTS := "2024-01-15T10:30:00.000000000Z"
		payload := map[string]any{
			"key":       "value",
			"timestamp": existingTS,
		}
		b.Publish("test.event", payload)

		if payload["timestamp"] != existingTS {
			t.Error("Expected existing timestamp to be preserved")
		}
	})

	t.Run("handles nil node gracefully", func(t *testing.T) {
		b := NewBroadcaster(nil)

		// Should not panic
		payload := map[string]any{"key": "value"}
		b.Publish("test.event", payload)
	})
}

func TestBroadcasterPublishTaskEvent(t *testing.T) {
	t.Run("adds task_id to payload", func(t *testing.T) {
		b := NewBroadcaster(nil)

		payload := map[string]any{"status": "running"}
		b.PublishTaskEvent(EventTaskUpdated, "task-123", payload)

		if payload["task_id"] != "task-123" {
			t.Errorf("Expected task_id to be 'task-123', got %v", payload["task_id"])
		}
	})

	t.Run("creates payload if nil", func(t *testing.T) {
		b := NewBroadcaster(nil)

		// This should not panic
		b.PublishTaskEvent(EventTaskCreated, "task-456", nil)
	})

	t.Run("preserves existing payload fields", func(t *testing.T) {
		b := NewBroadcaster(nil)

		payload := map[string]any{
			"status":  "completed",
			"outcome": "success",
		}
		b.PublishTaskEvent(EventSessionCompleted, "task-789", payload)

		if payload["status"] != "completed" {
			t.Error("Expected status to be preserved")
		}
		if payload["outcome"] != "success" {
			t.Error("Expected outcome to be preserved")
		}
		if payload["task_id"] != "task-789" {
			t.Error("Expected task_id to be added")
		}
	})
}

func TestBroadcasterPublishQuestEvent(t *testing.T) {
	t.Run("adds quest_id to payload", func(t *testing.T) {
		b := NewBroadcaster(nil)

		payload := map[string]any{"content": "test message"}
		b.PublishQuestEvent(EventQuestMessage, "quest-abc", payload)

		if payload["quest_id"] != "quest-abc" {
			t.Errorf("Expected quest_id to be 'quest-abc', got %v", payload["quest_id"])
		}
	})

	t.Run("creates payload if nil", func(t *testing.T) {
		b := NewBroadcaster(nil)

		// This should not panic
		b.PublishQuestEvent(EventQuestCreated, "quest-xyz", nil)
	})
}

func TestEventConstants(t *testing.T) {
	// Verify event constants follow the expected naming pattern
	tests := []struct {
		constant string
		prefix   string
	}{
		// Task events
		{EventTaskCreated, "task."},
		{EventTaskUpdated, "task."},
		{EventTaskCancelled, "task."},
		{EventTaskPaused, "task."},
		{EventTaskResumed, "task."},
		{EventTaskUnblocked, "task."},
		{EventTaskAutoStarted, "task."},
		{EventTaskAutoStartFailed, "task."},
		// Session events
		{EventSessionKilled, "session."},
		{EventSessionStarted, "session."},
		{EventSessionIteration, "session."},
		{EventSessionCompleted, "session."},
		// Activity events
		{EventActivityNew, "activity."},
		// Quest events
		{EventQuestCreated, "quest."},
		{EventQuestUpdated, "quest."},
		{EventQuestDeleted, "quest."},
		{EventQuestCompleted, "quest."},
		{EventQuestReopened, "quest."},
		{EventQuestContentDelta, "quest."},
		{EventQuestToolCall, "quest."},
		{EventQuestToolResult, "quest."},
		{EventQuestMessage, "quest."},
		{EventQuestObjectiveDraft, "quest."},
		{EventQuestQuestion, "quest."},
		{EventQuestReady, "quest."},
		// Planning events
		{EventPlanningStarted, "planning."},
		{EventPlanningUpdated, "planning."},
		{EventPlanningCompleted, "planning."},
		{EventPlanningSkipped, "planning."},
		// Checklist events
		{EventChecklistUpdated, "checklist."},
		// Approval events
		{EventApprovalRequired, "approval."},
		{EventApprovalResolved, "approval."},
	}

	for _, tt := range tests {
		t.Run(tt.constant, func(t *testing.T) {
			if !strings.HasPrefix(tt.constant, tt.prefix) {
				t.Errorf("Event constant %q should start with %q", tt.constant, tt.prefix)
			}
		})
	}
}

func TestEventConstantsAreUnique(t *testing.T) {
	events := []string{
		EventTaskCreated, EventTaskUpdated, EventTaskCancelled,
		EventTaskPaused, EventTaskResumed, EventTaskUnblocked,
		EventTaskAutoStarted, EventTaskAutoStartFailed,
		EventSessionKilled, EventSessionStarted, EventSessionIteration, EventSessionCompleted,
		EventActivityNew,
		EventQuestCreated, EventQuestUpdated, EventQuestDeleted, EventQuestCompleted,
		EventQuestReopened, EventQuestContentDelta, EventQuestToolCall, EventQuestToolResult,
		EventQuestMessage, EventQuestObjectiveDraft, EventQuestQuestion, EventQuestReady,
		EventPlanningStarted, EventPlanningUpdated, EventPlanningCompleted, EventPlanningSkipped,
		EventChecklistUpdated,
		EventApprovalRequired, EventApprovalResolved,
	}

	seen := make(map[string]bool)
	for _, event := range events {
		if seen[event] {
			t.Errorf("Duplicate event constant: %s", event)
		}
		seen[event] = true
	}
}
