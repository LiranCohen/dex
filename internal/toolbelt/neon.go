// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const neonAPIBaseURL = "https://console.neon.tech/api/v2"

// NeonClient wraps the Neon API for Poindexter's needs
type NeonClient struct {
	httpClient    *http.Client
	apiKey        string
	defaultRegion string
}

// NewNeonClient creates a new NeonClient from configuration
func NewNeonClient(config *NeonConfig) *NeonClient {
	if config == nil || config.APIKey == "" {
		return nil
	}

	defaultRegion := config.DefaultRegion
	if defaultRegion == "" {
		defaultRegion = "aws-us-east-1"
	}

	return &NeonClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		apiKey:        config.APIKey,
		defaultRegion: defaultRegion,
	}
}

// doRequest performs an HTTP request to the Neon API
func (c *NeonClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// neonErrorResponse represents a Neon API error response
type neonErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// parseNeonResponse reads and unmarshals a Neon API response
func parseNeonResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp neonErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("neon API error (status %d): %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("neon API error (%s): %s", errResp.Code, errResp.Message)
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// Ping verifies the Neon connection by getting the current user
func (c *NeonClient) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/users/me", neonAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("neon ping failed: %w", err)
	}

	_, err = parseNeonResponse[struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}](resp)
	if err != nil {
		return fmt.Errorf("neon ping failed: %w", err)
	}

	return nil
}

// --- Projects ---

