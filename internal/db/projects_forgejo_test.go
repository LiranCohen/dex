package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// setupTestDB creates a temporary database for testing and returns it along with a cleanup function.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "dex-projects-forgejo-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUpdateProjectGitProvider(t *testing.T) {
	db := setupTestDB(t)

	// Insert a project
	_, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-1', 'Test', '/test')`)
	if err != nil {
		t.Fatal(err)
	}

	// Update git provider fields
	if err := db.UpdateProjectGitProvider("proj-1", GitProviderForgejo, "myorg", "myrepo"); err != nil {
		t.Fatalf("UpdateProjectGitProvider() error = %v", err)
	}

	// Verify
	project, err := db.GetProjectByID("proj-1")
	if err != nil {
		t.Fatalf("GetProjectByID() error = %v", err)
	}

	if project.GetGitProvider() != GitProviderForgejo {
		t.Errorf("GetGitProvider() = %q, want %q", project.GetGitProvider(), GitProviderForgejo)
	}
	if project.GetOwner() != "myorg" {
		t.Errorf("GetOwner() = %q, want %q", project.GetOwner(), "myorg")
	}
	if project.GetRepo() != "myrepo" {
		t.Errorf("GetRepo() = %q, want %q", project.GetRepo(), "myrepo")
	}
}

func TestUpdateProjectGitProvider_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.UpdateProjectGitProvider("nonexistent", GitProviderForgejo, "org", "repo")
	if err == nil {
		t.Fatal("expected error for nonexistent project, got nil")
	}
}

