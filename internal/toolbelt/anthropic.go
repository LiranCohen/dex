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

const anthropicAPIBaseURL = "https://api.anthropic.com/v1"

// AnthropicClient wraps the Anthropic API for Poindexter's AI/LLM needs.
type AnthropicClient struct {
	httpClient *http.Client
	apiKey     string
}

// NewAnthropicClient creates a new AnthropicClient from configuration
func NewAnthropicClient(config *AnthropicConfig) *AnthropicClient {
	if config == nil || config.APIKey == "" {
		return nil
	}

	return &AnthropicClient{
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Longer timeout for LLM responses
		},
		apiKey: config.APIKey,
	}
}

// doRequest performs an HTTP request to the Anthropic API
func (c *AnthropicClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

	// Anthropic uses x-api-key header (not Bearer auth)
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	return c.httpClient.Do(req)
}

// anthropicErrorResponse represents an Anthropic API error response
type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// parseAnthropicResponse reads and unmarshals an Anthropic API response
func parseAnthropicResponse[T any](resp *http.Response) (*T, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp anthropicErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(body))
		}
		if errResp.Error.Message != "" {
			return nil, fmt.Errorf("anthropic API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Message Types ---

// AnthropicMessage represents a message in a conversation
type AnthropicMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // Message text
}

// AnthropicChatRequest represents a request to the messages API
type AnthropicChatRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
}

// AnthropicContentBlock represents a content block in a response
type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicUsage represents token usage in a response
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicChatResponse represents a response from the messages API
type AnthropicChatResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

// Text returns the text content from the response
func (r *AnthropicChatResponse) Text() string {
	for _, block := range r.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

// --- API Operations ---

// Ping verifies the Anthropic connection by making a minimal API call
// Uses the messages endpoint with minimal tokens to verify credentials
func (c *AnthropicClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/messages", anthropicAPIBaseURL)

	reqBody := AnthropicChatRequest{
		Model:     "claude-3-haiku-20240307",
		MaxTokens: 1,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("anthropic ping failed: %w", err)
	}

	_, err = parseAnthropicResponse[AnthropicChatResponse](resp)
	return err
}

// Chat sends a conversational request to the Anthropic API
// This is the primary method for multi-turn conversations
func (c *AnthropicClient) Chat(ctx context.Context, req *AnthropicChatRequest) (*AnthropicChatResponse, error) {
	reqURL := fmt.Sprintf("%s/messages", anthropicAPIBaseURL)

	// Set defaults if not provided
	if req.Model == "" {
		req.Model = "claude-sonnet-4-20250514"
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, req)
	if err != nil {
		return nil, fmt.Errorf("failed to chat: %w", err)
	}

	return parseAnthropicResponse[AnthropicChatResponse](resp)
}

// Complete sends a single-turn completion request to the Anthropic API
// This is a convenience method for simple prompts without conversation history
func (c *AnthropicClient) Complete(ctx context.Context, prompt string, maxTokens int) (*AnthropicChatResponse, error) {
	if maxTokens == 0 {
		maxTokens = 4096
	}

	req := &AnthropicChatRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: maxTokens,
		Messages: []AnthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	return c.Chat(ctx, req)
}
