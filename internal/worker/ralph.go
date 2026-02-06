package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/hints"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// Signals that Ralph looks for in responses
const (
	SignalChecklistDone       = "CHECKLIST_DONE:"
	SignalChecklistFailed     = "CHECKLIST_FAILED:"
	SignalAcknowledgeFailures = "ACKNOWLEDGE_FAILURES"
	SignalScratchpad          = "SCRATCHPAD:"
	SignalEvent               = "EVENT:"
)

// Budget limit errors
var (
	ErrBudgetExceeded    = errors.New("budget exceeded")
	ErrIterationLimit    = errors.New("iteration limit exceeded")
	ErrTokenBudget       = errors.New("token budget exceeded")
	ErrRuntimeLimit      = errors.New("runtime limit exceeded")
	ErrNoAnthropicClient = errors.New("anthropic client not configured")
	ErrCancelled         = errors.New("execution cancelled")
)

// Loop detection constants
const (
	MaxHatVisits        = 3  // Max times to visit the same hat
	MaxTotalTransitions = 10 // Max total hat transitions per session
)

// WorkerRalphLoop orchestrates objective execution in the worker context.
// It's adapted from session.RalphLoop for worker-specific requirements.
type WorkerRalphLoop struct {
	session      *WorkerSession
	client       ChatClient // Interface for testability
	activity     *WorkerActivityRecorder
	conn         *Conn
	promptLoader *WorkerPromptLoader
	executor     *WorkerToolExecutor
	localDB      *LocalDB

	// Conversation history
	messages []toolbelt.AnthropicMessage

	// Tools available for current hat
	tools []toolbelt.AnthropicTool

	// Objective context
	objective   *Objective
	project     *Project
	githubToken string

	// Hints loader for project context
	hintsLoader *hints.Loader

	// Model to use
	model string

	// Progress callback for streaming
	onProgress func(iteration int, inputTokens, outputTokens int64)

	// Checkpoint interval (save state every N iterations)
	checkpointInterval int
}

// NewWorkerRalphLoop creates a new RalphLoop for worker context.
func NewWorkerRalphLoop(
	session *WorkerSession,
	client ChatClient,
	activity *WorkerActivityRecorder,
	conn *Conn,
	promptLoader *WorkerPromptLoader,
	executor *WorkerToolExecutor,
	objective *Objective,
	project *Project,
	githubToken string,
) *WorkerRalphLoop {
	return &WorkerRalphLoop{
		session:            session,
		client:             client,
		activity:           activity,
		conn:               conn,
		promptLoader:       promptLoader,
		executor:           executor,
		objective:          objective,
		project:            project,
		githubToken:        githubToken,
		messages:           make([]toolbelt.AnthropicMessage, 0),
		tools:              getToolDefinitionsForHat(session.Hat),
		model:              "claude-sonnet-4-5-20250929", // Default to Sonnet
		checkpointInterval: 5, // Save state every 5 iterations
	}
}

// SetLocalDB sets the local database for checkpointing.
func (r *WorkerRalphLoop) SetLocalDB(db *LocalDB) {
	r.localDB = db
}

// SetModel sets the AI model to use (sonnet or opus).
func (r *WorkerRalphLoop) SetModel(model string) {
	if model == "opus" {
		r.model = "claude-opus-4-5-20251101"
	} else {
		r.model = "claude-sonnet-4-5-20250929"
	}
}

// SetProgressCallback sets a callback for progress updates after each iteration.
func (r *WorkerRalphLoop) SetProgressCallback(cb func(iteration int, inputTokens, outputTokens int64)) {
	r.onProgress = cb
}

