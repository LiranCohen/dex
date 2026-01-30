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

const dopplerAPIBaseURL = "https://api.doppler.com/v3"

// DopplerClient wraps the Doppler API for Poindexter's secrets management needs.
type DopplerClient struct {
	httpClient *http.Client
	token      string
}

// NewDopplerClient creates a new DopplerClient from configuration
func NewDopplerClient(config *DopplerConfig) *DopplerClient {
	if config == nil || config.Token == "" {
		return nil
	}

	return &DopplerClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: config.Token,
	}
}

// doRequest performs an HTTP request to the Doppler API
func (c *DopplerClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// dopplerErrorResponse represents a Doppler API error response
type dopplerErrorResponse struct {
	Messages []string `json:"messages"`
	Success  bool     `json:"success"`
}

// parseDopplerResponse reads and unmarshals a Doppler API response
func parseDopplerResponse[T any](resp *http.Response) (*T, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp dopplerErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("doppler API error (status %d): %s", resp.StatusCode, string(body))
		}
		if len(errResp.Messages) > 0 {
			return nil, fmt.Errorf("doppler API error: %s", errResp.Messages[0])
		}
		return nil, fmt.Errorf("doppler API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Project Types ---

// DopplerProject represents a Doppler project
type DopplerProject struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// DopplerProjectResponse wraps a single project response
type DopplerProjectResponse struct {
	Project DopplerProject `json:"project"`
}

// DopplerProjectsResponse wraps a list of projects response
type DopplerProjectsResponse struct {
	Projects []DopplerProject `json:"projects"`
	Page     int              `json:"page"`
	Success  bool             `json:"success"`
}

// --- Secret Types ---

// DopplerSecret represents a secret value
type DopplerSecret struct {
	Raw      string `json:"raw"`
	Computed string `json:"computed"`
}

// DopplerSecretsResponse wraps secrets download response
type DopplerSecretsResponse struct {
	Secrets map[string]DopplerSecret `json:"secrets"`
	Success bool                     `json:"success"`
}

// DopplerSecretsUpdateResponse wraps secrets update response
type DopplerSecretsUpdateResponse struct {
	Secrets map[string]DopplerSecret `json:"secrets"`
	Success bool                     `json:"success"`
}

// --- Workplace Types (for Ping) ---

// DopplerWorkplace represents workplace info
type DopplerWorkplace struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BillingEmail string `json:"billing_email"`
}

// DopplerMeResponse wraps the /me endpoint response
type DopplerMeResponse struct {
	Workplace DopplerWorkplace `json:"workplace"`
	Success   bool             `json:"success"`
}

// --- API Operations ---

// Ping verifies the Doppler connection by calling the /me endpoint
func (c *DopplerClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/me", dopplerAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("doppler ping failed: %w", err)
	}

	_, err = parseDopplerResponse[DopplerMeResponse](resp)
	return err
}

// --- Project Operations ---

// CreateProject creates a new Doppler project
func (c *DopplerClient) CreateProject(ctx context.Context, name, description string) (*DopplerProject, error) {
	reqURL := fmt.Sprintf("%s/projects", dopplerAPIBaseURL)

	reqBody := map[string]any{
		"name":        name,
		"description": description,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	result, err := parseDopplerResponse[DopplerProjectResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Project, nil
}

// GetProject retrieves a project by slug
func (c *DopplerClient) GetProject(ctx context.Context, projectSlug string) (*DopplerProject, error) {
	reqURL := fmt.Sprintf("%s/projects/project?project=%s", dopplerAPIBaseURL, url.QueryEscape(projectSlug))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	result, err := parseDopplerResponse[DopplerProjectResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Project, nil
}

// ListProjects lists all projects
func (c *DopplerClient) ListProjects(ctx context.Context) ([]*DopplerProject, error) {
	reqURL := fmt.Sprintf("%s/projects", dopplerAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	result, err := parseDopplerResponse[DopplerProjectsResponse](resp)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice for consistency
	projects := make([]*DopplerProject, len(result.Projects))
	for i := range result.Projects {
		projects[i] = &result.Projects[i]
	}

	return projects, nil
}

// DeleteProject deletes a project by slug
func (c *DopplerClient) DeleteProject(ctx context.Context, projectSlug string) error {
	reqURL := fmt.Sprintf("%s/projects/project", dopplerAPIBaseURL)

	reqBody := map[string]any{
		"project": projectSlug,
	}

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete project (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// --- Secrets Operations ---

// SetSecrets sets or updates secrets in a project config
// The secrets map contains secret names as keys and their values
func (c *DopplerClient) SetSecrets(ctx context.Context, project, config string, secrets map[string]string) error {
	reqURL := fmt.Sprintf("%s/configs/config/secrets", dopplerAPIBaseURL)

	reqBody := map[string]any{
		"project": project,
		"config":  config,
		"secrets": secrets,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to set secrets: %w", err)
	}

	_, err = parseDopplerResponse[DopplerSecretsUpdateResponse](resp)
	if err != nil {
		return err
	}

	return nil
}

// GetSecrets retrieves all secrets for a project config
// Returns a map of secret names to their computed values
func (c *DopplerClient) GetSecrets(ctx context.Context, project, config string) (map[string]string, error) {
	reqURL := fmt.Sprintf("%s/configs/config/secrets/download?project=%s&config=%s&format=json",
		dopplerAPIBaseURL, url.QueryEscape(project), url.QueryEscape(config))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("doppler API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Manual body handling required: the download endpoint returns a flat JSON object
	// of key-value pairs ({"SECRET_NAME": "value", ...}) rather than the standard
	// Doppler wrapper format used by other endpoints.
	var secrets map[string]string
	if err := json.Unmarshal(body, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets response: %w", err)
	}

	return secrets, nil
}

// DeleteSecret deletes a specific secret from a project config
func (c *DopplerClient) DeleteSecret(ctx context.Context, project, config, name string) error {
	reqURL := fmt.Sprintf("%s/configs/config/secret", dopplerAPIBaseURL)

	reqBody := map[string]any{
		"project": project,
		"config":  config,
		"name":    name,
	}

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete secret (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
