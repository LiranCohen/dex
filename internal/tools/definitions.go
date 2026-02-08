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

func ListRuntimesTool() Tool {
	return Tool{
		Name:        "list_runtimes",
		Description: "List available programming language runtimes and tools installed on the system. Use this to discover what languages and package managers are available before running build commands.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
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
					"description": "Repository name. Can be just 'repo-name' (uses project owner) or 'owner/repo-name' for a different owner.",
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

// Quality Gate Tools

func RunTestsTool() Tool {
	return Tool{
		Name:        "run_tests",
		Description: "Run the project's test suite. Auto-detects the test command based on project type (go test, npm test, cargo test, pytest, etc.). Returns test output and pass/fail status.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds (default: 300, max: 600)",
				},
				"verbose": map[string]any{
					"type":        "boolean",
					"description": "If true, run tests in verbose mode (default: false)",
				},
			},
			"required": []string{},
		},
		ReadOnly: false,
	}
}

func RunLintTool() Tool {
	return Tool{
		Name:        "run_lint",
		Description: "Run the project's linter. Auto-detects the lint command based on project type (go vet, golangci-lint, eslint, cargo clippy, ruff, etc.). Returns lint issues found.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"fix": map[string]any{
					"type":        "boolean",
					"description": "If true, attempt to auto-fix lint issues (default: false)",
				},
			},
			"required": []string{},
		},
		ReadOnly: false,
	}
}

func RunBuildTool() Tool {
	return Tool{
		Name:        "run_build",
		Description: "Run the project's build command. Auto-detects based on project type (go build, npm run build, cargo build, etc.). Returns build output and success/failure.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds (default: 300, max: 600)",
				},
			},
			"required": []string{},
		},
		ReadOnly: false,
	}
}

func TaskCompleteTool() Tool {
	return Tool{
		Name:        "task_complete",
		Description: "Signal that the task is complete and trigger quality gate validation. Runs tests, lint, and build checks automatically. Returns QUALITY_PASSED if all checks pass, or QUALITY_BLOCKED with specific feedback if any check fails. Use skip flags to bypass specific checks when appropriate.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "Brief summary of what was accomplished",
				},
				"skip_tests": map[string]any{
					"type":        "boolean",
					"description": "Skip test validation (use when no tests exist or tests are not applicable)",
				},
				"skip_lint": map[string]any{
					"type":        "boolean",
					"description": "Skip lint validation (use when no linter configured)",
				},
				"skip_build": map[string]any{
					"type":        "boolean",
					"description": "Skip build validation (use when no build step or not applicable)",
				},
			},
			"required": []string{"summary"},
		},
		ReadOnly: false,
	}
}

// =============================================================================
// Quest Tools - for Quest conversation phase
// =============================================================================

// AskQuestionTool returns the tool definition for asking clarifying questions
func AskQuestionTool() Tool {
	return Tool{
		Name: "ask_question",
		Description: `Ask the user a clarifying question. Execution pauses until the user responds.

Usage notes:
- When options are provided, user can select one (or multiple if allow_multiple=true)
- If allow_custom=true (default), an "Other" option lets users type a custom answer
- Put the recommended option first and set recommended_index=0
- Returns: {"answer": "selected label or custom text", "selected_indices": [0], "is_custom": false}`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The question to ask the user",
				},
				"header": map[string]any{
					"type":        "string",
					"description": "Short label for the question (max 30 chars)",
				},
				"options": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"label": map[string]any{
								"type":        "string",
								"description": "Display text (1-5 words, concise)",
							},
							"description": map[string]any{
								"type":        "string",
								"description": "Explanation of choice",
							},
						},
						"required": []string{"label"},
					},
					"description": "Optional list of choices for the user",
				},
				"allow_multiple": map[string]any{
					"type":        "boolean",
					"description": "Allow selecting multiple options (default: false)",
				},
				"allow_custom": map[string]any{
					"type":        "boolean",
					"description": "Show 'Other' option for custom answer (default: true)",
				},
				"recommended_index": map[string]any{
					"type":        "integer",
					"description": "Index of recommended option (0-based), adds '(Recommended)' label",
				},
			},
			"required": []string{"question"},
		},
		ReadOnly: true, // No filesystem changes
	}
}

