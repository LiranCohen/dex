// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/lirancohen/dex/internal/toolbelt"
)

// Context window management constants
const (
	DefaultContextWindowMax  = 200000 // Claude's context window
	DefaultContextWarnPct    = 40     // Warn at 40% (earlier warning to monitor growth)
	DefaultContextCompactPct = 50     // Compact at 50% (leaves 50% buffer for responses)
	MaxRecentMessages        = 6      // Messages to keep after compaction
	CharsPerToken            = 4      // Approximate chars per token

	// Summarization model options
	SummaryModelHaiku  = "claude-haiku-4-5-20251001"  // Default: fast and cheap
	SummaryModelSonnet = "claude-sonnet-4-5-20250929" // Higher quality but more expensive
	SummaryModelSame   = ""                           // Use same model as main conversation

	// Threshold for using higher-quality summarization
	AggressiveRemovalThreshold = 50 // Use Sonnet when removing >50% of tool responses
)

// RemovalLevels defines progressive percentages of tool responses to remove
// Inspired by Goose's context_mgmt system
// More aggressive levels to prevent token bloat (was 0, 10, 20, 50, 100)
var RemovalLevels = []int{30, 50, 70, 100}

// ContextGuard monitors context window usage and triggers compaction
type ContextGuard struct {
	windowMax      int // Model limit (200000 for Claude)
	warnAt         int // Warning threshold (50%)
	compactAt      int // Compaction threshold (60%)
	activity       *ActivityRecorder
	client         *toolbelt.AnthropicClient // For LLM-based summarization
	promptLoader   *PromptLoader             // For loading summarization prompt
	summaryModel   string                    // Model to use for summarization (default: Haiku)
	lastUsagePct   int                       // Last calculated usage percentage for UI
}

// NewContextGuard creates a new context guard with default thresholds
func NewContextGuard(activity *ActivityRecorder) *ContextGuard {
	return &ContextGuard{
		windowMax:    DefaultContextWindowMax,
		warnAt:       DefaultContextWindowMax * DefaultContextWarnPct / 100,
		compactAt:    DefaultContextWindowMax * DefaultContextCompactPct / 100,
		activity:     activity,
		summaryModel: SummaryModelHaiku, // Default to Haiku for cost efficiency
	}
}

// SetThresholds configures custom thresholds
func (g *ContextGuard) SetThresholds(windowMax, warnPct, compactPct int) {
	g.windowMax = windowMax
	g.warnAt = windowMax * warnPct / 100
	g.compactAt = windowMax * compactPct / 100
}

// SetSummarizer configures LLM-based summarization
// If client is nil, falls back to rule-based summarization
// Model can be SummaryModelHaiku (default), SummaryModelSonnet, or SummaryModelSame
func (g *ContextGuard) SetSummarizer(client *toolbelt.AnthropicClient, promptLoader *PromptLoader, model string) {
	g.client = client
	g.promptLoader = promptLoader
	if model != "" {
		g.summaryModel = model
	}
}

// WindowMax returns the maximum context window size
func (g *ContextGuard) WindowMax() int {
	return g.windowMax
}

// UsagePercent returns the last calculated context usage percentage (0-100)
func (g *ContextGuard) UsagePercent() int {
	return g.lastUsagePct
}

// ContextStatus returns current context usage stats for UI display
type ContextStatus struct {
	UsedTokens   int    `json:"used_tokens"`
	MaxTokens    int    `json:"max_tokens"`
	UsagePercent int    `json:"usage_percent"`
	Status       string `json:"status"` // "ok", "warning", "critical"
}

// GetStatus returns current context status for UI
func (g *ContextGuard) GetStatus(messages []toolbelt.AnthropicMessage, systemPrompt string) ContextStatus {
	tokens := EstimateTokens(messages, systemPrompt)
	pct := tokens * 100 / g.windowMax
	g.lastUsagePct = pct

	status := "ok"
	if pct >= 85 {
		status = "critical"
	} else if pct >= 50 {
		status = "warning"
	}

	return ContextStatus{
		UsedTokens:   tokens,
		MaxTokens:    g.windowMax,
		UsagePercent: pct,
		Status:       status,
	}
}

// EstimateTokens estimates the token count for a message list
// Uses ~4 chars per token as approximation
func EstimateTokens(messages []toolbelt.AnthropicMessage, systemPrompt string) int {
	total := 0

	// System prompt
	total += len(systemPrompt) / CharsPerToken

	// All messages
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}

	return total
}

