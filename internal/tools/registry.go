package tools

import "sync"

// toolRegistry maps tool names to their factory functions
var toolRegistry = map[string]func() Tool{
	// File system read
	"read_file":  ReadFileTool,
	"list_files": ListFilesTool,
	"glob":       GlobTool,
	"grep":       GrepTool,

	// File system write
	"write_file": WriteFileTool,

	// Git read
	"git_status": GitStatusTool,
	"git_diff":   GitDiffTool,
	"git_log":    GitLogTool,

	// Git write
	"git_init":       GitInitTool,
	"git_commit":     GitCommitTool,
	"git_remote_add": GitRemoteAddTool,
	"git_push":       GitPushTool,

	// GitHub
	"github_create_repo": GitHubCreateRepoTool,
	"github_create_pr":   GitHubCreatePRTool,

	// Web
	"web_search": WebSearchTool,
	"web_fetch":  WebFetchTool,

	// Runtime
	"bash":          BashTool,
	"list_runtimes": ListRuntimesTool,

	// Quality
	"run_tests": RunTestsTool,
	"run_lint":  RunLintTool,
	"run_build": RunBuildTool,

	// Completion
	"task_complete": TaskCompleteTool,
}

var registryMu sync.RWMutex

// GetToolByName returns a tool by its name, or nil if not found
func GetToolByName(name string) *Tool {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if factory, exists := toolRegistry[name]; exists {
		tool := factory()
		return &tool
	}
	return nil
}

// RegisterTool registers a new tool factory
func RegisterTool(name string, factory func() Tool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	toolRegistry[name] = factory
}

// ListRegisteredTools returns all registered tool names
func ListRegisteredTools() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}
	return names
}

// GetAllRegisteredTools returns all registered tools
func GetAllRegisteredTools() []Tool {
	registryMu.RLock()
	defer registryMu.RUnlock()

	tools := make([]Tool, 0, len(toolRegistry))
	for _, factory := range toolRegistry {
		tools = append(tools, factory())
	}
	return tools
}