// Run executes the Ralph loop until completion, error, or budget exceeded.
func (r *WorkerRalphLoop) Run(ctx context.Context) (*CompletionReport, error) {
	fmt.Printf("WorkerRalphLoop.Run: starting for session %s (hat: %s)\n", r.session.ID, r.session.Hat)

	if r.client == nil {
		return nil, ErrNoAnthropicClient
	}

	// Initialize hints loader for project context
	if r.session.WorkDir != "" {
		r.hintsLoader = hints.NewLoader(r.session.WorkDir)
	}

	// Build initial system prompt
	systemPrompt, err := r.buildPrompt()
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}
	fmt.Printf("WorkerRalphLoop.Run: prompt built successfully (%d chars)\n", len(systemPrompt))

	// Initialize conversation with checklist or task instructions
	r.setupInitialConversation()

	// Main Ralph loop
	for {
		// 1. Check for cancellation
		select {
		case <-ctx.Done():
			return r.buildReport("cancelled", "Execution cancelled"), ErrCancelled
		default:
		}

		// 2. Check budget limits
		if err := r.checkBudget(); err != nil {
			return r.buildReport("budget_exceeded", err.Error()), err
		}

		// 3. Send to Claude
		iteration := r.session.GetIteration() + 1
		fmt.Printf("WorkerRalphLoop.Run: iteration %d - sending message to Claude\n", iteration)
		r.activity.Debug(iteration, fmt.Sprintf("Sending API request (iteration %d, %d messages)", iteration, len(r.messages)))

		apiStart := time.Now()
		response, err := r.sendMessage(ctx, systemPrompt)
		apiDuration := time.Since(apiStart).Milliseconds()

		if err != nil {
			fmt.Printf("WorkerRalphLoop.Run: ERROR - Claude API call failed: %v\n", err)
			r.activity.DebugError(iteration, fmt.Sprintf("API call failed after %dms", apiDuration), map[string]any{"error": err.Error()})
			return r.buildReport("failed", err.Error()), fmt.Errorf("claude API error: %w", err)
		}

		r.activity.DebugWithDuration(iteration, fmt.Sprintf("API response received (in:%d out:%d tokens, stop:%s)",
			response.Usage.InputTokens, response.Usage.OutputTokens, response.StopReason), apiDuration)

		// 4. Update session tracking
		r.session.RecordIteration(response.Usage.InputTokens, response.Usage.OutputTokens)

		// 5. Notify progress
		if r.onProgress != nil {
			r.onProgress(iteration, r.session.InputTokens, r.session.OutputTokens)
		}

		// Send progress to HQ
		r.sendProgress()

		// 5.5. Checkpoint periodically
		if r.checkpointInterval > 0 && iteration%r.checkpointInterval == 0 {
			r.saveCheckpoint()
		}

		// 6. Handle tool use if requested
		if response.HasToolUse() {
			toolBlocks := response.ToolUseBlocks()
			r.activity.Debug(iteration, fmt.Sprintf("Processing %d tool calls", len(toolBlocks)))

			// Add assistant message with tool_use blocks
			r.messages = append(r.messages, toolbelt.AnthropicMessage{
				Role:    "assistant",
				Content: response.NormalizedContent(),
			})

			// Record assistant response
			_ = r.activity.RecordAssistantResponse(iteration, response.Text(),
				response.Usage.InputTokens, response.Usage.OutputTokens)

			// Process checklist signals even in tool-use responses
			responseText := response.Text()
			if responseText != "" {
				r.processChecklistSignals(responseText)
			}

			// Execute tools and add results
			results := r.executeToolCalls(ctx, toolBlocks, iteration)
			r.messages = append(r.messages, toolbelt.AnthropicMessage{
				Role:    "user",
				Content: results,
			})

			r.activity.Debug(iteration, "All tools complete, continuing to next iteration")
			continue
		}

		// 7. Get response text (non-tool response)
		responseText := response.Text()

		// Guard against empty content
		if responseText == "" {
			if response.StopReason == "max_tokens" {
				responseText = "[Response truncated due to token limit. Continuing...]"
			} else {
				responseText = "[No response content]"
			}
		}

		// Add assistant response to conversation
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "assistant",
			Content: responseText,
		})

		// Record assistant response
		_ = r.activity.RecordAssistantResponse(iteration, responseText,
			response.Usage.InputTokens, response.Usage.OutputTokens)

		// 8. Process signals
		r.processChecklistSignals(responseText)
		r.processScratchpadSignal(responseText)

		// 9. Check for completion signals
		if r.detectCompletion(responseText) {
			_ = r.activity.RecordCompletion(iteration, "task.complete")
			r.markSessionComplete("completed")
			return r.buildReport("completed", "Task completed successfully"), nil
		}

		// 10. Check for hat transition - handle locally instead of exiting
		if event := r.detectEvent(responseText); event != "" {
			if isHatTransitionEvent(event) {
				targetHat := getTargetHatForEvent(event, r.session.GetHat())
				if targetHat != "" && r.canTransitionTo(targetHat) {
					// Perform the transition and continue working
					if err := r.performHatTransition(event, targetHat); err != nil {
						return nil, fmt.Errorf("hat transition failed: %w", err)
					}
					// Rebuild system prompt for new hat
					var promptErr error
					systemPrompt, promptErr = r.buildPrompt()
					if promptErr != nil {
						return nil, fmt.Errorf("failed to rebuild prompt after transition: %w", promptErr)
					}
					// Continue to next iteration with new hat
					continue
				} else if targetHat != "" {
					// Hit loop detection limit - exit gracefully
					_ = r.activity.RecordCompletion(iteration, event)
					r.markSessionComplete("loop_limit")
					return r.buildReport("loop_limit", fmt.Sprintf("Loop limit reached during %s", event)), nil
				}
				// Unknown target hat - exit as before
				_ = r.activity.RecordCompletion(iteration, event)
				r.markSessionComplete("hat_transition")
				return r.buildReport("hat_transition", event), nil
			}
		}

		// 11. Add continuation prompt
		continuationMsg := r.getContinuationPrompt()
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "user",
			Content: continuationMsg,
		})
		_ = r.activity.RecordUserMessage(iteration, continuationMsg)
	}
}

