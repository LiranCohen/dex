package tools

// ReadOnly tools - safe for Quest chat (research/planning phase)

func ReadFileTool() Tool {
	return Tool{
		Name:        "read_file",
		Description: "Read the contents of a file. Path is relative to the project root.",
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
		ReadOnly: true,
	}
}

func ListFilesTool() Tool {
	return Tool{
		Name:        "list_files",
		Description: "List files and directories. Path is relative to the project root. Use empty path or '.' for root.",
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
		ReadOnly: true,
	}
}

func GlobTool() Tool {
	return Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Returns paths relative to project root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to match (e.g., '**/*.go', 'src/**/*.ts')",
				},
			},
			"required": []string{"pattern"},
		},
		ReadOnly: true,
	}
}

func GrepTool() Tool {
	return Tool{
		Name:        "grep",
		Description: "Search for a pattern in files. Returns matching lines with file paths and line numbers.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Regular expression pattern to search for",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory or file to search in (default: project root)",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "Glob pattern to filter files (e.g., '*.go', '*.ts')",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default: 100)",
				},
			},
			"required": []string{"pattern"},
		},
		ReadOnly: true,
	}
}

func GitStatusTool() Tool {
	return Tool{
		Name:        "git_status",
		Description: "Get git status showing modified, staged, and untracked files.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		ReadOnly: true,
	}
}

func GitDiffTool() Tool {
	return Tool{
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
		ReadOnly: true,
	}
}

func GitLogTool() Tool {
	return Tool{
		Name:        "git_log",
		Description: "Show git commit history.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_count": map[string]any{
					"type":        "integer",
					"description": "Maximum number of commits to show (default: 20)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Show commits affecting this path",
				},
				"oneline": map[string]any{
					"type":        "boolean",
					"description": "Use compact one-line format (default: true)",
				},
			},
			"required": []string{},
		},
		ReadOnly: true,
	}
}

func WebSearchTool() Tool {
	return Tool{
		Name:        "web_search",
		Description: "Search the web for information. Useful for finding documentation, examples, or up-to-date information.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (default: 10)",
				},
			},
			"required": []string{"query"},
		},
		ReadOnly: true,
	}
}

func WebFetchTool() Tool {
	return Tool{
		Name:        "web_fetch",
		Description: "Fetch the content of a web page. Useful for reading documentation, API references, or code examples.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL to fetch",
				},
			},
			"required": []string{"url"},
		},
		ReadOnly: true,
	}
}

// ReadWrite tools - only for objective execution (RalphLoop)

func BashTool() Tool {
	return Tool{
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
		ReadOnly: false,
	}
}

func WriteFileTool() Tool {
	return Tool{
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
		ReadOnly: false,
	}
}

func GitInitTool() Tool {
	return Tool{
		Name:        "git_init",
		Description: "Initialize a new git repository in the working directory. Use this when starting a new project that doesn't have git yet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"default_branch": map[string]any{
					"type":        "string",
					"description": "Name for the initial branch (default: main)",
				},
			},
			"required": []string{},
		},
		ReadOnly: false,
	}
}

func GitCommitTool() Tool {
	return Tool{
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
		ReadOnly: false,
	}
}

func GitRemoteAddTool() Tool {
	return Tool{
		Name:        "git_remote_add",
		Description: "Add a remote repository. Use this after creating a GitHub repo to connect your local repo to it.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Remote name (default: origin)",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "Remote repository URL (e.g., https://github.com/owner/repo.git)",
				},
			},
			"required": []string{"url"},
		},
		ReadOnly: false,
	}
}

func GitPushTool() Tool {
	return Tool{
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
		ReadOnly: false,
	}
}

func GitHubCreateRepoTool() Tool {
	return Tool{
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
		ReadOnly: false,
	}
}

func GitHubCreatePRTool() Tool {
	return Tool{
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
		ReadOnly: false,
	}
}
