// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

// CloudflareClient wraps the Cloudflare API for Poindexter's needs
type CloudflareClient struct {
	httpClient *http.Client
	apiToken   string
	accountID  string
}

// NewCloudflareClient creates a new CloudflareClient from configuration
func NewCloudflareClient(config *CloudflareConfig) *CloudflareClient {
	if config == nil || config.APIToken == "" {
		return nil
	}

	return &CloudflareClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		apiToken:  config.APIToken,
		accountID: config.AccountID,
	}
}

// doRequest performs an HTTP request to the Cloudflare API
func (c *CloudflareClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// cloudflareResponse wraps the standard Cloudflare API response format
type cloudflareResponse[T any] struct {
	Success  bool              `json:"success"`
	Errors   []cloudflareError `json:"errors"`
	Messages []string          `json:"messages"`
	Result   T                 `json:"result"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// parseCloudflareResponse reads and unmarshals a Cloudflare API response
func parseCloudflareResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var cfResp cloudflareResponse[T]
	if err := json.Unmarshal(body, &cfResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !cfResp.Success {
		if len(cfResp.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare API error (code %d): %s", cfResp.Errors[0].Code, cfResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare API error: unknown error")
	}

	return &cfResp.Result, nil
}

// Ping verifies the Cloudflare connection by getting the user details
func (c *CloudflareClient) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/user/tokens/verify", cloudflareAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("cloudflare ping failed: %w", err)
	}

	result, err := parseCloudflareResponse[struct {
		Status string `json:"status"`
	}](resp)
	if err != nil {
		return fmt.Errorf("cloudflare ping failed: %w", err)
	}

	if result.Status != "" && result.Status != "active" {
		return fmt.Errorf("cloudflare token is not active: %s", result.Status)
	}

	return nil
}

// --- DNS Records ---

// DNSRecord represents a Cloudflare DNS record
type DNSRecord struct {
	ID        string `json:"id"`
	ZoneID    string `json:"zone_id"`
	ZoneName  string `json:"zone_name"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Proxied   bool   `json:"proxied"`
	TTL       int    `json:"ttl"`
	Priority  *int   `json:"priority,omitempty"`
	CreatedOn string `json:"created_on"`
	ModifiedOn string `json:"modified_on"`
}

// AddDNSRecordOptions specifies options for adding a DNS record
type AddDNSRecordOptions struct {
	ZoneID   string // Zone ID (required)
	Type     string // A, AAAA, CNAME, TXT, MX, etc.
	Name     string // Record name (e.g., "api" for api.example.com)
	Content  string // Record content (IP address, target hostname, etc.)
	TTL      int    // Time to live (1 = automatic)
	Proxied  bool   // Whether to proxy through Cloudflare
	Priority *int   // Priority for MX records
}

// AddDNSRecord creates a new DNS record in the specified zone
func (c *CloudflareClient) AddDNSRecord(ctx context.Context, opts AddDNSRecordOptions) (*DNSRecord, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records", cloudflareAPIBaseURL, opts.ZoneID)

	reqBody := map[string]any{
		"type":    opts.Type,
		"name":    opts.Name,
		"content": opts.Content,
		"proxied": opts.Proxied,
	}

	if opts.TTL > 0 {
		reqBody["ttl"] = opts.TTL
	} else {
		reqBody["ttl"] = 1 // Automatic
	}

	if opts.Priority != nil {
		reqBody["priority"] = *opts.Priority
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to add DNS record: %w", err)
	}

	record, err := parseCloudflareResponse[DNSRecord](resp)
	if err != nil {
		return nil, err
	}

	return record, nil
}

// UpdateDNSRecordOptions specifies options for updating a DNS record
type UpdateDNSRecordOptions struct {
	ZoneID   string // Zone ID (required)
	RecordID string // Record ID (required)
	Type     string // A, AAAA, CNAME, TXT, MX, etc.
	Name     string // Record name
	Content  string // Record content
	TTL      int    // Time to live (1 = automatic)
	Proxied  bool   // Whether to proxy through Cloudflare
	Priority *int   // Priority for MX records
}

// UpdateDNSRecord updates an existing DNS record
func (c *CloudflareClient) UpdateDNSRecord(ctx context.Context, opts UpdateDNSRecordOptions) (*DNSRecord, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareAPIBaseURL, opts.ZoneID, opts.RecordID)

	reqBody := map[string]any{
		"type":    opts.Type,
		"name":    opts.Name,
		"content": opts.Content,
		"proxied": opts.Proxied,
	}

	if opts.TTL > 0 {
		reqBody["ttl"] = opts.TTL
	} else {
		reqBody["ttl"] = 1 // Automatic
	}

	if opts.Priority != nil {
		reqBody["priority"] = *opts.Priority
	}

	resp, err := c.doRequest(ctx, http.MethodPut, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update DNS record: %w", err)
	}

	record, err := parseCloudflareResponse[DNSRecord](resp)
	if err != nil {
		return nil, err
	}

	return record, nil
}

