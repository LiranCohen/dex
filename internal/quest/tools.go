// Package quest provides Quest conversation handling for Poindexter
package quest

import (
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// Quest-specific tools that need database access
// These are handled directly in the quest handler, not through the generic executor

// =============================================================================
// Conversation Tools (blocking/interactive)
// =============================================================================

// AskQuestionToolDef returns the Anthropic tool definition for ask_question
func AskQuestionToolDef() toolbelt.AnthropicTool {
	t := tools.AskQuestionTool()
	return toolbelt.AnthropicTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// ProposeObjectiveToolDef returns the Anthropic tool definition for propose_objective
func ProposeObjectiveToolDef() toolbelt.AnthropicTool {
	t := tools.ProposeObjectiveTool()
	return toolbelt.AnthropicTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// CompleteQuestToolDef returns the Anthropic tool definition for complete_quest
func CompleteQuestToolDef() toolbelt.AnthropicTool {
	t := tools.CompleteQuestTool()
	return toolbelt.AnthropicTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// =============================================================================
// Objective Management Tools (database access)
// =============================================================================

// ListObjectivesTool returns a tool definition for listing quest objectives
func ListObjectivesTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "list_objectives",
		Description: "List all objectives (tasks) for the current quest with their current status and progress. Use this to understand what work has been done, what's in progress, and what might need attention.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}
}

// GetObjectiveDetailsTool returns a tool definition for getting objective details
func GetObjectiveDetailsTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "get_objective_details",
		Description: "Get detailed information about a specific objective including checklist progress, session history, and any error messages. Use this when you need to understand why an objective failed or what progress was made.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"objective_id": map[string]any{
					"type":        "string",
					"description": "The ID of the objective to get details for",
				},
			},
			"required": []string{"objective_id"},
		},
	}
}

// CancelObjectiveTool returns a tool definition for cancelling an objective
func CancelObjectiveTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "cancel_objective",
		Description: "Cancel a stuck or failed objective. Use this when an objective cannot be completed and needs to be recreated with a different approach. The user will be informed and you can propose a new objective to replace it.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"objective_id": map[string]any{
					"type":        "string",
					"description": "The ID of the objective to cancel",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Brief explanation of why the objective is being cancelled",
				},
			},
			"required": []string{"objective_id", "reason"},
		},
	}
}

// QuestTools returns all quest-specific tools
func QuestTools() []toolbelt.AnthropicTool {
	return []toolbelt.AnthropicTool{
		// Conversation tools
		AskQuestionToolDef(),
		ProposeObjectiveToolDef(),
		CompleteQuestToolDef(),
		// Objective management tools
		ListObjectivesTool(),
		GetObjectiveDetailsTool(),
		CancelObjectiveTool(),
	}
}

// IsQuestTool returns true if the tool name is a quest-specific tool
func IsQuestTool(name string) bool {
	switch name {
	case "ask_question", "propose_objective", "complete_quest",
		"list_objectives", "get_objective_details", "cancel_objective":
		return true
	default:
		return false
	}
}

// IsBlockingQuestTool returns true if the tool blocks for user input
func IsBlockingQuestTool(name string) bool {
	return name == "ask_question"
}