// buildPrompt renders the hat template with objective context.
func (r *WorkerRalphLoop) buildPrompt() (string, error) {
	if r.promptLoader == nil {
		return "", errors.New("prompt loader not initialized")
	}

	// Load project hints
	var projectHints string
	if r.hintsLoader != nil {
		loadedHints, err := r.hintsLoader.Load()
		if err != nil {
			fmt.Printf("WorkerRalphLoop: warning - failed to load hints: %v\n", err)
		} else if loadedHints != "" {
			projectHints = loadedHints
		}
	}

	// Build tool descriptions
	toolDescriptions := r.buildToolDescriptions()

	// Build tool names list
	toolNames := make([]string, len(r.tools))
	for i, tool := range r.tools {
		toolNames[i] = tool.Name
	}

	// Detect project type for language guidelines
	var projectType tools.ProjectType
	if r.session.WorkDir != "" {
		projectConfig := tools.DetectProject(r.session.WorkDir)
		projectType = projectConfig.Type
	}

	ctx := &WorkerPromptContext{
		ObjectiveID:          r.objective.ID,
		ObjectiveTitle:       r.objective.Title,
		ObjectiveDescription: r.objective.Description,
		BranchName:           r.objective.BaseBranch,
		SessionID:            r.session.ID,
		WorkDir:              r.session.WorkDir,
		Scratchpad:           r.session.GetScratchpad(),
		ProjectName:          r.project.Name,
		GitHubOwner:          r.project.GitHubOwner,
		GitHubRepo:           r.project.GitHubRepo,
		IsNewProject:         false, // Determined by project setup
		Tools:                toolNames,
		ToolDescriptions:     toolDescriptions,
		Checklist:            r.objective.Checklist,
		ProjectHints:         projectHints,
		PredecessorContext:   r.session.PredecessorContext,
		Language:             projectType,
	}

	return r.promptLoader.Get(r.session.Hat, ctx)
}

// setupInitialConversation builds the initial message for the conversation.
func (r *WorkerRalphLoop) setupInitialConversation() {
	var initialMessage string

	if len(r.objective.Checklist) > 0 {
		// Build checklist prompt
		var sb strings.Builder
		sb.WriteString("## Task Checklist\n\n")
		sb.WriteString("Complete the following items and report status for each:\n\n")
		for i, item := range r.objective.Checklist {
			sb.WriteString(fmt.Sprintf("- [ ] %d. %s\n", i+1, item))
		}
		sb.WriteString("\n---\n\n")
		sb.WriteString("## Reporting Checklist Status\n\n")
		sb.WriteString("Output CHECKLIST signals as you complete each item:\n")
		sb.WriteString("- CHECKLIST_DONE:<item_number> - Output after completing that item\n")
		sb.WriteString("- CHECKLIST_FAILED:<item_number>:<reason> - Output if an item fails\n\n")
		sb.WriteString("When all items are addressed, output EVENT:task.complete.\n\n")
		sb.WriteString("Begin working on the task.")
		initialMessage = sb.String()
	} else {
		initialMessage = fmt.Sprintf("## Task: %s\n\n%s\n\n---\n\nBegin working on the task. Follow your hat instructions and output EVENT:task.complete when done.",
			r.objective.Title, r.objective.Description)
	}

	r.messages = append(r.messages, toolbelt.AnthropicMessage{
		Role:    "user",
		Content: initialMessage,
	})

	_ = r.activity.RecordUserMessage(0, initialMessage)
}

