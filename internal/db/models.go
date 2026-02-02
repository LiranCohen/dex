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
	ID             string
	Name           string
	RepoPath       string
	GitHubOwner    sql.NullString // GitHub owner/org for this project
	GitHubRepo     sql.NullString // GitHub repo name
	RemoteOrigin   sql.NullString // git remote origin URL (e.g., git@github.com:user/repo.git)
	RemoteUpstream sql.NullString // git remote upstream URL (if fork, e.g., git@github.com:org/repo.git)
	DefaultBranch  string
	Services       ProjectServices
	CreatedAt      time.Time
}

// IsFork returns true if this project has an upstream remote (indicating it's a fork)
func (p *Project) IsFork() bool {
	return p.RemoteUpstream.Valid && p.RemoteUpstream.String != ""
}

// GetOwner returns the GitHub owner or empty string if not set
func (p *Project) GetOwner() string {
	if p.GitHubOwner.Valid {
		return p.GitHubOwner.String
	}
	return ""
}

// GetRepo returns the GitHub repo name or empty string if not set
func (p *Project) GetRepo() string {
	if p.GitHubRepo.Valid {
		return p.GitHubRepo.String
	}
	return ""
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
	QuestID           sql.NullString // Optional: the Quest that spawned this task
	GitHubIssueNumber sql.NullInt64  // GitHub Issue number for this Objective
	Title             string
	Description       sql.NullString
	ParentID          sql.NullString
	Type              string // epic, feature, bug, task, chore
	Hat               sql.NullString
	Model             sql.NullString // AI model to use: "sonnet" (default) or "opus" (complex tasks)
	Priority          int            // 1-5 (1 highest)
	AutonomyLevel     int            // 0-3
	Status            string
	BaseBranch        string
	WorktreePath      sql.NullString
	BranchName        sql.NullString
	ContentPath       sql.NullString // Path to git content (relative to repo): tasks/{task-id}/
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

// GetContentPath returns the content path string, or empty if null
func (t *Task) GetContentPath() string {
	if t.ContentPath.Valid {
		return t.ContentPath.String
	}
	return ""
}

// Task model constants
const (
	TaskModelSonnet = "sonnet" // Fast, capable - for simple/medium tasks
	TaskModelOpus   = "opus"   // Extended thinking - for complex tasks
)

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
	InputTokens       int64   // Total input tokens used
	OutputTokens      int64   // Total output tokens used
	InputRate         float64 // $/MTok for input at session start
	OutputRate        float64 // $/MTok for output at session start
	TokensBudget      sql.NullInt64
	DollarsBudget     sql.NullFloat64
	CreatedAt         time.Time
	StartedAt         sql.NullTime
	EndedAt           sql.NullTime
	Outcome           sql.NullString
}

// Cost calculates the session cost from tokens and rates
func (s *Session) Cost() float64 {
	inputCost := float64(s.InputTokens) * s.InputRate / 1_000_000
	outputCost := float64(s.OutputTokens) * s.OutputRate / 1_000_000
	return inputCost + outputCost
}

// TotalTokens returns the combined input + output tokens
func (s *Session) TotalTokens() int64 {
	return s.InputTokens + s.OutputTokens
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
	ID               string
	TaskID           string
	Status           string // processing, awaiting_response, completed, skipped
	RefinedPrompt    sql.NullString
	OriginalPrompt   string
	PendingChecklist sql.NullString // JSON: transient checklist before acceptance
	CreatedAt        time.Time
	CompletedAt      sql.NullTime
}

// PendingChecklistData represents the transient checklist structure during planning
type PendingChecklistData struct {
	MustHave []string `json:"must_have"`
	Optional []string `json:"optional"`
}

// GetPendingChecklist parses and returns the pending checklist data
func (p *PlanningSession) GetPendingChecklist() *PendingChecklistData {
	if !p.PendingChecklist.Valid {
		return nil
	}
	var data PendingChecklistData
	if err := json.Unmarshal([]byte(p.PendingChecklist.String), &data); err != nil {
		return nil
	}
	return &data
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
// Note: Items are only created after acceptance - category distinction is transient during planning
type ChecklistItem struct {
	ID                string
	ChecklistID       string
	ParentID          sql.NullString
	Description       string
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

// Quest status constants
const (
	QuestStatusActive    = "active"
	QuestStatusCompleted = "completed"
)

// Quest model constants
const (
	QuestModelSonnet = "sonnet"
	QuestModelOpus   = "opus"
)

// Quest represents a conversation with Dex that spawns tasks
type Quest struct {
	ID                string
	ProjectID         string
	Title             sql.NullString
	Status            string
	Model             string
	AutoStartDefault  bool
	ConversationPath  sql.NullString // Path to git conversation file: quests/{quest-id}/conversation.md
	GitHubIssueNumber sql.NullInt64  // GitHub Issue number for this Quest
	CreatedAt         time.Time
	CompletedAt       sql.NullTime
}

// GetTitle returns the title string, or empty if null
func (q *Quest) GetTitle() string {
	if q.Title.Valid {
		return q.Title.String
	}
	return ""
}

// GetConversationPath returns the conversation path string, or empty if null
func (q *Quest) GetConversationPath() string {
	if q.ConversationPath.Valid {
		return q.ConversationPath.String
	}
	return ""
}

// QuestToolCall represents a tool invocation during Quest chat
type QuestToolCall struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input"`
	Output     string         `json:"output"`
	IsError    bool           `json:"is_error"`
	DurationMs int64          `json:"duration_ms"`
}

// QuestMessage represents a message in a Quest conversation
type QuestMessage struct {
	ID        string
	QuestID   string
	Role      string // user, assistant
	Content   string
	ToolCalls []QuestToolCall // Tool calls made during this message (assistant only)
	CreatedAt time.Time
}

// QuestTemplate represents a reusable quest template
type QuestTemplate struct {
	ID            string
	ProjectID     string
	Name          string
	Description   sql.NullString
	InitialPrompt string
	CreatedAt     time.Time
}

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

// GetQuestID returns the quest ID string, or empty if null
func (t *Task) GetQuestID() string {
	if t.QuestID.Valid {
		return t.QuestID.String
	}
	return ""
}
