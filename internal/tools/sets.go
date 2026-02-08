package tools

// ReadOnlyTools returns the set of read-only tools suitable for Quest chat (research/planning)
// These tools can explore the codebase and gather information but cannot modify anything
func ReadOnlyTools() *Set {
	return NewSet([]Tool{
		ReadFileTool(),
		ListFilesTool(),
		GlobTool(),
		GrepTool(),
		GitStatusTool(),
		GitDiffTool(),
		GitLogTool(),
		WebSearchTool(),
		WebFetchTool(),
		ListRuntimesTool(),
	})
}

// QuestTools returns tools for Quest conversation phase
// Includes read-only exploration tools plus quest-specific tools for questions and objectives
func QuestTools() *Set {
	return NewSet([]Tool{
		// Read-only exploration tools
		ReadFileTool(),
		ListFilesTool(),
		GlobTool(),
		GrepTool(),
		GitStatusTool(),
		GitDiffTool(),
		GitLogTool(),
		WebSearchTool(),
		WebFetchTool(),
		ListRuntimesTool(),
		// Quest-specific tools
		AskQuestionTool(),
		ProposeObjectiveTool(),
		CompleteQuestTool(),
	})
}

// WorkflowTools returns tools for task execution (used by both HQ and workers)
// These are the tools that replace signals in Ralph
func WorkflowTools() *Set {
	return NewSet([]Tool{
		MarkChecklistItemTool(),
		SignalEventTool(),
		UpdateScratchpadTool(),
		StoreMemoryTool(),
	})
}

// PlanningTools returns tools for the planning phase
func PlanningTools() *Set {
	return NewSet([]Tool{
		ConfirmPlanTool(),
		ProposeChecklistTool(),
	})
}

// ReadWriteTools returns the full set of tools for objective execution (RalphLoop)
// Includes all read-only tools plus tools that can modify files, run commands, and interact with services
func ReadWriteTools() *Set {
	return NewSet([]Tool{
		// Read-only tools
		ReadFileTool(),
		ListFilesTool(),
		GlobTool(),
		GrepTool(),
		GitStatusTool(),
		GitDiffTool(),
		GitLogTool(),
		WebSearchTool(),
		WebFetchTool(),
		ListRuntimesTool(),
		// Write tools
		BashTool(),
		WriteFileTool(),
		GitInitTool(),
		GitCommitTool(),
		GitRemoteAddTool(),
		GitPushTool(),
		GitHubCreateRepoTool(),
		GitHubCreatePRTool(),
		// Quality gate tools
		RunTestsTool(),
		RunLintTool(),
		RunBuildTool(),
		TaskCompleteTool(),
	})
}

// TaskExecutionTools returns the full set of tools for task execution (RalphLoop)
// Includes read-write tools plus workflow tools for checklist/event signaling
func TaskExecutionTools() *Set {
	return NewSet([]Tool{
		// Read-only tools
		ReadFileTool(),
		ListFilesTool(),
		GlobTool(),
		GrepTool(),
		GitStatusTool(),
		GitDiffTool(),
		GitLogTool(),
		WebSearchTool(),
		WebFetchTool(),
		ListRuntimesTool(),
		// Write tools
		BashTool(),
		WriteFileTool(),
		GitInitTool(),
		GitCommitTool(),
		GitRemoteAddTool(),
		GitPushTool(),
		GitHubCreateRepoTool(),
		GitHubCreatePRTool(),
		// Quality gate tools
		RunTestsTool(),
		RunLintTool(),
		RunBuildTool(),
		TaskCompleteTool(),
		// Workflow tools (replacing signals)
		MarkChecklistItemTool(),
		SignalEventTool(),
		UpdateScratchpadTool(),
		StoreMemoryTool(),
	})
}

// AllTools returns all defined tools
func AllTools() []Tool {
	return []Tool{
		// Read-only
		ReadFileTool(),
		ListFilesTool(),
		GlobTool(),
		GrepTool(),
		GitStatusTool(),
		GitDiffTool(),
		GitLogTool(),
		WebSearchTool(),
		WebFetchTool(),
		ListRuntimesTool(),
		// Write
		BashTool(),
		WriteFileTool(),
		GitInitTool(),
		GitCommitTool(),
		GitRemoteAddTool(),
		GitPushTool(),
		GitHubCreateRepoTool(),
		GitHubCreatePRTool(),
		// Quality gate
		RunTestsTool(),
		RunLintTool(),
		RunBuildTool(),
		TaskCompleteTool(),
		// Quest tools
		AskQuestionTool(),
		ProposeObjectiveTool(),
		CompleteQuestTool(),
		// Workflow tools
		MarkChecklistItemTool(),
		SignalEventTool(),
		UpdateScratchpadTool(),
		StoreMemoryTool(),
		// Planning tools
		ConfirmPlanTool(),
		ProposeChecklistTool(),
	}
}
