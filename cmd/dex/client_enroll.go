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

// ClientEnrollmentRequest is sent to Central for client enrollment.
type ClientEnrollmentRequest struct {
	Key        string `json:"key"`         // Enrollment key (dexkey-xxx)
	MachineKey string `json:"machine_key"` // Machine's public key for mesh registration
	Hostname   string `json:"hostname"`    // Device hostname
}

// ClientEnrollmentResponse is returned by Central's /api/v1/enroll endpoint for clients.
// This is a simplified response - no tunnel/ACME/owner info needed.
type ClientEnrollmentResponse struct {
	Namespace string `json:"namespace"`
	Hostname  string `json:"hostname"`

	// Domain configuration - use these instead of hardcoding
	Domains struct {
		Public string `json:"public"` // Base domain for public URLs (e.g., "enbox.id")
		Mesh   string `json:"mesh"`   // Base domain for MagicDNS mesh hostnames (e.g., "dex")
	} `json:"domains"`

	// Mesh configuration for dexnet
	Mesh struct {
		ControlURL string `json:"control_url"`
	} `json:"mesh"`
}

func runClientEnroll(args []string) error {
	fs := flag.NewFlagSet("client enroll", flag.ExitOnError)
	keyFlag := fs.String("key", "", "Enrollment key from HQ dashboard")
	dataDirFlag := fs.String("data-dir", "", "Data directory (default: ~/.dex)")
	centralURLFlag := fs.String("central-url", DefaultCentralURL, "Central server URL")
	hostnameFlag := fs.String("hostname", "", "Hostname for this device (default: auto-detected)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dex client enroll [options]\n\n")
		fmt.Fprintf(os.Stderr, "Enroll this device with your HQ using a client enrollment key.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  dex client enroll                              # Interactive mode\n")
		fmt.Fprintf(os.Stderr, "  dex client enroll --key dexkey-alice-a1b2c3d4  # Non-interactive\n")
		fmt.Fprintf(os.Stderr, "  dex client enroll --key dexkey-xxx --hostname macbook\n")
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
		dataDir = os.Getenv("DEX_CLIENT_DATA_DIR")
	}
	if dataDir == "" {
		dataDir = DefaultClientDataDir()
	}

	// 3. Check if already enrolled
	configPath := filepath.Join(dataDir, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("already enrolled (config exists at %s). To re-enroll, remove the config file first", configPath)
	}

	// 4. Create data directory and mesh state directory
	// Clean any stale mesh state from a previous enrollment so tsnet starts fresh.
	// This is important because tsnet caches node keys and profile data that become
	// invalid after a re-enrollment (e.g., after a server-side DB reset).
	meshStateDir := filepath.Join(dataDir, "mesh")
	if err := os.RemoveAll(meshStateDir); err != nil {
		return fmt.Errorf("failed to clean mesh state directory: %w", err)
	}
	if err := os.MkdirAll(meshStateDir, 0755); err != nil {
		return fmt.Errorf("failed to create mesh state directory: %w", err)
	}

	// 5. Generate machine key for mesh registration
	fmt.Println("Generating machine key...")
	machineKey := key.NewMachine()
	machineKeyPublic := machineKey.Public()

	// Save machine key to state directory (tsnet format)
	if err := saveClientMachineKey(meshStateDir, machineKey); err != nil {
		return fmt.Errorf("failed to save machine key: %w", err)
	}

	// 6. Get hostname (auto-detect or user-provided)
	hostname := *hostnameFlag
	if hostname == "" {
		// Auto-detect from OS hostname
		osHostname, err := os.Hostname()
		if err != nil {
			osHostname = "client"
		}
		// Clean up hostname (lowercase, alphanumeric + dashes)
		hostname = cleanHostname(osHostname)
	}

	// 7. Call Central enrollment API with machine key
	fmt.Println("Enrolling with Central...")

	resp, err := callClientEnrollmentAPI(*centralURLFlag, ClientEnrollmentRequest{
		Key:        enrollKey,
		MachineKey: machineKeyPublic.String(),
		Hostname:   hostname,
	})
	if err != nil {
		// Clean up machine key on failure
		_ = os.RemoveAll(meshStateDir)
		return fmt.Errorf("enrollment failed: %w", err)
	}

	// 8. Build and save configuration
	// Use domains from response, with fallbacks for backwards compatibility
	publicDomain := resp.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}
	meshDomain := resp.Domains.Mesh
	if meshDomain == "" {
		meshDomain = "dex"
	}

	config := &ClientConfig{
		Namespace: resp.Namespace,
		Hostname:  resp.Hostname,
		Domains: ClientDomainConfig{
			Public: publicDomain,
			Mesh:   meshDomain,
		},
		Mesh: ClientMeshConfig{
			ControlURL: resp.Mesh.ControlURL,
		},
	}

	if err := config.SaveClientConfig(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 9. Print success
	fmt.Println()
	fmt.Println("Enrollment successful!")
	fmt.Println()
	fmt.Printf("   Device:    %s\n", resp.Hostname)
	fmt.Printf("   Namespace: %s\n", resp.Namespace)
	fmt.Printf("   Config:    %s\n", configPath)
	fmt.Println()
	fmt.Println("Start the mesh client with:")
	fmt.Println("   dex client start")
	fmt.Println()

	return nil
}

// saveClientMachineKey saves the machine private key to the tsnet state directory.
func saveClientMachineKey(stateDir string, machineKey key.MachinePrivate) error {
	// Marshal the private key to text format
	keyText, err := machineKey.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal machine key: %w", err)
	}

	// Create the state file in tsnet's expected format
	state := map[string]string{
		"_machinekey": base64.StdEncoding.EncodeToString(keyText),
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

func callClientEnrollmentAPI(centralURL string, req ClientEnrollmentRequest) (*ClientEnrollmentResponse, error) {
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
	httpReq.Header.Set("User-Agent", "dex-client/"+version)

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

	var resp ClientEnrollmentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// cleanHostname converts a hostname to a valid mesh hostname.
// Lowercase, alphanumeric with dashes, max 32 characters.
func cleanHostname(hostname string) string {
	hostname = strings.ToLower(hostname)

	var result strings.Builder
	for _, c := range hostname {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result.WriteRune(c)
		} else if c == '-' || c == '_' || c == '.' {
			result.WriteRune('-')
		}
	}

	cleaned := result.String()

	// Trim leading/trailing dashes
	cleaned = strings.Trim(cleaned, "-")

	// Limit length
	if len(cleaned) > 32 {
		cleaned = cleaned[:32]
	}

	// Ensure minimum length
	if len(cleaned) < 2 {
		cleaned = "client"
	}

	return cleaned
}
