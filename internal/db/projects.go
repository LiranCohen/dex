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
		`SELECT id, name, repo_path, github_owner, github_repo, default_branch, services, created_at
		 FROM projects WHERE id = ?`,
		id,
	).Scan(
		&project.ID, &project.Name, &project.RepoPath,
		&project.GitHubOwner, &project.GitHubRepo,
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
		`SELECT id, name, repo_path, github_owner, github_repo, default_branch, services, created_at
		 FROM projects ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		project := &Project{}
		var servicesJSON sql.NullString

		err := rows.Scan(
			&project.ID, &project.Name, &project.RepoPath,
			&project.GitHubOwner, &project.GitHubRepo,
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
		`SELECT id, name, repo_path, github_owner, github_repo, default_branch, services, created_at
		 FROM projects WHERE repo_path = ?`,
		repoPath,
	).Scan(
		&project.ID, &project.Name, &project.RepoPath,
		&project.GitHubOwner, &project.GitHubRepo,
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
