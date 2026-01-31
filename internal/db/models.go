// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

// User represents an authenticated user
type User struct {
	ID          string
	PublicKey   string // Legacy: Ed25519 public key (may be empty for passkey-only users)
	CreatedAt   time.Time
	LastLoginAt sql.NullTime
}

// WebAuthnCredential represents a stored passkey credential
type WebAuthnCredential struct {
	ID           string
	UserID       string
	CredentialID []byte // Raw credential ID from authenticator
	PublicKey    []byte // COSE-encoded public key
	AttestationType string
	AAGUID       []byte // Authenticator Attestation GUID
	SignCount    uint32 // Signature counter for replay protection
	CreatedAt    time.Time
}

// Project represents a managed project
type Project struct {
	ID            string
	Name          string
	RepoPath      string
	GitHubOwner   sql.NullString
	GitHubRepo    sql.NullString
	DefaultBranch string
	Services      ProjectServices
	CreatedAt     time.Time
}

// ProjectServices tracks which toolbelt services are used by a project
type ProjectServices struct {
	FlyApp             *string `json:"fly_app,omitempty"`
	NeonProject        *string `json:"neon_project,omitempty"`
	NeonDatabase       *string `json:"neon_database,omitempty"`
	UpstashRedis       *string `json:"upstash_redis,omitempty"`
	CloudflareDomain   *string `json:"cloudflare_domain,omitempty"`
	DopplerProject     *string `json:"doppler_project,omitempty"`
	BetterStackMonitor *string `json:"better_stack_monitor,omitempty"`
	ResendDomain       *string `json:"resend_domain,omitempty"`
}

// Task represents a work item
type Task struct {
	ID                string
	ProjectID         string
	GitHubIssueNumber sql.NullInt64
	Title             string
	Description       sql.NullString
	ParentID          sql.NullString
	Type              string // epic, feature, bug, task, chore
	Hat               sql.NullString
	Priority          int // 1-5 (1 highest)
	AutonomyLevel     int // 0-3
	Status            string
	BaseBranch        string
	WorktreePath      sql.NullString
	BranchName        sql.NullString
	PRNumber          sql.NullInt64
	TokenBudget       sql.NullInt64
	TokenUsed         int64
	TimeBudgetMin     sql.NullInt64
	TimeUsedMin       int64
	DollarBudget      sql.NullFloat64
	DollarUsed        float64
	CreatedAt         time.Time
	StartedAt         sql.NullTime
	CompletedAt       sql.NullTime
}

// TaskDependency represents a blocker relationship between tasks
type TaskDependency struct {
	BlockerID string
	BlockedID string
	CreatedAt time.Time
}

// Session represents a Claude session working on a task
type Session struct {
	ID                string
	TaskID            string
	Hat               string
	ClaudeSessionID   sql.NullString
	Status            string
	WorktreePath      string
	IterationCount    int
	MaxIterations     int
	CompletionPromise sql.NullString
	TokensUsed        int64
	TokensBudget      sql.NullInt64
	DollarsUsed       float64
	DollarsBudget     sql.NullFloat64
	CreatedAt         time.Time
	StartedAt         sql.NullTime
	EndedAt           sql.NullTime
	Outcome           sql.NullString
}

// SessionCheckpoint represents a saved state of a session
type SessionCheckpoint struct {
	ID        string
	SessionID string
	Iteration int
	State     json.RawMessage
	CreatedAt time.Time
}

// Approval represents a pending approval request
type Approval struct {
	ID          string
	TaskID      sql.NullString
	SessionID   sql.NullString
	Type        string // commit, hat_transition, pr, merge, conflict_resolution
	Title       string
	Description sql.NullString
	Data        json.RawMessage
	Status      string // pending, approved, rejected
	CreatedAt   time.Time
	ResolvedAt  sql.NullTime
}

// PlanningSession represents a planning phase for a task
type PlanningSession struct {
	ID             string
	TaskID         string
	Status         string // processing, awaiting_response, completed, skipped
	RefinedPrompt  sql.NullString
	OriginalPrompt string
	CreatedAt      time.Time
	CompletedAt    sql.NullTime
}

