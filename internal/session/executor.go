// Package session provides session lifecycle management for Poindexter
package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// ToolResult represents the result of executing a tool
type ToolResult struct {
	Output  string
	IsError bool
}

// ToolExecutor executes tools in the context of a worktree
type ToolExecutor struct {
	worktreePath string
	gitOps       *git.Operations
	githubClient *toolbelt.GitHubClient
	owner        string
	repo         string
}

// NewToolExecutor creates a new ToolExecutor
func NewToolExecutor(worktreePath string, gitOps *git.Operations, githubClient *toolbelt.GitHubClient, owner, repo string) *ToolExecutor {
	return &ToolExecutor{
		worktreePath: worktreePath,
		gitOps:       gitOps,
		githubClient: githubClient,
		owner:        owner,
		repo:         repo,
	}
}

// Execute runs a tool with the given input and returns the result
func (e *ToolExecutor) Execute(ctx context.Context, toolName string, input map[string]any) ToolResult {
	switch toolName {
	case "bash":
		return e.executeBash(ctx, input)
	case "read_file":
		return e.executeReadFile(input)
	case "write_file":
		return e.executeWriteFile(input)
	case "list_files":
		return e.executeListFiles(input)
	case "git_init":
		return e.executeGitInit(input)
	case "git_status":
		return e.executeGitStatus()
	case "git_diff":
		return e.executeGitDiff(input)
	case "git_commit":
		return e.executeGitCommit(input)
	case "git_remote_add":
		return e.executeGitRemoteAdd(input)
	case "git_push":
		return e.executeGitPush(input)
	case "github_create_repo":
		return e.executeGitHubCreateRepo(ctx, input)
	case "github_create_pr":
		return e.executeGitHubCreatePR(ctx, input)
	default:
		return ToolResult{
			Output:  fmt.Sprintf("Unknown tool: %s", toolName),
			IsError: true,
		}
	}
}

