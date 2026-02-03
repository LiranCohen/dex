// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

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
	// Callback when a GitHub repo is created - allows updating project DB record
	onRepoCreated func(owner, repo string)
	// Callback when quality gate runs - allows posting issue comments
	onQualityGateResult func(result *GateResult)
	// Quality gate for task completion validation
	qualityGate *QualityGate
	// Activity recorder for logging quality gate attempts
	activity *ActivityRecorder
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

// SetOnRepoCreated sets the callback for when a GitHub repo is created
func (e *ToolExecutor) SetOnRepoCreated(callback func(owner, repo string)) {
	e.onRepoCreated = callback
}

// SetQualityGate sets the quality gate for task completion validation
func (e *ToolExecutor) SetQualityGate(qg *QualityGate) {
	e.qualityGate = qg
}

// SetActivityRecorder sets the activity recorder for quality gate logging
func (e *ToolExecutor) SetActivityRecorder(activity *ActivityRecorder) {
	e.activity = activity
}

// SetOnQualityGateResult sets the callback for quality gate results
// This allows posting issue comments when quality gates are run
func (e *ToolExecutor) SetOnQualityGateResult(callback func(result *GateResult)) {
	e.onQualityGateResult = callback
}

// Execute runs a tool with the given input and returns the result
// Overrides base executor for tools that need git.Operations or GitHub client
func (e *ToolExecutor) Execute(ctx context.Context, toolName string, input map[string]any) ToolResult {
	var result ToolResult

	switch toolName {
	// Tools that need advanced git operations
	case "git_diff":
		result = e.executeGitDiff(input)
	case "git_commit":
		result = e.executeGitCommit(input)
	case "git_push":
		result = e.executeGitPush(input)
	case "git_remote_add":
		result = e.executeGitRemoteAdd(input)
	// Tools that need GitHub client
	case "github_create_repo":
		result = e.executeGitHubCreateRepo(ctx, input)
	case "github_create_pr":
		result = e.executeGitHubCreatePR(ctx, input)
	// Quality gate tools
	case "run_tests":
		result = e.executeRunTests(ctx, input)
	case "run_lint":
		result = e.executeRunLint(ctx, input)
	case "run_build":
		result = e.executeRunBuild(ctx, input)
	case "task_complete":
		result = e.executeTaskComplete(ctx, input)
	default:
		// Use base executor for all other tools (already applies large response processing)
		baseResult := e.Executor.Execute(ctx, toolName, input)
		return ToolResult{
			Output:  baseResult.Output,
			IsError: baseResult.IsError,
		}
	}

	// Apply large response processing for session executor handled tools
	// This prevents massive git diffs, test outputs, etc. from bloating context
	if !result.IsError && len(result.Output) > tools.LargeResponseThreshold {
		result.Output = tools.ProcessLargeResponse(toolName, result.Output)
	}

	return result
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

	// If we have a GitHub client, set up authenticated remote URL before pushing
	if e.githubClient != nil && e.githubClient.Token() != "" {
		if err := e.setupAuthenticatedRemote(); err != nil {
			// Log but don't fail - maybe auth isn't needed (e.g., SSH)
			fmt.Printf("Warning: failed to set up authenticated remote: %v\n", err)
		}
	}

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

func (e *ToolExecutor) executeGitRemoteAdd(input map[string]any) ToolResult {
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return ToolResult{Output: "url is required", IsError: true}
	}

	name := "origin"
	if n, ok := input["name"].(string); ok && n != "" {
		name = n
	}

	// Convert to authenticated URL if we have a GitHub client
	if e.githubClient != nil && e.githubClient.Token() != "" {
		url = e.githubClient.AuthURL(url)
	}

	cmd := exec.Command("git", "remote", "add", name, url)
	cmd.Dir = e.WorkDir()

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			updateCmd := exec.Command("git", "remote", "set-url", name, url)
			updateCmd.Dir = e.WorkDir()
			updateOutput, updateErr := updateCmd.CombinedOutput()
			if updateErr != nil {
				return ToolResult{
					Output:  fmt.Sprintf("git remote set-url failed: %s: %v", string(updateOutput), updateErr),
					IsError: true,
				}
			}
			return ToolResult{
				Output:  fmt.Sprintf("Updated remote '%s'", name),
				IsError: false,
			}
		}
		return ToolResult{
			Output:  fmt.Sprintf("git remote add failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Added remote '%s'", name),
		IsError: false,
	}
}

// setupAuthenticatedRemote converts the origin remote URL to use token authentication
func (e *ToolExecutor) setupAuthenticatedRemote() error {
	if e.githubClient == nil {
		return fmt.Errorf("no GitHub client configured")
	}

	// Get current remote URL
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = e.WorkDir()
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	currentURL := strings.TrimSpace(string(output))
	if currentURL == "" {
		return fmt.Errorf("no origin remote configured")
	}

	// Convert to authenticated URL
	authURL := e.githubClient.AuthURL(currentURL)
	if authURL == currentURL {
		// No change needed (might be SSH or already authenticated)
		return nil
	}

	// Set the authenticated URL
	cmd = exec.Command("git", "remote", "set-url", "origin", authURL)
	cmd.Dir = e.WorkDir()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set remote URL: %s: %w", string(output), err)
	}

	return nil
}

