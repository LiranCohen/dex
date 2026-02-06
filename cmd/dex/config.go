package main

import (
	"encoding/json"
	"os"
)

// Config represents the HQ configuration saved by 'dex enroll'
type Config struct {
	Namespace string `json:"namespace"`
	PublicURL string `json:"public_url"`
	Hostname  string `json:"hostname,omitempty"`

	Mesh   MeshConfig   `json:"mesh"`
	Tunnel TunnelConfig `json:"tunnel"`
	ACME   ACMEConfig   `json:"acme"`
}

// MeshConfig contains mesh networking configuration
type MeshConfig struct {
	Enabled    bool   `json:"enabled"`
	ControlURL string `json:"control_url"`
	AuthKey    string `json:"auth_key"`
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