// sendMessage sends the current conversation to Claude.
func (r *WorkerRalphLoop) sendMessage(ctx context.Context, systemPrompt string) (*toolbelt.AnthropicChatResponse, error) {
	req := &toolbelt.AnthropicChatRequest{
		Model:     r.model,
		MaxTokens: 8192,
		System:    systemPrompt,
		Messages:  r.messages,
		Tools:     r.tools,
	}

	// Use streaming for real-time signal detection
	response, err := r.client.ChatWithStreaming(ctx, req, func(delta string) {
		// Could process streaming signals here if needed
	})

	return response, err
}

// executeToolCalls processes tool use blocks and returns the results.
func (r *WorkerRalphLoop) executeToolCalls(ctx context.Context, toolBlocks []toolbelt.AnthropicContentBlock, iteration int) []toolbelt.ContentBlock {
	var results []toolbelt.ContentBlock

	for i, block := range toolBlocks {
		fmt.Printf("WorkerRalphLoop: executing tool %s\n", block.Name)
		r.activity.Debug(iteration, fmt.Sprintf("Executing tool %d/%d: %s", i+1, len(toolBlocks), block.Name))

		// Record tool call
		_ = r.activity.RecordToolCall(iteration, block.Name, block.Input)

		// Execute the tool
		toolStart := time.Now()
		result := r.executor.Execute(ctx, block.Name, block.Input)
		toolDuration := time.Since(toolStart).Milliseconds()

		// Record tool result
		_ = r.activity.RecordToolResult(iteration, block.Name, result)

		if result.IsError {
			r.activity.DebugError(iteration, fmt.Sprintf("Tool %s failed after %dms", block.Name, toolDuration),
				map[string]any{"output": truncateOutput(result.Output, 500)})
		} else {
			r.activity.DebugWithDuration(iteration, fmt.Sprintf("Tool %s completed (%d bytes output)", block.Name, len(result.Output)), toolDuration)
		}

		results = append(results, toolbelt.ContentBlock{
			Type:      "tool_result",
			ToolUseID: block.ID,
			Content:   result.Output,
			IsError:   result.IsError,
		})
	}

	return results
}

// checkBudget returns an error if any budget limit is exceeded.
func (r *WorkerRalphLoop) checkBudget() error {
	// Check iteration limit
	if r.session.MaxIterations > 0 && r.session.GetIteration() >= r.session.MaxIterations {
		return ErrIterationLimit
	}

	// Check token budget
	if r.session.TokenBudget > 0 && int(r.session.TotalTokens()) >= r.session.TokenBudget {
		return ErrTokenBudget
	}

	// Check runtime limit
	if r.session.MaxRuntime > 0 && r.session.Runtime() > r.session.MaxRuntime {
		return ErrRuntimeLimit
	}

	return nil
}

// processChecklistSignals detects and processes checklist update signals.
func (r *WorkerRalphLoop) processChecklistSignals(response string) {
	// Process CHECKLIST_DONE signals
	for _, sig := range findAllSignals(response, SignalChecklistDone) {
		itemID := strings.TrimSpace(sig)
		if itemID != "" {
			r.session.MarkChecklistDone(itemID)
			_ = r.activity.RecordChecklistUpdate(r.session.GetIteration(), itemID, "done", "")
			fmt.Printf("WorkerRalphLoop: marked checklist item %s as done\n", itemID)
		}
	}

	// Process CHECKLIST_FAILED signals
	for _, sig := range findAllSignals(response, SignalChecklistFailed) {
		parts := strings.SplitN(sig, ":", 2)
		itemID := strings.TrimSpace(parts[0])
		reason := ""
		if len(parts) > 1 {
			reason = strings.TrimSpace(parts[1])
		}
		if itemID != "" {
			r.session.MarkChecklistFailed(itemID)
			_ = r.activity.RecordChecklistUpdate(r.session.GetIteration(), itemID, "failed", reason)
			fmt.Printf("WorkerRalphLoop: marked checklist item %s as failed: %s\n", itemID, reason)
		}
	}
}

