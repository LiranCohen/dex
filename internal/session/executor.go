// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"

	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// ToolResult represents the result of executing a tool
// Wraps tools.Result for backwards compatibility
type ToolResult struct {
	Output  string
	IsError bool
}

// ToolExecutor executes tools in the context of a worktree
// Extends the base tools.Executor with git operations and GitHub client
type ToolExecutor struct {
	*tools.Executor
	gitOps       *git.Operations
	githubClient *toolbelt.GitHubClient
	owner        string
	repo         string
}

// NewToolExecutor creates a new ToolExecutor
func NewToolExecutor(worktreePath string, gitOps *git.Operations, githubClient *toolbelt.GitHubClient, owner, repo string) *ToolExecutor {
	return &ToolExecutor{
		Executor:     tools.NewExecutor(worktreePath, tools.ReadWriteTools(), false),
		gitOps:       gitOps,
		githubClient: githubClient,
		owner:        owner,
		repo:         repo,
	}
}

// Execute runs a tool with the given input and returns the result
// Overrides base executor for tools that need git.Operations or GitHub client
func (e *ToolExecutor) Execute(ctx context.Context, toolName string, input map[string]any) ToolResult {
	switch toolName {
	// Tools that need advanced git operations
	case "git_diff":
		return e.executeGitDiff(input)
	case "git_commit":
		return e.executeGitCommit(input)
	case "git_push":
		return e.executeGitPush(input)
	// Tools that need GitHub client
	case "github_create_repo":
		return e.executeGitHubCreateRepo(ctx, input)
	case "github_create_pr":
		return e.executeGitHubCreatePR(ctx, input)
	default:
		// Use base executor for all other tools
		result := e.Executor.Execute(ctx, toolName, input)
		return ToolResult{
			Output:  result.Output,
			IsError: result.IsError,
		}
	}
}

func (e *ToolExecutor) executeGitDiff(input map[string]any) ToolResult {
	if e.gitOps == nil {
		return ToolResult{Output: "Git operations not configured", IsError: true}
	}

	opts := git.DiffOptions{}

	if staged, ok := input["staged"].(bool); ok {
		opts.Staged = staged
	}
	if path, ok := input["path"].(string); ok {
		opts.Path = path
	}

	diff, err := e.gitOps.GetDiff(e.WorkDir(), opts)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("git diff failed: %v", err),
			IsError: true,
		}
	}

	if diff == "" {
		return ToolResult{Output: "No changes", IsError: false}
	}

	return ToolResult{Output: diff, IsError: false}
}

func (e *ToolExecutor) executeGitCommit(input map[string]any) ToolResult {
	if e.gitOps == nil {
		return ToolResult{Output: "Git operations not configured", IsError: true}
	}

	message, ok := input["message"].(string)
	if !ok || message == "" {
		return ToolResult{Output: "message is required", IsError: true}
	}

	// Stage files if specified
	if files, ok := input["files"].([]any); ok && len(files) > 0 {
		var paths []string
		for _, f := range files {
			if s, ok := f.(string); ok {
				paths = append(paths, s)
			}
		}
		if len(paths) > 0 {
			if err := e.gitOps.Stage(e.WorkDir(), paths...); err != nil {
				return ToolResult{
					Output:  fmt.Sprintf("Failed to stage files: %v", err),
					IsError: true,
				}
			}
		}
	}

	hash, err := e.gitOps.Commit(e.WorkDir(), git.CommitOptions{
		Message: message,
	})
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("git commit failed: %v", err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Created commit %s", hash),
		IsError: false,
	}
}

func (e *ToolExecutor) executeGitPush(input map[string]any) ToolResult {
	if e.gitOps == nil {
		return ToolResult{Output: "Git operations not configured", IsError: true}
	}

	opts := git.PushOptions{
		Remote: "origin",
	}

	if setUpstream, ok := input["set_upstream"].(bool); ok {
		opts.SetUpstream = setUpstream
	}

	// Get current branch for the push
	branch, err := e.gitOps.GetCurrentBranch(e.WorkDir())
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to get current branch: %v", err),
			IsError: true,
		}
	}
	opts.Branch = branch

	if err := e.gitOps.Push(e.WorkDir(), opts); err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("git push failed: %v", err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Pushed branch %s to origin", branch),
		IsError: false,
	}
}

func (e *ToolExecutor) executeGitHubCreateRepo(ctx context.Context, input map[string]any) ToolResult {
	if e.githubClient == nil {
		return ToolResult{Output: "GitHub client not configured", IsError: true}
	}

	name, ok := input["name"].(string)
	if !ok || name == "" {
		return ToolResult{Output: "name is required", IsError: true}
	}

	opts := toolbelt.CreateRepoOptions{
		Name: name,
		Org:  e.owner, // Use project owner for consistency
	}

	if desc, ok := input["description"].(string); ok {
		opts.Description = desc
	}
	if private, ok := input["private"].(bool); ok {
		opts.Private = private
	}

	repo, err := e.githubClient.CreateRepo(ctx, opts)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to create repository: %v", err),
			IsError: true,
		}
	}

	// Include both the HTML URL for viewing and clone URL for git remote add
	cloneURL := repo.GetCloneURL()
	if cloneURL == "" {
		cloneURL = repo.GetHTMLURL() + ".git"
	}

	return ToolResult{
		Output:  fmt.Sprintf("Created repository: %s\nClone URL: %s\nUse this URL with git_remote_add to connect your local repo.", repo.GetHTMLURL(), cloneURL),
		IsError: false,
	}
}

func (e *ToolExecutor) executeGitHubCreatePR(ctx context.Context, input map[string]any) ToolResult {
	if e.githubClient == nil {
		return ToolResult{Output: "GitHub client not configured", IsError: true}
	}

	title, ok := input["title"].(string)
	if !ok || title == "" {
		return ToolResult{Output: "title is required", IsError: true}
	}

	// Get current branch for the head
	if e.gitOps == nil {
		return ToolResult{Output: "Git operations not configured", IsError: true}
	}

	branch, err := e.gitOps.GetCurrentBranch(e.WorkDir())
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to get current branch: %v", err),
			IsError: true,
		}
	}

	opts := toolbelt.CreatePROptions{
		Owner: e.owner,
		Repo:  e.repo,
		Title: title,
		Head:  branch,
		Base:  "main", // default
	}

	if body, ok := input["body"].(string); ok {
		opts.Body = body
	}
	if base, ok := input["base"].(string); ok && base != "" {
		opts.Base = base
	}
	if draft, ok := input["draft"].(bool); ok {
		opts.Draft = draft
	}

	pr, err := e.githubClient.CreatePR(ctx, opts)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to create pull request: %v", err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Created pull request #%d: %s", pr.GetNumber(), pr.GetHTMLURL()),
		IsError: false,
	}
}
