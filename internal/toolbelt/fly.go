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

const (
	flyAPIBaseURL      = "https://api.machines.dev/v1"
	flyGraphQLEndpoint = "https://api.fly.io/graphql"
)

// FlyClient wraps the Fly.io Machines API for Poindexter's needs
type FlyClient struct {
	httpClient    *http.Client
	token         string
	defaultRegion string
}

// NewFlyClient creates a new FlyClient from configuration
func NewFlyClient(config *FlyConfig) *FlyClient {
	if config == nil || config.Token == "" {
		return nil
	}

	return &FlyClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		token:         config.Token,
		defaultRegion: config.DefaultRegion,
	}
}

// doRequest performs an HTTP request to the Fly.io API
func (f *FlyClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")

	return f.httpClient.Do(req)
}

// parseResponse reads and unmarshals a JSON response body
func parseResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// Ping verifies the Fly.io connection by listing apps
func (f *FlyClient) Ping(ctx context.Context) error {
	// Use GraphQL to get the current user/organization info
	query := `query { viewer { email } }`
	resp, err := f.graphQL(ctx, query, nil)
	if err != nil {
		return fmt.Errorf("fly ping failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fly ping failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response and check for GraphQL errors
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fly ping failed: failed to read response: %w", err)
	}

	var result struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("fly ping failed: failed to parse response: %w", err)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("fly ping failed: %s", result.Errors[0].Message)
	}

	return nil
}

// graphQL performs a GraphQL request to Fly.io
func (f *FlyClient) graphQL(ctx context.Context, query string, variables map[string]any) (*http.Response, error) {
	payload := map[string]any{
		"query": query,
	}
	if variables != nil {
		payload["variables"] = variables
	}

	return f.doRequest(ctx, http.MethodPost, flyGraphQLEndpoint, payload)
}

// FlyApp represents a Fly.io application
type FlyApp struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Organization struct {
		Slug string `json:"slug"`
	} `json:"organization"`
	Status    string `json:"status"`
	Deployed  bool   `json:"deployed"`
	Hostname  string `json:"hostname"`
	AppURL    string `json:"appUrl"`
	CreatedAt string `json:"createdAt"`
}

// CreateAppOptions specifies options for creating a Fly.io app
type CreateAppOptions struct {
	Name   string `json:"app_name"`
	Org    string `json:"org_slug,omitempty"`
	Region string `json:"-"` // Used for initial machine placement, not in create request
}

// CreateApp creates a new Fly.io application
func (f *FlyClient) CreateApp(ctx context.Context, opts CreateAppOptions) (*FlyApp, error) {
	url := fmt.Sprintf("%s/apps", flyAPIBaseURL)

	reqBody := map[string]any{
		"app_name": opts.Name,
	}
	if opts.Org != "" {
		reqBody["org_slug"] = opts.Org
	}

	resp, err := f.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}

	app, err := parseResponse[FlyApp](resp)
	if err != nil {
		return nil, err // parseResponse already has context
	}

	return app, nil
}

