package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
	"github.com/lirancohen/dex/internal/db"
)

// Manager manages a pool of workers (both local and remote).
// It handles worker lifecycle, dispatching objectives, and syncing activity back to HQ.
type Manager struct {
	db        *db.DB
	config    *ManagerConfig
	hqKeyPair *crypto.KeyPair

	workers    map[string]Worker     // All workers by ID
	localPool  []*LocalWorker        // Local subprocess workers
	remotePool []*RemoteWorker       // Remote mesh workers
	queue      chan *dispatchRequest // Pending dispatch requests

	// Callbacks for events
	onProgress  func(objectiveID string, progress *ProgressPayload)
	onActivity  func(events []*ActivityEvent)
	onCompleted func(report *CompletionReport)
	onFailed    func(objectiveID, sessionID, error string)

	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

type dispatchRequest struct {
	payload  *ObjectivePayload
	secrets  *WorkerSecrets // Unencrypted secrets (will be encrypted per-worker)
	response chan error
}

// NewManager creates a new worker manager.
func NewManager(database *db.DB, config *ManagerConfig, hqKeyPair *crypto.KeyPair) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}

	return &Manager{
		db:        database,
		config:    config,
		hqKeyPair: hqKeyPair,
		workers:   make(map[string]Worker),
		queue:     make(chan *dispatchRequest, 100),
	}
}

// SetCallbacks sets the callback functions for worker events.
func (m *Manager) SetCallbacks(
	onProgress func(objectiveID string, progress *ProgressPayload),
	onActivity func(events []*ActivityEvent),
	onCompleted func(report *CompletionReport),
	onFailed func(objectiveID, sessionID, error string),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onProgress = onProgress
	m.onActivity = onActivity
	m.onCompleted = onCompleted
	m.onFailed = onFailed
}

// Start initializes the worker pool and starts the dispatch loop.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("manager already started")
	}
	m.started = true
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	// Spawn initial local workers
	for i := 0; i < m.config.MaxLocalWorkers; i++ {
		if err := m.spawnLocalWorker(); err != nil {
			fmt.Printf("Warning: failed to spawn local worker %d: %v\n", i, err)
		}
	}

	// Start dispatch loop
	m.wg.Add(1)
	go m.dispatchLoop()

	// Start health check loop
	m.wg.Add(1)
	go m.healthCheckLoop()

	return nil
}

// Stop gracefully shuts down all workers and the manager.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.cancel()
	m.mu.Unlock()

	// Stop all workers
	var wg sync.WaitGroup
	m.mu.RLock()
	for _, w := range m.workers {
		wg.Add(1)
		go func(worker Worker) {
			defer wg.Done()
			_ = worker.Stop(ctx)
		}(w)
	}
	m.mu.RUnlock()

	// Wait for workers to stop
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for goroutines
	m.wg.Wait()

	return nil
}

// spawnLocalWorker creates and starts a new local worker.
func (m *Manager) spawnLocalWorker() error {
	workerID := fmt.Sprintf("local-%d", time.Now().UnixNano())

	dataDir := ""
	if m.config.WorkerDataDir != "" {
		dataDir = filepath.Join(m.config.WorkerDataDir, workerID)
	}

	config := &WorkerConfig{
		ID:          workerID,
		Type:        WorkerTypeLocal,
		BinaryPath:  m.config.WorkerBinaryPath,
		DataDir:     dataDir,
		HQPublicKey: m.config.HQPublicKey,
	}

	worker := NewLocalWorker(config)

	if err := worker.Start(m.ctx); err != nil {
		return err
	}

	m.mu.Lock()
	m.workers[workerID] = worker
	m.localPool = append(m.localPool, worker)
	m.mu.Unlock()

	// Start event handler for this worker
	m.wg.Add(1)
	go m.handleWorkerEvents(worker)

	return nil
}

// handleWorkerEvents processes events from a worker.
func (m *Manager) handleWorkerEvents(worker *LocalWorker) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case msg, ok := <-worker.Events():
			if !ok {
				return
			}
			m.processWorkerMessage(worker.ID(), msg)
		}
	}
}

// processWorkerMessage handles a message received from a worker.
func (m *Manager) processWorkerMessage(workerID string, msg *Message) {
	switch msg.Type {
	case MsgTypeProgress:
		payload, err := ParsePayload[ProgressPayload](msg)
		if err == nil && m.onProgress != nil {
			m.onProgress(payload.ObjectiveID, payload)
		}

	case MsgTypeActivity:
		payload, err := ParsePayload[ActivityPayload](msg)
		if err == nil && m.onActivity != nil {
			m.onActivity(payload.Events)
		}

	case MsgTypeCompleted:
		payload, err := ParsePayload[CompletedPayload](msg)
		if err == nil && m.onCompleted != nil {
			m.onCompleted(payload.Report)
		}

	case MsgTypeFailed:
		payload, err := ParsePayload[FailedPayload](msg)
		if err == nil && m.onFailed != nil {
			m.onFailed(payload.ObjectiveID, payload.SessionID, payload.Error)
		}

	case MsgTypeError:
		payload, err := ParsePayload[ErrorPayload](msg)
		if err == nil {
			fmt.Printf("Worker %s error: %s: %s\n", workerID, payload.Code, payload.Message)
		}
	}
}

// dispatchLoop processes the dispatch queue.
func (m *Manager) dispatchLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case req := <-m.queue:
			err := m.dispatchToWorkerWithSecrets(req.payload, req.secrets)
			req.response <- err
		}
	}
}

// dispatchToWorker finds an available worker and dispatches the objective.
// Deprecated: Use dispatchToWorkerWithSecrets instead.
func (m *Manager) dispatchToWorker(payload *ObjectivePayload) error {
	return m.dispatchToWorkerWithSecrets(payload, nil)
}

