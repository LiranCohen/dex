package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifuge"
)

// Node wraps a Centrifuge node for real-time messaging
type Node struct {
	node *centrifuge.Node
}

// Config holds configuration for the realtime node
type Config struct {
	// ClientQueueMaxSize is the max bytes to buffer per client before disconnect (default 2MB)
	ClientQueueMaxSize int
	// ClientChannelLimit is max channels per client (default 128)
	ClientChannelLimit int
}

// NewNode creates a new Centrifuge node with the given configuration
func NewNode(cfg Config) (*Node, error) {
	if cfg.ClientQueueMaxSize == 0 {
		cfg.ClientQueueMaxSize = 2 * 1024 * 1024 // 2MB
	}
	if cfg.ClientChannelLimit == 0 {
		cfg.ClientChannelLimit = 128
	}

	node, err := centrifuge.New(centrifuge.Config{
		LogLevel:           centrifuge.LogLevelInfo,
		ClientQueueMaxSize: cfg.ClientQueueMaxSize,
		ClientChannelLimit: cfg.ClientChannelLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create centrifuge node: %w", err)
	}

	n := &Node{node: node}
	n.setupHandlers()

	return n, nil
}

// setupHandlers configures the Centrifuge event handlers
func (n *Node) setupHandlers() {
	// OnConnecting is called before the client is fully connected
	// Credentials are set via HTTP middleware context before this
	n.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		cred, ok := centrifuge.GetCredentials(ctx)
		if !ok {
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnauthorized
		}

		fmt.Printf("[Realtime] Client connecting: user=%s\n", cred.UserID)

		// Return credentials and auto-subscribe to user's personal channel
		return centrifuge.ConnectReply{
			Credentials: cred,
			Subscriptions: map[string]centrifuge.SubscribeOptions{
				"user:" + cred.UserID: {},
			},
		}, nil
	})

	// OnConnect is called after successful connection - set up client handlers here
	n.node.OnConnect(func(client *centrifuge.Client) {
		fmt.Printf("[Realtime] Client connected: user=%s\n", client.UserID())

		// Subscription handler - called when client subscribes to a channel
		client.OnSubscribe(func(e centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			fmt.Printf("[Realtime] Subscribe: client=%s channel=%s\n", client.UserID(), e.Channel)

			// Validate channel access
			if !canSubscribe(client.UserID(), e.Channel) {
				cb(centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied)
				return
			}

			cb(centrifuge.SubscribeReply{}, nil)
		})

		// Unsubscribe handler
		client.OnUnsubscribe(func(e centrifuge.UnsubscribeEvent) {
			fmt.Printf("[Realtime] Unsubscribe: client=%s channel=%s\n", client.UserID(), e.Channel)
		})

		// Disconnect handler
		client.OnDisconnect(func(e centrifuge.DisconnectEvent) {
			fmt.Printf("[Realtime] Client disconnected: user=%s reason=%s\n", client.UserID(), e.Reason)
		})
	})
}

// Run starts the Centrifuge node
func (n *Node) Run() error {
	return n.node.Run()
}

// Shutdown gracefully stops the node
func (n *Node) Shutdown(ctx context.Context) error {
	return n.node.Shutdown(ctx)
}

// WebSocketHandler returns an HTTP handler for WebSocket connections
func (n *Node) WebSocketHandler() http.Handler {
	return centrifuge.NewWebsocketHandler(n.node, centrifuge.WebsocketConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	})
}

// Publish sends an event to the appropriate channel(s)
func (n *Node) Publish(eventType string, payload map[string]any) error {
	// Add event metadata
	payload["type"] = eventType
	payload["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Route to appropriate channel(s)
	channels := routeEvent(eventType, payload)
	for _, channel := range channels {
		if _, err := n.node.Publish(channel, data); err != nil {
			fmt.Printf("[Realtime] Failed to publish to %s: %v\n", channel, err)
		}
	}

	return nil
}

// PublishToChannel sends data directly to a specific channel
func (n *Node) PublishToChannel(channel string, data []byte) error {
	_, err := n.node.Publish(channel, data)
	return err
}

// canSubscribe checks if a user can subscribe to a channel
func canSubscribe(userID, channel string) bool {
	// Handle global channel (receives all events)
	if channel == "global" {
		return true
	}

	// Handle system channel (no ":" delimiter)
	if channel == "system" {
		return true
	}

	// Parse channel type
	parts := strings.SplitN(channel, ":", 2)
	if len(parts) != 2 {
		// Invalid channel format
		return false
	}

	channelType := parts[0]

	switch channelType {
	case "user":
		// Users can only subscribe to their own user channel
		return parts[1] == userID
	case "task", "quest", "project":
		// For now, allow all authenticated users to subscribe
		// TODO: Implement proper authorization based on project membership
		return true
	default:
		return false
	}
}

// routeEvent determines which channel(s) an event should be published to
func routeEvent(eventType string, payload map[string]any) []string {
	// Always publish to global channel for clients that want all events
	channels := []string{"global"}

	// Route to specific channels based on event type prefix
	switch {
	case strings.HasPrefix(eventType, "task."),
		strings.HasPrefix(eventType, "session."),
		strings.HasPrefix(eventType, "activity."),
		strings.HasPrefix(eventType, "planning."),
		strings.HasPrefix(eventType, "checklist."):
		// These events go to task channel
		if taskID, ok := payload["task_id"].(string); ok && taskID != "" {
			channels = append(channels, "task:"+taskID)
		}

	case strings.HasPrefix(eventType, "quest."):
		// Quest events go to quest channel
		if questID, ok := payload["quest_id"].(string); ok && questID != "" {
			channels = append(channels, "quest:"+questID)
		}

	case strings.HasPrefix(eventType, "approval."):
		// Approvals also go to system channel
		channels = append(channels, "system")

	case strings.HasPrefix(eventType, "project."):
		if projectID, ok := payload["project_id"].(string); ok && projectID != "" {
			channels = append(channels, "project:"+projectID)
		}
	}

	// Also publish task events to project channel for list views
	if strings.HasPrefix(eventType, "task.") {
		if projectID, ok := payload["project_id"].(string); ok && projectID != "" {
			channels = append(channels, "project:"+projectID)
		}
	}

	return channels
}
