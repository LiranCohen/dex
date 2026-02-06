package worker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// LocalWorker manages a worker running as a subprocess on the same machine.
type LocalWorker struct {
	id     string
	config *WorkerConfig
	cmd    *exec.Cmd
	conn   *Conn
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	state         WorkerState
	objectiveID   string
	sessionID     string
	iteration     int
	tokensUsed    int
	lastActivity  time.Time
	lastHeartbeat time.Time
	startedAt     time.Time
	workerPubKey  string
	version       string
	err           error

	mu        sync.RWMutex
	done      chan struct{}
	eventChan chan *Message // Channel for received messages
}

// NewLocalWorker creates a new local subprocess worker.
// The worker is not started until Start() is called.
func NewLocalWorker(config *WorkerConfig) *LocalWorker {
	id := config.ID
	if id == "" {
		id = fmt.Sprintf("local-%d", time.Now().UnixNano())
	}

	return &LocalWorker{
		id:        id,
		config:    config,
		state:     WorkerStateStopped,
		done:      make(chan struct{}),
		eventChan: make(chan *Message, 100),
	}
}

// ID returns the worker's unique identifier.
func (w *LocalWorker) ID() string {
	return w.id
}

// Type returns WorkerTypeLocal.
func (w *LocalWorker) Type() WorkerType {
	return WorkerTypeLocal
}

// Start spawns the worker subprocess and waits for it to be ready.
func (w *LocalWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != WorkerStateStopped {
		return fmt.Errorf("worker already started")
	}

	w.state = WorkerStateStarting

	// Find worker binary
	binaryPath := w.config.BinaryPath
	if binaryPath == "" {
		var err error
		binaryPath, err = exec.LookPath("dex-worker")
		if err != nil {
			w.state = WorkerStateError
			w.err = fmt.Errorf("dex-worker binary not found: %w", err)
			return w.err
		}
	}

	// Prepare command
	args := []string{
		"--mode=subprocess",
		fmt.Sprintf("--id=%s", w.id),
	}
	if w.config.DataDir != "" {
		args = append(args, fmt.Sprintf("--data-dir=%s", w.config.DataDir))
	}
	if w.config.HQPublicKey != "" {
		args = append(args, fmt.Sprintf("--hq-public-key=%s", w.config.HQPublicKey))
	}

	w.cmd = exec.CommandContext(ctx, binaryPath, args...)

	// Set up pipes
	var err error
	w.stdin, err = w.cmd.StdinPipe()
	if err != nil {
		w.state = WorkerStateError
		w.err = fmt.Errorf("failed to create stdin pipe: %w", err)
		return w.err
	}

	w.stdout, err = w.cmd.StdoutPipe()
	if err != nil {
		w.state = WorkerStateError
		w.err = fmt.Errorf("failed to create stdout pipe: %w", err)
		return w.err
	}

	w.stderr, err = w.cmd.StderrPipe()
	if err != nil {
		w.state = WorkerStateError
		w.err = fmt.Errorf("failed to create stderr pipe: %w", err)
		return w.err
	}

	// Start process
	if err := w.cmd.Start(); err != nil {
		w.state = WorkerStateError
		w.err = fmt.Errorf("failed to start worker process: %w", err)
		return w.err
	}

	w.startedAt = time.Now()
	w.conn = NewConn(w.stdout, w.stdin)

	// Start message receiver goroutine
	go w.receiveLoop()

	// Start stderr logger goroutine
	go w.logStderr()

	// Wait for ready message
	select {
	case msg := <-w.eventChan:
		if msg.Type != MsgTypeReady {
			w.state = WorkerStateError
			w.err = fmt.Errorf("expected ready message, got %s", msg.Type)
			return w.err
		}
		ready, err := ParsePayload[ReadyPayload](msg)
		if err != nil {
			w.state = WorkerStateError
			w.err = fmt.Errorf("failed to parse ready payload: %w", err)
			return w.err
		}
		w.workerPubKey = ready.PublicKey
		w.version = ready.Version
		w.state = WorkerStateIdle
		w.lastActivity = time.Now()
		return nil

	case <-time.After(30 * time.Second):
		w.state = WorkerStateError
		w.err = fmt.Errorf("timeout waiting for worker ready")
		_ = w.cmd.Process.Kill()
		return w.err

	case <-ctx.Done():
		w.state = WorkerStateError
		w.err = ctx.Err()
		_ = w.cmd.Process.Kill()
		return w.err
	}
}

// receiveLoop continuously reads messages from the worker.
func (w *LocalWorker) receiveLoop() {
	defer close(w.done)
	defer close(w.eventChan)

	for {
		msg, err := w.conn.Receive()
		if err != nil {
			// Check if we're shutting down
			w.mu.RLock()
			state := w.state
			w.mu.RUnlock()
			if state == WorkerStateStopping || state == WorkerStateStopped {
				return
			}

			w.mu.Lock()
			w.state = WorkerStateError
			w.err = fmt.Errorf("receive error: %w", err)
			w.mu.Unlock()
			return
		}

		w.handleMessage(msg)
	}
}