// DeleteDNSRecord deletes a DNS record
func (c *CloudflareClient) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareAPIBaseURL, zoneID, recordID)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}

	_, err = parseCloudflareResponse[struct{ ID string }](resp)
	if err != nil {
		return err
	}

	return nil
}

// ListDNSRecords lists all DNS records for a zone
func (c *CloudflareClient) ListDNSRecords(ctx context.Context, zoneID string) ([]DNSRecord, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records", cloudflareAPIBaseURL, zoneID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}

	records, err := parseCloudflareResponse[[]DNSRecord](resp)
	if err != nil {
		return nil, err
	}

	if records == nil {
		return []DNSRecord{}, nil
	}
	return *records, nil
}

// GetZoneByName retrieves a zone by its domain name
func (c *CloudflareClient) GetZoneByName(ctx context.Context, name string) (*Zone, error) {
	url := fmt.Sprintf("%s/zones?name=%s", cloudflareAPIBaseURL, name)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone: %w", err)
	}

	zones, err := parseCloudflareResponse[[]Zone](resp)
	if err != nil {
		return nil, err
	}

	if zones == nil || len(*zones) == 0 {
		return nil, fmt.Errorf("zone not found: %s", name)
	}

	return &(*zones)[0], nil
}

// Zone represents a Cloudflare zone (domain)
type Zone struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	NameServers       []string `json:"name_servers"`
	OriginalNameServers []string `json:"original_name_servers"`
	CreatedOn         string `json:"created_on"`
	ModifiedOn        string `json:"modified_on"`
}

// --- R2 Storage ---

// R2Bucket represents a Cloudflare R2 bucket
type R2Bucket struct {
	Name         string `json:"name"`
	CreationDate string `json:"creation_date"`
	Location     string `json:"location"`
}

// CreateR2Bucket creates a new R2 bucket
func (c *CloudflareClient) CreateR2Bucket(ctx context.Context, bucketName string) (*R2Bucket, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for R2 operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/r2/buckets", cloudflareAPIBaseURL, c.accountID)

	reqBody := map[string]any{
		"name": bucketName,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create R2 bucket: %w", err)
	}

	bucket, err := parseCloudflareResponse[R2Bucket](resp)
	if err != nil {
		return nil, err
	}

	return bucket, nil
}

// DeleteR2Bucket deletes an R2 bucket
func (c *CloudflareClient) DeleteR2Bucket(ctx context.Context, bucketName string) error {
	if c.accountID == "" {
		return fmt.Errorf("account ID is required for R2 operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/r2/buckets/%s", cloudflareAPIBaseURL, c.accountID, bucketName)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete R2 bucket: %w", err)
	}

	_, err = parseCloudflareResponse[any](resp)
	if err != nil {
		return err
	}

	return nil
}

