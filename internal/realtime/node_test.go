package realtime

import (
	"context"
	"testing"
	"time"
)

func TestNewNode(t *testing.T) {
	node, err := NewNode(Config{})
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Start the node
	if err := node.Run(); err != nil {
		t.Fatalf("Failed to run node: %v", err)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := node.Shutdown(ctx); err != nil {
		t.Fatalf("Failed to shutdown node: %v", err)
	}
}

func TestRouteEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		payload   map[string]any
		expected  []string
	}{
		{
			name:      "task event routes to task channel",
			eventType: "task.created",
			payload:   map[string]any{"task_id": "123"},
			expected:  []string{"task:123"},
		},
		{
			name:      "task event with project routes to both channels",
			eventType: "task.updated",
			payload:   map[string]any{"task_id": "123", "project_id": "proj-1"},
			expected:  []string{"task:123", "project:proj-1"},
		},
		{
			name:      "session event routes to task channel",
			eventType: "session.started",
			payload:   map[string]any{"task_id": "456"},
			expected:  []string{"task:456"},
		},
		{
			name:      "activity event routes to task channel",
			eventType: "activity.new",
			payload:   map[string]any{"task_id": "789"},
			expected:  []string{"task:789"},
		},
		{
			name:      "quest event routes to quest channel",
			eventType: "quest.message",
			payload:   map[string]any{"quest_id": "q-1"},
			expected:  []string{"quest:q-1"},
		},
		{
			name:      "planning event routes to task channel",
			eventType: "planning.completed",
			payload:   map[string]any{"task_id": "t-1"},
			expected:  []string{"task:t-1"},
		},
		{
			name:      "checklist event routes to task channel",
			eventType: "checklist.updated",
			payload:   map[string]any{"task_id": "t-2"},
			expected:  []string{"task:t-2"},
		},
		{
			name:      "approval event routes to system channel",
			eventType: "approval.required",
			payload:   map[string]any{},
			expected:  []string{"system"},
		},
		{
			name:      "project event routes to project channel",
			eventType: "project.updated",
			payload:   map[string]any{"project_id": "p-1"},
			expected:  []string{"project:p-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channels := routeEvent(tt.eventType, tt.payload)

			if len(channels) != len(tt.expected) {
				t.Errorf("Expected %d channels, got %d: %v", len(tt.expected), len(channels), channels)
				return
			}

			for i, expected := range tt.expected {
				if channels[i] != expected {
					t.Errorf("Expected channel %s at index %d, got %s", expected, i, channels[i])
				}
			}
		})
	}
}

func TestCanSubscribe(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		channel  string
		expected bool
	}{
		{
			name:     "user can subscribe to own channel",
			userID:   "user-1",
			channel:  "user:user-1",
			expected: true,
		},
		{
			name:     "user cannot subscribe to other user channel",
			userID:   "user-1",
			channel:  "user:user-2",
			expected: false,
		},
		{
			name:     "user can subscribe to task channel",
			userID:   "user-1",
			channel:  "task:task-1",
			expected: true,
		},
		{
			name:     "user can subscribe to quest channel",
			userID:   "user-1",
			channel:  "quest:quest-1",
			expected: true,
		},
		{
			name:     "user can subscribe to project channel",
			userID:   "user-1",
			channel:  "project:proj-1",
			expected: true,
		},
		{
			name:     "user can subscribe to system channel",
			userID:   "user-1",
			channel:  "system",
			expected: true,
		},
		{
			name:     "invalid channel format rejected",
			userID:   "user-1",
			channel:  "invalidchannel",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := canSubscribe(tt.userID, tt.channel)
			if result != tt.expected {
				t.Errorf("canSubscribe(%q, %q) = %v, expected %v", tt.userID, tt.channel, result, tt.expected)
			}
		})
	}
}
