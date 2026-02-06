package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnrollmentRequest is sent to Central
type EnrollmentRequest struct {
	Key      string `json:"key"`
	Hostname string `json:"hostname,omitempty"`
}

// EnrollmentResponse is returned by Central's /api/v1/enroll endpoint
type EnrollmentResponse struct {
	// Account/Network info
	AccountID string `json:"account_id"`
	NetworkID string `json:"network_id"`
	Namespace string `json:"namespace"`
	PublicURL string `json:"public_url"`

	// Top-level fields (for convenience)
	ControlURL  string `json:"control_url"`
	TunnelToken string `json:"tunnel_token"`

	// Mesh configuration for dexnet
	Mesh struct {
		ControlURL string `json:"control_url"`
		AuthKey    string `json:"auth_key"` // Headscale pre-auth key
	} `json:"mesh"`

	// Tunnel configuration for Edge connection
	Tunnel struct {
		IngressAddr string `json:"ingress_addr"`
		Token       string `json:"token"`
	} `json:"tunnel"`

	// ACME configuration for TLS certificates
	ACME struct {
		Email  string `json:"email"`
		DNSAPI string `json:"dns_api"`
	} `json:"acme"`
}

const (
	DefaultCentralURL = "https://central.enbox.id"
	DefaultDataDir    = "/opt/dex"
)

func runEnroll(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	keyFlag := fs.String("key", "", "Enrollment key from Central dashboard")
	dataDirFlag := fs.String("data-dir", "", "Data directory (default: /opt/dex)")
	centralURLFlag := fs.String("central-url", DefaultCentralURL, "Central server URL")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dex enroll [options]\n\n")
		fmt.Fprintf(os.Stderr, "Enroll this HQ with Central using an enrollment key.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  dex enroll                              # Interactive mode\n")
		fmt.Fprintf(os.Stderr, "  dex enroll --key dexkey-alice-a1b2c3d4  # Non-interactive\n")
		fmt.Fprintf(os.Stderr, "  dex enroll --key dexkey-xxx --data-dir /opt/dex\n")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// 1. Get enrollment key
	key := *keyFlag
	if key == "" {
		fmt.Print("Enter your enrollment key: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		key = strings.TrimSpace(input)
	}

	if key == "" {
		return fmt.Errorf("enrollment key is required")
	}

	// Validate key format
	if !strings.HasPrefix(key, "dexkey-") {
		return fmt.Errorf("invalid enrollment key format (should start with 'dexkey-')")
	}

	// 2. Determine data directory
	dataDir := *dataDirFlag
	if dataDir == "" {
		dataDir = os.Getenv("DEX_DATA_DIR")
	}
	if dataDir == "" {
		dataDir = DefaultDataDir
	}

	// 3. Check if already enrolled
	configPath := filepath.Join(dataDir, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("already enrolled (config exists at %s). To re-enroll, remove the config file first", configPath)
	}

	// 4. Create data directory if needed
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// 5. Get hostname
	hostname, _ := os.Hostname()

	// 6. Call Central enrollment API
	fmt.Println("Enrolling with Central...")

	resp, err := callEnrollmentAPI(*centralURLFlag, EnrollmentRequest{
		Key:      key,
		Hostname: hostname,
	})
	if err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}

	// 7. Build and save configuration
	config := buildConfigFromResponse(resp)
	config.Hostname = hostname

	if err := config.SaveConfig(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 8. Print success
	fmt.Println()
	fmt.Println("Enrollment successful!")
	fmt.Println()
	fmt.Printf("   Namespace:  %s\n", resp.Namespace)
	fmt.Printf("   Public URL: %s\n", resp.PublicURL)
	fmt.Printf("   Config:     %s\n", configPath)
	fmt.Println()
	fmt.Println("Start Dex with:")
	fmt.Printf("   dex start --base-dir %s\n", dataDir)
	fmt.Println()

	return nil
}

func callEnrollmentAPI(centralURL string, req EnrollmentRequest) (*EnrollmentResponse, error) {
	url := strings.TrimSuffix(centralURL, "/") + "/api/v1/enroll"

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "dex/"+version)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Central: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		// Central returns plain text error messages
		errMsg := strings.TrimSpace(string(body))
		if errMsg == "" {
			errMsg = http.StatusText(httpResp.StatusCode)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var resp EnrollmentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

func buildConfigFromResponse(resp *EnrollmentResponse) *Config {
	config := &Config{
		Namespace: resp.Namespace,
		PublicURL: resp.PublicURL,
		Mesh: MeshConfig{
			Enabled:    true,
			ControlURL: resp.Mesh.ControlURL,
			AuthKey:    resp.Mesh.AuthKey,
		},
		Tunnel: TunnelConfig{
			Enabled:     true,
			IngressAddr: resp.Tunnel.IngressAddr,
			Token:       resp.Tunnel.Token,
			Endpoints: []EndpointConfig{
				{
					Hostname:  resp.Namespace + ".enbox.id",
					LocalPort: 8080,
				},
			},
		},
		ACME: ACMEConfig{
			Enabled: true,
			Email:   resp.ACME.Email,
			DNSAPI:  resp.ACME.DNSAPI,
		},
	}

	return config
}
