// Package quest provides Quest conversation handling for Poindexter
package quest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lirancohen/dex/internal/tools"
)

// PendingDraft represents a draft objective waiting for user approval
type PendingDraft struct {
	DraftID   string         `json:"draft_id"`
	Draft     ObjectiveDraft `json:"draft"`
	CreatedAt time.Time      `json:"created_at"`
	Status    string         `json:"status"` // "pending", "accepted", "rejected"
}

// QuestSession manages the state of a quest conversation including blocking tools
type QuestSession struct {
	mu sync.Mutex

	QuestID   string
	SessionID string

	// Blocking context for ask_question tool
	BlockingContext *tools.BlockingContext

	// Pending drafts from propose_objective tool
	PendingDrafts map[string]*PendingDraft

	// Quest ready flag
	IsReady bool
	Summary string
}

// NewQuestSession creates a new quest session
func NewQuestSession(questID string) *QuestSession {
	sessionID := uuid.New().String()
	return &QuestSession{
		QuestID:         questID,
		SessionID:       sessionID,
		BlockingContext: tools.NewBlockingContext(sessionID, questID, ""),
		PendingDrafts:   make(map[string]*PendingDraft),
	}
}

// NewTestQuestSession creates a quest session for testing
func NewTestQuestSession(questID string, answerFunc func(tools.QuestionOptions) tools.BlockingToolAnswer) *QuestSession {
	return &QuestSession{
		QuestID:         questID,
		SessionID:       "test-session",
		BlockingContext: tools.NewTestBlockingContext(answerFunc),
		PendingDrafts:   make(map[string]*PendingDraft),
	}
}

// AddPendingDraft adds a draft to the pending list
func (qs *QuestSession) AddPendingDraft(draft ObjectiveDraft) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.PendingDrafts[draft.DraftID] = &PendingDraft{
		DraftID:   draft.DraftID,
		Draft:     draft,
		CreatedAt: time.Now(),
		Status:    "pending",
	}
}

// GetPendingDraft retrieves a pending draft by ID
func (qs *QuestSession) GetPendingDraft(draftID string) *PendingDraft {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	return qs.PendingDrafts[draftID]
}

// AcceptDraft marks a draft as accepted
func (qs *QuestSession) AcceptDraft(draftID string) (*PendingDraft, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	draft, ok := qs.PendingDrafts[draftID]
	if !ok {
		return nil, fmt.Errorf("draft not found: %s", draftID)
	}
	if draft.Status != "pending" {
		return nil, fmt.Errorf("draft is not pending: %s (status: %s)", draftID, draft.Status)
	}

	draft.Status = "accepted"
	return draft, nil
}

// RejectDraft marks a draft as rejected
func (qs *QuestSession) RejectDraft(draftID string) (*PendingDraft, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	draft, ok := qs.PendingDrafts[draftID]
	if !ok {
		return nil, fmt.Errorf("draft not found: %s", draftID)
	}
	if draft.Status != "pending" {
		return nil, fmt.Errorf("draft is not pending: %s (status: %s)", draftID, draft.Status)
	}

	draft.Status = "rejected"
	return draft, nil
}

// GetAllPendingDrafts returns all pending drafts
func (qs *QuestSession) GetAllPendingDrafts() []*PendingDraft {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	result := make([]*PendingDraft, 0)
	for _, draft := range qs.PendingDrafts {
		if draft.Status == "pending" {
			result = append(result, draft)
		}
	}
	return result
}

// MarkReady marks the quest as ready
func (qs *QuestSession) MarkReady(summary string) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.IsReady = true
	qs.Summary = summary
}

// Cancel cancels the session and any pending blocking operations
func (qs *QuestSession) Cancel() {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if qs.BlockingContext != nil {
		qs.BlockingContext.Cancel()
	}
}

// QuestSessionRegistry manages quest sessions
type QuestSessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*QuestSession // questID -> session
}

// NewQuestSessionRegistry creates a new registry
func NewQuestSessionRegistry() *QuestSessionRegistry {
	return &QuestSessionRegistry{
		sessions: make(map[string]*QuestSession),
	}
}

// GetOrCreate gets an existing session or creates a new one
func (r *QuestSessionRegistry) GetOrCreate(questID string) *QuestSession {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session, ok := r.sessions[questID]; ok {
		return session
	}

	session := NewQuestSession(questID)
	r.sessions[questID] = session
	return session
}

// Get retrieves a session by quest ID
func (r *QuestSessionRegistry) Get(questID string) *QuestSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[questID]
}

// Remove removes and cancels a session
func (r *QuestSessionRegistry) Remove(questID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session, ok := r.sessions[questID]; ok {
		session.Cancel()
		delete(r.sessions, questID)
	}
}

// DeliverAnswer delivers an answer to a quest session's blocking context
func (r *QuestSessionRegistry) DeliverAnswer(questID string, answer tools.BlockingToolAnswer) error {
	r.mu.RLock()
	session := r.sessions[questID]
	r.mu.RUnlock()

	if session == nil {
		return fmt.Errorf("no session for quest: %s", questID)
	}

	return session.BlockingContext.DeliverAnswer(answer)
}

// QuestionBroadcaster is an interface for broadcasting pending questions
type QuestionBroadcaster interface {
	BroadcastPendingQuestion(questID string, callID string, question tools.QuestionOptions)
}

