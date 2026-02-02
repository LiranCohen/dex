package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ValidateGitHubToken validates a GitHub personal access token
func ValidateGitHubToken(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	return nil
}

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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("invalid key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	return nil
}

// GitHubOrgInfo contains information about a GitHub organization
type GitHubOrgInfo struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Type  string `json:"type"` // "Organization" or "User"
}

// ValidateGitHubOrg validates that a GitHub organization exists and is accessible
// It uses the GitHub App's JWT to check (does not require user auth)
func ValidateGitHubOrg(ctx context.Context, orgName string) (*GitHubOrgInfo, error) {
	// Clean up org name
	orgName = strings.TrimSpace(orgName)
	if orgName == "" {
		return nil, errors.New("organization name is required")
	}

	// Remove any URL prefix if user pasted a full URL
	orgName = strings.TrimPrefix(orgName, "https://github.com/")
	orgName = strings.TrimPrefix(orgName, "http://github.com/")
	orgName = strings.TrimPrefix(orgName, "github.com/")
	orgName = strings.TrimSuffix(orgName, "/")

	// Check for invalid characters
	if strings.Contains(orgName, "/") || strings.Contains(orgName, " ") {
		return nil, errors.New("invalid organization name format")
	}

	// Make unauthenticated request to check if org exists
	// GitHub allows anonymous requests to public orgs
	url := fmt.Sprintf("https://api.github.com/orgs/%s", orgName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Try checking if it's a user account instead
		return checkGitHubUser(ctx, orgName)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	var org struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Type  string `json:"type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &GitHubOrgInfo{
		ID:    org.ID,
		Login: org.Login,
		Name:  org.Name,
		Type:  org.Type,
	}, nil
}

// checkGitHubUser checks if a username is a valid GitHub user
func checkGitHubUser(ctx context.Context, username string) (*GitHubOrgInfo, error) {
	url := fmt.Sprintf("https://api.github.com/users/%s", username)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("organization not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Type  string `json:"type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if it's actually an organization
	if user.Type == "Organization" {
		return &GitHubOrgInfo{
			ID:    user.ID,
			Login: user.Login,
			Name:  user.Name,
			Type:  user.Type,
		}, nil
	}

	// It's a user account, not an organization
	return nil, fmt.Errorf("'%s' is a personal account, not an organization: GitHub Apps can only create repositories in organizations", username)
}

// ValidateGitHubTokenFormat checks if a token has the expected format
func ValidateGitHubTokenFormat(token string) error {
	if token == "" {
		return errors.New("token is required")
	}
	if !strings.HasPrefix(token, "ghp_") && !strings.HasPrefix(token, "github_pat_") {
		return errors.New("invalid token format - should start with ghp_ or github_pat_")
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
