// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v68/github"
	"github.com/google/uuid"
	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/hints"
	"github.com/lirancohen/dex/internal/security"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// Signals that Ralph looks for in responses
const (
	SignalChecklistDone       = "CHECKLIST_DONE:"
	SignalChecklistFailed     = "CHECKLIST_FAILED:"
	SignalAcknowledgeFailures = "ACKNOWLEDGE_FAILURES"
	SignalScratchpad          = "SCRATCHPAD:"
	SignalMemory              = "MEMORY:"
)

// Budget limit errors
var (
	ErrBudgetExceeded    = errors.New("budget exceeded")
	ErrIterationLimit    = errors.New("iteration limit exceeded")
	ErrTokenBudget       = errors.New("token budget exceeded")
	ErrDollarBudget      = errors.New("dollar budget exceeded")
	ErrRuntimeLimit      = errors.New("runtime limit exceeded")
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

	// Loop health tracking
	health *LoopHealth

	// Quality gate for task completion
	qualityGate *QualityGate

	// Event routing for hat transitions
	eventRouter *EventRouter

	// Context management
	contextGuard     *ContextGuard
	handoffGen       *HandoffGenerator
	hintsLoader      *hints.Loader
	lastSystemPrompt string // Cached for token estimation

	// Failure context for checkpoint recovery
	lastError    string // Last error encountered
	failedAt     string // Where failure occurred: "tool", "api", "validation"
	recoveryHint string // Hint for recovery attempt

	// Issue activity sync
	issueCommenter *github.IssueCommenter
	githubClient   *gogithub.Client
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
		tools:              GetToolDefinitionsForHat(session.Hat),
		health:             NewLoopHealth(),
	}
}

// InitExecutor initializes the tool executor with project context
func (r *RalphLoop) InitExecutor(worktreePath string, gitOps *git.Operations, githubClient *toolbelt.GitHubClient, owner, repo string) {
	r.executor = NewToolExecutor(worktreePath, gitOps, githubClient, owner, repo)
	// Quality gate will be initialized when activity recorder is ready
	r.qualityGate = NewQualityGate(worktreePath, nil)
}

// SetEventRouter sets the event router for hat transitions
func (r *RalphLoop) SetEventRouter(router *EventRouter) {
	r.eventRouter = router
}

// SetOnRepoCreated sets the callback for when a GitHub repo is created
// This allows updating the project's GitHub info in the database
func (r *RalphLoop) SetOnRepoCreated(callback func(owner, repo string)) {
	if r.executor != nil {
		r.executor.SetOnRepoCreated(callback)
	}
}

// SetGitHubClient sets the GitHub client for issue commenting
func (r *RalphLoop) SetGitHubClient(client *gogithub.Client) {
	r.githubClient = client
}

// initIssueCommenter initializes the issue commenter if task has a linked issue
func (r *RalphLoop) initIssueCommenter(task *db.Task) {
	if r.githubClient == nil {
		return
	}

	// Check if task has a linked GitHub issue
	if !task.GitHubIssueNumber.Valid || task.GitHubIssueNumber.Int64 == 0 {
		return
	}

	// Get project for GitHub owner/repo
	project, err := r.db.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		return
	}

	if !project.GitHubOwner.Valid || !project.GitHubRepo.Valid {
		return
	}

	r.issueCommenter = github.NewIssueCommenter(
		r.githubClient,
		project.GitHubOwner.String,
		project.GitHubRepo.String,
		int(task.GitHubIssueNumber.Int64),
		github.DefaultIssueCommenterConfig(),
	)
}

// postIssueComment posts a comment to the linked GitHub issue (if any)
func (r *RalphLoop) postIssueComment(ctx context.Context, comment string) {
	if r.issueCommenter == nil {
		return
	}

	if err := r.issueCommenter.Post(ctx, comment); err != nil {
		r.activity.Debug(r.session.IterationCount, fmt.Sprintf("failed to post issue comment: %v", err))
	}
}

// postQualityGateComment posts a comment about quality gate results to the linked GitHub issue
func (r *RalphLoop) postQualityGateComment(ctx context.Context, result *GateResult) {
	if r.issueCommenter == nil || result == nil {
		return
	}

	// Convert GateResult to github.QualityGateResult
	qgResult := &github.QualityGateResult{
		Passed: result.Passed,
	}

	if result.Tests != nil {
		qgResult.Tests = &github.CheckResultSummary{
			Passed:  result.Tests.Passed,
			Skipped: result.Tests.Skipped,
		}
		// Extract failure details if any
		if !result.Tests.Passed && !result.Tests.Skipped && result.Tests.Output != "" {
			qgResult.Tests.Details = extractTestFailureDetails(result.Tests.Output)
		}
	}

	if result.Lint != nil {
		qgResult.Lint = &github.CheckResultSummary{
			Passed:  result.Lint.Passed,
			Skipped: result.Lint.Skipped,
		}
	}

	if result.Build != nil {
		qgResult.Build = &github.CheckResultSummary{
			Passed:  result.Build.Passed,
			Skipped: result.Build.Skipped,
		}
	}

	commentData := r.buildCommentData(ctx)
	commentData.QualityResult = qgResult

	comment := github.BuildQualityGateComment(commentData)
	r.postIssueComment(ctx, comment)
}

