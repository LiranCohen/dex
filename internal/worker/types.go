// Package worker provides types and utilities for Dex worker nodes.
// Workers are remote execution environments that receive objectives from HQ,
// execute them locally, and report results back.
package worker

import (
	"time"
)

// ObjectivePayload is the data structure sent from HQ to workers.
// Secrets are encrypted with the worker's public key.
type ObjectivePayload struct {
	// Objective contains the task to execute
	Objective Objective `json:"objective"`

	// Project contains project metadata needed for execution
	Project Project `json:"project"`

	// SecretsEncrypted contains NaCl-encrypted secrets
	// Only the target worker can decrypt this using their private key
	SecretsEncrypted string `json:"secrets_encrypted"`

	// Sync contains HQ sync configuration
	Sync SyncConfig `json:"sync"`

	// DispatchedAt is when HQ sent this payload
	DispatchedAt time.Time `json:"dispatched_at"`

	// HQPublicKey is HQ's public key for the worker to encrypt responses
	HQPublicKey string `json:"hq_public_key"`
}

// Objective represents a task to be executed by the worker.
type Objective struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Hat         string   `json:"hat"`
	BaseBranch  string   `json:"base_branch"`
	TokenBudget int      `json:"token_budget,omitempty"`
	Checklist   []string `json:"checklist,omitempty"`
}

// Project contains project metadata needed for execution.
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	GitHubOwner string `json:"github_owner"`
	GitHubRepo  string `json:"github_repo"`
	CloneURL    string `json:"clone_url"`
}

// SyncConfig contains HQ sync configuration.
type SyncConfig struct {
	// HQEndpoint is the mesh address for syncing back to HQ
	HQEndpoint string `json:"hq_endpoint"`

	// ActivityIntervalSec is how often to sync activity (0 = only on completion)
	ActivityIntervalSec int `json:"activity_interval_sec"`

	// HeartbeatIntervalSec is how often to send heartbeats
	HeartbeatIntervalSec int `json:"heartbeat_interval_sec"`
}

// WorkerSecrets contains the decrypted secrets needed for execution.
type WorkerSecrets struct {
	AnthropicKey string `json:"anthropic_key"`
	GitHubToken  string `json:"github_token"`

	// Optional service credentials
	FlyToken        string `json:"fly_token,omitempty"`
	CloudflareToken string `json:"cloudflare_token,omitempty"`
}

// ActivityEvent represents an event to sync back to HQ.
type ActivityEvent struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	ObjectiveID  string    `json:"objective_id"`
	Iteration    int       `json:"iteration"`
	EventType    string    `json:"event_type"`
	Content      string    `json:"content,omitempty"`
	TokensInput  int       `json:"tokens_input,omitempty"`
	TokensOutput int       `json:"tokens_output,omitempty"`
	Hat          string    `json:"hat,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// StatusUpdate represents a status update to sync to HQ.
type StatusUpdate struct {
	ObjectiveID string    `json:"objective_id"`
	SessionID   string    `json:"session_id"`
	Status      string    `json:"status"`
	Progress    float64   `json:"progress,omitempty"` // 0-1
	Message     string    `json:"message,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CompletionReport represents the final report sent to HQ when an objective completes.
type CompletionReport struct {
	ObjectiveID   string          `json:"objective_id"`
	SessionID     string          `json:"session_id"`
	Status        string          `json:"status"` // completed, failed, cancelled
	Summary       string          `json:"summary"`
	PRNumber      int             `json:"pr_number,omitempty"`
	PRURL         string          `json:"pr_url,omitempty"`
	BranchName    string          `json:"branch_name,omitempty"`
	TotalTokens   int             `json:"total_tokens"`
	TotalCost     float64         `json:"total_cost"`
	Iterations    int             `json:"iterations"`
	ChecklistDone []string        `json:"checklist_done,omitempty"`
	Errors        []string        `json:"errors,omitempty"`
	Activities    []ActivityEvent `json:"activities"` // Final batch of unsynced activities
	CompletedAt   time.Time       `json:"completed_at"`
}

// HeartbeatMessage is sent periodically to HQ to indicate the worker is alive.
type Heartbeat struct {
	WorkerID     string    `json:"worker_id"`
	ObjectiveID  string    `json:"objective_id,omitempty"` // Empty if idle
	SessionID    string    `json:"session_id,omitempty"`
	Status       string    `json:"status"` // idle, running, error
	MeshIP       string    `json:"mesh_ip"`
	Iteration    int       `json:"iteration,omitempty"`
	TokensUsed   int       `json:"tokens_used,omitempty"`
	LastActivity time.Time `json:"last_activity,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// EnrollmentRequest is sent by a worker to HQ to request enrollment.
type EnrollmentRequest struct {
	WorkerID  string    `json:"worker_id"`
	Hostname  string    `json:"hostname"`
	PublicKey string    `json:"public_key"` // Base64-encoded NaCl public key
	MeshIP    string    `json:"mesh_ip"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EnrollmentResponse is sent by HQ in response to an enrollment request.
type EnrollmentResponse struct {
	Approved    bool   `json:"approved"`
	Message     string `json:"message,omitempty"`
	HQPublicKey string `json:"hq_public_key"` // For worker to encrypt messages to HQ
}
