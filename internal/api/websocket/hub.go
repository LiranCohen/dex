package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message represents a WebSocket event message
type Message struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
	TaskID    string         `json:"task_id,omitempty"`
	ProjectID string         `json:"project_id,omitempty"`
}

// ClientCommand represents a command from a client (subscribe, unsubscribe)
type ClientCommand struct {
	Action    string `json:"action"`     // "subscribe", "unsubscribe", "subscribe_all"
	TaskID    string `json:"task_id"`    // optional: subscribe to specific task
	ProjectID string `json:"project_id"` // optional: subscribe to specific project
}

// Client represents a connected WebSocket client
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	subscriptions map[string]bool // task_id, project_id, or "*" for all
	mu            sync.RWMutex
}

// Hub manages WebSocket connections and message broadcasting
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("websocket: failed to marshal message: %v", err)
				continue
			}

			var toDelete []*Client
			h.mu.RLock()
			for client := range h.clients {
				if h.matchesSubscription(client, msg) {
					select {
					case client.send <- data:
					default:
						// Client buffer full, mark for removal
						toDelete = append(toDelete, client)
					}
				}
			}
			h.mu.RUnlock()

			if len(toDelete) > 0 {
				h.mu.Lock()
				for _, client := range toDelete {
					if _, ok := h.clients[client]; ok {
						close(client.send)
						delete(h.clients, client)
					}
				}
				h.mu.Unlock()
			}
		}
	}
}

// Broadcast sends a message to all matching subscribers
func (h *Hub) Broadcast(msg Message) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	h.mu.RLock()
	clientCount := len(h.clients)
	h.mu.RUnlock()
	log.Printf("[WebSocket] Broadcasting %s to %d clients (task_id=%s)", msg.Type, clientCount, msg.TaskID)
	h.broadcast <- msg
}

// matchesSubscription checks if a client should receive a message
func (h *Hub) matchesSubscription(client *Client, msg Message) bool {
	client.mu.RLock()
	defer client.mu.RUnlock()

	// Check for "all" subscription
	if client.subscriptions["*"] {
		return true
	}

	// Check for task-specific subscription
	if msg.TaskID != "" && client.subscriptions["task:"+msg.TaskID] {
		return true
	}

	// Check for project-specific subscription
	if msg.ProjectID != "" && client.subscriptions["project:"+msg.ProjectID] {
		return true
	}

	return false
}

// newClient creates a new client instance
func newClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:           hub,
		conn:          conn,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket: unexpected close error: %v", err)
			}
			break
		}
		c.handleMessage(message)
	}
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send first message
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Drain any queued messages immediately (each as separate frame)
			n := len(c.send)
			for i := 0; i < n; i++ {
				if err := c.conn.WriteMessage(websocket.TextMessage, <-c.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming client commands
func (c *Client) handleMessage(data []byte) {
	var cmd ClientCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		log.Printf("websocket: failed to parse client command: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch cmd.Action {
	case "subscribe_all":
		c.subscriptions["*"] = true

	case "subscribe":
		if cmd.TaskID != "" {
			c.subscriptions["task:"+cmd.TaskID] = true
		}
		if cmd.ProjectID != "" {
			c.subscriptions["project:"+cmd.ProjectID] = true
		}

	case "unsubscribe":
		if cmd.TaskID != "" {
			delete(c.subscriptions, "task:"+cmd.TaskID)
		}
		if cmd.ProjectID != "" {
			delete(c.subscriptions, "project:"+cmd.ProjectID)
		}

	case "unsubscribe_all":
		c.subscriptions = make(map[string]bool)
	}
}