// processScratchpadSignal extracts and saves scratchpad content.
func (r *WorkerRalphLoop) processScratchpadSignal(response string) {
	idx := strings.Index(response, SignalScratchpad)
	if idx == -1 {
		return
	}

	content := response[idx+len(SignalScratchpad):]

	// Find end of scratchpad
	endSignals := []string{SignalEvent, SignalChecklistDone, SignalChecklistFailed}
	endIdx := len(content)
	for _, sig := range endSignals {
		if sigIdx := strings.Index(content, sig); sigIdx != -1 && sigIdx < endIdx {
			endIdx = sigIdx
		}
	}

	scratchpad := strings.TrimSpace(content[:endIdx])
	if scratchpad != "" {
		r.session.UpdateScratchpad(scratchpad)
		r.activity.Debug(r.session.GetIteration(), fmt.Sprintf("Updated scratchpad (%d chars)", len(scratchpad)))
	}
}

// detectCompletion checks if the response indicates task completion.
func (r *WorkerRalphLoop) detectCompletion(response string) bool {
	return strings.Contains(response, "EVENT:task.complete")
}

// detectEvent extracts an EVENT signal from the response.
func (r *WorkerRalphLoop) detectEvent(response string) string {
	idx := strings.Index(response, SignalEvent)
	if idx == -1 {
		return ""
	}

	content := response[idx+len(SignalEvent):]
	endIdx := strings.IndexAny(content, "\n\r ")
	if endIdx == -1 {
		endIdx = len(content)
	}

	return strings.TrimSpace(content[:endIdx])
}

// isHatTransitionEvent checks if an event triggers a hat transition.
func isHatTransitionEvent(event string) bool {
	transitionEvents := []string{
		"plan.complete",
		"design.complete",
		"implementation.done",
		"review.approved",
		"review.rejected",
		"resolved",
		"task.blocked",
	}
	return slices.Contains(transitionEvents, event)
}

// getTargetHatForEvent determines the target hat for a given event.
func getTargetHatForEvent(event, currentHat string) string {
	switch event {
	case "plan.complete":
		return "designer"
	case "design.complete":
		return "creator"
	case "implementation.done":
		return "critic"
	case "review.approved":
		return "editor"
	case "review.rejected":
		return "creator" // Back to creator for fixes
	case "resolved":
		// Return to where we were blocked from, default to creator
		return "creator"
	case "task.blocked":
		return "resolver"
	default:
		return ""
	}
}

// canTransitionTo checks if transitioning to the target hat is allowed.
func (r *WorkerRalphLoop) canTransitionTo(targetHat string) bool {
	// Check total transition limit
	if r.session.GetTransitionCount() >= MaxTotalTransitions {
		fmt.Printf("WorkerRalphLoop: hit max total transitions (%d)\n", MaxTotalTransitions)
		return false
	}

	// Check per-hat visit limit
	visitCount := r.session.HatVisitCount(targetHat)
	if visitCount >= MaxHatVisits {
		fmt.Printf("WorkerRalphLoop: hit max visits for hat %s (%d)\n", targetHat, MaxHatVisits)
		return false
	}

	return true
}

// performHatTransition executes a hat change within the session.
func (r *WorkerRalphLoop) performHatTransition(event, targetHat string) error {
	fromHat := r.session.GetHat()
	iteration := r.session.GetIteration()

	fmt.Printf("WorkerRalphLoop: transitioning %s → %s (event: %s)\n", fromHat, targetHat, event)

	// Record in session state
	r.session.RecordHatTransition(fromHat, targetHat, event)

	// Update activity recorder
	r.activity.SetHat(targetHat)

	// Record the transition as an activity event
	_ = r.activity.RecordHatTransition(iteration, fromHat, targetHat)

	// Update tools for new hat
	r.tools = getToolDefinitionsForHat(targetHat)

	// Send progress to HQ with transition info
	r.sendProgressWithStatus("hat_transition", fmt.Sprintf("%s → %s", fromHat, targetHat))

	// Add transition context to conversation
	transitionMsg := fmt.Sprintf("## Hat Transition: %s → %s\n\nYou have transitioned from the '%s' hat to the '%s' hat due to: %s.\n\n%s\n\nContinue working with your new responsibilities.",
		fromHat, targetHat, fromHat, targetHat, event, r.getHatInstructions(targetHat))
	r.messages = append(r.messages, toolbelt.AnthropicMessage{
		Role:    "user",
		Content: transitionMsg,
	})
	_ = r.activity.RecordUserMessage(iteration, transitionMsg)

	return nil
}

