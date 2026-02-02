// Package session provides session lifecycle management for Poindexter
package session

import (
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// GetToolDefinitions returns all available tools for the Ralph loop
// Uses the shared tools package with read-write tools for objective execution
// Deprecated: Use GetToolDefinitionsForHat instead for hat-specific tool sets
func GetToolDefinitions() []toolbelt.AnthropicTool {
	toolSet := tools.ReadWriteTools()
	return toolSetToAnthropic(toolSet)
}

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
