package realtime

import (
	"encoding/json"
	"time"

	"github.com/lirancohen/dex/internal/api/websocket"
)

// Broadcaster publishes events to both the legacy WebSocket hub and
// the new Centrifuge realtime node during the migration period.
// Once migration is complete, the legacy hub can be removed.
type Broadcaster struct {
	hub      *websocket.Hub
	realtime *Node
}

// NewBroadcaster creates a new broadcaster that publishes to both systems
func NewBroadcaster(hub *websocket.Hub, realtime *Node) *Broadcaster {
	return &Broadcaster{
		hub:      hub,
		realtime: realtime,
	}
}

// Publish sends an event to both the legacy hub and the new realtime system
func (b *Broadcaster) Publish(eventType string, payload map[string]any) {
	// Add timestamp if not present
	if _, ok := payload["timestamp"]; !ok {
		payload["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	// Publish to legacy hub (if available)
	if b.hub != nil {
		b.publishToLegacyHub(eventType, payload)
	}

	// Publish to new realtime system (if available)
	if b.realtime != nil {
		b.realtime.Publish(eventType, payload)
	}
}

// PublishTaskEvent publishes a task-related event
func (b *Broadcaster) PublishTaskEvent(eventType, taskID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["task_id"] = taskID
	b.Publish(eventType, payload)
}

// PublishQuestEvent publishes a quest-related event
func (b *Broadcaster) PublishQuestEvent(eventType, questID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["quest_id"] = questID
	b.Publish(eventType, payload)
}

// PublishProjectEvent publishes a project-related event
func (b *Broadcaster) PublishProjectEvent(eventType, projectID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["project_id"] = projectID
	b.Publish(eventType, payload)
}

// PublishActivityEvent publishes an activity event for a task
func (b *Broadcaster) PublishActivityEvent(taskID string, activity map[string]any) {
	payload := map[string]any{
		"task_id":  taskID,
		"activity": activity,
	}
	b.Publish("activity.new", payload)
}

// publishToLegacyHub converts the event to the legacy hub format and broadcasts
func (b *Broadcaster) publishToLegacyHub(eventType string, payload map[string]any) {
	// Build legacy message format
	msg := websocket.Message{
		Type:    eventType,
		Payload: payload,
	}

	// Set TaskID if present for subscription filtering
	if taskID, ok := payload["task_id"].(string); ok {
		msg.TaskID = taskID
	}

	b.hub.Broadcast(msg)
}

// Event types as constants for consistency
const (
	// Task events
	EventTaskCreated        = "task.created"
	EventTaskUpdated        = "task.updated"
	EventTaskCompleted      = "task.completed"
	EventTaskCancelled      = "task.cancelled"
	EventTaskUnblocked      = "task.unblocked"
	EventTaskAutoStarted    = "task.auto_started"
	EventTaskAutoStartFailed = "task.auto_start_failed"

	// Session events
	EventSessionStarted   = "session.started"
	EventSessionIteration = "session.iteration"
	EventSessionCompleted = "session.completed"

	// Activity events
	EventActivityNew = "activity.new"

	// Quest events
	EventQuestContentDelta  = "quest.content_delta"
	EventQuestToolCall      = "quest.tool_call"
	EventQuestToolResult    = "quest.tool_result"
	EventQuestMessage       = "quest.message"
	EventQuestObjectiveDraft = "quest.objective_draft"
	EventQuestQuestion      = "quest.question"
	EventQuestReady         = "quest.ready"

	// Planning events
	EventPlanningStarted   = "planning.started"
	EventPlanningUpdated   = "planning.updated"
	EventPlanningCompleted = "planning.completed"
	EventPlanningSkipped   = "planning.skipped"

	// Checklist events
	EventChecklistUpdated = "checklist.updated"

	// Approval events
	EventApprovalRequired = "approval.required"
	EventApprovalResolved = "approval.resolved"

	// Project events
	EventProjectUpdated = "project.updated"
)

// ActivityType constants
const (
	ActivityUserMessage      = "user_message"
	ActivityAssistantMessage = "assistant_response"
	ActivityToolCall         = "tool_call"
	ActivityToolResult       = "tool_result"
	ActivityHatTransition    = "hat_transition"
	ActivityChecklistUpdate  = "checklist_update"
	ActivityError            = "error"
	ActivityCompletion       = "completion"
)

// NewActivityPayload creates a properly structured activity payload
func NewActivityPayload(activityType string, content any, opts ...ActivityOption) map[string]any {
	activity := map[string]any{
		"type":      activityType,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}

	// Set content based on type
	switch v := content.(type) {
	case string:
		activity["content"] = v
	case map[string]any:
		for k, val := range v {
			activity[k] = val
		}
	default:
		// Try JSON marshal for other types
		if data, err := json.Marshal(content); err == nil {
			activity["content"] = string(data)
		}
	}

	// Apply options
	for _, opt := range opts {
		opt(activity)
	}

	return activity
}

// ActivityOption is a functional option for activity payloads
type ActivityOption func(map[string]any)

// WithToolUseID adds a tool_use_id to pair tool calls with results
func WithToolUseID(id string) ActivityOption {
	return func(a map[string]any) {
		a["tool_use_id"] = id
	}
}

// WithToolName adds the tool name
func WithToolName(name string) ActivityOption {
	return func(a map[string]any) {
		a["tool_name"] = name
	}
}

// WithSessionID adds the session ID
func WithSessionID(id string) ActivityOption {
	return func(a map[string]any) {
		a["session_id"] = id
	}
}

// WithIterationNumber adds the iteration number
func WithIterationNumber(n int) ActivityOption {
	return func(a map[string]any) {
		a["iteration"] = n
	}
}
