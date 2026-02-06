package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ActivityType constants for worker activity events
const (
	ActivityTypeUserMessage      = "user_message"
	ActivityTypeAssistantResponse = "assistant_response"
	ActivityTypeToolCall         = "tool_call"
	ActivityTypeToolResult       = "tool_result"
	ActivityTypeCompletion       = "completion"
	ActivityTypeHatTransition    = "hat_transition"
	ActivityTypeChecklistUpdate  = "checklist_update"
	ActivityTypeDebugLog         = "debug_log"
)

// WorkerActivityRecorder records session activity to local DB and batches for HQ sync.
// It's designed for offline-resilient operation, storing locally first then syncing.
type WorkerActivityRecorder struct {
	mu sync.Mutex

	localDB     *LocalDB
	conn        *Conn
	session     *WorkerSession
	objectiveID string
	hat         string

	// Sync configuration
	syncInterval time.Duration
	stopSync     chan struct{}
	syncWg       sync.WaitGroup

	// Pending events for sync
	pendingEvents []*ActivityEvent
}

// NewWorkerActivityRecorder creates a new activity recorder.
func NewWorkerActivityRecorder(localDB *LocalDB, conn *Conn, session *WorkerSession, syncIntervalSec int) *WorkerActivityRecorder {
	interval := time.Duration(syncIntervalSec) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second // Default 30 seconds
	}

	return &WorkerActivityRecorder{
		localDB:      localDB,
		conn:         conn,
		session:      session,
		objectiveID:  session.ObjectiveID,
		hat:          session.Hat,
		syncInterval: interval,
		stopSync:     make(chan struct{}),
	}
}

// SetHat updates the current hat for activity tracking.
func (r *WorkerActivityRecorder) SetHat(hat string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hat = hat
}

// StartSyncLoop starts a background goroutine that periodically syncs activity to HQ.
func (r *WorkerActivityRecorder) StartSyncLoop(ctx context.Context) {
	r.syncWg.Add(1)
	go func() {
		defer r.syncWg.Done()
		ticker := time.NewTicker(r.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Final flush before exit
				if err := r.Flush(); err != nil {
					fmt.Printf("Warning: final activity flush failed: %v\n", err)
				}
				return
			case <-r.stopSync:
				// Final flush before exit
				_ = r.Flush()
				return
			case <-ticker.C:
				if err := r.Flush(); err != nil {
					fmt.Printf("Warning: activity sync failed: %v\n", err)
				}
			}
		}
	}()
}

// StopSyncLoop stops the background sync goroutine and waits for it to finish.
func (r *WorkerActivityRecorder) StopSyncLoop() {
	close(r.stopSync)
	r.syncWg.Wait()
}

// recordEvent creates and stores an activity event.
func (r *WorkerActivityRecorder) recordEvent(iteration int, eventType, content string, inputTokens, outputTokens int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	event := &ActivityEvent{
		ID:           uuid.New().String(),
		SessionID:    r.session.ID,
		ObjectiveID:  r.objectiveID,
		Iteration:    iteration,
		EventType:    eventType,
		Content:      content,
		TokensInput:  inputTokens,
		TokensOutput: outputTokens,
		Hat:          r.hat,
		CreatedAt:    time.Now(),
	}

	// Store in local DB first (offline resilience)
	if r.localDB != nil {
		if err := r.localDB.RecordActivity(event); err != nil {
			return fmt.Errorf("failed to record activity locally: %w", err)
		}
	}

	// Add to pending for next sync
	r.pendingEvents = append(r.pendingEvents, event)

	return nil
}

// RecordUserMessage records a user message sent to Claude.
func (r *WorkerActivityRecorder) RecordUserMessage(iteration int, content string) error {
	return r.recordEvent(iteration, ActivityTypeUserMessage, content, 0, 0)
}

// RecordAssistantResponse records Claude's response.
func (r *WorkerActivityRecorder) RecordAssistantResponse(iteration int, content string, inputTokens, outputTokens int) error {
	return r.recordEvent(iteration, ActivityTypeAssistantResponse, content, inputTokens, outputTokens)
}

// ToolCallData represents a tool call for activity recording.
type ToolCallData struct {
	Name  string `json:"name"`
	Input any    `json:"input"`
}

// RecordToolCall records a tool call made by Claude.
func (r *WorkerActivityRecorder) RecordToolCall(iteration int, toolName string, input any) error {
	data := ToolCallData{
		Name:  toolName,
		Input: input,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal tool call: %w", err)
	}
	return r.recordEvent(iteration, ActivityTypeToolCall, string(content), 0, 0)
}

// ToolResultData represents a tool result for activity recording.
type ToolResultData struct {
	Name   string `json:"name"`
	Result any    `json:"result"`
}

