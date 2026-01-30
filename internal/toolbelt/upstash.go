// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	upstashAPIBaseURL   = "https://api.upstash.com/v2"
	upstashQStashAPIURL = "https://qstash.upstash.io/v2"
)

// UpstashClient wraps the Upstash API for Poindexter's needs.
// Supports both Redis database management and QStash queue operations.
type UpstashClient struct {
	httpClient  *http.Client
	email       string
	apiKey      string
	basicAuth   string // base64 encoded email:api_key for Redis API
	qstashToken string // Optional, for QStash operations
}

// NewUpstashClient creates a new UpstashClient from configuration
func NewUpstashClient(config *UpstashConfig) *UpstashClient {
	if config == nil || config.Email == "" || config.APIKey == "" {
		return nil
	}

	auth := base64.StdEncoding.EncodeToString([]byte(config.Email + ":" + config.APIKey))

	return &UpstashClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		email:       config.Email,
		apiKey:      config.APIKey,
		basicAuth:   auth,
		qstashToken: config.QStashToken,
	}
}

// doRequest performs an HTTP request to the Upstash Redis Management API
func (c *UpstashClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

	req.Header.Set("Authorization", "Basic "+c.basicAuth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// doQStashRequest performs an HTTP request to the QStash API
func (c *UpstashClient) doQStashRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	if c.qstashToken == "" {
		return nil, fmt.Errorf("QStash token not configured")
	}

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

	req.Header.Set("Authorization", "Bearer "+c.qstashToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// upstashErrorResponse represents an Upstash API error response
type upstashErrorResponse struct {
	Error string `json:"error"`
}

// parseUpstashResponse reads and unmarshals an Upstash API response
func parseUpstashResponse[T any](resp *http.Response) (*T, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp upstashErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("upstash API error (status %d): %s", resp.StatusCode, string(body))
		}
		if errResp.Error != "" {
			return nil, fmt.Errorf("upstash API error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("upstash API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Redis Database Types ---

// UpstashRedisDatabase represents an Upstash Redis database
type UpstashRedisDatabase struct {
	DatabaseID        string `json:"database_id"`
	DatabaseName      string `json:"database_name"`
	DatabaseType      string `json:"database_type"`       // "Pay as You Go"
	Region            string `json:"region"`              // "global", "us-east-1", etc.
	Type              string `json:"type"`                // "paid", "free"
	Port              int    `json:"port"`
	CreationTime      int64  `json:"creation_time"`
	State             string `json:"state"`               // "active"
	Password          string `json:"password"`
	UserEmail         string `json:"user_email"`
	Endpoint          string `json:"endpoint"`            // Redis endpoint
	TLS               bool   `json:"tls"`
	RestToken         string `json:"rest_token"`          // For REST API access
	ReadOnlyRestToken string `json:"read_only_rest_token"`
}

// --- QStash Queue Types ---

// UpstashQStashQueue represents a QStash queue
type UpstashQStashQueue struct {
	Name        string    `json:"name"`
	Parallelism int       `json:"parallelism"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Paused      bool      `json:"paused"`
}

// --- Redis Database Operations ---

// Ping verifies the Upstash connection by listing databases
func (c *UpstashClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/redis/databases", upstashAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("upstash ping failed: %w", err)
	}

	_, err = parseUpstashResponse[[]UpstashRedisDatabase](resp)
	if err != nil {
		return fmt.Errorf("upstash ping failed: %w", err)
	}

	return nil
}

// PingQStash verifies the QStash connection by listing queues
func (c *UpstashClient) PingQStash(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/queues", upstashQStashAPIURL)

	resp, err := c.doQStashRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("qstash ping failed: %w", err)
	}

	_, err = parseUpstashResponse[[]UpstashQStashQueue](resp)
	if err != nil {
		return fmt.Errorf("qstash ping failed: %w", err)
	}

	return nil
}

// CreateRedisOptions specifies options for creating an Upstash Redis database
type CreateRedisOptions struct {
	Name   string // Database name (required)
	Region string // Region: "global", "us-east-1", "eu-west-1", etc. (default: "global")
	TLS    *bool  // Enable TLS (default: true when nil)
}

// CreateRedis creates a new Upstash Redis database
func (c *UpstashClient) CreateRedis(ctx context.Context, opts CreateRedisOptions) (*UpstashRedisDatabase, error) {
	reqURL := fmt.Sprintf("%s/redis/database", upstashAPIBaseURL)

	region := opts.Region
	if region == "" {
		region = "global"
	}

	// Default TLS to true when not explicitly set
	tls := true
	if opts.TLS != nil {
		tls = *opts.TLS
	}

	reqBody := map[string]any{
		"name":   opts.Name,
		"region": region,
		"tls":    tls,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis database: %w", err)
	}

	result, err := parseUpstashResponse[UpstashRedisDatabase](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetRedis retrieves an Upstash Redis database by ID
func (c *UpstashClient) GetRedis(ctx context.Context, databaseID string) (*UpstashRedisDatabase, error) {
	reqURL := fmt.Sprintf("%s/redis/database/%s", upstashAPIBaseURL, databaseID)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get redis database: %w", err)
	}

	result, err := parseUpstashResponse[UpstashRedisDatabase](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListRedis lists all Upstash Redis databases
func (c *UpstashClient) ListRedis(ctx context.Context) ([]*UpstashRedisDatabase, error) {
	reqURL := fmt.Sprintf("%s/redis/databases", upstashAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list redis databases: %w", err)
	}

	result, err := parseUpstashResponse[[]UpstashRedisDatabase](resp)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice for consistency
	databases := make([]*UpstashRedisDatabase, len(*result))
	for i := range *result {
		databases[i] = &(*result)[i]
	}

	return databases, nil
}

// DeleteRedis deletes an Upstash Redis database
func (c *UpstashClient) DeleteRedis(ctx context.Context, databaseID string) error {
	reqURL := fmt.Sprintf("%s/redis/database/%s", upstashAPIBaseURL, databaseID)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to delete redis database: %w", err)
	}

	// Upstash returns the deleted database info on success
	_, err = parseUpstashResponse[UpstashRedisDatabase](resp)
	if err != nil {
		return err
	}

	return nil
}

// RenameRedis renames an Upstash Redis database
func (c *UpstashClient) RenameRedis(ctx context.Context, databaseID, newName string) error {
	reqURL := fmt.Sprintf("%s/redis/rename/%s", upstashAPIBaseURL, databaseID)

	reqBody := map[string]any{
		"name": newName,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to rename redis database: %w", err)
	}

	_, err = parseUpstashResponse[UpstashRedisDatabase](resp)
	if err != nil {
		return err
	}

	return nil
}

// --- Credentials Helper ---

// RedisCredentials contains connection information for an Upstash Redis database
type RedisCredentials struct {
	RedisURL  string // Redis connection URL (rediss:// for TLS, redis:// otherwise)
	RestURL   string // REST API URL (https://{endpoint})
	RestToken string // REST API token for authentication
}

// GetCredentials retrieves connection credentials for an Upstash Redis database
func (c *UpstashClient) GetCredentials(ctx context.Context, databaseID string) (*RedisCredentials, error) {
	db, err := c.GetRedis(ctx, databaseID)
	if err != nil {
		return nil, err
	}

	// Build Redis URL
	// Format: redis[s]://default:{password}@{endpoint}:{port}
	scheme := "redis"
	if db.TLS {
		scheme = "rediss"
	}

	redisURL := fmt.Sprintf("%s://default:%s@%s:%d",
		scheme,
		url.QueryEscape(db.Password),
		db.Endpoint,
		db.Port,
	)

	// Build REST URL
	restURL := fmt.Sprintf("https://%s", db.Endpoint)

	return &RedisCredentials{
		RedisURL:  redisURL,
		RestURL:   restURL,
		RestToken: db.RestToken,
	}, nil
}

// --- QStash Queue Operations ---

// CreateQStashQueue creates a new QStash queue
func (c *UpstashClient) CreateQStashQueue(ctx context.Context, name string, parallelism int) (*UpstashQStashQueue, error) {
	reqURL := fmt.Sprintf("%s/queues", upstashQStashAPIURL)

	if parallelism < 1 {
		parallelism = 1
	}

	reqBody := map[string]any{
		"queueName":   name,
		"parallelism": parallelism,
	}

	resp, err := c.doQStashRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create qstash queue: %w", err)
	}

	result, err := parseUpstashResponse[UpstashQStashQueue](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetQStashQueue retrieves a QStash queue by name
func (c *UpstashClient) GetQStashQueue(ctx context.Context, name string) (*UpstashQStashQueue, error) {
	reqURL := fmt.Sprintf("%s/queues/%s", upstashQStashAPIURL, url.PathEscape(name))

	resp, err := c.doQStashRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get qstash queue: %w", err)
	}

	result, err := parseUpstashResponse[UpstashQStashQueue](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListQStashQueues lists all QStash queues
func (c *UpstashClient) ListQStashQueues(ctx context.Context) ([]*UpstashQStashQueue, error) {
	reqURL := fmt.Sprintf("%s/queues", upstashQStashAPIURL)

	resp, err := c.doQStashRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list qstash queues: %w", err)
	}

	result, err := parseUpstashResponse[[]UpstashQStashQueue](resp)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice for consistency
	queues := make([]*UpstashQStashQueue, len(*result))
	for i := range *result {
		queues[i] = &(*result)[i]
	}

	return queues, nil
}

// DeleteQStashQueue deletes a QStash queue
func (c *UpstashClient) DeleteQStashQueue(ctx context.Context, name string) error {
	reqURL := fmt.Sprintf("%s/queues/%s", upstashQStashAPIURL, url.PathEscape(name))

	resp, err := c.doQStashRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to delete qstash queue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete qstash queue (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// PauseQStashQueue pauses a QStash queue
func (c *UpstashClient) PauseQStashQueue(ctx context.Context, name string) error {
	reqURL := fmt.Sprintf("%s/queues/%s/pause", upstashQStashAPIURL, url.PathEscape(name))

	resp, err := c.doQStashRequest(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to pause qstash queue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to pause qstash queue (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ResumeQStashQueue resumes a paused QStash queue
func (c *UpstashClient) ResumeQStashQueue(ctx context.Context, name string) error {
	reqURL := fmt.Sprintf("%s/queues/%s/resume", upstashQStashAPIURL, url.PathEscape(name))

	resp, err := c.doQStashRequest(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to resume qstash queue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to resume qstash queue (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateQStashQueueOptions specifies options for updating a QStash queue
type UpdateQStashQueueOptions struct {
	Parallelism int  // New parallelism value (optional, 0 means no change)
	Paused      *bool // Set paused state (optional, nil means no change)
}

// UpdateQStashQueue updates a QStash queue's settings
func (c *UpstashClient) UpdateQStashQueue(ctx context.Context, name string, opts UpdateQStashQueueOptions) (*UpstashQStashQueue, error) {
	reqURL := fmt.Sprintf("%s/queues/%s", upstashQStashAPIURL, url.PathEscape(name))

	reqBody := map[string]any{}
	if opts.Parallelism > 0 {
		reqBody["parallelism"] = opts.Parallelism
	}
	if opts.Paused != nil {
		reqBody["paused"] = *opts.Paused
	}

	resp, err := c.doQStashRequest(ctx, http.MethodPut, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update qstash queue: %w", err)
	}

	result, err := parseUpstashResponse[UpstashQStashQueue](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}