// ProposeObjectiveTool returns the tool definition for proposing objectives
func ProposeObjectiveTool() Tool {
	return Tool{
		Name: "propose_objective",
		Description: `Propose a task/objective for user approval. Non-blocking - creates a pending draft.

Returns: {"draft_id": "uuid", "status": "pending"}
The user accepts/rejects via UI, which triggers objective creation.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Action-oriented title (e.g., 'Add user authentication')",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed description of what this objective accomplishes",
				},
				"hat": map[string]any{
					"type":        "string",
					"enum":        []string{"explorer", "planner", "designer", "creator"},
					"description": "Starting hat for this objective",
				},
				"checklist_must_have": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Required checklist items (3-5 items, outcome-focused)",
				},
				"checklist_optional": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional nice-to-have items",
				},
				"blocked_by": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Draft IDs this objective depends on",
				},
				"auto_start": map[string]any{
					"type":        "boolean",
					"description": "Start immediately when accepted (default: true)",
				},
				"complexity": map[string]any{
					"type":        "string",
					"enum":        []string{"simple", "complex"},
					"description": "simple=Sonnet model, complex=Opus model",
				},
				"estimated_iterations": map[string]any{
					"type":        "integer",
					"description": "Estimated number of iterations needed",
				},
				"estimated_budget": map[string]any{
					"type":        "number",
					"description": "Estimated cost in USD",
				},
				"git_provider": map[string]any{
					"type":        "string",
					"enum":        []string{"github", "forgejo"},
					"description": "Git provider (default: github)",
				},
				"git_owner": map[string]any{
					"type":        "string",
					"description": "Owner/org for the repository",
				},
				"git_repo": map[string]any{
					"type":        "string",
					"description": "Repository name",
				},
				"clone_url": map[string]any{
					"type":        "string",
					"description": "Upstream URL to fork from (null for new repos)",
				},
			},
			"required": []string{"title", "hat", "checklist_must_have"},
		},
		ReadOnly: true,
	}
}

// CompleteQuestTool returns the tool definition for completing a quest
func CompleteQuestTool() Tool {
	return Tool{
		Name:        "complete_quest",
		Description: "Signal that all objectives have been proposed and quest planning is complete.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "Brief summary of what the proposed objectives will accomplish",
				},
			},
			"required": []string{"summary"},
		},
		ReadOnly: true,
	}
}

// =============================================================================
// Workflow Tools - for task execution phase (shared between HQ and workers)
// =============================================================================

// MarkChecklistItemTool returns the tool definition for marking checklist items
func MarkChecklistItemTool() Tool {
	return Tool{
		Name: "mark_checklist_item",
		Description: `Update the status of a checklist item. Call IMMEDIATELY after completing each item's work.

Correct pattern (interleaved):
  [do work for item 1]
  mark_checklist_item(item_id="citm-123", status="done")
  [do work for item 2]
  mark_checklist_item(item_id="citm-456", status="done")

Wrong pattern (batched at end):
  [do all work]
  mark_checklist_item(...)  // defeats real-time progress tracking`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{
					"type":        "string",
					"description": "The checklist item ID (e.g., 'citm-abc123')",
				},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"done", "failed", "skipped"},
					"description": "New status for the item",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Reason for failure or skip (required if status is 'failed' or 'skipped')",
				},
			},
			"required": []string{"item_id", "status"},
		},
		ReadOnly: false,
	}
}

// SignalEventTool returns the tool definition for signaling workflow events
func SignalEventTool() Tool {
	return Tool{
		Name: "signal_event",
		Description: `Signal a workflow state transition. This triggers hat changes and workflow progression.

Event routing:
- plan.complete → designer
- design.complete → creator
- implementation.done → critic
- review.approved → editor
- review.rejected → creator
- task.blocked → resolver
- resolved → creator
- task.complete → (terminal)`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"event": map[string]any{
					"type": "string",
					"enum": []string{
						"plan.complete",
						"design.complete",
						"implementation.done",
						"review.approved",
						"review.rejected",
						"task.blocked",
						"resolved",
						"task.complete",
					},
					"description": "The event to signal",
				},
				"payload": map[string]any{
					"type":        "object",
					"description": "Optional event payload (e.g., {\"reason\": \"...\"} for task.blocked)",
				},
				"acknowledge_failures": map[string]any{
					"type":        "boolean",
					"description": "For task.complete: acknowledge known checklist failures",
				},
			},
			"required": []string{"event"},
		},
		ReadOnly: false,
	}
}

// UpdateScratchpadTool returns the tool definition for updating the scratchpad
func UpdateScratchpadTool() Tool {
	return Tool{
		Name: "update_scratchpad",
		Description: `Update your working notes. Persists across iterations and context compaction.

Call after:
- Completing a significant step
- Making an important decision
- Encountering a blocker
- Natural stopping points`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"understanding": map[string]any{
					"type":        "string",
					"description": "Current understanding of task and codebase",
				},
				"plan": map[string]any{
					"type":        "string",
					"description": "Current plan/checklist (mark completed with [x])",
				},
				"decisions": map[string]any{
					"type":        "string",
					"description": "Key decisions made and rationale",
				},
				"blockers": map[string]any{
					"type":        "string",
					"description": "Any issues preventing progress",
				},
				"last_action": map[string]any{
					"type":        "string",
					"description": "What you just did and what's next",
				},
			},
			"required": []string{"understanding", "plan", "last_action"},
		},
		ReadOnly: false,
	}
}

// StoreMemoryTool returns the tool definition for storing project memories
func StoreMemoryTool() Tool {
	return Tool{
		Name:        "store_memory",
		Description: "Record a learning about this project for future tasks. Keep memories concise (1-2 sentences) and actionable.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category": map[string]any{
					"type": "string",
					"enum": []string{
						"architecture",
						"pattern",
						"pitfall",
						"decision",
						"fix",
						"convention",
						"dependency",
						"constraint",
					},
					"description": "Type of memory",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The learning (1-2 sentences, actionable)",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Where this was discovered (file path, tool output, etc.)",
				},
			},
			"required": []string{"category", "content"},
		},
		ReadOnly: false,
	}
}

// =============================================================================
// Planning Tools - for planning phase
// =============================================================================

// ConfirmPlanTool returns the tool definition for confirming a plan
func ConfirmPlanTool() Tool {
	return Tool{
		Name:        "confirm_plan",
		Description: "Confirm the plan and provide the refined prompt for task execution.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"refined_prompt": map[string]any{
					"type":        "string",
					"description": "The refined, clarified prompt for task execution",
				},
			},
			"required": []string{"refined_prompt"},
		},
		ReadOnly: true,
	}
}

// ProposeChecklistTool returns the tool definition for proposing a checklist
func ProposeChecklistTool() Tool {
	return Tool{
		Name:        "propose_checklist",
		Description: "Propose a checklist for the task with required and optional items.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"must_have": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Required checklist items",
				},
				"optional": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional nice-to-have items",
				},
			},
			"required": []string{"must_have"},
		},
		ReadOnly: true,
	}
}
