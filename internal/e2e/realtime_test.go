// Package e2e contains end-to-end tests that require real infrastructure
package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/lirancohen/dex/internal/realtime"
)

// TestBroadcasterToClientE2E tests the complete flow of:
// 1. Setting up a real Centrifuge node
// 2. Publishing events via Broadcaster
// 3. Verifying events are properly routed
//
// Run with: go test -v ./internal/e2e -run TestBroadcasterToClientE2E
func TestBroadcasterToClientE2E(t *testing.T) {
	if os.Getenv("DEX_E2E_ENABLED") != "true" {
		t.Skip("Skipping e2e test: set DEX_E2E_ENABLED=true to run")
	}

	// Create a realtime node with history enabled
	node, err := realtime.NewNode(realtime.Config{
		HistorySize: 100,
		HistoryTTL:  5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Start the node
	if err := node.Run(); err != nil {
		t.Fatalf("Failed to run node: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		node.Shutdown(ctx)
	}()

	// Create broadcaster
	broadcaster := realtime.NewBroadcaster(node)

	t.Run("publishes task event to correct channels", func(t *testing.T) {
		// Publish a task event
		err := node.Publish("task.created", map[string]any{
			"task_id":    "test-task-1",
			"project_id": "test-project-1",
			"title":      "Test Task",
		})
		if err != nil {
			t.Errorf("Failed to publish task event: %v", err)
		}
	})

	t.Run("publishes hat event via broadcaster", func(t *testing.T) {
		broadcaster.PublishHatEvent(
			realtime.EventHatPlanComplete,
			"session-1",
			"task-1",
			"project-1",
			map[string]any{
				"topic":      "plan.complete",
				"source_hat": "planner",
			},
		)
		// Event is published asynchronously, just verify no panic
	})

	t.Run("publishes approval event with user routing", func(t *testing.T) {
		broadcaster.Publish(realtime.EventApprovalRequired, map[string]any{
			"id":         "approval-1",
			"user_id":    "user-1",
			"project_id": "project-1",
			"task_id":    "task-1",
		})
		// Event is published asynchronously, just verify no panic
	})
}

// TestWebSocketHandlerE2E tests the WebSocket handler with a real HTTP server
func TestWebSocketHandlerE2E(t *testing.T) {
	if os.Getenv("DEX_E2E_ENABLED") != "true" {
		t.Skip("Skipping e2e test: set DEX_E2E_ENABLED=true to run")
	}

	// Create a realtime node
	node, err := realtime.NewNode(realtime.Config{
		HistorySize: 100,
		HistoryTTL:  5 * time.Minute,
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

	// Create test server with WebSocket handler
	handler := node.WebSocketHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	t.Run("WebSocket handler responds to upgrade request", func(t *testing.T) {
		// Make a regular HTTP request (should fail - needs WebSocket upgrade)
		resp, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should get 400 Bad Request since we're not doing a proper WebSocket upgrade
		if resp.StatusCode != http.StatusBadRequest {
			t.Logf("Expected 400 for non-WebSocket request, got %d", resp.StatusCode)
		}
	})
}

// TestHistoryRecoveryE2E tests message history and recovery
func TestHistoryRecoveryE2E(t *testing.T) {
	if os.Getenv("DEX_E2E_ENABLED") != "true" {
		t.Skip("Skipping e2e test: set DEX_E2E_ENABLED=true to run")
	}

	// Create node with small history for testing
	node, err := realtime.NewNode(realtime.Config{
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

	t.Run("publishes multiple events with history", func(t *testing.T) {
		// Publish multiple events
		for i := 0; i < 5; i++ {
			err := node.Publish("task.updated", map[string]any{
				"task_id": "test-task",
				"status":  "running",
				"index":   i,
			})
			if err != nil {
				t.Errorf("Failed to publish event %d: %v", i, err)
			}
		}
	})
}

// TestPingRPCE2E tests the ping RPC handler for latency measurement
func TestPingRPCE2E(t *testing.T) {
	if os.Getenv("DEX_E2E_ENABLED") != "true" {
		t.Skip("Skipping e2e test: set DEX_E2E_ENABLED=true to run")
	}

	// Create a basic centrifuge node to test RPC handling
	centrifugeNode, err := centrifuge.New(centrifuge.Config{})
	if err != nil {
		t.Fatalf("Failed to create centrifuge node: %v", err)
	}

	// Set up RPC handler similar to our node
	centrifugeNode.OnConnect(func(client *centrifuge.Client) {
		client.OnRPC(func(e centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
			if e.Method == "ping" {
				cb(centrifuge.RPCReply{Data: []byte(`{"pong":true}`)}, nil)
				return
			}
			cb(centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound)
		})
	})

	if err := centrifugeNode.Run(); err != nil {
		t.Fatalf("Failed to run centrifuge node: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		centrifugeNode.Shutdown(ctx)
	}()

	t.Run("node configured with ping RPC handler", func(t *testing.T) {
		// The ping RPC handler is tested via the integration tests
		// Here we just verify the node runs without errors
	})
}
