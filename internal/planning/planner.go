// Package planning provides task planning services for Poindexter
package planning

import (
	"context"
	"fmt"
	"strings"

	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/toolbelt"
)

const planningSystemPrompt = `You are a task planning assistant for Poindexter, an AI orchestration system. Your job is to clarify user requests before they are executed by an AI agent.

IMPORTANT: The execution agent (not you) has access to powerful tools including:
- GitHub operations (create repos, manage issues, PRs, etc.)
- Shell command execution
- File system operations (read, write, edit files)
- Web browsing and API calls
- Code analysis and generation

Your role is ONLY to understand and clarify the task. You do NOT execute anything.

When analyzing a task request:
1. Identify ambiguities or missing information
2. Ask 1-2 clarifying questions if needed (e.g., repo name, visibility, language preferences)
3. When the task is clear, output your understanding in this format:

PLAN_CONFIRMED
---
[Clear, actionable task summary that the execution agent will follow]
---

Keep responses concise. Focus on understanding intent, not implementation details.`

const planningModel = "claude-sonnet-4-20250514"

// Planner handles the planning phase for tasks
type Planner struct {
	db     *db.DB
	client *toolbelt.AnthropicClient
	hub    *websocket.Hub
}

// NewPlanner creates a new Planner instance
func NewPlanner(database *db.DB, client *toolbelt.AnthropicClient, hub *websocket.Hub) *Planner {
	return &Planner{
		db:     database,
		client: client,
		hub:    hub,
	}
}

// StartPlanning creates a planning session and begins the planning conversation
func (p *Planner) StartPlanning(ctx context.Context, taskID, prompt string) (*db.PlanningSession, error) {
	if p.client == nil {
		return nil, fmt.Errorf("anthropic client not configured")
	}

	// Create planning session
	session, err := p.db.CreatePlanningSession(taskID, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning session: %w", err)
	}

	// Store user's original prompt as first message
	_, err = p.db.CreatePlanningMessage(session.ID, "user", prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to store initial prompt: %w", err)
	}

	// Call Sonnet to analyze the prompt
	response, err := p.client.Chat(ctx, &toolbelt.AnthropicChatRequest{
		Model:     planningModel,
		MaxTokens: 1024,
		System:    planningSystemPrompt,
		Messages: []toolbelt.AnthropicMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		// Update session status to indicate error
		p.db.UpdatePlanningSessionStatus(session.ID, db.PlanningStatusAwaitingResponse)
		return nil, fmt.Errorf("failed to get planning response: %w", err)
	}

	// Store assistant's response
	assistantMsg := response.Text()
	_, err = p.db.CreatePlanningMessage(session.ID, "assistant", assistantMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to store assistant response: %w", err)
	}

	// Check if plan is already confirmed
	if isPlanConfirmed(assistantMsg) {
		refinedPrompt := extractRefinedPrompt(assistantMsg)
		if err := p.db.CompletePlanningSession(session.ID, refinedPrompt); err != nil {
			return nil, fmt.Errorf("failed to complete planning session: %w", err)
		}
		session.Status = db.PlanningStatusCompleted
	} else {
		// Awaiting user response
		if err := p.db.UpdatePlanningSessionStatus(session.ID, db.PlanningStatusAwaitingResponse); err != nil {
			return nil, fmt.Errorf("failed to update planning session status: %w", err)
		}
		session.Status = db.PlanningStatusAwaitingResponse
	}

	// Broadcast planning event
	if p.hub != nil {
		p.hub.Broadcast(websocket.Message{
			Type: "planning.started",
			Payload: map[string]any{
				"task_id":    taskID,
				"session_id": session.ID,
				"status":     session.Status,
			},
		})
	}

	return session, nil
}

// ProcessResponse handles a user's response during planning
func (p *Planner) ProcessResponse(ctx context.Context, sessionID, response string) (*db.PlanningSession, error) {
	if p.client == nil {
		return nil, fmt.Errorf("anthropic client not configured")
	}

	// Get the planning session
	session, err := p.db.GetPlanningSessionByID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get planning session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("planning session not found: %s", sessionID)
	}

	// Store user's response
	_, err = p.db.CreatePlanningMessage(session.ID, "user", response)
	if err != nil {
		return nil, fmt.Errorf("failed to store user response: %w", err)
	}

	// Get all messages for context
	messages, err := p.db.GetPlanningMessages(session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get planning messages: %w", err)
	}

	// Convert to Anthropic message format
	anthropicMessages := make([]toolbelt.AnthropicMessage, len(messages))
	for i, msg := range messages {
		anthropicMessages[i] = toolbelt.AnthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Call Sonnet to continue the conversation
	anthropicResp, err := p.client.Chat(ctx, &toolbelt.AnthropicChatRequest{
		Model:     planningModel,
		MaxTokens: 1024,
		System:    planningSystemPrompt,
		Messages:  anthropicMessages,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get planning response: %w", err)
	}

	// Store assistant's response
	assistantMsg := anthropicResp.Text()
	_, err = p.db.CreatePlanningMessage(session.ID, "assistant", assistantMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to store assistant response: %w", err)
	}

	// Check if plan is confirmed
	if isPlanConfirmed(assistantMsg) {
		refinedPrompt := extractRefinedPrompt(assistantMsg)
		if err := p.db.CompletePlanningSession(session.ID, refinedPrompt); err != nil {
			return nil, fmt.Errorf("failed to complete planning session: %w", err)
		}
		session.Status = db.PlanningStatusCompleted
	} else {
		session.Status = db.PlanningStatusAwaitingResponse
	}

	// Broadcast planning update
	if p.hub != nil {
		p.hub.Broadcast(websocket.Message{
			Type: "planning.updated",
			Payload: map[string]any{
				"task_id":    session.TaskID,
				"session_id": session.ID,
				"status":     session.Status,
			},
		})
	}

	return session, nil
}

// AcceptPlan marks the planning session as completed and returns the refined prompt
func (p *Planner) AcceptPlan(ctx context.Context, sessionID string) (string, error) {
	session, err := p.db.GetPlanningSessionByID(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get planning session: %w", err)
	}
	if session == nil {
		return "", fmt.Errorf("planning session not found: %s", sessionID)
	}

	// If already completed, return the refined prompt
	if session.Status == db.PlanningStatusCompleted {
		if session.RefinedPrompt.Valid {
			return session.RefinedPrompt.String, nil
		}
		return session.OriginalPrompt, nil
	}

	// Get the last assistant message as the refined prompt
	messages, err := p.db.GetPlanningMessages(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get planning messages: %w", err)
	}

	// Find last assistant message
	var refinedPrompt string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			refinedPrompt = extractRefinedPrompt(messages[i].Content)
			break
		}
	}

	// If no refined prompt found, use original
	if refinedPrompt == "" {
		refinedPrompt = session.OriginalPrompt
	}

	// Complete the session
	if err := p.db.CompletePlanningSession(sessionID, refinedPrompt); err != nil {
		return "", fmt.Errorf("failed to complete planning session: %w", err)
	}

	// Broadcast planning completed
	if p.hub != nil {
		p.hub.Broadcast(websocket.Message{
			Type: "planning.completed",
			Payload: map[string]any{
				"task_id":    session.TaskID,
				"session_id": session.ID,
			},
		})
	}

	return refinedPrompt, nil
}

