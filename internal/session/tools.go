// Package session provides session lifecycle management for Poindexter
package session

import (
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// GetToolDefinitionsForHat returns tools appropriate for a specific hat
// Uses the tool profile system to provide role-appropriate tools
func GetToolDefinitionsForHat(hat string) []toolbelt.AnthropicTool {
	toolSet := tools.GetToolsForHat(hat)
	return toolSetToAnthropic(toolSet)
}

// toolSetToAnthropic converts a tools.Set to Anthropic tool format
func toolSetToAnthropic(toolSet *tools.Set) []toolbelt.AnthropicTool {
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
