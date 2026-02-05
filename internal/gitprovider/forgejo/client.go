// Package forgejo implements the gitprovider.Provider interface for Forgejo.
// It uses the Gitea SDK since Forgejo maintains API compatibility with Gitea.
package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lirancohen/dex/internal/gitprovider"
)

// Client implements gitprovider.Provider for a Forgejo instance.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Verify interface compliance at compile time.
var _ gitprovider.Provider = (*Client)(nil)

// New creates a new Forgejo provider client.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Name() string { return "forgejo" }

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "/api/v1/version")
	return err
}

// --- Repositories ---

func (c *Client) CreateRepo(ctx context.Context, owner string, opts gitprovider.CreateRepoOpts) (*gitprovider.Repository, error) {
	defaultBranch := opts.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	body := map[string]interface{}{
		"name":           opts.Name,
		"private":        opts.Private,
		"auto_init":      opts.AutoInit,
		"default_branch": defaultBranch,
	}
	if opts.Description != "" {
		body["description"] = opts.Description
	}

	resp, err := c.post(ctx, fmt.Sprintf("/api/v1/orgs/%s/repos", owner), body)
	if err != nil {
		return nil, fmt.Errorf("create repo: %w", err)
	}

	return parseRepo(resp)
}

func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*gitprovider.Repository, error) {
	resp, err := c.get(ctx, fmt.Sprintf("/api/v1/repos/%s/%s", owner, repo))
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	return parseRepo(resp)
}

func (c *Client) DeleteRepo(ctx context.Context, owner, repo string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/repos/%s/%s", owner, repo))
}

// --- Organizations ---

func (c *Client) CreateOrg(ctx context.Context, name string) error {
	body := map[string]interface{}{
		"username":   name,
		"visibility": "private",
	}
	_, err := c.post(ctx, "/api/v1/orgs", body)
	return err
}

// --- Issues ---

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, opts gitprovider.CreateIssueOpts) (*gitprovider.Issue, error) {
	body := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
	}

	resp, err := c.post(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/issues", owner, repo), body)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	return parseIssue(resp)
}

func (c *Client) UpdateIssue(ctx context.Context, owner, repo string, number int, opts gitprovider.UpdateIssueOpts) error {
	body := map[string]interface{}{}
	if opts.Title != nil {
		body["title"] = *opts.Title
	}
	if opts.Body != nil {
		body["body"] = *opts.Body
	}
	if opts.State != nil {
		body["state"] = *opts.State
	}

	_, err := c.patch(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d", owner, repo, number), body)
	return err
}

func (c *Client) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	body := map[string]interface{}{
		"state": "closed",
	}
	_, err := c.patch(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d", owner, repo, number), body)
	return err
}

func (c *Client) AddComment(ctx context.Context, owner, repo string, number int, commentBody string) (*gitprovider.Comment, error) {
	body := map[string]interface{}{
		"body": commentBody,
	}

	resp, err := c.post(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/comments", owner, repo, number), body)
	if err != nil {
		return nil, fmt.Errorf("add comment: %w", err)
	}

	return parseComment(resp)
}

func (c *Client) SetLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	// Forgejo's API expects label IDs, not names. We need to resolve names to IDs first.
	labelIDs, err := c.resolveLabelIDs(ctx, owner, repo, labels)
	if err != nil {
		return fmt.Errorf("resolve labels: %w", err)
	}

	body := map[string]interface{}{
		"labels": labelIDs,
	}
	_, err = c.put(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/labels", owner, repo, number), body)
	return err
}

// --- Pull Requests ---

func (c *Client) CreatePR(ctx context.Context, owner, repo string, opts gitprovider.CreatePROpts) (*gitprovider.PullRequest, error) {
	body := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
		"head":  opts.Head,
		"base":  opts.Base,
	}

	resp, err := c.post(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner, repo), body)
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	return parsePR(resp)
}

