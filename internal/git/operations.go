package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Operations provides git commands for working with repositories
type Operations struct{}

// NewOperations creates a new Operations instance
func NewOperations() *Operations {
	return &Operations{}
}

// CommitOptions configures a git commit
type CommitOptions struct {
	Message    string // Commit message (required)
	All        bool   // Stage all tracked files (-a flag)
	AllowEmpty bool   // Allow empty commit
	Author     string // Override author (optional, format: "Name <email>")
}

// Commit creates a git commit in the specified directory
func (o *Operations) Commit(dir string, opts CommitOptions) (string, error) {
	if opts.Message == "" {
		return "", fmt.Errorf("commit message is required")
	}

	args := []string{"commit"}

	if opts.All {
		args = append(args, "-a")
	}
	if opts.AllowEmpty {
		args = append(args, "--allow-empty")
	}
	if opts.Author != "" {
		args = append(args, "--author", opts.Author)
	}
	args = append(args, "-m", opts.Message)

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("commit failed: %s: %w", string(output), err)
	}

	// Get the commit hash
	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = dir
	hash, err := hashCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}

	return strings.TrimSpace(string(hash)), nil
}

// PushOptions configures a git push
type PushOptions struct {
	Remote      string // Remote name (default: "origin")
	Branch      string // Branch to push (default: current branch)
	SetUpstream bool   // Set upstream tracking (-u flag)
	Force       bool   // Force push (use with caution)
}

// Push pushes commits to a remote
func (o *Operations) Push(dir string, opts PushOptions) error {
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	args := []string{"push"}

	if opts.SetUpstream {
		args = append(args, "-u")
	}
	if opts.Force {
		args = append(args, "--force-with-lease") // Safer than --force
	}

	args = append(args, remote)

	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("push failed: %s: %w", string(output), err)
	}

	return nil
}

// PullOptions configures a git pull
type PullOptions struct {
	Remote string // Remote name (default: "origin")
	Branch string // Branch to pull (optional)
	Rebase bool   // Use rebase instead of merge
	FFOnly bool   // Only fast-forward (fail if not possible)
}

// Pull pulls changes from a remote
func (o *Operations) Pull(dir string, opts PullOptions) error {
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	args := []string{"pull"}

	if opts.Rebase {
		args = append(args, "--rebase")
	}
	if opts.FFOnly {
		args = append(args, "--ff-only")
	}

	args = append(args, remote)

	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pull failed: %s: %w", string(output), err)
	}

	return nil
}

// GetCurrentBranch returns the current branch name
func (o *Operations) GetCurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		// Detached HEAD state
		return "", fmt.Errorf("detached HEAD state")
	}

	return branch, nil
}

// DiffOptions configures git diff output
type DiffOptions struct {
	Staged   bool   // Show staged changes (--cached)
	Base     string // Compare against base (e.g., "main", "origin/main")
	Path     string // Limit to specific path
	NameOnly bool   // Only show file names
	Stat     bool   // Show diffstat summary
}

// GetDiff returns the diff output
func (o *Operations) GetDiff(dir string, opts DiffOptions) (string, error) {
	args := []string{"diff"}

	if opts.Staged {
		args = append(args, "--cached")
	}
	if opts.NameOnly {
		args = append(args, "--name-only")
	}
	if opts.Stat {
		args = append(args, "--stat")
	}
	if opts.Base != "" {
		args = append(args, opts.Base+"..HEAD")
	}
	if opts.Path != "" {
		args = append(args, "--", opts.Path)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("diff failed: %w", err)
	}

	return string(output), nil
}

// Stage stages files for commit
func (o *Operations) Stage(dir string, paths ...string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no paths specified")
	}

	args := append([]string{"add"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stage failed: %s: %w", string(output), err)
	}

	return nil
}

// Fetch fetches from remote without merging
func (o *Operations) Fetch(dir, remote string) error {
	if remote == "" {
		remote = "origin"
	}

	cmd := exec.Command("git", "fetch", remote)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetch failed: %s: %w", string(output), err)
	}

	return nil
}

// LogEntry represents a git log entry
type LogEntry struct {
	Hash    string
	Subject string
	Author  string
	Date    string
}

// GetLog returns recent commit log entries
func (o *Operations) GetLog(dir string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 10
	}

	// Format: hash<NUL>subject<NUL>author<NUL>date (null byte delimiter avoids collision with pipe in subjects)
	format := "%H%x00%s%x00%an%x00%aI"
	cmd := exec.Command("git", "log", fmt.Sprintf("-n%d", limit), fmt.Sprintf("--format=%s", format))
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("log failed: %w", err)
	}

	var entries []LogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) == 4 {
			entries = append(entries, LogEntry{
				Hash:    parts[0],
				Subject: parts[1],
				Author:  parts[2],
				Date:    parts[3],
			})
		}
	}

	return entries, nil
}