// estimateMessageTokens estimates tokens for a single message
func estimateMessageTokens(msg toolbelt.AnthropicMessage) int {
	total := 0

	// Handle Content based on type
	switch c := msg.Content.(type) {
	case string:
		total += len(c) / CharsPerToken
	case []any:
		// Array of content blocks
		for _, block := range c {
			if blockMap, ok := block.(map[string]any); ok {
				total += estimateBlockTokens(blockMap)
			}
		}
	case []toolbelt.ContentBlock:
		for _, block := range c {
			if block.Text != "" {
				total += len(block.Text) / CharsPerToken
			}
			if block.Content != "" {
				total += len(block.Content) / CharsPerToken
			}
			// Tool calls add overhead
			if block.Type == "tool_use" || block.Type == "tool_result" {
				total += 50 // Structure overhead
			}
		}
	}

	return total
}

// estimateBlockTokens estimates tokens for a content block map
func estimateBlockTokens(block map[string]any) int {
	total := 0

	if text, ok := block["text"].(string); ok {
		total += len(text) / CharsPerToken
	}
	if content, ok := block["content"].(string); ok {
		total += len(content) / CharsPerToken
	}
	if input, ok := block["input"].(map[string]any); ok {
		// Estimate JSON input size
		for k, v := range input {
			total += len(k) / CharsPerToken
			if s, ok := v.(string); ok {
				total += len(s) / CharsPerToken
			} else {
				total += 10 // Approximate for non-string values
			}
		}
	}

	// Tool calls add overhead
	if blockType, ok := block["type"].(string); ok {
		if blockType == "tool_use" || blockType == "tool_result" {
			total += 50
		}
	}

	return total
}

// CheckAndCompact checks context usage and compacts if needed
// Returns the compacted messages and whether compaction occurred
func (g *ContextGuard) CheckAndCompact(messages []toolbelt.AnthropicMessage, systemPrompt string, scratchpad string) ([]toolbelt.AnthropicMessage, bool, error) {
	tokens := EstimateTokens(messages, systemPrompt)

	if tokens >= g.compactAt {
		if g.activity != nil {
			g.activity.Debug(0, fmt.Sprintf("context at %d%%, triggering compaction", tokens*100/g.windowMax))
		}
		compacted, err := g.compactProgressive(messages, scratchpad)
		if err != nil {
			return messages, false, err
		}
		return compacted, true, nil
	} else if tokens >= g.warnAt {
		if g.activity != nil {
			g.activity.Debug(0, fmt.Sprintf("context at %d%%, approaching limit", tokens*100/g.windowMax))
		}
	}

	return messages, false, nil
}

// compactProgressive tries progressive tool response removal before full compaction
func (g *ContextGuard) compactProgressive(messages []toolbelt.AnthropicMessage, scratchpad string) ([]toolbelt.AnthropicMessage, error) {
	targetTokens := g.windowMax * 35 / 100 // Target 35% of context window (leaves 65% for responses)

	for _, pct := range RemovalLevels {
		filtered := filterToolResponses(messages, pct)
		tokens := EstimateTokens(filtered, "")

		if tokens < targetTokens {
			if g.activity != nil {
				originalTokens := EstimateTokens(messages, "")
				g.activity.Debug(0, fmt.Sprintf(
					"compaction: removed %d%% of tool responses, %d -> %d tokens",
					pct, originalTokens, tokens))
			}

			// For aggressive removal (>50%), ensure we have a good summary first
			// This preserves important context that would otherwise be lost
			if pct >= AggressiveRemovalThreshold && g.client != nil && g.promptLoader != nil {
				// Get the messages that will be removed
				removedMessages := getRemovedMessages(messages, filtered)
				if len(removedMessages) > 0 {
					// Use Sonnet for better quality summary when removing lots of context
					originalModel := g.summaryModel
					g.summaryModel = SummaryModelSonnet

					summary, err := g.summarizeWithLLM(removedMessages)
					g.summaryModel = originalModel

					if err == nil && summary != "" {
						// Prepend summary context to filtered messages
						summaryMsg := toolbelt.AnthropicMessage{
							Role:    "user",
							Content: fmt.Sprintf("## Compacted Context Summary\n\n%s\n\nContinue with the task.", summary),
						}
						filtered = append([]toolbelt.AnthropicMessage{summaryMsg}, filtered...)

						if g.activity != nil {
							g.activity.Debug(0, fmt.Sprintf(
								"compaction: added Sonnet-quality summary for %d removed messages",
								len(removedMessages)))
						}
					}
				}
			}

			return filtered, nil
		}
	}

	// All tool responses removed but still over limit - fall back to keeping recent messages
	return g.keepRecentWithSummary(messages, scratchpad), nil
}

