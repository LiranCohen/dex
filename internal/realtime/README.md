# Realtime Event System

This package provides WebSocket-based real-time event broadcasting using [Centrifuge](https://github.com/centrifugal/centrifuge).

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     Frontend (Browser)                          │
├─────────────────────────────────────────────────────────────────┤
│  useWebSocket Hook                                              │
│  ├── Centrifuge client (centrifuge-js)                         │
│  ├── Auto-subscribes to 'global' channel                       │
│  ├── Components call subscribeToChannel() for targeted events  │
│  └── Message recovery on reconnect (100 msgs / 5 min)          │
└──────────────────────────┬──────────────────────────────────────┘
                           │ ws[s]://host/api/v1/realtime
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Backend (Go)                                │
├─────────────────────────────────────────────────────────────────┤
│  AuthMiddleware                                                 │
│  └── JWT validation, sets Centrifuge credentials                │
│                                                                  │
│  Node (node.go)                                                 │
│  ├── Centrifuge server wrapper                                  │
│  ├── Connection/subscription handlers                           │
│  ├── Channel authorization (canSubscribe)                       │
│  ├── Event routing (routeEvent)                                 │
│  └── Ping RPC for latency measurement                           │
│                                                                  │
│  Broadcaster (broadcaster.go)                                   │
│  ├── High-level event publishing API                            │
│  ├── PublishTaskEvent, PublishQuestEvent, PublishHatEvent      │
│  └── Auto-adds timestamps                                       │
└─────────────────────────────────────────────────────────────────┘
```

## Channel Types

Events are routed to channels based on their type prefix:

| Channel Pattern    | Events                                    | Use Case                    |
|--------------------|-------------------------------------------|-----------------------------|
| `global`           | All events                                | Clients wanting everything  |
| `user:<userID>`    | Approvals, notifications                  | User-specific updates       |
| `task:<taskID>`    | task.*, session.*, activity.*, planning.* | Task detail pages           |
| `quest:<questID>`  | quest.*                                   | Quest conversation pages    |
| `project:<projID>` | project.*, hat.*, task.* (with proj_id)  | Project list views          |
| `system`           | approval.*                                | System-wide notifications   |

## Event Types

### Task Events (`task.`)
- `task.created`, `task.updated`, `task.cancelled`
- `task.paused`, `task.resumed`, `task.unblocked`
- `task.auto_started`, `task.auto_start_failed`

### Session Events (`session.`)
- `session.started`, `session.iteration`, `session.completed`, `session.killed`

### Quest Events (`quest.`)
- `quest.created`, `quest.updated`, `quest.deleted`, `quest.completed`, `quest.reopened`
- `quest.message` - Complete assistant message
- `quest.content_delta` - Streaming content chunks
- `quest.tool_call`, `quest.tool_result` - Tool execution lifecycle
- `quest.objective_draft`, `quest.question`, `quest.ready` - Parsed content events (see note below)

### Other Events
- `activity.new` - New activity log entry
- `planning.*` - Planning phase updates
- `checklist.updated` - Checklist changes
- `approval.required`, `approval.resolved` - Approval workflow
- `hat.*` - Hat transition events for workflow management

## Design Decisions

### Quest Event Redundancy

The events `quest.objective_draft`, `quest.question`, and `quest.ready` are broadcast separately from `quest.message`, even though the frontend currently parses this information from the message content.

**Why they exist:**
1. **Future streaming**: Enable UI updates before the full message arrives
2. **Client flexibility**: Clients can handle structured events without parsing
3. **Decoupling**: Separates event structure from message content format

**Current behavior**: Frontend uses `quest.message` and parses content. These events are available for future use.

### Single-User Design

Currently optimized for single-user:
- `canSubscribe()` allows all authenticated users to subscribe to task/quest/project channels
- No presence tracking or collaboration features

**For multi-user (future):**
- Add project membership checks in `canSubscribe()`
- Consider presence channels for "User X is viewing this task"
- Add rate limiting per user

### Message Recovery

Centrifuge maintains message history per channel:
- **Default**: 100 messages, 5 minute TTL
- **Purpose**: Clients can recover missed messages on reconnect
- **Frontend**: `recover: true` option on subscriptions

## Usage

### Publishing Events (Backend)

```go
// Get broadcaster from deps
broadcaster := deps.GetBroadcaster()

// Publish task event
broadcaster.PublishTaskEvent(realtime.EventTaskUpdated, taskID, map[string]any{
    "status":     "running",
    "project_id": projectID, // Include for project channel routing
})

// Publish quest event
broadcaster.PublishQuestEvent(realtime.EventQuestMessage, questID, map[string]any{
    "message": message,
})

// Publish hat/workflow event
broadcaster.PublishHatEvent(realtime.EventHatPlanComplete, sessionID, taskID, projectID, map[string]any{
    "topic": "planning",
})
```

### Subscribing (Frontend)

```typescript
const { subscribe, subscribeToChannel } = useWebSocket();

// Subscribe to all events (via global channel)
useEffect(() => {
  return subscribe((event) => {
    if (event.type === 'task.updated') {
      // Handle event
    }
  });
}, [subscribe]);

// Subscribe to specific channel
useEffect(() => {
  return subscribeToChannel(`quest:${questId}`);
}, [questId, subscribeToChannel]);
```

## Testing

```bash
# Run backend tests
go test ./internal/realtime/...

# Run frontend tests
cd frontend && npm test -- useWebSocket
```

## Connection Status

The `ConnectionStatusBanner` component shows connection state:
- **Connected** (green): Real-time updates active
- **Reconnecting** (yellow): Attempting to reconnect
- **Failed** (red): Connection lost after max attempts

This banner is shown on pages that depend on real-time updates (Home, Inbox, QuestDetail, ObjectiveDetail, AllObjectives).