func (e *ToolExecutor) executeGitHubCreateRepo(ctx context.Context, input map[string]any) ToolResult {
	if e.githubClient == nil {
		return ToolResult{Output: "GitHub client not configured", IsError: true}
	}

	name, ok := input["name"].(string)
	if !ok || name == "" {
		return ToolResult{Output: "name is required", IsError: true}
	}

	// Parse owner/repo format if provided, otherwise use project owner
	owner := e.owner
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		owner = parts[0]
		name = parts[1]
	}

	opts := toolbelt.CreateRepoOptions{
		Name: name,
		Org:  owner,
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

	// Notify that a repo was created (allows updating project DB record)
	if e.onRepoCreated != nil {
		e.onRepoCreated(owner, name)
	}

	// Update executor's owner/repo so subsequent operations use the new repo
	e.owner = owner
	e.repo = name

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

	// Auto-push the branch before creating PR to ensure it exists on remote
	// This prevents 422 errors from GitHub when the branch hasn't been pushed yet
	pushResult := e.executeGitPush(map[string]any{"set_upstream": true})
	if pushResult.IsError {
		// Only fail if it's not an "already up to date" style message
		if !strings.Contains(strings.ToLower(pushResult.Output), "up to date") &&
			!strings.Contains(strings.ToLower(pushResult.Output), "everything up-to-date") {
			return ToolResult{
				Output:  fmt.Sprintf("Failed to push branch before creating PR: %v", pushResult.Output),
				IsError: true,
			}
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
		errMsg := err.Error()
		// Provide more helpful error messages for common issues
		if strings.Contains(errMsg, "422") {
			if strings.Contains(errMsg, "already exists") {
				return ToolResult{
					Output:  fmt.Sprintf("A pull request already exists for branch '%s'. Check existing PRs or use a different branch.", branch),
					IsError: true,
				}
			}
			if strings.Contains(errMsg, "No commits") || strings.Contains(errMsg, "no difference") {
				return ToolResult{
					Output:  fmt.Sprintf("Cannot create PR: branch '%s' has no changes compared to '%s'. Make sure you have committed changes.", branch, opts.Base),
					IsError: true,
				}
			}
		}
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

// Quality gate tool implementations

func (e *ToolExecutor) executeRunTests(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewQualityGate(e.WorkDir(), e.activity)
	}

	verbose := false
	if v, ok := input["verbose"].(bool); ok {
		verbose = v
	}

	timeoutSecs := 300
	if t, ok := input["timeout_seconds"].(float64); ok {
		timeoutSecs = int(t)
	}

	result := e.qualityGate.RunTests(ctx, verbose, timeoutSecs)

	if result.Skipped {
		return ToolResult{
			Output:  fmt.Sprintf("Tests skipped: %s", result.SkipReason),
			IsError: false,
		}
	}

	if result.Passed {
		return ToolResult{
			Output:  fmt.Sprintf("Tests passed (%dms)\n\n%s", result.DurationMs, result.Output),
			IsError: false,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Tests failed (%dms)\n\n%s", result.DurationMs, result.Output),
		IsError: true,
	}
}

func (e *ToolExecutor) executeRunLint(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewQualityGate(e.WorkDir(), e.activity)
	}

	fix := false
	if f, ok := input["fix"].(bool); ok {
		fix = f
	}

	result := e.qualityGate.RunLint(ctx, fix)

	if result.Skipped {
		return ToolResult{
			Output:  fmt.Sprintf("Lint skipped: %s", result.SkipReason),
			IsError: false,
		}
	}

	if result.Passed {
		return ToolResult{
			Output:  fmt.Sprintf("Lint passed (%dms)\n\n%s", result.DurationMs, result.Output),
			IsError: false,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Lint issues found (%dms)\n\n%s", result.DurationMs, result.Output),
		IsError: true,
	}
}

func (e *ToolExecutor) executeRunBuild(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewQualityGate(e.WorkDir(), e.activity)
	}

	timeoutSecs := 300
	if t, ok := input["timeout_seconds"].(float64); ok {
		timeoutSecs = int(t)
	}

	result := e.qualityGate.RunBuild(ctx, timeoutSecs)

	if result.Skipped {
		return ToolResult{
			Output:  fmt.Sprintf("Build skipped: %s", result.SkipReason),
			IsError: false,
		}
	}

	if result.Passed {
		return ToolResult{
			Output:  fmt.Sprintf("Build succeeded (%dms)\n\n%s", result.DurationMs, result.Output),
			IsError: false,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Build failed (%dms)\n\n%s", result.DurationMs, result.Output),
		IsError: true,
	}
}

func (e *ToolExecutor) executeTaskComplete(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewQualityGate(e.WorkDir(), e.activity)
	}

	summary, ok := input["summary"].(string)
	if !ok || summary == "" {
		return ToolResult{Output: "summary is required", IsError: true}
	}

	opts := TaskCompleteOpts{
		Summary: summary,
	}

	if skipTests, ok := input["skip_tests"].(bool); ok {
		opts.SkipTests = skipTests
	}
	if skipLint, ok := input["skip_lint"].(bool); ok {
		opts.SkipLint = skipLint
	}
	if skipBuild, ok := input["skip_build"].(bool); ok {
		opts.SkipBuild = skipBuild
	}

	result := e.qualityGate.Validate(ctx, opts)

	// Invoke callback to allow posting issue comments
	if e.onQualityGateResult != nil {
		e.onQualityGateResult(result)
	}

	return ToolResult{
		Output:  result.Feedback,
		IsError: !result.Passed,
	}
}
