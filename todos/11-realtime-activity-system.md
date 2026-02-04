# Real-Time Event System with Centrifuge

**Priority**: High
**Effort**: Medium-High
**Impact**: High

## Overview

Replace the current hand-rolled WebSocket hub with [Centrifuge](https://github.com/centrifugal/centrifuge), a mature Go library for real-time messaging. This provides per-client queues, configurable backpressure, channel-based routing, and better scalability.

This document covers **all** real-time eventing in Dex, not just activity updates.

---

## Current State

### Existing Infrastructure

**Backend** (`internal/api/websocket/`):
- `hub.go` - Central pub/sub hub with 256-message broadcast channel
- `handler.go` - WebSocket upgrade at `/api/v1/ws`
- Subscription types: `*` (all), `task:<id>`, `project:<id>`
- 30-second ping interval, 10-second write timeout

**Frontend** (`frontend/src/hooks/useWebSocket.ts`):
- Single global WebSocket connection
- Reconnection with exponential backoff (max 5 attempts)
- Handler-based pub/sub for components

### Current Event Inventory (42 Broadcast Locations)

| Category | Events | Source Files |
|----------|--------|--------------|
| **Task Lifecycle** | `task.created`, `task.updated`, `task.completed`, `task.cancelled`, `task.unblocked`, `task.auto_started`, `task.auto_start_failed` | `task_orchestration.go`, `quest/handler.go`, `github/sync.go` |
| **Session** | `session.started`, `session.iteration`, `session.completed` | `session/ralph.go` |
| **Activity** | `activity.new` (user_message, assistant_response, tool_call, tool_result, hat_transition, checklist_update, etc.) | `session/activity.go` |
| **Quest Conversation** | `quest.content_delta`, `quest.tool_call`, `quest.tool_result`, `quest.message`, `quest.objective_draft`, `quest.question`, `quest.ready` | `quest/handler.go` |
| **Planning** | `planning.started`, `planning.updated`, `planning.completed`, `planning.skipped` | `planning/planner.go`, `handlers/planning/` |
| **Checklist** | `checklist.updated` | `session/activity.go`, `handlers/planning/checklist.go` |
| **Approval** | `approval.required`, `approval.resolved` | `session/ralph.go`, `handlers/approvals/` |

### Known Issues

1. **Shared broadcast channel** - One slow client blocks all
2. **O(n) subscription matching** - Iterates all clients per message
3. **No backpressure control** - Fixed 256-message buffer, then disconnect
4. **Tool call/result pairing bug** - Uses tool name instead of tool_use_id
5. **Token in WebSocket URL** - Security concern

---

## Centrifuge Architecture

### Why Centrifuge (Library) vs Centrifugo (Server)

**Centrifuge** embeds directly into your Go service - no separate process:

```
┌─────────────────────────────────────────┐
│  Dex Server                             │
│                                         │
│  ┌───────────────────────────────────┐  │
│  │  Centrifuge Node (embedded)       │  │
│  │  - Per-client queues              │  │
│  │  - Channel routing                │  │
│  │  - Backpressure handling          │  │
│  └───────────────────────────────────┘  │
│                                         │
│  ┌───────────────────────────────────┐  │
│  │  Existing HTTP/REST handlers      │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

### Bidirectional Mode

Using bidirectional mode (not unidirectional) because:
1. Quest conversation requires streaming content deltas
2. Future features may need client→server commands
3. Centrifuge client SDK handles reconnection, subscription management

### Channel Structure

```
Channel Pattern              Purpose                          Subscribers
─────────────────────────────────────────────────────────────────────────
task:{task_id}               Task-specific events             ObjectiveDetail page
quest:{quest_id}             Quest conversation events        QuestDetail page
project:{project_id}         Project-wide events              Home, AllObjectives
user:{user_id}               User-specific notifications      Global (approvals)
system                       System-wide broadcasts           All clients
```

### Event→Channel Mapping

```go
// Event routing rules
var channelRouting = map[string]func(payload map[string]any) string{
    // Task events → task channel
    "task.created":          func(p) { return "task:" + p["task_id"] },
    "task.updated":          func(p) { return "task:" + p["task_id"] },
    "task.completed":        func(p) { return "task:" + p["task_id"] },
    "task.cancelled":        func(p) { return "task:" + p["task_id"] },
    "task.unblocked":        func(p) { return "task:" + p["task_id"] },
    "task.auto_started":     func(p) { return "task:" + p["task_id"] },

    // Session events → task channel (sessions belong to tasks)
    "session.started":       func(p) { return "task:" + p["task_id"] },
    "session.iteration":     func(p) { return "task:" + p["task_id"] },
    "session.completed":     func(p) { return "task:" + p["task_id"] },

    // Activity events → task channel
    "activity.new":          func(p) { return "task:" + p["task_id"] },

    // Checklist events → task channel
    "checklist.updated":     func(p) { return "task:" + p["task_id"] },

    // Quest events → quest channel
    "quest.content_delta":   func(p) { return "quest:" + p["quest_id"] },
    "quest.tool_call":       func(p) { return "quest:" + p["quest_id"] },
    "quest.tool_result":     func(p) { return "quest:" + p["quest_id"] },
    "quest.message":         func(p) { return "quest:" + p["quest_id"] },
    "quest.objective_draft": func(p) { return "quest:" + p["quest_id"] },
    "quest.question":        func(p) { return "quest:" + p["quest_id"] },
    "quest.ready":           func(p) { return "quest:" + p["quest_id"] },

    // Planning events → task channel
    "planning.started":      func(p) { return "task:" + p["task_id"] },
    "planning.updated":      func(p) { return "task:" + p["task_id"] },
    "planning.completed":    func(p) { return "task:" + p["task_id"] },
    "planning.skipped":      func(p) { return "task:" + p["task_id"] },

    // Approval events → user channel (global notifications)
    "approval.required":     func(p) { return "user:" + p["user_id"] },
    "approval.resolved":     func(p) { return "user:" + p["user_id"] },

    // Project-wide events
    "project.updated":       func(p) { return "project:" + p["project_id"] },
}
```

---

## Implementation Plan

### Phase 1: Core Infrastructure

#### 1.1 Add Centrifuge Dependency

```bash
go get github.com/centrifugal/centrifuge
```

#### 1.2 Create Centrifuge Node

**New file**: `internal/realtime/node.go`

```go
package realtime

import (
    "context"
    "encoding/json"
    "log"
    "time"

    "github.com/centrifugal/centrifuge"
)

type Node struct {
    node *centrifuge.Node
}

type Config struct {
    // Client queue size before disconnect (default 2MB)
    ClientQueueMaxSize int
    // Write timeout for slow clients (default 5s)
    WriteTimeout time.Duration
}

func NewNode(cfg Config) (*Node, error) {
    if cfg.ClientQueueMaxSize == 0 {
        cfg.ClientQueueMaxSize = 2 * 1024 * 1024 // 2MB
    }
    if cfg.WriteTimeout == 0 {
        cfg.WriteTimeout = 5 * time.Second
    }

    node, err := centrifuge.New(centrifuge.Config{
        LogLevel:           centrifuge.LogLevelInfo,
        ClientQueueMaxSize: cfg.ClientQueueMaxSize,
    })
    if err != nil {
        return nil, err
    }

    // Connection handler
    node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
        // Extract user/auth from context (set by HTTP middleware)
        cred, ok := centrifuge.GetCredentials(ctx)
        if !ok {
            return centrifuge.ConnectReply{}, centrifuge.DisconnectUnauthorized
        }

        log.Printf("[Realtime] Client connected: user=%s", cred.UserID)

        return centrifuge.ConnectReply{
            // Auto-subscribe to user's personal channel
            Subscriptions: map[string]centrifuge.SubscribeOptions{
                "user:" + cred.UserID: {},
            },
        }, nil
    })

    // Subscription handler
    node.OnSubscribe(func(client *centrifuge.Client, e centrifuge.SubscribeEvent) (centrifuge.SubscribeReply, error) {
        log.Printf("[Realtime] Subscribe: client=%s channel=%s", client.UserID(), e.Channel)

        // Validate channel access (implement authorization)
        if !canSubscribe(client.UserID(), e.Channel) {
            return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
        }

        return centrifuge.SubscribeReply{}, nil
    })

    // Disconnect handler
    node.OnDisconnect(func(client *centrifuge.Client, e centrifuge.DisconnectEvent) {
        log.Printf("[Realtime] Client disconnected: user=%s reason=%s", client.UserID(), e.Reason)
    })

    return &Node{node: node}, nil
}

