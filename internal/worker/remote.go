package worker

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"sync"
	"time"
)

// RemoteWorker manages a worker connected via mesh network.
type RemoteWorker struct {
	id       string
	hostname string
	meshIP   string
	pubKey   [32]byte
	conn     net.Conn
	protocol *Conn

	state        WorkerState
	objectiveID  string
	sessionID    string
	iteration    int
	tokensUsed   int
	lastActivity time.Time
	connectedAt  time.Time
	version      string
	err          error

	mu        sync.RWMutex
	done      chan struct{}
	eventChan chan *Message
}

// NewRemoteWorker creates a new remote worker from a mesh connection.
func NewRemoteWorker(id, hostname, meshIP string, pubKey [32]byte, conn net.Conn) *RemoteWorker {
	return &RemoteWorker{
		id:          id,
		hostname:    hostname,
		meshIP:      meshIP,
		pubKey:      pubKey,
		conn:        conn,
		protocol:    NewConn(conn, conn),
		state:       WorkerStateIdle,
		connectedAt: time.Now(),
		done:        make(chan struct{}),
		eventChan:   make(chan *Message, 100),
	}
}

// ID returns the worker's unique identifier.
func (w *RemoteWorker) ID() string {
	return w.id
}

// Type returns WorkerTypeRemote.
func (w *RemoteWorker) Type() WorkerType {
	return WorkerTypeRemote
}

// Start begins the message receive loop for this remote worker.
// Should be called after the worker has sent its ready message.
func (w *RemoteWorker) Start(ctx context.Context) error {
	go w.receiveLoop(ctx)
	return nil
}

// receiveLoop continuously reads messages from the remote worker.
func (w *RemoteWorker) receiveLoop(ctx context.Context) {
	defer close(w.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := w.protocol.Receive()
		if err != nil {
			w.mu.Lock()
			w.state = WorkerStateError
			w.err = fmt.Errorf("receive error: %w", err)
			w.mu.Unlock()
			return
		}

		w.handleMessage(msg)
	}
}

// handleMessage processes a received message.
func (w *RemoteWorker) handleMessage(msg *Message) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lastActivity = time.Now()

	switch msg.Type {
	case MsgTypeAccepted:
		payload, _ := ParsePayload[AcceptedPayload](msg)
		if payload != nil {
			w.sessionID = payload.SessionID
			w.state = WorkerStateRunning
		}

	case MsgTypeProgress:
		payload, _ := ParsePayload[ProgressPayload](msg)
		if payload != nil {
			w.iteration = payload.Iteration
			w.tokensUsed += payload.TokensInput + payload.TokensOutput
		}
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeActivity:
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeCompleted:
		w.state = WorkerStateIdle
		w.objectiveID = ""
		w.sessionID = ""
		w.iteration = 0
		w.tokensUsed = 0
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeFailed:
		w.state = WorkerStateIdle
		w.objectiveID = ""
		w.sessionID = ""
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeCancelled:
		w.state = WorkerStateIdle
		w.objectiveID = ""
		w.sessionID = ""
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypePong:
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeError:
		payload, _ := ParsePayload[ErrorPayload](msg)
		if payload != nil {
			w.err = fmt.Errorf("%s: %s", payload.Code, payload.Message)
		}
		select {
		case w.eventChan <- msg:
		default:
		}
	}
}

// Dispatch sends an objective to the remote worker.
// Secrets in the payload should already be encrypted for this worker's public key.
func (w *RemoteWorker) Dispatch(ctx context.Context, payload *ObjectivePayload) error {
	w.mu.Lock()
	if w.state != WorkerStateIdle {
		w.mu.Unlock()
		return fmt.Errorf("worker not idle (state: %s)", w.state)
	}
	w.objectiveID = payload.Objective.ID
	w.state = WorkerStateRunning
	w.mu.Unlock()

	if err := w.protocol.SendDispatch(payload); err != nil {
		w.mu.Lock()
		w.state = WorkerStateIdle
		w.objectiveID = ""
		w.mu.Unlock()
		return fmt.Errorf("failed to send dispatch: %w", err)
	}

	return nil
}

// Status returns the current worker status.
func (w *RemoteWorker) Status() *WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	errStr := ""
	if w.err != nil {
		errStr = w.err.Error()
	}

	return &WorkerStatus{
		ID:           w.id,
		Type:         WorkerTypeRemote,
		State:        w.state,
		Hostname:     w.hostname,
		MeshIP:       w.meshIP,
		ObjectiveID:  w.objectiveID,
		SessionID:    w.sessionID,
		Iteration:    w.iteration,
		TokensUsed:   w.tokensUsed,
		LastActivity: w.lastActivity,
		StartedAt:    w.connectedAt,
		Error:        errStr,
		Version:      w.version,
	}
}

// Cancel cancels the currently running objective.
func (w *RemoteWorker) Cancel(ctx context.Context) error {
	w.mu.RLock()
	objectiveID := w.objectiveID
	state := w.state
	w.mu.RUnlock()

	if state != WorkerStateRunning {
		return nil
	}

	return w.protocol.SendCancel(objectiveID, "cancelled by HQ")
}

// Stop gracefully disconnects from the remote worker.
func (w *RemoteWorker) Stop(ctx context.Context) error {
	w.mu.Lock()
	if w.state == WorkerStateStopped {
		w.mu.Unlock()
		return nil
	}
	w.state = WorkerStateStopping
	w.mu.Unlock()

	// Send shutdown message
	_ = w.protocol.SendShutdown()

	// Close connection
	if w.conn != nil {
		_ = w.conn.Close()
	}

	w.mu.Lock()
	w.state = WorkerStateStopped
	w.mu.Unlock()

	return nil
}

// Events returns a channel for receiving worker events.
func (w *RemoteWorker) Events() <-chan *Message {
	return w.eventChan
}

// PublicKey returns the worker's public key for encryption.
func (w *RemoteWorker) PublicKey() [32]byte {
	return w.pubKey
}

// EncryptPayload encrypts an objective payload for this specific remote worker.
func (w *RemoteWorker) EncryptPayload(dispatcher *Dispatcher, objective Objective, project Project, secrets WorkerSecrets, syncConfig SyncConfig) (*ObjectivePayload, error) {
	pubKeyB64 := base64.StdEncoding.EncodeToString(w.pubKey[:])
	return dispatcher.PreparePayload(objective, project, secrets, pubKeyB64, syncConfig)
}
