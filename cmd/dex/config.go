package main

import (
	"encoding/json"
	"os"
)

// Config represents the HQ configuration saved by 'dex enroll'
type Config struct {
	Namespace string       `json:"namespace"`
	PublicURL string       `json:"public_url"`
	Hostname  string       `json:"hostname,omitempty"`
	IsPublic  bool         `json:"is_public"` // Explicit flag for public accessibility
	Domains   DomainConfig `json:"domains"`   // Domain configuration from Central

	Mesh   MeshConfig   `json:"mesh"`
	Tunnel TunnelConfig `json:"tunnel"`
	ACME   ACMEConfig   `json:"acme"`
	Owner  OwnerConfig  `json:"owner"`
}

// DomainConfig contains domain configuration from Central.
// These should be used instead of hardcoding domain names.
type DomainConfig struct {
	Public string `json:"public"` // Base domain for public URLs (e.g., "enbox.id")
	Mesh   string `json:"mesh"`   // Base domain for MagicDNS mesh hostnames (e.g., "dex")
}

// OwnerConfig contains the HQ owner's identity and authentication credentials
type OwnerConfig struct {
	UserID      string        `json:"user_id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name,omitempty"`
	Passkey     PasskeyConfig `json:"passkey,omitempty"`
}

// PasskeyConfig contains the WebAuthn credential for owner authentication
type PasskeyConfig struct {
	CredentialID []byte `json:"credential_id,omitempty"`  // WebAuthn credential ID
	PublicKey    []byte `json:"public_key,omitempty"`     // COSE-encoded public key
	PublicKeyAlg int    `json:"public_key_alg,omitempty"` // COSE algorithm (-7 for ES256)
	SignCount    uint32 `json:"sign_count,omitempty"`     // For replay protection
}

// MeshConfig contains mesh networking configuration.
// Note: AuthKey is NOT stored here - it's consumed during enrollment.
// After enrollment, the machine key in the mesh state directory is used.
type MeshConfig struct {
	Enabled    bool   `json:"enabled"`
	ControlURL string `json:"control_url"`
}

// TunnelConfig contains tunnel configuration for public ingress
type TunnelConfig struct {
	Enabled     bool             `json:"enabled"`
	IngressAddr string           `json:"ingress_addr"`
	Token       string           `json:"token"`
	Endpoints   []EndpointConfig `json:"endpoints"`
}

// EndpointConfig defines a public endpoint to expose
type EndpointConfig struct {
	Hostname  string `json:"hostname"`
	LocalPort int    `json:"local_port"`
}

// ACMEConfig contains ACME certificate configuration
type ACMEConfig struct {
	Enabled bool   `json:"enabled"`
	Email   string `json:"email"`
	DNSAPI  string `json:"dns_api"`
}

// LoadConfig reads and parses a config file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig writes config to a file
func (c *Config) SaveConfig(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
