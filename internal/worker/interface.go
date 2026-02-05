package worker

import (
	"context"
	"time"
)

// Worker is the interface implemented by both local (subprocess) and remote workers.
// This uniform interface allows HQ to treat all workers the same regardless of where
// they're running.
type Worker interface {
	// ID returns the unique identifier for this worker.
	ID() string

	// Type returns the worker type (local, remote, etc.)
	Type() WorkerType

	// Dispatch sends an objective payload to the worker for execution.
	// The worker will decrypt secrets using its private key and begin execution.
	Dispatch(ctx context.Context, payload *ObjectivePayload) error

	// Status returns the current status of the worker.
	Status() *WorkerStatus

	// Cancel cancels the currently running objective (if any).
	Cancel(ctx context.Context) error

	// Stop gracefully stops the worker.
	// For subprocesses, this sends a shutdown signal and waits for clean exit.
	// For remote workers, this disconnects from the mesh.
	Stop(ctx context.Context) error

	// PublicKey returns the worker's public key for encrypting payloads.
	// Returns empty string if the worker doesn't have a key yet (not ready).
	PublicKey() string
}

// WorkerType identifies how the worker is connected.
type WorkerType string

const (
	// WorkerTypeLocal is a subprocess on the same machine as HQ.
	WorkerTypeLocal WorkerType = "local"

	// WorkerTypeRemote is a worker connected via mesh network.
	WorkerTypeRemote WorkerType = "remote"

	// WorkerTypeInProcess is a goroutine in the same process (legacy/dev mode).
	WorkerTypeInProcess WorkerType = "in_process"
)

// WorkerState represents the current state of a worker.
type WorkerState string

const (
	WorkerStateStarting WorkerState = "starting" // Worker is initializing
	WorkerStateIdle     WorkerState = "idle"     // Ready to accept work
	WorkerStateRunning  WorkerState = "running"  // Executing an objective
	WorkerStateStopping WorkerState = "stopping" // Gracefully shutting down
	WorkerStateStopped  WorkerState = "stopped"  // Not running
	WorkerStateError    WorkerState = "error"    // In error state
)

// WorkerStatus contains the current status of a worker.
type WorkerStatus struct {
	ID           string      `json:"id"`
	Type         WorkerType  `json:"type"`
	State        WorkerState `json:"state"`
	Hostname     string      `json:"hostname,omitempty"`
	MeshIP       string      `json:"mesh_ip,omitempty"`
	ObjectiveID  string      `json:"objective_id,omitempty"`  // Current objective (if running)
	SessionID    string      `json:"session_id,omitempty"`    // Current session (if running)
	Iteration    int         `json:"iteration,omitempty"`     // Current iteration
	TokensUsed   int         `json:"tokens_used,omitempty"`   // Tokens used in current objective
	LastActivity time.Time   `json:"last_activity,omitempty"` // Last activity timestamp
	StartedAt    time.Time   `json:"started_at,omitempty"`    // When worker started
	Error        string      `json:"error,omitempty"`         // Error message if in error state
	Version      string      `json:"version,omitempty"`       // Worker binary version
}

// WorkerConfig contains configuration for spawning a worker.
type WorkerConfig struct {
	// ID is the unique identifier for this worker.
	// If empty, a UUID will be generated.
	ID string

	// Type specifies how the worker should be created.
	Type WorkerType

	// For local workers:
	BinaryPath  string  // Path to dex-worker binary (default: find in PATH)
	DataDir     string  // Worker's data directory
	MaxMemoryMB int     // Memory limit in MB (0 = no limit)
	MaxCPU      float64 // CPU limit as fraction (0 = no limit)

	// For remote workers:
	MeshIP    string // Mesh IP address of remote worker
	PublicKey string // Base64-encoded public key for encryption

	// Common:
	HQPublicKey string // HQ's public key for worker to encrypt responses
}

// ManagerConfig contains configuration for the WorkerManager.
type ManagerConfig struct {
	// MaxLocalWorkers is the maximum number of local subprocess workers.
	// Default: number of CPUs
	MaxLocalWorkers int

	// MaxRemoteWorkers is the maximum number of remote workers to track.
	// Default: unlimited (0)
	MaxRemoteWorkers int

	// WorkerBinaryPath is the path to the dex-worker binary.
	// Default: search in PATH
	WorkerBinaryPath string

	// WorkerDataDir is the base directory for worker data.
	// Each worker gets a subdirectory: {WorkerDataDir}/{worker-id}/
	WorkerDataDir string

	// SpawnTimeout is how long to wait for a worker to start.
	// Default: 30 seconds
	SpawnTimeout time.Duration

	// HealthCheckInterval is how often to check worker health.
	// Default: 10 seconds
	HealthCheckInterval time.Duration

	// HQKeyPair is HQ's keypair for encrypting payloads.
	HQPublicKey string
}

// DefaultManagerConfig returns a ManagerConfig with sensible defaults.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		MaxLocalWorkers:     4, // Conservative default
		MaxRemoteWorkers:    0, // Unlimited
		SpawnTimeout:        30 * time.Second,
		HealthCheckInterval: 10 * time.Second,
	}
}