// RecordToolResult records the result of a tool call.
func (r *WorkerActivityRecorder) RecordToolResult(iteration int, toolName string, result any) error {
	data := ToolResultData{
		Name:   toolName,
		Result: result,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal tool result: %w", err)
	}
	return r.recordEvent(iteration, ActivityTypeToolResult, string(content), 0, 0)
}

// RecordCompletion records a completion signal.
func (r *WorkerActivityRecorder) RecordCompletion(iteration int, signal string) error {
	return r.recordEvent(iteration, ActivityTypeCompletion, signal, 0, 0)
}

// HatTransitionData represents a hat transition for activity recording.
type HatTransitionData struct {
	FromHat string `json:"from_hat"`
	ToHat   string `json:"to_hat"`
}

// RecordHatTransition records a hat transition.
func (r *WorkerActivityRecorder) RecordHatTransition(iteration int, fromHat, toHat string) error {
	data := HatTransitionData{
		FromHat: fromHat,
		ToHat:   toHat,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal hat transition: %w", err)
	}
	return r.recordEvent(iteration, ActivityTypeHatTransition, string(content), 0, 0)
}

// ChecklistUpdateData represents a checklist item update for activity recording.
type ChecklistUpdateData struct {
	ItemID string `json:"item_id"`
	Status string `json:"status"`
	Notes  string `json:"notes,omitempty"`
}

// RecordChecklistUpdate records a checklist item status change.
func (r *WorkerActivityRecorder) RecordChecklistUpdate(iteration int, itemID, status, notes string) error {
	data := ChecklistUpdateData{
		ItemID: itemID,
		Status: status,
		Notes:  notes,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal checklist update: %w", err)
	}
	return r.recordEvent(iteration, ActivityTypeChecklistUpdate, string(content), 0, 0)
}

// DebugLogData represents a debug log entry.
type DebugLogData struct {
	Level      string `json:"level"`
	Message    string `json:"message"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Details    any    `json:"details,omitempty"`
}

// RecordDebugLog records a debug-level log entry.
func (r *WorkerActivityRecorder) RecordDebugLog(iteration int, level, message string, durationMs int64, details any) error {
	data := DebugLogData{
		Level:      level,
		Message:    message,
		DurationMs: durationMs,
		Details:    details,
	}
	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal debug log: %w", err)
	}
	return r.recordEvent(iteration, ActivityTypeDebugLog, string(content), 0, 0)
}

// Debug is a convenience method for info-level debug logs.
func (r *WorkerActivityRecorder) Debug(iteration int, message string) {
	_ = r.RecordDebugLog(iteration, "info", message, 0, nil)
}

// DebugWithDuration logs with timing information.
func (r *WorkerActivityRecorder) DebugWithDuration(iteration int, message string, durationMs int64) {
	_ = r.RecordDebugLog(iteration, "info", message, durationMs, nil)
}

// DebugError logs an error-level debug message.
func (r *WorkerActivityRecorder) DebugError(iteration int, message string, details any) {
	_ = r.RecordDebugLog(iteration, "error", message, 0, details)
}

// Flush sends all pending activity events to HQ.
func (r *WorkerActivityRecorder) Flush() error {
	r.mu.Lock()
	if len(r.pendingEvents) == 0 {
		r.mu.Unlock()
		return nil
	}

	// Take ownership of pending events
	events := r.pendingEvents
	r.pendingEvents = nil
	r.mu.Unlock()

	// Send to HQ via protocol
	if r.conn != nil {
		payload := &ActivityPayload{
			ObjectiveID: r.objectiveID,
			SessionID:   r.session.ID,
			Events:      events,
		}
		if err := r.conn.Send(MsgTypeActivity, payload); err != nil {
			// Put events back for retry
			r.mu.Lock()
			r.pendingEvents = append(events, r.pendingEvents...)
			r.mu.Unlock()
			return fmt.Errorf("failed to send activity to HQ: %w", err)
		}

		// Mark as synced in local DB
		if r.localDB != nil {
			ids := make([]string, len(events))
			for i, e := range events {
				ids[i] = e.ID
			}
			if err := r.localDB.MarkActivitySynced(ids); err != nil {
				fmt.Printf("Warning: failed to mark activity as synced: %v\n", err)
			}
		}
	}

	return nil
}

// GetUnsyncedCount returns the number of events waiting to be synced.
func (r *WorkerActivityRecorder) GetUnsyncedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pendingEvents)
}

// GetAllUnsynced returns all unsynced events from local DB.
// This is useful for recovery after a crash.
func (r *WorkerActivityRecorder) GetAllUnsynced(limit int) ([]*ActivityEvent, error) {
	if r.localDB == nil {
		return nil, nil
	}
	return r.localDB.GetUnsyncedActivity(limit)
}
