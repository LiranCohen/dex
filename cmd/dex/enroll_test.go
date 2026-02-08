package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnrollmentKeyValidation(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid key",
			key:     "dexkey-alice-abc123",
			wantErr: false,
		},
		{
			name:    "invalid prefix",
			key:     "invalid-key",
			wantErr: true,
			errMsg:  "invalid enrollment key format",
		},
		{
			name:    "empty key (stdin EOF)",
			key:     "",
			wantErr: true,
			errMsg:  "failed to read input", // stdin returns EOF in test environment
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create a mock server that rejects all keys for this test
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Central returns plain text error messages
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("invalid key"))
			}))
			defer server.Close()

			args := []string{"--data-dir", tmpDir, "--central-url", server.URL}
			if tt.key != "" {
				args = append(args, "--key", tt.key)
			}

			err := runEnroll(args)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					// Check if error contains the expected message
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
					}
				}
			} else {
				// For valid key format, we expect a different error (server rejection)
				if err == nil {
					t.Error("expected error from server rejection but got nil")
				}
			}
		})
	}
}

func TestEnrollmentSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock Central server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/enroll" {
			t.Errorf("expected /api/v1/enroll, got %s", r.URL.Path)
		}

		var req EnrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Key != "dexkey-alice-abc123" {
			t.Errorf("expected key dexkey-alice-abc123, got %s", req.Key)
		}

		// Build response with optional fields populated (simulates public node enrollment)
		resp := EnrollmentResponse{
			Namespace: "alice",
			PublicURL: "https://alice.enbox.id",
			Tunnel: &struct {
				IngressAddr string `json:"ingress_addr"`
				Token       string `json:"token"`
			}{
				IngressAddr: "ingress.enbox.id:9443",
				Token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
			},
			ACME: &struct {
				Email  string `json:"email"`
				DNSAPI string `json:"dns_api"`
			}{
				Email:  "alice@example.com",
				DNSAPI: "https://central.enbox.id/api/v1/dns/acme-challenge",
			},
			Owner: &struct {
				UserID      string `json:"user_id"`
				Email       string `json:"email"`
				DisplayName string `json:"display_name,omitempty"`
				Passkey     *struct {
					CredentialID string `json:"credential_id"`
					PublicKey    string `json:"public_key"`
					PublicKeyAlg int    `json:"public_key_alg"`
					SignCount    uint32 `json:"sign_count"`
				} `json:"passkey,omitempty"`
			}{
				UserID:      "user-123",
				Email:       "alice@example.com",
				DisplayName: "Alice",
			},
		}
		resp.Domains.Public = "enbox.id"
		resp.Domains.Mesh = "dex"
		resp.Mesh.ControlURL = "https://central.enbox.id"

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	err := runEnroll([]string{
		"--key", "dexkey-alice-abc123",
		"--data-dir", tmpDir,
		"--central-url", server.URL,
	})
	if err != nil {
		t.Fatalf("enrollment failed: %v", err)
	}

	// Verify config was saved
	configPath := filepath.Join(tmpDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.Namespace != "alice" {
		t.Errorf("expected namespace alice, got %s", config.Namespace)
	}
	if config.PublicURL != "https://alice.enbox.id" {
		t.Errorf("expected public URL https://alice.enbox.id, got %s", config.PublicURL)
	}
	if !config.Mesh.Enabled {
		t.Error("expected mesh to be enabled")
	}
	if config.Mesh.ControlURL != "https://central.enbox.id" {
		t.Errorf("expected mesh control URL https://central.enbox.id, got %s", config.Mesh.ControlURL)
	}
	if !config.Tunnel.Enabled {
		t.Error("expected tunnel to be enabled")
	}
	if config.Tunnel.IngressAddr != "ingress.enbox.id:9443" {
		t.Errorf("expected ingress addr ingress.enbox.id:9443, got %s", config.Tunnel.IngressAddr)
	}
	if len(config.Tunnel.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(config.Tunnel.Endpoints))
	}
	if config.Tunnel.Endpoints[0].Hostname != "hq.alice.enbox.id" {
		t.Errorf("expected endpoint hostname hq.alice.enbox.id, got %s", config.Tunnel.Endpoints[0].Hostname)
	}
	// Verify domains are stored
	if config.Domains.Public != "enbox.id" {
		t.Errorf("expected public domain enbox.id, got %s", config.Domains.Public)
	}
	if config.Domains.Mesh != "dex" {
		t.Errorf("expected mesh domain dex, got %s", config.Domains.Mesh)
	}
	if config.Owner.UserID != "user-123" {
		t.Errorf("expected owner user ID user-123, got %s", config.Owner.UserID)
	}
	if config.Owner.Email != "alice@example.com" {
		t.Errorf("expected owner email alice@example.com, got %s", config.Owner.Email)
	}
}

func TestEnrollmentMeshOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock Central server that returns mesh-only config (no tunnel)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response without Tunnel, ACME, or Owner - simulates HQ enrollment
		resp := EnrollmentResponse{
			Namespace: "alice",
			Hostname:  "hq",
			PublicURL: "", // No public URL for mesh-only
			// Tunnel, ACME, Owner are nil (not included)
		}
		resp.Domains.Public = "enbox.id"
		resp.Domains.Mesh = "dex"
		resp.Mesh.ControlURL = "https://central.enbox.id"

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	err := runEnroll([]string{
		"--key", "dexkey-alice-abc123",
		"--data-dir", tmpDir,
		"--central-url", server.URL,
	})
	if err != nil {
		t.Fatalf("enrollment failed: %v", err)
	}

	// Verify config was saved
	configPath := filepath.Join(tmpDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.Namespace != "alice" {
		t.Errorf("expected namespace alice, got %s", config.Namespace)
	}
	if config.Hostname != "hq" {
		t.Errorf("expected hostname hq, got %s", config.Hostname)
	}
	if !config.Mesh.Enabled {
		t.Error("expected mesh to be enabled")
	}
	if config.Mesh.ControlURL != "https://central.enbox.id" {
		t.Errorf("expected mesh control URL https://central.enbox.id, got %s", config.Mesh.ControlURL)
	}
	// Tunnel should be DISABLED for mesh-only nodes
	if config.Tunnel.Enabled {
		t.Error("expected tunnel to be DISABLED for mesh-only node")
	}
	// ACME should be DISABLED for mesh-only nodes
	if config.ACME.Enabled {
		t.Error("expected ACME to be DISABLED for mesh-only node")
	}
	// Verify domains are stored
	if config.Domains.Public != "enbox.id" {
		t.Errorf("expected public domain enbox.id, got %s", config.Domains.Public)
	}
	if config.Domains.Mesh != "dex" {
		t.Errorf("expected mesh domain dex, got %s", config.Domains.Mesh)
	}
}

// TestEnrollmentBackwardsCompatibility tests that enrollment works when
// domains are not present in the response (legacy Central servers).
func TestEnrollmentBackwardsCompatibility(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock Central server that returns response WITHOUT domains
	// (simulates old Central server before domain config was added)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := EnrollmentResponse{
			Namespace: "alice",
			Hostname:  "hq",
			// Domains NOT set - testing backwards compatibility
		}
		resp.Mesh.ControlURL = "https://central.enbox.id"

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	err := runEnroll([]string{
		"--key", "dexkey-alice-abc123",
		"--data-dir", tmpDir,
		"--central-url", server.URL,
	})
	if err != nil {
		t.Fatalf("enrollment failed: %v", err)
	}

	// Verify config was saved with default domains
	configPath := filepath.Join(tmpDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// Should use default domains when not provided by Central
	if config.Domains.Public != "enbox.id" {
		t.Errorf("expected default public domain enbox.id, got %s", config.Domains.Public)
	}
	if config.Domains.Mesh != "dex" {
		t.Errorf("expected default mesh domain dex, got %s", config.Domains.Mesh)
	}
}

func TestAlreadyEnrolled(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing config
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"namespace": "existing"}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	err := runEnroll([]string{
		"--key", "dexkey-alice-abc123",
		"--data-dir", tmpDir,
	})

	if err == nil {
		t.Fatal("expected error for already enrolled")
	}
	if !containsString(err.Error(), "already enrolled") {
		t.Errorf("expected 'already enrolled' error, got: %v", err)
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	config := &Config{
		Namespace: "test",
		PublicURL: "https://test.enbox.id",
		Hostname:  "test-hq",
		Domains: DomainConfig{
			Public: "enbox.id",
			Mesh:   "dex",
		},
		Mesh: MeshConfig{
			Enabled:    true,
			ControlURL: "https://central.enbox.id",
		},
		Tunnel: TunnelConfig{
			Enabled:     true,
			IngressAddr: "ingress.enbox.id:9443",
			Token:       "jwt-token",
			Endpoints: []EndpointConfig{
				{Hostname: "test.enbox.id", LocalPort: 8080},
			},
		},
		ACME: ACMEConfig{
			Enabled: true,
			Email:   "test@example.com",
			DNSAPI:  "https://central.enbox.id/api/v1/dns/acme-challenge",
		},
	}

	if err := config.SaveConfig(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Namespace != config.Namespace {
		t.Errorf("namespace mismatch: got %s, want %s", loaded.Namespace, config.Namespace)
	}
	if loaded.Mesh.ControlURL != config.Mesh.ControlURL {
		t.Errorf("control URL mismatch: got %s, want %s", loaded.Mesh.ControlURL, config.Mesh.ControlURL)
	}
	if len(loaded.Tunnel.Endpoints) != len(config.Tunnel.Endpoints) {
		t.Errorf("endpoints count mismatch: got %d, want %d", len(loaded.Tunnel.Endpoints), len(config.Tunnel.Endpoints))
	}
	if loaded.Domains.Public != config.Domains.Public {
		t.Errorf("public domain mismatch: got %s, want %s", loaded.Domains.Public, config.Domains.Public)
	}
	if loaded.Domains.Mesh != config.Domains.Mesh {
		t.Errorf("mesh domain mismatch: got %s, want %s", loaded.Domains.Mesh, config.Domains.Mesh)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
