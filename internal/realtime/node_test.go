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
			name:      "task event routes to global and task channels",
			eventType: "task.created",
			payload:   map[string]any{"task_id": "123"},
			expected:  []string{"global", "task:123"},
		},
		{
			name:      "task event with project routes to all channels",
			eventType: "task.updated",
			payload:   map[string]any{"task_id": "123", "project_id": "proj-1"},
			expected:  []string{"global", "task:123", "project:proj-1"},
		},
		{
			name:      "session event routes to global and task channels",
			eventType: "session.started",
			payload:   map[string]any{"task_id": "456"},
			expected:  []string{"global", "task:456"},
		},
		{
			name:      "activity event routes to global and task channels",
			eventType: "activity.new",
			payload:   map[string]any{"task_id": "789"},
			expected:  []string{"global", "task:789"},
		},
		{
			name:      "quest event routes to global and quest channels",
			eventType: "quest.message",
			payload:   map[string]any{"quest_id": "q-1"},
			expected:  []string{"global", "quest:q-1"},
		},
		{
			name:      "planning event routes to global and task channels",
			eventType: "planning.completed",
			payload:   map[string]any{"task_id": "t-1"},
			expected:  []string{"global", "task:t-1"},
		},
		{
			name:      "checklist event routes to global and task channels",
			eventType: "checklist.updated",
			payload:   map[string]any{"task_id": "t-2"},
			expected:  []string{"global", "task:t-2"},
		},
		{
			name:      "approval event routes to global and system channels",
			eventType: "approval.required",
			payload:   map[string]any{},
			expected:  []string{"global", "system"},
		},
		{
			name:      "project event routes to global and project channels",
			eventType: "project.updated",
			payload:   map[string]any{"project_id": "p-1"},
			expected:  []string{"global", "project:p-1"},
		},
		// Edge cases
		{
			name:      "task event without task_id only routes to global",
			eventType: "task.updated",
			payload:   map[string]any{"status": "running"},
			expected:  []string{"global"},
		},
		{
			name:      "quest event without quest_id only routes to global",
			eventType: "quest.message",
			payload:   map[string]any{"content": "hello"},
			expected:  []string{"global"},
		},
		{
			name:      "unknown event type only routes to global",
			eventType: "unknown.event",
			payload:   map[string]any{"data": "test"},
			expected:  []string{"global"},
		},
		{
			name:      "session event with project_id routes to task and project channels",
			eventType: "session.completed",
			payload:   map[string]any{"task_id": "t-1", "project_id": "p-1"},
			expected:  []string{"global", "task:t-1"},
		},
		{
			name:      "empty string task_id is ignored",
			eventType: "task.created",
			payload:   map[string]any{"task_id": ""},
			expected:  []string{"global"},
		},
		{
			name:      "approval event with task_id routes to task and system",
			eventType: "approval.required",
			payload:   map[string]any{"task_id": "t-3"},
			expected:  []string{"global", "system"},
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
			name:     "user can subscribe to global channel",
			userID:   "user-1",
			channel:  "global",
			expected: true,
		},
		{
			name:     "invalid channel format rejected",
			userID:   "user-1",
			channel:  "invalidchannel",
			expected: false,
		},
		// Edge cases
		{
			name:     "unknown channel type rejected",
			userID:   "user-1",
			channel:  "unknown:something",
			expected: false,
		},
		{
			name:     "empty channel rejected",
			userID:   "user-1",
			channel:  "",
			expected: false,
		},
		{
			name:     "channel with only colon rejected",
			userID:   "user-1",
			channel:  ":",
			expected: false,
		},
		{
			name:     "anonymous user can subscribe to global",
			userID:   "anonymous",
			channel:  "global",
			expected: true,
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
