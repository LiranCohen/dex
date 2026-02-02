package git

import (
	"fmt"
	"path/filepath"

	"github.com/lirancohen/dex/internal/db"
)

// Service coordinates git operations with database records
type Service struct {
	db         *db.DB
	worktrees  *WorktreeManager
	operations *Operations
	repos      *RepoManager
}

// NewService creates a git service
func NewService(database *db.DB, worktreeBase, reposBase string) *Service {
	s := &Service{
		db:         database,
		worktrees:  NewWorktreeManager(worktreeBase),
		operations: NewOperations(),
	}
	if reposBase != "" {
		s.repos = NewRepoManager(reposBase)
	}
	return s
}

// SetupTaskWorktree creates a worktree for a task and updates the task record
// projectPath: path to the main repo
// taskID: the task to setup
// baseBranch: branch to base the worktree on (e.g., "main")
func (s *Service) SetupTaskWorktree(projectPath, taskID, baseBranch string) (string, error) {
	// Get the task to extract its short ID
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return "", fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	// Extract short ID from task ID (e.g., "task-abc123" -> "abc123")
	shortID := taskID
	if len(taskID) > 5 && taskID[:5] == "task-" {
		shortID = taskID[5:]
	}

	// Create worktree
	worktreePath, err := s.worktrees.Create(projectPath, shortID, baseBranch)
	if err != nil {
		return "", fmt.Errorf("failed to create worktree: %w", err)
	}

	// Build branch name (matches worktree.go logic)
	branchName := fmt.Sprintf("task/task-%s", shortID)

	// Update task record with worktree info
	if err := s.db.UpdateTaskWorktree(taskID, worktreePath, branchName); err != nil {
		// Try to clean up the worktree we just created
		_ = s.worktrees.Remove(projectPath, worktreePath, true, false)
		return "", fmt.Errorf("failed to update task worktree: %w", err)
	}

	return worktreePath, nil
}

// CleanupTaskWorktree removes the worktree for a task
// cleanupBranch: if true, also delete the task branch
func (s *Service) CleanupTaskWorktree(projectPath, taskID string, cleanupBranch bool) error {
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if !task.WorktreePath.Valid || task.WorktreePath.String == "" {
		return fmt.Errorf("task has no worktree: %s", taskID)
	}

	// Remove the worktree
	if err := s.worktrees.Remove(projectPath, task.WorktreePath.String, true, cleanupBranch); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	// Clear worktree info from task record
	if err := s.db.UpdateTaskWorktree(taskID, "", ""); err != nil {
		return fmt.Errorf("failed to clear task worktree: %w", err)
	}

	return nil
}

// GetTaskWorktreeStatus returns the git status of a task's worktree
func (s *Service) GetTaskWorktreeStatus(taskID string) (*GitStatus, error) {
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if !task.WorktreePath.Valid || task.WorktreePath.String == "" {
		return nil, fmt.Errorf("task has no worktree: %s", taskID)
	}

	return s.worktrees.GetStatus(task.WorktreePath.String)
}

// ListWorktrees returns all worktrees for a project
func (s *Service) ListWorktrees(projectPath string) ([]WorktreeInfo, error) {
	return s.worktrees.List(projectPath)
}

// GetWorktreePath returns the expected worktree path for a project and task
func (s *Service) GetWorktreePath(projectPath, taskID string) string {
	// Extract short ID from task ID (e.g., "task-abc123" -> "abc123")
	shortID := taskID
	if len(taskID) > 5 && taskID[:5] == "task-" {
		shortID = taskID[5:]
	}
	projectName := filepath.Base(projectPath)
	return s.worktrees.GetWorktreePath(projectName, shortID)
}

// WorktreeExists checks if a worktree exists at the expected path
func (s *Service) WorktreeExists(worktreePath string) bool {
	return s.worktrees.Exists(worktreePath)
}

// Operations returns the git operations helper for direct git commands
func (s *Service) Operations() *Operations {
	return s.operations
}

// CreateRepo creates a new git repository
func (s *Service) CreateRepo(opts CreateOptions) (string, error) {
	if s.repos == nil {
		return "", fmt.Errorf("repository manager not configured")
	}
	return s.repos.Create(opts)
}

// RepoExists checks if a git repository exists at the given path
func (s *Service) RepoExists(repoPath string) bool {
	if s.repos == nil {
		return false
	}
	return s.repos.Exists(repoPath)
}

// SetRepoRemote adds or updates the origin remote on a repository
func (s *Service) SetRepoRemote(repoPath, remoteURL string) error {
	if s.repos == nil {
		return fmt.Errorf("repository manager not configured")
	}
	return s.repos.SetRemote(repoPath, remoteURL)
}

// CloneRepo clones a repository from a URL
func (s *Service) CloneRepo(cloneURL, name string) (string, error) {
	if s.repos == nil {
		return "", fmt.Errorf("repository manager not configured")
	}
	return s.repos.Clone(cloneURL, name)
}

// GetRepoPath returns the full path for a repository name
func (s *Service) GetRepoPath(name string) string {
	if s.repos == nil {
		return ""
	}
	return s.repos.GetPath(name)
}

// GetRepoPathWithOwner returns the full path for a repository with owner/repo structure
func (s *Service) GetRepoPathWithOwner(owner, repo string) string {
	if s.repos == nil {
		return ""
	}
	return s.repos.GetPathWithOwner(owner, repo)
}

// CloneRepoWithOptions clones a repository with full options including owner/repo structure
func (s *Service) CloneRepoWithOptions(opts CloneOptions) (string, error) {
	if s.repos == nil {
		return "", fmt.Errorf("repository manager not configured")
	}
	return s.repos.CloneWithOptions(opts)
}

// SetRepoUpstream adds or updates the upstream remote for fork workflows
func (s *Service) SetRepoUpstream(repoPath, remoteURL string) error {
	if s.repos == nil {
		return fmt.Errorf("repository manager not configured")
	}
	return s.repos.SetUpstream(repoPath, remoteURL)
}

// GetRepoRemotes returns the origin and upstream remote URLs for a repository
func (s *Service) GetRepoRemotes(repoPath string) (origin, upstream string, err error) {
	if s.repos == nil {
		return "", "", fmt.Errorf("repository manager not configured")
	}
	return s.repos.GetRemotes(repoPath)
}

// GetReposDir returns the base repos directory
func (s *Service) GetReposDir() string {
	if s.repos == nil {
		return ""
	}
	return s.repos.GetReposDir()
}

// GetWorktreeBase returns the base directory for worktrees
func (s *Service) GetWorktreeBase() string {
	if s.worktrees == nil {
		return ""
	}
	return s.worktrees.GetWorktreeBase()
}

// CommitTaskContent stages and commits task content files in a directory
func (s *Service) CommitTaskContent(dir, taskID, message string) (string, error) {
	return s.operations.CommitTaskContent(dir, CommitContentOptions{
		TaskID:  taskID,
		Message: message,
	})
}

// CommitQuestContent stages and commits quest content files in a directory
func (s *Service) CommitQuestContent(dir, questID, message string) (string, error) {
	return s.operations.CommitQuestContent(dir, questID, message)
}