// executeAskQuestion handles the ask_question tool
func executeAskQuestion(ctx context.Context, session *QuestSession, input map[string]any, broadcaster QuestionBroadcaster) (tools.Result, error) {
	// Parse input
	question, _ := input["question"].(string)
	if question == "" {
		return tools.Result{Output: "question is required", IsError: true}, nil
	}

	header, _ := input["header"].(string)
	allowMultiple, _ := input["allow_multiple"].(bool)
	allowCustom := true // default
	if ac, ok := input["allow_custom"].(bool); ok {
		allowCustom = ac
	}

	var recommendedIndex *int
	if ri, ok := input["recommended_index"].(float64); ok {
		idx := int(ri)
		recommendedIndex = &idx
	}

	// Parse options
	var options []tools.QuestionOption
	if opts, ok := input["options"].([]any); ok {
		for _, opt := range opts {
			if optMap, ok := opt.(map[string]any); ok {
				label, _ := optMap["label"].(string)
				description, _ := optMap["description"].(string)
				if label != "" {
					options = append(options, tools.QuestionOption{
						Label:       label,
						Description: description,
					})
				}
			}
		}
	}

	questionOpts := tools.QuestionOptions{
		Question:         question,
		Header:           header,
		Options:          options,
		AllowMultiple:    allowMultiple,
		AllowCustom:      allowCustom,
		RecommendedIndex: recommendedIndex,
	}

	// Generate call ID
	callID := uuid.New().String()

	// Broadcast the pending question BEFORE blocking
	// This allows the frontend to show the question UI while we wait
	if broadcaster != nil {
		broadcaster.BroadcastPendingQuestion(session.QuestID, callID, questionOpts)
	}

	// Wait for user answer (blocking)
	answer, err := session.BlockingContext.WaitForAnswer(ctx, callID, questionOpts)
	if err != nil {
		return tools.Result{Output: fmt.Sprintf("Failed to get answer: %v", err), IsError: true}, err
	}

	// Format result
	return tools.Result{Output: tools.FormatQuestionResult(answer)}, nil
}

// executeProposeObjective handles the propose_objective tool
func executeProposeObjective(session *QuestSession, input map[string]any) tools.Result {
	// Parse required fields
	title, _ := input["title"].(string)
	if title == "" {
		return tools.Result{Output: "title is required", IsError: true}
	}

	hat, _ := input["hat"].(string)
	if hat == "" {
		return tools.Result{Output: "hat is required", IsError: true}
	}

	var checklistMustHave []string
	if items, ok := input["checklist_must_have"].([]any); ok {
		for _, item := range items {
			if s, ok := item.(string); ok {
				checklistMustHave = append(checklistMustHave, s)
			}
		}
	}
	if len(checklistMustHave) == 0 {
		return tools.Result{Output: "checklist_must_have is required and must have at least one item", IsError: true}
	}

	// Parse optional fields
	description, _ := input["description"].(string)

	var checklistOptional []string
	if items, ok := input["checklist_optional"].([]any); ok {
		for _, item := range items {
			if s, ok := item.(string); ok {
				checklistOptional = append(checklistOptional, s)
			}
		}
	}

	var blockedBy []string
	if items, ok := input["blocked_by"].([]any); ok {
		for _, item := range items {
			if s, ok := item.(string); ok {
				blockedBy = append(blockedBy, s)
			}
		}
	}

	autoStart := true // default
	if as, ok := input["auto_start"].(bool); ok {
		autoStart = as
	}

	complexity, _ := input["complexity"].(string)
	if complexity == "" {
		complexity = "simple"
	}

	estimatedIterations := 0
	if ei, ok := input["estimated_iterations"].(float64); ok {
		estimatedIterations = int(ei)
	}

	estimatedBudget := 0.0
	if eb, ok := input["estimated_budget"].(float64); ok {
		estimatedBudget = eb
	}

	gitProvider, _ := input["git_provider"].(string)
	gitOwner, _ := input["git_owner"].(string)
	gitRepo, _ := input["git_repo"].(string)
	cloneURL, _ := input["clone_url"].(string)

	// Generate draft ID
	draftID := uuid.New().String()

	// Create draft
	draft := ObjectiveDraft{
		DraftID:             draftID,
		Title:               title,
		Description:         description,
		Hat:                 hat,
		Checklist:           Checklist{MustHave: checklistMustHave, Optional: checklistOptional},
		BlockedBy:           blockedBy,
		AutoStart:           autoStart,
		Complexity:          complexity,
		EstimatedIterations: estimatedIterations,
		EstimatedBudget:     estimatedBudget,
		GitProvider:         gitProvider,
		GitOwner:            gitOwner,
		GitRepoName:         gitRepo,
		CloneURL:            cloneURL,
	}

	// Add to pending drafts
	session.AddPendingDraft(draft)

	// Return result
	result := map[string]any{
		"draft_id": draftID,
		"status":   "pending",
	}
	output, _ := json.Marshal(result)
	return tools.Result{Output: string(output)}
}

// executeCompleteQuest handles the complete_quest tool
func executeCompleteQuest(session *QuestSession, input map[string]any) tools.Result {
	summary, _ := input["summary"].(string)
	if summary == "" {
		return tools.Result{Output: "summary is required", IsError: true}
	}

	session.MarkReady(summary)

	result := map[string]any{
		"status":  "ready",
		"summary": summary,
	}
	output, _ := json.Marshal(result)
	return tools.Result{Output: string(output)}
}
