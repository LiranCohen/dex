package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClientConfig represents the client configuration saved by 'dex client enroll'.
// This is a simplified version of Config - clients don't need tunnel/ACME/owner info.
type ClientConfig struct {
	Namespace string             `json:"namespace"`
	Hostname  string             `json:"hostname"`
	Domains   ClientDomainConfig `json:"domains"` // Domain configuration from Central
	Mesh      ClientMeshConfig   `json:"mesh"`
}

// ClientDomainConfig contains domain configuration from Central.
// These should be used instead of hardcoding domain names.
type ClientDomainConfig struct {
	Public string `json:"public"` // Base domain for public URLs (e.g., "enbox.id")
	Mesh   string `json:"mesh"`   // Base domain for MagicDNS mesh hostnames (e.g., "dex")
}

// ClientMeshConfig contains mesh networking configuration for clients.
type ClientMeshConfig struct {
	ControlURL string `json:"control_url"`
}

// DefaultClientDataDir returns the default client data directory.
// Uses ~/.dex/ for user-local installation (no root required).
func DefaultClientDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".dex"
	}
	return filepath.Join(home, ".dex")
}

// LoadClientConfig reads and parses a client config file.
func LoadClientConfig(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ClientConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveClientConfig writes client config to a file.
func (c *ClientConfig) SaveClientConfig(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
