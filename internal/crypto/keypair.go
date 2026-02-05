package crypto

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// WorkerIdentity represents a worker's cryptographic identity.
// The private key is stored locally on the worker, and the public key
// is registered with HQ during enrollment.
type WorkerIdentity struct {
	ID       string `json:"id"`
	identity *age.X25519Identity

	// String representations for JSON serialization
	PublicKeyStr  string `json:"public_key"`
	PrivateKeyStr string `json:"private_key,omitempty"` // Only in local storage
}

// NewWorkerIdentity generates a new worker identity with a random keypair.
func NewWorkerIdentity(id string) (*WorkerIdentity, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}

	return &WorkerIdentity{
		ID:            id,
		identity:      identity,
		PublicKeyStr:  identity.Recipient().String(),
		PrivateKeyStr: identity.String(),
	}, nil
}

// LoadWorkerIdentity loads a worker identity from a file.
func LoadWorkerIdentity(path string) (*WorkerIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var wi WorkerIdentity
	if err := json.Unmarshal(data, &wi); err != nil {
		return nil, fmt.Errorf("failed to parse identity file: %w", err)
	}

	// Parse the private key to reconstruct the identity
	if wi.PrivateKeyStr != "" {
		identity, err := age.ParseX25519Identity(wi.PrivateKeyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid private key in identity file: %w", err)
		}
		wi.identity = identity
		// Ensure public key matches
		wi.PublicKeyStr = identity.Recipient().String()
	}

	return &wi, nil
}

// Save writes the worker identity to a file.
// The file is created with restricted permissions (0600).
func (wi *WorkerIdentity) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create identity directory: %w", err)
	}

	data, err := json.MarshalIndent(wi, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal identity: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	return nil
}

// PublicKey returns the public key string (age format).
func (wi *WorkerIdentity) PublicKey() string {
	return wi.PublicKeyStr
}

// PublicIdentity returns a copy of the identity with only public information.
// This is safe to send to HQ during enrollment.
func (wi *WorkerIdentity) PublicIdentity() *WorkerIdentity {
	return &WorkerIdentity{
		ID:           wi.ID,
		PublicKeyStr: wi.PublicKeyStr,
	}
}

// ToKeyPair converts the WorkerIdentity to a KeyPair for encryption operations.
func (wi *WorkerIdentity) ToKeyPair() *KeyPair {
	if wi.identity == nil {
		return nil
	}
	return &KeyPair{
		identity:  wi.identity,
		recipient: wi.identity.Recipient(),
	}
}

// Decrypt decrypts a message encrypted for this worker.
func (wi *WorkerIdentity) Decrypt(encoded string) ([]byte, error) {
	kp := wi.ToKeyPair()
	if kp == nil {
		return nil, errors.New("worker identity has no private key")
	}
	return kp.Decrypt(encoded)
}

// EnsureWorkerIdentity loads or creates a worker identity.
// If the identity file doesn't exist, a new one is created.
func EnsureWorkerIdentity(path, workerID string) (*WorkerIdentity, error) {
	// Try to load existing identity
	identity, err := LoadWorkerIdentity(path)
	if err == nil {
		return identity, nil
	}

	// Create new identity if file doesn't exist
	if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
		identity, err = NewWorkerIdentity(workerID)
		if err != nil {
			return nil, err
		}
		if err := identity.Save(path); err != nil {
			return nil, err
		}
		return identity, nil
	}

	return nil, err
}
