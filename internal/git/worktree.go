package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeManager handles git worktree operations for task isolation
type WorktreeManager struct {
	worktreeBase string // Base dir for all worktrees, e.g., ~/src/worktrees
}

// NewWorktreeManager creates a worktree manager with the given base directory
func NewWorktreeManager(worktreeBase string) *WorktreeManager {
	return &WorktreeManager{
		worktreeBase: worktreeBase,
	}
}

// WorktreeInfo contains information about a worktree
type WorktreeInfo struct {
	Path       string // Absolute path to worktree
	Branch     string // Branch name (e.g., "task/task-a1b2")
	CommitHash string // Current HEAD commit
	Bare       bool   // Is this the bare main worktree entry?
}

// Create creates a new worktree for a task
// projectPath: path to the main repo (e.g., ~/src/project-alpha)
// taskID: the task identifier (e.g., "a1b2c3d4")
// baseBranch: branch to base the worktree on (e.g., "main")
// Returns the path to the created worktree
func (m *WorktreeManager) Create(projectPath, taskID, baseBranch string) (string, error) {
	// Extract project name from path
	projectName := filepath.Base(projectPath)

	// Build worktree path: ~/src/worktrees/project-alpha-task-a1b2
	worktreePath := filepath.Join(m.worktreeBase, fmt.Sprintf("%s-task-%s", projectName, taskID))

	// Build branch name: task/task-a1b2
	branchName := fmt.Sprintf("task/task-%s", taskID)

	// Ensure worktree base directory exists
	if err := os.MkdirAll(m.worktreeBase, 0755); err != nil {
		return "", fmt.Errorf("failed to create worktree base dir: %w", err)
	}

	// Check if branch already exists
	checkCmd := exec.Command("git", "rev-parse", "--verify", "--quiet", branchName)
	checkCmd.Dir = projectPath
	branchExists := checkCmd.Run() == nil

	var cmd *exec.Cmd
	if branchExists {
		// Branch exists - create worktree using existing branch
		cmd = exec.Command("git", "worktree", "add", worktreePath, branchName)
	} else {
		// Create new branch
		cmd = exec.Command("git", "worktree", "add", worktreePath, "-b", branchName, baseBranch)
	}
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}

	return worktreePath, nil
}

// Remove removes a worktree and optionally its associated branch
// force: if true, remove even if there are uncommitted changes
// cleanupBranch: if true, also delete the task branch after removal
func (m *WorktreeManager) Remove(projectPath, worktreePath string, force, cleanupBranch bool) error {
	// First, get the branch name from the worktree before removing it
	var branchName string
	if cleanupBranch {
		worktrees, err := m.List(projectPath)
		if err == nil {
			for _, wt := range worktrees {
				if wt.Path == worktreePath {
					branchName = wt.Branch
					break
				}
			}
		}
	}

	// Remove the worktree
	args := []string{"worktree", "remove", worktreePath}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
	}

	// Optionally delete the branch
	if cleanupBranch && branchName != "" {
		branchCmd := exec.Command("git", "branch", "-D", branchName)
		branchCmd.Dir = projectPath
		_, _ = branchCmd.CombinedOutput() // Ignore errors - branch may already be merged/deleted
	}

	return nil
}

// List returns all worktrees for a project
func (m *WorktreeManager) List(projectPath string) ([]WorktreeInfo, error) {
	// Run: git worktree list --porcelain
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return parseWorktreeList(string(output)), nil
}

// parseWorktreeList parses the porcelain output of git worktree list
func parseWorktreeList(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	var current WorktreeInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "HEAD ") {
			current.CommitHash = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			// Format: "branch refs/heads/task/task-a1b2"
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "bare" {
			current.Bare = true
		} else if line == "detached" {
			current.Branch = "(detached)"
		}
	}

	// Don't forget the last one
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// GitStatus represents the status of a worktree
type GitStatus struct {
	CurrentBranch  string
	Ahead          int
	Behind         int
	StagedFiles    int
	ModifiedFiles  int
	UntrackedFiles int
	HasConflicts   bool
}