func (n *Node) Run() error {
    return n.node.Run()
}

func (n *Node) Shutdown(ctx context.Context) error {
    return n.node.Shutdown(ctx)
}

// WebSocketHandler returns the HTTP handler for WebSocket connections
func (n *Node) WebSocketHandler(cfg centrifuge.WebsocketConfig) http.Handler {
    return centrifuge.NewWebsocketHandler(n.node, cfg)
}

// Publish sends an event to the appropriate channel(s)
func (n *Node) Publish(eventType string, payload map[string]any) error {
    // Add event metadata
    payload["type"] = eventType
    payload["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)

    data, err := json.Marshal(payload)
    if err != nil {
        return err
    }

    // Route to appropriate channel(s)
    channels := routeEvent(eventType, payload)
    for _, channel := range channels {
        if _, err := n.node.Publish(channel, data); err != nil {
            log.Printf("[Realtime] Failed to publish to %s: %v", channel, err)
        }
    }

    return nil
}

// canSubscribe checks if a user can subscribe to a channel
func canSubscribe(userID, channel string) bool {
    // Parse channel type and ID
    // For now, allow all subscriptions (implement proper auth later)
    return true
}

// routeEvent determines which channel(s) an event should be published to
func routeEvent(eventType string, payload map[string]any) []string {
    var channels []string

    // Primary channel based on event type
    switch {
    case strings.HasPrefix(eventType, "task."):
        if taskID, ok := payload["task_id"].(string); ok {
            channels = append(channels, "task:"+taskID)
        }
    case strings.HasPrefix(eventType, "session."):
        if taskID, ok := payload["task_id"].(string); ok {
            channels = append(channels, "task:"+taskID)
        }
    case strings.HasPrefix(eventType, "activity."):
        if taskID, ok := payload["task_id"].(string); ok {
            channels = append(channels, "task:"+taskID)
        }
    case strings.HasPrefix(eventType, "quest."):
        if questID, ok := payload["quest_id"].(string); ok {
            channels = append(channels, "quest:"+questID)
        }
    case strings.HasPrefix(eventType, "planning."):
        if taskID, ok := payload["task_id"].(string); ok {
            channels = append(channels, "task:"+taskID)
        }
    case strings.HasPrefix(eventType, "checklist."):
        if taskID, ok := payload["task_id"].(string); ok {
            channels = append(channels, "task:"+taskID)
        }
    case strings.HasPrefix(eventType, "approval."):
        // Approvals go to user channel (implement user lookup)
        channels = append(channels, "system")
    case strings.HasPrefix(eventType, "project."):
        if projectID, ok := payload["project_id"].(string); ok {
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
```

#### 1.3 Authentication Middleware

**New file**: `internal/realtime/auth.go`

```go
package realtime

import (
    "context"
    "net/http"

    "github.com/centrifugal/centrifuge"
)

// AuthMiddleware extracts auth from request and sets Centrifuge credentials
func AuthMiddleware(authService AuthService) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract token from query param or header
            token := r.URL.Query().Get("token")
            if token == "" {
                token = r.Header.Get("Authorization")
            }

            // Validate token and get user
            user, err := authService.ValidateToken(r.Context(), token)
            if err != nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Set Centrifuge credentials
            cred := &centrifuge.Credentials{
                UserID: user.ID,
                Info:   []byte(`{}`), // Optional user info
            }
            ctx := centrifuge.SetCredentials(r.Context(), cred)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

#### 1.4 Wire into Server

**Update**: `internal/api/server.go`

```go
import "github.com/lirancohen/dex/internal/realtime"

type Server struct {
    // ... existing fields

    // Replace hub with realtime node
    realtime *realtime.Node
}

func NewServer(...) (*Server, error) {
    // ... existing setup

    // Initialize Centrifuge node
    rtNode, err := realtime.NewNode(realtime.Config{
        ClientQueueMaxSize: 2 * 1024 * 1024, // 2MB
        WriteTimeout:       5 * time.Second,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create realtime node: %w", err)
    }

    s.realtime = rtNode

    // Start the node
    if err := rtNode.Run(); err != nil {
        return nil, fmt.Errorf("failed to start realtime node: %w", err)
    }

    return s, nil
}

func (s *Server) setupRoutes() {
    // ... existing routes

    // WebSocket endpoint with auth middleware
    wsHandler := s.realtime.WebSocketHandler(centrifuge.WebsocketConfig{
        WriteTimeout: 5 * time.Second,
    })
    s.router.Handle("/api/v1/ws", realtime.AuthMiddleware(s.authService)(wsHandler))
}
```

---

### Phase 2: Migrate Broadcast Calls

#### 2.1 Create Publisher Interface

**New file**: `internal/realtime/publisher.go`

```go
package realtime

// Publisher is the interface for publishing events
type Publisher interface {
    Publish(eventType string, payload map[string]any) error
}

// Ensure Node implements Publisher
var _ Publisher = (*Node)(nil)
```

#### 2.2 Update All Broadcast Locations

Replace all `hub.Broadcast(websocket.Message{...})` calls with `realtime.Publish(...)`.

**Example - Quest Handler** (`internal/quest/handler.go`):

```go
// Before
if h.hub != nil {
    h.hub.Broadcast(websocket.Message{
        Type: "quest.content_delta",
        Payload: map[string]any{
            "quest_id": questID,
            "delta":    delta,
            "content":  streamedContent.String(),
        },
    })
}

// After
if h.publisher != nil {
    h.publisher.Publish("quest.content_delta", map[string]any{
        "quest_id": questID,
        "delta":    delta,
        "content":  streamedContent.String(),
    })
}
```

**Files to update** (42 total broadcast locations):

| File | Broadcasts | Events |
|------|------------|--------|
| `internal/quest/handler.go` | 8 | quest.*, task.created, task.cancelled |
| `internal/session/ralph.go` | 6 | session.*, approval.required, activity.* |
| `internal/api/task_orchestration.go` | 4 | task.updated, task.unblocked, task.auto_* |
| `internal/planning/planner.go` | 4 | planning.* |
| `internal/api/handlers/planning/checklist.go` | 3 | checklist.updated, task.updated |
| `internal/api/handlers/planning/handlers.go` | 2 | planning.* |
| `internal/api/handlers/approvals/handlers.go` | 2 | approval.resolved |
| `internal/api/handlers/quests/objectives.go` | 2 | quest.message, task.created |
| `internal/api/handlers/quests/handlers.go` | 5 | quest.*, task.* |
| `internal/api/handlers/sessions/handlers.go` | 4 | session.* |
| `internal/api/handlers/github/sync.go` | 3 | task.unblocked, task.auto_* |
| `internal/session/activity.go` | 2 | activity.new, checklist.updated |

---

### Phase 3: Frontend Migration

#### 3.1 Install Centrifuge JS Client

```bash
cd frontend
npm install centrifuge
```

#### 3.2 Create Centrifuge Hook

**New file**: `frontend/src/hooks/useCentrifuge.ts`

```typescript
import { createContext, useContext, useEffect, useRef, useState, useCallback } from 'react';
import { Centrifuge, Subscription, PublicationContext } from 'centrifuge';
import { useAuthStore } from '../stores/auth';

interface CentrifugeContextType {
  connected: boolean;
  subscribe: (channel: string, handler: (ctx: PublicationContext) => void) => () => void;
}

const CentrifugeContext = createContext<CentrifugeContextType | null>(null);

export function CentrifugeProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = useState(false);
  const clientRef = useRef<Centrifuge | null>(null);
  const subscriptionsRef = useRef<Map<string, Subscription>>(new Map());

  const token = useAuthStore((state) => state.token);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  useEffect(() => {
    if (!isAuthenticated || !token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws`;

    const client = new Centrifuge(wsUrl, {
      token,
      // Reconnection settings
      minReconnectDelay: 1000,
      maxReconnectDelay: 10000,
    });

    client.on('connected', () => {
      console.log('[Centrifuge] Connected');
      setConnected(true);
    });

    client.on('disconnected', (ctx) => {
      console.log('[Centrifuge] Disconnected:', ctx.reason);
      setConnected(false);
    });

    client.on('error', (ctx) => {
      console.error('[Centrifuge] Error:', ctx.error);
    });

    client.connect();
    clientRef.current = client;

    return () => {
      // Cleanup subscriptions
      subscriptionsRef.current.forEach((sub) => sub.unsubscribe());
      subscriptionsRef.current.clear();
      client.disconnect();
      clientRef.current = null;
    };
  }, [isAuthenticated, token]);

  const subscribe = useCallback((channel: string, handler: (ctx: PublicationContext) => void) => {
    const client = clientRef.current;
    if (!client) {
      console.warn('[Centrifuge] Cannot subscribe, client not connected');
      return () => {};
    }

    // Check if already subscribed
    let sub = subscriptionsRef.current.get(channel);
    if (!sub) {
      sub = client.newSubscription(channel);
      subscriptionsRef.current.set(channel, sub);
      sub.subscribe();
    }

    // Add publication handler
    const listener = sub.on('publication', handler);

    // Return unsubscribe function
    return () => {
      listener(); // Remove this handler
      // Note: Don't unsubscribe from channel - other handlers may exist
    };
  }, []);

  return (
    <CentrifugeContext.Provider value={{ connected, subscribe }}>
      {children}
    </CentrifugeContext.Provider>
  );
}

export function useCentrifuge() {
  const ctx = useContext(CentrifugeContext);
  if (!ctx) {
    throw new Error('useCentrifuge must be used within CentrifugeProvider');
  }
  return ctx;
}

// Convenience hook for subscribing to a channel
export function useChannel(channel: string | null, handler: (data: any) => void) {
  const { subscribe } = useCentrifuge();

  useEffect(() => {
    if (!channel) return;

    return subscribe(channel, (ctx) => {
      const data = JSON.parse(new TextDecoder().decode(ctx.data));
      handler(data);
    });
  }, [channel, handler, subscribe]);
}
```

#### 3.3 Update Pages to Use Channels

**Example - ObjectiveDetail.tsx**:

```typescript
import { useChannel } from '../../hooks/useCentrifuge';

export function ObjectiveDetail() {
  const { id } = useParams<{ id: string }>();

  // Subscribe to task-specific channel
  useChannel(id ? `task:${id}` : null, useCallback((event) => {
    switch (event.type) {
      case 'activity.new':
        setActivity((prev) => [...prev, event.activity]);
        break;
      case 'checklist.updated':
        // Update checklist item
        break;
      case 'session.iteration':
        setContextStatus(event.context);
        break;
      case 'session.completed':
        loadData();
        break;
      // ... other events
    }
  }, [loadData]));

  // ... rest of component
}
```

**Example - QuestDetail.tsx** (streaming):

```typescript
export function QuestDetail() {
  const { id } = useParams<{ id: string }>();
  const [streamingContent, setStreamingContent] = useState('');

  // Subscribe to quest channel for streaming
  useChannel(id ? `quest:${id}` : null, useCallback((event) => {
    switch (event.type) {
      case 'quest.content_delta':
        setStreamingContent(event.content);
        break;
      case 'quest.message':
        // Final message received
        setMessages((prev) => [...prev, event.message]);
        setStreamingContent('');
        break;
      case 'quest.tool_call':
        // Show tool execution
        break;
      // ... other events
    }
  }, []));
}
```

---

### Phase 4: Activity System Fixes

#### 4.1 Add tool_use_id to Activity Schema

```sql
-- Migration
ALTER TABLE session_activity ADD COLUMN tool_use_id TEXT;
CREATE INDEX idx_session_activity_tool_use_id ON session_activity(tool_use_id);
```

**Update** `internal/db/activity.go`:

```go
type SessionActivity struct {
    // ... existing fields
    ToolUseID sql.NullString // NEW: Claude API's tool_use_id for pairing
}

func (db *DB) CreateSessionActivity(
    sessionID string,
    iteration int,
    eventType string,
    hat string,
    content string,
    tokensInput, tokensOutput *int,
    toolUseID string,  // NEW parameter
) (*SessionActivity, error) {
    // ... include tool_use_id in INSERT
}
```

#### 4.2 Update ActivityRecorder

**Update** `internal/session/activity.go`:

```go
// RecordToolCall now includes tool_use_id
func (r *ActivityRecorder) RecordToolCall(iteration int, toolUseID, toolName string, input any) error {
    data := ToolCallData{
        ToolUseID: toolUseID,  // NEW
        Name:      toolName,
        Input:     input,
    }
    // ...
}

// RecordToolResult matches by tool_use_id
func (r *ActivityRecorder) RecordToolResult(iteration int, toolUseID, toolName string, result any) error {
    data := ToolResultData{
        ToolUseID: toolUseID,  // NEW
        Name:      toolName,
        Result:    result,
    }
    // ...
}
```

#### 4.3 Update Frontend ActivityLog

**Update** `frontend/src/app/components/ActivityLog.tsx`:

```typescript
// Group by tool_use_id instead of tool name
function groupActivities(items: Activity[]): DisplayItem[] {
  const pendingToolCalls = new Map<string, Activity>(); // tool_use_id -> call

  for (const item of items) {
    if (item.event_type === 'tool_call') {
      const toolUseId = getToolUseId(item);
      if (toolUseId) {
        pendingToolCalls.set(toolUseId, item);
        // Add grouped item...
      }
    } else if (item.event_type === 'tool_result') {
      const toolUseId = getToolUseId(item);
      if (toolUseId && pendingToolCalls.has(toolUseId)) {
        // Match by tool_use_id - guaranteed unique
        // ...
      }
    }
  }
}
```

---

### Phase 5: Cleanup

#### 5.1 Remove Old WebSocket Code

Delete:
- `internal/api/websocket/hub.go`
- `internal/api/websocket/handler.go`

#### 5.2 Remove Old Hook

Delete:
- `frontend/src/hooks/useWebSocket.ts`

#### 5.3 Update Imports

Remove all imports of `internal/api/websocket` and update to use `internal/realtime`.

---

## Event Reference

### Task Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `task.created` | `{task_id, quest_id, title, auto_start}` | `task:{id}`, `project:{id}` | Quest accepts objective |
| `task.updated` | `{task_id, status}` | `task:{id}`, `project:{id}` | Status change |
| `task.completed` | `{task_id, outcome}` | `task:{id}`, `project:{id}` | Task finishes |
| `task.cancelled` | `{task_id, reason}` | `task:{id}`, `project:{id}` | User/system cancels |
| `task.unblocked` | `{task_id, unblocked_by}` | `task:{id}` | Dependency completed |
| `task.auto_started` | `{task_id, session_id, inherited_from}` | `task:{id}` | Auto-start triggered |
| `task.auto_start_failed` | `{task_id, error}` | `task:{id}` | Auto-start failed |

### Session Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `session.started` | `{session_id, task_id, hat}` | `task:{id}` | Session begins |
| `session.iteration` | `{session_id, task_id, iteration, context}` | `task:{id}` | Each iteration |
| `session.completed` | `{session_id, task_id, outcome}` | `task:{id}` | Session ends |

### Activity Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `activity.new` | `{task_id, session_id, activity: {...}}` | `task:{id}` | Any activity recorded |

Activity types: `user_message`, `assistant_response`, `tool_call`, `tool_result`, `hat_transition`, `completion_signal`, `debug_log`, `checklist_update`, `quality_gate`, `loop_health`, `decision`, `memory_created`

### Quest Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `quest.content_delta` | `{quest_id, delta, content}` | `quest:{id}` | Streaming response |
| `quest.tool_call` | `{quest_id, call_id, tool_name, status}` | `quest:{id}` | Tool execution starts |
| `quest.tool_result` | `{quest_id, call_id, tool_name, output, is_error}` | `quest:{id}` | Tool execution completes |
| `quest.message` | `{quest_id, message: {...}}` | `quest:{id}` | Final message saved |
| `quest.objective_draft` | `{quest_id, draft: {...}}` | `quest:{id}` | Objective proposed |
| `quest.question` | `{quest_id, question: {...}}` | `quest:{id}` | Clarifying question |
| `quest.ready` | `{quest_id, drafts, summary}` | `quest:{id}` | Quest ready for execution |

### Planning Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `planning.started` | `{task_id, session_id, status}` | `task:{id}` | Planning begins |
| `planning.updated` | `{task_id, session_id, status}` | `task:{id}` | Planning continues |
| `planning.completed` | `{task_id, session_id}` | `task:{id}` | Plan accepted |
| `planning.skipped` | `{task_id, session_id}` | `task:{id}` | Planning skipped |

### Checklist Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `checklist.updated` | `{task_id, checklist_id, item: {...}}` | `task:{id}` | Item status changes |

### Approval Events

| Event | Payload | Channel | Trigger |
|-------|---------|---------|---------|
| `approval.required` | `{approval_id, task_id, type, message}` | `system` | Budget exceeded, etc. |
| `approval.resolved` | `{approval_id, task_id, status, resolved_by}` | `system` | User approves/rejects |

---

## Configuration

### Backend

```go
realtime.Config{
    // Max bytes queued per client before disconnect
    ClientQueueMaxSize: 2 * 1024 * 1024,  // 2MB

    // Max time to wait for write to complete
    WriteTimeout: 5 * time.Second,

    // Ping interval for keepalive
    PingInterval: 25 * time.Second,
}
```

### Frontend

```typescript
const client = new Centrifuge(wsUrl, {
    token,
    minReconnectDelay: 1000,    // 1 second
    maxReconnectDelay: 10000,   // 10 seconds
    maxServerPingDelay: 10000,  // 10 seconds
});
```

---

## Acceptance Criteria

### Phase 1: Infrastructure
- [ ] Centrifuge node initializes and runs
- [ ] WebSocket endpoint accepts connections with auth
- [ ] Clients can subscribe to channels
- [ ] Events publish to correct channels

### Phase 2: Backend Migration
- [ ] All 42 broadcast locations updated
- [ ] Events routed to correct channels
- [ ] Project-level events for list views
- [ ] Old hub code removed

### Phase 3: Frontend Migration
- [ ] CentrifugeProvider wraps app
- [ ] useChannel hook works
- [ ] All pages use channel subscriptions
- [ ] Old useWebSocket removed

### Phase 4: Activity Fixes
- [ ] tool_use_id in database schema
- [ ] Tool calls recorded with tool_use_id
- [ ] Frontend groups by tool_use_id
- [ ] No more pairing bugs

### Phase 5: Testing
- [ ] Slow client doesn't block others
- [ ] Reconnection works smoothly
- [ ] Streaming content displays correctly
- [ ] All event types handled

---

## Future Enhancements

1. **Message History/Recovery**: Use Centrifuge's history feature for reconnection recovery
2. **Presence**: Show who's viewing a task/quest
3. **Redis Backend**: Scale horizontally with Redis broker
4. **Rate Limiting**: Per-channel publish rate limits
5. **Metrics**: Prometheus metrics for connection counts, message rates