// extractTestFailureDetails extracts individual test failure messages from test output
func extractTestFailureDetails(output string) []string {
	var details []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for FAIL or --- FAIL lines in Go test output
		if strings.Contains(line, "--- FAIL:") || strings.HasPrefix(line, "FAIL") {
			line = strings.TrimSpace(line)
			if line != "" && line != "FAIL" {
				details = append(details, line)
			}
		}
	}
	// Limit to first 5 failures
	if len(details) > 5 {
		details = details[:5]
	}
	return details
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

	// Cleanup temp files from large tool responses when session ends
	defer func() {
		if err := tools.CleanupTempResponses(); err != nil {
			fmt.Printf("RalphLoop.Run: warning - failed to cleanup temp responses: %v\n", err)
		}
	}()

	if r.client == nil {
		fmt.Printf("RalphLoop.Run: ERROR - Anthropic client is nil\n")
		return ErrNoAnthropicClient
	}

	// Initialize activity recorder with WebSocket broadcasting
	r.activity = NewActivityRecorder(r.db, r.session.ID, r.session.TaskID, r.broadcastEvent)
	r.activity.SetHat(r.session.Hat)

	// Initialize context guard for token management
	r.contextGuard = NewContextGuard(r.activity)

	// Initialize handoff generator for checkpoint summaries
	r.handoffGen = NewHandoffGenerator(r.db, r.manager.gitOps)

	// Initialize hints loader for project context
	if r.session.WorktreePath != "" {
		r.hintsLoader = hints.NewLoader(r.session.WorktreePath)
	}

	// Initialize quality gate with activity recorder
	if r.qualityGate != nil {
		r.qualityGate.activity = r.activity
	}

	// Set activity recorder on executor for quality gate logging
	if r.executor != nil {
		r.executor.SetActivityRecorder(r.activity)
		r.executor.SetQualityGate(r.qualityGate)
	}

	// Initialize issue commenter for GitHub issue sync
	task, _ := r.db.GetTaskByID(r.session.TaskID)

	// Set up quality gate result callback for issue comments (after task is loaded)
	if r.executor != nil {
		r.executor.SetOnQualityGateResult(func(result *GateResult) {
			r.postQualityGateComment(ctx, result)
		})
	}
	if task != nil {
		r.initIssueCommenter(task)
	}

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

	// Post "started" comment to linked GitHub issue
	if r.issueCommenter != nil && len(r.messages) == 0 {
		commentData := &github.CommentData{
			Iteration:   0,
			TotalTokens: 0,
			Hat:         r.session.Hat,
		}
		if task != nil {
			commentData.Branch = task.GetBranchName()
		}
		comment := github.BuildStartedComment(commentData)
		r.postIssueComment(ctx, comment)
	}

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

		// 2.5. Check loop health
		if shouldTerminate, reason := r.health.ShouldTerminate(); shouldTerminate {
			r.activity.RecordLoopHealth(r.session.IterationCount, &LoopHealthData{
				Status:              string(r.health.Status()),
				ConsecutiveFailures: r.health.ConsecutiveFailures,
				QualityGateAttempts: r.health.QualityGateAttempts,
				TotalFailures:       r.health.TotalFailures,
			})
			r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
				"session_id": r.session.ID,
				"outcome":    string(reason),
				"iterations": r.session.IterationCount,
			})
			return fmt.Errorf("loop terminated: %s", reason)
		}

		// Record health status if changed
		if r.health.StatusChanged() {
			r.activity.RecordLoopHealth(r.session.IterationCount, &LoopHealthData{
				Status:              string(r.health.Status()),
				ConsecutiveFailures: r.health.ConsecutiveFailures,
				QualityGateAttempts: r.health.QualityGateAttempts,
				TotalFailures:       r.health.TotalFailures,
			})
		}

		// 3. Check and compact context if needed
		if r.contextGuard != nil {
			compacted, wasCompacted, err := r.contextGuard.CheckAndCompact(r.messages, systemPrompt, r.session.Scratchpad)
			if err != nil {
				fmt.Printf("RalphLoop.Run: warning - context compaction failed: %v\n", err)
			} else if wasCompacted {
				r.messages = compacted
				// Save checkpoint after compaction
				if err := r.checkpoint(); err != nil {
					fmt.Printf("RalphLoop.Run: warning - post-compaction checkpoint failed: %v\n", err)
				}
			}
		}

		// 4. Send to Claude
		fmt.Printf("RalphLoop.Run: iteration %d - sending message to Claude\n", r.session.IterationCount+1)
		r.activity.Debug(r.session.IterationCount+1, fmt.Sprintf("Sending API request (iteration %d, %d messages)", r.session.IterationCount+1, len(r.messages)))

		r.lastSystemPrompt = systemPrompt // Cache for token estimation
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

				// Check for tool repetition before execution
				paramsJSON, _ := json.Marshal(block.Input)
				if allowed, reason := r.health.CheckToolCall(block.Name, string(paramsJSON)); !allowed {
					r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Tool %s blocked: %s", block.Name, reason))

					results = append(results, toolbelt.ContentBlock{
						Type:      "tool_result",
						ToolUseID: block.ID,
						Content:   fmt.Sprintf("Tool call blocked: %s. Please try a different approach or use different parameters.", reason),
						IsError:   true,
					})
					continue // Skip execution, move to next tool
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
					r.health.RecordFailure(block.Name)

					// Track quality gate blocks specifically
					if block.Name == "task_complete" && strings.Contains(result.Output, "QUALITY_BLOCKED") {
						r.health.RecordQualityBlock()
					}
				} else {
					r.activity.DebugWithDuration(r.session.IterationCount, fmt.Sprintf("Tool %s completed (%d bytes output)", block.Name, len(result.Output)), toolDuration)
					r.health.RecordSuccess()

					// Track quality gate passes
					if block.Name == "task_complete" && strings.Contains(result.Output, "QUALITY_PASSED") {
						r.health.RecordQualityPass()
					}
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

			r.activity.Debug(r.session.IterationCount, "All tools complete, continuing to next iteration")

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

		// 7.6. Process scratchpad signal
		if scratchpad, found := parseScratchpadSignal(responseText); found {
			r.session.Scratchpad = security.SanitizeForPrompt(scratchpad)
			r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Updated scratchpad (%d chars)", len(r.session.Scratchpad)))
		}

		// 7.7. Process memory signals
		r.processMemorySignals(responseText)

		// 8. Check for task completion
		if r.detectCompletion(responseText) {
			// Verify checklist completion
			allComplete, issues := r.verifyChecklist()

			// If there are issues, check if they're acknowledged
			if !allComplete {
				hasAcknowledgment := strings.Contains(responseText, SignalAcknowledgeFailures)

				if !hasAcknowledgment {
					// Send back for resolution - require explicit acknowledgment
					issuesList := r.formatChecklistIssues(issues)
					r.messages = append(r.messages, toolbelt.AnthropicMessage{
						Role: "user",
						Content: fmt.Sprintf(`Some checklist items are not complete:
%s

Please either:
1. Complete the remaining items and signal EVENT:task.complete again
2. Mark items as failed with CHECKLIST_FAILED:<id>:<reason>
3. If failures are known and accepted, output ACKNOWLEDGE_FAILURES along with EVENT:task.complete
4. If blocked, use EVENT:task.blocked:{"reason":"description"}`, issuesList),
					})
					fmt.Printf("RalphLoop.Run: task completion blocked - %d unacknowledged checklist issues\n", len(issues))
					continue // Continue loop, don't complete
				}
			}

			// Determine outcome
			outcome := "completed"
			if !allComplete {
				outcome = "completed_with_acknowledged_issues"
				fmt.Printf("RalphLoop.Run: task completed with %d acknowledged checklist issues\n", len(issues))
			}

			// Record completion signal
			if err := r.activity.RecordCompletion(r.session.IterationCount, TopicTaskComplete); err != nil {
				fmt.Printf("RalphLoop.Run: warning - failed to record completion: %v\n", err)
			}

			// Post completion comment to GitHub issue
			if r.issueCommenter != nil {
				commentData := r.buildCommentData(ctx)
				summary := r.getCompletionSummary()
				comment := github.BuildCompletedComment(commentData, summary)
				r.postIssueComment(ctx, comment)
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

		// 9. Check for event-based transition
		if event := r.detectEvent(responseText); event != nil {
			// Route the event through the event router
			if r.eventRouter != nil {
				result := r.eventRouter.RouteAndPersist(event, r.session.Hat)

				if result.Error != nil {
					// Log routing error but continue
					fmt.Printf("RalphLoop.Run: event routing error: %v\n", result.Error)
					r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Event routing error: %v", result.Error))
				} else if result.IsTerminal {
					// Terminal event (task.complete) - end the session
					r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Terminal event: %s", event.Topic))
					r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
						"session_id": r.session.ID,
						"outcome":    "event_complete",
						"event":      event.Topic,
					})
					return nil
				} else if result.NextHat != "" {
					// Hat transition via event
					oldHat := r.session.Hat
					nextHat := result.NextHat

					if err := r.activity.RecordHatTransition(r.session.IterationCount, oldHat, nextHat); err != nil {
						fmt.Printf("RalphLoop.Run: warning - failed to record hat transition: %v\n", err)
					}

					// Post hat transition comment to GitHub issue (with debouncing)
					if r.issueCommenter != nil && r.issueCommenter.ShouldPostHatTransition(r.session.IterationCount) {
						commentData := r.buildCommentData(ctx)
						commentData.Hat = nextHat
						commentData.PreviousHat = oldHat
						comment := github.BuildHatTransitionComment(commentData)
						r.postIssueComment(ctx, comment)
					}

					// Store transition for manager to handle
					r.session.Hat = nextHat
					r.activity.SetHat(nextHat)
					r.broadcastEvent(websocket.EventSessionCompleted, map[string]any{
						"session_id": r.session.ID,
						"outcome":    "hat_transition",
						"next_hat":   nextHat,
						"event":      event.Topic,
					})
					return nil
				}
			}
		}

		// 10. Checkpoint periodically
		if r.session.IterationCount%r.checkpointInterval == 0 {
			if err := r.checkpoint(); err != nil {
				// Log but don't fail on checkpoint error
				fmt.Printf("warning: checkpoint failed: %v\n", err)
			}
		}

		// 11. Add continuation prompt for next iteration (hat-specific)
		continuationMsg := r.getContinuationPrompt()
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

// buildToolDescriptions creates a formatted list of available tools with descriptions
func (r *RalphLoop) buildToolDescriptions() string {
	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")

	for _, tool := range r.tools {
		// Truncate description to keep it concise
		desc := tool.Description
		if len(desc) > 200 {
			desc = desc[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, desc))
	}

	return sb.String()
}

// buildMemorySection retrieves relevant memories and formats them for the prompt
func (r *RalphLoop) buildMemorySection(projectID string) string {
	if r.db == nil {
		return ""
	}

	// Get task for keywords
	task, err := r.db.GetTaskByID(r.session.TaskID)
	if err != nil || task == nil {
		return ""
	}

	// Extract keywords from task
	keywords := extractKeywords(task.Title + " " + task.GetDescription())

	// Get relevant memories
	ctx := db.MemoryContext{
		ProjectID:        projectID,
		CurrentHat:       r.session.Hat,
		CurrentSessionID: r.session.ID, // Exclude self
		RelevantPaths:    []string{},   // Could be populated from recent tool calls
		TaskKeywords:     keywords,
	}

	memories, err := r.db.GetRelevantMemories(ctx, 8)
	if err != nil || len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Project Knowledge\n\n")
	sb.WriteString("Learnings from previous work on this project:\n\n")

	// Group by type for readability
	byType := make(map[db.MemoryType][]db.Memory)
	for _, m := range memories {
		byType[m.Type] = append(byType[m.Type], m)
	}

	// Order types for consistent display
	typeOrder := []db.MemoryType{
		db.MemoryArchitecture, db.MemoryPattern, db.MemoryPitfall,
		db.MemoryDecision, db.MemoryFix, db.MemoryConvention,
		db.MemoryDependency, db.MemoryConstraint,
	}

	for _, memType := range typeOrder {
		mems := byType[memType]
		if len(mems) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n", memType.Title()))
		for _, m := range mems {
			// Sanitize memory content before injection (defense in depth)
			safeTitle := security.SanitizeForPrompt(m.Title)
			safeContent := security.SanitizeForPrompt(m.Content)
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", safeTitle, safeContent))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// extractKeywords extracts relevant keywords from text for memory matching
func extractKeywords(text string) []string {
	// Simple keyword extraction - split on whitespace and filter
	words := strings.Fields(strings.ToLower(text))
	keywords := make([]string, 0, len(words))

	// Filter out common words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"to": true, "in": true, "on": true, "for": true, "of": true,
		"is": true, "it": true, "this": true, "that": true, "with": true,
		"as": true, "be": true, "are": true, "was": true, "were": true,
	}

	for _, word := range words {
		// Skip short words and stop words
		if len(word) < 3 || stopWords[word] {
			continue
		}
		keywords = append(keywords, word)
	}

	// Limit to first 10 keywords
	if len(keywords) > 10 {
		keywords = keywords[:10]
	}

	return keywords
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

	// Check runtime limit
	if r.session.MaxRuntime > 0 && !r.session.StartedAt.IsZero() {
		if time.Since(r.session.StartedAt) > r.session.MaxRuntime {
			return ErrRuntimeLimit
		}
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

	// Build tool descriptions for context
	toolDescriptions := r.buildToolDescriptions()

	// Load project hints
	var projectHints string
	if r.hintsLoader != nil {
		loadedHints, err := r.hintsLoader.Load()
		if err != nil {
			fmt.Printf("RalphLoop.buildPrompt: warning - failed to load hints: %v\n", err)
		} else if loadedHints != "" {
			projectHints = loadedHints
			fmt.Printf("RalphLoop.buildPrompt: loaded project hints (%d chars)\n", len(loadedHints))
		}
	}

	// Fetch refined prompt from planning session (if any)
	var refinedPrompt string
	if planningSession, err := r.db.GetPlanningSessionByTaskID(r.session.TaskID); err == nil && planningSession != nil {
		if planningSession.RefinedPrompt.Valid && planningSession.RefinedPrompt.String != "" {
			refinedPrompt = planningSession.RefinedPrompt.String
		}
	}

	// Load project memories from database
	var projectMemories string
	if project != nil {
		projectMemories = r.buildMemorySection(task.ProjectID)
	}

	// Detect programming language from project
	var detectedLanguage tools.ProjectType
	if r.qualityGate != nil {
		detectedLanguage = r.qualityGate.GetProjectType()
	}

	ctx := &PromptContext{
		Task:               task,
		Session:            r.session,
		Project:            projectCtx,
		Tools:              toolNames,
		RefinedPrompt:      refinedPrompt,
		ToolDescriptions:   toolDescriptions,
		ProjectHints:       projectHints,
		ProjectMemories:    projectMemories,
		PredecessorContext: r.session.PredecessorContext,
		Language:           detectedLanguage,
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

// detectCompletion checks if the response indicates task completion via EVENT:task.complete
func (r *RalphLoop) detectCompletion(response string) bool {
	event, found := ParseEvent(response, r.session.ID, r.session.Hat)
	if !found {
		return false
	}
	return IsTerminalEvent(event.Topic)
}

// detectEvent parses the response for an EVENT:topic signal
// Returns the parsed Event or nil if no event found
func (r *RalphLoop) detectEvent(response string) *Event {
	event, found := ParseEvent(response, r.session.ID, r.session.Hat)
	if !found {
		return nil
	}
	return event
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
		"scratchpad":    r.session.Scratchpad,
	}

	// Include failure context if present
	if r.lastError != "" {
		state["last_error"] = r.lastError
		state["failed_at"] = r.failedAt
		state["recovery_hint"] = r.recoveryHint
	}

	// Generate handoff summary for easier review and resume
	if r.handoffGen != nil {
		handoff := r.handoffGen.Generate(r.session, r.session.Scratchpad, r.session.WorktreePath)
		state["handoff"] = handoff.FormatForAPI()
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

// SetFailureContext sets failure information for checkpoint recovery
func (r *RalphLoop) SetFailureContext(err error, failedAt, recoveryHint string) {
	if err != nil {
		r.lastError = err.Error()
	}
	r.failedAt = failedAt
	r.recoveryHint = recoveryHint
}

// ClearFailureContext clears any previous failure state
func (r *RalphLoop) ClearFailureContext() {
	r.lastError = ""
	r.failedAt = ""
	r.recoveryHint = ""
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
		TokensUsed   int64                       `json:"tokens_used"`
		DollarsUsed  float64                     `json:"dollars_used"`
		Hat          string                      `json:"hat"`
		Messages     []toolbelt.AnthropicMessage `json:"messages"`
		Scratchpad   string                      `json:"scratchpad,omitempty"`
		Handoff      map[string]any              `json:"handoff,omitempty"`
		// Failure context for recovery
		LastError    string                      `json:"last_error,omitempty"`
		FailedAt     string                      `json:"failed_at,omitempty"`
		RecoveryHint string                      `json:"recovery_hint,omitempty"`
	}

	if err := json.Unmarshal(checkpoint.State, &state); err != nil {
		return fmt.Errorf("failed to unmarshal checkpoint state: %w", err)
	}

	r.session.IterationCount = state.Iteration
	r.session.Hat = state.Hat
	if r.activity != nil {
		r.activity.SetHat(state.Hat)
	}

	// Update tools for the restored hat
	r.tools = GetToolDefinitionsForHat(state.Hat)

	// Restore scratchpad
	r.session.Scratchpad = security.SanitizeForPrompt(state.Scratchpad)

	// Sanitize restored messages to prevent prompt injection via stored content
	for i := range state.Messages {
		state.Messages[i].Content = sanitizeMessageContent(state.Messages[i].Content)
	}
	r.messages = state.Messages

	fmt.Printf("RestoreFromCheckpoint: restored iteration=%d, hat=%s, messages=%d, inputTokens=%d, outputTokens=%d, scratchpad=%d chars\n",
		state.Iteration, state.Hat, len(state.Messages), state.InputTokens, state.OutputTokens, len(state.Scratchpad))

	// Use new fields if available, otherwise estimate from legacy
	if state.InputTokens > 0 || state.OutputTokens > 0 {
		r.session.InputTokens = state.InputTokens
		r.session.OutputTokens = state.OutputTokens
	} else if state.TokensUsed > 0 {
		// Legacy: split evenly as approximation (input usually larger)
		r.session.InputTokens = state.TokensUsed * 2 / 3
		r.session.OutputTokens = state.TokensUsed / 3
	}

	// Build recovery/continuation context
	var recoveryMsg strings.Builder

	// Add handoff context if available
	if state.Handoff != nil {
		if continuation, ok := state.Handoff["continuation_prompt"].(string); ok && continuation != "" {
			recoveryMsg.WriteString("## Resuming Session\n\n")
			recoveryMsg.WriteString(continuation)
			recoveryMsg.WriteString("\n\n")
		}
	}

	// If checkpoint had failure context, add recovery message
	if state.LastError != "" {
		recoveryMsg.WriteString(fmt.Sprintf("Previous attempt failed: %s\n", state.LastError))
		recoveryMsg.WriteString(fmt.Sprintf("Location: %s\n", state.FailedAt))
		if state.RecoveryHint != "" {
			recoveryMsg.WriteString(fmt.Sprintf("Hint: %s\n", state.RecoveryHint))
		}
		recoveryMsg.WriteString("\nPlease try a different approach. If blocked, use EVENT:task.blocked:{\"reason\":\"description\"}.\n")
	}

	// Add recovery message if we have any content
	if recoveryMsg.Len() > 0 {
		r.messages = append(r.messages, toolbelt.AnthropicMessage{
			Role:    "user",
			Content: recoveryMsg.String(),
		})
		fmt.Printf("RestoreFromCheckpoint: added recovery/continuation context\n")
	}

	return nil
}

// sanitizeMessageContent sanitizes the Content field of an AnthropicMessage.
// Content can be either a string or []ContentBlock, both need sanitization.
func sanitizeMessageContent(content any) any {
	switch c := content.(type) {
	case string:
		return security.SanitizeForPrompt(c)
	case []any:
		// Handle []ContentBlock (arrives as []any from JSON unmarshal)
		for i, block := range c {
			if blockMap, ok := block.(map[string]any); ok {
				// Sanitize text content in the block
				if text, ok := blockMap["text"].(string); ok {
					blockMap["text"] = security.SanitizeForPrompt(text)
				}
				// Sanitize tool input if present
				if input, ok := blockMap["input"].(string); ok {
					blockMap["input"] = security.SanitizeForPrompt(input)
				}
				// Sanitize tool result content
				if content, ok := blockMap["content"].(string); ok {
					blockMap["content"] = security.SanitizeForPrompt(content)
				}
				c[i] = blockMap
			}
		}
		return c
	case []toolbelt.ContentBlock:
		// Handle typed []ContentBlock
		for i := range c {
			c[i].Text = security.SanitizeForPrompt(c[i].Text)
			c[i].Content = security.SanitizeForPrompt(c[i].Content)
		}
		return c
	default:
		// Unknown type, return as-is
		return content
	}
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
	sb.WriteString("When all items are addressed (done or failed), output EVENT:task.complete.\n\n")
	sb.WriteString("Begin working on the task. Follow your hat instructions and report progress.")

	return sb.String()
}

// processChecklistSignals detects and processes checklist update signals
// Uses findAllSignals to process all signals in a single pass without reset bugs
func (r *RalphLoop) processChecklistSignals(response string) {
	// Process all CHECKLIST_DONE signals
	doneSignals := findAllSignals(response, SignalChecklistDone)
	for _, sig := range doneSignals {
		itemID := strings.TrimSpace(sig)
		if itemID != "" {
			if err := r.db.UpdateChecklistItemStatus(itemID, db.ChecklistItemStatusDone, ""); err != nil {
				fmt.Printf("RalphLoop: warning - failed to update checklist item %s: %v\n", itemID, err)
			} else {
				if r.activity != nil {
					r.activity.RecordChecklistUpdate(r.session.IterationCount, itemID, db.ChecklistItemStatusDone, "")
				}
				fmt.Printf("RalphLoop: marked checklist item %s as done\n", itemID)
				r.manager.NotifyChecklistUpdated(r.session.TaskID)
			}
		}
	}

	// Process all CHECKLIST_FAILED signals
	failedSignals := findAllSignals(response, SignalChecklistFailed)
	for _, sig := range failedSignals {
		// Format: <item_id>:<reason>
		parts := strings.SplitN(sig, ":", 2)
		itemID := strings.TrimSpace(parts[0])
		reason := ""
		if len(parts) > 1 {
			reason = strings.TrimSpace(parts[1])
		}

		if itemID != "" {
			if err := r.db.UpdateChecklistItemStatus(itemID, db.ChecklistItemStatusFailed, reason); err != nil {
				fmt.Printf("RalphLoop: warning - failed to update checklist item %s: %v\n", itemID, err)
			} else {
				if r.activity != nil {
					r.activity.RecordChecklistUpdate(r.session.IterationCount, itemID, db.ChecklistItemStatusFailed, reason)
				}
				fmt.Printf("RalphLoop: marked checklist item %s as failed: %s\n", itemID, reason)
				r.manager.NotifyChecklistUpdated(r.session.TaskID)
			}
		}
	}
}

// findAllSignals finds all instances of a signal and extracts their content
func findAllSignals(content, signal string) []string {
	var results []string
	remaining := content

	for {
		idx := strings.Index(remaining, signal)
		if idx == -1 {
			break
		}

		// Extract signal content until newline or end
		start := idx + len(signal)
		contentAfter := remaining[start:]

		// Find end of signal content (newline or double newline)
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

// parseScratchpadSignal extracts scratchpad content from a response
// The scratchpad continues from the signal until the next major signal or end of text
func parseScratchpadSignal(text string) (string, bool) {
	idx := strings.Index(text, SignalScratchpad)
	if idx == -1 {
		return "", false
	}

	// Extract from signal to end or next major signal
	content := text[idx+len(SignalScratchpad):]

	// Find end of scratchpad (next signal or end)
	// Check for common signals that would end the scratchpad
	endSignals := []string{
		SignalEvent,
		SignalChecklistDone,
		SignalChecklistFailed,
	}

	endIdx := len(content)
	for _, sig := range endSignals {
		if sigIdx := strings.Index(content, sig); sigIdx != -1 && sigIdx < endIdx {
			endIdx = sigIdx
		}
	}

	return strings.TrimSpace(content[:endIdx]), true
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

// formatChecklistIssues formats checklist issues for display to the AI
func (r *RalphLoop) formatChecklistIssues(issues []db.ChecklistIssue) string {
	var sb strings.Builder
	for _, issue := range issues {
		sb.WriteString(fmt.Sprintf("- [%s] %s (id: %s)", issue.Status, issue.Description, issue.ItemID))
		if issue.Notes != "" {
			sb.WriteString(fmt.Sprintf(" - %s", issue.Notes))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// hatContinuations provides hat-specific continuation prompts using the event system
var hatContinuations = map[string]string{
	"explorer": `Continue exploring. When you have enough information:
- Plan is ready: EVENT:plan.complete
- Design is ready: EVENT:design.complete`,

	"planner": `Continue planning. When the strategy is ready:
- Plan complete, needs design: EVENT:plan.complete
- Plan complete, ready to build: EVENT:design.complete`,

	"designer": `Continue designing. When the architecture is ready:
- Design complete: EVENT:design.complete`,

	"creator": `Continue implementing. Report progress with CHECKLIST_DONE/FAILED signals.
When implementation is complete:
- Ready for review: EVENT:implementation.done
If blocked: EVENT:task.blocked:{"reason":"description of blocker"}`,

	"critic": `Continue reviewing. When review is complete:
- Approved, ready to finalize: EVENT:review.approved
- Needs fixes: EVENT:review.rejected
If blocked: EVENT:task.blocked:{"reason":"description of blocker"}`,

	"editor": `Continue polishing. When ready to finalize:
- Commit any remaining changes
- Create PR if needed
- Task complete: EVENT:task.complete
If blocked: EVENT:task.blocked:{"reason":"description of blocker"}`,

	"resolver": `Continue resolving blockers. When resolved:
- Blocker cleared: EVENT:resolved
- Task complete (if nothing left to do): EVENT:task.complete`,
}

// getContinuationPrompt returns a hat-specific continuation prompt
func (r *RalphLoop) getContinuationPrompt() string {
	if cont, ok := hatContinuations[r.session.Hat]; ok {
		return cont
	}
	return "Continue. Output EVENT:task.complete when done or EVENT:<topic> to signal progress."
}

// processMemorySignals detects and stores memory signals from the response
func (r *RalphLoop) processMemorySignals(response string) {
	memories := findAllSignals(response, SignalMemory)
	if len(memories) == 0 {
		return
	}

	// Get task for project ID
	task, err := r.db.GetTaskByID(r.session.TaskID)
	if err != nil || task == nil {
		fmt.Printf("RalphLoop: warning - cannot store memories without task: %v\n", err)
		return
	}

	for _, sig := range memories {
		memory, valid := parseMemorySignal(sig, task.ProjectID, r.session)
		if !valid {
			continue
		}

		if err := r.db.CreateMemory(memory); err != nil {
			fmt.Printf("RalphLoop: warning - failed to store memory: %v\n", err)
			continue
		}

		// Record memory creation in activity log
		if r.activity != nil {
			r.activity.RecordMemoryCreated(r.session.IterationCount, &MemoryCreatedData{
				MemoryID: memory.ID,
				Type:     string(memory.Type),
				Title:    memory.Title,
				Source:   string(memory.Source),
			})
		}

		r.activity.Debug(r.session.IterationCount, fmt.Sprintf("Stored memory: %s - %s", memory.Type, memory.Title))
	}
}

// parseMemorySignal parses a memory signal into a Memory struct
// Format: MEMORY:<type>:<content>
func parseMemorySignal(sig, projectID string, session *ActiveSession) (*db.Memory, bool) {
	parts := strings.SplitN(sig, ":", 2)
	if len(parts) != 2 {
		return nil, false
	}

	memType := strings.TrimSpace(parts[0])
	content := strings.TrimSpace(parts[1])

	if content == "" || !db.IsValidMemoryType(memType) {
		return nil, false
	}

	// Sanitize content to prevent prompt injection
	content = security.SanitizeForPrompt(content)

	// Extract title (first sentence or line)
	title := content
	if idx := strings.IndexAny(content, ".\n"); idx != -1 {
		title = content[:idx]
	}
	if len(title) > 100 {
		title = title[:100] + "..."
	}

	return &db.Memory{
		ID:                 uuid.New().String(),
		ProjectID:          projectID,
		Type:               db.MemoryType(memType),
		Title:              title,
		Content:            content,
		Confidence:         db.InitialConfidenceExplicit,
		CreatedByHat:       session.Hat,
		CreatedByTaskID:    toNullString(session.TaskID),
		CreatedBySessionID: toNullString(session.ID),
		Source:             db.SourceExplicit,
		CreatedAt:          time.Now(),
	}, true
}

// toNullString converts a string to sql.NullString
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// buildCommentData builds a CommentData struct for GitHub issue comments
func (r *RalphLoop) buildCommentData(ctx context.Context) *github.CommentData {
	data := &github.CommentData{
		SessionID:   r.session.ID,
		Iteration:   r.session.IterationCount,
		TotalTokens: r.session.InputTokens + r.session.OutputTokens,
		Hat:         r.session.Hat,
	}

	// Get task for branch name
	if task, err := r.db.GetTaskByID(r.session.TaskID); err == nil && task != nil {
		data.Branch = task.GetBranchName()

		// Get checklist items for progress
		if checklist, err := r.db.GetChecklistByTaskID(task.ID); err == nil && checklist != nil {
			if items, err := r.db.GetChecklistItems(checklist.ID); err == nil {
				for _, item := range items {
					data.ChecklistItems = append(data.ChecklistItems, github.ChecklistItemStatus{
						Description: item.Description,
						Status:      item.Status,
					})
				}
			}
		}
	}

	// Note: Changed files could be populated from git status if needed
	// For now, we rely on the checklist for progress tracking

	return data
}

// getCompletionSummary returns a summary of completed checklist items
func (r *RalphLoop) getCompletionSummary() []string {
	var summary []string

	task, err := r.db.GetTaskByID(r.session.TaskID)
	if err != nil || task == nil {
		return summary
	}

	checklist, err := r.db.GetChecklistByTaskID(task.ID)
	if err != nil || checklist == nil {
		return summary
	}

	items, err := r.db.GetChecklistItems(checklist.ID)
	if err != nil {
		return summary
	}

	for _, item := range items {
		if item.Status == "done" {
			summary = append(summary, item.Description)
		}
	}

	return summary
}
