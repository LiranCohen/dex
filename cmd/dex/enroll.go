package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/WebP2P/dexnet/types/key"
)

// EnrollmentRequest is sent to Central
type EnrollmentRequest struct {
	Key        string `json:"key"`                   // Enrollment key (dexkey-xxx)
	MachineKey string `json:"machine_key"`           // Machine's public key for mesh registration
	Hostname   string `json:"hostname,omitempty"`    // Requested hostname
}

// EnrollmentResponse is returned by Central's /api/v1/enroll endpoint
type EnrollmentResponse struct {
	Namespace string `json:"namespace"`
	Hostname  string `json:"hostname"`   // Server hostname (e.g., "hq")
	PublicURL string `json:"public_url"` // e.g., "https://hq.alice.enbox.id"

	// Mesh configuration for dexnet
	Mesh struct {
		ControlURL string `json:"control_url"` // Central coordination service URL
	} `json:"mesh"`

	// Tunnel configuration for Edge connection
	Tunnel struct {
		IngressAddr string `json:"ingress_addr"` // Edge address (e.g., "edge.enbox.id:443")
		Token       string `json:"token"`        // JWT for tunnel authentication
	} `json:"tunnel"`

	// ACME configuration for TLS certificates
	ACME struct {
		Email  string `json:"email"`   // Email for Let's Encrypt
		DNSAPI string `json:"dns_api"` // Central's DNS API for ACME challenges
	} `json:"acme"`

	// Owner identity for authentication
	Owner struct {
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name,omitempty"`
		Passkey     *struct {
			CredentialID string `json:"credential_id"`  // Base64 URL encoded
			PublicKey    string `json:"public_key"`     // Base64 URL encoded COSE key
			PublicKeyAlg int    `json:"public_key_alg"` // COSE algorithm
			SignCount    uint32 `json:"sign_count"`
		} `json:"passkey,omitempty"`
	} `json:"owner"`
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
	enrollKey := *keyFlag
	if enrollKey == "" {
		fmt.Print("Enter your enrollment key: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		enrollKey = strings.TrimSpace(input)
	}

	if enrollKey == "" {
		return fmt.Errorf("enrollment key is required")
	}

	// Validate key format
	if !strings.HasPrefix(enrollKey, "dexkey-") {
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

	// 4. Create data directory and mesh state directory
	meshStateDir := filepath.Join(dataDir, "mesh")
	if err := os.MkdirAll(meshStateDir, 0755); err != nil {
		return fmt.Errorf("failed to create mesh state directory: %w", err)
	}

	// 5. Generate machine key for mesh registration
	fmt.Println("Generating machine key...")
	machineKey := key.NewMachine()
	machineKeyPublic := machineKey.Public()

	// Save machine key to state directory (tsnet format)
	if err := saveMachineKey(meshStateDir, machineKey); err != nil {
		return fmt.Errorf("failed to save machine key: %w", err)
	}

	// 6. Get hostname
	hostname, _ := os.Hostname()

	// 7. Call Central enrollment API with machine key
	fmt.Println("Enrolling with Central...")

	resp, err := callEnrollmentAPI(*centralURLFlag, EnrollmentRequest{
		Key:        enrollKey,
		MachineKey: machineKeyPublic.String(), // Send public key to Central
		Hostname:   hostname,
	})
	if err != nil {
		// Clean up machine key on failure
		_ = os.RemoveAll(meshStateDir)
		return fmt.Errorf("enrollment failed: %w", err)
	}

	// 8. Build and save configuration
	config := buildConfigFromResponse(resp)

	if err := config.SaveConfig(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 9. Print success
	fmt.Println()
	fmt.Println("Enrollment successful!")
	fmt.Println()
	fmt.Printf("   Server:     %s.%s\n", resp.Hostname, resp.Namespace)
	fmt.Printf("   Public URL: %s\n", resp.PublicURL)
	fmt.Printf("   Config:     %s\n", configPath)
	fmt.Println()
	fmt.Println("Start Dex with:")
	fmt.Printf("   dex start --base-dir %s\n", dataDir)
	fmt.Println()

	return nil
}

// saveMachineKey saves the machine private key to the tsnet state directory.
// The key is stored in the format expected by tsnet's FileStore.
func saveMachineKey(stateDir string, machineKey key.MachinePrivate) error {
	// Marshal the private key to text format (privkey:hexencoded)
	keyText, err := machineKey.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal machine key: %w", err)
	}

	// Create the state file in tsnet's expected format
	// The state is a JSON object with "_machinekey" as the key
	state := map[string]string{
		"_machinekey": string(keyText),
	}

	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	statePath := filepath.Join(stateDir, "tailscaled.state")
	if err := os.WriteFile(statePath, stateJSON, 0600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

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
	// Use hostname from response, default to "hq" if not set
	hostname := resp.Hostname
	if hostname == "" {
		hostname = "hq"
	}

	config := &Config{
		Namespace: resp.Namespace,
		PublicURL: resp.PublicURL,
		Hostname:  hostname, // Set hostname for mesh registration (used by main.go)
		Mesh: MeshConfig{
			Enabled:    true,
			ControlURL: resp.Mesh.ControlURL,
			// Note: AuthKey is NOT stored - it's consumed during enrollment via RegisterOnce
		},
		Tunnel: TunnelConfig{
			Enabled:     true,
			IngressAddr: resp.Tunnel.IngressAddr,
			Token:       resp.Tunnel.Token,
			Endpoints: []EndpointConfig{
				{
					Hostname:  hostname + "." + resp.Namespace + ".enbox.id",
					LocalPort: 8080,
				},
			},
		},
		ACME: ACMEConfig{
			Enabled: true,
			Email:   resp.ACME.Email,
			DNSAPI:  resp.ACME.DNSAPI,
		},
		Owner: OwnerConfig{
			UserID:      resp.Owner.UserID,
			Email:       resp.Owner.Email,
			DisplayName: resp.Owner.DisplayName,
		},
	}

	// Parse passkey if provided
	if resp.Owner.Passkey != nil {
		credID, err := base64.RawURLEncoding.DecodeString(resp.Owner.Passkey.CredentialID)
		if err != nil {
			// Log but don't fail - passkey is optional
			fmt.Fprintf(os.Stderr, "Warning: failed to decode passkey credential ID: %v\n", err)
		}
		pubKey, err := base64.RawURLEncoding.DecodeString(resp.Owner.Passkey.PublicKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to decode passkey public key: %v\n", err)
		}

		// Only set passkey config if both decoded successfully
		if len(credID) > 0 && len(pubKey) > 0 {
			config.Owner.Passkey = PasskeyConfig{
				CredentialID: credID,
				PublicKey:    pubKey,
				PublicKeyAlg: resp.Owner.Passkey.PublicKeyAlg,
				SignCount:    resp.Owner.Passkey.SignCount,
			}
		}
	}

	return config
}