// SkipPlanning skips the planning phase for a task
func (p *Planner) SkipPlanning(ctx context.Context, taskID string) error {
	// Check if there's an existing planning session
	session, err := p.db.GetPlanningSessionByTaskID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get planning session: %w", err)
	}

	if session != nil {
		// Mark existing session as skipped
		if err := p.db.SkipPlanningSession(session.ID); err != nil {
			return fmt.Errorf("failed to skip planning session: %w", err)
		}

		// Broadcast planning skipped
		if p.hub != nil {
			p.hub.Broadcast(websocket.Message{
				Type: "planning.skipped",
				Payload: map[string]any{
					"task_id":    taskID,
					"session_id": session.ID,
				},
			})
		}
	}

	return nil
}

// GetSession retrieves a planning session and its messages
func (p *Planner) GetSession(sessionID string) (*db.PlanningSession, []*db.PlanningMessage, error) {
	session, err := p.db.GetPlanningSessionByID(sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get planning session: %w", err)
	}
	if session == nil {
		return nil, nil, nil
	}

	messages, err := p.db.GetPlanningMessages(sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get planning messages: %w", err)
	}

	return session, messages, nil
}

// GetSessionByTask retrieves a planning session for a task
func (p *Planner) GetSessionByTask(taskID string) (*db.PlanningSession, []*db.PlanningMessage, error) {
	session, err := p.db.GetPlanningSessionByTaskID(taskID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get planning session: %w", err)
	}
	if session == nil {
		return nil, nil, nil
	}

	messages, err := p.db.GetPlanningMessages(session.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get planning messages: %w", err)
	}

	return session, messages, nil
}

// isPlanConfirmed checks if the assistant's message contains a confirmed plan
func isPlanConfirmed(msg string) bool {
	return strings.Contains(msg, "PLAN_CONFIRMED")
}

// extractRefinedPrompt extracts the refined prompt from a PLAN_CONFIRMED message
func extractRefinedPrompt(msg string) string {
	// Look for the pattern:
	// PLAN_CONFIRMED
	// ---
	// [refined prompt]
	// ---
	parts := strings.Split(msg, "---")
	if len(parts) >= 2 {
		// The refined prompt is between the first and second ---
		refined := strings.TrimSpace(parts[1])
		if refined != "" {
			return refined
		}
	}

	// If no formatted prompt found, return the whole message without PLAN_CONFIRMED
	msg = strings.Replace(msg, "PLAN_CONFIRMED", "", 1)
	return strings.TrimSpace(msg)
}
