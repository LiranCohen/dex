// Package quest provides Quest conversation handling for Poindexter
package quest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/api/websocket"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/session"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// Model constants for quest conversations
const (
	ModelSonnet = "claude-sonnet-4-5-20250929"
	ModelOpus   = "claude-opus-4-5-20251101"
)

// ObjectiveDraft represents a draft objective (task) proposed by Dex
type ObjectiveDraft struct {
	DraftID             string    `json:"draft_id"`
	Title               string    `json:"title"`
	Description         string    `json:"description"`
	Hat                 string    `json:"hat"`
	Checklist           Checklist `json:"checklist"`
	BlockedBy           []string  `json:"blocked_by,omitempty"`
	AutoStart           bool      `json:"auto_start"`
	Complexity          string    `json:"complexity,omitempty"`          // "simple" or "complex" - determines AI model
	EstimatedIterations int       `json:"estimated_iterations,omitempty"`
	EstimatedBudget     float64   `json:"estimated_budget,omitempty"` // estimated cost in dollars
}

// Checklist represents must-have and optional items for an objective
type Checklist struct {
	MustHave []string `json:"must_have"`
	Optional []string `json:"optional,omitempty"`
}

// Question represents a clarifying question from Dex
type Question struct {
	DraftID  string   `json:"draft_id,omitempty"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

// PreflightCheck represents the result of pre-flight checks before starting a task
type PreflightCheck struct {
	OK       bool     `json:"ok"`
	Warnings []string `json:"warnings,omitempty"`
}

// GitHubClientFetcher is a function that returns a GitHub client for a given login/org
type GitHubClientFetcher func(ctx context.Context, login string) (*toolbelt.GitHubClient, error)

// Handler manages Quest conversations with Dex
type Handler struct {
	db             *db.DB
	client         *toolbelt.AnthropicClient
	github         *toolbelt.GitHubClient  // Static client (PAT-based)
	githubFetcher  GitHubClientFetcher     // Dynamic client fetcher (GitHub App)
	hub            *websocket.Hub
	promptLoader   *session.PromptLoader
	githubUsername string        // cached GitHub username
	toolSet        *tools.Set    // Read-only tools for Quest exploration
	readOnlyTools  []toolbelt.AnthropicTool
}

// NewHandler creates a new Quest handler
func NewHandler(database *db.DB, client *toolbelt.AnthropicClient, hub *websocket.Hub) *Handler {
	// Build read-only tools for Quest exploration
	toolSet := tools.ReadOnlyTools()
	readOnlyTools := make([]toolbelt.AnthropicTool, 0, len(toolSet.All())+len(QuestTools()))
	for _, t := range toolSet.All() {
		readOnlyTools = append(readOnlyTools, toolbelt.AnthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	// Add quest-specific tools for objective management
	readOnlyTools = append(readOnlyTools, QuestTools()...)

	return &Handler{
		db:            database,
		client:        client,
		hub:           hub,
		toolSet:       toolSet,
		readOnlyTools: readOnlyTools,
	}
}

// SetGitHubClient sets the static GitHub client for the handler (PAT-based)
func (h *Handler) SetGitHubClient(client *toolbelt.GitHubClient) {
	h.github = client
}

// SetGitHubClientFetcher sets the dynamic GitHub client fetcher (GitHub App)
func (h *Handler) SetGitHubClientFetcher(fetcher GitHubClientFetcher) {
	h.githubFetcher = fetcher
}

// SetPromptLoader sets the prompt loader for the handler
func (h *Handler) SetPromptLoader(loader *session.PromptLoader) {
	h.promptLoader = loader
}

// getGitHubUsername returns the cached GitHub username/org, fetching it if needed
// Tries: 1) cached value, 2) onboarding progress (org name), 3) GitHub client (static or fetched)
func (h *Handler) getGitHubUsername(ctx context.Context) string {
	if h.githubUsername != "" {
		return h.githubUsername
	}

	// Try to get org name from onboarding progress (works with GitHub App auth)
	progress, err := h.db.GetOnboardingProgress()
	if err == nil && progress != nil {
		orgName := progress.GetGitHubOrgName()
		if orgName != "" {
			h.githubUsername = orgName
			return orgName
		}
	}

	// Get GitHub client - try static client first, then fetcher
	githubClient := h.github
	if githubClient == nil && h.githubFetcher != nil {
		fetchedClient, err := h.githubFetcher(ctx, "")
		if err == nil {
			githubClient = fetchedClient
		}
	}

	// Try to get username from GitHub client
	if githubClient != nil {
		username, err := githubClient.GetUsername(ctx)
		if err == nil && username != "" {
			h.githubUsername = username
			return username
		}
	}

	return ""
}

// buildQuestPrompt builds the system prompt for quest conversations using PromptLoom
func (h *Handler) buildQuestPrompt(ctx context.Context, projectID, questID string) string {
	var basePrompt string

	// Try to get prompt from PromptLoom
	if h.promptLoader != nil {
		prompt, err := h.promptLoader.Get("quest", nil)
		if err == nil {
			basePrompt = prompt
		} else {
			fmt.Printf("warning: failed to load quest prompt from PromptLoom: %v, using fallback\n", err)
		}
	}

	// Fallback if PromptLoom not available
	if basePrompt == "" {
		basePrompt = "You are Dex, an AI orchestration assistant. Help users break down work into objectives."
	}

	// Add dynamic context that can't be in YAML
	return basePrompt + h.buildUserContext(ctx) + h.buildCrossQuestContext(projectID, questID)
}

// ProcessMessage handles a user message in a quest conversation
func (h *Handler) ProcessMessage(ctx context.Context, questID, content string) (*db.QuestMessage, error) {
	if h.client == nil {
		return nil, fmt.Errorf("anthropic client not configured")
	}

	// Get the quest
	quest, err := h.db.GetQuestByID(questID)
	if err != nil {
		return nil, fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}
	if quest.Status != db.QuestStatusActive {
		return nil, fmt.Errorf("quest is not active")
	}

	// Get project for tool execution context
	project, err := h.db.GetProjectByID(quest.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// User message was already saved by the API handler
	// Get all messages for context
	messages, err := h.db.GetQuestMessages(questID)
	if err != nil {
		return nil, fmt.Errorf("failed to get quest messages: %w", err)
	}

	// Convert to Anthropic message format
	anthropicMessages := make([]toolbelt.AnthropicMessage, len(messages))
	for i, msg := range messages {
		anthropicMessages[i] = toolbelt.AnthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Select model based on quest settings
	model := ModelSonnet
	if quest.Model == db.QuestModelOpus {
		model = ModelOpus
	}

	// Build system prompt with user context and cross-quest awareness
	systemPrompt := h.buildQuestPrompt(ctx, quest.ProjectID, questID)

	// Create tool executor for this project (read-only mode)
	// Always create an executor - use project path if available, otherwise use temp dir for web tools
	workDir := os.TempDir()
	if project != nil && project.RepoPath != "" && h.isValidProjectPath(project.RepoPath) {
		workDir = project.RepoPath
	}
	executor := tools.NewExecutor(workDir, h.toolSet, true)

	// Collect tool calls for this response
	var allToolCalls []db.QuestToolCall

	// Tool use loop - continue until we get a non-tool response
	maxToolIterations := 10
	for i := 0; i < maxToolIterations; i++ {
		// Track accumulated streaming content for this iteration
		var streamedContent strings.Builder

		// Streaming callback to broadcast content deltas in real-time
		onDelta := func(delta string) {
			streamedContent.WriteString(delta)
			if h.hub != nil {
				h.hub.Broadcast(websocket.Message{
					Type: "quest.content_delta",
					Payload: map[string]any{
						"quest_id": questID,
						"delta":    delta,
						"content":  streamedContent.String(), // Full content so far
					},
				})
			}
		}

		// Call the model with streaming
		response, err := h.client.ChatWithStreaming(ctx, &toolbelt.AnthropicChatRequest{
			Model:     model,
			MaxTokens: 4096,
			System:    systemPrompt,
			Messages:  anthropicMessages,
			Tools:     h.readOnlyTools,
		}, onDelta)
		if err != nil {
			return nil, fmt.Errorf("failed to get response from Dex: %w", err)
		}

		// Check if model wants to use tools
		if response.HasToolUse() {
			toolBlocks := response.ToolUseBlocks()

			// Add assistant message with tool_use blocks
			anthropicMessages = append(anthropicMessages, toolbelt.AnthropicMessage{
				Role:    "assistant",
				Content: response.NormalizedContent(),
			})

			// Execute tools and collect results
			var results []toolbelt.ContentBlock
			for _, block := range toolBlocks {
				// Broadcast tool call start
				if h.hub != nil {
					h.hub.Broadcast(websocket.Message{
						Type: "quest.tool_call",
						Payload: map[string]any{
							"quest_id":  questID,
							"tool_name": block.Name,
							"status":    "running",
						},
					})
				}

				// Execute the tool - use quest handler for quest-specific tools
				start := time.Now()
				var result tools.Result
				if IsQuestTool(block.Name) {
					result = h.executeQuestTool(ctx, questID, block.Name, block.Input)
				} else {
					result = executor.Execute(ctx, block.Name, block.Input)
				}
				durationMs := time.Since(start).Milliseconds()

				// Record tool call
				toolCall := db.QuestToolCall{
					ToolName:   block.Name,
					Input:      block.Input,
					Output:     result.Output,
					IsError:    result.IsError,
					DurationMs: durationMs,
				}
				allToolCalls = append(allToolCalls, toolCall)

				// Broadcast tool result
				if h.hub != nil {
					h.hub.Broadcast(websocket.Message{
						Type: "quest.tool_result",
						Payload: map[string]any{
							"quest_id":    questID,
							"tool_name":   block.Name,
							"output":      truncateForBroadcast(result.Output, 1000),
							"is_error":    result.IsError,
							"duration_ms": durationMs,
						},
					})
				}

				results = append(results, toolbelt.ContentBlock{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   result.Output,
					IsError:   result.IsError,
				})
			}

			// Add tool results as user message
			anthropicMessages = append(anthropicMessages, toolbelt.AnthropicMessage{
				Role:    "user",
				Content: results,
			})

			// Continue loop to get model's response to tool results
			continue
		}

		// No tool use - this is the final response
		assistantContent := response.Text()
		assistantMsg, err := h.db.CreateQuestMessageWithToolCalls(questID, "assistant", assistantContent, allToolCalls)
		if err != nil {
			return nil, fmt.Errorf("failed to store assistant response: %w", err)
		}

		// Parse signals from the response
		drafts := h.parseObjectiveDrafts(assistantContent)
		questions := h.parseQuestions(assistantContent)
		questReady := h.parseQuestReady(assistantContent)

		// Broadcast the assistant message
		if h.hub != nil {
			h.hub.Broadcast(websocket.Message{
				Type: "quest.message",
				Payload: map[string]any{
					"quest_id": questID,
					"message": map[string]any{
						"id":         assistantMsg.ID,
						"quest_id":   assistantMsg.QuestID,
						"role":       assistantMsg.Role,
						"content":    assistantMsg.Content,
						"tool_calls": allToolCalls,
						"created_at": assistantMsg.CreatedAt,
					},
				},
			})

			// Broadcast any draft objectives
			for _, draft := range drafts {
				h.hub.Broadcast(websocket.Message{
					Type: "quest.objective_draft",
					Payload: map[string]any{
						"quest_id": questID,
						"draft":    draft,
					},
				})
			}

			// Broadcast any questions
			for _, q := range questions {
				h.hub.Broadcast(websocket.Message{
					Type: "quest.question",
					Payload: map[string]any{
						"quest_id": questID,
						"question": q,
					},
				})
			}

			// Broadcast quest ready signal
			if questReady != nil {
				h.hub.Broadcast(websocket.Message{
					Type: "quest.ready",
					Payload: map[string]any{
						"quest_id": questID,
						"drafts":   questReady["drafts"],
						"summary":  questReady["summary"],
					},
				})
			}
		}

		// Auto-generate title from first user message if not set
		if !quest.Title.Valid && len(messages) >= 1 {
			title := h.generateTitle(messages[0].Content)
			if title != "" {
				h.db.UpdateQuestTitle(questID, title)
			}
		}

		return assistantMsg, nil
	}

	return nil, fmt.Errorf("tool execution loop exceeded maximum iterations")
}

// truncateForBroadcast truncates a string for WebSocket broadcast
func truncateForBroadcast(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseObjectiveDrafts extracts OBJECTIVE_DRAFT signals from a response
func (h *Handler) parseObjectiveDrafts(content string) []ObjectiveDraft {
	var drafts []ObjectiveDraft

	// Find all OBJECTIVE_DRAFT signals
	marker := "OBJECTIVE_DRAFT:"
	remaining := content

	for {
		idx := strings.Index(remaining, marker)
		if idx == -1 {
			break
		}

		// Extract JSON portion using balanced brace matching
		jsonStart := idx + len(marker)
		jsonStr, endIdx := extractJSONObject(remaining[jsonStart:])
		if jsonStr == "" {
			remaining = remaining[jsonStart:]
			continue
		}

		var draft ObjectiveDraft
		if err := json.Unmarshal([]byte(jsonStr), &draft); err != nil {
			// Try to fix common JSON issues
			fixed := h.fixJSON(jsonStr)
			if err := json.Unmarshal([]byte(fixed), &draft); err != nil {
				fmt.Printf("warning: failed to parse OBJECTIVE_DRAFT JSON: %v\n", err)
				remaining = remaining[jsonStart+endIdx:]
				continue
			}
		}

		if draft.Title != "" {
			drafts = append(drafts, draft)
		}

		remaining = remaining[jsonStart+endIdx:]
	}

	return drafts
}

// extractJSONObject extracts a JSON object from a string using balanced brace matching
// Returns the JSON string and the end index
func extractJSONObject(s string) (string, int) {
	// Skip whitespace
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	if start >= len(s) || s[start] != '{' {
		return "", 0
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1], i + 1
			}
		}
	}

	// Unbalanced braces
	return "", 0
}

// parseQuestions extracts QUESTION signals from a response
func (h *Handler) parseQuestions(content string) []Question {
	var questions []Question

	// Match QUESTION:{...} patterns
	re := regexp.MustCompile(`QUESTION:\s*(\{[^}]+\})`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		var q Question
		if err := json.Unmarshal([]byte(match[1]), &q); err != nil {
			fixed := h.fixJSON(match[1])
			if err := json.Unmarshal([]byte(fixed), &q); err != nil {
				continue
			}
		}

		if q.Question != "" {
			questions = append(questions, q)
		}
	}

	return questions
}

// parseQuestReady extracts QUEST_READY signal from a response
func (h *Handler) parseQuestReady(content string) map[string]any {
	// Match QUEST_READY:{...} pattern
	re := regexp.MustCompile(`QUEST_READY:\s*(\{[^}]+\})`)
	match := re.FindStringSubmatch(content)

	if len(match) < 2 {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(match[1]), &result); err != nil {
		fixed := h.fixJSON(match[1])
		if err := json.Unmarshal([]byte(fixed), &result); err != nil {
			return nil
		}
	}

	return result
}

// fixJSON attempts to fix common JSON formatting issues
func (h *Handler) fixJSON(s string) string {
	// Replace single quotes with double quotes
	s = strings.ReplaceAll(s, "'", "\"")
	// Fix trailing commas
	re := regexp.MustCompile(`,\s*([}\]])`)
	s = re.ReplaceAllString(s, "$1")
	return s
}

// generateTitle creates a short title from the first user message
func (h *Handler) generateTitle(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "Untitled Quest"
	}

	// Extract first sentence with proper punctuation handling
	title := extractFirstSentence(content)

	// Clean up the title
	title = strings.TrimSpace(title)
	if title == "" {
		return "Untitled Quest"
	}

	// Limit to 100 chars
	if len(title) > 100 {
		title = title[:97] + "..."
	}

	return title
}

// extractFirstSentence extracts the first sentence from content
func extractFirstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Find first sentence-ending punctuation
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			// Make sure it's not a decimal point or abbreviation
			if i+1 < len(s) {
				next := rune(s[i+1])
				// If followed by digit, it's likely a decimal
				if next >= '0' && next <= '9' {
					continue
				}
				// If followed by space or newline, it's end of sentence
				if next == ' ' || next == '\n' || next == '\r' {
					return strings.TrimSpace(s[:i+1])
				}
			} else {
				// End of string
				return strings.TrimSpace(s[:i+1])
			}
		}
		if i > 150 { // Max scan length
			break
		}
	}

	// No sentence-ending punctuation found, use first line or truncate
	if idx := strings.IndexAny(s, "\n\r"); idx > 0 && idx < 150 {
		return strings.TrimSpace(s[:idx])
	}

	// Truncate at word boundary
	if len(s) > 100 {
		lastSpace := strings.LastIndex(s[:100], " ")
		if lastSpace > 50 {
			return s[:lastSpace] + "..."
		}
		return s[:97] + "..."
	}

	return s
}

// complexityToPriority maps complexity and estimated iterations to a priority value
// Priority scale: 1 (critical) to 5 (low)
func complexityToPriority(complexity string, estimatedIterations int) int {
	switch complexity {
	case "simple":
		return 4 // Low priority - simple tasks
	case "complex":
		if estimatedIterations > 50 {
			return 1 // Critical - large complex tasks
		}
		if estimatedIterations > 20 {
			return 2 // High - medium complex tasks
		}
		return 2 // High - complex tasks default
	default:
		// Medium complexity or unspecified
		if estimatedIterations > 30 {
			return 2 // High
		}
		return 3 // Medium - default
	}
}

// validStartingHats defines hats that can be used as starting hats for tasks
var validStartingHats = []string{"explorer", "planner", "creator", "designer"}

// CreateObjectiveFromDraft creates a task from an accepted draft
func (h *Handler) CreateObjectiveFromDraft(ctx context.Context, questID string, draft ObjectiveDraft, selectedOptional []int) (*db.Task, error) {
	// Validate hat
	if !session.IsValidHat(draft.Hat) {
		return nil, fmt.Errorf("invalid hat: %s", draft.Hat)
	}

	// Validate hat is valid as a starting hat
	isValidStarter := false
	for _, s := range validStartingHats {
		if draft.Hat == s {
			isValidStarter = true
			break
		}
	}
	if !isValidStarter {
		return nil, fmt.Errorf("hat %s cannot be used as starting hat; valid starters: %v", draft.Hat, validStartingHats)
	}

	// Get the quest to find the project
	quest, err := h.db.GetQuestByID(questID)
	if err != nil {
		return nil, fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}

	// Determine model based on complexity
	model := db.TaskModelSonnet
	if draft.Complexity == "complex" {
		model = db.TaskModelOpus
	}

	// Calculate priority from complexity and estimated iterations
	priority := complexityToPriority(draft.Complexity, draft.EstimatedIterations)

	// Create the task with auto_start preference
	task, err := h.db.CreateTaskForQuestWithStatus(
		questID,
		quest.ProjectID,
		draft.Title,
		draft.Description,
		draft.Hat,
		db.TaskTypeTask,
		model,
		priority,
		db.TaskStatusReady,
		draft.AutoStart,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Create the checklist
	checklist, err := h.db.CreateTaskChecklist(task.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create checklist: %w", err)
	}

	// Add must-have items
	sortOrder := 0
	for _, item := range draft.Checklist.MustHave {
		_, err := h.db.CreateChecklistItem(checklist.ID, item, sortOrder)
		if err != nil {
			return nil, fmt.Errorf("failed to create checklist item: %w", err)
		}
		sortOrder++
	}

	// Add selected optional items
	for _, idx := range selectedOptional {
		if idx >= 0 && idx < len(draft.Checklist.Optional) {
			item := draft.Checklist.Optional[idx]
			_, err := h.db.CreateChecklistItem(checklist.ID, item, sortOrder)
			if err != nil {
				return nil, fmt.Errorf("failed to create checklist item: %w", err)
			}
			sortOrder++
		}
	}

	// Broadcast task created
	if h.hub != nil {
		h.hub.Broadcast(websocket.Message{
			Type: "task.created",
			Payload: map[string]any{
				"task_id":   task.ID,
				"quest_id":  questID,
				"title":     task.Title,
				"auto_start": draft.AutoStart,
			},
		})
	}

	return task, nil
}

// RunPreflightChecks performs pre-flight checks for a project before starting a task
func (h *Handler) RunPreflightChecks(projectID string) (*PreflightCheck, error) {
	// Get the project to find the repo path
	project, err := h.db.GetProjectByID(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	check := &PreflightCheck{OK: true}

	// Check if the project path is valid for checking
	if !h.isValidProjectPath(project.RepoPath) {
		// Project path is not configured or points to a system directory
		// This is OK for new projects - the task will create its own directory
		check.Warnings = append(check.Warnings, "New project - a working directory will be created when the objective starts")
		return check, nil
	}

	// Check for uncommitted changes
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = project.RepoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Git command failed - maybe not a git repo yet
		// This is OK for new projects
		check.Warnings = append(check.Warnings, "Not a git repository yet - git will be initialized when the objective starts")
	} else if stdout.Len() > 0 {
		// There are uncommitted changes
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		check.Warnings = append(check.Warnings, fmt.Sprintf("Uncommitted changes (%d files)", len(lines)))
		check.OK = false
	}

	// Check for merge conflicts (only if we're in a git repo)
	cmd = exec.Command("git", "diff", "--check")
	cmd.Dir = project.RepoPath
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stdout.Len() > 0 {
			check.Warnings = append(check.Warnings, "Potential merge conflicts detected")
			check.OK = false
		}
	}

	return check, nil
}

// isValidProjectPath checks if a path is appropriate for use as a project directory
func (h *Handler) isValidProjectPath(path string) bool {
	if path == "" || path == "." || path == ".." {
		return false
	}

	// System directories that should never be used (including subdirectories)
	systemPrefixes := []string{
		"/usr/",
		"/bin/",
		"/sbin/",
		"/lib/",
		"/etc/",
	}

	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}

	return true
}

// GetPreflightCheck performs preflight checks for a quest's project
func (h *Handler) GetPreflightCheck(questID string) (*PreflightCheck, error) {
	quest, err := h.db.GetQuestByID(questID)
	if err != nil {
		return nil, fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}

	return h.RunPreflightChecks(quest.ProjectID)
}

// buildUserContext creates a context section about the user
func (h *Handler) buildUserContext(ctx context.Context) string {
	username := h.getGitHubUsername(ctx)
	if username == "" {
		return ""
	}

	return fmt.Sprintf(`

