// Package workspace manages the Dex workspace repository
package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lirancohen/dex/internal/toolbelt"
)

// Service manages the dex-workspace repository on GitHub
type Service struct {
	github    *toolbelt.GitHubClient
	repoOwner string // authenticated user
	repoName  string // "dex-workspace"
	localPath string // local path to the workspace repo
}

// NewService creates a new workspace Service
func NewService(github *toolbelt.GitHubClient, localPath string) *Service {
	return &Service{
		github:    github,
		repoName:  "dex-workspace",
		localPath: localPath,
	}
}

// EnsureRemoteExists creates the dex-workspace repo on GitHub if needed
// and sets up the remote on the local repository
func (s *Service) EnsureRemoteExists(ctx context.Context) error {
	fmt.Printf("workspace.Service: checking for GitHub workspace\n")

	// Get the owner (works for both PAT and GitHub App tokens)
	owner, err := s.github.GetOwner(ctx)
	if err != nil {
		return fmt.Errorf("failed to get owner: %w", err)
	}
	s.repoOwner = owner

	fmt.Printf("workspace.Service: using owner %s\n", s.repoOwner)

	// Check if the dex-workspace repo exists, create if not
	repo, err := s.github.EnsureRepo(ctx, toolbelt.CreateRepoOptions{
		Name:        s.repoName,
		Description: "Dex workspace - managed by Dex",
		Private:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure workspace repo: %w", err)
	}

	fmt.Printf("workspace.Service: workspace repo ready at %s\n", repo.GetHTMLURL())

	// Check if local repo exists
	if _, err := os.Stat(filepath.Join(s.localPath, ".git")); os.IsNotExist(err) {
		fmt.Printf("workspace.Service: no local repo at %s, skipping remote setup\n", s.localPath)
		return nil
	}

	// Set up remote on local repo
	cloneURL := repo.GetCloneURL()
	if cloneURL == "" {
		cloneURL = repo.GetSSHURL()
	}

	if cloneURL != "" {
		if err := s.setRemote(cloneURL); err != nil {
			return fmt.Errorf("failed to set remote: %w", err)
		}
		fmt.Printf("workspace.Service: remote origin set to %s\n", cloneURL)
	}

	return nil
}

// setRemote sets the origin remote on the local repository
func (s *Service) setRemote(url string) error {
	// Check if origin already exists
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = s.localPath
	if err := cmd.Run(); err == nil {
		// Origin exists, update it
		cmd = exec.Command("git", "remote", "set-url", "origin", url)
		cmd.Dir = s.localPath
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git remote set-url failed: %w\n%s", err, output)
		}
	} else {
		// Origin doesn't exist, add it
		cmd = exec.Command("git", "remote", "add", "origin", url)
		cmd.Dir = s.localPath
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git remote add failed: %w\n%s", err, output)
		}
	}

	return nil
}

// Sync pushes local changes to GitHub
func (s *Service) Sync(ctx context.Context) error {
	// Check if there are any commits to push
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = s.localPath
	if err := cmd.Run(); err != nil {
		// No commits yet, nothing to sync
		return nil
	}

	// Push to origin
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = s.localPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %w\n%s", err, output)
	}

	fmt.Printf("workspace.Service: synced to GitHub\n")
	return nil
}

// GetRemoteURL returns the GitHub URL for the workspace
func (s *Service) GetRemoteURL() string {
	if s.repoOwner == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s", s.repoOwner, s.repoName)
}

// GetRepoOwner returns the authenticated user (repo owner)
func (s *Service) GetRepoOwner() string {
	return s.repoOwner
}

// GetRepoName returns the workspace repo name
func (s *Service) GetRepoName() string {
	return s.repoName
}
