package worker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProjectManager handles project setup for worker execution.
// It clones projects and manages the working directory.
type ProjectManager struct {
	dataDir string // Base directory for worker data
}

// NewProjectManager creates a new ProjectManager.
func NewProjectManager(dataDir string) *ProjectManager {
	return &ProjectManager{
		dataDir: dataDir,
	}
}

// SetupProject clones or updates a project and returns the working directory.
// Projects are cloned to: {dataDir}/projects/{owner}/{repo}/
func (pm *ProjectManager) SetupProject(project Project, baseBranch string) (workDir string, err error) {
	if project.CloneURL == "" {
		return "", fmt.Errorf("project has no clone URL")
	}

	// Determine the project directory
	projectDir := pm.getProjectDir(project)

	// Create parent directory
	parentDir := filepath.Dir(projectDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create project parent directory: %w", err)
	}

	// Check if project already exists
	if pm.projectExists(projectDir) {
		// Update existing project
		fmt.Printf("ProjectManager: updating existing project at %s\n", projectDir)
		if err := pm.updateProject(projectDir, baseBranch); err != nil {
			// If update fails, try removing and re-cloning
			fmt.Printf("ProjectManager: update failed, attempting fresh clone: %v\n", err)
			if rmErr := os.RemoveAll(projectDir); rmErr != nil {
				return "", fmt.Errorf("failed to remove corrupt project: %w", rmErr)
			}
			if err := pm.cloneProject(project.CloneURL, projectDir, baseBranch); err != nil {
				return "", err
			}
		}
	} else {
		// Clone new project
		fmt.Printf("ProjectManager: cloning new project to %s\n", projectDir)
		if err := pm.cloneProject(project.CloneURL, projectDir, baseBranch); err != nil {
			return "", err
		}
	}

	return projectDir, nil
}

// getProjectDir returns the directory path for a project.
func (pm *ProjectManager) getProjectDir(project Project) string {
	owner := project.GitHubOwner
	repo := project.GitHubRepo

	// Fall back to parsing from clone URL if owner/repo not provided
	if owner == "" || repo == "" {
		owner, repo = parseCloneURL(project.CloneURL)
	}

	if owner == "" {
		owner = "unknown"
	}
	if repo == "" {
		repo = project.ID
	}

	return filepath.Join(pm.dataDir, "projects", owner, repo)
}

// projectExists checks if a project directory exists and is a valid git repo.
func (pm *ProjectManager) projectExists(projectDir string) bool {
	gitDir := filepath.Join(projectDir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// cloneProject clones a project to the given directory.
func (pm *ProjectManager) cloneProject(cloneURL, projectDir, baseBranch string) error {
	args := []string{"clone", "--depth", "50"}

	if baseBranch != "" {
		args = append(args, "--branch", baseBranch)
	}

	args = append(args, cloneURL, projectDir)

	fmt.Printf("ProjectManager: git %s\n", strings.Join(args, " "))

	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}

	return nil
}

// updateProject updates an existing project to the latest state.
func (pm *ProjectManager) updateProject(projectDir, baseBranch string) error {
	// Fetch latest changes
	fetchCmd := exec.Command("git", "fetch", "--depth", "50", "origin")
	fetchCmd.Dir = projectDir
	fetchCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if output, err := fetchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch failed: %s: %w", string(output), err)
	}

	// Reset any local changes
	resetCmd := exec.Command("git", "reset", "--hard")
	resetCmd.Dir = projectDir

	if output, err := resetCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset failed: %s: %w", string(output), err)
	}

	// Checkout the base branch
	branch := baseBranch
	if branch == "" {
		branch = "main"
	}

	checkoutCmd := exec.Command("git", "checkout", branch)
	checkoutCmd.Dir = projectDir

	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		// Try 'master' if 'main' fails
		if branch == "main" {
			checkoutCmd = exec.Command("git", "checkout", "master")
			checkoutCmd.Dir = projectDir
			if output2, err2 := checkoutCmd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git checkout failed (tried %s and master): %s: %w", branch, string(output2), err)
			}
		} else {
			return fmt.Errorf("git checkout %s failed: %s: %w", branch, string(output), err)
		}
	}

	// Pull latest changes
	pullCmd := exec.Command("git", "pull", "--ff-only")
	pullCmd.Dir = projectDir
	pullCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if output, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %s: %w", string(output), err)
	}

	return nil
}

// CreateBranch creates a new branch for the objective.
func (pm *ProjectManager) CreateBranch(workDir, branchName string) error {
	// Check if branch already exists
	checkCmd := exec.Command("git", "rev-parse", "--verify", branchName)
	checkCmd.Dir = workDir
	if err := checkCmd.Run(); err == nil {
		// Branch exists, checkout
		checkoutCmd := exec.Command("git", "checkout", branchName)
		checkoutCmd.Dir = workDir
		if output, err := checkoutCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout %s failed: %s: %w", branchName, string(output), err)
		}
		return nil
	}

	// Create and checkout new branch
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout -b %s failed: %s: %w", branchName, string(output), err)
	}

	return nil
}

// GetCurrentBranch returns the current git branch.
func (pm *ProjectManager) GetCurrentBranch(workDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// Cleanup removes the project directory.
func (pm *ProjectManager) Cleanup(workDir string) error {
	if workDir == "" {
		return nil
	}

	// Verify the path is within our data directory
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("failed to resolve work directory: %w", err)
	}

	absDataDir, err := filepath.Abs(pm.dataDir)
	if err != nil {
		return fmt.Errorf("failed to resolve data directory: %w", err)
	}

	if !strings.HasPrefix(absWorkDir, absDataDir+string(filepath.Separator)) {
		return fmt.Errorf("refusing to clean up directory outside data directory")
	}

	return os.RemoveAll(workDir)
}

// parseCloneURL extracts owner/repo from a GitHub clone URL.
func parseCloneURL(url string) (owner, repo string) {
	// Handle HTTPS URLs: https://github.com/owner/repo.git
	// Handle SSH URLs: git@github.com:owner/repo.git

	url = strings.TrimSuffix(url, ".git")

	if strings.HasPrefix(url, "https://") {
		// HTTPS format
		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2], parts[len(parts)-1]
		}
	} else if strings.HasPrefix(url, "git@") {
		// SSH format: git@github.com:owner/repo
		if idx := strings.Index(url, ":"); idx != -1 {
			path := url[idx+1:]
			parts := strings.Split(path, "/")
			if len(parts) >= 2 {
				return parts[0], parts[1]
			}
		}
	}

	return "", ""
}

// SetupAuthenticatedCloneURL updates a clone URL to include authentication.
// This is used when cloning private repositories.
func SetupAuthenticatedCloneURL(cloneURL, githubToken string) string {
	if githubToken == "" {
		return cloneURL
	}

	// Only modify HTTPS URLs
	if !strings.HasPrefix(cloneURL, "https://") {
		return cloneURL
	}

	// Transform https://github.com/... to https://x-access-token:TOKEN@github.com/...
	// or https://TOKEN@github.com/...
	if strings.HasPrefix(cloneURL, "https://github.com/") {
		return strings.Replace(cloneURL, "https://github.com/",
			fmt.Sprintf("https://x-access-token:%s@github.com/", githubToken), 1)
	}

	return cloneURL
}
