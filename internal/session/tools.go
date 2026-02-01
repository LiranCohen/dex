// Package session provides session lifecycle management for Poindexter
package session

import (
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// GetToolDefinitions returns all available tools for the Ralph loop
// Uses the shared tools package with read-write tools for objective execution
func GetToolDefinitions() []toolbelt.AnthropicTool {
	toolSet := tools.ReadWriteTools()
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

