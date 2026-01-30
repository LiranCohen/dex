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
	betterStackUptimeAPIBaseURL    = "https://uptime.betterstack.com/api/v2"
	betterStackTelemetryAPIBaseURL = "https://telemetry.betterstack.com/api/v1"
)

// BetterStackClient wraps the Better Stack API for Poindexter's monitoring and logging needs.
type BetterStackClient struct {
	httpClient *http.Client
	apiToken   string
}

// NewBetterStackClient creates a new BetterStackClient from configuration
func NewBetterStackClient(config *BetterStackConfig) *BetterStackClient {
	if config == nil || config.APIToken == "" {
		return nil
	}

	return &BetterStackClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiToken: config.APIToken,
	}
}

// doRequest performs an HTTP request to the Better Stack API
func (c *BetterStackClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// betterStackErrorResponse represents a Better Stack API error response
type betterStackErrorResponse struct {
	Errors []string `json:"errors"`
}

// parseBetterStackResponse reads and unmarshals a Better Stack API response
func parseBetterStackResponse[T any](resp *http.Response) (*T, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp betterStackErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("better stack API error (status %d): %s", resp.StatusCode, string(body))
		}
		if len(errResp.Errors) > 0 {
			return nil, fmt.Errorf("better stack API error: %s", errResp.Errors[0])
		}
		return nil, fmt.Errorf("better stack API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Monitor Types ---

// BetterStackMonitor represents an uptime monitor
type BetterStackMonitor struct {
	ID         string                      `json:"id"`
	Type       string                      `json:"type"` // "monitor"
	Attributes BetterStackMonitorAttributes `json:"attributes"`
}

// BetterStackMonitorAttributes contains monitor configuration and status
type BetterStackMonitorAttributes struct {
	URL                 string   `json:"url"`
	PronounceableName   string   `json:"pronounceable_name,omitempty"`
	MonitorType         string   `json:"monitor_type"` // status, keyword, ping, tcp, udp, smtp, pop, imap, dns, playwright
	CheckFrequency      int      `json:"check_frequency,omitempty"` // seconds
	RequestTimeout      int      `json:"request_timeout,omitempty"`
	Call                bool     `json:"call,omitempty"`
	SMS                 bool     `json:"sms,omitempty"`
	Email               bool     `json:"email,omitempty"`
	Push                bool     `json:"push,omitempty"`
	VerifySSL           bool     `json:"verify_ssl,omitempty"`
	RequiredKeyword     string   `json:"required_keyword,omitempty"`
	ExpectedStatusCodes []int    `json:"expected_status_codes,omitempty"`
	Regions             []string `json:"regions,omitempty"`
	RequestHeaders      []string `json:"request_headers,omitempty"`
	RequestBody         string   `json:"request_body,omitempty"`
	Status              string   `json:"status,omitempty"` // up, down, validating, paused, pending, maintenance
	LastCheckedAt       string   `json:"last_checked_at,omitempty"`
	Paused              bool     `json:"paused,omitempty"`
	CreatedAt           string   `json:"created_at,omitempty"`
	UpdatedAt           string   `json:"updated_at,omitempty"`
}

// BetterStackMonitorResponse wraps a single monitor response
type BetterStackMonitorResponse struct {
	Data BetterStackMonitor `json:"data"`
}

// BetterStackMonitorsResponse wraps a list of monitors response
type BetterStackMonitorsResponse struct {
	Data       []BetterStackMonitor `json:"data"`
	Pagination *Pagination          `json:"pagination,omitempty"`
}

// Pagination represents API pagination info
type Pagination struct {
	First string `json:"first,omitempty"`
	Last  string `json:"last,omitempty"`
	Prev  string `json:"prev,omitempty"`
	Next  string `json:"next,omitempty"`
}

// --- Source Types ---

// BetterStackSource represents a log source
type BetterStackSource struct {
	ID         string                      `json:"id"`
	Type       string                      `json:"type,omitempty"` // "source"
	Attributes BetterStackSourceAttributes `json:"attributes"`
}

// BetterStackSourceAttributes contains source configuration
type BetterStackSourceAttributes struct {
	Name              string `json:"name"`
	Platform          string `json:"platform"` // nginx, apache2, docker, kubernetes, http, prometheus, etc.
	TableName         string `json:"table_name,omitempty"`
	Token             string `json:"token,omitempty"` // ingestion token
	IngestingHost     string `json:"ingesting_host,omitempty"`
	IngestingPaused   bool   `json:"ingesting_paused,omitempty"`
	LogsRetention     int    `json:"logs_retention,omitempty"`  // days
	MetricsRetention  int    `json:"metrics_retention,omitempty"` // days
	VRLTransformation string `json:"vrl_transformation,omitempty"`
	LiveTailPattern   string `json:"live_tail_pattern,omitempty"`
	SourceGroupID     string `json:"source_group_id,omitempty"`
	DataRegion        string `json:"data_region,omitempty"` // us_east, germany, singapore
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

// BetterStackSourceResponse wraps a single source response
type BetterStackSourceResponse struct {
	Data BetterStackSource `json:"data"`
}

// BetterStackSourcesResponse wraps a list of sources response
type BetterStackSourcesResponse struct {
	Data       []BetterStackSource `json:"data"`
	Pagination *Pagination         `json:"pagination,omitempty"`
}

// --- Monitor Operations ---

// Ping verifies the Better Stack connection by listing monitors
func (c *BetterStackClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/monitors?per_page=1", betterStackUptimeAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("better stack ping failed: %w", err)
	}

	_, err = parseBetterStackResponse[BetterStackMonitorsResponse](resp)
	if err != nil {
		return fmt.Errorf("better stack ping failed: %w", err)
	}

	return nil
}

// CreateMonitorOptions specifies options for creating a monitor
type CreateMonitorOptions struct {
	URL                 string   // URL to monitor (required)
	Name                string   // Pronounceable name for voice alerts
	MonitorType         string   // status, keyword, ping, tcp, udp, smtp, pop, imap, dns (default: status)
	CheckFrequency      int      // Check interval in seconds (default: 180)
	RequestTimeout      int      // Timeout in seconds
	RequiredKeyword     string   // For keyword monitors
	ExpectedStatusCodes []int    // Acceptable HTTP status codes
	VerifySSL           bool     // Verify SSL certificates
	Email               bool     // Send email notifications
	SMS                 bool     // Send SMS notifications
	Call                bool     // Send phone call notifications
	Push                bool     // Send push notifications
	Regions             []string // Geographic regions to monitor from
}

// CreateMonitor creates a new uptime monitor
func (c *BetterStackClient) CreateMonitor(ctx context.Context, opts CreateMonitorOptions) (*BetterStackMonitor, error) {
	reqURL := fmt.Sprintf("%s/monitors", betterStackUptimeAPIBaseURL)

	monitorType := opts.MonitorType
	if monitorType == "" {
		monitorType = "status"
	}

	checkFrequency := opts.CheckFrequency
	if checkFrequency == 0 {
		checkFrequency = 180 // 3 minutes default
	}

	reqBody := map[string]any{
		"url":           opts.URL,
		"monitor_type":  monitorType,
		"check_frequency": checkFrequency,
	}

	if opts.Name != "" {
		reqBody["pronounceable_name"] = opts.Name
	}
	if opts.RequestTimeout > 0 {
		reqBody["request_timeout"] = opts.RequestTimeout
	}
	if opts.RequiredKeyword != "" {
		reqBody["required_keyword"] = opts.RequiredKeyword
	}
	if len(opts.ExpectedStatusCodes) > 0 {
		reqBody["expected_status_codes"] = opts.ExpectedStatusCodes
	}
	if opts.VerifySSL {
		reqBody["verify_ssl"] = true
	}
	if opts.Email {
		reqBody["email"] = true
	}
	if opts.SMS {
		reqBody["sms"] = true
	}
	if opts.Call {
		reqBody["call"] = true
	}
	if opts.Push {
		reqBody["push"] = true
	}
	if len(opts.Regions) > 0 {
		reqBody["regions"] = opts.Regions
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitor: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackMonitorResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// GetMonitor retrieves a monitor by ID
func (c *BetterStackClient) GetMonitor(ctx context.Context, monitorID string) (*BetterStackMonitor, error) {
	reqURL := fmt.Sprintf("%s/monitors/%s", betterStackUptimeAPIBaseURL, monitorID)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackMonitorResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// ListMonitors lists all monitors
func (c *BetterStackClient) ListMonitors(ctx context.Context) ([]*BetterStackMonitor, error) {
	reqURL := fmt.Sprintf("%s/monitors", betterStackUptimeAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list monitors: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackMonitorsResponse](resp)
	if err != nil {
		return nil, err
	}

	monitors := make([]*BetterStackMonitor, len(result.Data))
	for i := range result.Data {
		monitors[i] = &result.Data[i]
	}

	return monitors, nil
}

// UpdateMonitor updates an existing monitor
func (c *BetterStackClient) UpdateMonitor(ctx context.Context, monitorID string, opts CreateMonitorOptions) (*BetterStackMonitor, error) {
	reqURL := fmt.Sprintf("%s/monitors/%s", betterStackUptimeAPIBaseURL, monitorID)

	reqBody := make(map[string]any)

	if opts.URL != "" {
		reqBody["url"] = opts.URL
	}
	if opts.Name != "" {
		reqBody["pronounceable_name"] = opts.Name
	}
	if opts.MonitorType != "" {
		reqBody["monitor_type"] = opts.MonitorType
	}
	if opts.CheckFrequency > 0 {
		reqBody["check_frequency"] = opts.CheckFrequency
	}
	if opts.RequestTimeout > 0 {
		reqBody["request_timeout"] = opts.RequestTimeout
	}
	if opts.RequiredKeyword != "" {
		reqBody["required_keyword"] = opts.RequiredKeyword
	}
	if len(opts.ExpectedStatusCodes) > 0 {
		reqBody["expected_status_codes"] = opts.ExpectedStatusCodes
	}

	// Boolean fields - always set if explicitly true
	if opts.VerifySSL {
		reqBody["verify_ssl"] = true
	}
	if opts.Email {
		reqBody["email"] = true
	}
	if opts.SMS {
		reqBody["sms"] = true
	}
	if opts.Call {
		reqBody["call"] = true
	}
	if opts.Push {
		reqBody["push"] = true
	}
	if len(opts.Regions) > 0 {
		reqBody["regions"] = opts.Regions
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update monitor: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackMonitorResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// DeleteMonitor removes a monitor
func (c *BetterStackClient) DeleteMonitor(ctx context.Context, monitorID string) error {
	reqURL := fmt.Sprintf("%s/monitors/%s", betterStackUptimeAPIBaseURL, monitorID)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to delete monitor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete monitor (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// PauseMonitor pauses a monitor
func (c *BetterStackClient) PauseMonitor(ctx context.Context, monitorID string) (*BetterStackMonitor, error) {
	reqURL := fmt.Sprintf("%s/monitors/%s", betterStackUptimeAPIBaseURL, monitorID)

	resp, err := c.doRequest(ctx, http.MethodPatch, reqURL, map[string]any{"paused": true})
	if err != nil {
		return nil, fmt.Errorf("failed to pause monitor: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackMonitorResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// ResumeMonitor resumes a paused monitor
func (c *BetterStackClient) ResumeMonitor(ctx context.Context, monitorID string) (*BetterStackMonitor, error) {
	reqURL := fmt.Sprintf("%s/monitors/%s", betterStackUptimeAPIBaseURL, monitorID)

	resp, err := c.doRequest(ctx, http.MethodPatch, reqURL, map[string]any{"paused": false})
	if err != nil {
		return nil, fmt.Errorf("failed to resume monitor: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackMonitorResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// --- Source Operations (Logs/Telemetry) ---

// CreateSourceOptions specifies options for creating a log source
type CreateSourceOptions struct {
	Name              string // Source name (required)
	Platform          string // Platform type: nginx, apache2, docker, kubernetes, http, prometheus, etc. (required)
	DataRegion        string // Region: us_east, germany, singapore (optional)
	IngestingPaused   bool   // Pause ingestion (optional)
	SourceGroupID     string // Group to assign to (optional)
	LogsRetention     int    // Days to retain logs (optional)
	MetricsRetention  int    // Days to retain metrics (optional)
	LiveTailPattern   string // Format for live tail (optional)
	VRLTransformation string // VRL code for transformation (optional)
}

// CreateSource creates a new log source
func (c *BetterStackClient) CreateSource(ctx context.Context, opts CreateSourceOptions) (*BetterStackSource, error) {
	reqURL := fmt.Sprintf("%s/sources", betterStackTelemetryAPIBaseURL)

	reqBody := map[string]any{
		"name":     opts.Name,
		"platform": opts.Platform,
	}

	if opts.DataRegion != "" {
		reqBody["data_region"] = opts.DataRegion
	}
	if opts.IngestingPaused {
		reqBody["ingesting_paused"] = true
	}
	if opts.SourceGroupID != "" {
		reqBody["source_group_id"] = opts.SourceGroupID
	}
	if opts.LogsRetention > 0 {
		reqBody["logs_retention"] = opts.LogsRetention
	}
	if opts.MetricsRetention > 0 {
		reqBody["metrics_retention"] = opts.MetricsRetention
	}
	if opts.LiveTailPattern != "" {
		reqBody["live_tail_pattern"] = opts.LiveTailPattern
	}
	if opts.VRLTransformation != "" {
		reqBody["vrl_transformation"] = opts.VRLTransformation
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackSourceResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// GetSource retrieves a source by ID
func (c *BetterStackClient) GetSource(ctx context.Context, sourceID string) (*BetterStackSource, error) {
	reqURL := fmt.Sprintf("%s/sources/%s", betterStackTelemetryAPIBaseURL, sourceID)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackSourceResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// ListSources lists all log sources
func (c *BetterStackClient) ListSources(ctx context.Context) ([]*BetterStackSource, error) {
	reqURL := fmt.Sprintf("%s/sources", betterStackTelemetryAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackSourcesResponse](resp)
	if err != nil {
		return nil, err
	}

	sources := make([]*BetterStackSource, len(result.Data))
	for i := range result.Data {
		sources[i] = &result.Data[i]
	}

	return sources, nil
}

// UpdateSource updates an existing source
func (c *BetterStackClient) UpdateSource(ctx context.Context, sourceID string, opts CreateSourceOptions) (*BetterStackSource, error) {
	reqURL := fmt.Sprintf("%s/sources/%s", betterStackTelemetryAPIBaseURL, sourceID)

	reqBody := make(map[string]any)

	if opts.Name != "" {
		reqBody["name"] = opts.Name
	}
	if opts.Platform != "" {
		reqBody["platform"] = opts.Platform
	}
	if opts.DataRegion != "" {
		reqBody["data_region"] = opts.DataRegion
	}
	if opts.IngestingPaused {
		reqBody["ingesting_paused"] = true
	}
	if opts.SourceGroupID != "" {
		reqBody["source_group_id"] = opts.SourceGroupID
	}
	if opts.LogsRetention > 0 {
		reqBody["logs_retention"] = opts.LogsRetention
	}
	if opts.MetricsRetention > 0 {
		reqBody["metrics_retention"] = opts.MetricsRetention
	}
	if opts.LiveTailPattern != "" {
		reqBody["live_tail_pattern"] = opts.LiveTailPattern
	}
	if opts.VRLTransformation != "" {
		reqBody["vrl_transformation"] = opts.VRLTransformation
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update source: %w", err)
	}

	result, err := parseBetterStackResponse[BetterStackSourceResponse](resp)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// DeleteSource removes a source
func (c *BetterStackClient) DeleteSource(ctx context.Context, sourceID string) error {
	reqURL := fmt.Sprintf("%s/sources/%s", betterStackTelemetryAPIBaseURL, sourceID)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete source (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