// handleMessage processes a received message and updates worker state.
func (w *LocalWorker) handleMessage(msg *Message) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lastActivity = time.Now()

	switch msg.Type {
	case MsgTypeReady, MsgTypePong:
		// Send to event channel for synchronous handling
		select {
		case w.eventChan <- msg:
		default:
			// Channel full, drop message
		}

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
		// Forward to event channel for manager
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeActivity:
		// Forward to event channel for manager to sync to HQ DB
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

	case MsgTypeShutdownAck:
		w.state = WorkerStateStopped
		select {
		case w.eventChan <- msg:
		default:
		}

	case MsgTypeHeartbeat:
		// Update heartbeat timestamp
		w.lastHeartbeat = time.Now()
		// Forward to event channel for manager
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

// logStderr reads and logs stderr from the worker process.
func (w *LocalWorker) logStderr() {
	buf := make([]byte, 4096)
	for {
		n, err := w.stderr.Read(buf)
		if n > 0 {
			// Log to stderr with worker prefix
			fmt.Fprintf(os.Stderr, "[worker:%s] %s", w.id, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Dispatch sends an objective to the worker.
func (w *LocalWorker) Dispatch(ctx context.Context, payload *ObjectivePayload) error {
	w.mu.Lock()
	if w.state != WorkerStateIdle {
		w.mu.Unlock()
		return fmt.Errorf("worker not idle (state: %s)", w.state)
	}
	w.objectiveID = payload.Objective.ID
	w.state = WorkerStateRunning
	w.mu.Unlock()

	if err := w.conn.SendDispatch(payload); err != nil {
		w.mu.Lock()
		w.state = WorkerStateIdle
		w.objectiveID = ""
		w.mu.Unlock()
		return fmt.Errorf("failed to send dispatch: %w", err)
	}

	return nil
}

// Status returns the current worker status.
func (w *LocalWorker) Status() *WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return &WorkerStatus{
		ID:           w.id,
		Type:         WorkerTypeLocal,
		State:        w.state,
		ObjectiveID:  w.objectiveID,
		SessionID:    w.sessionID,
		Iteration:    w.iteration,
		TokensUsed:   w.tokensUsed,
		LastActivity: w.lastActivity,
		StartedAt:    w.startedAt,
		Error:        errToString(w.err),
		Version:      w.version,
	}
}

// Cancel cancels the currently running objective.
func (w *LocalWorker) Cancel(ctx context.Context) error {
	w.mu.RLock()
	objectiveID := w.objectiveID
	state := w.state
	w.mu.RUnlock()

	if state != WorkerStateRunning {
		return nil // Nothing to cancel
	}

	return w.conn.SendCancel(objectiveID, "cancelled by HQ")
}

// Stop gracefully stops the worker.
func (w *LocalWorker) Stop(ctx context.Context) error {
	w.mu.Lock()
	if w.state == WorkerStateStopped {
		w.mu.Unlock()
		return nil
	}
	w.state = WorkerStateStopping
	w.mu.Unlock()

	// Send shutdown message
	if err := w.conn.SendShutdown(); err != nil {
		// Force kill if can't send shutdown
		if w.cmd != nil && w.cmd.Process != nil {
			_ = w.cmd.Process.Kill()
		}
	}

	// Wait for clean shutdown or timeout
	select {
	case <-w.done:
		// Clean shutdown
	case <-time.After(10 * time.Second):
		// Force kill
		if w.cmd != nil && w.cmd.Process != nil {
			_ = w.cmd.Process.Kill()
		}
	case <-ctx.Done():
		if w.cmd != nil && w.cmd.Process != nil {
			_ = w.cmd.Process.Kill()
		}
		return ctx.Err()
	}

	w.mu.Lock()
	w.state = WorkerStateStopped
	w.mu.Unlock()

	// Wait for process to exit
	if w.cmd != nil {
		_ = w.cmd.Wait()
	}

	return nil
}

// PublicKey returns the worker's public key for encrypting payloads.
func (w *LocalWorker) PublicKey() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.workerPubKey
}

// Events returns a channel for receiving worker events.
// The manager uses this to receive activity, progress, and completion events.
func (w *LocalWorker) Events() <-chan *Message {
	return w.eventChan
}

// UpdateLastHeartbeat updates the last heartbeat timestamp.
func (w *LocalWorker) UpdateLastHeartbeat() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastHeartbeat = time.Now()
}

// LastHeartbeat returns the time of the last heartbeat.
func (w *LocalWorker) LastHeartbeat() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastHeartbeat
}

// IsStalled returns true if no heartbeat received within the threshold.
func (w *LocalWorker) IsStalled(threshold time.Duration) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Not stalled if not started or not running
	if w.state != WorkerStateRunning {
		return false
	}

	// Not stalled if we've never received a heartbeat (worker might be starting up)
	if w.lastHeartbeat.IsZero() {
		// Use lastActivity as fallback
		if w.lastActivity.IsZero() {
			return false
		}
		return time.Since(w.lastActivity) > threshold
	}

	return time.Since(w.lastHeartbeat) > threshold
}

// Wait blocks until the worker stops.
func (w *LocalWorker) Wait() {
	<-w.done
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
