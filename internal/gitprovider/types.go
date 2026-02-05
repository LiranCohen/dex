package gitprovider

import "time"

// Repository represents a git repository on a provider.
type Repository struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// Issue represents an issue on a provider.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // "open" or "closed"
	Labels    []string  `json:"labels,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Comment represents a comment on an issue or PR.
type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// PullRequest represents a pull request on a provider.
type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // "open", "closed", "merged"
	Head      string    `json:"head"`  // Source branch
	Base      string    `json:"base"`  // Target branch
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRepoOpts contains options for creating a repository.
type CreateRepoOpts struct {
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	AutoInit      bool   `json:"auto_init"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description,omitempty"`
}

// CreateIssueOpts contains options for creating an issue.
type CreateIssueOpts struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

// UpdateIssueOpts contains options for updating an issue.
type UpdateIssueOpts struct {
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	State *string `json:"state,omitempty"` // "open" or "closed"
}

// CreatePROpts contains options for creating a pull request.
type CreatePROpts struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"` // Source branch
	Base  string `json:"base"` // Target branch
}

// CreateWebhookOpts contains options for creating a webhook.
type CreateWebhookOpts struct {
	URL         string   `json:"url"`
	ContentType string   `json:"content_type"` // "json" or "form"
	Events      []string `json:"events"`
	Active      bool     `json:"active"`
}
