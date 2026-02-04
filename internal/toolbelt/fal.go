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

const falAPIBaseURL = "https://fal.run"

// FalClient wraps the fal.ai API for media generation.
type FalClient struct {
	httpClient *http.Client
	apiKey     string
}

// NewFalClient creates a new FalClient from configuration
func NewFalClient(config *FalConfig) *FalClient {
	if config == nil || config.APIKey == "" {
		return nil
	}

	return &FalClient{
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // Longer timeout for media generation
		},
		apiKey: config.APIKey,
	}
}

// doRequest performs an HTTP request to the fal.ai API
func (c *FalClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
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

	// fal.ai uses Bearer token auth
	req.Header.Set("Authorization", "Key "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// falErrorResponse represents a fal.ai API error response
type falErrorResponse struct {
	Detail string `json:"detail"`
}

// parseFalResponse reads and unmarshals a fal.ai API response
func parseFalResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp falErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("fal API error (status %d): %s", resp.StatusCode, string(body))
		}
		if errResp.Detail != "" {
			return nil, fmt.Errorf("fal API error: %s", errResp.Detail)
		}
		return nil, fmt.Errorf("fal API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// --- Image Generation Types ---

// FalImageSize represents the size specification for image generation
type FalImageSize struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// FalGenerateImageRequest represents a request to generate images
type FalGenerateImageRequest struct {
	Prompt            string        `json:"prompt"`
	ImageSize         *FalImageSize `json:"image_size,omitempty"`
	NumInferenceSteps int           `json:"num_inference_steps,omitempty"`
	NumImages         int           `json:"num_images,omitempty"`
	Seed              int           `json:"seed,omitempty"`
}

// FalImage represents a generated image in the response
type FalImage struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// FalTimings represents timing information in the response
type FalTimings struct {
	Inference float64 `json:"inference,omitempty"`
}

// FalGenerateImageResponse represents the response from image generation
type FalGenerateImageResponse struct {
	Images  []FalImage `json:"images"`
	Seed    int        `json:"seed,omitempty"`
	Timings FalTimings `json:"timings,omitempty"`
	Prompt  string     `json:"prompt,omitempty"`
}

// --- Video Generation Types ---

// FalGenerateVideoRequest represents a request to generate videos
type FalGenerateVideoRequest struct {
	Prompt   string `json:"prompt"`
	ImageURL string `json:"image_url,omitempty"` // Optional reference image
	Duration int    `json:"duration,omitempty"`  // Duration in seconds
	Seed     int    `json:"seed,omitempty"`
}

// FalVideo represents a generated video in the response
type FalVideo struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
}

// FalGenerateVideoResponse represents the response from video generation
type FalGenerateVideoResponse struct {
	Video   FalVideo   `json:"video"`
	Seed    int        `json:"seed,omitempty"`
	Timings FalTimings `json:"timings,omitempty"`
}

// --- API Operations ---

// Ping verifies the fal.ai connection by making a minimal API call
// Uses fal-ai/flux/schnell with a minimal request to verify credentials
func (c *FalClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/fal-ai/flux/schnell", falAPIBaseURL)

	reqBody := FalGenerateImageRequest{
		Prompt:            "test",
		NumInferenceSteps: 1,
		NumImages:         1,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("fal ping failed: %w", err)
	}

	_, err = parseFalResponse[FalGenerateImageResponse](resp)
	return err
}

// GenerateImage generates images using the specified model
// Common models: fal-ai/flux/schnell, fal-ai/flux/dev, fal-ai/stable-diffusion-v3-medium
func (c *FalClient) GenerateImage(ctx context.Context, model string, params *FalGenerateImageRequest) (*FalGenerateImageResponse, error) {
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if params == nil || params.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	reqURL := fmt.Sprintf("%s/%s", falAPIBaseURL, model)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}

	return parseFalResponse[FalGenerateImageResponse](resp)
}

// GenerateVideo generates videos using the specified model
// Common models: fal-ai/minimax/video-01, fal-ai/runway/gen3-turbo
func (c *FalClient) GenerateVideo(ctx context.Context, model string, params *FalGenerateVideoRequest) (*FalGenerateVideoResponse, error) {
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if params == nil || params.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	reqURL := fmt.Sprintf("%s/%s", falAPIBaseURL, model)

	resp, err := c.doRequest(ctx, http.MethodPost, reqURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate video: %w", err)
	}

	return parseFalResponse[FalGenerateVideoResponse](resp)
}
