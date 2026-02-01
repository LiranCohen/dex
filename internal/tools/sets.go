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
		// Write tools
		BashTool(),
		WriteFileTool(),
		GitInitTool(),
		GitCommitTool(),
		GitRemoteAddTool(),
		GitPushTool(),
		GitHubCreateRepoTool(),
		GitHubCreatePRTool(),
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
		// Write
		BashTool(),
		WriteFileTool(),
		GitInitTool(),
		GitCommitTool(),
		GitRemoteAddTool(),
		GitPushTool(),
		GitHubCreateRepoTool(),
		GitHubCreatePRTool(),
	}
}
