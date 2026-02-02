// Package session provides session lifecycle management for Poindexter
package session

import (
	"fmt"
	"strings"

	"github.com/lirancohen/dex/internal/toolbelt"
)

// Context window management constants
const (
	DefaultContextWindowMax  = 200000 // Claude's context window
	DefaultContextWarnPct    = 80     // Warn at 80%
	DefaultContextCompactPct = 90     // Compact at 90%
	MaxRecentMessages        = 10     // Messages to keep after compaction
	CharsPerToken            = 4      // Approximate chars per token
)

// RemovalLevels defines progressive percentages of tool responses to remove
// Inspired by Goose's context_mgmt system
var RemovalLevels = []int{0, 10, 20, 50, 100}

// ContextGuard monitors context window usage and triggers compaction
type ContextGuard struct {
	windowMax  int // Model limit (200000 for Claude)
	warnAt     int // Warning threshold (80%)
	compactAt  int // Compaction threshold (90%)
	activity   *ActivityRecorder
}

// NewContextGuard creates a new context guard with default thresholds
func NewContextGuard(activity *ActivityRecorder) *ContextGuard {
	return &ContextGuard{
		windowMax: DefaultContextWindowMax,
		warnAt:    DefaultContextWindowMax * DefaultContextWarnPct / 100,
		compactAt: DefaultContextWindowMax * DefaultContextCompactPct / 100,
		activity:  activity,
	}
}

// SetThresholds configures custom thresholds
func (g *ContextGuard) SetThresholds(windowMax, warnPct, compactPct int) {
	g.windowMax = windowMax
	g.warnAt = windowMax * warnPct / 100
	g.compactAt = windowMax * compactPct / 100
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
	targetTokens := g.compactAt * 80 / 100 // Target 80% of compact threshold

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
			return filtered, nil
		}
	}

	// All tool responses removed but still over limit - fall back to keeping recent messages
	return g.keepRecentWithSummary(messages, scratchpad), nil
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
	summary := summarizeMessages(oldMessages)

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
