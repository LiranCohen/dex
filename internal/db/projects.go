// Package db provides SQLite database access for Poindexter
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CreateProject inserts a new project into the database
func (db *DB) CreateProject(name, repoPath string) (*Project, error) {
	return db.CreateProjectWithID(NewPrefixedID("proj"), name, repoPath)
}

// CreateProjectWithID inserts a new project with a specific ID
func (db *DB) CreateProjectWithID(id, name, repoPath string) (*Project, error) {
	project := &Project{
		ID:            id,
		Name:          name,
		RepoPath:      repoPath,
		DefaultBranch: "main",
		Services:      ProjectServices{},
		CreatedAt:     time.Now(),
	}

	servicesJSON, err := json.Marshal(project.Services)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal services: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO projects (id, name, repo_path, default_branch, services, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		project.ID, project.Name, project.RepoPath, project.DefaultBranch, string(servicesJSON), project.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return project, nil
}

// GetProjectByID retrieves a project by its ID
func (db *DB) GetProjectByID(id string) (*Project, error) {
	project := &Project{}
	var servicesJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, name, repo_path, github_owner, github_repo, git_provider, git_owner, git_repo, remote_origin, remote_upstream, default_branch, services, created_at
		 FROM projects WHERE id = ?`,
		id,
	).Scan(
		&project.ID, &project.Name, &project.RepoPath,
		&project.GitHubOwner, &project.GitHubRepo,
		&project.GitProvider, &project.GitOwner, &project.GitRepo,
		&project.RemoteOrigin, &project.RemoteUpstream,
		&project.DefaultBranch, &servicesJSON, &project.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if servicesJSON.Valid && servicesJSON.String != "" {
		if err := json.Unmarshal([]byte(servicesJSON.String), &project.Services); err != nil {
			return nil, fmt.Errorf("failed to unmarshal services: %w", err)
		}
	}

	return project, nil
}

// ListProjects returns all projects
func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.Query(
		`SELECT id, name, repo_path, github_owner, github_repo, git_provider, git_owner, git_repo, remote_origin, remote_upstream, default_branch, services, created_at
		 FROM projects ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projects []*Project
	for rows.Next() {
		project := &Project{}
		var servicesJSON sql.NullString

		err := rows.Scan(
			&project.ID, &project.Name, &project.RepoPath,
			&project.GitHubOwner, &project.GitHubRepo,
			&project.GitProvider, &project.GitOwner, &project.GitRepo,
			&project.RemoteOrigin, &project.RemoteUpstream,
			&project.DefaultBranch, &servicesJSON, &project.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}

		if servicesJSON.Valid && servicesJSON.String != "" {
			if err := json.Unmarshal([]byte(servicesJSON.String), &project.Services); err != nil {
				return nil, fmt.Errorf("failed to unmarshal services: %w", err)
			}
		}

		projects = append(projects, project)
	}

	return projects, nil
}

// UpdateProject updates a project's basic fields
func (db *DB) UpdateProject(id string, name, repoPath, defaultBranch string) error {
	result, err := db.Exec(
		`UPDATE projects SET name = ?, repo_path = ?, default_branch = ? WHERE id = ?`,
		name, repoPath, defaultBranch, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}

	return nil
}

// UpdateProjectGitHub sets the GitHub owner and repo for a project
func (db *DB) UpdateProjectGitHub(id, owner, repo string) error {
	result, err := db.Exec(
		`UPDATE projects SET github_owner = ?, github_repo = ? WHERE id = ?`,
		owner, repo, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update project github: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}

	return nil
}

// UpdateProjectServices updates the toolbelt services configuration for a project
func (db *DB) UpdateProjectServices(id string, services ProjectServices) error {
	servicesJSON, err := json.Marshal(services)
	if err != nil {
		return fmt.Errorf("failed to marshal services: %w", err)
	}

	result, err := db.Exec(
		`UPDATE projects SET services = ? WHERE id = ?`,
		string(servicesJSON), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update project services: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}

	return nil
}

// UpdateProjectRemotes sets the origin and upstream remote URLs for a project
func (db *DB) UpdateProjectRemotes(id string, origin, upstream string) error {
	var originVal, upstreamVal sql.NullString
	if origin != "" {
		originVal = sql.NullString{String: origin, Valid: true}
	}
	if upstream != "" {
		upstreamVal = sql.NullString{String: upstream, Valid: true}
	}

	result, err := db.Exec(
		`UPDATE projects SET remote_origin = ?, remote_upstream = ? WHERE id = ?`,
		originVal, upstreamVal, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update project remotes: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}

	return nil
}

// DeleteProject removes a project from the database
func (db *DB) DeleteProject(id string) error {
	result, err := db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}

	return nil
}