// dispatchToWorkerWithSecrets finds an available worker, encrypts secrets, and dispatches.
func (m *Manager) dispatchToWorkerWithSecrets(payload *ObjectivePayload, secrets *WorkerSecrets) error {
	// Find an idle worker
	worker := m.getIdleWorker()
	if worker == nil {
		return fmt.Errorf("no idle workers available")
	}

	// If secrets are provided and worker has a public key, encrypt them
	if secrets != nil && worker.PublicKey() != "" {
		dispatcher := NewDispatcher(m.hqKeyPair)
		encPayload, err := dispatcher.PreparePayload(
			payload.Objective,
			payload.Project,
			*secrets,
			worker.PublicKey(),
			payload.Sync,
		)
		if err != nil {
			return fmt.Errorf("failed to encrypt payload: %w", err)
		}
		payload = encPayload
	}

	return worker.Dispatch(m.ctx, payload)
}

// getIdleWorker returns an idle worker, preferring local workers.
func (m *Manager) getIdleWorker() Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check local workers first
	for _, w := range m.localPool {
		if w.Status().State == WorkerStateIdle {
			return w
		}
	}

	// Check remote workers
	for _, w := range m.remotePool {
		if w.Status().State == WorkerStateIdle {
			return w
		}
	}

	return nil
}

// Dispatch queues an objective for dispatch to an available worker.
// This is the main entry point for HQ to send work to workers.
// Deprecated: Use DispatchWithSecrets instead.
func (m *Manager) Dispatch(ctx context.Context, payload *ObjectivePayload) error {
	return m.DispatchWithSecrets(ctx, payload, nil)
}

// DispatchWithSecrets queues an objective with secrets for dispatch.
// Secrets are encrypted per-worker using their public key.
func (m *Manager) DispatchWithSecrets(ctx context.Context, payload *ObjectivePayload, secrets *WorkerSecrets) error {
	req := &dispatchRequest{
		payload:  payload,
		secrets:  secrets,
		response: make(chan error, 1),
	}

	select {
	case m.queue <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-req.response:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DispatchImmediate dispatches an objective immediately without queuing.
// Returns an error if no worker is available.
func (m *Manager) DispatchImmediate(ctx context.Context, payload *ObjectivePayload) error {
	return m.dispatchToWorkerWithSecrets(payload, nil)
}

// DispatchImmediateWithSecrets dispatches an objective with secrets immediately.
func (m *Manager) DispatchImmediateWithSecrets(ctx context.Context, payload *ObjectivePayload, secrets *WorkerSecrets) error {
	return m.dispatchToWorkerWithSecrets(payload, secrets)
}

// healthCheckLoop periodically checks worker health and restarts failed workers.
func (m *Manager) healthCheckLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkWorkerHealth()
		}
	}
}

// checkWorkerHealth checks all workers and restarts any that have failed.
func (m *Manager) checkWorkerHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check local workers
	for i, w := range m.localPool {
		status := w.Status()
		if status.State == WorkerStateError || status.State == WorkerStateStopped {
			fmt.Printf("Worker %s is unhealthy (state: %s), restarting...\n", w.ID(), status.State)

			// Remove from pool
			delete(m.workers, w.ID())
			m.localPool = append(m.localPool[:i], m.localPool[i+1:]...)

			// Try to restart (outside lock)
			go func() {
				if err := m.spawnLocalWorker(); err != nil {
					fmt.Printf("Failed to restart worker: %v\n", err)
				}
			}()

			return // Only handle one per tick to avoid issues
		}
	}
}

// Workers returns a list of all worker statuses.
func (m *Manager) Workers() []*WorkerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]*WorkerStatus, 0, len(m.workers))
	for _, w := range m.workers {
		statuses = append(statuses, w.Status())
	}
	return statuses
}

// IdleWorkerCount returns the number of idle workers.
func (m *Manager) IdleWorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, w := range m.workers {
		if w.Status().State == WorkerStateIdle {
			count++
		}
	}
	return count
}

// RunningWorkerCount returns the number of running workers.
func (m *Manager) RunningWorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, w := range m.workers {
		if w.Status().State == WorkerStateRunning {
			count++
		}
	}
	return count
}

// RegisterRemoteWorker registers a remote worker that connected via mesh.
func (m *Manager) RegisterRemoteWorker(worker *RemoteWorker) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.workers[worker.ID()]; exists {
		return fmt.Errorf("worker %s already registered", worker.ID())
	}

	m.workers[worker.ID()] = worker
	m.remotePool = append(m.remotePool, worker)

	// Start event handler
	m.wg.Add(1)
	go m.handleRemoteWorkerEvents(worker)

	return nil
}

// handleRemoteWorkerEvents processes events from a remote worker.
func (m *Manager) handleRemoteWorkerEvents(worker *RemoteWorker) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case msg, ok := <-worker.Events():
			if !ok {
				// Worker disconnected
				m.unregisterRemoteWorker(worker.ID())
				return
			}
			m.processWorkerMessage(worker.ID(), msg)
		}
	}
}

// unregisterRemoteWorker removes a remote worker from the pool.
func (m *Manager) unregisterRemoteWorker(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.workers, id)

	for i, w := range m.remotePool {
		if w.ID() == id {
			m.remotePool = append(m.remotePool[:i], m.remotePool[i+1:]...)
			break
		}
	}
}

// CancelObjective cancels an objective on whatever worker is running it.
func (m *Manager) CancelObjective(ctx context.Context, objectiveID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, w := range m.workers {
		status := w.Status()
		if status.ObjectiveID == objectiveID {
			return w.Cancel(ctx)
		}
	}

	return fmt.Errorf("objective %s not found on any worker", objectiveID)
}
