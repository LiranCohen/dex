package worker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Protocol defines the message types exchanged between HQ and workers.
// Messages are JSON-encoded, one per line (newline-delimited JSON).

// MessageType identifies the type of protocol message.
type MessageType string

const (
	// HQ -> Worker messages
	MsgTypeDispatch MessageType = "dispatch" // Send objective to worker
	MsgTypeCancel   MessageType = "cancel"   // Cancel current objective
	MsgTypeShutdown MessageType = "shutdown" // Gracefully stop worker
	MsgTypePing     MessageType = "ping"     // Health check

	// Worker -> HQ messages
	MsgTypeReady       MessageType = "ready"        // Worker is ready to receive work
	MsgTypeAccepted    MessageType = "accepted"     // Objective accepted, starting execution
	MsgTypeProgress    MessageType = "progress"     // Progress update (iteration complete)
	MsgTypeActivity    MessageType = "activity"     // Activity events to sync
	MsgTypeCompleted   MessageType = "completed"    // Objective completed
	MsgTypeFailed      MessageType = "failed"       // Objective failed
	MsgTypeCancelled   MessageType = "cancelled"    // Objective was cancelled
	MsgTypePong        MessageType = "pong"         // Health check response
	MsgTypeError       MessageType = "error"        // Protocol or worker error
	MsgTypeShutdownAck MessageType = "shutdown_ack" // Acknowledging shutdown
)

// Message is the envelope for all protocol messages.
type Message struct {
	Type      MessageType     `json:"type"`
	ID        string          `json:"id,omitempty"` // Message ID for correlation
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// DispatchPayload is the payload for MsgTypeDispatch.
type DispatchPayload struct {
	Objective *ObjectivePayload `json:"objective"`
}

// CancelPayload is the payload for MsgTypeCancel.
type CancelPayload struct {
	ObjectiveID string `json:"objective_id"`
	Reason      string `json:"reason,omitempty"`
}

// ReadyPayload is the payload for MsgTypeReady.
type ReadyPayload struct {
	WorkerID  string `json:"worker_id"`
	Version   string `json:"version"`
	PublicKey string `json:"public_key"` // Worker's public key for encryption
}

// AcceptedPayload is the payload for MsgTypeAccepted.
type AcceptedPayload struct {
	ObjectiveID string `json:"objective_id"`
	SessionID   string `json:"session_id"`
}

// ProgressPayload is the payload for MsgTypeProgress.
type ProgressPayload struct {
	ObjectiveID  string `json:"objective_id"`
	SessionID    string `json:"session_id"`
	Iteration    int    `json:"iteration"`
	TokensInput  int    `json:"tokens_input"`
	TokensOutput int    `json:"tokens_output"`
	Hat          string `json:"hat,omitempty"`
	Status       string `json:"status,omitempty"` // Current status description
}

// ActivityPayload is the payload for MsgTypeActivity.
type ActivityPayload struct {
	ObjectiveID string           `json:"objective_id"`
	SessionID   string           `json:"session_id"`
	Events      []*ActivityEvent `json:"events"`
}

// CompletedPayload is the payload for MsgTypeCompleted.
type CompletedPayload struct {
	Report *CompletionReport `json:"report"`
}

// FailedPayload is the payload for MsgTypeFailed.
type FailedPayload struct {
	ObjectiveID string `json:"objective_id"`
	SessionID   string `json:"session_id"`
	Error       string `json:"error"`
	Iteration   int    `json:"iteration"`
}

// ErrorPayload is the payload for MsgTypeError.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PongPayload is the payload for MsgTypePong.
type PongPayload struct {
	WorkerID    string      `json:"worker_id"`
	State       WorkerState `json:"state"`
	ObjectiveID string      `json:"objective_id,omitempty"`
	Iteration   int         `json:"iteration,omitempty"`
	TokensUsed  int         `json:"tokens_used,omitempty"`
}

// Conn wraps a reader/writer pair for protocol communication.
// It's safe for concurrent use - reads and writes are serialized.
type Conn struct {
	reader  *bufio.Reader
	writer  io.Writer
	readMu  sync.Mutex
	writeMu sync.Mutex
}

// NewConn creates a new protocol connection.
func NewConn(r io.Reader, w io.Writer) *Conn {
	return &Conn{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// Send sends a message with the given type and payload.
func (c *Conn) Send(msgType MessageType, payload interface{}) error {
	var payloadBytes json.RawMessage
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Payload:   payloadBytes,
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write message followed by newline
	if _, err := c.writer.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// Receive reads and returns the next message.
// Blocks until a message is available or an error occurs.
func (c *Conn) Receive() (*Message, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// ParsePayload unmarshals the message payload into the given type.
func ParsePayload[T any](msg *Message) (*T, error) {
	if msg.Payload == nil {
		return nil, fmt.Errorf("message has no payload")
	}

	var payload T
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &payload, nil
}

// SendDispatch is a helper to send a dispatch message.
func (c *Conn) SendDispatch(payload *ObjectivePayload) error {
	return c.Send(MsgTypeDispatch, &DispatchPayload{Objective: payload})
}

// SendCancel is a helper to send a cancel message.
func (c *Conn) SendCancel(objectiveID, reason string) error {
	return c.Send(MsgTypeCancel, &CancelPayload{
		ObjectiveID: objectiveID,
		Reason:      reason,
	})
}

// SendShutdown is a helper to send a shutdown message.
func (c *Conn) SendShutdown() error {
	return c.Send(MsgTypeShutdown, nil)
}

// SendPing is a helper to send a ping message.
func (c *Conn) SendPing() error {
	return c.Send(MsgTypePing, nil)
}

// SendReady is a helper to send a ready message.
func (c *Conn) SendReady(workerID, version, publicKey string) error {
	return c.Send(MsgTypeReady, &ReadyPayload{
		WorkerID:  workerID,
		Version:   version,
		PublicKey: publicKey,
	})
}

// SendAccepted is a helper to send an accepted message.
func (c *Conn) SendAccepted(objectiveID, sessionID string) error {
	return c.Send(MsgTypeAccepted, &AcceptedPayload{
		ObjectiveID: objectiveID,
		SessionID:   sessionID,
	})
}

// SendProgress is a helper to send a progress message.
func (c *Conn) SendProgress(progress *ProgressPayload) error {
	return c.Send(MsgTypeProgress, progress)
}

// SendActivity is a helper to send activity events.
func (c *Conn) SendActivity(objectiveID, sessionID string, events []*ActivityEvent) error {
	return c.Send(MsgTypeActivity, &ActivityPayload{
		ObjectiveID: objectiveID,
		SessionID:   sessionID,
		Events:      events,
	})
}

// SendCompleted is a helper to send a completed message.
func (c *Conn) SendCompleted(report *CompletionReport) error {
	return c.Send(MsgTypeCompleted, &CompletedPayload{Report: report})
}

// SendFailed is a helper to send a failed message.
func (c *Conn) SendFailed(objectiveID, sessionID, errorMsg string, iteration int) error {
	return c.Send(MsgTypeFailed, &FailedPayload{
		ObjectiveID: objectiveID,
		SessionID:   sessionID,
		Error:       errorMsg,
		Iteration:   iteration,
	})
}

// SendPong is a helper to send a pong message.
func (c *Conn) SendPong(status *PongPayload) error {
	return c.Send(MsgTypePong, status)
}

// SendError is a helper to send an error message.
func (c *Conn) SendError(code, message string) error {
	return c.Send(MsgTypeError, &ErrorPayload{
		Code:    code,
		Message: message,
	})
}
