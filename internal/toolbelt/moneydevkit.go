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

const moneydevkitAPIBaseURL = "https://api.moneydevkit.com/v1"

// MoneyDevKitClient wraps the MoneyDevKit API for Poindexter's payment needs.
type MoneyDevKitClient struct {
	httpClient    *http.Client
	apiKey        string
	webhookSecret string
}

// NewMoneyDevKitClient creates a new MoneyDevKitClient from configuration
func NewMoneyDevKitClient(config *MoneyDevKitConfig) *MoneyDevKitClient {
	if config == nil || config.APIKey == "" {
		return nil
	}

	return &MoneyDevKitClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey:        config.APIKey,
		webhookSecret: config.WebhookSecret,
	}
}

// doRequest performs an HTTP request to the MoneyDevKit API
func (c *MoneyDevKitClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

// moneydevkitErrorResponse represents a MoneyDevKit API error response
type moneydevkitErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// parseMoneyDevKitResponse reads and unmarshals a MoneyDevKit API response
func parseMoneyDevKitResponse[T any](resp *http.Response) (*T, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp moneydevkitErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("moneydevkit API error (status %d): %s", resp.StatusCode, string(body))
		}
		if errResp.Message != "" {
			return nil, fmt.Errorf("moneydevkit API error: %s", errResp.Message)
		}
		if errResp.Error != "" {
			return nil, fmt.Errorf("moneydevkit API error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("moneydevkit API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Product Types ---

// MoneyDevKitProduct represents a product in MoneyDevKit
type MoneyDevKitProduct struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Active      bool              `json:"active"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// --- Price Types ---

// MoneyDevKitPrice represents a price for a product
type MoneyDevKitPrice struct {
	ID              string            `json:"id"`
	ProductID       string            `json:"product_id"`
	Currency        string            `json:"currency"`
	UnitAmount      int64             `json:"unit_amount"` // Amount in smallest currency unit (cents)
	Recurring       *PriceRecurring   `json:"recurring,omitempty"`
	Active          bool              `json:"active"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// PriceRecurring holds recurring billing info for a price
type PriceRecurring struct {
	Interval      string `json:"interval"`       // "day", "week", "month", "year"
	IntervalCount int    `json:"interval_count"` // Number of intervals between billings
}

// --- Checkout Link Types ---

// MoneyDevKitCheckoutLink represents a checkout link
type MoneyDevKitCheckoutLink struct {
	ID         string            `json:"id"`
	URL        string            `json:"url"`
	PriceID    string            `json:"price_id"`
	SuccessURL string            `json:"success_url,omitempty"`
	CancelURL  string            `json:"cancel_url,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// --- Account Types (for Ping) ---

// MoneyDevKitAccount represents account info
type MoneyDevKitAccount struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// --- API Operations ---

// Ping verifies the MoneyDevKit connection by calling the account endpoint
func (c *MoneyDevKitClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/account", moneydevkitAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("moneydevkit ping failed: %w", err)
	}

	_, err = parseMoneyDevKitResponse[MoneyDevKitAccount](resp)
	return err
}

// --- Product Operations ---

// CreateProductParams holds parameters for creating a product
type CreateProductParams struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Active      bool              `json:"active"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CreateProduct creates a new product
func (c *MoneyDevKitClient) CreateProduct(ctx context.Context, params CreateProductParams) (*MoneyDevKitProduct, error) {
	reqURL := fmt.Sprintf("%s/products", moneydevkitAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create product: %w", err)
	}

	return parseMoneyDevKitResponse[MoneyDevKitProduct](resp)
}

// --- Price Operations ---

// CreatePriceParams holds parameters for creating a price
type CreatePriceParams struct {
	ProductID  string            `json:"product_id"`
	Currency   string            `json:"currency"`
	UnitAmount int64             `json:"unit_amount"` // Amount in smallest currency unit (cents)
	Recurring  *PriceRecurring   `json:"recurring,omitempty"`
	Active     bool              `json:"active"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// CreatePrice creates a new price for a product
func (c *MoneyDevKitClient) CreatePrice(ctx context.Context, params CreatePriceParams) (*MoneyDevKitPrice, error) {
	reqURL := fmt.Sprintf("%s/prices", moneydevkitAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create price: %w", err)
	}

	return parseMoneyDevKitResponse[MoneyDevKitPrice](resp)
}

// --- Checkout Link Operations ---

// CreateCheckoutLinkParams holds parameters for creating a checkout link
type CreateCheckoutLinkParams struct {
	PriceID    string            `json:"price_id"`
	SuccessURL string            `json:"success_url,omitempty"`
	CancelURL  string            `json:"cancel_url,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// CreateCheckoutLink generates a checkout link for a price
func (c *MoneyDevKitClient) CreateCheckoutLink(ctx context.Context, params CreateCheckoutLinkParams) (*MoneyDevKitCheckoutLink, error) {
	reqURL := fmt.Sprintf("%s/checkout/links", moneydevkitAPIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkout link: %w", err)
	}

	return parseMoneyDevKitResponse[MoneyDevKitCheckoutLink](resp)
}

// WebhookSecret returns the configured webhook secret for signature verification
func (c *MoneyDevKitClient) WebhookSecret() string {
	return c.webhookSecret
}