// PlanningMessage represents a message in a planning session conversation
type PlanningMessage struct {
	ID                string
	PlanningSessionID string
	Role              string // user, assistant
	Content           string
	CreatedAt         time.Time
}

// TaskChecklist represents a structured checklist for a task
type TaskChecklist struct {
	ID        string
	TaskID    string
	CreatedAt time.Time
}

// ChecklistItem represents an individual item in a checklist
type ChecklistItem struct {
	ID                string
	ChecklistID       string
	ParentID          sql.NullString
	Description       string
	Category          string // must_have, optional
	Selected          bool
	Status            string // pending, in_progress, done, failed, skipped
	VerificationNotes sql.NullString
	CompletedAt       sql.NullTime
	SortOrder         int
}

// GetParentID returns the parent ID string, or empty if null
func (c *ChecklistItem) GetParentID() string {
	if c.ParentID.Valid {
		return c.ParentID.String
	}
	return ""
}

// GetVerificationNotes returns the verification notes string, or empty if null
func (c *ChecklistItem) GetVerificationNotes() string {
	if c.VerificationNotes.Valid {
		return c.VerificationNotes.String
	}
	return ""
}

// Task status constants
const (
	TaskStatusPending     = "pending"
	TaskStatusPlanning    = "planning"
	TaskStatusBlocked     = "blocked"
	TaskStatusReady       = "ready"
	TaskStatusRunning     = "running"
	TaskStatusPaused      = "paused"
	TaskStatusQuarantined = "quarantined"
	TaskStatusCompleted   = "completed"
	TaskStatusCancelled   = "cancelled"
)

// Task type constants
const (
	TaskTypeEpic    = "epic"
	TaskTypeFeature = "feature"
	TaskTypeBug     = "bug"
	TaskTypeTask    = "task"
	TaskTypeChore   = "chore"
)

// Session status constants
const (
	SessionStatusPending   = "pending"
	SessionStatusRunning   = "running"
	SessionStatusPaused    = "paused"
	SessionStatusCompleted = "completed"
	SessionStatusFailed    = "failed"
)

// Approval type constants
const (
	ApprovalTypeCommit             = "commit"
	ApprovalTypeHatTransition      = "hat_transition"
	ApprovalTypePR                 = "pr"
	ApprovalTypeMerge              = "merge"
	ApprovalTypeConflictResolution = "conflict_resolution"
)

// Approval status constants
const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

// Planning session status constants
const (
	PlanningStatusProcessing       = "processing"
	PlanningStatusAwaitingResponse = "awaiting_response"
	PlanningStatusCompleted        = "completed"
	PlanningStatusSkipped          = "skipped"
)

// Checklist item category constants
const (
	ChecklistCategoryMustHave = "must_have"
	ChecklistCategoryOptional = "optional"
)

// Checklist item status constants
const (
	ChecklistItemStatusPending    = "pending"
	ChecklistItemStatusInProgress = "in_progress"
	ChecklistItemStatusDone       = "done"
	ChecklistItemStatusFailed     = "failed"
	ChecklistItemStatusSkipped    = "skipped"
)

// Task status for completed with issues
const TaskStatusCompletedWithIssues = "completed_with_issues"

// GetDescription returns the description string, or empty if null
func (t *Task) GetDescription() string {
	if t.Description.Valid {
		return t.Description.String
	}
	return ""
}

// GetBranchName returns the branch name string, or empty if null
func (t *Task) GetBranchName() string {
	if t.BranchName.Valid {
		return t.BranchName.String
	}
	return ""
}

// GetWorktreePath returns the worktree path string, or empty if null
func (t *Task) GetWorktreePath() string {
	if t.WorktreePath.Valid {
		return t.WorktreePath.String
	}
	return ""
}

// GetHat returns the hat string, or empty if null
func (t *Task) GetHat() string {
	if t.Hat.Valid {
		return t.Hat.String
	}
	return ""
}

// GetParentID returns the parent ID string, or empty if null
func (t *Task) GetParentID() string {
	if t.ParentID.Valid {
		return t.ParentID.String
	}
	return ""
}
