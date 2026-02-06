// Package gitprovider defines the interface for git hosting backends.
// Implementations exist for Forgejo (primary) and GitHub (legacy/optional).
package gitprovider

import "context"

// Provider is the interface that git hosting backends must implement.
// It covers the operations that Dex needs: repos, issues, PRs, and webhooks.
type Provider interface {
	// Name returns the provider identifier (e.g., "forgejo", "github").
	Name() string

	// Ping checks connectivity to the provider.
	Ping(ctx context.Context) error

	// --- Repositories ---

	CreateRepo(ctx context.Context, owner string, opts CreateRepoOpts) (*Repository, error)
	GetRepo(ctx context.Context, owner, repo string) (*Repository, error)
	DeleteRepo(ctx context.Context, owner, repo string) error

	// --- Organizations ---

	CreateOrg(ctx context.Context, name string) error

	// --- Issues ---

	CreateIssue(ctx context.Context, owner, repo string, opts CreateIssueOpts) (*Issue, error)
	UpdateIssue(ctx context.Context, owner, repo string, number int, opts UpdateIssueOpts) error
	CloseIssue(ctx context.Context, owner, repo string, number int) error
	AddComment(ctx context.Context, owner, repo string, number int, body string) (*Comment, error)
	SetLabels(ctx context.Context, owner, repo string, number int, labels []string) error

	// --- Pull Requests ---

	CreatePR(ctx context.Context, owner, repo string, opts CreatePROpts) (*PullRequest, error)
	MergePR(ctx context.Context, owner, repo string, number int, method MergeMethod) error

	// --- Webhooks ---

	CreateWebhook(ctx context.Context, owner, repo string, opts CreateWebhookOpts) error
}

// MergeMethod specifies how a PR should be merged.
type MergeMethod string

const (
	MergeMerge  MergeMethod = "merge"
	MergeRebase MergeMethod = "rebase"
	MergeSquash MergeMethod = "squash"
)
