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
	Owner         string // GitHub owner/org (used for path: {reposDir}/{owner}/{repo})
	Name          string // Repository name (will be sanitized)
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

	// Determine the full path using owner/repo structure if owner is provided
	var repoPath string
	if opts.Owner != "" {
		safeOwner := sanitizeRepoName(opts.Owner)
		if safeOwner == "" {
			return "", fmt.Errorf("invalid owner name: %s", opts.Owner)
		}
		repoPath = filepath.Join(m.reposDir, safeOwner, safeName)
	} else {
		repoPath = filepath.Join(m.reposDir, safeName)
	}

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
		_ = os.RemoveAll(repoPath)
		return "", fmt.Errorf("failed to initialize git repo: %w\n%s", err, output)
	}

	// Create initial commit if requested
	if opts.InitialCommit {
		if err := m.createInitialCommit(repoPath, safeName, opts.Description); err != nil {
			// Clean up on failure
			_ = os.RemoveAll(repoPath)
			return "", fmt.Errorf("failed to create initial commit: %w", err)
		}
	}

	return repoPath, nil
}

// Exists checks if a git repository exists at the given path.
// Detects both regular repos (with .git directory) and bare repos.
func (m *RepoManager) Exists(repoPath string) bool {
	// Regular repo: has .git directory or .git file (worktree)
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitDir)
	if err == nil && (info.IsDir() || info.Mode().IsRegular()) {
		return true
	}

	// Bare repo: has HEAD file directly in the directory
	return IsBareRepo(repoPath)
}

// IsBareRepo checks if the given path is a bare git repository.
// Bare repos have HEAD, objects/, and refs/ directly in the directory
// (no .git subdirectory). Forgejo stores repos this way.
func IsBareRepo(path string) bool {
	headPath := filepath.Join(path, "HEAD")
	if _, err := os.Stat(headPath); err != nil {
		return false
	}
	objectsPath := filepath.Join(path, "objects")
	if info, err := os.Stat(objectsPath); err != nil || !info.IsDir() {
		return false
	}
	refsPath := filepath.Join(path, "refs")
	if info, err := os.Stat(refsPath); err != nil || !info.IsDir() {
		return false
	}
	return true
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

// GetPath returns the full path for a repository name (without owner)
// For owner/repo paths, use GetPathWithOwner instead
func (m *RepoManager) GetPath(name string) string {
	safeName := sanitizeRepoName(name)
	return filepath.Join(m.reposDir, safeName)
}

// GetPathWithOwner returns the full path for a repository with owner/repo structure
func (m *RepoManager) GetPathWithOwner(owner, repo string) string {
	safeOwner := sanitizeRepoName(owner)
	safeRepo := sanitizeRepoName(repo)
	return filepath.Join(m.reposDir, safeOwner, safeRepo)
}

// GetReposDir returns the base repos directory
func (m *RepoManager) GetReposDir() string {
	return m.reposDir
}

// CloneOptions configures a clone operation
type CloneOptions struct {
	URL    string // Clone URL
	Owner  string // GitHub owner/org (for path: {reposDir}/{owner}/{repo})
	Name   string // Repository name (extracted from URL if empty)
}

// Clone clones a repository from a URL
// Deprecated: Use CloneWithOptions for owner/repo structure support
func (m *RepoManager) Clone(cloneURL, name string) (string, error) {
	return m.CloneWithOptions(CloneOptions{
		URL:  cloneURL,
		Name: name,
	})
}

// CloneWithOptions clones a repository with full options including owner/repo structure
func (m *RepoManager) CloneWithOptions(opts CloneOptions) (string, error) {
	name := opts.Name
	if name == "" {
		// Extract name from URL
		name = extractRepoNameFromURL(opts.URL)
	}
	if name == "" {
		return "", fmt.Errorf("could not determine repository name from URL")
	}

	safeName := sanitizeRepoName(name)

	// Determine path with optional owner
	var repoPath string
	if opts.Owner != "" {
		safeOwner := sanitizeRepoName(opts.Owner)
		if safeOwner == "" {
			return "", fmt.Errorf("invalid owner name: %s", opts.Owner)
		}
		repoPath = filepath.Join(m.reposDir, safeOwner, safeName)
	} else {
		repoPath = filepath.Join(m.reposDir, safeName)
	}

	// Check if repo already exists
	if m.Exists(repoPath) {
		return "", fmt.Errorf("repository already exists: %s", repoPath)
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(repoPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create repos directory: %w", err)
	}

	// Clone the repository
	cmd := exec.Command("git", "clone", opts.URL, repoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to clone repository: %w\n%s", err, output)
	}

	return repoPath, nil
}

// SetUpstream adds or updates the upstream remote (for fork workflows)
func (m *RepoManager) SetUpstream(repoPath, remoteURL string) error {
	if !m.Exists(repoPath) {
		return fmt.Errorf("repository does not exist: %s", repoPath)
	}

	// Check if upstream already exists
	cmd := exec.Command("git", "remote", "get-url", "upstream")
	cmd.Dir = repoPath
	if err := cmd.Run(); err == nil {
		// Upstream exists, update it
		cmd = exec.Command("git", "remote", "set-url", "upstream", remoteURL)
	} else {
		// Upstream doesn't exist, add it
		cmd = exec.Command("git", "remote", "add", "upstream", remoteURL)
	}
	cmd.Dir = repoPath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set upstream: %w\n%s", err, output)
	}

	return nil
}

// GetRemotes returns the origin and upstream remote URLs for a repository
func (m *RepoManager) GetRemotes(repoPath string) (origin, upstream string, err error) {
	if !m.Exists(repoPath) {
		return "", "", fmt.Errorf("repository does not exist: %s", repoPath)
	}

	// Get origin
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err == nil {
		origin = strings.TrimSpace(string(output))
	}

	// Get upstream (may not exist)
	cmd = exec.Command("git", "remote", "get-url", "upstream")
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err == nil {
		upstream = strings.TrimSpace(string(output))
	}

	return origin, upstream, nil
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

// ExtractOwnerRepoFromURL extracts the owner and repository name from a git URL
// Supports formats like:
//   - https://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
//   - ssh://git@github.com/owner/repo.git
func ExtractOwnerRepoFromURL(url string) (owner, repo string) {
	// Remove trailing .git
	url = strings.TrimSuffix(url, ".git")

	// Handle SSH format: git@github.com:owner/repo
	if strings.Contains(url, "@") && strings.Contains(url, ":") && !strings.Contains(url, "://") {
		// git@github.com:owner/repo
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			pathParts := strings.Split(parts[1], "/")
			if len(pathParts) >= 2 {
				return pathParts[len(pathParts)-2], pathParts[len(pathParts)-1]
			}
		}
		return "", ""
	}

	// Handle HTTPS/SSH URL format
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "ssh://")
	url = strings.TrimPrefix(url, "git://")

	// Remove host and possible user@
	parts := strings.Split(url, "/")
	// Skip host part (first element after removing protocol)
	if len(parts) >= 3 {
		// parts[0] = host, parts[1] = owner, parts[2] = repo
		return parts[len(parts)-2], parts[len(parts)-1]
	}

	return "", ""
}
