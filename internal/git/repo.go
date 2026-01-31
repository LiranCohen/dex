package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RepoManager handles git repository creation and management
type RepoManager struct {
	reposDir string // Base directory for repos (e.g., /opt/dex/repos)
}

// NewRepoManager creates a new RepoManager
func NewRepoManager(reposDir string) *RepoManager {
	return &RepoManager{
		reposDir: reposDir,
	}
}

// CreateOptions configures a new repository
type CreateOptions struct {
	Name          string // Directory name (will be sanitized)
	Description   string // For README
	DefaultBranch string // Default: "main"
	InitialCommit bool   // Create initial commit with README
}

// Create initializes a new git repository
// Returns the full path to the created repository
func (m *RepoManager) Create(opts CreateOptions) (string, error) {
	if opts.Name == "" {
		return "", fmt.Errorf("repository name is required")
	}

	// Sanitize the name
	safeName := sanitizeRepoName(opts.Name)
	if safeName == "" {
		return "", fmt.Errorf("invalid repository name: %s", opts.Name)
	}

	// Determine the full path
	repoPath := filepath.Join(m.reposDir, safeName)

	// Check if repo already exists
	if m.Exists(repoPath) {
		return "", fmt.Errorf("repository already exists: %s", repoPath)
	}

	// Create the directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create repository directory: %w", err)
	}

	// Determine default branch
	defaultBranch := opts.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", "-b", defaultBranch)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		// Clean up on failure
		os.RemoveAll(repoPath)
		return "", fmt.Errorf("failed to initialize git repo: %w\n%s", err, output)
	}

	// Create initial commit if requested
	if opts.InitialCommit {
		if err := m.createInitialCommit(repoPath, safeName, opts.Description); err != nil {
			// Clean up on failure
			os.RemoveAll(repoPath)
			return "", fmt.Errorf("failed to create initial commit: %w", err)
		}
	}

	return repoPath, nil
}

// Exists checks if a git repository exists at the given path
func (m *RepoManager) Exists(repoPath string) bool {
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// SetRemote adds or updates the origin remote
func (m *RepoManager) SetRemote(repoPath, remoteURL string) error {
	if !m.Exists(repoPath) {
		return fmt.Errorf("repository does not exist: %s", repoPath)
	}

	// Check if origin already exists
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	if err := cmd.Run(); err == nil {
		// Origin exists, update it
		cmd = exec.Command("git", "remote", "set-url", "origin", remoteURL)
	} else {
		// Origin doesn't exist, add it
		cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
	}
	cmd.Dir = repoPath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set remote: %w\n%s", err, output)
	}

	return nil
}

// GetPath returns the full path for a repository name
func (m *RepoManager) GetPath(name string) string {
	safeName := sanitizeRepoName(name)
	return filepath.Join(m.reposDir, safeName)
}

// Clone clones a repository from a URL
func (m *RepoManager) Clone(cloneURL, name string) (string, error) {
	if name == "" {
		// Extract name from URL
		name = extractRepoNameFromURL(cloneURL)
	}
	if name == "" {
		return "", fmt.Errorf("could not determine repository name from URL")
	}

	safeName := sanitizeRepoName(name)
	repoPath := filepath.Join(m.reposDir, safeName)

	// Check if repo already exists
	if m.Exists(repoPath) {
		return "", fmt.Errorf("repository already exists: %s", repoPath)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(m.reposDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create repos directory: %w", err)
	}

	// Clone the repository
	cmd := exec.Command("git", "clone", cloneURL, repoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to clone repository: %w\n%s", err, output)
	}

	return repoPath, nil
}

// createInitialCommit creates a README and initial commit
func (m *RepoManager) createInitialCommit(repoPath, name, description string) error {
	// Create README content
	readmeContent := fmt.Sprintf("# %s\n", name)
	if description != "" {
		readmeContent += fmt.Sprintf("\n%s\n", description)
	}

	// Write README
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to create README: %w", err)
	}

	// Stage README
	cmd := exec.Command("git", "add", "README.md")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage README: %w\n%s", err, output)
	}

	// Create initial commit
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	// Set git identity for the commit
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Dex",
		"GIT_AUTHOR_EMAIL=dex@local",
		"GIT_COMMITTER_NAME=Dex",
		"GIT_COMMITTER_EMAIL=dex@local",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create initial commit: %w\n%s", err, output)
	}

	return nil
}

// sanitizeRepoName converts a name to a safe directory name
// Only allows alphanumeric, dashes, and underscores
func sanitizeRepoName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces with dashes
	name = strings.ReplaceAll(name, " ", "-")

	// Remove any characters that aren't alphanumeric, dashes, or underscores
	reg := regexp.MustCompile(`[^a-z0-9\-_]`)
	name = reg.ReplaceAllString(name, "")

	// Remove leading/trailing dashes
	name = strings.Trim(name, "-_")

	// Collapse multiple dashes
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	return name
}

// extractRepoNameFromURL extracts the repository name from a git URL
func extractRepoNameFromURL(url string) string {
	// Remove trailing .git
	url = strings.TrimSuffix(url, ".git")

	// Get the last path component
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
