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

const resendAPIBaseURL = "https://api.resend.com"

// ResendClient wraps the Resend API for Poindexter's transactional email needs.
type ResendClient struct {
	httpClient *http.Client
	apiKey     string
}

// NewResendClient creates a new ResendClient from configuration
func NewResendClient(config *ResendConfig) *ResendClient {
	if config == nil || config.APIKey == "" {
		return nil
	}

	return &ResendClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey: config.APIKey,
	}
}

// doRequest performs an HTTP request to the Resend API
func (c *ResendClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

// resendErrorResponse represents a Resend API error response
type resendErrorResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Name       string `json:"name"`
}

// parseResendResponse reads and unmarshals a Resend API response
func parseResendResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp resendErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("resend API error (status %d): %s", resp.StatusCode, string(body))
		}
		if errResp.Message != "" {
			return nil, fmt.Errorf("resend API error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("resend API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Email Types ---

// ResendEmail represents an email to send via Resend
type ResendEmail struct {
	From        string            `json:"from"`                   // Sender email address
	To          []string          `json:"to"`                     // Recipient email addresses
	Subject     string            `json:"subject"`                // Email subject
	HTML        string            `json:"html,omitempty"`         // HTML body
	Text        string            `json:"text,omitempty"`         // Plain text body
	CC          []string          `json:"cc,omitempty"`           // CC recipients
	BCC         []string          `json:"bcc,omitempty"`          // BCC recipients
	ReplyTo     []string          `json:"reply_to,omitempty"`     // Reply-to addresses
	Headers     map[string]string `json:"headers,omitempty"`      // Custom headers
	Attachments []ResendAttachment `json:"attachments,omitempty"` // File attachments
	Tags        []ResendTag       `json:"tags,omitempty"`         // Tags for tracking
}

// ResendAttachment represents an email attachment
type ResendAttachment struct {
	Filename    string `json:"filename"`              // Attachment filename
	Content     string `json:"content,omitempty"`     // Base64 encoded content
	Path        string `json:"path,omitempty"`        // URL to fetch content from
	ContentType string `json:"content_type,omitempty"` // MIME type
}

// ResendTag represents an email tag for tracking
type ResendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ResendEmailResponse represents the response from sending an email
type ResendEmailResponse struct {
	ID string `json:"id"` // Email ID for tracking
}

// ResendEmailInfo represents detailed email information
type ResendEmailInfo struct {
	ID        string    `json:"id"`
	Object    string    `json:"object"`    // "email"
	To        []string  `json:"to"`
	From      string    `json:"from"`
	Subject   string    `json:"subject"`
	HTML      string    `json:"html,omitempty"`
	Text      string    `json:"text,omitempty"`
	BCC       []string  `json:"bcc,omitempty"`
	CC        []string  `json:"cc,omitempty"`
	ReplyTo   []string  `json:"reply_to,omitempty"`
	LastEvent string    `json:"last_event"` // "delivered", "bounced", etc.
	CreatedAt time.Time `json:"created_at"`
}

// --- Domain Types ---

// ResendDomain represents a verified sending domain
type ResendDomain struct {
	ID        string            `json:"id"`
	Object    string            `json:"object"` // "domain"
	Name      string            `json:"name"`   // Domain name
	Status    string            `json:"status"` // "not_started", "pending", "verified", "failed"
	CreatedAt time.Time         `json:"created_at"`
	Region    string            `json:"region"` // "us-east-1", "eu-west-1", etc.
	Records   []ResendDNSRecord `json:"records,omitempty"`
}

// ResendDNSRecord represents a DNS record required for domain verification
type ResendDNSRecord struct {
	Record   string `json:"record"`   // "SPF", "DKIM", "DMARC"
	Name     string `json:"name"`     // DNS record name
	Type     string `json:"type"`     // "TXT", "CNAME", "MX"
	TTL      string `json:"ttl"`      // Time to live
	Status   string `json:"status"`   // "not_started", "pending", "verified", "failed"
	Value    string `json:"value"`    // DNS record value
	Priority int    `json:"priority,omitempty"` // For MX records
}

// ResendDomainsResponse represents the response from listing domains
type ResendDomainsResponse struct {
	Data []ResendDomain `json:"data"`
}

// --- API Key Types ---

// ResendAPIKey represents an API key
type ResendAPIKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Token     string    `json:"token,omitempty"` // Only returned on creation
}

// ResendAPIKeysResponse represents the response from listing API keys
type ResendAPIKeysResponse struct {
	Data []ResendAPIKey `json:"data"`
}

// --- Email Operations ---

// Ping verifies the Resend connection by listing domains
func (c *ResendClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/domains", resendAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("resend ping failed: %w", err)
	}

	_, err = parseResendResponse[ResendDomainsResponse](resp)
	if err != nil {
		return fmt.Errorf("resend ping failed: %w", err)
	}

	return nil
}

