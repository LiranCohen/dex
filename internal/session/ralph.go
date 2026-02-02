// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// Completion/transition signals that Ralph looks for in responses
const (
	SignalTaskComplete    = "TASK_COMPLETE"
	SignalHatComplete     = "HAT_COMPLETE"
	SignalHatTransition   = "HAT_TRANSITION:"
	SignalChecklistDone   = "CHECKLIST_DONE:"
	SignalChecklistFailed = "CHECKLIST_FAILED:"
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

	// Activity recorder for visibility
	activity *ActivityRecorder

	// AI model to use for this loop (sonnet or opus)
	model string

	// Tool use support
	executor *ToolExecutor
	tools    []toolbelt.AnthropicTool
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
		tools:              GetToolDefinitions(),
	}
}

// InitExecutor initializes the tool executor with project context
func (r *RalphLoop) InitExecutor(worktreePath string, gitOps *git.Operations, githubClient *toolbelt.GitHubClient, owner, repo string) {
	r.executor = NewToolExecutor(worktreePath, gitOps, githubClient, owner, repo)
}

// SetModel sets the AI model to use for this loop and captures the rates
// model should be "sonnet" or "opus"
func (r *RalphLoop) SetModel(model string) {
	r.model = model
	// Capture rates at session start for historical accuracy
	if model == db.TaskModelOpus {
		r.session.InputRate = getEnvFloat("DEX_OPUS_INPUT_COST", 5.0)
		r.session.OutputRate = getEnvFloat("DEX_OPUS_OUTPUT_COST", 25.0)
	} else {
		r.session.InputRate = getEnvFloat("DEX_SONNET_INPUT_COST", 3.0)
		r.session.OutputRate = getEnvFloat("DEX_SONNET_OUTPUT_COST", 15.0)
	}
	// Persist rates to database
	if r.db != nil {
		r.db.SetSessionRates(r.session.ID, r.session.InputRate, r.session.OutputRate)
	}
}