// Command blocklist patterns for security
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rm\s+(-[rf]+\s+)?/`),           // rm -rf /
	regexp.MustCompile(`(?i)>\s*/dev/`),                     // redirect to /dev/
	regexp.MustCompile(`(?i)sudo\s`),                        // sudo commands
	regexp.MustCompile(`(?i)chmod\s+777`),                   // chmod 777
	regexp.MustCompile(`(?i)mkfs\.`),                        // filesystem format
	regexp.MustCompile(`(?i)dd\s+.*of=/dev/`),               // dd to device
	regexp.MustCompile(`(?i):\(\)\s*\{\s*:\|\s*:\s*&\s*\}`), // fork bomb
}

func (e *ToolExecutor) isDangerousCommand(cmd string) bool {
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(cmd) {
			return true
		}
	}
	return false
}

func (e *ToolExecutor) executeBash(ctx context.Context, input map[string]any) ToolResult {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return ToolResult{Output: "command is required", IsError: true}
	}

	// Security check
	if e.isDangerousCommand(command) {
		return ToolResult{
			Output:  "Command blocked: potentially dangerous operation detected",
			IsError: true,
		}
	}

	// Parse timeout (default 5 minutes, max 5 minutes)
	timeoutSecs := 300
	if t, ok := input["timeout_seconds"].(float64); ok {
		timeoutSecs = int(t)
		if timeoutSecs > 300 {
			timeoutSecs = 300
		}
		if timeoutSecs < 1 {
			timeoutSecs = 1
		}
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	cmd.Dir = e.worktreePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return ToolResult{
				Output:  fmt.Sprintf("Command timed out after %d seconds", timeoutSecs),
				IsError: true,
			}
		}
		return ToolResult{
			Output:  fmt.Sprintf("%s\nError: %v", string(output), err),
			IsError: true,
		}
	}

	return ToolResult{Output: string(output), IsError: false}
}

// resolvePath safely resolves a relative path within the worktree
// Returns error if the path escapes the worktree
func (e *ToolExecutor) resolvePath(relativePath string) (string, error) {
	// Clean the path to remove any .. or . components
	cleanPath := filepath.Clean(relativePath)

	// Reject absolute paths
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths not allowed: %s", relativePath)
	}

	// Join with worktree path
	fullPath := filepath.Join(e.worktreePath, cleanPath)

	// Verify the resolved path is still within the worktree
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	absWorktree, err := filepath.Abs(e.worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve worktree path: %w", err)
	}

	// Ensure path is within worktree (path traversal prevention)
	if !strings.HasPrefix(absPath, absWorktree+string(filepath.Separator)) && absPath != absWorktree {
		return "", fmt.Errorf("path escapes worktree: %s", relativePath)
	}

	return fullPath, nil
}

func (e *ToolExecutor) executeReadFile(input map[string]any) ToolResult {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return ToolResult{Output: "path is required", IsError: true}
	}

	fullPath, err := e.resolvePath(path)
	if err != nil {
		return ToolResult{Output: err.Error(), IsError: true}
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to read file: %v", err),
			IsError: true,
		}
	}

	return ToolResult{Output: string(content), IsError: false}
}

func (e *ToolExecutor) executeWriteFile(input map[string]any) ToolResult {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return ToolResult{Output: "path is required", IsError: true}
	}

	content, ok := input["content"].(string)
	if !ok {
		return ToolResult{Output: "content is required", IsError: true}
	}

	fullPath, err := e.resolvePath(path)
	if err != nil {
		return ToolResult{Output: err.Error(), IsError: true}
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to create directory: %v", err),
			IsError: true,
		}
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to write file: %v", err),
			IsError: true,
		}
	}

	return ToolResult{Output: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), IsError: false}
}

func (e *ToolExecutor) executeListFiles(input map[string]any) ToolResult {
	path := "."
	if p, ok := input["path"].(string); ok && p != "" {
		path = p
	}

	fullPath, err := e.resolvePath(path)
	if err != nil {
		return ToolResult{Output: err.Error(), IsError: true}
	}

	recursive := false
	if r, ok := input["recursive"].(bool); ok {
		recursive = r
	}

	var files []string

	if recursive {
		err = filepath.WalkDir(fullPath, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(e.worktreePath, p)
			if d.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return ToolResult{
				Output:  fmt.Sprintf("Failed to list directory: %v", err),
				IsError: true,
			}
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			files = append(files, name)
		}
	}

	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to list files: %v", err),
			IsError: true,
		}
	}

	return ToolResult{Output: strings.Join(files, "\n"), IsError: false}
}

func (e *ToolExecutor) executeGitInit(input map[string]any) ToolResult {
	defaultBranch := "main"
	if branch, ok := input["default_branch"].(string); ok && branch != "" {
		defaultBranch = branch
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", defaultBranch)
	cmd.Dir = e.worktreePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("git init failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Initialized git repository with default branch '%s'\n%s", defaultBranch, string(output)),
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

	// Add remote
	cmd := exec.Command("git", "remote", "add", name, url)
	cmd.Dir = e.worktreePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if remote already exists
		if strings.Contains(string(output), "already exists") {
			// Update the remote URL instead
			updateCmd := exec.Command("git", "remote", "set-url", name, url)
			updateCmd.Dir = e.worktreePath
			updateOutput, updateErr := updateCmd.CombinedOutput()
			if updateErr != nil {
				return ToolResult{
					Output:  fmt.Sprintf("git remote set-url failed: %s: %v", string(updateOutput), updateErr),
					IsError: true,
				}
			}
			return ToolResult{
				Output:  fmt.Sprintf("Updated remote '%s' to %s", name, url),
				IsError: false,
			}
		}
		return ToolResult{
			Output:  fmt.Sprintf("git remote add failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return ToolResult{
		Output:  fmt.Sprintf("Added remote '%s' pointing to %s", name, url),
		IsError: false,
	}
}

func (e *ToolExecutor) executeGitStatus() ToolResult {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = e.worktreePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("git status failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	if len(output) == 0 {
		return ToolResult{Output: "Working directory clean", IsError: false}
	}

	return ToolResult{Output: string(output), IsError: false}
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

	diff, err := e.gitOps.GetDiff(e.worktreePath, opts)
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
			if err := e.gitOps.Stage(e.worktreePath, paths...); err != nil {
				return ToolResult{
					Output:  fmt.Sprintf("Failed to stage files: %v", err),
					IsError: true,
				}
			}
		}
	}

	hash, err := e.gitOps.Commit(e.worktreePath, git.CommitOptions{
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
	branch, err := e.gitOps.GetCurrentBranch(e.worktreePath)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Failed to get current branch: %v", err),
			IsError: true,
		}
	}
	opts.Branch = branch

	if err := e.gitOps.Push(e.worktreePath, opts); err != nil {
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

	branch, err := e.gitOps.GetCurrentBranch(e.worktreePath)
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
