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

// ParsedChecklist represents the parsed checklist from the planner response
type ParsedChecklist struct {
	MustHave []string
	Optional []string
}

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
3. When the task is clear, produce a structured checklist in this format:

PLAN_CHECKLIST
---
must_have:
- First required step
- Second required step
- Third required step

optional:
- Nice-to-have enhancement
- Another optional improvement
---

Guidelines for the checklist:
- must_have items are required for task success
- optional items are nice-to-have enhancements the user can select
- Keep items atomic and verifiable (each should be a discrete action)
- 3-7 items total is ideal
- Focus on outcomes, not implementation details
- Each item should be completable independently

Keep responses concise. Focus on understanding intent and breaking down into clear steps.`

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

	// Check if plan has a checklist or is confirmed
	if isPlanChecklist(assistantMsg) {
		checklist := parseChecklist(assistantMsg)
		if err := p.createChecklistItems(taskID, checklist); err != nil {
			return nil, fmt.Errorf("failed to create checklist items: %w", err)
		}
		refinedPrompt := buildRefinedPromptFromChecklist(checklist)
		if err := p.db.CompletePlanningSession(session.ID, refinedPrompt); err != nil {
			return nil, fmt.Errorf("failed to complete planning session: %w", err)
		}
		session.Status = db.PlanningStatusCompleted
	} else if isPlanConfirmed(assistantMsg) {
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

	// Check if plan has a checklist or is confirmed
	if isPlanChecklist(assistantMsg) {
		checklist := parseChecklist(assistantMsg)
		if err := p.createChecklistItems(session.TaskID, checklist); err != nil {
			return nil, fmt.Errorf("failed to create checklist items: %w", err)
		}
		refinedPrompt := buildRefinedPromptFromChecklist(checklist)
		if err := p.db.CompletePlanningSession(session.ID, refinedPrompt); err != nil {
			return nil, fmt.Errorf("failed to complete planning session: %w", err)
		}
		session.Status = db.PlanningStatusCompleted
	} else if isPlanConfirmed(assistantMsg) {
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

// isPlanChecklist checks if the assistant's message contains a checklist plan
func isPlanChecklist(msg string) bool {
	return strings.Contains(msg, "PLAN_CHECKLIST")
}

// parseChecklist extracts the checklist from a PLAN_CHECKLIST message
func parseChecklist(msg string) *ParsedChecklist {
	checklist := &ParsedChecklist{
		MustHave: []string{},
		Optional: []string{},
	}

	// Extract content between --- delimiters
	parts := strings.Split(msg, "---")
	if len(parts) < 2 {
		return checklist
	}

	content := strings.TrimSpace(parts[1])
	lines := strings.Split(content, "\n")

	var currentSection string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for section headers
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "must_have:") || strings.HasPrefix(lowerLine, "must-have:") {
			currentSection = "must_have"
			continue
		}
		if strings.HasPrefix(lowerLine, "optional:") {
			currentSection = "optional"
			continue
		}

		// Parse list items
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			item := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}

			switch currentSection {
			case "must_have":
				checklist.MustHave = append(checklist.MustHave, item)
			case "optional":
				checklist.Optional = append(checklist.Optional, item)
			}
		}
	}

	return checklist
}

// buildRefinedPromptFromChecklist creates a refined prompt text from the checklist
func buildRefinedPromptFromChecklist(checklist *ParsedChecklist) string {
	var sb strings.Builder

	if len(checklist.MustHave) > 0 {
		sb.WriteString("Required steps:\n")
		for _, item := range checklist.MustHave {
			sb.WriteString("- ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	}

	if len(checklist.Optional) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Optional enhancements:\n")
		for _, item := range checklist.Optional {
			sb.WriteString("- ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// createChecklistItems creates checklist items in the database from the parsed checklist
func (p *Planner) createChecklistItems(taskID string, checklist *ParsedChecklist) error {
	// Create the task checklist
	taskChecklist, err := p.db.CreateTaskChecklist(taskID)
	if err != nil {
		return fmt.Errorf("failed to create task checklist: %w", err)
	}

	sortOrder := 0

	// Create must-have items
	for _, item := range checklist.MustHave {
		_, err := p.db.CreateChecklistItem(taskChecklist.ID, item, db.ChecklistCategoryMustHave, sortOrder)
		if err != nil {
			return fmt.Errorf("failed to create must-have item: %w", err)
		}
		sortOrder++
	}

	// Create optional items
	for _, item := range checklist.Optional {
		_, err := p.db.CreateChecklistItem(taskChecklist.ID, item, db.ChecklistCategoryOptional, sortOrder)
		if err != nil {
			return fmt.Errorf("failed to create optional item: %w", err)
		}
		sortOrder++
	}

	return nil
}

// GetChecklistByTask retrieves the checklist for a task
func (p *Planner) GetChecklistByTask(taskID string) (*db.TaskChecklist, []*db.ChecklistItem, error) {
	checklist, err := p.db.GetChecklistByTaskID(taskID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get checklist: %w", err)
	}
	if checklist == nil {
		return nil, nil, nil
	}

	items, err := p.db.GetChecklistItems(checklist.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get checklist items: %w", err)
	}

	return checklist, items, nil
}
