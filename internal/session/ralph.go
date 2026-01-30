// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liranmauda/dex/internal/api/websocket"
	"github.com/liranmauda/dex/internal/db"
	"github.com/liranmauda/dex/internal/toolbelt"
)

// Completion/transition signals that Ralph looks for in responses
const (
	SignalTaskComplete = "TASK_COMPLETE"
	SignalHatComplete  = "HAT_COMPLETE"
	SignalHatTransition = "HAT_TRANSITION:"
)

// Budget limit errors
var (
	ErrBudgetExceeded    = errors.New("budget exceeded")
	ErrIterationLimit    = errors.New("iteration limit exceeded")
	ErrTokenBudget       = errors.New("token budget exceeded")
	ErrDollarBudget      = errors.New("dollar budget exceeded")
	ErrNoAnthropicClient = errors.New("anthropic client not configured")
)

// RalphLoop orchestrates a session's execution cycle
type RalphLoop struct {
	manager  *Manager
	session  *ActiveSession
	client   *toolbelt.AnthropicClient
	hub      *websocket.Hub
	db       *db.DB

	// Conversation history for multi-turn chat
	messages []toolbelt.AnthropicMessage

	// Checkpoint frequency (save every N iterations)
	checkpointInterval int
}

// NewRalphLoop creates a new RalphLoop for the given session
func NewRalphLoop(manager *Manager, session *ActiveSession, client *toolbelt.AnthropicClient, hub *websocket.Hub, database *db.DB) *RalphLoop {
	return &RalphLoop{
		manager:            manager,
		session:            session,
		client:             client,
		hub:                hub,
		db:                 database,
		messages:           make([]toolbelt.AnthropicMessage, 0),
		checkpointInterval: 5,
	}
}

// Run executes the Ralph loop until completion, error, or budget exceeded
func (r *RalphLoop) Run(ctx context.Context) error {
	if r.client == nil {
		return ErrNoAnthropicClient
	}

	// Build initial system prompt from hat template
	systemPrompt, err := r.buildPrompt()
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	// Broadcast session started event
	r.broadcastEvent(websocket.EventSessionStarted, map[string]any{
		"session_id":    r.session.ID,
		"hat":           r.session.Hat,
		"worktree_path": r.session.WorktreePath,
	})

	// Initialize conversation with context message
	r.messages = append(r.messages, toolbelt.AnthropicMessage{
		Role:    "user",
		Content: "Begin working on the task. Follow your hat instructions and report progress.",
	})

	// Main Ralph loop
	for {
		// 1. Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 2. Check budget limits
		if err := r.checkBudget(); err != nil {
			r.broadcastEvent(websocket.EventApprovalRequired, map[string]any{
				"session_id": r.session.ID,
				"reason":     err.Error(),
			})
			return err
		}

		// 3. Send to Claude
		response, err := r.sendMessage(ctx, systemPrompt)
		if err != nil {
			return fmt.Errorf("claude API error: %w", err)
		}

		// 4. Update usage tracking
		r.session.TokensUsed += int64(response.Usage.InputTokens + response.Usage.OutputTokens)
		r.session.DollarsUsed += r.estimateCost(response.Usage)
		r.session.IterationCount++
		r.session.LastActivity = time.Now()

		// Broadcast iteration event
		r.broadcastEvent(websocket.EventSessionIteration, map[string]any{
			"session_id": r.session.ID,
			"iteration":  r.session.IterationCount,
			"tokens":     r.session.TokensUsed,
		})

		// 5. Get response text
		responseText := response.Text()

		// 6. Add assistant response to conversation
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "assistant",
			Content: responseText,
		})

		// 7. Check for task completion
		if r.detectCompletion(responseText) {
			r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
				"session_id": r.session.ID,
				"outcome":    "completed",
				"iterations": r.session.IterationCount,
			})
			return nil
		}

		// 8. Check for hat transition
		if nextHat := r.detectHatTransition(responseText); nextHat != "" {
			// Store transition for manager to handle
			r.session.Hat = nextHat
			r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
				"session_id": r.session.ID,
				"outcome":    "hat_transition",
				"next_hat":   nextHat,
			})
			return nil
		}

		// 9. Checkpoint periodically
		if r.session.IterationCount%r.checkpointInterval == 0 {
			if err := r.checkpoint(); err != nil {
				// Log but don't fail on checkpoint error
				fmt.Printf("warning: checkpoint failed: %v\n", err)
			}
		}

		// 10. Add continuation prompt for next iteration
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "user",
			Content: "Continue. If the task is complete, output TASK_COMPLETE. If you need to transition to a different hat, output HAT_TRANSITION:<hat_name>.",
		})
	}
}

