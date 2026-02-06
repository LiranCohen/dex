package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCloneURL(t *testing.T) {
	tests := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
	}{
		{
			url:           "https://github.com/lirancohen/dex.git",
			expectedOwner: "lirancohen",
			expectedRepo:  "dex",
		},
		{
			url:           "https://github.com/lirancohen/dex",
			expectedOwner: "lirancohen",
			expectedRepo:  "dex",
		},
		{
			url:           "git@github.com:lirancohen/dex.git",
			expectedOwner: "lirancohen",
			expectedRepo:  "dex",
		},
		{
			url:           "git@github.com:lirancohen/dex",
			expectedOwner: "lirancohen",
			expectedRepo:  "dex",
		},
		{
			url:           "https://github.com/org/repo-name.git",
			expectedOwner: "org",
			expectedRepo:  "repo-name",
		},
		{
			url:           "",
			expectedOwner: "",
			expectedRepo:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			owner, repo := parseCloneURL(tc.url)
			if owner != tc.expectedOwner {
				t.Errorf("expected owner %q, got %q", tc.expectedOwner, owner)
			}
			if repo != tc.expectedRepo {
				t.Errorf("expected repo %q, got %q", tc.expectedRepo, repo)
			}
		})
	}
}

func TestSetupAuthenticatedCloneURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		token    string
		expected string
	}{
		{
			name:     "HTTPS URL with token",
			url:      "https://github.com/lirancohen/dex.git",
			token:    "ghp_test123",
			expected: "https://x-access-token:ghp_test123@github.com/lirancohen/dex.git",
		},
		{
			name:     "HTTPS URL without token",
			url:      "https://github.com/lirancohen/dex.git",
			token:    "",
			expected: "https://github.com/lirancohen/dex.git",
		},
		{
			name:     "SSH URL unchanged",
			url:      "git@github.com:lirancohen/dex.git",
			token:    "ghp_test123",
			expected: "git@github.com:lirancohen/dex.git",
		},
		{
			name:     "Non-GitHub HTTPS URL unchanged",
			url:      "https://gitlab.com/org/repo.git",
			token:    "ghp_test123",
			expected: "https://gitlab.com/org/repo.git",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SetupAuthenticatedCloneURL(tc.url, tc.token)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestProjectManager_getProjectDir(t *testing.T) {
	pm := NewProjectManager("/data")

	tests := []struct {
		name     string
		project  Project
		expected string
	}{
		{
			name: "With owner and repo",
			project: Project{
				ID:          "proj-123",
				GitHubOwner: "lirancohen",
				GitHubRepo:  "dex",
			},
			expected: "/data/projects/lirancohen/dex",
		},
		{
			name: "Without owner/repo, parse from clone URL",
			project: Project{
				ID:       "proj-123",
				CloneURL: "https://github.com/org/myrepo.git",
			},
			expected: "/data/projects/org/myrepo",
		},
		{
			name: "No info available, use ID",
			project: Project{
				ID: "proj-123",
			},
			expected: "/data/projects/unknown/proj-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := pm.getProjectDir(tc.project)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestProjectManager_projectExists(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "project-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pm := NewProjectManager(tmpDir)

	// Non-existent directory
	if pm.projectExists("/nonexistent/path") {
		t.Error("expected false for non-existent path")
	}

	// Directory without .git
	noGitDir := filepath.Join(tmpDir, "no-git")
	if err := os.MkdirAll(noGitDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if pm.projectExists(noGitDir) {
		t.Error("expected false for directory without .git")
	}

	// Directory with .git
	gitDir := filepath.Join(tmpDir, "with-git")
	if err := os.MkdirAll(filepath.Join(gitDir, ".git"), 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	if !pm.projectExists(gitDir) {
		t.Error("expected true for directory with .git")
	}
}

func TestProjectManager_Cleanup(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "project-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pm := NewProjectManager(tmpDir)

	// Create a project directory to clean up
	projectDir := filepath.Join(tmpDir, "projects", "owner", "repo")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Cleanup should work
	if err := pm.Cleanup(projectDir); err != nil {
		t.Errorf("expected cleanup to succeed: %v", err)
	}

	// Directory should be gone
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Error("expected project directory to be removed")
	}
}

func TestProjectManager_Cleanup_Safety(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "project-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pm := NewProjectManager(tmpDir)

	// Should refuse to clean up directories outside data directory
	outsideDir := "/tmp/outside-data-dir"
	if err := pm.Cleanup(outsideDir); err == nil {
		t.Error("expected cleanup to fail for directory outside data dir")
	}

	// Empty path should be OK (no-op)
	if err := pm.Cleanup(""); err != nil {
		t.Errorf("expected cleanup of empty path to succeed: %v", err)
	}
}