func (c *Client) MergePR(ctx context.Context, owner, repo string, number int, method gitprovider.MergeMethod) error {
	body := map[string]interface{}{
		"Do": string(method),
	}
	_, err := c.post(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge", owner, repo, number), body)
	return err
}

// --- Webhooks ---

func (c *Client) CreateWebhook(ctx context.Context, owner, repo string, opts gitprovider.CreateWebhookOpts) error {
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "json"
	}

	body := map[string]interface{}{
		"type":   "gitea",
		"active": opts.Active,
		"events": opts.Events,
		"config": map[string]string{
			"url":          opts.URL,
			"content_type": contentType,
		},
	}
	_, err := c.post(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/hooks", owner, repo), body)
	return err
}

// --- Label resolution ---

func (c *Client) resolveLabelIDs(ctx context.Context, owner, repo string, labelNames []string) ([]int64, error) {
	resp, err := c.get(ctx, fmt.Sprintf("/api/v1/repos/%s/%s/labels", owner, repo))
	if err != nil {
		return nil, err
	}

	var allLabels []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp, &allLabels); err != nil {
		return nil, fmt.Errorf("parse labels: %w", err)
	}

	nameToID := make(map[string]int64, len(allLabels))
	for _, l := range allLabels {
		nameToID[l.Name] = l.ID
	}

	ids := make([]int64, 0, len(labelNames))
	for _, name := range labelNames {
		if id, ok := nameToID[name]; ok {
			ids = append(ids, id)
		}
		// Skip labels that don't exist yet — caller can create them separately
	}
	return ids, nil
}

// --- HTTP helpers ---

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

func (c *Client) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	return c.doRequest(ctx, "POST", path, body)
}

func (c *Client) patch(ctx context.Context, path string, body interface{}) ([]byte, error) {
	return c.doRequest(ctx, "PATCH", path, body)
}

func (c *Client) put(ctx context.Context, path string, body interface{}) ([]byte, error) {
	return c.doRequest(ctx, "PUT", path, body)
}

func (c *Client) delete(ctx context.Context, path string) error {
	_, err := c.doRequest(ctx, "DELETE", path, nil)
	return err
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewBuffer(data)
	}

	url := c.baseURL + path

	var req *http.Request
	var err error
	if reqBody != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, reqBody)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var respBuf bytes.Buffer
	_, _ = respBuf.ReadFrom(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, respBuf.String())
	}

	return respBuf.Bytes(), nil
}

// --- Response parsers ---

func parseRepo(data []byte) (*gitprovider.Repository, error) {
	var raw struct {
		Owner         struct{ Login string } `json:"owner"`
		Name          string                 `json:"name"`
		FullName      string                 `json:"full_name"`
		CloneURL      string                 `json:"clone_url"`
		DefaultBranch string                 `json:"default_branch"`
		Private       bool                   `json:"private"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse repo response: %w", err)
	}
	return &gitprovider.Repository{
		Owner:         raw.Owner.Login,
		Name:          raw.Name,
		FullName:      raw.FullName,
		CloneURL:      raw.CloneURL,
		DefaultBranch: raw.DefaultBranch,
		Private:       raw.Private,
	}, nil
}

func parseIssue(data []byte) (*gitprovider.Issue, error) {
	var raw struct {
		Number    int64     `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		State     string    `json:"state"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse issue response: %w", err)
	}
	return &gitprovider.Issue{
		Number:    int(raw.Number),
		Title:     raw.Title,
		Body:      raw.Body,
		State:     raw.State,
		CreatedAt: raw.CreatedAt,
	}, nil
}

func parseComment(data []byte) (*gitprovider.Comment, error) {
	var raw struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse comment response: %w", err)
	}
	return &gitprovider.Comment{
		ID:        raw.ID,
		Body:      raw.Body,
		CreatedAt: raw.CreatedAt,
	}, nil
}

func parsePR(data []byte) (*gitprovider.PullRequest, error) {
	var raw struct {
		Number    int64     `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		State     string    `json:"state"`
		HTMLURL   string    `json:"html_url"`
		Head      struct{ Ref string } `json:"head"`
		Base      struct{ Ref string } `json:"base"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse PR response: %w", err)
	}
	return &gitprovider.PullRequest{
		Number:    int(raw.Number),
		Title:     raw.Title,
		Body:      raw.Body,
		State:     raw.State,
		Head:      raw.Head.Ref,
		Base:      raw.Base.Ref,
		HTMLURL:   raw.HTMLURL,
		CreatedAt: raw.CreatedAt,
	}, nil
}