// getHatInstructions returns brief instructions for the new hat.
func (r *WorkerRalphLoop) getHatInstructions(hat string) string {
	instructions := map[string]string{
		"explorer": "Focus on understanding the codebase and gathering context.",
		"planner":  "Create a detailed plan for implementing the objective.",
		"designer": "Design the technical approach and architecture.",
		"creator":  "Implement the solution following the plan and design.",
		"critic":   "Review the implementation for correctness, quality, and completeness.",
		"editor":   "Polish the code, improve formatting, and prepare for delivery.",
		"resolver": "Resolve the blocking issue and return to normal workflow.",
	}

	if instr, ok := instructions[hat]; ok {
		return instr
	}
	return "Continue working on the objective."
}

// sendProgressWithStatus sends a progress update with a specific status and message.
func (r *WorkerRalphLoop) sendProgressWithStatus(status, message string) {
	if r.conn == nil {
		return
	}

	input, output := r.session.GetTokenUsage()
	payload := &ProgressPayload{
		ObjectiveID:  r.objective.ID,
		SessionID:    r.session.ID,
		Iteration:    r.session.GetIteration(),
		TokensInput:  int(input),
		TokensOutput: int(output),
		Status:       status,
		Message:      message,
	}

	if err := r.conn.Send(MsgTypeProgress, payload); err != nil {
		fmt.Printf("WorkerRalphLoop: warning - failed to send progress: %v\n", err)
	}
}

// saveCheckpoint saves the current session state for potential resumption.
func (r *WorkerRalphLoop) saveCheckpoint() {
	if r.localDB == nil {
		return
	}

	// Serialize conversation
	conversationJSON, err := json.Marshal(r.messages)
	if err != nil {
		fmt.Printf("WorkerRalphLoop: warning - failed to serialize conversation: %v\n", err)
		conversationJSON = []byte("[]")
	}

	// Serialize hat history
	hatHistoryJSON, err := json.Marshal(r.session.GetHatHistory())
	if err != nil {
		fmt.Printf("WorkerRalphLoop: warning - failed to serialize hat history: %v\n", err)
		hatHistoryJSON = []byte("[]")
	}

	done, failed := r.session.GetChecklistStatus()
	input, output := r.session.GetTokenUsage()

	state := &SessionState{
		SessionID:       r.session.ID,
		ObjectiveID:     r.objective.ID,
		Hat:             r.session.GetHat(),
		Iteration:       r.session.GetIteration(),
		TokensInput:     input,
		TokensOutput:    output,
		Conversation:    string(conversationJSON),
		Scratchpad:      r.session.GetScratchpad(),
		ChecklistDone:   done,
		ChecklistFailed: failed,
		HatHistory:      string(hatHistoryJSON),
		TransitionCount: r.session.GetTransitionCount(),
		PreviousHat:     r.session.PreviousHat,
		Status:          "running",
		WorkDir:         r.session.WorkDir,
	}

	if err := r.localDB.SaveSessionState(state); err != nil {
		fmt.Printf("WorkerRalphLoop: warning - failed to save checkpoint: %v\n", err)
	}
}

// markSessionComplete marks the session as complete in the checkpoint store.
func (r *WorkerRalphLoop) markSessionComplete(status string) {
	if r.localDB == nil {
		return
	}

	if err := r.localDB.MarkSessionComplete(r.session.ID, status); err != nil {
		fmt.Printf("WorkerRalphLoop: warning - failed to mark session complete: %v\n", err)
	}
}

// RestoreFromCheckpoint restores conversation state from a saved checkpoint.
func (r *WorkerRalphLoop) RestoreFromCheckpoint(state *SessionState) error {
	// Restore conversation
	if state.Conversation != "" && state.Conversation != "[]" {
		var messages []toolbelt.AnthropicMessage
		if err := json.Unmarshal([]byte(state.Conversation), &messages); err != nil {
			return fmt.Errorf("failed to restore conversation: %w", err)
		}
		r.messages = messages
	}

	// Restore session state
	r.session.UpdateScratchpad(state.Scratchpad)
	r.session.UpdateHat(state.Hat)
	r.session.RestoreIteration(state.Iteration)
	r.session.RestoreTokenUsage(state.TokensInput, state.TokensOutput)
	r.session.SetTransitionCount(state.TransitionCount)
	r.session.SetPreviousHat(state.PreviousHat)

	for _, id := range state.ChecklistDone {
		r.session.MarkChecklistDone(id)
	}
	for _, id := range state.ChecklistFailed {
		r.session.MarkChecklistFailed(id)
	}

	// Restore hat history
	if state.HatHistory != "" && state.HatHistory != "[]" {
		var hatHistory []HatVisit
		if err := json.Unmarshal([]byte(state.HatHistory), &hatHistory); err != nil {
			return fmt.Errorf("failed to restore hat history: %w", err)
		}
		r.session.RestoreHatHistory(hatHistory)
	}

	// Update tools for restored hat
	r.tools = getToolDefinitionsForHat(state.Hat)

	fmt.Printf("WorkerRalphLoop: restored checkpoint (iteration %d, %d messages, hat: %s)\n",
		state.Iteration, len(r.messages), state.Hat)

	return nil
}