## User Context
- GitHub username: %s
- Default Go module path: github.com/%s/<project-name>

Use this information when creating repositories, Go modules, or any resources that need the user's identity. Do NOT ask for this information.
`, username, username)
}

// buildCrossQuestContext creates a context section about other active quests
func (h *Handler) buildCrossQuestContext(projectID, currentQuestID string) string {
	// Get all active quests for the project
	activeQuests, err := h.db.GetActiveQuests(projectID)
	if err != nil || len(activeQuests) <= 1 {
		// No other active quests, or error - skip context
		return ""
	}

	var context strings.Builder
	context.WriteString("\n\n## Other Active Quests\nBe aware of these other active quests in this project to avoid conflicts or duplicated work:\n")

	for _, q := range activeQuests {
		if q.ID == currentQuestID {
			continue // Skip the current quest
		}

		title := q.GetTitle()
		if title == "" {
			title = "Untitled Quest"
		}

		// Get quest summary
		summary, err := h.db.GetQuestSummary(q.ID)
		if err != nil {
			continue
		}

		context.WriteString(fmt.Sprintf("\n- **%s** (ID: %s)\n", title, q.ID))
		if summary.TotalTasks > 0 {
			context.WriteString(fmt.Sprintf("  - %d objectives: %d running, %d pending, %d completed\n",
				summary.TotalTasks, summary.RunningTasks, summary.PendingTasks, summary.CompletedTasks))
		}

		// Get tasks to show what's being worked on
		tasks, err := h.db.GetTasksByQuestID(q.ID)
		if err == nil && len(tasks) > 0 {
			context.WriteString("  - Objectives: ")
			taskNames := make([]string, 0, len(tasks))
			for _, t := range tasks {
				if len(taskNames) >= 3 {
					taskNames = append(taskNames, "...")
					break
				}
				taskNames = append(taskNames, t.Title)
			}
			context.WriteString(strings.Join(taskNames, ", "))
			context.WriteString("\n")
		}
	}

	return context.String()
}

// executeQuestTool executes quest-specific tools that need database access
func (h *Handler) executeQuestTool(ctx context.Context, questID, toolName string, input map[string]any) tools.Result {
	switch toolName {
	case "list_objectives":
		return h.executeListObjectives(questID)
	case "get_objective_details":
		objectiveID, _ := input["objective_id"].(string)
		return h.executeGetObjectiveDetails(objectiveID)
	case "cancel_objective":
		objectiveID, _ := input["objective_id"].(string)
		reason, _ := input["reason"].(string)
		return h.executeCancelObjective(questID, objectiveID, reason)
	default:
		return tools.Result{Output: fmt.Sprintf("Unknown quest tool: %s", toolName), IsError: true}
	}
}

// executeListObjectives lists all objectives for a quest
func (h *Handler) executeListObjectives(questID string) tools.Result {
	tasks, err := h.db.GetTasksByQuestID(questID)
	if err != nil {
		return tools.Result{Output: fmt.Sprintf("Failed to get objectives: %v", err), IsError: true}
	}

	if len(tasks) == 0 {
		return tools.Result{Output: "No objectives have been created for this quest yet."}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d objectives:\n\n", len(tasks)))

	for _, task := range tasks {
		sb.WriteString(fmt.Sprintf("## %s\n", task.Title))
		sb.WriteString(fmt.Sprintf("- **ID:** %s\n", task.ID))
		sb.WriteString(fmt.Sprintf("- **Status:** %s\n", task.Status))
		sb.WriteString(fmt.Sprintf("- **Hat:** %s\n", task.Hat.String))

		// Get checklist progress
		if checklist, err := h.db.GetChecklistByTaskID(task.ID); err == nil && checklist != nil {
			if items, err := h.db.GetChecklistItems(checklist.ID); err == nil && len(items) > 0 {
				completed := 0
				for _, item := range items {
					if item.Status == "done" {
						completed++
					}
				}
				sb.WriteString(fmt.Sprintf("- **Progress:** %d/%d checklist items completed\n", completed, len(items)))
			}
		}

		sb.WriteString("\n")
	}

	return tools.Result{Output: sb.String()}
}

// executeGetObjectiveDetails gets detailed info about a specific objective
func (h *Handler) executeGetObjectiveDetails(objectiveID string) tools.Result {
	if objectiveID == "" {
		return tools.Result{Output: "objective_id is required", IsError: true}
	}

	task, err := h.db.GetTaskByID(objectiveID)
	if err != nil {
		return tools.Result{Output: fmt.Sprintf("Failed to get objective: %v", err), IsError: true}
	}
	if task == nil {
		return tools.Result{Output: fmt.Sprintf("Objective not found: %s", objectiveID), IsError: true}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", task.Title))
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", task.Status))
	sb.WriteString(fmt.Sprintf("**Hat:** %s\n", task.Hat.String))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n", task.CreatedAt.Format("2006-01-02 15:04:05")))

	if task.Description.Valid && task.Description.String != "" {
		sb.WriteString(fmt.Sprintf("\n## Description\n%s\n", task.Description.String))
	}

	// Get checklist items
	if checklist, err := h.db.GetChecklistByTaskID(task.ID); err == nil && checklist != nil {
		if items, err := h.db.GetChecklistItems(checklist.ID); err == nil && len(items) > 0 {
			sb.WriteString("\n## Checklist Progress\n")
			for _, item := range items {
				status := "[ ]"
				if item.Status == "done" {
					status = "[x]"
				} else if item.Status == "failed" {
					status = "[!]"
				} else if item.Status == "skipped" {
					status = "[-]"
				}
				sb.WriteString(fmt.Sprintf("- %s %s (status: %s)\n", status, item.Description, item.Status))
			}
		}
	}

	// Get session history
	sessions, err := h.db.ListSessionsByTask(objectiveID)
	if err == nil && len(sessions) > 0 {
		sb.WriteString(fmt.Sprintf("\n## Session History (%d sessions)\n", len(sessions)))
		for i, sess := range sessions {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("... and %d more sessions\n", len(sessions)-5))
				break
			}
			sb.WriteString(fmt.Sprintf("- Session %s: status=%s, iterations=%d",
				sess.ID[:8], sess.Status, sess.IterationCount))
			if sess.Outcome.Valid && sess.Outcome.String != "" {
				sb.WriteString(fmt.Sprintf(", outcome=%s", sess.Outcome.String))
			}
			sb.WriteString("\n")
		}
	}

	// Status-specific information
	switch task.Status {
	case db.TaskStatusPaused:
		sb.WriteString("\n## Status Notes\n")
		sb.WriteString("This objective is **paused**. It may have encountered an error or exceeded its budget.\n")
		sb.WriteString("You can either:\n")
		sb.WriteString("1. Ask the user to resume it if the issue might be transient\n")
		sb.WriteString("2. Cancel it and create a new objective with a different approach\n")
	case db.TaskStatusCancelled:
		sb.WriteString("\n## Status Notes\n")
		sb.WriteString("This objective was **cancelled**. Consider creating a replacement objective if the work is still needed.\n")
	case db.TaskStatusCompleted:
		sb.WriteString("\n## Status Notes\n")
		sb.WriteString("This objective is **completed**.\n")
	}

	return tools.Result{Output: sb.String()}
}

// executeCancelObjective cancels an objective
func (h *Handler) executeCancelObjective(questID, objectiveID, reason string) tools.Result {
	if objectiveID == "" {
		return tools.Result{Output: "objective_id is required", IsError: true}
	}
	if reason == "" {
		return tools.Result{Output: "reason is required", IsError: true}
	}

	// Verify the task belongs to this quest
	task, err := h.db.GetTaskByID(objectiveID)
	if err != nil {
		return tools.Result{Output: fmt.Sprintf("Failed to get objective: %v", err), IsError: true}
	}
	if task == nil {
		return tools.Result{Output: fmt.Sprintf("Objective not found: %s", objectiveID), IsError: true}
	}
	if task.QuestID.Valid && task.QuestID.String != questID {
		return tools.Result{Output: "Objective does not belong to this quest", IsError: true}
	}

	// Check if already cancelled or completed
	if task.Status == db.TaskStatusCancelled {
		return tools.Result{Output: "Objective is already cancelled"}
	}
	if task.Status == db.TaskStatusCompleted {
		return tools.Result{Output: "Cannot cancel a completed objective", IsError: true}
	}

	// Cancel the task
	if err := h.db.UpdateTaskStatus(objectiveID, db.TaskStatusCancelled); err != nil {
		return tools.Result{Output: fmt.Sprintf("Failed to cancel objective: %v", err), IsError: true}
	}

	// Broadcast cancellation
	if h.hub != nil {
		h.hub.Broadcast(websocket.Message{
			Type: "task.cancelled",
			Payload: map[string]any{
				"task_id":  objectiveID,
				"quest_id": questID,
				"reason":   reason,
			},
		})
	}

	return tools.Result{Output: fmt.Sprintf("Objective '%s' has been cancelled. Reason: %s\n\nYou can now propose a new objective to replace it if needed.", task.Title, reason)}
}