// GetOrCreateDefaultProject returns the default project, creating it if it doesn't exist
func (db *DB) GetOrCreateDefaultProject() (*Project, error) {
	// Try to get the first project
	projects, err := db.ListProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) > 0 {
		return projects[0], nil
	}

	// Create a default project
	return db.CreateProject("Default Project", ".")
}

// GetProjectByRepoPath retrieves a project by its local repository path
func (db *DB) GetProjectByRepoPath(repoPath string) (*Project, error) {
	project := &Project{}
	var servicesJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, name, repo_path, github_owner, github_repo, git_provider, git_owner, git_repo, remote_origin, remote_upstream, default_branch, services, created_at
		 FROM projects WHERE repo_path = ?`,
		repoPath,
	).Scan(
		&project.ID, &project.Name, &project.RepoPath,
		&project.GitHubOwner, &project.GitHubRepo,
		&project.GitProvider, &project.GitOwner, &project.GitRepo,
		&project.RemoteOrigin, &project.RemoteUpstream,
		&project.DefaultBranch, &servicesJSON, &project.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project by repo path: %w", err)
	}

	if servicesJSON.Valid && servicesJSON.String != "" {
		if err := json.Unmarshal([]byte(servicesJSON.String), &project.Services); err != nil {
			return nil, fmt.Errorf("failed to unmarshal services: %w", err)
		}
	}

	return project, nil
}

// GetProjectByGitHub retrieves a project by its GitHub owner and repo name
func (db *DB) GetProjectByGitHub(owner, repo string) (*Project, error) {
	project := &Project{}
	var servicesJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, name, repo_path, github_owner, github_repo, git_provider, git_owner, git_repo, remote_origin, remote_upstream, default_branch, services, created_at
		 FROM projects WHERE github_owner = ? AND github_repo = ?`,
		owner, repo,
	).Scan(
		&project.ID, &project.Name, &project.RepoPath,
		&project.GitHubOwner, &project.GitHubRepo,
		&project.GitProvider, &project.GitOwner, &project.GitRepo,
		&project.RemoteOrigin, &project.RemoteUpstream,
		&project.DefaultBranch, &servicesJSON, &project.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project by github: %w", err)
	}

	if servicesJSON.Valid && servicesJSON.String != "" {
		if err := json.Unmarshal([]byte(servicesJSON.String), &project.Services); err != nil {
			return nil, fmt.Errorf("failed to unmarshal services: %w", err)
		}
	}

	return project, nil
}

