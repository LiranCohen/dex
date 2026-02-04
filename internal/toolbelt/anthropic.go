// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// AnthropicAPIError represents a specific error from the Anthropic API
type AnthropicAPIError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *AnthropicAPIError) Error() string {
	return fmt.Sprintf("anthropic API error: %s", e.Message)
}

// IsBillingError returns true if this is a billing/credit balance error
func (e *AnthropicAPIError) IsBillingError() bool {
	return e.StatusCode == 400 && (e.Type == "invalid_request_error" ||
		contains(e.Message, "credit balance") ||
		contains(e.Message, "billing") ||
		contains(e.Message, "payment"))
}

// IsRateLimitError returns true if this is a rate limit error
func (e *AnthropicAPIError) IsRateLimitError() bool {
	return e.StatusCode == 429
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// parseAnthropicResponse reads and unmarshals an Anthropic API response
func parseAnthropicResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp anthropicErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, &AnthropicAPIError{
				StatusCode: resp.StatusCode,
				Type:       "unknown",
				Message:    string(body),
			}
		}
		return nil, &AnthropicAPIError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
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
// Checks both stop_reason and actual content blocks (handles max_tokens truncation)
func (r *AnthropicChatResponse) HasToolUse() bool {
	if r.StopReason == "tool_use" {
		return true
	}
	// Also check content blocks in case response was truncated (max_tokens)
	for _, block := range r.Content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
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

// --- Streaming API ---

// StreamEvent represents an event from the streaming API
type StreamEvent struct {
	Type       string // "content_delta", "tool_use", "message_stop", "error"
	Delta      string // Text delta for content_delta events
	Error      error  // Error for error events
	ToolUse    *AnthropicContentBlock // For tool_use events
	StopReason string // For message_stop events
}

// StreamCallback is called for each text delta during streaming
type StreamCallback func(delta string)

// streamRequest is the request body for streaming
type streamRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

// ChatStream sends a streaming request to the Anthropic API
// Returns a channel that receives StreamEvents until the message is complete
// The final event will have Type="message_stop" and the channel will be closed
func (c *AnthropicClient) ChatStream(ctx context.Context, req *AnthropicChatRequest) (<-chan StreamEvent, error) {
	reqURL := fmt.Sprintf("%s/messages", anthropicAPIBaseURL)

	// Set defaults if not provided
	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Create streaming request
	streamReq := streamRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  req.Messages,
		System:    req.System,
		Tools:     req.Tools,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(streamReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		var errResp anthropicErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, &AnthropicAPIError{
				StatusCode: resp.StatusCode,
				Type:       "unknown",
				Message:    string(body),
			}
		}
		return nil, &AnthropicAPIError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
	}

	events := make(chan StreamEvent, 100)

	// Start goroutine to read SSE events
	go func() {
		defer close(events)
		defer func() { _ = resp.Body.Close() }()

		c.readSSEEvents(ctx, resp.Body, events)
	}()

	return events, nil
}

// ChatWithStreaming sends a request with streaming, calling the callback for each text delta,
// and returns the complete response (including any tool_use blocks) when done.
// This allows both real-time UI updates AND full tool detection.
func (c *AnthropicClient) ChatWithStreaming(ctx context.Context, req *AnthropicChatRequest, onDelta StreamCallback) (*AnthropicChatResponse, error) {
	reqURL := fmt.Sprintf("%s/messages", anthropicAPIBaseURL)

	// Set defaults if not provided
	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Create streaming request
	streamReq := streamRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  req.Messages,
		System:    req.System,
		Tools:     req.Tools,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(streamReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		var errResp anthropicErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, &AnthropicAPIError{
				StatusCode: resp.StatusCode,
				Type:       "unknown",
				Message:    string(body),
			}
		}
		return nil, &AnthropicAPIError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
	}

	// Read and process SSE events, building the complete response
	return c.readSSEAndBuildResponse(ctx, resp.Body, onDelta)
}

