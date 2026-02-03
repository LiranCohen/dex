// Package projects provides HTTP handlers for project operations.
package projects

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/git"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// Handler handles project-related HTTP requests.
type Handler struct {
	deps *core.Deps
}

// New creates a new projects handler.
func New(deps *core.Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterRoutes registers all project routes on the given group.
// All routes require authentication.
//   - GET /projects
//   - POST /projects
//   - GET /projects/:id
//   - PUT /projects/:id
//   - DELETE /projects/:id
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/projects", h.HandleList)
	g.POST("/projects", h.HandleCreate)
	g.GET("/projects/:id", h.HandleGet)
	g.PUT("/projects/:id", h.HandleUpdate)
	g.DELETE("/projects/:id", h.HandleDelete)
}

// HandleList returns all projects.
// GET /api/v1/projects
func (h *Handler) HandleList(c echo.Context) error {
	projects, err := h.deps.DB.ListProjects()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"projects": projects,
		"count":    len(projects),
	})
}

// HandleCreate creates a new project.
// POST /api/v1/projects
func (h *Handler) HandleCreate(c echo.Context) error {
	var req struct {
		Name string `json:"name"`

		// Option 1: Use existing repo
		RepoPath string `json:"repo_path,omitempty"`

		// Option 2: Create new repo
		CreateRepo bool `json:"create_repo,omitempty"`

		// Option 3: Clone from URL
		CloneURL string `json:"clone_url,omitempty"`

		// GitHub options (when create_repo=true)
		GitHubCreate  bool `json:"github_create,omitempty"`
		GitHubPrivate bool `json:"github_private,omitempty"`

		Description string `json:"description,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	var repoPath string

	if req.CreateRepo {
		// Create new local repository
		if h.deps.GitService == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
		}

		var err error
		repoPath, err = h.deps.GitService.CreateRepo(git.CreateOptions{
			Name:          req.Name,
			Description:   req.Description,
			InitialCommit: true,
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create repository: %v", err))
		}

		// Optionally create on GitHub
		tb := h.deps.GetToolbelt()
		if req.GitHubCreate && tb != nil && tb.GitHub != nil {
			ghRepo, err := tb.GitHub.CreateRepo(c.Request().Context(), toolbelt.CreateRepoOptions{
				Name:        req.Name,
				Description: req.Description,
				Private:     req.GitHubPrivate,
			})
			if err != nil {
				// Log but don't fail - local repo was created successfully
				fmt.Printf("warning: failed to create GitHub repo: %v\n", err)
			} else if ghRepo != nil && ghRepo.CloneURL != nil && *ghRepo.CloneURL != "" {
				if err := h.deps.GitService.SetRepoRemote(repoPath, *ghRepo.CloneURL); err != nil {
					fmt.Printf("warning: failed to set remote: %v\n", err)
				}
			}
		}
	} else if req.CloneURL != "" {
		// Clone from URL
		if h.deps.GitService == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "git service not configured")
		}

		var err error
		repoPath, err = h.deps.GitService.CloneRepo(req.CloneURL, req.Name)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to clone repository: %v", err))
		}
	} else {
		// Use existing repo path
		if req.RepoPath == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "repo_path is required when not creating or cloning a repo")
		}
		repoPath = req.RepoPath
	}

	project, err := h.deps.DB.CreateProject(req.Name, repoPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, project)
}

// HandleGet returns a single project by ID.
// GET /api/v1/projects/:id
func (h *Handler) HandleGet(c echo.Context) error {
	id := c.Param("id")

	project, err := h.deps.DB.GetProjectByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if project == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	return c.JSON(http.StatusOK, project)
}

// HandleUpdate updates a project.
// PUT /api/v1/projects/:id
func (h *Handler) HandleUpdate(c echo.Context) error {
	id := c.Param("id")

	// First fetch the existing project to get current values
	existing, err := h.deps.DB.GetProjectByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "project not found")
	}

	var req struct {
		Name          *string             `json:"name"`
		RepoPath      *string             `json:"repo_path"`
		DefaultBranch *string             `json:"default_branch"`
		GitHubOwner   *string             `json:"github_owner"`
		GitHubRepo    *string             `json:"github_repo"`
		Services      *db.ProjectServices `json:"services"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Update basic fields (use existing values if not provided)
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	repoPath := existing.RepoPath
	if req.RepoPath != nil {
		repoPath = *req.RepoPath
	}
	defaultBranch := existing.DefaultBranch
	if req.DefaultBranch != nil {
		defaultBranch = *req.DefaultBranch
	}

	if err := h.deps.DB.UpdateProject(id, name, repoPath, defaultBranch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Update GitHub info if provided
	if req.GitHubOwner != nil || req.GitHubRepo != nil {
		owner := ""
		repo := ""
		if existing.GitHubOwner.Valid {
			owner = existing.GitHubOwner.String
		}
		if existing.GitHubRepo.Valid {
			repo = existing.GitHubRepo.String
		}
		if req.GitHubOwner != nil {
			owner = *req.GitHubOwner
		}
		if req.GitHubRepo != nil {
			repo = *req.GitHubRepo
		}
		if err := h.deps.DB.UpdateProjectGitHub(id, owner, repo); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	// Update services if provided
	if req.Services != nil {
		if err := h.deps.DB.UpdateProjectServices(id, *req.Services); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	// Return updated project
	updated, err := h.deps.DB.GetProjectByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, updated)
}

// HandleDelete removes a project.
// DELETE /api/v1/projects/:id
func (h *Handler) HandleDelete(c echo.Context) error {
	id := c.Param("id")

	if err := h.deps.DB.DeleteProject(id); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}
