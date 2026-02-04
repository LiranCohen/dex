package realtime

import (
	"context"
	"testing"
	"time"
)

func TestWebSocketConnectionLifecycle(t *testing.T) {
	// Test node creation with history config
	t.Run("node creates with history config", func(t *testing.T) {
		cfg := Config{
			HistorySize: 50,
			HistoryTTL:  2 * time.Minute,
		}
		node, err := NewNode(cfg)
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}

		if node.historySize != 50 {
			t.Errorf("Expected historySize 50, got %d", node.historySize)
		}
		if node.historyTTL != 2*time.Minute {
			t.Errorf("Expected historyTTL 2m, got %v", node.historyTTL)
		}

		if err := node.Run(); err != nil {
			t.Fatalf("Failed to run node: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := node.Shutdown(ctx); err != nil {
			t.Fatalf("Failed to shutdown: %v", err)
		}
	})

	// Test default history config values
	t.Run("node uses default history config", func(t *testing.T) {
		node, err := NewNode(Config{})
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}

		if node.historySize != 100 {
			t.Errorf("Expected default historySize 100, got %d", node.historySize)
		}
		if node.historyTTL != 5*time.Minute {
			t.Errorf("Expected default historyTTL 5m, got %v", node.historyTTL)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := node.Shutdown(ctx); err != nil {
			t.Fatalf("Failed to shutdown: %v", err)
		}
	})
}

func TestEventRouting(t *testing.T) {
	// Test hat events route correctly
	t.Run("hat events route to task and project channels", func(t *testing.T) {
		payload := map[string]any{
			"task_id":    "task-123",
			"project_id": "proj-456",
			"source_hat": "implementer",
		}

		channels := routeEvent("hat.plan_complete", payload)

		// Should contain global, task, and project channels
		hasGlobal := false
		hasTask := false
		hasProject := false

		for _, ch := range channels {
			switch ch {
			case "global":
				hasGlobal = true
			case "task:task-123":
				hasTask = true
			case "project:proj-456":
				hasProject = true
			}
		}

		if !hasGlobal {
			t.Error("Hat event should route to global channel")
		}
		if !hasTask {
			t.Error("Hat event should route to task channel")
		}
		if !hasProject {
			t.Error("Hat event should route to project channel")
		}
	})

	// Test approval events route to user channel
	t.Run("approval events route to user channel", func(t *testing.T) {
		payload := map[string]any{
			"user_id":    "user-123",
			"project_id": "proj-456",
			"task_id":    "task-789",
		}

		channels := routeEvent("approval.required", payload)

		// Should contain global, system, user, project, and task channels
		hasGlobal := false
		hasSystem := false
		hasUser := false
		hasProject := false
		hasTask := false

		for _, ch := range channels {
			switch ch {
			case "global":
				hasGlobal = true
			case "system":
				hasSystem = true
			case "user:user-123":
				hasUser = true
			case "project:proj-456":
				hasProject = true
			case "task:task-789":
				hasTask = true
			}
		}

		if !hasGlobal {
			t.Error("Approval event should route to global channel")
		}
		if !hasSystem {
			t.Error("Approval event should route to system channel")
		}
		if !hasUser {
			t.Error("Approval event should route to user channel")
		}
		if !hasProject {
			t.Error("Approval event should route to project channel")
		}
		if !hasTask {
			t.Error("Approval event should route to task channel")
		}
	})
}

func TestPublishWithHistory(t *testing.T) {
	node, err := NewNode(Config{
		HistorySize: 10,
		HistoryTTL:  1 * time.Minute,
	})
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

	// Publish an event - this should succeed even without subscribers
	err = node.Publish("task.created", map[string]any{
		"task_id":    "test-task",
		"project_id": "test-project",
	})
	if err != nil {
		t.Errorf("Failed to publish event: %v", err)
	}
}
