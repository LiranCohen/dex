// Package realtime provides WebSocket-based real-time event broadcasting using Centrifuge.
//
// # Architecture Overview
//
// The realtime system consists of:
//   - Node: Wraps the Centrifuge server, handles WebSocket connections and channel subscriptions
//   - Broadcaster: High-level API for publishing events with automatic channel routing
//   - AuthMiddleware: JWT validation for WebSocket connections
//
// # Channel Types
//
// Events are routed to channels based on their type:
//   - global: All events (for clients that want everything)
//   - user:<id>: User-specific events (approvals, notifications)
//   - task:<id>: Task lifecycle events (status changes, activity)
//   - quest:<id>: Quest conversation events (messages, tool calls)
//   - project:<id>: Project-level events (currently unused by frontend)
//   - system: System-wide events (approvals)
//
// # Message Recovery
//
// Centrifuge maintains message history (100 messages, 5 min TTL by default) to support
// client reconnection. When a client reconnects, it can recover missed messages.
//
// # Frontend Integration
//
// The frontend useWebSocket hook:
//  1. Auto-subscribes to 'global' channel on connect
//  2. Components call subscribeToChannel() for targeted subscriptions
//  3. All events flow through the subscribe() handler mechanism
//
// # Single-User vs Multi-User
//
// Currently designed for single-user. For multi-user:
//   - canSubscribe() needs project membership checks
//   - Consider adding presence channels for collaboration features
package realtime

import (
	"time"
)

// Broadcaster publishes events to the Centrifuge realtime node.
// It provides convenience methods for common event types and handles
// automatic channel routing based on event type and payload.
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
		_ = b.node.Publish(eventType, payload)
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

// PublishHatEvent publishes a hat workflow event to task and project channels
func (b *Broadcaster) PublishHatEvent(eventType, sessionID, taskID, projectID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["session_id"] = sessionID
	payload["task_id"] = taskID
	payload["project_id"] = projectID
	b.Publish(eventType, payload)
}

// PublishWorkerProgress publishes a worker progress update event
func (b *Broadcaster) PublishWorkerProgress(objectiveID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["objective_id"] = objectiveID
	b.Publish(EventWorkerProgress, payload)
}

// PublishWorkerCompletion publishes a worker completion event
func (b *Broadcaster) PublishWorkerCompletion(objectiveID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["objective_id"] = objectiveID
	b.Publish(EventWorkerCompleted, payload)
}

// PublishWorkerFailed publishes a worker failure event
func (b *Broadcaster) PublishWorkerFailed(objectiveID string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["objective_id"] = objectiveID
	b.Publish(EventWorkerFailed, payload)
}

// Event types as constants for consistency.
//
// Events are published to channels based on their prefix:
//   - task.*, session.*, activity.*, planning.*, checklist.* → task channel
//   - quest.* → quest channel
//   - approval.* → system + user + project + task channels
//   - hat.* → task + project channels
//   - All events also go to the global channel
const (
	// Task events - published to task:<id> and project:<id> channels
	EventTaskCreated         = "task.created"
	EventTaskUpdated         = "task.updated"
	EventTaskCancelled       = "task.cancelled"
	EventTaskPaused          = "task.paused"
	EventTaskResumed         = "task.resumed"
	EventTaskUnblocked       = "task.unblocked"
	EventTaskAutoStarted     = "task.auto_started"
	EventTaskAutoStartFailed = "task.auto_start_failed"

	// Session events - published to task:<id> channel
	EventSessionKilled    = "session.killed"
	EventSessionStarted   = "session.started"
	EventSessionIteration = "session.iteration"
	EventSessionCompleted = "session.completed"

	// Activity events - published to task:<id> channel
	EventActivityNew = "activity.new"

	// Quest events - published to quest:<id> channel
	//
	// Note on quest.objective_draft, quest.question, quest.ready:
	// These events are broadcast separately from quest.message to support future
	// streaming scenarios where drafts/questions arrive before the full message.
	// Currently, the frontend parses these from quest.message content directly,
	// but these events are kept for:
	//   1. Future streaming UI improvements
	//   2. Clients that want granular event handling without parsing
	//   3. Decoupling event structure from message content format
	EventQuestCreated        = "quest.created"
	EventQuestUpdated        = "quest.updated"
	EventQuestDeleted        = "quest.deleted"
	EventQuestCompleted      = "quest.completed"
	EventQuestReopened       = "quest.reopened"
	EventQuestContentDelta   = "quest.content_delta" // Streaming content chunks
	EventQuestToolCall       = "quest.tool_call"     // Tool execution started
	EventQuestToolResult     = "quest.tool_result"   // Tool execution completed
	EventQuestMessage        = "quest.message"       // Complete assistant message
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

	// Hat events (workflow transitions)
	EventHatPlanComplete       = "hat.plan_complete"
	EventHatDesignComplete     = "hat.design_complete"
	EventHatImplementationDone = "hat.implementation_done"
	EventHatReviewApproved     = "hat.review_approved"
	EventHatReviewRejected     = "hat.review_rejected"
	EventHatTaskBlocked        = "hat.task_blocked"
	EventHatResolved           = "hat.resolved"

	// Worker events (distributed execution)
	EventWorkerProgress  = "worker.progress"
	EventWorkerCompleted = "worker.completed"
	EventWorkerFailed    = "worker.failed"
)