// readSSEAndBuildResponse reads SSE events and constructs a complete response
func (c *AnthropicClient) readSSEAndBuildResponse(ctx context.Context, body io.Reader, onDelta StreamCallback) (*AnthropicChatResponse, error) {
	reader := bufio.NewReader(body)
	var eventType string

	// Response builder
	response := &AnthropicChatResponse{
		Role:    "assistant",
		Content: []AnthropicContentBlock{},
	}

	// Track current content blocks being built
	var currentBlocks []AnthropicContentBlock
	var textBuilder strings.Builder
	var currentToolInput strings.Builder
	currentBlockIndex := -1

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse SSE format
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		switch eventType {
		case "message_start":
			var msgStart struct {
				Type    string `json:"type"`
				Message struct {
					ID    string `json:"id"`
					Model string `json:"model"`
					Role  string `json:"role"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &msgStart); err == nil {
				response.ID = msgStart.Message.ID
				response.Model = msgStart.Message.Model
				response.Role = msgStart.Message.Role
			}

		case "content_block_start":
			var blockStart struct {
				Type         string `json:"type"`
				Index        int    `json:"index"`
				ContentBlock struct {
					Type  string `json:"type"`
					ID    string `json:"id,omitempty"`
					Name  string `json:"name,omitempty"`
					Text  string `json:"text,omitempty"`
					Input any    `json:"input,omitempty"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &blockStart); err == nil {
				currentBlockIndex = blockStart.Index

				// Expand currentBlocks slice if needed
				for len(currentBlocks) <= currentBlockIndex {
					currentBlocks = append(currentBlocks, AnthropicContentBlock{})
				}

				currentBlocks[currentBlockIndex] = AnthropicContentBlock{
					Type: blockStart.ContentBlock.Type,
					ID:   blockStart.ContentBlock.ID,
					Name: blockStart.ContentBlock.Name,
					Text: blockStart.ContentBlock.Text,
				}

				// Reset builders for new block
				if blockStart.ContentBlock.Type == "text" {
					textBuilder.Reset()
					textBuilder.WriteString(blockStart.ContentBlock.Text)
				} else if blockStart.ContentBlock.Type == "tool_use" {
					currentToolInput.Reset()
				}
			}

		case "content_block_delta":
			var delta struct {
				Type  string `json:"type"`
				Index int    `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text,omitempty"`
					PartialJSON string `json:"partial_json,omitempty"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &delta); err == nil {
				if delta.Delta.Type == "text_delta" && delta.Delta.Text != "" {
					textBuilder.WriteString(delta.Delta.Text)
					// Call the callback for real-time updates
					if onDelta != nil {
						onDelta(delta.Delta.Text)
					}
				} else if delta.Delta.Type == "input_json_delta" && delta.Delta.PartialJSON != "" {
					currentToolInput.WriteString(delta.Delta.PartialJSON)
				}
			}

		case "content_block_stop":
			var blockStop struct {
				Type  string `json:"type"`
				Index int    `json:"index"`
			}
			if err := json.Unmarshal([]byte(data), &blockStop); err == nil {
				idx := blockStop.Index
				if idx < len(currentBlocks) {
					if currentBlocks[idx].Type == "text" {
						currentBlocks[idx].Text = textBuilder.String()
					} else if currentBlocks[idx].Type == "tool_use" {
						// Parse accumulated JSON input
						inputStr := currentToolInput.String()
						if inputStr != "" {
							var input map[string]any
							if err := json.Unmarshal([]byte(inputStr), &input); err == nil {
								currentBlocks[idx].Input = input
							}
						}
						if currentBlocks[idx].Input == nil {
							currentBlocks[idx].Input = map[string]any{}
						}
					}
				}
			}

		case "message_delta":
			var msgDelta struct {
				Type  string `json:"type"`
				Delta struct {
					StopReason   string  `json:"stop_reason"`
					StopSequence *string `json:"stop_sequence"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &msgDelta); err == nil {
				response.StopReason = msgDelta.Delta.StopReason
				response.StopSequence = msgDelta.Delta.StopSequence
				response.Usage.OutputTokens = msgDelta.Usage.OutputTokens
			}

		case "message_stop":
			// Copy accumulated blocks to response
			response.Content = currentBlocks
			return response, nil

		case "error":
			var errData struct {
				Type  string `json:"type"`
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &errData); err == nil {
				return nil, &AnthropicAPIError{
					StatusCode: 500,
					Type:       errData.Error.Type,
					Message:    errData.Error.Message,
				}
			}
			return nil, fmt.Errorf("streaming error: %s", data)
		}
	}

	// If we get here, stream ended without message_stop
	response.Content = currentBlocks
	return response, nil
}

// readSSEEvents reads Server-Sent Events from the response body
func (c *AnthropicClient) readSSEEvents(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	reader := bufio.NewReader(body)
	var eventType string

	for {
		select {
		case <-ctx.Done():
			events <- StreamEvent{Type: "error", Error: ctx.Err()}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				events <- StreamEvent{Type: "error", Error: err}
			}
			return
		}

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse SSE format
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Handle different event types
			switch eventType {
			case "content_block_delta":
				var delta struct {
					Type  string `json:"type"`
					Index int    `json:"index"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					if delta.Delta.Type == "text_delta" && delta.Delta.Text != "" {
						events <- StreamEvent{Type: "content_delta", Delta: delta.Delta.Text}
					}
				}

			case "message_stop":
				events <- StreamEvent{Type: "message_stop"}
				return

			case "error":
				var errData struct {
					Type  string `json:"type"`
					Error struct {
						Type    string `json:"type"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(data), &errData); err == nil {
					events <- StreamEvent{
						Type:  "error",
						Error: fmt.Errorf("%s: %s", errData.Error.Type, errData.Error.Message),
					}
				}
				return
			}
		}
	}
}
