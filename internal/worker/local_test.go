package worker

import (
	"testing"
	"time"
)

func TestLocalWorker_NewLocalWorker(t *testing.T) {
	t.Run("With ID", func(t *testing.T) {
		config := &WorkerConfig{ID: "test-worker-1"}
		w := NewLocalWorker(config)

		if w.ID() != "test-worker-1" {
			t.Errorf("expected ID test-worker-1, got %s", w.ID())
		}
		if w.Type() != WorkerTypeLocal {
			t.Errorf("expected type local, got %s", w.Type())
		}
	})

	t.Run("Without ID", func(t *testing.T) {
		config := &WorkerConfig{}
		w := NewLocalWorker(config)

		if w.ID() == "" {
			t.Error("expected generated ID, got empty")
		}
		if w.ID()[:6] != "local-" {
			t.Errorf("expected ID to start with 'local-', got %s", w.ID())
		}
	})
}

func TestLocalWorker_Heartbeat(t *testing.T) {
	config := &WorkerConfig{ID: "test-worker-1"}
	w := NewLocalWorker(config)

	// Initially no heartbeat
	if !w.LastHeartbeat().IsZero() {
		t.Error("expected zero heartbeat time initially")
	}

	// Update heartbeat
	w.UpdateLastHeartbeat()
	hb := w.LastHeartbeat()

	if hb.IsZero() {
		t.Error("expected non-zero heartbeat after update")
	}
	if time.Since(hb) > time.Second {
		t.Error("expected heartbeat to be recent")
	}

	// Update again
	time.Sleep(10 * time.Millisecond)
	w.UpdateLastHeartbeat()
	hb2 := w.LastHeartbeat()

	if !hb2.After(hb) {
		t.Error("expected second heartbeat to be after first")
	}
}

func TestLocalWorker_IsStalled(t *testing.T) {
	config := &WorkerConfig{ID: "test-worker-1"}
	w := NewLocalWorker(config)

	threshold := 100 * time.Millisecond

	t.Run("Not stalled when stopped", func(t *testing.T) {
		// Worker starts in stopped state
		if w.IsStalled(threshold) {
			t.Error("stopped worker should not be considered stalled")
		}
	})

	t.Run("Not stalled when idle", func(t *testing.T) {
		w.mu.Lock()
		w.state = WorkerStateIdle
		w.mu.Unlock()

		if w.IsStalled(threshold) {
			t.Error("idle worker should not be considered stalled")
		}
	})

	t.Run("Not stalled with recent heartbeat", func(t *testing.T) {
		w.mu.Lock()
		w.state = WorkerStateRunning
		w.mu.Unlock()
		w.UpdateLastHeartbeat()

		if w.IsStalled(threshold) {
			t.Error("worker with recent heartbeat should not be stalled")
		}
	})

	t.Run("Stalled with old heartbeat", func(t *testing.T) {
		w.mu.Lock()
		w.state = WorkerStateRunning
		w.lastHeartbeat = time.Now().Add(-200 * time.Millisecond)
		w.mu.Unlock()

		if !w.IsStalled(threshold) {
			t.Error("worker with old heartbeat should be stalled")
		}
	})

	t.Run("Uses lastActivity as fallback", func(t *testing.T) {
		w.mu.Lock()
		w.state = WorkerStateRunning
		w.lastHeartbeat = time.Time{} // Zero heartbeat
		w.lastActivity = time.Now()
		w.mu.Unlock()

		if w.IsStalled(threshold) {
			t.Error("worker with recent activity should not be stalled")
		}

		w.mu.Lock()
		w.lastActivity = time.Now().Add(-200 * time.Millisecond)
		w.mu.Unlock()

		if !w.IsStalled(threshold) {
			t.Error("worker with old activity and no heartbeat should be stalled")
		}
	})
}

func TestLocalWorker_Status(t *testing.T) {
	config := &WorkerConfig{ID: "test-worker-1"}
	w := NewLocalWorker(config)

	// Set some state
	w.mu.Lock()
	w.state = WorkerStateRunning
	w.objectiveID = "obj-123"
	w.sessionID = "sess-456"
	w.iteration = 5
	w.tokensUsed = 1000
	w.version = "1.0.0"
	w.startedAt = time.Now().Add(-5 * time.Minute)
	w.lastActivity = time.Now()
	w.mu.Unlock()

	status := w.Status()

	if status.ID != "test-worker-1" {
		t.Errorf("expected ID test-worker-1, got %s", status.ID)
	}
	if status.Type != WorkerTypeLocal {
		t.Errorf("expected type local, got %s", status.Type)
	}
	if status.State != WorkerStateRunning {
		t.Errorf("expected state running, got %s", status.State)
	}
	if status.ObjectiveID != "obj-123" {
		t.Errorf("expected objective obj-123, got %s", status.ObjectiveID)
	}
	if status.SessionID != "sess-456" {
		t.Errorf("expected session sess-456, got %s", status.SessionID)
	}
	if status.Iteration != 5 {
		t.Errorf("expected iteration 5, got %d", status.Iteration)
	}
	if status.TokensUsed != 1000 {
		t.Errorf("expected tokens 1000, got %d", status.TokensUsed)
	}
	if status.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", status.Version)
	}
	if status.Error != "" {
		t.Errorf("expected no error, got %s", status.Error)
	}
}

func TestLocalWorker_Events(t *testing.T) {
	config := &WorkerConfig{ID: "test-worker-1"}
	w := NewLocalWorker(config)

	events := w.Events()
	if events == nil {
		t.Error("expected non-nil events channel")
	}

	// Verify channel is the internal eventChan (receive-only view)
	// We can test this by sending to the internal channel and receiving from Events()
	go func() {
		w.eventChan <- &Message{Type: MsgTypeProgress}
	}()

	select {
	case msg := <-events:
		if msg.Type != MsgTypeProgress {
			t.Errorf("expected MsgTypeProgress, got %s", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive message from events channel")
	}
}

func TestLocalWorker_Concurrency(t *testing.T) {
	config := &WorkerConfig{ID: "test-worker-1"}
	w := NewLocalWorker(config)

	done := make(chan bool)

	// Concurrent heartbeat updates
	for i := 0; i < 10; i++ {
		go func() {
			w.UpdateLastHeartbeat()
			done <- true
		}()
	}

	// Concurrent status reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = w.Status()
			_ = w.LastHeartbeat()
			_ = w.IsStalled(time.Second)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestErrToString(t *testing.T) {
	if errToString(nil) != "" {
		t.Error("expected empty string for nil error")
	}

	err := &testError{msg: "test error"}
	if errToString(err) != "test error" {
		t.Errorf("expected 'test error', got %q", errToString(err))
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
