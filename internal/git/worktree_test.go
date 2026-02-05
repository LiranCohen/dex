package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []WorktreeInfo
	}{
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name: "single main worktree",
			input: `worktree /home/user/project
HEAD abc123def456
branch refs/heads/main
`,
			expected: []WorktreeInfo{
				{
					Path:       "/home/user/project",
					CommitHash: "abc123def456",
					Branch:     "main",
				},
			},
		},
		{
			name: "main and task worktree",
			input: `worktree /home/user/project
HEAD abc123def456
branch refs/heads/main

worktree /home/user/worktrees/project-task-abc123
HEAD def456abc789
branch refs/heads/task/task-abc123
`,
			expected: []WorktreeInfo{
				{
					Path:       "/home/user/project",
					CommitHash: "abc123def456",
					Branch:     "main",
				},
				{
					Path:       "/home/user/worktrees/project-task-abc123",
					CommitHash: "def456abc789",
					Branch:     "task/task-abc123",
				},
			},
		},
		{
			name: "detached head worktree",
			input: `worktree /home/user/worktrees/detached
HEAD abc123
detached
`,
			expected: []WorktreeInfo{
				{
					Path:       "/home/user/worktrees/detached",
					CommitHash: "abc123",
					Branch:     "(detached)",
				},
			},
		},
		{
			name: "bare worktree",
			input: `worktree /home/user/project.git
bare
`,
			expected: []WorktreeInfo{
				{
					Path: "/home/user/project.git",
					Bare: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseWorktreeList(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d worktrees, got %d", len(tt.expected), len(result))
			}

			for i, exp := range tt.expected {
				got := result[i]
				if got.Path != exp.Path {
					t.Errorf("worktree[%d].Path = %q, want %q", i, got.Path, exp.Path)
				}
				if got.Branch != exp.Branch {
					t.Errorf("worktree[%d].Branch = %q, want %q", i, got.Branch, exp.Branch)
				}
				if got.CommitHash != exp.CommitHash {
					t.Errorf("worktree[%d].CommitHash = %q, want %q", i, got.CommitHash, exp.CommitHash)
				}
				if got.Bare != exp.Bare {
					t.Errorf("worktree[%d].Bare = %v, want %v", i, got.Bare, exp.Bare)
				}
			}
		})
	}
}

// setupTestRepo creates a temporary git repository for testing
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
		{"git", "config", "commit.gpgsign", "false"}, // Disable signing for tests
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmpDir
		if output, err := cmd.CombinedOutput(); err != nil {
			cleanup()
			t.Fatalf("setup command %v failed: %s: %v", args, output, err)
		}
	}

	return tmpDir, cleanup
}

// createCommit creates a commit with the given message
func createCommit(t *testing.T, repoPath, message string) string {
	t.Helper()

	// Create a file to commit
	testFile := filepath.Join(repoPath, "test.txt")
	content := []byte(message + "\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmds := [][]string{
		{"git", "add", "test.txt"},
		{"git", "commit", "-m", message},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoPath
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit command %v failed: %s: %v", args, output, err)
		}
	}

	// Get commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}

	return string(output[:len(output)-1]) // trim newline
}

func TestIsBranchMerged(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create initial commit on main
	createCommit(t, repoPath, "initial commit")

	// Rename default branch to main (git init might use master)
	cmd := exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoPath
	_, _ = cmd.CombinedOutput() // Ignore error if already named main

	// Create a feature branch
	cmd = exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create feature branch: %s: %v", output, err)
	}

	// Add a commit to feature branch
	createCommit(t, repoPath, "feature commit")

	// Switch back to main
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to checkout main: %s: %v", output, err)
	}

	mgr := NewWorktreeManager("/tmp/worktrees")

	// Test: feature branch should NOT be merged yet
	merged, err := mgr.IsBranchMerged(repoPath, "feature-branch", "main")
	if err != nil {
		t.Fatalf("IsBranchMerged failed: %v", err)
	}
	if merged {
		t.Error("expected feature-branch to NOT be merged, but it was reported as merged")
	}

	// Merge the feature branch into main
	cmd = exec.Command("git", "merge", "feature-branch", "-m", "merge feature")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to merge: %s: %v", output, err)
	}

	// Test: feature branch should now be merged
	merged, err = mgr.IsBranchMerged(repoPath, "feature-branch", "main")
	if err != nil {
		t.Fatalf("IsBranchMerged failed: %v", err)
	}
	if !merged {
		t.Error("expected feature-branch to be merged, but it was reported as not merged")
	}
}

func TestIsBranchMerged_DeletedBranch(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create initial commit on main
	createCommit(t, repoPath, "initial commit")

	// Rename default branch to main
	cmd := exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoPath
	_, _ = cmd.CombinedOutput()

	// Create and merge a feature branch
	cmd = exec.Command("git", "checkout", "-b", "temp-branch")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create temp branch: %s: %v", output, err)
	}

	createCommit(t, repoPath, "temp commit")

	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to checkout main: %s: %v", output, err)
	}

	cmd = exec.Command("git", "merge", "temp-branch", "-m", "merge temp")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to merge: %s: %v", output, err)
	}

	// Delete the branch
	cmd = exec.Command("git", "branch", "-d", "temp-branch")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to delete branch: %s: %v", output, err)
	}

	mgr := NewWorktreeManager("/tmp/worktrees")

	// Test: deleted branch should be considered merged (branch doesn't exist)
	merged, err := mgr.IsBranchMerged(repoPath, "temp-branch", "main")
	if err != nil {
		t.Fatalf("IsBranchMerged failed: %v", err)
	}
	if !merged {
		t.Error("expected deleted branch to be considered merged")
	}
}

func TestIsBranchMerged_NonexistentBranch(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create initial commit
	createCommit(t, repoPath, "initial commit")

	mgr := NewWorktreeManager("/tmp/worktrees")

	// Test: nonexistent branch should be considered merged (assuming it was merged and deleted)
	merged, err := mgr.IsBranchMerged(repoPath, "nonexistent-branch", "main")
	if err != nil {
		t.Fatalf("IsBranchMerged failed: %v", err)
	}
	if !merged {
		t.Error("expected nonexistent branch to be considered merged")
	}
}
