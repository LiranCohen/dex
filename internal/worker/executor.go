package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/toolbelt"
	"github.com/lirancohen/dex/internal/tools"
)

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	Output  string
	IsError bool
}

// WorkerToolExecutor executes tools in the worker context.
// It extends the base tools.Executor with git operations and GitHub client support.
type WorkerToolExecutor struct {
	*tools.Executor
	gitOps       *git.Operations
	githubClient *toolbelt.GitHubClient
	owner        string
	repo         string
	workDir      string
	qualityGate  *WorkerQualityGate
	cmdRunner    CommandRunner
}

// NewWorkerToolExecutor creates a new tool executor for the worker.
func NewWorkerToolExecutor(workDir, owner, repo, githubToken string) *WorkerToolExecutor {
	// Create git operations
	gitOps := git.NewOperations()

	// Create GitHub client if token is provided
	var githubClient *toolbelt.GitHubClient
	if githubToken != "" {
		githubClient = toolbelt.NewGitHubClient(&toolbelt.GitHubConfig{
			Token: githubToken,
		})
	}

	return &WorkerToolExecutor{
		Executor:     tools.NewExecutor(workDir, tools.ReadWriteTools(), false),
		gitOps:       gitOps,
		githubClient: githubClient,
		owner:        owner,
		repo:         repo,
		workDir:      workDir,
		cmdRunner:    NewExecCommandRunner(),
	}
}

// SetCommandRunner sets a custom command runner (for testing).
func (e *WorkerToolExecutor) SetCommandRunner(runner CommandRunner) {
	e.cmdRunner = runner
}

// SetGitHubClient sets the GitHub client (for cases where it's configured later).
func (e *WorkerToolExecutor) SetGitHubClient(client *toolbelt.GitHubClient) {
	e.githubClient = client
}

// SetQualityGate sets the quality gate for task completion validation.
func (e *WorkerToolExecutor) SetQualityGate(qg *WorkerQualityGate) {
	e.qualityGate = qg
}

// Execute runs a tool with the given input and returns the result.
// Overrides base executor for tools that need git.Operations or GitHub client.
func (e *WorkerToolExecutor) Execute(ctx context.Context, toolName string, input map[string]any) ToolResult {
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
		// Use base executor for all other tools
		baseResult := e.Executor.Execute(ctx, toolName, input)
		return ToolResult{
			Output:  baseResult.Output,
			IsError: baseResult.IsError,
		}
	}

	// Apply large response processing
	if !result.IsError && len(result.Output) > tools.LargeResponseThreshold {
		result.Output = tools.ProcessLargeResponse(toolName, result.Output)
	}

	return result
}

