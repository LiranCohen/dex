package worker

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMessage_JSONRoundtrip(t *testing.T) {
	msg := Message{
		Type:      MsgTypeDispatch,
		ID:        "msg-123",
		Timestamp: time.Now().Truncate(time.Millisecond), // JSON doesn't preserve nanoseconds
		Payload:   json.RawMessage(`{"test": "value"}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, msg.Type)
	}
	if decoded.ID != msg.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, msg.ID)
	}
	if !decoded.Timestamp.Equal(msg.Timestamp) {
		t.Errorf("Timestamp mismatch: got %v, want %v", decoded.Timestamp, msg.Timestamp)
	}
	if string(decoded.Payload) != string(msg.Payload) {
		t.Errorf("Payload mismatch: got %q, want %q", decoded.Payload, msg.Payload)
	}
}

func TestConn_SendReceive(t *testing.T) {
	// Create a pipe to simulate stdin/stdout
	reader, writer := io.Pipe()

	// Sender side
	senderConn := NewConn(nil, writer)

	// Receiver side
	receiverConn := NewConn(reader, nil)

	// Send in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- senderConn.Send(MsgTypeReady, &ReadyPayload{
			WorkerID:  "worker-1",
			Version:   "1.0.0",
			PublicKey: "base64key",
		})
		writer.Close()
	}()

	// Receive
	msg, err := receiverConn.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if msg.Type != MsgTypeReady {
		t.Errorf("Type mismatch: got %q, want %q", msg.Type, MsgTypeReady)
	}

	payload, err := ParsePayload[ReadyPayload](msg)
	if err != nil {
		t.Fatalf("ParsePayload failed: %v", err)
	}

	if payload.WorkerID != "worker-1" {
		t.Errorf("WorkerID mismatch: got %q", payload.WorkerID)
	}
	if payload.Version != "1.0.0" {
		t.Errorf("Version mismatch: got %q", payload.Version)
	}
}

func TestConn_SendWithoutPayload(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.Send(MsgTypePing, nil); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	var msg Message
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg.Type != MsgTypePing {
		t.Errorf("Type mismatch: got %q", msg.Type)
	}
	if msg.Payload != nil {
		t.Errorf("Payload should be nil, got %v", msg.Payload)
	}
}

func TestConn_ReceiveInvalidJSON(t *testing.T) {
	reader := strings.NewReader("not valid json\n")
	conn := NewConn(reader, nil)

	_, err := conn.Receive()
	if err == nil {
		t.Error("Receive should fail on invalid JSON")
	}
}

func TestParsePayload_Success(t *testing.T) {
	payload := &ProgressPayload{
		ObjectiveID:  "obj-123",
		SessionID:    "sess-456",
		Iteration:    5,
		TokensInput:  100,
		TokensOutput: 50,
		Hat:          "engineer",
		Status:       "working",
	}

	payloadBytes, _ := json.Marshal(payload)
	msg := &Message{
		Type:    MsgTypeProgress,
		Payload: payloadBytes,
	}

	parsed, err := ParsePayload[ProgressPayload](msg)
	if err != nil {
		t.Fatalf("ParsePayload failed: %v", err)
	}

	if parsed.ObjectiveID != payload.ObjectiveID {
		t.Errorf("ObjectiveID mismatch")
	}
	if parsed.Iteration != payload.Iteration {
		t.Errorf("Iteration mismatch")
	}
	if parsed.TokensInput != payload.TokensInput {
		t.Errorf("TokensInput mismatch")
	}
}

func TestParsePayload_NoPayload(t *testing.T) {
	msg := &Message{
		Type:    MsgTypePing,
		Payload: nil,
	}

	_, err := ParsePayload[PongPayload](msg)
	if err == nil {
		t.Error("ParsePayload should fail with no payload")
	}
}

func TestParsePayload_InvalidPayload(t *testing.T) {
	msg := &Message{
		Type:    MsgTypeProgress,
		Payload: json.RawMessage(`{"iteration": "not a number"}`),
	}

	_, err := ParsePayload[ProgressPayload](msg)
	if err == nil {
		t.Error("ParsePayload should fail on type mismatch")
	}
}

func TestConn_SendDispatch(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	payload := &ObjectivePayload{
		Objective: Objective{
			ID:    "obj-123",
			Title: "Test Objective",
			Hat:   "engineer",
		},
	}

	if err := conn.SendDispatch(payload); err != nil {
		t.Fatalf("SendDispatch failed: %v", err)
	}

	// Parse the output
	var msg Message
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg.Type != MsgTypeDispatch {
		t.Errorf("Type should be dispatch, got %q", msg.Type)
	}

	var dispatch DispatchPayload
	if err := json.Unmarshal(msg.Payload, &dispatch); err != nil {
		t.Fatalf("Payload unmarshal failed: %v", err)
	}

	if dispatch.Objective.Objective.ID != "obj-123" {
		t.Errorf("Objective ID mismatch")
	}
}

func TestConn_SendCancel(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendCancel("obj-123", "user requested"); err != nil {
		t.Fatalf("SendCancel failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeCancel {
		t.Errorf("Type should be cancel")
	}

	var cancel CancelPayload
	json.Unmarshal(msg.Payload, &cancel)

	if cancel.ObjectiveID != "obj-123" {
		t.Error("ObjectiveID mismatch")
	}
	if cancel.Reason != "user requested" {
		t.Error("Reason mismatch")
	}
}

func TestConn_SendReady(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendReady("worker-1", "1.0.0", "pubkey123"); err != nil {
		t.Fatalf("SendReady failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeReady {
		t.Errorf("Type should be ready")
	}

	parsed, _ := ParsePayload[ReadyPayload](&msg)
	if parsed.WorkerID != "worker-1" {
		t.Error("WorkerID mismatch")
	}
	if parsed.Version != "1.0.0" {
		t.Error("Version mismatch")
	}
	if parsed.PublicKey != "pubkey123" {
		t.Error("PublicKey mismatch")
	}
}

func TestConn_SendAccepted(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendAccepted("obj-123", "sess-456"); err != nil {
		t.Fatalf("SendAccepted failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeAccepted {
		t.Errorf("Type should be accepted")
	}

	parsed, _ := ParsePayload[AcceptedPayload](&msg)
	if parsed.ObjectiveID != "obj-123" {
		t.Error("ObjectiveID mismatch")
	}
	if parsed.SessionID != "sess-456" {
		t.Error("SessionID mismatch")
	}
}

func TestConn_SendProgress(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	progress := &ProgressPayload{
		ObjectiveID:  "obj-123",
		SessionID:    "sess-456",
		Iteration:    3,
		TokensInput:  150,
		TokensOutput: 75,
		Hat:          "analyst",
		Status:       "analyzing code",
	}

	if err := conn.SendProgress(progress); err != nil {
		t.Fatalf("SendProgress failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeProgress {
		t.Errorf("Type should be progress")
	}

	parsed, _ := ParsePayload[ProgressPayload](&msg)
	if parsed.Iteration != 3 {
		t.Errorf("Iteration mismatch: got %d", parsed.Iteration)
	}
}

func TestConn_SendActivity(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	events := []*ActivityEvent{
		{EventType: "file_read", Content: "/src/main.go"},
		{EventType: "file_write", Content: "/src/util.go"},
	}

	if err := conn.SendActivity("obj-123", "sess-456", events); err != nil {
		t.Fatalf("SendActivity failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeActivity {
		t.Errorf("Type should be activity")
	}

	parsed, _ := ParsePayload[ActivityPayload](&msg)
	if len(parsed.Events) != 2 {
		t.Errorf("Should have 2 events, got %d", len(parsed.Events))
	}
}

func TestConn_SendCompleted(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	report := &CompletionReport{
		ObjectiveID: "obj-123",
		SessionID:   "sess-456",
		Status:      "completed",
		Summary:     "All tasks done",
		CompletedAt: time.Now(),
	}

	if err := conn.SendCompleted(report); err != nil {
		t.Fatalf("SendCompleted failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeCompleted {
		t.Errorf("Type should be completed")
	}

	parsed, _ := ParsePayload[CompletedPayload](&msg)
	if parsed.Report.Status != "completed" {
		t.Error("Status mismatch")
	}
}

func TestConn_SendFailed(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendFailed("obj-123", "sess-456", "something went wrong", 5); err != nil {
		t.Fatalf("SendFailed failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeFailed {
		t.Errorf("Type should be failed")
	}

	parsed, _ := ParsePayload[FailedPayload](&msg)
	if parsed.Error != "something went wrong" {
		t.Error("Error mismatch")
	}
	if parsed.Iteration != 5 {
		t.Errorf("Iteration mismatch: got %d", parsed.Iteration)
	}
}

func TestConn_SendPong(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	status := &PongPayload{
		WorkerID:    "worker-1",
		State:       WorkerStateRunning,
		ObjectiveID: "obj-123",
		Iteration:   7,
		TokensUsed:  500,
	}

	if err := conn.SendPong(status); err != nil {
		t.Fatalf("SendPong failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypePong {
		t.Errorf("Type should be pong")
	}

	parsed, _ := ParsePayload[PongPayload](&msg)
	if parsed.State != WorkerStateRunning {
		t.Errorf("State mismatch: got %q", parsed.State)
	}
	if parsed.TokensUsed != 500 {
		t.Errorf("TokensUsed mismatch")
	}
}

func TestConn_SendError(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendError("INVALID_PAYLOAD", "could not parse dispatch"); err != nil {
		t.Fatalf("SendError failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeError {
		t.Errorf("Type should be error")
	}

	parsed, _ := ParsePayload[ErrorPayload](&msg)
	if parsed.Code != "INVALID_PAYLOAD" {
		t.Error("Code mismatch")
	}
	if parsed.Message != "could not parse dispatch" {
		t.Error("Message mismatch")
	}
}

func TestConn_SendShutdown(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendShutdown(); err != nil {
		t.Fatalf("SendShutdown failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypeShutdown {
		t.Errorf("Type should be shutdown, got %q", msg.Type)
	}
}

func TestConn_SendPing(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	if err := conn.SendPing(); err != nil {
		t.Fatalf("SendPing failed: %v", err)
	}

	var msg Message
	json.Unmarshal(buf.Bytes(), &msg)

	if msg.Type != MsgTypePing {
		t.Errorf("Type should be ping, got %q", msg.Type)
	}
}

func TestConn_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(nil, &buf)

	// Send multiple messages concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			conn.Send(MsgTypePing, nil)
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}

	// Count messages (each ends with newline)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("Should have 10 messages, got %d", len(lines))
	}
}

func TestMessageTypes(t *testing.T) {
	// Verify all message types are distinct
	types := []MessageType{
		MsgTypeDispatch,
		MsgTypeCancel,
		MsgTypeShutdown,
		MsgTypePing,
		MsgTypeReady,
		MsgTypeAccepted,
		MsgTypeProgress,
		MsgTypeActivity,
		MsgTypeCompleted,
		MsgTypeFailed,
		MsgTypeCancelled,
		MsgTypePong,
		MsgTypeError,
		MsgTypeShutdownAck,
	}

	seen := make(map[MessageType]bool)
	for _, mt := range types {
		if seen[mt] {
			t.Errorf("Duplicate message type: %s", mt)
		}
		seen[mt] = true
	}
}
