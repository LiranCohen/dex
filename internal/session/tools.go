// Package session provides session lifecycle management for Poindexter
package session

import "github.com/lirancohen/dex/internal/toolbelt"

// GetToolDefinitions returns all available tools for the Ralph loop
func GetToolDefinitions() []toolbelt.AnthropicTool {
	return []toolbelt.AnthropicTool{
		bashTool(),
		readFileTool(),
		writeFileTool(),
		listFilesTool(),
		gitStatusTool(),
		gitDiffTool(),
		gitCommitTool(),
		gitPushTool(),
		githubCreateRepoTool(),
		githubCreatePRTool(),
	}
}

func bashTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "bash",
		Description: "Execute a bash command in the worktree directory. Use for running builds, tests, scripts, and general shell operations. Commands run with the worktree as working directory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds (default: 300, max: 300)",
				},
			},
			"required": []string{"command"},
		},
	}
}

func readFileTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "read_file",
		Description: "Read the contents of a file. Path is relative to the worktree root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative path to the file to read",
				},
			},
			"required": []string{"path"},
		},
	}
}

func writeFileTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Path is relative to the worktree root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative path to the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

func listFilesTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "list_files",
		Description: "List files and directories. Path is relative to the worktree root. Use empty path or '.' for root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative path to the directory to list (default: root)",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "If true, list files recursively (default: false)",
				},
			},
			"required": []string{},
		},
	}
}

func gitStatusTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "git_status",
		Description: "Get git status showing modified, staged, and untracked files.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}
}

func gitDiffTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "git_diff",
		Description: "Show git diff of changes. Can show staged, unstaged, or changes against a base branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{
					"type":        "boolean",
					"description": "If true, show only staged changes",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Limit diff to a specific path",
				},
			},
			"required": []string{},
		},
	}
}

func gitCommitTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "git_commit",
		Description: "Stage files and create a git commit. Use git_status first to see what needs to be committed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Commit message",
				},
				"files": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Files to stage before committing. Use ['.'] to stage all changes.",
				},
			},
			"required": []string{"message"},
		},
	}
}

func gitPushTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "git_push",
		Description: "Push commits to the remote repository.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"set_upstream": map[string]any{
					"type":        "boolean",
					"description": "Set upstream tracking (use for new branches)",
				},
			},
			"required": []string{},
		},
	}
}

func githubCreateRepoTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "github_create_repo",
		Description: "Create a new GitHub repository.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Repository name",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Repository description",
				},
				"private": map[string]any{
					"type":        "boolean",
					"description": "If true, create a private repository (default: false)",
				},
			},
			"required": []string{"name"},
		},
	}
}

func githubCreatePRTool() toolbelt.AnthropicTool {
	return toolbelt.AnthropicTool{
		Name:        "github_create_pr",
		Description: "Create a pull request on GitHub. Requires the branch to be pushed first.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Pull request title",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Pull request description",
				},
				"base": map[string]any{
					"type":        "string",
					"description": "Base branch to merge into (default: main)",
				},
				"draft": map[string]any{
					"type":        "boolean",
					"description": "If true, create as draft PR (default: false)",
				},
			},
			"required": []string{"title"},
		},
	}
}
