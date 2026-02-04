package realtime

import (
	"time"
)

// Broadcaster publishes events to the Centrifuge realtime node
type Broadcaster struct {
	node *Node
}

// NewBroadcaster creates a new broadcaster
func NewBroadcaster(node *Node) *Broadcaster {
	return &Broadcaster{
		node: node,
	}
}

// Publish sends an event to the realtime system
func (b *Broadcaster) Publish(eventType string, payload map[string]any) {
	// Add timestamp if not present
	if _, ok := payload["timestamp"]; !ok {
		payload["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if b.node != nil {
		b.node.Publish(eventType, payload)
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

// Event types as constants for consistency
const (
	// Task events
	EventTaskCreated         = "task.created"
	EventTaskUpdated         = "task.updated"
	EventTaskCancelled       = "task.cancelled"
	EventTaskPaused          = "task.paused"
	EventTaskResumed         = "task.resumed"
	EventTaskUnblocked       = "task.unblocked"
	EventTaskAutoStarted     = "task.auto_started"
	EventTaskAutoStartFailed = "task.auto_start_failed"

	// Session events
	EventSessionKilled    = "session.killed"
	EventSessionStarted   = "session.started"
	EventSessionIteration = "session.iteration"
	EventSessionCompleted = "session.completed"

	// Activity events
	EventActivityNew = "activity.new"

	// Quest events
	EventQuestCreated        = "quest.created"
	EventQuestUpdated        = "quest.updated"
	EventQuestDeleted        = "quest.deleted"
	EventQuestCompleted      = "quest.completed"
	EventQuestReopened       = "quest.reopened"
	EventQuestContentDelta   = "quest.content_delta"
	EventQuestToolCall       = "quest.tool_call"
	EventQuestToolResult     = "quest.tool_result"
	EventQuestMessage        = "quest.message"
	EventQuestObjectiveDraft = "quest.objective_draft"
	EventQuestQuestion       = "quest.question"
	EventQuestReady          = "quest.ready"

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
)