// ListR2Buckets lists all R2 buckets
func (c *CloudflareClient) ListR2Buckets(ctx context.Context) ([]R2Bucket, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for R2 operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/r2/buckets", cloudflareAPIBaseURL, c.accountID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list R2 buckets: %w", err)
	}

	// R2 bucket list has a different response structure
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var listResp struct {
		Success bool              `json:"success"`
		Errors  []cloudflareError `json:"errors"`
		Result  struct {
			Buckets []R2Bucket `json:"buckets"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !listResp.Success && len(listResp.Errors) > 0 {
		return nil, fmt.Errorf("cloudflare API error: %s", listResp.Errors[0].Message)
	}

	return listResp.Result.Buckets, nil
}

// --- KV Namespaces ---

// KVNamespace represents a Cloudflare Workers KV namespace
type KVNamespace struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	SupportURLEncoding bool `json:"supports_url_encoding"`
}

// CreateKVNamespace creates a new Workers KV namespace
func (c *CloudflareClient) CreateKVNamespace(ctx context.Context, title string) (*KVNamespace, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for KV operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces", cloudflareAPIBaseURL, c.accountID)

	reqBody := map[string]any{
		"title": title,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create KV namespace: %w", err)
	}

	namespace, err := parseCloudflareResponse[KVNamespace](resp)
	if err != nil {
		return nil, err
	}

	return namespace, nil
}

// DeleteKVNamespace deletes a Workers KV namespace
func (c *CloudflareClient) DeleteKVNamespace(ctx context.Context, namespaceID string) error {
	if c.accountID == "" {
		return fmt.Errorf("account ID is required for KV operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s", cloudflareAPIBaseURL, c.accountID, namespaceID)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV namespace: %w", err)
	}

	_, err = parseCloudflareResponse[any](resp)
	if err != nil {
		return err
	}

	return nil
}

// ListKVNamespaces lists all Workers KV namespaces
func (c *CloudflareClient) ListKVNamespaces(ctx context.Context) ([]KVNamespace, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for KV operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces", cloudflareAPIBaseURL, c.accountID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list KV namespaces: %w", err)
	}

	namespaces, err := parseCloudflareResponse[[]KVNamespace](resp)
	if err != nil {
		return nil, err
	}

	if namespaces == nil {
		return []KVNamespace{}, nil
	}
	return *namespaces, nil
}

// KVPut writes a key-value pair to a KV namespace
func (c *CloudflareClient) KVPut(ctx context.Context, namespaceID, key, value string) error {
	if c.accountID == "" {
		return fmt.Errorf("account ID is required for KV operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s", cloudflareAPIBaseURL, c.accountID, namespaceID, key)

	// KV put uses raw body, not JSON
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader([]byte(value)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to put KV value: %w", err)
	}

	_, err = parseCloudflareResponse[any](resp)
	if err != nil {
		return err
	}

	return nil
}

// KVGet retrieves a value from a KV namespace
func (c *CloudflareClient) KVGet(ctx context.Context, namespaceID, key string) (string, error) {
	if c.accountID == "" {
		return "", fmt.Errorf("account ID is required for KV operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s", cloudflareAPIBaseURL, c.accountID, namespaceID, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get KV value: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("key not found: %s", key)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get KV value (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read KV value: %w", err)
	}

	return string(body), nil
}

// KVDelete deletes a key from a KV namespace
func (c *CloudflareClient) KVDelete(ctx context.Context, namespaceID, key string) error {
	if c.accountID == "" {
		return fmt.Errorf("account ID is required for KV operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s", cloudflareAPIBaseURL, c.accountID, namespaceID, key)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV key: %w", err)
	}

	_, err = parseCloudflareResponse[any](resp)
	if err != nil {
		return err
	}

	return nil
}

// --- Pages Projects ---

// PagesProject represents a Cloudflare Pages project
type PagesProject struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Subdomain          string `json:"subdomain"`
	ProductionBranch   string `json:"production_branch"`
	CreatedOn          string `json:"created_on"`
	CanonicalDeployment *PagesDeployment `json:"canonical_deployment"`
	LatestDeployment   *PagesDeployment `json:"latest_deployment"`
	Source             *PagesSource     `json:"source"`
}

// PagesDeployment represents a Pages deployment
type PagesDeployment struct {
	ID          string `json:"id"`
	ShortID     string `json:"short_id"`
	URL         string `json:"url"`
	Environment string `json:"environment"`
	CreatedOn   string `json:"created_on"`
	ModifiedOn  string `json:"modified_on"`
}

// PagesSource represents the source configuration for a Pages project
type PagesSource struct {
	Type   string           `json:"type"` // "github" or empty for direct upload
	Config *PagesGitHubConfig `json:"config,omitempty"`
}

// PagesGitHubConfig represents GitHub source configuration
type PagesGitHubConfig struct {
	Owner              string `json:"owner"`
	RepoName           string `json:"repo_name"`
	ProductionBranch   string `json:"production_branch"`
	PRCommentsEnabled  bool   `json:"pr_comments_enabled"`
	DeploymentsEnabled bool   `json:"deployments_enabled"`
}

// CreatePagesProjectOptions specifies options for creating a Pages project
type CreatePagesProjectOptions struct {
	Name             string // Project name (required)
	ProductionBranch string // Production branch name (default: "main")
}

// CreatePagesProject creates a new Cloudflare Pages project
func (c *CloudflareClient) CreatePagesProject(ctx context.Context, opts CreatePagesProjectOptions) (*PagesProject, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects", cloudflareAPIBaseURL, c.accountID)

	productionBranch := opts.ProductionBranch
	if productionBranch == "" {
		productionBranch = "main"
	}

	reqBody := map[string]any{
		"name":              opts.Name,
		"production_branch": productionBranch,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create Pages project: %w", err)
	}

	project, err := parseCloudflareResponse[PagesProject](resp)
	if err != nil {
		return nil, err
	}

	return project, nil
}

// DeletePagesProject deletes a Pages project
func (c *CloudflareClient) DeletePagesProject(ctx context.Context, projectName string) error {
	if c.accountID == "" {
		return fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s", cloudflareAPIBaseURL, c.accountID, projectName)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete Pages project: %w", err)
	}

	_, err = parseCloudflareResponse[any](resp)
	if err != nil {
		return err
	}

	return nil
}

// GetPagesProject retrieves a Pages project by name
func (c *CloudflareClient) GetPagesProject(ctx context.Context, projectName string) (*PagesProject, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s", cloudflareAPIBaseURL, c.accountID, projectName)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Pages project: %w", err)
	}

	project, err := parseCloudflareResponse[PagesProject](resp)
	if err != nil {
		return nil, err
	}

	return project, nil
}

// ListPagesProjects lists all Pages projects
func (c *CloudflareClient) ListPagesProjects(ctx context.Context) ([]PagesProject, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects", cloudflareAPIBaseURL, c.accountID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list Pages projects: %w", err)
	}

	projects, err := parseCloudflareResponse[[]PagesProject](resp)
	if err != nil {
		return nil, err
	}

	if projects == nil {
		return []PagesProject{}, nil
	}
	return *projects, nil
}

// DeployPagesOptions specifies options for deploying to Pages
type DeployPagesOptions struct {
	ProjectName string // Project name (required)
	Branch      string // Branch name (default: "main")
}

// DeployPages creates a new deployment for a Pages project
// Note: This creates a deployment placeholder. For actual file uploads,
// use the Cloudflare wrangler CLI or the direct upload API with multipart form data.
func (c *CloudflareClient) DeployPages(ctx context.Context, opts DeployPagesOptions) (*PagesDeployment, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments", cloudflareAPIBaseURL, c.accountID, opts.ProjectName)

	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}

	reqBody := map[string]any{
		"branch": branch,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy to Pages: %w", err)
	}

	deployment, err := parseCloudflareResponse[PagesDeployment](resp)
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

// GetPagesDeployment retrieves a specific Pages deployment
func (c *CloudflareClient) GetPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeployment, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments/%s", cloudflareAPIBaseURL, c.accountID, projectName, deploymentID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Pages deployment: %w", err)
	}

	deployment, err := parseCloudflareResponse[PagesDeployment](resp)
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

// ListPagesDeployments lists all deployments for a Pages project
func (c *CloudflareClient) ListPagesDeployments(ctx context.Context, projectName string) ([]PagesDeployment, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments", cloudflareAPIBaseURL, c.accountID, projectName)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list Pages deployments: %w", err)
	}

	deployments, err := parseCloudflareResponse[[]PagesDeployment](resp)
	if err != nil {
		return nil, err
	}

	if deployments == nil {
		return []PagesDeployment{}, nil
	}
	return *deployments, nil
}

// --- Custom Domains for Pages ---

// PagesDomain represents a custom domain for a Pages project
type PagesDomain struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	ZoneTag     string `json:"zone_tag"`
	ValidationData struct {
		Status string `json:"status"`
		Method string `json:"method"`
	} `json:"validation_data"`
	CreatedOn   string `json:"created_on"`
}

// AddPagesDomain adds a custom domain to a Pages project
func (c *CloudflareClient) AddPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomain, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/domains", cloudflareAPIBaseURL, c.accountID, projectName)

	reqBody := map[string]any{
		"name": domain,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to add Pages domain: %w", err)
	}

	domainResult, err := parseCloudflareResponse[PagesDomain](resp)
	if err != nil {
		return nil, err
	}

	return domainResult, nil
}

// DeletePagesDomain removes a custom domain from a Pages project
func (c *CloudflareClient) DeletePagesDomain(ctx context.Context, projectName, domain string) error {
	if c.accountID == "" {
		return fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/domains/%s", cloudflareAPIBaseURL, c.accountID, projectName, domain)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete Pages domain: %w", err)
	}

	_, err = parseCloudflareResponse[any](resp)
	if err != nil {
		return err
	}

	return nil
}

// ListPagesDomains lists all custom domains for a Pages project
func (c *CloudflareClient) ListPagesDomains(ctx context.Context, projectName string) ([]PagesDomain, error) {
	if c.accountID == "" {
		return nil, fmt.Errorf("account ID is required for Pages operations")
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/domains", cloudflareAPIBaseURL, c.accountID, projectName)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list Pages domains: %w", err)
	}

	domains, err := parseCloudflareResponse[[]PagesDomain](resp)
	if err != nil {
		return nil, err
	}

	if domains == nil {
		return []PagesDomain{}, nil
	}
	return *domains, nil
}