// getRemovedMessages returns messages that were filtered out
func getRemovedMessages(original, filtered []toolbelt.AnthropicMessage) []toolbelt.AnthropicMessage {
	// Build a set of filtered message contents for quick lookup
	filteredSet := make(map[string]bool)
	for _, msg := range filtered {
		key := fmt.Sprintf("%s:%v", msg.Role, msg.Content)
		filteredSet[key] = true
	}

	// Find messages that were removed
	var removed []toolbelt.AnthropicMessage
	for _, msg := range original {
		key := fmt.Sprintf("%s:%v", msg.Role, msg.Content)
		if !filteredSet[key] {
			removed = append(removed, msg)
		}
	}
	return removed
}

// filterToolResponses removes a percentage of tool responses from the middle outward
// Middle-out removal preserves recent context and initial task understanding
func filterToolResponses(messages []toolbelt.AnthropicMessage, removePercent int) []toolbelt.AnthropicMessage {
	if removePercent == 0 {
		return messages
	}

	// Find indices of tool response messages
	toolIndices := []int{}
	for i, msg := range messages {
		if msg.Role == "user" && hasToolResponse(msg) {
			toolIndices = append(toolIndices, i)
		}
	}

	if len(toolIndices) == 0 {
		return messages
	}

	// Calculate how many to remove
	numToRemove := (len(toolIndices) * removePercent) / 100
	if numToRemove == 0 && removePercent > 0 {
		numToRemove = 1
	}

	// Remove from middle outward
	middle := len(toolIndices) / 2
	toRemove := make(map[int]bool)

	for i := 0; i < numToRemove; i++ {
		if i%2 == 0 {
			offset := i / 2
			if middle-offset-1 >= 0 {
				toRemove[toolIndices[middle-offset-1]] = true
			}
		} else {
			offset := i / 2
			if middle+offset < len(toolIndices) {
				toRemove[toolIndices[middle+offset]] = true
			}
		}
	}

	// Filter out removed messages
	result := make([]toolbelt.AnthropicMessage, 0, len(messages)-numToRemove)
	for i, msg := range messages {
		if !toRemove[i] {
			result = append(result, msg)
		}
	}

	return result
}

// hasToolResponse checks if a message contains tool responses
func hasToolResponse(msg toolbelt.AnthropicMessage) bool {
	switch c := msg.Content.(type) {
	case []any:
		for _, block := range c {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_result" {
					return true
				}
			}
		}
	case []toolbelt.ContentBlock:
		for _, block := range c {
			if block.Type == "tool_result" {
				return true
			}
		}
	}
	return false
}

// keepRecentWithSummary keeps only recent messages and adds a summary context message
func (g *ContextGuard) keepRecentWithSummary(messages []toolbelt.AnthropicMessage, scratchpad string) []toolbelt.AnthropicMessage {
	if len(messages) <= MaxRecentMessages {
		return messages
	}

	// Keep recent messages
	recentMessages := messages[len(messages)-MaxRecentMessages:]

	// Create summary of compacted history
	oldMessages := messages[:len(messages)-MaxRecentMessages]

	// Try LLM-based summarization if client is available
	var summary string
	if g.client != nil && g.promptLoader != nil {
		llmSummary, err := g.summarizeWithLLM(oldMessages)
		if err != nil {
			if g.activity != nil {
				g.activity.Debug(0, fmt.Sprintf("LLM summarization failed, falling back to rule-based: %v", err))
			}
			summary = summarizeMessages(oldMessages)
		} else {
			summary = llmSummary
			if g.activity != nil {
				g.activity.Debug(0, fmt.Sprintf("LLM summarization complete (%d chars)", len(summary)))
			}
		}
	} else {
		summary = summarizeMessages(oldMessages)
	}

	// Build context message
	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Session Context (compacted)\n\n")

	if scratchpad != "" {
		contextBuilder.WriteString("### Scratchpad\n")
		contextBuilder.WriteString(scratchpad)
		contextBuilder.WriteString("\n\n")
	}

	if summary != "" {
		contextBuilder.WriteString("### Compacted History\n")
		contextBuilder.WriteString(summary)
		contextBuilder.WriteString("\n\n")
	}

	contextBuilder.WriteString("Continue working on the task. Use your scratchpad to track progress.\n")

	// Prepend context message
	result := make([]toolbelt.AnthropicMessage, 0, len(recentMessages)+1)
	result = append(result, toolbelt.AnthropicMessage{
		Role:    "user",
		Content: contextBuilder.String(),
	})
	result = append(result, recentMessages...)

	return result
}