// DeleteApp deletes a Fly.io application
func (f *FlyClient) DeleteApp(ctx context.Context, appName string) error {
	url := fmt.Sprintf("%s/apps/%s", flyAPIBaseURL, appName)

	resp, err := f.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete app (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetApp retrieves information about a Fly.io application
func (f *FlyClient) GetApp(ctx context.Context, appName string) (*FlyApp, error) {
	url := fmt.Sprintf("%s/apps/%s", flyAPIBaseURL, appName)

	resp, err := f.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	app, err := parseResponse[FlyApp](resp)
	if err != nil {
		return nil, err // parseResponse already has context
	}

	return app, nil
}

// FlyMachine represents a Fly.io machine (VM instance)
type FlyMachine struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Region     string `json:"region"`
	InstanceID string `json:"instance_id"`
	PrivateIP  string `json:"private_ip"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Config     struct {
		Image string `json:"image"`
		Env   map[string]string `json:"env"`
		Services []FlyService `json:"services"`
	} `json:"config"`
}

// FlyService represents a service configuration for a machine
type FlyService struct {
	Protocol     string `json:"protocol"`
	InternalPort int    `json:"internal_port"`
	Ports        []struct {
		Port     int      `json:"port"`
		Handlers []string `json:"handlers"`
	} `json:"ports"`
}

// DeployOptions specifies options for deploying to Fly.io
type DeployOptions struct {
	AppName      string
	Image        string            // Docker image to deploy
	Region       string            // Region for the machine (uses default if empty)
	Env          map[string]string // Environment variables
	InternalPort int               // Port the app listens on (default: 8080)
	CPUs         int               // Number of CPUs (default: 1)
	MemoryMB     int               // Memory in MB (default: 256)
}

// Deploy deploys a Docker image to a Fly.io app by creating a machine
func (f *FlyClient) Deploy(ctx context.Context, opts DeployOptions) (*FlyMachine, error) {
	region := opts.Region
	if region == "" {
		region = f.defaultRegion
	}
	if region == "" {
		region = "ord" // Default to Chicago
	}

	internalPort := opts.InternalPort
	if internalPort == 0 {
		internalPort = 8080
	}

	cpus := opts.CPUs
	if cpus == 0 {
		cpus = 1
	}

	memoryMB := opts.MemoryMB
	if memoryMB == 0 {
		memoryMB = 256
	}

	url := fmt.Sprintf("%s/apps/%s/machines", flyAPIBaseURL, opts.AppName)

	machineConfig := map[string]any{
		"image": opts.Image,
		"guest": map[string]any{
			"cpus":      cpus,
			"memory_mb": memoryMB,
		},
		"services": []map[string]any{
			{
				"protocol":      "tcp",
				"internal_port": internalPort,
				"ports": []map[string]any{
					{
						"port":     443,
						"handlers": []string{"tls", "http"},
					},
					{
						"port":     80,
						"handlers": []string{"http"},
					},
				},
			},
		},
	}

	if len(opts.Env) > 0 {
		machineConfig["env"] = opts.Env
	}

	reqBody := map[string]any{
		"region": region,
		"config": machineConfig,
	}

	resp, err := f.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy: %w", err)
	}

	machine, err := parseResponse[FlyMachine](resp)
	if err != nil {
		return nil, err // parseResponse already has context
	}

	return machine, nil
}

// SetSecrets sets secrets (environment variables) for a Fly.io app
// Note: Secrets are set via GraphQL and require a re-deploy to take effect
func (f *FlyClient) SetSecrets(ctx context.Context, appName string, secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}

	// Convert secrets map to GraphQL input format
	secretInputs := make([]map[string]string, 0, len(secrets))
	for key, value := range secrets {
		secretInputs = append(secretInputs, map[string]string{
			"key":   key,
			"value": value,
		})
	}

	query := `
		mutation SetSecrets($appId: String!, $secrets: [SecretInput!]!) {
			setSecrets(input: { appId: $appId, secrets: $secrets }) {
				app {
					name
				}
			}
		}
	`

	variables := map[string]any{
		"appId":   appName,
		"secrets": secretInputs,
	}

	resp, err := f.graphQL(ctx, query, variables)
	if err != nil {
		return fmt.Errorf("failed to set secrets: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set secrets (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// FlyAppStatus represents the status of a Fly.io application
type FlyAppStatus struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Status       string       `json:"status"`
	Deployed     bool         `json:"deployed"`
	MachineCount int          `json:"machine_count"`
	Machines     []FlyMachine `json:"machines"`
}

// GetStatus retrieves the status of a Fly.io application including its machines
func (f *FlyClient) GetStatus(ctx context.Context, appName string) (*FlyAppStatus, error) {
	// Get app info
	app, err := f.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}

	// Get machines for the app
	machines, err := f.listMachines(ctx, appName)
	if err != nil {
		return nil, err
	}

	return &FlyAppStatus{
		ID:           app.ID,
		Name:         app.Name,
		Status:       app.Status,
		Deployed:     app.Deployed,
		MachineCount: len(machines),
		Machines:     machines,
	}, nil
}

// listMachines lists all machines for an app
func (f *FlyClient) listMachines(ctx context.Context, appName string) ([]FlyMachine, error) {
	url := fmt.Sprintf("%s/apps/%s/machines", flyAPIBaseURL, appName)

	resp, err := f.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list machines: %w", err)
	}

	machines, err := parseResponse[[]FlyMachine](resp)
	if err != nil {
		return nil, err // parseResponse already has context
	}

	if machines == nil {
		return []FlyMachine{}, nil
	}
	return *machines, nil
}

// FlyLogEntry represents a single log entry from Fly.io
type FlyLogEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Instance  string `json:"instance"`
	Region    string `json:"region"`
}

// GetLogs retrieves recent logs for a Fly.io application
// Note: Fly.io logs are accessed via Nats, this uses the GraphQL API for basic log access
func (f *FlyClient) GetLogs(ctx context.Context, appName string, limit int) ([]FlyLogEntry, error) {
	if limit == 0 {
		limit = 100
	}

	query := `
		query GetLogs($appName: String!, $limit: Int) {
			app(name: $appName) {
				logs(limit: $limit) {
					nodes {
						timestamp
						message
						level
						instance
						region
					}
				}
			}
		}
	`

	variables := map[string]any{
		"appName": appName,
		"limit":   limit,
	}

	resp, err := f.graphQL(ctx, query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get logs (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			App struct {
				Logs struct {
					Nodes []FlyLogEntry `json:"nodes"`
				} `json:"logs"`
			} `json:"app"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse logs response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}

	return result.Data.App.Logs.Nodes, nil
}

// ScaleOptions specifies options for scaling a Fly.io app
type ScaleOptions struct {
	AppName  string
	Count    int    // Number of machines
	Region   string // Region to scale in (empty for all regions)
	CPUs     int    // CPUs per machine (0 to keep existing)
	MemoryMB int    // Memory per machine in MB (0 to keep existing)
}