func (e *WorkerToolExecutor) executeGitDiff(input map[string]any) ToolResult {
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

	diff, err := e.gitOps.GetDiff(e.workDir, opts)
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

func (e *WorkerToolExecutor) executeGitCommit(input map[string]any) ToolResult {
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
			if err := e.gitOps.Stage(e.workDir, paths...); err != nil {
				return ToolResult{
					Output:  fmt.Sprintf("Failed to stage files: %v", err),
					IsError: true,
				}
			}
		}
	}

	hash, err := e.gitOps.Commit(e.workDir, git.CommitOptions{
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

func (e *WorkerToolExecutor) executeGitPush(input map[string]any) ToolResult {
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
	branch, err := e.gitOps.GetCurrentBranch(e.workDir)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to get current branch: %v", err),
			IsError: true,
		}
	}
	opts.Branch = branch

	// Set up authenticated remote URL before pushing
	if e.githubClient != nil && e.githubClient.Token() != "" {
		if err := e.setupAuthenticatedRemote(); err != nil {
			fmt.Printf("Warning: failed to set up authenticated remote: %v\n", err)
		}
	}

	if err := e.gitOps.Push(e.workDir, opts); err != nil {
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

func (e *WorkerToolExecutor) executeGitRemoteAdd(input map[string]any) ToolResult {
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

	result := e.cmdRunner.RunGit(context.Background(), e.workDir, "remote", "add", name, url)
	if result.Err != nil {
		if strings.Contains(result.Output, "already exists") {
			updateResult := e.cmdRunner.RunGit(context.Background(), e.workDir, "remote", "set-url", name, url)
			if updateResult.Err != nil {
				return ToolResult{
					Output:  fmt.Sprintf("git remote set-url failed: %s: %v", updateResult.Output, updateResult.Err),
					IsError: true,
				}
			}
			return ToolResult{
				Output:  fmt.Sprintf("Updated remote '%s'", name),
				IsError: false,
			}
		}
		return ToolResult{
			Output:  fmt.Sprintf("git remote add failed: %s: %v", result.Output, result.Err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Added remote '%s'", name),
		IsError: false,
	}
}

// setupAuthenticatedRemote converts the origin remote URL to use token authentication.
func (e *WorkerToolExecutor) setupAuthenticatedRemote() error {
	if e.githubClient == nil {
		return fmt.Errorf("no GitHub client configured")
	}

	// Get current remote URL
	result := e.cmdRunner.RunGit(context.Background(), e.workDir, "remote", "get-url", "origin")
	if result.Err != nil {
		return fmt.Errorf("failed to get remote URL: %w", result.Err)
	}

	currentURL := strings.TrimSpace(result.Output)
	if currentURL == "" {
		return fmt.Errorf("no origin remote configured")
	}

	// Convert to authenticated URL
	authURL := e.githubClient.AuthURL(currentURL)
	if authURL == currentURL {
		return nil // No change needed
	}

	// Set the authenticated URL
	setResult := e.cmdRunner.RunGit(context.Background(), e.workDir, "remote", "set-url", "origin", authURL)
	if setResult.Err != nil {
		return fmt.Errorf("failed to set remote URL: %s: %w", setResult.Output, setResult.Err)
	}

	return nil
}

func (e *WorkerToolExecutor) executeGitHubCreateRepo(ctx context.Context, input map[string]any) ToolResult {
	if e.githubClient == nil {
		return ToolResult{Output: "GitHub client not configured", IsError: true}
	}

	name, ok := input["name"].(string)
	if !ok || name == "" {
		return ToolResult{Output: "name is required", IsError: true}
	}

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

	cloneURL := repo.GetCloneURL()
	if cloneURL == "" {
		cloneURL = repo.GetHTMLURL() + ".git"
	}

	// Update executor's owner/repo
	e.owner = owner
	e.repo = name

	return ToolResult{
		Output:  fmt.Sprintf("Created repository: %s\nClone URL: %s\nUse this URL with git_remote_add to connect your local repo.", repo.GetHTMLURL(), cloneURL),
		IsError: false,
	}
}

func (e *WorkerToolExecutor) executeGitHubCreatePR(ctx context.Context, input map[string]any) ToolResult {
	if e.githubClient == nil {
		return ToolResult{Output: "GitHub client not configured", IsError: true}
	}

	title, ok := input["title"].(string)
	if !ok || title == "" {
		return ToolResult{Output: "title is required", IsError: true}
	}

	if e.gitOps == nil {
		return ToolResult{Output: "Git operations not configured", IsError: true}
	}

	branch, err := e.gitOps.GetCurrentBranch(e.workDir)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to get current branch: %v", err),
			IsError: true,
		}
	}

	// Auto-push the branch before creating PR
	pushResult := e.executeGitPush(map[string]any{"set_upstream": true})
	if pushResult.IsError {
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
		Base:  "main",
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

func (e *WorkerToolExecutor) executeRunTests(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewWorkerQualityGate(e.workDir)
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

func (e *WorkerToolExecutor) executeRunLint(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewWorkerQualityGate(e.workDir)
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

func (e *WorkerToolExecutor) executeRunBuild(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewWorkerQualityGate(e.workDir)
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

func (e *WorkerToolExecutor) executeTaskComplete(ctx context.Context, input map[string]any) ToolResult {
	if e.qualityGate == nil {
		e.qualityGate = NewWorkerQualityGate(e.workDir)
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

	return ToolResult{
		Output:  result.Feedback,
		IsError: !result.Passed,
	}
}

// TaskCompleteOpts are options for task completion validation.
type TaskCompleteOpts struct {
	Summary   string
	SkipTests bool
	SkipLint  bool
	SkipBuild bool
}

// WorkerQualityGate runs quality checks for task completion.
type WorkerQualityGate struct {
	workDir       string
	projectConfig *tools.ProjectConfig
	cmdRunner     CommandRunner
}

// GateResult represents the result of a quality gate check.
type GateResult struct {
	Passed     bool
	Skipped    bool
	SkipReason string
	Output     string
	DurationMs int64
	Feedback   string
}

// NewWorkerQualityGate creates a new quality gate.
func NewWorkerQualityGate(workDir string) *WorkerQualityGate {
	return &WorkerQualityGate{
		workDir:       workDir,
		projectConfig: tools.DetectProject(workDir),
		cmdRunner:     NewExecCommandRunner(),
	}
}

// NewWorkerQualityGateWithRunner creates a quality gate with a custom command runner.
// This is useful for testing.
func NewWorkerQualityGateWithRunner(workDir string, runner CommandRunner) *WorkerQualityGate {
	return &WorkerQualityGate{
		workDir:       workDir,
		projectConfig: tools.DetectProject(workDir),
		cmdRunner:     runner,
	}
}

// SetCommandRunner sets a custom command runner (for testing).
func (qg *WorkerQualityGate) SetCommandRunner(runner CommandRunner) {
	qg.cmdRunner = runner
}

// RunTests runs the test suite.
func (qg *WorkerQualityGate) RunTests(ctx context.Context, verbose bool, timeoutSecs int) *GateResult {
	cmd, ok := qg.projectConfig.GetTestCommand()
	if !ok {
		return &GateResult{
			Skipped:    true,
			SkipReason: "No test command configured for this project type",
		}
	}

	return qg.runCommand(ctx, cmd, timeoutSecs)
}

// RunLint runs the linter.
func (qg *WorkerQualityGate) RunLint(ctx context.Context, fix bool) *GateResult {
	cmd, ok := qg.projectConfig.GetLintCommand()
	if !ok {
		return &GateResult{
			Skipped:    true,
			SkipReason: "No lint command configured for this project type",
		}
	}

	if fix && qg.projectConfig.Type == tools.ProjectTypeGo {
		cmd = "golangci-lint run --fix"
	}

	return qg.runCommand(ctx, cmd, 300)
}

// RunBuild runs the build.
func (qg *WorkerQualityGate) RunBuild(ctx context.Context, timeoutSecs int) *GateResult {
	cmd, ok := qg.projectConfig.GetBuildCommand()
	if !ok {
		return &GateResult{
			Skipped:    true,
			SkipReason: "No build command configured for this project type",
		}
	}

	return qg.runCommand(ctx, cmd, timeoutSecs)
}

// Validate runs all quality checks and returns a combined result.
func (qg *WorkerQualityGate) Validate(ctx context.Context, opts TaskCompleteOpts) *GateResult {
	var failures []string

	if !opts.SkipTests {
		testResult := qg.RunTests(ctx, false, 300)
		if !testResult.Skipped && !testResult.Passed {
			failures = append(failures, fmt.Sprintf("Tests failed:\n%s", testResult.Output))
		}
	}

	if !opts.SkipLint {
		lintResult := qg.RunLint(ctx, false)
		if !lintResult.Skipped && !lintResult.Passed {
			failures = append(failures, fmt.Sprintf("Lint issues:\n%s", lintResult.Output))
		}
	}

	if !opts.SkipBuild {
		buildResult := qg.RunBuild(ctx, 300)
		if !buildResult.Skipped && !buildResult.Passed {
			failures = append(failures, fmt.Sprintf("Build failed:\n%s", buildResult.Output))
		}
	}

	if len(failures) > 0 {
		return &GateResult{
			Passed:   false,
			Feedback: fmt.Sprintf("Quality gate failed:\n\n%s", strings.Join(failures, "\n\n")),
		}
	}

	return &GateResult{
		Passed:   true,
		Feedback: fmt.Sprintf("Quality gate passed. Task completed: %s", opts.Summary),
	}
}

// runCommand runs a shell command with timeout enforcement and returns the result.
func (qg *WorkerQualityGate) runCommand(ctx context.Context, command string, timeoutSecs int) *GateResult {
	start := time.Now()

	// Apply timeout if specified
	if timeoutSecs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
		defer cancel()
	}

	result := qg.cmdRunner.Run(ctx, qg.workDir, command)
	duration := time.Since(start).Milliseconds()

	if result.Err != nil {
		// Check if it was a timeout
		errMsg := result.Output
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("Command timed out after %d seconds\n%s", timeoutSecs, result.Output)
		}
		return &GateResult{
			Passed:     false,
			Output:     errMsg,
			DurationMs: duration,
		}
	}

	return &GateResult{
		Passed:     true,
		Output:     result.Output,
		DurationMs: duration,
	}
}
