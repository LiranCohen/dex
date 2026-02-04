package main

import (
	"bytes"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	ErrCloudflaredNotInstalled = errors.New("cloudflared is not installed")
	ErrInvalidAPIToken         = errors.New("invalid Cloudflare API token")
	ErrTunnelCreationFailed    = errors.New("failed to create tunnel")
	ErrNoAccountFound          = errors.New("no Cloudflare account found")
	ErrNoDomainFound           = errors.New("no domain found in account")
)

const (
	cfAPIBase = "https://api.cloudflare.com/client/v4"
)

// TunnelInfo contains information about a created tunnel
type TunnelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AccountID   string `json:"account_id"`
	CredPath    string `json:"credentials_path"`
	TunnelURL   string `json:"tunnel_url"`
	ConnectorID string `json:"connector_id,omitempty"`
}

// CloudflareClient handles Cloudflare API interactions
type CloudflareClient struct {
	apiToken   string
	httpClient *http.Client
	accountID  string
}

// NewCloudflareClient creates a new Cloudflare API client
func NewCloudflareClient(apiToken string) *CloudflareClient {
	return &CloudflareClient{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ValidateToken checks if the API token is valid
func (c *CloudflareClient) ValidateToken() error {
	req, err := http.NewRequest("GET", cfAPIBase+"/user/tokens/verify", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to verify token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ErrInvalidAPIToken
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			Status string `json:"status"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success || result.Result.Status != "active" {
		return ErrInvalidAPIToken
	}

	return nil
}

// GetAccountID retrieves the account ID for the token
func (c *CloudflareClient) GetAccountID() (string, error) {
	if c.accountID != "" {
		return c.accountID, nil
	}

	req, err := http.NewRequest("GET", cfAPIBase+"/accounts?page=1&per_page=1", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get accounts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success || len(result.Result) == 0 {
		return "", ErrNoAccountFound
	}

	c.accountID = result.Result[0].ID
	return c.accountID, nil
}

// GetZones returns available zones (domains) for the account
func (c *CloudflareClient) GetZones() ([]Zone, error) {
	accountID, err := c.GetAccountID()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/zones?account.id=%s&status=active", cfAPIBase, accountID), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get zones: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Success bool   `json:"success"`
		Result  []Zone `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("failed to list zones")
	}

	return result.Result, nil
}

// Zone represents a Cloudflare zone (domain)
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// CreateTunnel creates a new Cloudflare Tunnel
func (c *CloudflareClient) CreateTunnel(name string, dataDir string) (*TunnelInfo, error) {
	accountID, err := c.GetAccountID()
	if err != nil {
		return nil, err
	}

	// Create tunnel via API
	body := map[string]any{
		"name":          name,
		"tunnel_secret": generateTunnelSecret(),
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/accounts/%s/cfd_tunnel", cfAPIBase, accountID), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Credentials struct {
				AccountTag   string `json:"account_tag"`
				TunnelID     string `json:"tunnel_id"`
				TunnelSecret string `json:"tunnel_secret"`
			} `json:"credentials"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		errMsg := "unknown error"
		if len(result.Errors) > 0 {
			errMsg = result.Errors[0].Message
		}
		return nil, fmt.Errorf("tunnel creation failed: %s", errMsg)
	}

	// Save credentials file for cloudflared
	credPath := filepath.Join(dataDir, "cloudflared-creds.json")
	credData := map[string]string{
		"AccountTag":   result.Result.Credentials.AccountTag,
		"TunnelID":     result.Result.Credentials.TunnelID,
		"TunnelSecret": result.Result.Credentials.TunnelSecret,
	}
	credJSON, _ := json.Marshal(credData)
	if err := os.WriteFile(credPath, credJSON, 0600); err != nil {
		return nil, fmt.Errorf("failed to save credentials: %w", err)
	}

	return &TunnelInfo{
		ID:        result.Result.ID,
		Name:      result.Result.Name,
		AccountID: accountID,
		CredPath:  credPath,
	}, nil
}

// ConfigureTunnelRoute configures the tunnel to route to a local port and creates DNS
func (c *CloudflareClient) ConfigureTunnelRoute(tunnelID string, subdomain string, zoneID string, zoneName string, localPort int) (string, error) {
	accountID, err := c.GetAccountID()
	if err != nil {
		return "", err
	}

	// Full hostname
	hostname := fmt.Sprintf("%s.%s", subdomain, zoneName)

	// Configure tunnel ingress rule
	config := map[string]any{
		"config": map[string]any{
			"ingress": []map[string]any{
				{
					"hostname": hostname,
					"service":  fmt.Sprintf("http://localhost:%d", localPort),
				},
				{
					"service": "http_status:404",
				},
			},
		},
	}

	jsonBody, _ := json.Marshal(config)
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations", cfAPIBase, accountID, tunnelID), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to configure tunnel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to configure tunnel: %s", string(body))
	}

	// Create DNS record pointing to tunnel
	dnsBody := map[string]any{
		"type":    "CNAME",
		"name":    subdomain,
		"content": fmt.Sprintf("%s.cfargotunnel.com", tunnelID),
		"proxied": true,
	}

	jsonBody, _ = json.Marshal(dnsBody)
	req, err = http.NewRequest("POST", fmt.Sprintf("%s/zones/%s/dns_records", cfAPIBase, zoneID), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create DNS record: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// DNS record might already exist, that's okay
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Check if it's a duplicate error, which we can ignore
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("already exists")) {
			return "", fmt.Errorf("failed to create DNS record: %s", string(body))
		}
	}

	return fmt.Sprintf("https://%s", hostname), nil
}

// GetTunnelHealth checks if a tunnel has healthy connectors
func (c *CloudflareClient) GetTunnelHealth(tunnelID string) (bool, error) {
	accountID, err := c.GetAccountID()
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s", cfAPIBase, accountID, tunnelID), nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to get tunnel status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			Status string `json:"status"`
			Conns  []struct {
				IsActive bool `json:"is_active"`
			} `json:"connections"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if tunnel has at least one active connection
	for _, conn := range result.Result.Conns {
		if conn.IsActive {
			return true, nil
		}
	}

	return result.Result.Status == "healthy", nil
}

// DeleteTunnel deletes a tunnel
func (c *CloudflareClient) DeleteTunnel(tunnelID string) error {
	accountID, err := c.GetAccountID()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s", cfAPIBase, accountID, tunnelID), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete tunnel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete tunnel: %s", string(body))
	}

	return nil
}

// generateTunnelSecret generates a random secret for the tunnel
func generateTunnelSecret() string {
	b := make([]byte, 32)
	_, _ = crand.Read(b)
	return fmt.Sprintf("%x", b)
}

// CheckCloudflaredInstalled checks if cloudflared CLI is available
func CheckCloudflaredInstalled() error {
	_, err := exec.LookPath("cloudflared")
	if err != nil {
		return ErrCloudflaredNotInstalled
	}
	return nil
}

// RunCloudflaredTunnel starts cloudflared with the given credentials
func RunCloudflaredTunnel(credPath string, tunnelID string) (*exec.Cmd, error) {
	if err := CheckCloudflaredInstalled(); err != nil {
		return nil, err
	}

	cmd := exec.Command("cloudflared", "tunnel", "--credentials-file", credPath, "run", tunnelID)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start cloudflared: %w", err)
	}

	return cmd, nil
}