// Scale scales a Fly.io application by adjusting machine count or size.
// When opts.Count is 0, all machines in the specified region (or all regions if empty)
// will be destroyed. This effectively stops the app from serving traffic.
func (f *FlyClient) Scale(ctx context.Context, opts ScaleOptions) error {
	machines, err := f.listMachines(ctx, opts.AppName)
	if err != nil {
		return err
	}

	// Filter machines by region if specified
	var targetMachines []FlyMachine
	for _, m := range machines {
		if opts.Region == "" || m.Region == opts.Region {
			targetMachines = append(targetMachines, m)
		}
	}

	currentCount := len(targetMachines)
	targetCount := opts.Count

	if targetCount == currentCount {
		// Just update existing machines if size options are specified
		if opts.CPUs > 0 || opts.MemoryMB > 0 {
			for _, m := range targetMachines {
				if err := f.updateMachineSize(ctx, opts.AppName, m.ID, opts.CPUs, opts.MemoryMB); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if targetCount > currentCount {
		// Scale up - create new machines
		region := opts.Region
		if region == "" {
			region = f.defaultRegion
		}
		if region == "" && len(targetMachines) > 0 {
			region = targetMachines[0].Region
		}
		if region == "" {
			region = "ord"
		}

		// Get config from existing machine if available
		var templateConfig map[string]any
		if len(targetMachines) > 0 {
			templateConfig = map[string]any{
				"image": targetMachines[0].Config.Image,
				"env":   targetMachines[0].Config.Env,
			}
		}

		for range targetCount - currentCount {
			if err := f.createMachineFromConfig(ctx, opts.AppName, region, templateConfig, opts.CPUs, opts.MemoryMB); err != nil {
				return fmt.Errorf("failed to scale up: %w", err)
			}
		}
	} else {
		// Scale down - stop and destroy machines
		toRemove := currentCount - targetCount
		for i := 0; i < toRemove && i < len(targetMachines); i++ {
			if err := f.destroyMachine(ctx, opts.AppName, targetMachines[i].ID); err != nil {
				return fmt.Errorf("failed to scale down: %w", err)
			}
		}
	}

	return nil
}

// updateMachineSize updates a machine's CPU and memory configuration
func (f *FlyClient) updateMachineSize(ctx context.Context, appName, machineID string, cpus, memoryMB int) error {
	// First get the current machine config
	url := fmt.Sprintf("%s/apps/%s/machines/%s", flyAPIBaseURL, appName, machineID)

	resp, err := f.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to get machine: %w", err)
	}

	machine, err := parseResponse[FlyMachine](resp)
	if err != nil {
		return err // parseResponse already has context
	}

	// Update the machine with new size
	guest := map[string]any{}
	if cpus > 0 {
		guest["cpus"] = cpus
	}
	if memoryMB > 0 {
		guest["memory_mb"] = memoryMB
	}

	updateReq := map[string]any{
		"config": map[string]any{
			"image": machine.Config.Image,
			"env":   machine.Config.Env,
			"guest": guest,
		},
	}

	resp, err = f.doRequest(ctx, http.MethodPost, url, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update machine (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// createMachineFromConfig creates a new machine with the given config
func (f *FlyClient) createMachineFromConfig(ctx context.Context, appName, region string, config map[string]any, cpus, memoryMB int) error {
	url := fmt.Sprintf("%s/apps/%s/machines", flyAPIBaseURL, appName)

	if config == nil {
		config = map[string]any{}
	}
	if config["image"] == nil || config["image"] == "" {
		return fmt.Errorf("cannot create machine: no image specified")
	}

	// Add guest config if specified
	guest := map[string]any{}
	if cpus > 0 {
		guest["cpus"] = cpus
	} else {
		guest["cpus"] = 1
	}
	if memoryMB > 0 {
		guest["memory_mb"] = memoryMB
	} else {
		guest["memory_mb"] = 256
	}
	config["guest"] = guest

	createReqBody := map[string]any{
		"region": region,
		"config": config,
	}

	resp, err := f.doRequest(ctx, http.MethodPost, url, createReqBody)
	if err != nil {
		return fmt.Errorf("failed to create machine: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create machine (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// destroyMachine stops and destroys a machine
func (f *FlyClient) destroyMachine(ctx context.Context, appName, machineID string) error {
	// First stop the machine
	stopURL := fmt.Sprintf("%s/apps/%s/machines/%s/stop", flyAPIBaseURL, appName, machineID)
	resp, err := f.doRequest(ctx, http.MethodPost, stopURL, nil)
	if err != nil {
		return fmt.Errorf("failed to stop machine: %w", err)
	}
	_ = resp.Body.Close()

	// Wait for machine to stop (poll status with timeout)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		url := fmt.Sprintf("%s/apps/%s/machines/%s", flyAPIBaseURL, appName, machineID)
		resp, err := f.doRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			break // Machine might already be gone
		}
		machine, err := parseResponse[FlyMachine](resp)
		if err != nil || machine.State == "stopped" || machine.State == "destroyed" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Then destroy it
	destroyURL := fmt.Sprintf("%s/apps/%s/machines/%s?force=true", flyAPIBaseURL, appName, machineID)
	resp, err = f.doRequest(ctx, http.MethodDelete, destroyURL, nil)
	if err != nil {
		return fmt.Errorf("failed to destroy machine: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to destroy machine (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