// GetOrCreateProjectByGitHub finds an existing project by GitHub owner/repo or creates a new one
// The repoPath should be the full path where the repo will live (e.g., /opt/dex/repos/owner/repo)
func (db *DB) GetOrCreateProjectByGitHub(owner, repo, repoPath string) (*Project, error) {
	// First try to find by GitHub owner/repo
	project, err := db.GetProjectByGitHub(owner, repo)
	if err != nil {
		return nil, err
	}
	if project != nil {
		return project, nil
	}

	// Also check by repo path in case it exists with different github fields
	project, err = db.GetProjectByRepoPath(repoPath)
	if err != nil {
		return nil, err
	}
	if project != nil {
		// Update the GitHub fields if they're not set
		if !project.GitHubOwner.Valid || project.GitHubOwner.String == "" {
			if err := db.UpdateProjectGitHub(project.ID, owner, repo); err != nil {
				return nil, fmt.Errorf("failed to update project github info: %w", err)
			}
			project.GitHubOwner = sql.NullString{String: owner, Valid: true}
			project.GitHubRepo = sql.NullString{String: repo, Valid: true}
		}
		return project, nil
	}

	// Create new project with GitHub info
	projectName := fmt.Sprintf("%s/%s", owner, repo)
	project, err = db.CreateProject(projectName, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	// Set GitHub fields
	if err := db.UpdateProjectGitHub(project.ID, owner, repo); err != nil {
		return nil, fmt.Errorf("failed to set project github info: %w", err)
	}
	project.GitHubOwner = sql.NullString{String: owner, Valid: true}
	project.GitHubRepo = sql.NullString{String: repo, Valid: true}

	return project, nil
}

// UpdateProjectGitProvider sets the git provider, owner, and repo for a project
func (db *DB) UpdateProjectGitProvider(id, provider, owner, repo string) error {
	result, err := db.Exec(
		`UPDATE projects SET git_provider = ?, git_owner = ?, git_repo = ? WHERE id = ?`,
		provider, owner, repo, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update project git provider: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}

	return nil
}

// GetProjectByGitProvider retrieves a project by its git provider, owner, and repo
func (db *DB) GetProjectByGitProvider(provider, owner, repo string) (*Project, error) {
	project := &Project{}
	var servicesJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, name, repo_path, github_owner, github_repo, git_provider, git_owner, git_repo, remote_origin, remote_upstream, default_branch, services, created_at
		 FROM projects WHERE git_provider = ? AND git_owner = ? AND git_repo = ?`,
		provider, owner, repo,
	).Scan(
		&project.ID, &project.Name, &project.RepoPath,
		&project.GitHubOwner, &project.GitHubRepo,
		&project.GitProvider, &project.GitOwner, &project.GitRepo,
		&project.RemoteOrigin, &project.RemoteUpstream,
		&project.DefaultBranch, &servicesJSON, &project.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project by git provider: %w", err)
	}

	if servicesJSON.Valid && servicesJSON.String != "" {
		if err := json.Unmarshal([]byte(servicesJSON.String), &project.Services); err != nil {
			return nil, fmt.Errorf("failed to unmarshal services: %w", err)
		}
	}

	return project, nil
}

// GetOrCreateProjectByForgejo finds an existing project by Forgejo owner/repo or creates a new one.
// The repoPath should be the bare repo path under Forgejo's repositories directory.
func (db *DB) GetOrCreateProjectByForgejo(owner, repo, repoPath string) (*Project, error) {
	// First try to find by provider-agnostic fields
	project, err := db.GetProjectByGitProvider(GitProviderForgejo, owner, repo)
	if err != nil {
		return nil, err
	}
	if project != nil {
		return project, nil
	}

	// Also check by repo path in case it exists with different fields
	project, err = db.GetProjectByRepoPath(repoPath)
	if err != nil {
		return nil, err
	}
	if project != nil {
		// Update the git provider fields if they're not set
		if !project.GitProvider.Valid || project.GitProvider.String != GitProviderForgejo {
			if err := db.UpdateProjectGitProvider(project.ID, GitProviderForgejo, owner, repo); err != nil {
				return nil, fmt.Errorf("failed to update project git provider info: %w", err)
			}
			project.GitProvider = sql.NullString{String: GitProviderForgejo, Valid: true}
			project.GitOwner = sql.NullString{String: owner, Valid: true}
			project.GitRepo = sql.NullString{String: repo, Valid: true}
		}
		return project, nil
	}

	// Create new project with Forgejo info
	projectName := fmt.Sprintf("%s/%s", owner, repo)
	project, err = db.CreateProject(projectName, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	// Set git provider fields
	if err := db.UpdateProjectGitProvider(project.ID, GitProviderForgejo, owner, repo); err != nil {
		return nil, fmt.Errorf("failed to set project git provider info: %w", err)
	}
	project.GitProvider = sql.NullString{String: GitProviderForgejo, Valid: true}
	project.GitOwner = sql.NullString{String: owner, Valid: true}
	project.GitRepo = sql.NullString{String: repo, Valid: true}

	return project, nil
}

// ListForgejoProjects returns all projects that use the Forgejo git provider
func (db *DB) ListForgejoProjects() ([]*Project, error) {
	rows, err := db.Query(
		`SELECT id, name, repo_path, github_owner, github_repo, git_provider, git_owner, git_repo, remote_origin, remote_upstream, default_branch, services, created_at
		 FROM projects WHERE git_provider = ? ORDER BY created_at DESC`,
		GitProviderForgejo,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list forgejo projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projects []*Project
	for rows.Next() {
		project := &Project{}
		var servicesJSON sql.NullString

		err := rows.Scan(
			&project.ID, &project.Name, &project.RepoPath,
			&project.GitHubOwner, &project.GitHubRepo,
			&project.GitProvider, &project.GitOwner, &project.GitRepo,
			&project.RemoteOrigin, &project.RemoteUpstream,
			&project.DefaultBranch, &servicesJSON, &project.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}

		if servicesJSON.Valid && servicesJSON.String != "" {
			if err := json.Unmarshal([]byte(servicesJSON.String), &project.Services); err != nil {
				return nil, fmt.Errorf("failed to unmarshal services: %w", err)
			}
		}

		projects = append(projects, project)
	}

	return projects, nil
}