// Run executes the Ralph loop until completion, error, or budget exceeded
func (r *RalphLoop) Run(ctx context.Context) error {
	fmt.Printf("RalphLoop.Run: starting for session %s (hat: %s)\n", r.session.ID, r.session.Hat)

	if r.client == nil {
		fmt.Printf("RalphLoop.Run: ERROR - Anthropic client is nil\n")
		return ErrNoAnthropicClient
	}

	// Initialize activity recorder with WebSocket broadcasting
	r.activity = NewActivityRecorder(r.db, r.session.ID, r.session.TaskID, r.broadcastEvent)
	r.activity.SetHat(r.session.Hat)

	// Save checkpoint when function exits (success or failure) to preserve state for resume
	defer func() {
		if len(r.messages) > 0 && r.session.IterationCount > 0 {
			if err := r.checkpoint(); err != nil {
				fmt.Printf("RalphLoop.Run: warning - final checkpoint failed: %v\n", err)
			} else {
				fmt.Printf("RalphLoop.Run: saved final checkpoint at iteration %d with %d messages\n", r.session.IterationCount, len(r.messages))
			}
		}
	}()

	// Build initial system prompt from hat template
	fmt.Printf("RalphLoop.Run: building prompt for hat %s\n", r.session.Hat)
	systemPrompt, err := r.buildPrompt()
	if err != nil {
		fmt.Printf("RalphLoop.Run: ERROR - failed to build prompt: %v\n", err)
		return fmt.Errorf("failed to build prompt: %w", err)
	}
	fmt.Printf("RalphLoop.Run: prompt built successfully (%d chars)\n", len(systemPrompt))

	// Broadcast session started event
	r.broadcastEvent(websocket.EventSessionStarted, map[string]any{
		"session_id":    r.session.ID,
		"hat":           r.session.Hat,
		"worktree_path": r.session.WorktreePath,
	})

	// Initialize conversation with context message (only if not restored from checkpoint)
	// If messages already has content, we restored from checkpoint and should continue from there
	if len(r.messages) == 0 {
		initialMessage := "Begin working on the task. Follow your hat instructions and report progress."

		// Check for checklist first
		if checklist, err := r.db.GetChecklistByTaskID(r.session.TaskID); err == nil && checklist != nil {
			if items, err := r.db.GetChecklistItems(checklist.ID); err == nil && len(items) > 0 {
				initialMessage = r.buildChecklistPrompt(items)
				fmt.Printf("RalphLoop.Run: using checklist context (%d items)\n", len(items))
			}
		} else if planningSession, err := r.db.GetPlanningSessionByTaskID(r.session.TaskID); err == nil && planningSession != nil {
			if planningSession.RefinedPrompt.Valid && planningSession.RefinedPrompt.String != "" {
				initialMessage = fmt.Sprintf("## Task Instructions (from planning phase)\n\n%s\n\n---\n\nBegin working on this task. Follow your hat instructions and report progress.", planningSession.RefinedPrompt.String)
				fmt.Printf("RalphLoop.Run: using refined prompt from planning phase\n")
			}
		}
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "user",
			Content: initialMessage,
		})

		// Record initial user message
		if err := r.activity.RecordUserMessage(0, initialMessage); err != nil {
			fmt.Printf("RalphLoop.Run: warning - failed to record initial message: %v\n", err)
		}
	} else {
		fmt.Printf("RalphLoop.Run: restored from checkpoint with %d messages, skipping initial prompt\n", len(r.messages))
	}

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
		fmt.Printf("RalphLoop.Run: iteration %d - sending message to Claude\n", r.session.IterationCount+1)
		r.activity.Debug(r.session.IterationCount+1, fmt.Sprintf("Sending API request (iteration %d, %d messages)", r.session.IterationCount+1, len(r.messages)))

		apiStart := time.Now()
		response, err := r.sendMessage(ctx, systemPrompt)
		apiDuration := time.Since(apiStart).Milliseconds()

		if err != nil {
			fmt.Printf("RalphLoop.Run: ERROR - Claude API call failed: %v\n", err)
			r.activity.DebugError(r.session.IterationCount+1, fmt.Sprintf("API call failed after %dms", apiDuration), map[string]any{"error": err.Error()})
			return fmt.Errorf("claude API error: %w", err)
		}

		r.activity.DebugWithDuration(r.session.IterationCount+1, fmt.Sprintf("API response received (in:%d out:%d tokens, stop:%s)", response.Usage.InputTokens, response.Usage.OutputTokens, response.StopReason), apiDuration)
		fmt.Printf("RalphLoop.Run: received response (input tokens: %d, output tokens: %d)\n", response.Usage.InputTokens, response.Usage.OutputTokens)

		// 4. Update usage tracking
		r.session.InputTokens += int64(response.Usage.InputTokens)
		r.session.OutputTokens += int64(response.Usage.OutputTokens)
		r.session.IterationCount++
		r.session.LastActivity = time.Now()

		// Broadcast iteration event
		r.broadcastEvent(websocket.EventSessionIteration, map[string]any{
			"session_id": r.session.ID,
			"iteration":  r.session.IterationCount,
			"tokens":     r.session.TotalTokens(),
		})

		// 5. Handle tool use if requested
		if response.HasToolUse() {
			toolBlocks := response.ToolUseBlocks()
			r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Processing %d tool calls", len(toolBlocks)))

			// Add assistant message with tool_use blocks
			// Use NormalizedContent to ensure Input fields are never nil (API requirement)
			r.messages = append(r.messages, toolbelt.AnthropicMessage{
				Role:    "assistant",
				Content: response.NormalizedContent(),
			})

			// Record assistant response
			responseText := response.Text()
			if err := r.activity.RecordAssistantResponse(
				r.session.IterationCount,
				responseText,
				response.Usage.InputTokens,
				response.Usage.OutputTokens,
			); err != nil {
				fmt.Printf("RalphLoop.Run: warning - failed to record assistant response: %v\n", err)
			}

			// Execute tools and collect results
			var results []toolbelt.ContentBlock
			for i, block := range toolBlocks {
				fmt.Printf("RalphLoop.Run: executing tool %s\n", block.Name)
				r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Executing tool %d/%d: %s", i+1, len(toolBlocks), block.Name))

				// Record tool call
				if err := r.activity.RecordToolCall(r.session.IterationCount, block.Name, block.Input); err != nil {
					fmt.Printf("RalphLoop.Run: warning - failed to record tool call: %v\n", err)
				}

				// Execute the tool
				toolStart := time.Now()
				var result ToolResult
				if r.executor != nil {
					result = r.executor.Execute(ctx, block.Name, block.Input)
				} else {
					result = ToolResult{
						Output:  "Tool executor not initialized",
						IsError: true,
					}
					r.activity.DebugError(r.session.IterationCount, "Tool executor not initialized", nil)
				}
				toolDuration := time.Since(toolStart).Milliseconds()

				// Record tool result
				if err := r.activity.RecordToolResult(r.session.IterationCount, block.Name, result); err != nil {
					fmt.Printf("RalphLoop.Run: warning - failed to record tool result: %v\n", err)
				}

				if result.IsError {
					r.activity.DebugError(r.session.IterationCount, fmt.Sprintf("Tool %s failed after %dms", block.Name, toolDuration), map[string]any{"output": truncateOutput(result.Output, 500)})
				} else {
					r.activity.DebugWithDuration(r.session.IterationCount, fmt.Sprintf("Tool %s completed (%d bytes output)", block.Name, len(result.Output)), toolDuration)
				}

				fmt.Printf("RalphLoop.Run: tool %s result (error=%v): %s\n", block.Name, result.IsError, truncateOutput(result.Output, 200))

				results = append(results, toolbelt.ContentBlock{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   result.Output,
					IsError:   result.IsError,
				})
			}

			// Add tool results as user message
			r.messages = append(r.messages, toolbelt.AnthropicMessage{
				Role:    "user",
				Content: results,
			})

			r.activity.Debug(r.session.IterationCount, fmt.Sprintf("All tools complete, continuing to next iteration"))

			// Continue loop without adding continuation prompt
			continue
		}

		// 6. Get response text (non-tool response)
		responseText := response.Text()

		// 7. Add assistant response to conversation
		// Guard against empty content which would cause API error on next call
		if responseText == "" {
			if response.StopReason == "max_tokens" {
				responseText = "[Response truncated due to token limit. Continuing...]"
			} else {
				responseText = "[No response content]"
			}
		}
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "assistant",
			Content: responseText,
		})

		// Record assistant response with token usage
		if err := r.activity.RecordAssistantResponse(
			r.session.IterationCount,
			responseText,
			response.Usage.InputTokens,
			response.Usage.OutputTokens,
		); err != nil {
			fmt.Printf("RalphLoop.Run: warning - failed to record assistant response: %v\n", err)
		}

		// 7.5. Process checklist signals
		r.processChecklistSignals(responseText)

		// 8. Check for task completion
		if r.detectCompletion(responseText) {
			// Verify checklist completion
			allComplete, issues := r.verifyChecklist()

			// Determine outcome
			outcome := "completed"
			if !allComplete {
				outcome = "completed_with_issues"
				fmt.Printf("RalphLoop.Run: task completed with %d checklist issues\n", len(issues))
			}

			// Record completion signal
			if err := r.activity.RecordCompletion(r.session.IterationCount, SignalTaskComplete); err != nil {
				fmt.Printf("RalphLoop.Run: warning - failed to record completion: %v\n", err)
			}

			r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
				"session_id":   r.session.ID,
				"outcome":      outcome,
				"iterations":   r.session.IterationCount,
				"has_issues":   !allComplete,
				"issues_count": len(issues),
			})
			return nil
		}

		// 9. Check for hat transition
		if nextHat := r.detectHatTransition(responseText); nextHat != "" {
			// Record hat transition
			oldHat := r.session.Hat
			if err := r.activity.RecordHatTransition(r.session.IterationCount, oldHat, nextHat); err != nil {
				fmt.Printf("RalphLoop.Run: warning - failed to record hat transition: %v\n", err)
			}

			// Store transition for manager to handle
			r.session.Hat = nextHat
			r.activity.SetHat(nextHat)
			r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
				"session_id": r.session.ID,
				"outcome":    "hat_transition",
				"next_hat":   nextHat,
			})
			return nil
		}

		// 10. Checkpoint periodically
		if r.session.IterationCount%r.checkpointInterval == 0 {
			if err := r.checkpoint(); err != nil {
				// Log but don't fail on checkpoint error
				fmt.Printf("warning: checkpoint failed: %v\n", err)
			}
		}

		// 11. Add continuation prompt for next iteration
		continuationMsg := "Continue. If the task is complete, output TASK_COMPLETE. If you need to transition to a different hat, output HAT_TRANSITION:<hat_name>."
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "user",
			Content: continuationMsg,
		})

		// Record continuation prompt
		if err := r.activity.RecordUserMessage(r.session.IterationCount, continuationMsg); err != nil {
			fmt.Printf("RalphLoop.Run: warning - failed to record continuation: %v\n", err)
		}
	}
}

