package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewWorkerActivityRecorder(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	if recorder.objectiveID != "obj-456" {
		t.Errorf("expected objectiveID obj-456, got %s", recorder.objectiveID)
	}
	if recorder.hat != "explorer" {
		t.Errorf("expected hat explorer, got %s", recorder.hat)
	}
}

func TestWorkerActivityRecorder_SetHat(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	recorder.SetHat("creator")
	if recorder.hat != "creator" {
		t.Errorf("expected hat creator, got %s", recorder.hat)
	}
}

func TestWorkerActivityRecorder_RecordUserMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "activity-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create session in DB
	payload := &ObjectivePayload{
		Objective: Objective{ID: "obj-456", Title: "Test", Hat: "explorer"},
		Project:   Project{ID: "proj-1"},
	}
	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}
	if err := db.CreateSession("sess-123", "obj-456", "explorer"); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(db, nil, session, 30)

	if err := recorder.RecordUserMessage(1, "test message"); err != nil {
		t.Fatalf("failed to record user message: %v", err)
	}

	// Should have pending events
	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}

	// Check it was stored in DB
	events, err := db.GetUnsyncedActivity(10)
	if err != nil {
		t.Fatalf("failed to get unsynced activity: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event in DB, got %d", len(events))
	}
	if events[0].EventType != ActivityTypeUserMessage {
		t.Errorf("expected event type %s, got %s", ActivityTypeUserMessage, events[0].EventType)
	}
	if events[0].Content != "test message" {
		t.Errorf("expected content 'test message', got %s", events[0].Content)
	}
}

func TestWorkerActivityRecorder_RecordAssistantResponse(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	if err := recorder.RecordAssistantResponse(1, "response content", 100, 50); err != nil {
		t.Fatalf("failed to record assistant response: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_RecordToolCall(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	input := map[string]string{"path": "/test/file.go"}
	if err := recorder.RecordToolCall(1, "read_file", input); err != nil {
		t.Fatalf("failed to record tool call: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_RecordToolResult(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	result := map[string]any{"content": "file contents", "success": true}
	if err := recorder.RecordToolResult(1, "read_file", result); err != nil {
		t.Fatalf("failed to record tool result: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_RecordCompletion(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	if err := recorder.RecordCompletion(5, "task_complete"); err != nil {
		t.Fatalf("failed to record completion: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_RecordHatTransition(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	if err := recorder.RecordHatTransition(3, "explorer", "creator"); err != nil {
		t.Fatalf("failed to record hat transition: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_RecordChecklistUpdate(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	if err := recorder.RecordChecklistUpdate(2, "item-1", "done", "completed successfully"); err != nil {
		t.Fatalf("failed to record checklist update: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_RecordDebugLog(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	if err := recorder.RecordDebugLog(1, "info", "test log", 100, map[string]string{"key": "value"}); err != nil {
		t.Fatalf("failed to record debug log: %v", err)
	}

	if recorder.GetUnsyncedCount() != 1 {
		t.Errorf("expected 1 pending event, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_DebugMethods(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	recorder.Debug(1, "info message")
	recorder.DebugWithDuration(2, "timed message", 500)
	recorder.DebugError(3, "error message", map[string]string{"error": "test"})

	if recorder.GetUnsyncedCount() != 3 {
		t.Errorf("expected 3 pending events, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_FlushNoEvents(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	// Flush with no events should succeed
	if err := recorder.Flush(); err != nil {
		t.Fatalf("flush with no events failed: %v", err)
	}
}

func TestWorkerActivityRecorder_FlushNoConn(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	// Record some events
	_ = recorder.RecordUserMessage(1, "test")
	_ = recorder.RecordAssistantResponse(1, "response", 100, 50)

	// Flush without connection should clear pending events (no-op sync)
	if err := recorder.Flush(); err != nil {
		t.Fatalf("flush without conn failed: %v", err)
	}

	// Events should be cleared (no conn means no retry needed)
	if recorder.GetUnsyncedCount() != 0 {
		t.Errorf("expected 0 pending events after flush, got %d", recorder.GetUnsyncedCount())
	}
}

func TestWorkerActivityRecorder_GetAllUnsynced(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "activity-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create session in DB
	payload := &ObjectivePayload{
		Objective: Objective{ID: "obj-456", Title: "Test", Hat: "explorer"},
		Project:   Project{ID: "proj-1"},
	}
	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}
	if err := db.CreateSession("sess-123", "obj-456", "explorer"); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(db, nil, session, 30)

	// Record some events
	_ = recorder.RecordUserMessage(1, "msg1")
	_ = recorder.RecordUserMessage(2, "msg2")

	// Get all unsynced from DB
	events, err := recorder.GetAllUnsynced(10)
	if err != nil {
		t.Fatalf("failed to get all unsynced: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 unsynced events, got %d", len(events))
	}
}

func TestWorkerActivityRecorder_GetAllUnsynced_NoDB(t *testing.T) {
	session := NewWorkerSession("sess-123", "obj-456", "explorer", "/work")
	recorder := NewWorkerActivityRecorder(nil, nil, session, 30)

	events, err := recorder.GetAllUnsynced(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if events != nil {
		t.Error("expected nil events with no DB")
	}
}

func TestToolCallData_JSON(t *testing.T) {
	data := ToolCallData{
		Name:  "read_file",
		Input: map[string]string{"path": "/test"},
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ToolCallData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", decoded.Name)
	}
}

func TestToolResultData_JSON(t *testing.T) {
	data := ToolResultData{
		Name:   "read_file",
		Result: "file contents",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ToolResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", decoded.Name)
	}
}

func TestHatTransitionData_JSON(t *testing.T) {
	data := HatTransitionData{
		FromHat: "explorer",
		ToHat:   "creator",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded HatTransitionData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.FromHat != "explorer" || decoded.ToHat != "creator" {
		t.Error("unexpected hat transition values")
	}
}

func TestChecklistUpdateData_JSON(t *testing.T) {
	data := ChecklistUpdateData{
		ItemID: "item-1",
		Status: "done",
		Notes:  "completed",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ChecklistUpdateData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ItemID != "item-1" || decoded.Status != "done" {
		t.Error("unexpected checklist update values")
	}
}

func TestDebugLogData_JSON(t *testing.T) {
	data := DebugLogData{
		Level:      "info",
		Message:    "test message",
		DurationMs: 100,
		Details:    map[string]string{"key": "value"},
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded DebugLogData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Level != "info" || decoded.Message != "test message" {
		t.Error("unexpected debug log values")
	}
	if decoded.DurationMs != 100 {
		t.Errorf("expected duration 100, got %d", decoded.DurationMs)
	}
}