// summarizeMessages extracts key events from messages
func summarizeMessages(messages []toolbelt.AnthropicMessage) string {
	var summary strings.Builder

	for _, msg := range messages {
		var content string
		switch c := msg.Content.(type) {
		case string:
			content = c
		}

		if msg.Role == "assistant" && content != "" {
			// Look for tool calls
			if strings.Contains(content, "tool_use") {
				summary.WriteString("- Tool calls made\n")
			}
			// Look for decisions
			if strings.Contains(content, "decided") || strings.Contains(content, "choosing") {
				firstSentence := extractFirstSentence(content)
				if firstSentence != "" {
					summary.WriteString(fmt.Sprintf("- Decision: %s\n", firstSentence))
				}
			}
		}
		if msg.Role == "user" && content != "" {
			// Look for quality gate results
			if strings.Contains(content, "QUALITY_") {
				result := extractQualityResult(content)
				if result != "" {
					summary.WriteString(fmt.Sprintf("- Quality gate: %s\n", result))
				}
			}
		}
	}

	return summary.String()
}

// extractFirstSentence gets the first sentence from text
func extractFirstSentence(text string) string {
	// Find first period, question mark, or exclamation
	for i, c := range text {
		if c == '.' || c == '?' || c == '!' {
			if i > 0 && i < 200 {
				return strings.TrimSpace(text[:i+1])
			}
		}
	}
	// No sentence ending found, truncate
	if len(text) > 100 {
		return text[:100] + "..."
	}
	return text
}

// extractQualityResult extracts the quality result type
func extractQualityResult(content string) string {
	if strings.Contains(content, "QUALITY_PASSED") {
		return "passed"
	}
	if strings.Contains(content, "QUALITY_BLOCKED") {
		return "blocked"
	}
	return ""
}

// summarizeWithLLM uses Claude (Haiku by default) to create an intelligent summary
func (g *ContextGuard) summarizeWithLLM(messages []toolbelt.AnthropicMessage) (string, error) {
	if g.client == nil {
		return "", fmt.Errorf("no anthropic client configured")
	}
	if g.promptLoader == nil {
		return "", fmt.Errorf("no prompt loader configured")
	}

	// Build conversation text for summarization
	var conversationText strings.Builder
	var hasErrors bool
	for _, msg := range messages {
		var content string
		switch c := msg.Content.(type) {
		case string:
			content = c
		case []toolbelt.ContentBlock:
			for _, block := range c {
				if block.Text != "" {
					content += block.Text + "\n"
				}
				if block.Type == "tool_use" {
					content += fmt.Sprintf("[Tool: %s]\n", block.Name)
				}
				if block.Type == "tool_result" {
					// Truncate long tool results
					result := block.Content
					if len(result) > 500 {
						result = result[:500] + "..."
					}
					content += fmt.Sprintf("[Result: %s]\n", result)
					if block.IsError {
						hasErrors = true
					}
				}
			}
		}

		if content != "" {
			conversationText.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, content))
		}
	}

	// Limit input to avoid huge summarization requests
	inputText := conversationText.String()
	if len(inputText) > 50000 {
		inputText = inputText[:50000] + "\n...[truncated]"
	}

	// Get the summarizer prompt from PromptLoom
	prompt, err := g.promptLoader.GetSummarizerPrompt(inputText, hasErrors)
	if err != nil {
		return "", fmt.Errorf("failed to get summarizer prompt: %w", err)
	}

	model := g.summaryModel
	if model == "" || model == SummaryModelSame {
		model = SummaryModelHaiku // Default to Haiku if not specified
	}

	req := &toolbelt.AnthropicChatRequest{
		Model:     model,
		MaxTokens: 1024,
		Messages: []toolbelt.AnthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	ctx := context.Background()
	resp, err := g.client.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarization API call failed: %w", err)
	}

	return resp.Text(), nil
}
