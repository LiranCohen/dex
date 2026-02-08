// Package tools provides shared tool infrastructure for Dex
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrBlockingTimeout is returned when a blocking tool times out waiting for input
var ErrBlockingTimeout = errors.New("blocking tool timed out waiting for input")

// ErrBlockingCancelled is returned when a blocking tool is cancelled
var ErrBlockingCancelled = errors.New("blocking tool cancelled")

// ErrNoBlockingContext is returned when a blocking tool is called without a blocking context
var ErrNoBlockingContext = errors.New("blocking tool requires a blocking context")

// BlockingToolAnswer represents a user's answer to a blocking tool
type BlockingToolAnswer struct {
	Answer          string `json:"answer"`
	SelectedIndices []int  `json:"selected_indices,omitempty"`
	IsCustom        bool   `json:"is_custom"`
	Cancelled       bool   `json:"cancelled"`
}

// QuestionOptions represents the options for an ask_question tool call
type QuestionOptions struct {
	Question         string           `json:"question"`
	Header           string           `json:"header,omitempty"`
	Options          []QuestionOption `json:"options,omitempty"`
	AllowMultiple    bool             `json:"allow_multiple,omitempty"`
	AllowCustom      bool             `json:"allow_custom,omitempty"`
	RecommendedIndex *int             `json:"recommended_index,omitempty"`
}

// QuestionOption represents a single option in a question
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// PendingQuestion represents a question waiting for user input
type PendingQuestion struct {
	CallID    string          `json:"call_id"`
	SessionID string          `json:"session_id"`
	Question  QuestionOptions `json:"question"`
	CreatedAt time.Time       `json:"created_at"`
}

// BlockingContext provides context for tools that block for user input
type BlockingContext struct {
	SessionID string
	QuestID   string
	TaskID    string

	// Channels for answer delivery
	answerChan chan BlockingToolAnswer
	cancelChan chan struct{}

	// Test mode support
	TestMode       bool
	TestAnswerFunc func(question QuestionOptions) BlockingToolAnswer

	// Timeout configuration
	Timeout time.Duration

	mu      sync.Mutex
	pending map[string]*PendingQuestion // callID -> pending question
	closed  bool
}

// NewBlockingContext creates a new blocking context
func NewBlockingContext(sessionID, questID, taskID string) *BlockingContext {
	return &BlockingContext{
		SessionID:  sessionID,
		QuestID:    questID,
		TaskID:     taskID,
		answerChan: make(chan BlockingToolAnswer, 1),
		cancelChan: make(chan struct{}),
		Timeout:    5 * time.Minute, // Default timeout
		pending:    make(map[string]*PendingQuestion),
	}
}

// NewTestBlockingContext creates a blocking context for testing
func NewTestBlockingContext(answerFunc func(question QuestionOptions) BlockingToolAnswer) *BlockingContext {
	return &BlockingContext{
		SessionID:      "test-session",
		QuestID:        "test-quest",
		TestMode:       true,
		TestAnswerFunc: answerFunc,
		pending:        make(map[string]*PendingQuestion),
	}
}

// WaitForAnswer blocks until the user provides an answer or timeout/cancel occurs
func (bc *BlockingContext) WaitForAnswer(ctx context.Context, callID string, question QuestionOptions) (BlockingToolAnswer, error) {
	// In test mode, return immediately with test answer
	if bc.TestMode && bc.TestAnswerFunc != nil {
		return bc.TestAnswerFunc(question), nil
	}

	// Register the pending question
	bc.mu.Lock()
	if bc.closed {
		bc.mu.Unlock()
		return BlockingToolAnswer{}, ErrBlockingCancelled
	}
	bc.pending[callID] = &PendingQuestion{
		CallID:    callID,
		SessionID: bc.SessionID,
		Question:  question,
		CreatedAt: time.Now(),
	}
	bc.mu.Unlock()

	// Clean up on exit
	defer func() {
		bc.mu.Lock()
		delete(bc.pending, callID)
		bc.mu.Unlock()
	}()

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, bc.Timeout)
	defer cancel()

	select {
	case answer := <-bc.answerChan:
		if answer.Cancelled {
			return answer, ErrBlockingCancelled
		}
		return answer, nil
	case <-bc.cancelChan:
		return BlockingToolAnswer{Cancelled: true}, ErrBlockingCancelled
	case <-timeoutCtx.Done():
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return BlockingToolAnswer{}, ErrBlockingTimeout
		}
		return BlockingToolAnswer{}, timeoutCtx.Err()
	}
}

// DeliverAnswer delivers an answer to a waiting blocking tool
func (bc *BlockingContext) DeliverAnswer(answer BlockingToolAnswer) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return ErrBlockingCancelled
	}

	select {
	case bc.answerChan <- answer:
		return nil
	default:
		return errors.New("no blocking tool waiting for answer")
	}
}

// Cancel cancels all pending blocking operations
func (bc *BlockingContext) Cancel() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return
	}

	bc.closed = true
	close(bc.cancelChan)
}

// GetPendingQuestions returns all pending questions
func (bc *BlockingContext) GetPendingQuestions() []*PendingQuestion {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	result := make([]*PendingQuestion, 0, len(bc.pending))
	for _, pq := range bc.pending {
		result = append(result, pq)
	}
	return result
}

// HasPending returns true if there are pending questions
func (bc *BlockingContext) HasPending() bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return len(bc.pending) > 0
}

// BlockingToolRegistry manages blocking contexts across sessions
type BlockingToolRegistry struct {
	mu       sync.RWMutex
	contexts map[string]*BlockingContext // sessionID -> context
}

// NewBlockingToolRegistry creates a new registry
func NewBlockingToolRegistry() *BlockingToolRegistry {
	return &BlockingToolRegistry{
		contexts: make(map[string]*BlockingContext),
	}
}

// Register registers a blocking context for a session
func (r *BlockingToolRegistry) Register(ctx *BlockingContext) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contexts[ctx.SessionID] = ctx
}

// Unregister removes a blocking context
func (r *BlockingToolRegistry) Unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ctx, ok := r.contexts[sessionID]; ok {
		ctx.Cancel()
		delete(r.contexts, sessionID)
	}
}

// Get retrieves a blocking context by session ID
func (r *BlockingToolRegistry) Get(sessionID string) *BlockingContext {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.contexts[sessionID]
}

// DeliverAnswer delivers an answer to the appropriate session
func (r *BlockingToolRegistry) DeliverAnswer(sessionID string, answer BlockingToolAnswer) error {
	r.mu.RLock()
	ctx := r.contexts[sessionID]
	r.mu.RUnlock()

	if ctx == nil {
		return fmt.Errorf("no blocking context for session %s", sessionID)
	}

	return ctx.DeliverAnswer(answer)
}

// FormatQuestionResult formats a question answer as a tool result
func FormatQuestionResult(answer BlockingToolAnswer) string {
	result, _ := json.Marshal(map[string]any{
		"answer":           answer.Answer,
		"selected_indices": answer.SelectedIndices,
		"is_custom":        answer.IsCustom,
	})
	return string(result)
}
