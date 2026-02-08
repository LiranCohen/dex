// Package auth provides authentication for Poindexter
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// JWTKeyFile is the filename for persisted JWT signing keys.
	JWTKeyFile = "jwt_keys.json"
)

// JWTKeyPair holds the ED25519 key pair used for JWT signing.
type JWTKeyPair struct {
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"private_key"`
}

// GenerateJWTKeyPair generates a new ED25519 key pair for JWT signing.
func GenerateJWTKeyPair() (*JWTKeyPair, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ED25519 keys: %w", err)
	}

	return &JWTKeyPair{
		PublicKey:  pubKey,
		PrivateKey: privKey,
	}, nil
}

// LoadJWTKeyPair loads a JWT key pair from a file.
func LoadJWTKeyPair(path string) (*JWTKeyPair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var kp JWTKeyPair
	if err := json.Unmarshal(data, &kp); err != nil {
		return nil, fmt.Errorf("failed to parse JWT keys file: %w", err)
	}

	// Validate key lengths
	if len(kp.PublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: got %d, want %d", len(kp.PublicKey), ed25519.PublicKeySize)
	}
	if len(kp.PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: got %d, want %d", len(kp.PrivateKey), ed25519.PrivateKeySize)
	}

	return &kp, nil
}

// Save writes the JWT key pair to a file with restricted permissions (0600).
func (kp *JWTKeyPair) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	data, err := json.MarshalIndent(kp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JWT keys: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write JWT keys file: %w", err)
	}

	return nil
}

// EnsureJWTKeyPair loads a JWT key pair from file, or generates and saves a new one.
// This ensures JWT tokens survive server restarts.
func EnsureJWTKeyPair(dataDir string) (*JWTKeyPair, error) {
	keyPath := filepath.Join(dataDir, JWTKeyFile)

	// Try to load existing keys
	kp, err := LoadJWTKeyPair(keyPath)
	if err == nil {
		return kp, nil
	}

	// If file doesn't exist, generate new keys
	if os.IsNotExist(err) {
		kp, err = GenerateJWTKeyPair()
		if err != nil {
			return nil, err
		}
		if err := kp.Save(keyPath); err != nil {
			return nil, err
		}
		return kp, nil
	}

	// Some other error occurred
	return nil, fmt.Errorf("failed to load JWT keys: %w", err)
}
