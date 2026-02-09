// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
)

// GitHubClient wraps the go-github client for Poindexter's needs
type GitHubClient struct {
	client      *github.Client
	token       string // Stored for git HTTPS operations
	defaultOrg  string
	accountType string // "User" or "Organization" - affects how repos are created
}

// NewGitHubClient creates a new GitHubClient from configuration
func NewGitHubClient(config *GitHubConfig) *GitHubClient {
	if config == nil || config.Token == "" {
		return nil
	}

	client := github.NewClient(nil).WithAuthToken(config.Token)

	return &GitHubClient{
		client:     client,
		token:      config.Token,
		defaultOrg: config.DefaultOrg,
	}
}

// GetToken returns the configured GitHub token.
// This is used by the worker system to pass credentials to remote workers.
func (c *GitHubClient) GetToken() string {
	return c.token
}

// Token returns the auth token for git operations
func (g *GitHubClient) Token() string {
	return g.token
}

// AuthURL returns the authenticated URL for a git remote.
// Converts https://github.com/owner/repo to https://x-access-token:{token}@github.com/owner/repo
func (g *GitHubClient) AuthURL(repoURL string) string {
	if g.token == "" {
		return repoURL
	}
	// Handle both https://github.com/... and git@github.com:... formats
	if len(repoURL) > 19 && repoURL[:19] == "https://github.com/" {
		return fmt.Sprintf("https://x-access-token:%s@github.com/%s", g.token, repoURL[19:])
	}
	return repoURL
}

// Ping verifies the GitHub connection by getting the authenticated user
func (g *GitHubClient) Ping(ctx context.Context) error {
	_, _, err := g.client.Users.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("github ping failed: %w", err)
	}
	return nil
}

// GetUsername returns the authenticated user's GitHub username
func (g *GitHubClient) GetUsername(ctx context.Context) (string, error) {
	user, _, err := g.client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	return user.GetLogin(), nil
}

// CreateRepoOptions specifies options for creating a repository
type CreateRepoOptions struct {
	Name        string
	Description string
	Private     bool
	Org         string // If empty, uses defaultOrg; if still empty, creates in user's namespace
}

// CreateRepo creates a new GitHub repository
func (g *GitHubClient) CreateRepo(ctx context.Context, opts CreateRepoOptions) (*github.Repository, error) {
	repo := &github.Repository{
		Name:        github.Ptr(opts.Name),
		Description: github.Ptr(opts.Description),
		Private:     github.Ptr(opts.Private),
		AutoInit:    github.Ptr(false), // Don't auto-init - Dex pushes its own content
	}

	org := opts.Org
	if org == "" {
		org = g.defaultOrg
	}

	var created *github.Repository
	var err error

	// For organization accounts, pass the org name
	// For user accounts, pass empty string (creates under authenticated user)
	if org != "" && g.accountType == "Organization" {
		created, _, err = g.client.Repositories.Create(ctx, org, repo)
	} else {
		// User account or no org specified - create under user
		created, _, err = g.client.Repositories.Create(ctx, "", repo)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create repo: %w", err)
	}
	return created, nil
}

// CreatePROptions specifies options for creating a pull request
type CreatePROptions struct {
	Owner string
	Repo  string
	Title string
	Body  string
	Head  string // Branch containing changes
	Base  string // Branch to merge into (e.g., "main")
	Draft bool
}

// CreatePR creates a new pull request
func (g *GitHubClient) CreatePR(ctx context.Context, opts CreatePROptions) (*github.PullRequest, error) {
	owner := opts.Owner
	if owner == "" {
		owner = g.defaultOrg
	}

	prReq := &github.NewPullRequest{
		Title: github.Ptr(opts.Title),
		Body:  github.Ptr(opts.Body),
		Head:  github.Ptr(opts.Head),
		Base:  github.Ptr(opts.Base),
		Draft: github.Ptr(opts.Draft),
	}

	pr, _, err := g.client.PullRequests.Create(ctx, owner, opts.Repo, prReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}
	return pr, nil
}

// GetAuthenticatedUser returns the authenticated user's information.
func (g *GitHubClient) GetAuthenticatedUser(ctx context.Context) (*github.User, error) {
	user, _, err := g.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}
	return user, nil
}