// SendEmail sends an email via Resend
func (c *ResendClient) SendEmail(ctx context.Context, email ResendEmail) (*ResendEmailResponse, error) {
	reqURL := fmt.Sprintf("%s/emails", resendAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, email)
	if err != nil {
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	result, err := parseResendResponse[ResendEmailResponse](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetEmail retrieves information about a sent email
func (c *ResendClient) GetEmail(ctx context.Context, emailID string) (*ResendEmailInfo, error) {
	reqURL := fmt.Sprintf("%s/emails/%s", resendAPIBaseURL, emailID)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get email: %w", err)
	}

	result, err := parseResendResponse[ResendEmailInfo](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// --- Domain Operations ---

// CreateDomainOptions specifies options for creating a domain
type CreateDomainOptions struct {
	Name   string // Domain name (required)
	Region string // Region: "us-east-1", "eu-west-1", "sa-east-1" (default: "us-east-1")
}

// CreateDomain adds a new sending domain
func (c *ResendClient) CreateDomain(ctx context.Context, opts CreateDomainOptions) (*ResendDomain, error) {
	reqURL := fmt.Sprintf("%s/domains", resendAPIBaseURL)

	region := opts.Region
	if region == "" {
		region = "us-east-1"
	}

	reqBody := map[string]any{
		"name":   opts.Name,
		"region": region,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	result, err := parseResendResponse[ResendDomain](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetDomain retrieves a domain by ID
func (c *ResendClient) GetDomain(ctx context.Context, domainID string) (*ResendDomain, error) {
	reqURL := fmt.Sprintf("%s/domains/%s", resendAPIBaseURL, domainID)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain: %w", err)
	}

	result, err := parseResendResponse[ResendDomain](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListDomains lists all verified domains
func (c *ResendClient) ListDomains(ctx context.Context) ([]*ResendDomain, error) {
	reqURL := fmt.Sprintf("%s/domains", resendAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	result, err := parseResendResponse[ResendDomainsResponse](resp)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice for consistency
	domains := make([]*ResendDomain, len(result.Data))
	for i := range result.Data {
		domains[i] = &result.Data[i]
	}

	return domains, nil
}

// VerifyDomain initiates domain verification
func (c *ResendClient) VerifyDomain(ctx context.Context, domainID string) (*ResendDomain, error) {
	reqURL := fmt.Sprintf("%s/domains/%s/verify", resendAPIBaseURL, domainID)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to verify domain: %w", err)
	}

	result, err := parseResendResponse[ResendDomain](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteDomain removes a sending domain
func (c *ResendClient) DeleteDomain(ctx context.Context, domainID string) error {
	reqURL := fmt.Sprintf("%s/domains/%s", resendAPIBaseURL, domainID)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete domain (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// --- API Key Operations ---

// CreateAPIKeyOptions specifies options for creating an API key
type CreateAPIKeyOptions struct {
	Name       string // API key name (required)
	Permission string // "full_access" or "sending_access" (default: "full_access")
	DomainID   string // Restrict to specific domain (optional)
}

// CreateAPIKey creates a new API key
func (c *ResendClient) CreateAPIKey(ctx context.Context, opts CreateAPIKeyOptions) (*ResendAPIKey, error) {
	reqURL := fmt.Sprintf("%s/api-keys", resendAPIBaseURL)

	permission := opts.Permission
	if permission == "" {
		permission = "full_access"
	}

	reqBody := map[string]any{
		"name":       opts.Name,
		"permission": permission,
	}
	if opts.DomainID != "" {
		reqBody["domain_id"] = opts.DomainID
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	result, err := parseResendResponse[ResendAPIKey](resp)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListAPIKeys lists all API keys
func (c *ResendClient) ListAPIKeys(ctx context.Context) ([]*ResendAPIKey, error) {
	reqURL := fmt.Sprintf("%s/api-keys", resendAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	result, err := parseResendResponse[ResendAPIKeysResponse](resp)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice for consistency
	keys := make([]*ResendAPIKey, len(result.Data))
	for i := range result.Data {
		keys[i] = &result.Data[i]
	}

	return keys, nil
}

// DeleteAPIKey deletes an API key
func (c *ResendClient) DeleteAPIKey(ctx context.Context, keyID string) error {
	reqURL := fmt.Sprintf("%s/api-keys/%s", resendAPIBaseURL, keyID)

	resp, err := c.doRequest(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete API key (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