// truncateOutput truncates a string to maxLen characters, adding "..." if truncated
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// checkBudget returns an error if any budget limit is exceeded
func (r *RalphLoop) checkBudget() error {
	// Check iteration limit
	if r.session.MaxIterations > 0 && r.session.IterationCount >= r.session.MaxIterations {
		return ErrIterationLimit
	}

	// Check token budget
	if r.session.TokensBudget != nil && r.session.TotalTokens() >= *r.session.TokensBudget {
		return ErrTokenBudget
	}

	// Check dollar budget
	if r.session.DollarsBudget != nil && r.session.Cost() >= *r.session.DollarsBudget {
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

	// Get project from DB for context
	var projectCtx *ProjectContext
	project, err := r.db.GetProjectByID(task.ProjectID)
	if err == nil && project != nil {
		projectCtx = &ProjectContext{
			Name:     project.Name,
			RepoPath: project.RepoPath,
		}
		if project.GitHubOwner.Valid {
			projectCtx.GitHubOwner = project.GitHubOwner.String
		}
		if project.GitHubRepo.Valid {
			projectCtx.GitHubRepo = project.GitHubRepo.String
		}
		// Check if this is a new project (no .git directory in worktree)
		if r.session.WorktreePath != "" {
			gitDir := filepath.Join(r.session.WorktreePath, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				projectCtx.IsNewProject = true
			}
		}
	}

	// Build list of available tools
	toolNames := make([]string, len(r.tools))
	for i, tool := range r.tools {
		toolNames[i] = tool.Name
	}

	ctx := &PromptContext{
		Task:    task,
		Session: r.session,
		Project: projectCtx,
		Tools:   toolNames,
	}

	return r.manager.promptLoader.Get(r.session.Hat, ctx)
}

// sendMessage sends the current conversation to Claude
func (r *RalphLoop) sendMessage(ctx context.Context, systemPrompt string) (*toolbelt.AnthropicChatResponse, error) {
	// Determine model based on task settings
	model := "claude-sonnet-4-5-20250929" // default
	if r.model == db.TaskModelOpus {
		model = "claude-opus-4-5-20251101"
	}

	req := &toolbelt.AnthropicChatRequest{
		Model:     model,
		MaxTokens: 8192,
		System:    systemPrompt,
		Messages:  r.messages,
		Tools:     r.tools,
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
		"iteration":     r.session.IterationCount,
		"input_tokens":  r.session.InputTokens,
		"output_tokens": r.session.OutputTokens,
		"hat":           r.session.Hat,
		"messages":      r.messages,
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint state: %w", err)
	}

	// Also persist to sessions table for real-time queries
	if err := r.db.UpdateSessionUsage(r.session.ID, r.session.InputTokens, r.session.OutputTokens); err != nil {
		fmt.Printf("Warning: failed to update session usage: %v\n", err)
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

// getEnvFloat reads a float64 from an environment variable, returning defaultVal if not set or invalid
// Used for model pricing rates (DEX_SONNET_INPUT_COST, DEX_OPUS_OUTPUT_COST, etc.)
func getEnvFloat(key string, defaultVal float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return defaultVal
	}
	return f
}

// RestoreFromCheckpoint restores session state from a checkpoint
func (r *RalphLoop) RestoreFromCheckpoint(checkpoint *db.SessionCheckpoint) error {
	var state struct {
		Iteration    int                         `json:"iteration"`
		InputTokens  int64                       `json:"input_tokens"`
		OutputTokens int64                       `json:"output_tokens"`
		// Legacy fields for backwards compatibility
		TokensUsed  int64                        `json:"tokens_used"`
		DollarsUsed float64                      `json:"dollars_used"`
		Hat         string                       `json:"hat"`
		Messages    []toolbelt.AnthropicMessage  `json:"messages"`
	}

	if err := json.Unmarshal(checkpoint.State, &state); err != nil {
		return fmt.Errorf("failed to unmarshal checkpoint state: %w", err)
	}

	r.session.IterationCount = state.Iteration
	r.session.Hat = state.Hat
	if r.activity != nil {
		r.activity.SetHat(state.Hat)
	}
	r.messages = state.Messages

	fmt.Printf("RestoreFromCheckpoint: restored iteration=%d, hat=%s, messages=%d, inputTokens=%d, outputTokens=%d\n",
		state.Iteration, state.Hat, len(state.Messages), state.InputTokens, state.OutputTokens)

	// Use new fields if available, otherwise estimate from legacy
	if state.InputTokens > 0 || state.OutputTokens > 0 {
		r.session.InputTokens = state.InputTokens
		r.session.OutputTokens = state.OutputTokens
	} else if state.TokensUsed > 0 {
		// Legacy: split evenly as approximation (input usually larger)
		r.session.InputTokens = state.TokensUsed * 2 / 3
		r.session.OutputTokens = state.TokensUsed / 3
	}

	return nil
}

// buildChecklistPrompt creates the initial prompt with checklist context
// Note: All items passed here are already selected - the must-have vs optional
// distinction is only relevant during planning, not execution.
func (r *RalphLoop) buildChecklistPrompt(items []*db.ChecklistItem) string {
	var sb strings.Builder

	sb.WriteString("## Task Checklist\n\n")
	sb.WriteString("Complete these items and report status for each:\n\n")

	for _, item := range items {
		sb.WriteString(fmt.Sprintf("- [ ] %s (id: %s)\n", item.Description, item.ID))
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## Reporting Checklist Status\n\n")
	sb.WriteString("IMPORTANT: Only mark an item as done when it is FULLY and SUCCESSFULLY completed.\n\n")
	sb.WriteString("- CHECKLIST_DONE:<item_id> - Use ONLY when the item succeeded completely\n")
	sb.WriteString("- CHECKLIST_FAILED:<item_id>:<reason> - Use when an item failed or could not be completed\n\n")
	sb.WriteString("If a tool returns an error or an operation fails, you MUST use CHECKLIST_FAILED, not CHECKLIST_DONE.\n")
	sb.WriteString("Do not claim success for items that encountered errors.\n\n")
	sb.WriteString("When all items are addressed (done or failed), output TASK_COMPLETE.\n\n")
	sb.WriteString("Begin working on the task. Follow your hat instructions and report progress.")

	return sb.String()
}

// processChecklistSignals detects and processes checklist update signals
func (r *RalphLoop) processChecklistSignals(response string) {
	// Process CHECKLIST_DONE signals
	for {
		idx := strings.Index(response, SignalChecklistDone)
		if idx == -1 {
			break
		}

		// Extract item ID
		remaining := response[idx+len(SignalChecklistDone):]
		endIdx := strings.IndexAny(remaining, " \t\n\r")
		if endIdx == -1 {
			endIdx = len(remaining)
		}
		itemID := strings.TrimSpace(remaining[:endIdx])

		if itemID != "" {
			// Update item status in DB
			if err := r.db.UpdateChecklistItemStatus(itemID, db.ChecklistItemStatusDone, ""); err != nil {
				fmt.Printf("RalphLoop: warning - failed to update checklist item %s: %v\n", itemID, err)
			} else {
				// Record activity
				if r.activity != nil {
					r.activity.RecordChecklistUpdate(r.session.IterationCount, itemID, db.ChecklistItemStatusDone, "")
				}
				fmt.Printf("RalphLoop: marked checklist item %s as done\n", itemID)
				// Notify for GitHub sync
				r.manager.NotifyChecklistUpdated(r.session.TaskID)
			}
		}

		// Move past this signal for next search
		response = remaining[endIdx:]
	}

	// Process CHECKLIST_FAILED signals
	response = response // Reset for second pass
	for {
		idx := strings.Index(response, SignalChecklistFailed)
		if idx == -1 {
			break
		}

		// Extract item ID and reason
		remaining := response[idx+len(SignalChecklistFailed):]

		// Format: CHECKLIST_FAILED:<item_id>:<reason>
		parts := strings.SplitN(remaining, ":", 2)
		if len(parts) >= 1 {
			// Get item ID (may have trailing content)
			itemPart := parts[0]
			endIdx := strings.IndexAny(itemPart, " \t\n\r")
			if endIdx == -1 {
				endIdx = len(itemPart)
			}
			itemID := strings.TrimSpace(itemPart[:endIdx])

			reason := ""
			if len(parts) >= 2 {
				reasonPart := parts[1]
				endIdx := strings.IndexAny(reasonPart, "\n\r")
				if endIdx == -1 {
					endIdx = len(reasonPart)
				}
				reason = strings.TrimSpace(reasonPart[:endIdx])
			}

			if itemID != "" {
				// Update item status in DB
				if err := r.db.UpdateChecklistItemStatus(itemID, db.ChecklistItemStatusFailed, reason); err != nil {
					fmt.Printf("RalphLoop: warning - failed to update checklist item %s: %v\n", itemID, err)
				} else {
					// Record activity
					if r.activity != nil {
						r.activity.RecordChecklistUpdate(r.session.IterationCount, itemID, db.ChecklistItemStatusFailed, reason)
					}
					fmt.Printf("RalphLoop: marked checklist item %s as failed: %s\n", itemID, reason)
					// Notify for GitHub sync
					r.manager.NotifyChecklistUpdated(r.session.TaskID)
				}
			}
		}

		// Move past this signal
		response = remaining
	}
}

// verifyChecklist checks if all selected checklist items are completed
// Returns true if all done, false if there are issues
func (r *RalphLoop) verifyChecklist() (bool, []db.ChecklistIssue) {
	checklist, err := r.db.GetChecklistByTaskID(r.session.TaskID)
	if err != nil || checklist == nil {
		// No checklist, consider it complete
		return true, nil
	}

	issues, err := r.db.GetChecklistIssues(checklist.ID)
	if err != nil {
		fmt.Printf("RalphLoop: warning - failed to get checklist issues: %v\n", err)
		return true, nil
	}

	return len(issues) == 0, issues
}