// GetStatus returns the git status of a worktree
func (m *WorktreeManager) GetStatus(worktreePath string) (*GitStatus, error) {
	status := &GitStatus{}

	// Get current branch - fail if this doesn't work (invalid git dir)
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get branch (invalid git directory?): %w", err)
	}
	status.CurrentBranch = strings.TrimSpace(string(out))

	// Get ahead/behind from upstream
	cmd = exec.Command("git", "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	cmd.Dir = worktreePath
	if out, err := cmd.Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) == 2 {
			_, _ = fmt.Sscanf(parts[0], "%d", &status.Behind)
			_, _ = fmt.Sscanf(parts[1], "%d", &status.Ahead)
		}
	}

	// Get file status using git status --porcelain
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	if out, err := cmd.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if len(line) < 2 {
				continue
			}
			indexStatus := line[0]
			workStatus := line[1]

			// Check for conflicts (both modified)
			if indexStatus == 'U' || workStatus == 'U' ||
				(indexStatus == 'A' && workStatus == 'A') ||
				(indexStatus == 'D' && workStatus == 'D') {
				status.HasConflicts = true
			}

			// Staged
			if indexStatus != ' ' && indexStatus != '?' {
				status.StagedFiles++
			}

			// Modified in working tree
			if workStatus == 'M' || workStatus == 'D' {
				status.ModifiedFiles++
			}

			// Untracked
			if indexStatus == '?' {
				status.UntrackedFiles++
			}
		}
	}

	return status, nil
}

// Exists checks if a worktree path exists and is a valid worktree
func (m *WorktreeManager) Exists(worktreePath string) bool {
	// Check if directory exists
	info, err := os.Stat(worktreePath)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check if it's a git worktree (has .git file pointing to parent)
	gitPath := filepath.Join(worktreePath, ".git")
	_, err = os.Stat(gitPath)
	return err == nil
}

// GetWorktreePath returns the expected worktree path for a project and task
func (m *WorktreeManager) GetWorktreePath(projectPath, taskID string) string {
	projectName := filepath.Base(projectPath)
	return filepath.Join(m.worktreeBase, fmt.Sprintf("%s-task-%s", projectName, taskID))
}

// GetWorktreeBase returns the base directory for worktrees
func (m *WorktreeManager) GetWorktreeBase() string {
	return m.worktreeBase
}

// IsBranchMerged checks if a branch has been merged into the base branch
// Uses git merge-base --is-ancestor to check if all commits from the branch
// are in the base branch
func (m *WorktreeManager) IsBranchMerged(projectPath, branchName, baseBranch string) (bool, error) {
	// First fetch to ensure we have latest remote state
	fetchCmd := exec.Command("git", "fetch", "origin", baseBranch)
	fetchCmd.Dir = projectPath
	_, _ = fetchCmd.CombinedOutput() // Ignore fetch errors - may not have network

	// Check if branch exists locally
	checkCmd := exec.Command("git", "rev-parse", "--verify", "--quiet", branchName)
	checkCmd.Dir = projectPath
	if err := checkCmd.Run(); err != nil {
		// Branch doesn't exist locally - check if it was deleted (likely merged)
		// Try to find the branch on remote
		remoteCheckCmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "origin/"+branchName)
		remoteCheckCmd.Dir = projectPath
		if remoteCheckCmd.Run() != nil {
			// Branch doesn't exist locally or remotely - assume it was merged and deleted
			return true, nil
		}
		// Use remote branch for merge check
		branchName = "origin/" + branchName
	}

	// Check if branchName is an ancestor of baseBranch (i.e., merged)
	// git merge-base --is-ancestor <branch> <base> returns 0 if true
	cmd := exec.Command("git", "merge-base", "--is-ancestor", branchName, "origin/"+baseBranch)
	cmd.Dir = projectPath
	err := cmd.Run()

	if err == nil {
		return true, nil // Branch is merged
	}

	// Exit code 1 means not an ancestor (not merged)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	// Other error
	return false, fmt.Errorf("failed to check merge status: %w", err)
}
