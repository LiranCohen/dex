package core

import (
	"encoding/json"
	"time"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/session"
)

// TaskResponse is the JSON response format for tasks.
// This properly handles sql.Null* types for JSON serialization.
type TaskResponse struct {
	ID                string   `json:"ID"`
	ProjectID         string   `json:"ProjectID"`
	QuestID           *string  `json:"QuestID"`
	GitHubIssueNumber *int64   `json:"GitHubIssueNumber"`
	Title             string   `json:"Title"`
	Description       *string  `json:"Description"`
	ParentID          *string  `json:"ParentID"`
	Type              string   `json:"Type"`
	Hat               *string  `json:"Hat"`
	Priority          int      `json:"Priority"`
	AutonomyLevel     int      `json:"AutonomyLevel"`
	Status            string   `json:"Status"`
	BaseBranch        string   `json:"BaseBranch"`
	WorktreePath      *string  `json:"WorktreePath"`
	BranchName        *string  `json:"BranchName"`
	PRNumber          *int64   `json:"PRNumber"`
	TokenBudget       *int64   `json:"TokenBudget"`
	TokenUsed         int64    `json:"TokenUsed"`
	TimeBudgetMin     *int64   `json:"TimeBudgetMin"`
	TimeUsedMin       int64    `json:"TimeUsedMin"`
	DollarBudget      *float64 `json:"DollarBudget"`
	DollarUsed        float64  `json:"DollarUsed"`
	CreatedAt         string   `json:"CreatedAt"`
	StartedAt         *string  `json:"StartedAt"`
	CompletedAt       *string  `json:"CompletedAt"`
	// Derived blocking info - computed from dependencies
	IsBlocked bool     `json:"IsBlocked"`
	BlockedBy []string `json:"BlockedBy,omitempty"`
}

// ToTaskResponse converts a db.Task to TaskResponse for clean JSON.
// Note: This does not populate blocking info. Use ToTaskResponseWithBlocking
// for responses where blocking state matters.
func ToTaskResponse(t *db.Task) TaskResponse {
	resp := TaskResponse{
		ID:            t.ID,
		ProjectID:     t.ProjectID,
		Title:         t.Title,
		Type:          t.Type,
		Priority:      t.Priority,
		AutonomyLevel: t.AutonomyLevel,
		Status:        t.Status,
		BaseBranch:    t.BaseBranch,
		TokenUsed:     t.TokenUsed,
		TimeUsedMin:   t.TimeUsedMin,
		DollarUsed:    t.DollarUsed,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
	}
	if t.QuestID.Valid {
		resp.QuestID = &t.QuestID.String
	}
	if t.GitHubIssueNumber.Valid {
		resp.GitHubIssueNumber = &t.GitHubIssueNumber.Int64
	}
	if t.Description.Valid {
		resp.Description = &t.Description.String
	}
	if t.ParentID.Valid {
		resp.ParentID = &t.ParentID.String
	}
	if t.Hat.Valid {
		resp.Hat = &t.Hat.String
	}
	if t.WorktreePath.Valid {
		resp.WorktreePath = &t.WorktreePath.String
	}
	if t.BranchName.Valid {
		resp.BranchName = &t.BranchName.String
	}
	if t.PRNumber.Valid {
		resp.PRNumber = &t.PRNumber.Int64
	}
	if t.TokenBudget.Valid {
		resp.TokenBudget = &t.TokenBudget.Int64
	}
	if t.TimeBudgetMin.Valid {
		resp.TimeBudgetMin = &t.TimeBudgetMin.Int64
	}
	if t.DollarBudget.Valid {
		resp.DollarBudget = &t.DollarBudget.Float64
	}
	if t.StartedAt.Valid {
		s := t.StartedAt.Time.Format(time.RFC3339)
		resp.StartedAt = &s
	}
	if t.CompletedAt.Valid {
		s := t.CompletedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &s
	}
	return resp
}

// ToTaskResponseWithBlocking converts a db.Task to TaskResponse with blocking info.
// blockerIDs should be the list of incomplete blocker task IDs (from GetIncompleteBlockerIDs).
func ToTaskResponseWithBlocking(t *db.Task, blockerIDs []string) TaskResponse {
	resp := ToTaskResponse(t)
	resp.IsBlocked = len(blockerIDs) > 0
	resp.BlockedBy = blockerIDs
	return resp
}

