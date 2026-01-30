// Package e2e contains end-to-end tests that require real API credentials
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// TestGitHubPushE2E tests the complete flow of:
// 1. Creating a branch in a worktree
// 2. Making changes and committing
// 3. Pushing to GitHub
// 4. Creating a PR
//
// Prerequisites:
// - GITHUB_TOKEN environment variable set
// - DEX_E2E_REPO set to "owner/repo" format (a test repo you own)
// - The repo must exist and you must have push access
//
// Run with: go test -v ./internal/e2e -run TestGitHubPushE2E -tags=e2e
func TestGitHubPushE2E(t *testing.T) {
	if os.Getenv("DEX_E2E_ENABLED") != "true" {
		t.Skip("Skipping e2e test: set DEX_E2E_ENABLED=true to run")
	}

	// Get required environment variables
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Fatal("GITHUB_TOKEN environment variable required")
	}

	repoSpec := os.Getenv("DEX_E2E_REPO")
	if repoSpec == "" {
		t.Fatal("DEX_E2E_REPO environment variable required (format: owner/repo)")
	}

	// Parse owner/repo
	var owner, repo string
	if _, err := fmt.Sscanf(repoSpec, "%s/%s", &owner, &repo); err != nil {
		// Try splitting by /
		parts := splitRepoSpec(repoSpec)
		if len(parts) != 2 {
			t.Fatalf("Invalid DEX_E2E_REPO format, expected owner/repo: %s", repoSpec)
		}
		owner, repo = parts[0], parts[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup GitHub client
	ghClient := toolbelt.NewGitHubClient(&toolbelt.GitHubConfig{
		Token:      token,
		DefaultOrg: owner,
	})

	// Step 1: Test GitHub connection
	t.Log("Step 1: Testing GitHub connection...")
	if err := ghClient.Ping(ctx); err != nil {
		t.Fatalf("GitHub ping failed: %v", err)
	}
	t.Log("✓ GitHub connection successful")

	// Step 2: Clone the repo to a temp directory
	t.Log("Step 2: Cloning test repo...")
	tmpDir, err := os.MkdirTemp("", "dex-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	cloneCmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", repoURL, tmpDir)
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("Clone failed: %s: %v", string(output), err)
	}
	t.Logf("✓ Cloned to %s", tmpDir)

	// Detect the default branch
	defaultBranch := detectDefaultBranch(t, ctx, tmpDir)
	t.Logf("✓ Default branch: %s", defaultBranch)

	// Step 3: Create a test branch
	branchName := fmt.Sprintf("dex-e2e-test-%d", time.Now().Unix())
	t.Logf("Step 3: Creating branch %s...", branchName)

	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	checkoutCmd.Dir = tmpDir
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		t.Fatalf("Branch creation failed: %s: %v", string(output), err)
	}
	t.Logf("✓ Created branch %s", branchName)

	// Step 4: Make a change
	t.Log("Step 4: Making a test change...")
	testFile := filepath.Join(tmpDir, "dex-e2e-test.txt")
	testContent := fmt.Sprintf("E2E test file created at %s\nThis file can be safely deleted.", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	t.Log("✓ Created test file")

	// Step 5: Stage and commit using our git operations
	t.Log("Step 5: Staging and committing...")
	ops := git.NewOperations()

	if err := ops.Stage(tmpDir, "dex-e2e-test.txt"); err != nil {
		t.Fatalf("Stage failed: %v", err)
	}

	commitHash, err := ops.Commit(tmpDir, git.CommitOptions{
		Message: "[dex e2e] Test commit - safe to delete",
		Author:  "Dex E2E Test <dex-e2e@example.com>",
	})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	t.Logf("✓ Committed: %s", commitHash[:8])

	// Step 6: Push to GitHub
	t.Log("Step 6: Pushing to GitHub...")
	if err := ops.Push(tmpDir, git.PushOptions{
		SetUpstream: true,
		Branch:      branchName,
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	t.Log("✓ Pushed to GitHub")

	// Step 7: Create a PR
	t.Log("Step 7: Creating pull request...")
	pr, err := ghClient.CreatePR(ctx, toolbelt.CreatePROptions{
		Owner: owner,
		Repo:  repo,
		Title: "[dex e2e] Test PR - please close and delete branch",
		Body:  fmt.Sprintf("This is an automated E2E test PR created by dex.\n\nCreated: %s\nCommit: %s\n\n**This PR can be safely closed and the branch deleted.**", time.Now().Format(time.RFC3339), commitHash[:8]),
		Head:  branchName,
		Base:  defaultBranch,
		Draft: true,
	})
	if err != nil {
		t.Fatalf("Failed to create PR (base: %s): %v", defaultBranch, err)
	}
	t.Logf("✓ Created PR #%d: %s", pr.GetNumber(), pr.GetHTMLURL())

	// Step 8: Verify PR exists (optional - could close it here for cleanup)
	t.Log("Step 8: Verifying PR...")
	t.Logf("✓ E2E test complete! PR URL: %s", pr.GetHTMLURL())
	t.Log("")
	t.Log("CLEANUP REQUIRED:")
	t.Logf("  1. Close PR #%d at %s", pr.GetNumber(), pr.GetHTMLURL())
	t.Logf("  2. Delete branch %s", branchName)
}

// splitRepoSpec splits "owner/repo" into ["owner", "repo"]
func splitRepoSpec(spec string) []string {
	for i, c := range spec {
		if c == '/' {
			return []string{spec[:i], spec[i+1:]}
		}
	}
	return []string{spec}
}

// detectDefaultBranch detects the default branch of the cloned repo
func detectDefaultBranch(t *testing.T, ctx context.Context, repoDir string) string {
	t.Helper()

	// Method 1: Check what branch we're on after clone (shallow clone lands on default)
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(output))
		if branch != "" && branch != "HEAD" {
			return branch
		}
	}

	// Method 2: Try to get from remote HEAD
	cmd = exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoDir
	output, err = cmd.Output()
	if err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(string(output))
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}

	// Fallback to main
	t.Log("Could not detect default branch, falling back to 'main'")
	return "main"
}

// TestGitHubConnectionOnly tests just the GitHub API connection
// This is a lighter test that doesn't create any resources
func TestGitHubConnectionOnly(t *testing.T) {
	if os.Getenv("DEX_E2E_ENABLED") != "true" {
		t.Skip("Skipping e2e test: set DEX_E2E_ENABLED=true to run")
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Fatal("GITHUB_TOKEN environment variable required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ghClient := toolbelt.NewGitHubClient(&toolbelt.GitHubConfig{
		Token: token,
	})

	if err := ghClient.Ping(ctx); err != nil {
		t.Fatalf("GitHub ping failed: %v", err)
	}

	// List repos to verify token has proper scopes
	repos, err := ghClient.ListRepos(ctx, toolbelt.ListReposOptions{PerPage: 5})
	if err != nil {
		t.Fatalf("Failed to list repos: %v", err)
	}

	t.Logf("✓ GitHub connection successful, found %d repos", len(repos))
}
