// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v68/github"
)

// GitHubClient wraps the go-github client for Poindexter's needs
type GitHubClient struct {
	client     *github.Client
	token      string // Stored for git HTTPS operations
	defaultOrg string
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

// NewGitHubClientFromToken creates a new GitHubClient from a token string.
// This is useful for GitHub App installation tokens.
func NewGitHubClientFromToken(token string, defaultOrg string) *GitHubClient {
	if token == "" {
		return nil
	}

	client := github.NewClient(nil).WithAuthToken(token)

	return &GitHubClient{
		client:     client,
		token:      token,
		defaultOrg: defaultOrg,
	}
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
		AutoInit:    github.Ptr(true),
	}

	org := opts.Org
	if org == "" {
		org = g.defaultOrg
	}

	var created *github.Repository
	var err error

	if org != "" {
		created, _, err = g.client.Repositories.Create(ctx, org, repo)
	} else {
		created, _, err = g.client.Repositories.Create(ctx, "", repo)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create repo: %w", err)
	}
	return created, nil
}

// ListReposOptions specifies options for listing repositories
type ListReposOptions struct {
	Org     string // If empty, uses defaultOrg; if still empty, lists user's repos
	PerPage int
	Page    int
}

// ListRepos lists repositories for the configured org or authenticated user
func (g *GitHubClient) ListRepos(ctx context.Context, opts ListReposOptions) ([]*github.Repository, error) {
	org := opts.Org
	if org == "" {
		org = g.defaultOrg
	}

	listOpts := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{
			PerPage: opts.PerPage,
			Page:    opts.Page,
		},
	}

	if listOpts.PerPage == 0 {
		listOpts.PerPage = 30
	}

	var repos []*github.Repository
	var err error

	if org != "" {
		orgListOpts := &github.RepositoryListByOrgOptions{
			ListOptions: listOpts.ListOptions,
		}
		repos, _, err = g.client.Repositories.ListByOrg(ctx, org, orgListOpts)
	} else {
		authListOpts := &github.RepositoryListByAuthenticatedUserOptions{
			ListOptions: listOpts.ListOptions,
		}
		repos, _, err = g.client.Repositories.ListByAuthenticatedUser(ctx, authListOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %w", err)
	}
	return repos, nil
}

// CreateIssueOptions specifies options for creating an issue
type CreateIssueOptions struct {
	Owner    string
	Repo     string
	Title    string
	Body     string
	Labels   []string
	Assignee string
}

// CreateIssue creates a new issue in the specified repository
func (g *GitHubClient) CreateIssue(ctx context.Context, opts CreateIssueOptions) (*github.Issue, error) {
	owner := opts.Owner
	if owner == "" {
		owner = g.defaultOrg
	}

	issueReq := &github.IssueRequest{
		Title: github.Ptr(opts.Title),
		Body:  github.Ptr(opts.Body),
	}

	if len(opts.Labels) > 0 {
		issueReq.Labels = &opts.Labels
	}

	if opts.Assignee != "" {
		issueReq.Assignee = github.Ptr(opts.Assignee)
	}

	issue, _, err := g.client.Issues.Create(ctx, owner, opts.Repo, issueReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}
	return issue, nil
}

// UpdateIssueOptions specifies options for updating an issue
type UpdateIssueOptions struct {
	Owner       string
	Repo        string
	IssueNumber int
	Title       *string
	Body        *string
	State       *string // "open" or "closed"
	Labels      *[]string
	Assignee    *string
}

// UpdateIssue updates an existing issue
func (g *GitHubClient) UpdateIssue(ctx context.Context, opts UpdateIssueOptions) (*github.Issue, error) {
	owner := opts.Owner
	if owner == "" {
		owner = g.defaultOrg
	}

	issueReq := &github.IssueRequest{
		Title:    opts.Title,
		Body:     opts.Body,
		State:    opts.State,
		Labels:   opts.Labels,
		Assignee: opts.Assignee,
	}

	issue, _, err := g.client.Issues.Edit(ctx, owner, opts.Repo, opts.IssueNumber, issueReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update issue: %w", err)
	}
	return issue, nil
}