// ApprovalResponse is the JSON response format for approvals.
type ApprovalResponse struct {
	ID          string          `json:"id"`
	TaskID      *string         `json:"task_id,omitempty"`
	SessionID   *string         `json:"session_id,omitempty"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description *string         `json:"description,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
}

// ToApprovalResponse converts a db.Approval to ApprovalResponse for clean JSON.
func ToApprovalResponse(a *db.Approval) ApprovalResponse {
	resp := ApprovalResponse{
		ID:        a.ID,
		Type:      a.Type,
		Title:     a.Title,
		Data:      a.Data,
		Status:    a.Status,
		CreatedAt: a.CreatedAt,
	}
	if a.TaskID.Valid {
		resp.TaskID = &a.TaskID.String
	}
	if a.SessionID.Valid {
		resp.SessionID = &a.SessionID.String
	}
	if a.Description.Valid {
		resp.Description = &a.Description.String
	}
	if a.ResolvedAt.Valid {
		resp.ResolvedAt = &a.ResolvedAt.Time
	}
	return resp
}

// SessionResponse is the JSON response format for sessions.
type SessionResponse struct {
	ID             string   `json:"id"`
	TaskID         string   `json:"task_id"`
	Hat            string   `json:"hat"`
	State          string   `json:"state"`
	WorktreePath   string   `json:"worktree_path"`
	IterationCount int      `json:"iteration_count"`
	MaxIterations  int      `json:"max_iterations"`
	InputTokens    int64    `json:"input_tokens"`
	OutputTokens   int64    `json:"output_tokens"`
	TokensUsed     int64    `json:"tokens_used"`
	TokensBudget   *int64   `json:"tokens_budget,omitempty"`
	DollarsUsed    float64  `json:"dollars_used"`
	DollarsBudget  *float64 `json:"dollars_budget,omitempty"`
	StartedAt      string   `json:"started_at,omitempty"`
	LastActivity   string   `json:"last_activity,omitempty"`
}

// ToSessionResponse converts an ActiveSession to SessionResponse for clean JSON.
func ToSessionResponse(s *session.ActiveSession) SessionResponse {
	resp := SessionResponse{
		ID:             s.ID,
		TaskID:         s.TaskID,
		Hat:            s.Hat,
		State:          string(s.State),
		WorktreePath:   s.WorktreePath,
		IterationCount: s.IterationCount,
		MaxIterations:  s.MaxIterations,
		InputTokens:    s.InputTokens,
		OutputTokens:   s.OutputTokens,
		TokensUsed:     s.TotalTokens(),
		TokensBudget:   s.TokensBudget,
		DollarsUsed:    s.Cost(),
		DollarsBudget:  s.DollarsBudget,
	}
	if !s.StartedAt.IsZero() {
		resp.StartedAt = s.StartedAt.Format(time.RFC3339)
	}
	if !s.LastActivity.IsZero() {
		resp.LastActivity = s.LastActivity.Format(time.RFC3339)
	}
	return resp
}

