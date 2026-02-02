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
			Timeout: 5 * time.Minute, // Long timeout for large context LLM responses (200K tokens)
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

// AnthropicTool defines a tool Claude can use
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// AnthropicMessage represents a message in a conversation
// Content can be a string (simple message) or []ContentBlock (tool results)
type AnthropicMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content any    `json:"content"` // string OR []ContentBlock
}

// ContentBlock for tool_use and tool_result messages
type ContentBlock struct {
	Type      string         `json:"type"`                  // "text", "tool_use", "tool_result"
	Text      string         `json:"text,omitempty"`        // type=text
	ID        string         `json:"id,omitempty"`          // type=tool_use
	Name      string         `json:"name,omitempty"`        // type=tool_use
	Input     map[string]any `json:"input,omitempty"`       // type=tool_use
	ToolUseID string         `json:"tool_use_id,omitempty"` // type=tool_result
	Content   string         `json:"content,omitempty"`     // type=tool_result (the result text)
	IsError   bool           `json:"is_error,omitempty"`    // type=tool_result
}

// AnthropicChatRequest represents a request to the messages API
type AnthropicChatRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

// AnthropicContentBlock represents a content block in a response
type AnthropicContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`    // for tool_use
	Name  string         `json:"name,omitempty"`  // for tool_use
	Input map[string]any `json:"input,omitempty"` // for tool_use
}

// MarshalJSON implements custom JSON marshaling to ensure tool_use blocks always have input field
func (b AnthropicContentBlock) MarshalJSON() ([]byte, error) {
	if b.Type == "tool_use" {
		// For tool_use, we need input to always be present (even if empty)
		type toolUseBlock struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		}
		input := b.Input
		if input == nil {
			input = map[string]any{}
		}
		return json.Marshal(toolUseBlock{
			Type:  b.Type,
			ID:    b.ID,
			Name:  b.Name,
			Input: input,
		})
	}

	// For other types (text), use default behavior
	type contentBlock AnthropicContentBlock
	return json.Marshal(contentBlock(b))
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

// HasToolUse returns true if the response contains tool_use blocks
func (r *AnthropicChatResponse) HasToolUse() bool {
	return r.StopReason == "tool_use"
}

// ToolUseBlocks returns all tool_use content blocks from the response
func (r *AnthropicChatResponse) ToolUseBlocks() []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	for _, block := range r.Content {
		if block.Type == "tool_use" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// NormalizedContent returns content blocks with nil Input maps converted to empty maps
// This is required because Anthropic API requires the input field to be present on tool_use blocks
func (r *AnthropicChatResponse) NormalizedContent() []AnthropicContentBlock {
	normalized := make([]AnthropicContentBlock, len(r.Content))
	for i, block := range r.Content {
		normalized[i] = block
		if block.Type == "tool_use" && normalized[i].Input == nil {
			normalized[i].Input = map[string]any{}
		}
	}
	return normalized
}

// --- API Operations ---

// Ping verifies the Anthropic connection by making a minimal API call
// Uses the messages endpoint with minimal tokens to verify credentials
func (c *AnthropicClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/messages", anthropicAPIBaseURL)

	reqBody := AnthropicChatRequest{
		Model:     "claude-haiku-4-5-20251001",
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
		req.Model = "claude-sonnet-4-5-20250929"
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
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: maxTokens,
		Messages: []AnthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	return c.Chat(ctx, req)
}