// getContinuationPrompt returns a hat-specific continuation prompt.
func (r *WorkerRalphLoop) getContinuationPrompt() string {
	prompts := map[string]string{
		"explorer": "Continue exploring. When ready: EVENT:plan.complete or EVENT:design.complete",
		"planner":  "Continue planning. When ready: EVENT:plan.complete or EVENT:design.complete",
		"designer": "Continue designing. When ready: EVENT:design.complete",
		"creator":  "Continue implementing. Report progress with CHECKLIST signals. When done: EVENT:implementation.done or EVENT:task.complete",
		"critic":   "Continue reviewing. When done: EVENT:review.approved or EVENT:review.rejected",
		"editor":   "Continue polishing. When ready: EVENT:task.complete",
		"resolver": "Continue resolving. When done: EVENT:resolved or EVENT:task.complete",
	}

	if prompt, ok := prompts[r.session.Hat]; ok {
		return prompt
	}
	return "Continue. Output EVENT:task.complete when done."
}

// sendProgress sends a progress update to HQ.
func (r *WorkerRalphLoop) sendProgress() {
	if r.conn == nil {
		return
	}

	input, output := r.session.GetTokenUsage()
	payload := &ProgressPayload{
		ObjectiveID:  r.objective.ID,
		SessionID:    r.session.ID,
		Iteration:    r.session.GetIteration(),
		TokensInput:  int(input),
		TokensOutput: int(output),
		Status:       "running",
	}

	if err := r.conn.Send(MsgTypeProgress, payload); err != nil {
		fmt.Printf("WorkerRalphLoop: warning - failed to send progress: %v\n", err)
	}
}

// buildReport creates a completion report.
func (r *WorkerRalphLoop) buildReport(status, summary string) *CompletionReport {
	done, failed := r.session.GetChecklistStatus()
	input, output := r.session.GetTokenUsage()

	return &CompletionReport{
		ObjectiveID:   r.objective.ID,
		SessionID:     r.session.ID,
		Status:        status,
		Summary:       summary,
		TotalTokens:   int(input + output),
		Iterations:    r.session.GetIteration(),
		ChecklistDone: done,
		Errors:        failed,
		CompletedAt:   time.Now(),
	}
}

// buildToolDescriptions creates a formatted list of available tools.
func (r *WorkerRalphLoop) buildToolDescriptions() string {
	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")

	for _, tool := range r.tools {
		desc := tool.Description
		if len(desc) > 200 {
			desc = desc[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, desc))
	}

	return sb.String()
}

// getToolDefinitionsForHat returns tools appropriate for a specific hat.
func getToolDefinitionsForHat(hat string) []toolbelt.AnthropicTool {
	toolSet := tools.GetToolsForHat(hat)
	allTools := toolSet.All()

	result := make([]toolbelt.AnthropicTool, len(allTools))
	for i, t := range allTools {
		result[i] = toolbelt.AnthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return result
}

// findAllSignals finds all instances of a signal and extracts their content.
func findAllSignals(content, signal string) []string {
	var results []string
	remaining := content

	for {
		idx := strings.Index(remaining, signal)
		if idx == -1 {
			break
		}

		start := idx + len(signal)
		contentAfter := remaining[start:]

		endIdx := strings.IndexAny(contentAfter, "\n\r")
		if endIdx == -1 {
			endIdx = len(contentAfter)
		}

		signalContent := strings.TrimSpace(contentAfter[:endIdx])
		if signalContent != "" {
			results = append(results, signalContent)
		}

		remaining = remaining[start+endIdx:]
	}

	return results
}

// truncateOutput truncates a string to maxLen characters.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