func TestGetProjectByGitProvider(t *testing.T) {
	db := setupTestDB(t)

	// Insert a project with git provider fields
	_, err := db.Exec(
		`INSERT INTO projects (id, name, repo_path, git_provider, git_owner, git_repo)
		 VALUES ('proj-1', 'Forgejo Project', '/forgejo/repos/myorg/myrepo.git', 'forgejo', 'myorg', 'myrepo')`,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("found", func(t *testing.T) {
		project, err := db.GetProjectByGitProvider(GitProviderForgejo, "myorg", "myrepo")
		if err != nil {
			t.Fatalf("GetProjectByGitProvider() error = %v", err)
		}
		if project == nil {
			t.Fatal("expected project, got nil")
		}
		if project.ID != "proj-1" {
			t.Errorf("ID = %q, want %q", project.ID, "proj-1")
		}
		if !project.IsForgejo() {
			t.Error("IsForgejo() = false, want true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		project, err := db.GetProjectByGitProvider(GitProviderForgejo, "other", "repo")
		if err != nil {
			t.Fatalf("GetProjectByGitProvider() error = %v", err)
		}
		if project != nil {
			t.Error("expected nil for nonexistent provider/owner/repo")
		}
	})
}

func TestGetOrCreateProjectByForgejo(t *testing.T) {
	db := setupTestDB(t)

	repoPath := "/data/forgejo/repositories/myorg/myrepo.git"

	t.Run("creates new project", func(t *testing.T) {
		project, err := db.GetOrCreateProjectByForgejo("myorg", "myrepo", repoPath)
		if err != nil {
			t.Fatalf("GetOrCreateProjectByForgejo() error = %v", err)
		}
		if project == nil {
			t.Fatal("expected project, got nil")
		}
		if project.GetGitProvider() != GitProviderForgejo {
			t.Errorf("GetGitProvider() = %q, want %q", project.GetGitProvider(), GitProviderForgejo)
		}
		if project.GetOwner() != "myorg" {
			t.Errorf("GetOwner() = %q, want %q", project.GetOwner(), "myorg")
		}
		if project.GetRepo() != "myrepo" {
			t.Errorf("GetRepo() = %q, want %q", project.GetRepo(), "myrepo")
		}
		if project.RepoPath != repoPath {
			t.Errorf("RepoPath = %q, want %q", project.RepoPath, repoPath)
		}
	})

	t.Run("returns existing project", func(t *testing.T) {
		project2, err := db.GetOrCreateProjectByForgejo("myorg", "myrepo", repoPath)
		if err != nil {
			t.Fatalf("GetOrCreateProjectByForgejo() error = %v", err)
		}
		if project2 == nil {
			t.Fatal("expected project, got nil")
		}
		// Should be the same project
		project1, _ := db.GetProjectByGitProvider(GitProviderForgejo, "myorg", "myrepo")
		if project2.ID != project1.ID {
			t.Errorf("second call returned different project: got %q, want %q", project2.ID, project1.ID)
		}
	})

	t.Run("upgrades existing project by repo path", func(t *testing.T) {
		// Insert a project with matching repo_path but no git_provider
		_, err := db.Exec(
			`INSERT INTO projects (id, name, repo_path)
			 VALUES ('proj-legacy', 'Legacy', '/data/forgejo/repositories/legacy/proj.git')`,
		)
		if err != nil {
			t.Fatal(err)
		}

		project, err := db.GetOrCreateProjectByForgejo("legacy", "proj", "/data/forgejo/repositories/legacy/proj.git")
		if err != nil {
			t.Fatalf("GetOrCreateProjectByForgejo() error = %v", err)
		}
		if project.ID != "proj-legacy" {
			t.Errorf("ID = %q, want %q (should reuse existing)", project.ID, "proj-legacy")
		}
		if project.GetGitProvider() != GitProviderForgejo {
			t.Errorf("GetGitProvider() = %q, want %q (should have been upgraded)", project.GetGitProvider(), GitProviderForgejo)
		}
	})
}

func TestListForgejoProjects(t *testing.T) {
	db := setupTestDB(t)

	// Insert a mix of Forgejo and GitHub projects
	_, err := db.Exec(
		`INSERT INTO projects (id, name, repo_path, git_provider, git_owner, git_repo)
		 VALUES ('proj-f1', 'FP1', '/f1', 'forgejo', 'org', 'repo1'),
		        ('proj-f2', 'FP2', '/f2', 'forgejo', 'org', 'repo2'),
		        ('proj-gh', 'GHP', '/gh', 'github', 'ghorg', 'ghrepo')`,
	)
	if err != nil {
		t.Fatal(err)
	}

	projects, err := db.ListForgejoProjects()
	if err != nil {
		t.Fatalf("ListForgejoProjects() error = %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("ListForgejoProjects() returned %d projects, want 2", len(projects))
	}

	for _, p := range projects {
		if !p.IsForgejo() {
			t.Errorf("project %q: IsForgejo() = false, want true", p.ID)
		}
	}
}

func TestProjectModelHelpers(t *testing.T) {
	t.Run("IsForgejo", func(t *testing.T) {
		tests := []struct {
			name     string
			project  Project
			expected bool
		}{
			{"forgejo provider", Project{GitProvider: nullStr(GitProviderForgejo)}, true},
			{"github provider", Project{GitProvider: nullStr(GitProviderGitHub)}, false},
			{"empty provider defaults to github", Project{}, false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := tc.project.IsForgejo(); got != tc.expected {
					t.Errorf("IsForgejo() = %v, want %v", got, tc.expected)
				}
			})
		}
	})

	t.Run("GetOwner prefers GitOwner over GitHubOwner", func(t *testing.T) {
		p := Project{
			GitOwner:    nullStr("forgejo-org"),
			GitHubOwner: nullStr("github-org"),
		}
		if got := p.GetOwner(); got != "forgejo-org" {
			t.Errorf("GetOwner() = %q, want %q", got, "forgejo-org")
		}
	})

	t.Run("GetOwner falls back to GitHubOwner", func(t *testing.T) {
		p := Project{
			GitHubOwner: nullStr("github-org"),
		}
		if got := p.GetOwner(); got != "github-org" {
			t.Errorf("GetOwner() = %q, want %q", got, "github-org")
		}
	})

	t.Run("GetRepo prefers GitRepo over GitHubRepo", func(t *testing.T) {
		p := Project{
			GitRepo:    nullStr("forgejo-repo"),
			GitHubRepo: nullStr("github-repo"),
		}
		if got := p.GetRepo(); got != "forgejo-repo" {
			t.Errorf("GetRepo() = %q, want %q", got, "forgejo-repo")
		}
	})

	t.Run("GetGitProvider defaults to github", func(t *testing.T) {
		p := Project{}
		if got := p.GetGitProvider(); got != GitProviderGitHub {
			t.Errorf("GetGitProvider() = %q, want %q", got, GitProviderGitHub)
		}
	})
}