// NeonProject represents a Neon project
type NeonProject struct {
	ID                  string    `json:"id"`
	PlatformID          string    `json:"platform_id"`
	RegionID            string    `json:"region_id"`
	Name                string    `json:"name"`
	PgVersion           int       `json:"pg_version"`
	ProxyHost           string    `json:"proxy_host"`
	StorePasswords      bool      `json:"store_passwords"`
	CPUUsedSec          int64     `json:"cpu_used_sec"`
	MaintenanceStartsAt *string   `json:"maintenance_starts_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// NeonProjectResponse wraps project responses that include related resources
type NeonProjectResponse struct {
	Project           NeonProject     `json:"project"`
	ConnectionURIs    []ConnectionURI `json:"connection_uris,omitempty"`
	Databases         []NeonDatabase  `json:"databases,omitempty"`
	Endpoints         []NeonEndpoint  `json:"endpoints,omitempty"`
	Roles             []NeonRole      `json:"roles,omitempty"`
	Branch            *NeonBranch     `json:"branch,omitempty"`
	OperationsLimit   int             `json:"operations_limit,omitempty"`
	OperationsRemaining int           `json:"operations_remaining,omitempty"`
}

// ConnectionURI represents a database connection URI
type ConnectionURI struct {
	ConnectionURI       string             `json:"connection_uri"`
	ConnectionParameters ConnectionParams  `json:"connection_parameters"`
}

// ConnectionParams represents connection parameters
type ConnectionParams struct {
	Database string `json:"database"`
	Role     string `json:"role"`
	Host     string `json:"host"`
	PoolerHost string `json:"pooler_host,omitempty"`
}

// CreateProjectOptions specifies options for creating a Neon project
type CreateProjectOptions struct {
	Name      string // Project name (required)
	RegionID  string // Region ID (optional, defaults to client's default region)
	PgVersion int    // PostgreSQL version (optional, defaults to latest)
}

// CreateProject creates a new Neon project
func (c *NeonClient) CreateProject(ctx context.Context, opts CreateProjectOptions) (*NeonProjectResponse, error) {
	url := fmt.Sprintf("%s/projects", neonAPIBaseURL)

	regionID := opts.RegionID
	if regionID == "" {
		regionID = c.defaultRegion
	}

	reqBody := map[string]any{
		"project": map[string]any{
			"name":      opts.Name,
			"region_id": regionID,
		},
	}

	if opts.PgVersion > 0 {
		reqBody["project"].(map[string]any)["pg_version"] = opts.PgVersion
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	result, err := parseNeonResponse[NeonProjectResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetProject retrieves a Neon project by ID
func (c *NeonClient) GetProject(ctx context.Context, projectID string) (*NeonProjectResponse, error) {
	url := fmt.Sprintf("%s/projects/%s", neonAPIBaseURL, projectID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	result, err := parseNeonResponse[NeonProjectResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListProjects lists all Neon projects
func (c *NeonClient) ListProjects(ctx context.Context) ([]NeonProject, error) {
	url := fmt.Sprintf("%s/projects", neonAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	result, err := parseNeonResponse[struct {
		Projects []NeonProject `json:"projects"`
	}](resp)
	if err != nil {
		return nil, err
	}

	return result.Projects, nil
}

// DeleteProject deletes a Neon project
func (c *NeonClient) DeleteProject(ctx context.Context, projectID string) error {
	url := fmt.Sprintf("%s/projects/%s", neonAPIBaseURL, projectID)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	_, err = parseNeonResponse[NeonProjectResponse](resp)
	if err != nil {
		return err
	}

	return nil
}

// UpdateProjectOptions specifies options for updating a Neon project
type UpdateProjectOptions struct {
	Name string // New project name (optional)
}

// UpdateProject updates a Neon project
func (c *NeonClient) UpdateProject(ctx context.Context, projectID string, opts UpdateProjectOptions) (*NeonProjectResponse, error) {
	url := fmt.Sprintf("%s/projects/%s", neonAPIBaseURL, projectID)

	reqBody := map[string]any{
		"project": map[string]any{},
	}

	if opts.Name != "" {
		reqBody["project"].(map[string]any)["name"] = opts.Name
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	result, err := parseNeonResponse[NeonProjectResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// --- Branches ---

// NeonBranch represents a Neon branch
type NeonBranch struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	ParentID         *string    `json:"parent_id"`
	ParentLsn        *string    `json:"parent_lsn"`
	ParentTimestamp  *time.Time `json:"parent_timestamp"`
	Name             string     `json:"name"`
	CurrentState     string     `json:"current_state"`
	PendingState     *string    `json:"pending_state"`
	LogicalSize      *int64     `json:"logical_size"`
	PhysicalSize     *int64     `json:"physical_size"`
	Primary          bool       `json:"primary"`
	CPUUsedSec       int64      `json:"cpu_used_sec"`
	ComputeTimeSeconds int64    `json:"compute_time_seconds"`
	ActiveTimeSeconds  int64    `json:"active_time_seconds"`
	WrittenDataBytes   int64    `json:"written_data_bytes"`
	DataTransferBytes  int64    `json:"data_transfer_bytes"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// NeonBranchResponse wraps branch responses
type NeonBranchResponse struct {
	Branch    NeonBranch     `json:"branch"`
	Endpoints []NeonEndpoint `json:"endpoints,omitempty"`
}

// CreateBranchOptions specifies options for creating a Neon branch
type CreateBranchOptions struct {
	Name            string     // Branch name (optional)
	ParentID        string     // Parent branch ID (optional, defaults to primary branch)
	ParentLsn       string     // Parent LSN for point-in-time restore (optional)
	ParentTimestamp *time.Time // Parent timestamp for point-in-time restore (optional)
}

// CreateBranch creates a new Neon branch
func (c *NeonClient) CreateBranch(ctx context.Context, projectID string, opts CreateBranchOptions) (*NeonBranchResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches", neonAPIBaseURL, projectID)

	reqBody := map[string]any{
		"branch": map[string]any{},
	}

	branchSpec := reqBody["branch"].(map[string]any)
	if opts.Name != "" {
		branchSpec["name"] = opts.Name
	}
	if opts.ParentID != "" {
		branchSpec["parent_id"] = opts.ParentID
	}
	if opts.ParentLsn != "" {
		branchSpec["parent_lsn"] = opts.ParentLsn
	}
	if opts.ParentTimestamp != nil {
		branchSpec["parent_timestamp"] = opts.ParentTimestamp.Format(time.RFC3339)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	result, err := parseNeonResponse[NeonBranchResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetBranch retrieves a Neon branch by ID
func (c *NeonClient) GetBranch(ctx context.Context, projectID, branchID string) (*NeonBranchResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s", neonAPIBaseURL, projectID, branchID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get branch: %w", err)
	}

	result, err := parseNeonResponse[NeonBranchResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListBranches lists all branches in a Neon project
func (c *NeonClient) ListBranches(ctx context.Context, projectID string) ([]NeonBranch, error) {
	url := fmt.Sprintf("%s/projects/%s/branches", neonAPIBaseURL, projectID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	result, err := parseNeonResponse[struct {
		Branches []NeonBranch `json:"branches"`
	}](resp)
	if err != nil {
		return nil, err
	}

	return result.Branches, nil
}

// DeleteBranch deletes a Neon branch
func (c *NeonClient) DeleteBranch(ctx context.Context, projectID, branchID string) error {
	url := fmt.Sprintf("%s/projects/%s/branches/%s", neonAPIBaseURL, projectID, branchID)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}

	_, err = parseNeonResponse[NeonBranchResponse](resp)
	if err != nil {
		return err
	}

	return nil
}

// UpdateBranchOptions specifies options for updating a Neon branch
type UpdateBranchOptions struct {
	Name string // New branch name (optional)
}

// UpdateBranch updates a Neon branch
func (c *NeonClient) UpdateBranch(ctx context.Context, projectID, branchID string, opts UpdateBranchOptions) (*NeonBranchResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s", neonAPIBaseURL, projectID, branchID)

	reqBody := map[string]any{
		"branch": map[string]any{},
	}

	if opts.Name != "" {
		reqBody["branch"].(map[string]any)["name"] = opts.Name
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update branch: %w", err)
	}

	result, err := parseNeonResponse[NeonBranchResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// SetBranchAsPrimary sets a branch as the primary branch
func (c *NeonClient) SetBranchAsPrimary(ctx context.Context, projectID, branchID string) (*NeonBranchResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/set_as_primary", neonAPIBaseURL, projectID, branchID)

	resp, err := c.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to set branch as primary: %w", err)
	}

	result, err := parseNeonResponse[NeonBranchResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// --- Databases ---

// NeonDatabase represents a Neon database
type NeonDatabase struct {
	ID        int64     `json:"id"`
	BranchID  string    `json:"branch_id"`
	Name      string    `json:"name"`
	OwnerName string    `json:"owner_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NeonDatabaseResponse wraps database responses
type NeonDatabaseResponse struct {
	Database NeonDatabase `json:"database"`
}

// CreateDatabaseOptions specifies options for creating a Neon database
type CreateDatabaseOptions struct {
	Name      string // Database name (required)
	OwnerName string // Role name to own the database (required)
}

// CreateDatabase creates a new Neon database
func (c *NeonClient) CreateDatabase(ctx context.Context, projectID, branchID string, opts CreateDatabaseOptions) (*NeonDatabaseResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/databases", neonAPIBaseURL, projectID, branchID)

	reqBody := map[string]any{
		"database": map[string]any{
			"name":       opts.Name,
			"owner_name": opts.OwnerName,
		},
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	result, err := parseNeonResponse[NeonDatabaseResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetDatabase retrieves a Neon database
func (c *NeonClient) GetDatabase(ctx context.Context, projectID, branchID, databaseName string) (*NeonDatabaseResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/databases/%s", neonAPIBaseURL, projectID, branchID, databaseName)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	result, err := parseNeonResponse[NeonDatabaseResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListDatabases lists all databases in a Neon branch
func (c *NeonClient) ListDatabases(ctx context.Context, projectID, branchID string) ([]NeonDatabase, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/databases", neonAPIBaseURL, projectID, branchID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	result, err := parseNeonResponse[struct {
		Databases []NeonDatabase `json:"databases"`
	}](resp)
	if err != nil {
		return nil, err
	}

	return result.Databases, nil
}

// DeleteDatabase deletes a Neon database
func (c *NeonClient) DeleteDatabase(ctx context.Context, projectID, branchID, databaseName string) error {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/databases/%s", neonAPIBaseURL, projectID, branchID, databaseName)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}

	_, err = parseNeonResponse[NeonDatabaseResponse](resp)
	if err != nil {
		return err
	}

	return nil
}

// UpdateDatabaseOptions specifies options for updating a Neon database
type UpdateDatabaseOptions struct {
	Name      string // New database name (optional)
	OwnerName string // New owner role name (optional)
}

// UpdateDatabase updates a Neon database
func (c *NeonClient) UpdateDatabase(ctx context.Context, projectID, branchID, databaseName string, opts UpdateDatabaseOptions) (*NeonDatabaseResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/databases/%s", neonAPIBaseURL, projectID, branchID, databaseName)

	reqBody := map[string]any{
		"database": map[string]any{},
	}

	if opts.Name != "" {
		reqBody["database"].(map[string]any)["name"] = opts.Name
	}
	if opts.OwnerName != "" {
		reqBody["database"].(map[string]any)["owner_name"] = opts.OwnerName
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update database: %w", err)
	}

	result, err := parseNeonResponse[NeonDatabaseResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// --- Endpoints ---

// NeonEndpoint represents a Neon compute endpoint
type NeonEndpoint struct {
	ID                string     `json:"id"`
	ProjectID         string     `json:"project_id"`
	BranchID          string     `json:"branch_id"`
	Host              string     `json:"host"`
	PoolerHost        string     `json:"pooler_host"`
	PoolerEnabled     bool       `json:"pooler_enabled"`
	Type              string     `json:"type"` // "read_write" or "read_only"
	CurrentState      string     `json:"current_state"`
	PendingState      *string    `json:"pending_state"`
	AutoscalingLimitMinCu float64 `json:"autoscaling_limit_min_cu"`
	AutoscalingLimitMaxCu float64 `json:"autoscaling_limit_max_cu"`
	SuspendTimeoutSeconds int64  `json:"suspend_timeout_seconds"`
	Disabled          bool       `json:"disabled"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// NeonEndpointResponse wraps endpoint responses
type NeonEndpointResponse struct {
	Endpoint NeonEndpoint `json:"endpoint"`
}

// ListEndpoints lists all endpoints in a Neon project
func (c *NeonClient) ListEndpoints(ctx context.Context, projectID string) ([]NeonEndpoint, error) {
	url := fmt.Sprintf("%s/projects/%s/endpoints", neonAPIBaseURL, projectID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoints: %w", err)
	}

	result, err := parseNeonResponse[struct {
		Endpoints []NeonEndpoint `json:"endpoints"`
	}](resp)
	if err != nil {
		return nil, err
	}

	return result.Endpoints, nil
}

// GetEndpoint retrieves a Neon endpoint
func (c *NeonClient) GetEndpoint(ctx context.Context, projectID, endpointID string) (*NeonEndpointResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/endpoints/%s", neonAPIBaseURL, projectID, endpointID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint: %w", err)
	}

	result, err := parseNeonResponse[NeonEndpointResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// StartEndpoint starts a suspended Neon endpoint
func (c *NeonClient) StartEndpoint(ctx context.Context, projectID, endpointID string) (*NeonEndpointResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/endpoints/%s/start", neonAPIBaseURL, projectID, endpointID)

	resp, err := c.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start endpoint: %w", err)
	}

	result, err := parseNeonResponse[NeonEndpointResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// SuspendEndpoint suspends a Neon endpoint
func (c *NeonClient) SuspendEndpoint(ctx context.Context, projectID, endpointID string) (*NeonEndpointResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/endpoints/%s/suspend", neonAPIBaseURL, projectID, endpointID)

	resp, err := c.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to suspend endpoint: %w", err)
	}

	result, err := parseNeonResponse[NeonEndpointResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// --- Roles ---

// NeonRole represents a Neon database role
type NeonRole struct {
	BranchID  string    `json:"branch_id"`
	Name      string    `json:"name"`
	Password  *string   `json:"password,omitempty"`
	Protected bool      `json:"protected"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NeonRoleResponse wraps role responses
type NeonRoleResponse struct {
	Role NeonRole `json:"role"`
}

// CreateRole creates a new Neon database role
func (c *NeonClient) CreateRole(ctx context.Context, projectID, branchID, name string) (*NeonRoleResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/roles", neonAPIBaseURL, projectID, branchID)

	reqBody := map[string]any{
		"role": map[string]any{
			"name": name,
		},
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	result, err := parseNeonResponse[NeonRoleResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetRole retrieves a Neon database role
func (c *NeonClient) GetRole(ctx context.Context, projectID, branchID, roleName string) (*NeonRoleResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/roles/%s", neonAPIBaseURL, projectID, branchID, roleName)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	result, err := parseNeonResponse[NeonRoleResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListRoles lists all roles in a Neon branch
func (c *NeonClient) ListRoles(ctx context.Context, projectID, branchID string) ([]NeonRole, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/roles", neonAPIBaseURL, projectID, branchID)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}

	result, err := parseNeonResponse[struct {
		Roles []NeonRole `json:"roles"`
	}](resp)
	if err != nil {
		return nil, err
	}

	return result.Roles, nil
}

// DeleteRole deletes a Neon database role
func (c *NeonClient) DeleteRole(ctx context.Context, projectID, branchID, roleName string) error {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/roles/%s", neonAPIBaseURL, projectID, branchID, roleName)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	_, err = parseNeonResponse[NeonRoleResponse](resp)
	if err != nil {
		return err
	}

	return nil
}

// ResetRolePassword resets the password for a Neon database role
func (c *NeonClient) ResetRolePassword(ctx context.Context, projectID, branchID, roleName string) (*NeonRoleResponse, error) {
	url := fmt.Sprintf("%s/projects/%s/branches/%s/roles/%s/reset_password", neonAPIBaseURL, projectID, branchID, roleName)

	resp, err := c.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to reset role password: %w", err)
	}

	result, err := parseNeonResponse[NeonRoleResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// --- Connection Strings ---

// GetConnectionString returns a PostgreSQL connection string for a project.
// This is a convenience method that builds a connection string from the project's
// endpoint, database, and role information.
func (c *NeonClient) GetConnectionString(ctx context.Context, projectID string, database, role string) (string, error) {
	// Get the project to find the primary endpoint
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project: %w", err)
	}

	// Get the endpoints for this project
	endpoints, err := c.ListEndpoints(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to list endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		return "", fmt.Errorf("no endpoints found for project %s", projectID)
	}

	// Use the first read_write endpoint
	var endpoint *NeonEndpoint
	for i := range endpoints {
		if endpoints[i].Type == "read_write" {
			endpoint = &endpoints[i]
			break
		}
	}
	if endpoint == nil {
		endpoint = &endpoints[0]
	}

	// Get the role to get the password
	roleResp, err := c.GetRole(ctx, projectID, endpoint.BranchID, role)
	if err != nil {
		return "", fmt.Errorf("failed to get role: %w", err)
	}

	password := ""
	if roleResp.Role.Password != nil {
		password = *roleResp.Role.Password
	}

	// Build the connection string
	// Format: postgresql://[user[:password]@][host][:port][/database][?sslmode=require]
	// URL-encode the password to handle special characters like @, /, ?, #, etc.
	connStr := fmt.Sprintf("postgresql://%s:%s@%s/%s?sslmode=require",
		role,
		url.QueryEscape(password),
		endpoint.Host,
		database,
	)

	// If project has connection URIs, prefer those
	if len(project.ConnectionURIs) > 0 {
		for _, uri := range project.ConnectionURIs {
			if uri.ConnectionParameters.Database == database && uri.ConnectionParameters.Role == role {
				return uri.ConnectionURI, nil
			}
		}
	}

	return connStr, nil
}

// GetPoolerConnectionString returns a pooled PostgreSQL connection string.
// Uses Neon's built-in connection pooler for better performance with many connections.
func (c *NeonClient) GetPoolerConnectionString(ctx context.Context, projectID string, database, role string) (string, error) {
	// Get the endpoints for this project
	endpoints, err := c.ListEndpoints(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to list endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		return "", fmt.Errorf("no endpoints found for project %s", projectID)
	}

	// Use the first read_write endpoint with pooler enabled
	var endpoint *NeonEndpoint
	for i := range endpoints {
		if endpoints[i].Type == "read_write" && endpoints[i].PoolerEnabled {
			endpoint = &endpoints[i]
			break
		}
	}
	if endpoint == nil {
		// Fall back to any endpoint with pooler
		for i := range endpoints {
			if endpoints[i].PoolerEnabled {
				endpoint = &endpoints[i]
				break
			}
		}
	}
	if endpoint == nil {
		return "", fmt.Errorf("no pooler-enabled endpoints found for project %s", projectID)
	}

	// Get the role to get the password
	roleResp, err := c.GetRole(ctx, projectID, endpoint.BranchID, role)
	if err != nil {
		return "", fmt.Errorf("failed to get role: %w", err)
	}

	password := ""
	if roleResp.Role.Password != nil {
		password = *roleResp.Role.Password
	}

	// Build the pooler connection string
	// URL-encode the password to handle special characters like @, /, ?, #, etc.
	connStr := fmt.Sprintf("postgresql://%s:%s@%s/%s?sslmode=require",
		role,
		url.QueryEscape(password),
		endpoint.PoolerHost,
		database,
	)

	return connStr, nil
}