// checkBudget returns an error if any budget limit is exceeded
func (r *RalphLoop) checkBudget() error {
	// Check iteration limit
	if r.session.MaxIterations > 0 && r.session.IterationCount >= r.session.MaxIterations {
		return ErrIterationLimit
	}

	// Check token budget
	if r.session.TokensBudget != nil && r.session.TokensUsed >= *r.session.TokensBudget {
		return ErrTokenBudget
	}

	// Check dollar budget
	if r.session.DollarsBudget != nil && r.session.DollarsUsed >= *r.session.DollarsBudget {
		return ErrDollarBudget
	}

	return nil
}

// buildPrompt renders the hat template with task context
func (r *RalphLoop) buildPrompt() (string, error) {
	// Guard against nil manager or promptLoader
	if r.manager == nil || r.manager.promptLoader == nil {
		return "", errors.New("manager or prompt loader not initialized")
	}

	// Get task from DB
	task, err := r.db.GetTaskByID(r.session.TaskID)
	if err != nil {
		return "", fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return "", fmt.Errorf("task not found: %s", r.session.TaskID)
	}

	ctx := &PromptContext{
		Task:    task,
		Session: r.session,
	}

	return r.manager.promptLoader.Get(r.session.Hat, ctx)
}

// sendMessage sends the current conversation to Claude
func (r *RalphLoop) sendMessage(ctx context.Context, systemPrompt string) (*toolbelt.AnthropicChatResponse, error) {
	req := &toolbelt.AnthropicChatRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 8192,
		System:    systemPrompt,
		Messages:  r.messages,
	}

	return r.client.Chat(ctx, req)
}

// detectCompletion checks if the response indicates task completion
func (r *RalphLoop) detectCompletion(response string) bool {
	return strings.Contains(response, SignalTaskComplete) ||
		strings.Contains(response, SignalHatComplete)
}

// detectHatTransition parses the response for a hat transition signal
// Returns the next hat name, or empty string if no transition
func (r *RalphLoop) detectHatTransition(response string) string {
	// Look for HAT_TRANSITION:hat_name pattern
	idx := strings.Index(response, SignalHatTransition)
	if idx == -1 {
		return ""
	}

	// Extract hat name after the signal
	remaining := response[idx+len(SignalHatTransition):]

	// Find end of hat name (whitespace or end of string)
	endIdx := strings.IndexAny(remaining, " \t\n\r")
	if endIdx == -1 {
		endIdx = len(remaining)
	}

	hatName := strings.TrimSpace(remaining[:endIdx])

	// Validate hat name
	if IsValidHat(hatName) {
		return hatName
	}

	return ""
}

// checkpoint saves the current session state to the database
func (r *RalphLoop) checkpoint() error {
	// Build checkpoint state
	state := map[string]any{
		"iteration":    r.session.IterationCount,
		"tokens_used":  r.session.TokensUsed,
		"dollars_used": r.session.DollarsUsed,
		"hat":          r.session.Hat,
		"messages":     r.messages,
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint state: %w", err)
	}

	_, err = r.db.CreateSessionCheckpoint(r.session.ID, r.session.IterationCount, stateJSON)
	return err
}

// broadcastEvent sends an event through the WebSocket hub
func (r *RalphLoop) broadcastEvent(eventType string, payload map[string]any) {
	if r.hub == nil {
		return
	}

	r.hub.Broadcast(websocket.Message{
		Type:    eventType,
		TaskID:  r.session.TaskID,
		Payload: payload,
	})
}

// estimateCost calculates the estimated cost in dollars for the API usage
// Uses approximate pricing: $3 per 1M input tokens, $15 per 1M output tokens for Sonnet
func (r *RalphLoop) estimateCost(usage toolbelt.AnthropicUsage) float64 {
	inputCost := float64(usage.InputTokens) * 3.0 / 1_000_000
	outputCost := float64(usage.OutputTokens) * 15.0 / 1_000_000
	return inputCost + outputCost
}

// RestoreFromCheckpoint restores session state from a checkpoint
func (r *RalphLoop) RestoreFromCheckpoint(checkpoint *db.SessionCheckpoint) error {
	var state struct {
		Iteration   int                          `json:"iteration"`
		TokensUsed  int64                        `json:"tokens_used"`
		DollarsUsed float64                      `json:"dollars_used"`
		Hat         string                       `json:"hat"`
		Messages    []toolbelt.AnthropicMessage  `json:"messages"`
	}

	if err := json.Unmarshal(checkpoint.State, &state); err != nil {
		return fmt.Errorf("failed to unmarshal checkpoint state: %w", err)
	}

	r.session.IterationCount = state.Iteration
	r.session.TokensUsed = state.TokensUsed
	r.session.DollarsUsed = state.DollarsUsed
	r.session.Hat = state.Hat
	r.messages = state.Messages

	return nil
}