// ActivityResponse is the JSON response format for session activity.
type ActivityResponse struct {
	ID           string  `json:"id"`
	SessionID    string  `json:"session_id"`
	Iteration    int     `json:"iteration"`
	EventType    string  `json:"event_type"`
	Hat          *string `json:"hat,omitempty"`
	Content      *string `json:"content,omitempty"`
	TokensInput  *int64  `json:"tokens_input,omitempty"`
	TokensOutput *int64  `json:"tokens_output,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// ToActivityResponse converts a db.SessionActivity to ActivityResponse.
func ToActivityResponse(a *db.SessionActivity) ActivityResponse {
	resp := ActivityResponse{
		ID:        a.ID,
		SessionID: a.SessionID,
		Iteration: a.Iteration,
		EventType: a.EventType,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
	if a.Hat.Valid {
		resp.Hat = &a.Hat.String
	}
	if a.Content.Valid {
		resp.Content = &a.Content.String
	}
	if a.TokensInput.Valid {
		resp.TokensInput = &a.TokensInput.Int64
	}
	if a.TokensOutput.Valid {
		resp.TokensOutput = &a.TokensOutput.Int64
	}
	return resp
}

// ChecklistItemResponse is the JSON response format for checklist items.
type ChecklistItemResponse struct {
	ID                string  `json:"id"`
	ChecklistID       string  `json:"checklist_id"`
	ParentID          *string `json:"parent_id,omitempty"`
	Description       string  `json:"description"`
	Status            string  `json:"status"`
	VerificationNotes *string `json:"verification_notes,omitempty"`
	CompletedAt       *string `json:"completed_at,omitempty"`
	SortOrder         int     `json:"sort_order"`
}

// ToChecklistItemResponse converts a db.ChecklistItem to ChecklistItemResponse.
func ToChecklistItemResponse(item *db.ChecklistItem) ChecklistItemResponse {
	resp := ChecklistItemResponse{
		ID:          item.ID,
		ChecklistID: item.ChecklistID,
		Description: item.Description,
		Status:      item.Status,
		SortOrder:   item.SortOrder,
	}
	if item.ParentID.Valid {
		resp.ParentID = &item.ParentID.String
	}
	if item.VerificationNotes.Valid {
		resp.VerificationNotes = &item.VerificationNotes.String
	}
	if item.CompletedAt.Valid {
		s := item.CompletedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &s
	}
	return resp
}

// QuestResponse is the JSON response format for quests.
type QuestResponse struct {
	ID               string               `json:"id"`
	ProjectID        string               `json:"project_id"`
	Title            string               `json:"title,omitempty"`
	Status           string               `json:"status"`
	Model            string               `json:"model"`
	AutoStartDefault bool                 `json:"auto_start_default"`
	CreatedAt        time.Time            `json:"created_at"`
	CompletedAt      *time.Time           `json:"completed_at,omitempty"`
	Summary          *QuestSummaryResponse `json:"summary,omitempty"`
}

// QuestSummaryResponse is the summary of a quest's task progress.
type QuestSummaryResponse struct {
	TotalTasks       int     `json:"total_tasks"`
	CompletedTasks   int     `json:"completed_tasks"`
	RunningTasks     int     `json:"running_tasks"`
	FailedTasks      int     `json:"failed_tasks"`
	BlockedTasks     int     `json:"blocked_tasks"`
	PendingTasks     int     `json:"pending_tasks"`
	TotalDollarsUsed float64 `json:"total_dollars_used"`
}

// QuestToolCallResponse is the JSON response format for quest tool calls.
type QuestToolCallResponse struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input"`
	Output     string         `json:"output"`
	IsError    bool           `json:"is_error"`
	DurationMs int64          `json:"duration_ms"`
}

// QuestMessageResponse is the JSON response format for quest messages.
type QuestMessageResponse struct {
	ID        string                  `json:"id"`
	QuestID   string                  `json:"quest_id"`
	Role      string                  `json:"role"`
	Content   string                  `json:"content"`
	ToolCalls []QuestToolCallResponse `json:"tool_calls,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
}

// ToQuestResponse converts a db.Quest and optional summary to QuestResponse.
func ToQuestResponse(q *db.Quest, summary *db.QuestSummary) QuestResponse {
	resp := QuestResponse{
		ID:               q.ID,
		ProjectID:        q.ProjectID,
		Title:            q.GetTitle(),
		Status:           q.Status,
		Model:            q.Model,
		AutoStartDefault: q.AutoStartDefault,
		CreatedAt:        q.CreatedAt,
	}
	if q.CompletedAt.Valid {
		resp.CompletedAt = &q.CompletedAt.Time
	}
	if summary != nil {
		resp.Summary = &QuestSummaryResponse{
			TotalTasks:       summary.TotalTasks,
			CompletedTasks:   summary.CompletedTasks,
			RunningTasks:     summary.RunningTasks,
			FailedTasks:      summary.FailedTasks,
			BlockedTasks:     summary.BlockedTasks,
			PendingTasks:     summary.PendingTasks,
			TotalDollarsUsed: summary.TotalDollarsUsed,
		}
	}
	return resp
}

// ToQuestMessageResponse converts a db.QuestMessage to QuestMessageResponse.
func ToQuestMessageResponse(m *db.QuestMessage) QuestMessageResponse {
	resp := QuestMessageResponse{
		ID:        m.ID,
		QuestID:   m.QuestID,
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}

	if len(m.ToolCalls) > 0 {
		resp.ToolCalls = make([]QuestToolCallResponse, len(m.ToolCalls))
		for i, tc := range m.ToolCalls {
			resp.ToolCalls[i] = QuestToolCallResponse{
				ToolName:   tc.ToolName,
				Input:      tc.Input,
				Output:     tc.Output,
				IsError:    tc.IsError,
				DurationMs: tc.DurationMs,
			}
		}
	}

	return resp
}