// CloseIssue closes an issue by number
func (g *GitHubClient) CloseIssue(ctx context.Context, owner, repo string, issueNumber int) (*github.Issue, error) {
	closed := "closed"
	return g.UpdateIssue(ctx, UpdateIssueOptions{
		Owner:       owner,
		Repo:        repo,
		IssueNumber: issueNumber,
		State:       &closed,
	})
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

// MergePROptions specifies options for merging a pull request
type MergePROptions struct {
	Owner       string
	Repo        string
	PRNumber    int
	MergeMethod string // "merge", "squash", or "rebase"
	CommitTitle string
}

// MergePR merges a pull request
func (g *GitHubClient) MergePR(ctx context.Context, opts MergePROptions) (*github.PullRequestMergeResult, error) {
	owner := opts.Owner
	if owner == "" {
		owner = g.defaultOrg
	}

	mergeMethod := opts.MergeMethod
	if mergeMethod == "" {
		mergeMethod = "squash"
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: mergeMethod,
	}

	result, _, err := g.client.PullRequests.Merge(ctx, owner, opts.Repo, opts.PRNumber, opts.CommitTitle, mergeOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to merge PR: %w", err)
	}
	return result, nil
}

// SetupActionsOptions specifies options for setting up GitHub Actions
type SetupActionsOptions struct {
	Owner        string
	Repo         string
	WorkflowName string // e.g., "ci.yml"
	WorkflowYAML string // The workflow file content
	Branch       string // Branch to commit to (default: main)
	CommitMsg    string
}

// SetupActions creates or updates a GitHub Actions workflow file
func (g *GitHubClient) SetupActions(ctx context.Context, opts SetupActionsOptions) error {
	owner := opts.Owner
	if owner == "" {
		owner = g.defaultOrg
	}

	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}

	path := fmt.Sprintf(".github/workflows/%s", opts.WorkflowName)

	commitMsg := opts.CommitMsg
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("Add %s workflow", opts.WorkflowName)
	}

	// Check if file already exists
	fileContent, _, _, err := g.client.Repositories.GetContents(ctx, owner, opts.Repo, path, &github.RepositoryContentGetOptions{
		Ref: branch,
	})

	if err != nil {
		// File doesn't exist, create it
		_, _, err = g.client.Repositories.CreateFile(ctx, owner, opts.Repo, path, &github.RepositoryContentFileOptions{
			Message: github.Ptr(commitMsg),
			Content: []byte(opts.WorkflowYAML),
			Branch:  github.Ptr(branch),
		})
		if err != nil {
			return fmt.Errorf("failed to create workflow file: %w", err)
		}
	} else {
		// File exists, update it
		_, _, err = g.client.Repositories.UpdateFile(ctx, owner, opts.Repo, path, &github.RepositoryContentFileOptions{
			Message: github.Ptr(commitMsg),
			Content: []byte(opts.WorkflowYAML),
			SHA:     fileContent.SHA,
			Branch:  github.Ptr(branch),
		})
		if err != nil {
			return fmt.Errorf("failed to update workflow file: %w", err)
		}
	}

	return nil
}

// GetRepo retrieves repository information
// Returns nil, nil if the repository doesn't exist (404)
func (g *GitHubClient) GetRepo(ctx context.Context, owner, repo string) (*github.Repository, error) {
	if owner == "" {
		owner = g.defaultOrg
	}

	// If still no owner, get the authenticated user
	if owner == "" {
		user, err := g.GetAuthenticatedUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get authenticated user: %w", err)
		}
		owner = user.GetLogin()
	}

	repository, resp, err := g.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		// Check if it's a 404 (repo not found)
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get repo: %w", err)
	}

	return repository, nil
}

// EnsureRepo creates a repo if it doesn't exist, or returns the existing repo
func (g *GitHubClient) EnsureRepo(ctx context.Context, opts CreateRepoOptions) (*github.Repository, error) {
	owner := opts.Org
	if owner == "" {
		owner = g.defaultOrg
	}

	// If still no owner, get the authenticated user
	if owner == "" {
		user, err := g.GetAuthenticatedUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get authenticated user: %w", err)
		}
		owner = user.GetLogin()
	}

	// Check if repo already exists
	existing, err := g.GetRepo(ctx, owner, opts.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing repo: %w", err)
	}

	if existing != nil {
		return existing, nil
	}

	// Repo doesn't exist, create it
	return g.CreateRepo(ctx, opts)
}

// GetAuthenticatedUser returns the authenticated user's information
func (g *GitHubClient) GetAuthenticatedUser(ctx context.Context) (*github.User, error) {
	user, _, err := g.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}
	return user, nil
}
