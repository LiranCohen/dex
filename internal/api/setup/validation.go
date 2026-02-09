package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ValidateAnthropicKey validates an Anthropic API key
func ValidateAnthropicKey(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return err
	}

	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Anthropic: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("invalid key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	return nil
}

// ValidateAnthropicKeyFormat checks if a key has the expected format
func ValidateAnthropicKeyFormat(key string) error {
	if key == "" {
		return errors.New("key is required")
	}
	if !strings.HasPrefix(key, "sk-ant") {
		return errors.New("invalid key format - should start with sk-ant")
	}
	return nil
}
