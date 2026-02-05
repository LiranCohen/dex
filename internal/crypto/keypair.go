package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"
)

func init() {
	// Set the scalarBaseMult function to use curve25519
	scalarBaseMult = func(dst, scalar *[32]byte) {
		curve25519.ScalarBaseMult(dst, scalar)
	}
}

// WorkerIdentity represents a worker's cryptographic identity.
// The private key is stored locally on the worker, and the public key
// is registered with HQ during enrollment.
type WorkerIdentity struct {
	ID         string   `json:"id"`
	PublicKey  [32]byte `json:"-"`
	PrivateKey [32]byte `json:"-"`

	// Base64-encoded versions for JSON serialization
	PublicKeyB64  string `json:"public_key"`
	PrivateKeyB64 string `json:"private_key,omitempty"` // Only in local storage
}

// NewWorkerIdentity generates a new worker identity with a random keypair.
func NewWorkerIdentity(id string) (*WorkerIdentity, error) {
	// Generate random private key
	var privateKey [32]byte
	if _, err := io.ReadFull(rand.Reader, privateKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Derive public key from private key using Curve25519
	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return &WorkerIdentity{
		ID:            id,
		PublicKey:     publicKey,
		PrivateKey:    privateKey,
		PublicKeyB64:  base64.StdEncoding.EncodeToString(publicKey[:]),
		PrivateKeyB64: base64.StdEncoding.EncodeToString(privateKey[:]),
	}, nil
}

// LoadWorkerIdentity loads a worker identity from a file.
func LoadWorkerIdentity(path string) (*WorkerIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var identity WorkerIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("failed to parse identity file: %w", err)
	}

	// Decode base64 keys
	pub, err := base64.StdEncoding.DecodeString(identity.PublicKeyB64)
	if err != nil || len(pub) != 32 {
		return nil, errors.New("invalid public key in identity file")
	}
	copy(identity.PublicKey[:], pub)

	if identity.PrivateKeyB64 != "" {
		priv, err := base64.StdEncoding.DecodeString(identity.PrivateKeyB64)
		if err != nil || len(priv) != 32 {
			return nil, errors.New("invalid private key in identity file")
		}
		copy(identity.PrivateKey[:], priv)
	}

	return &identity, nil
}

// Save writes the worker identity to a file.
// The file is created with restricted permissions (0600).
func (wi *WorkerIdentity) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create identity directory: %w", err)
	}

	// Update base64 fields
	wi.PublicKeyB64 = base64.StdEncoding.EncodeToString(wi.PublicKey[:])
	wi.PrivateKeyB64 = base64.StdEncoding.EncodeToString(wi.PrivateKey[:])

	data, err := json.MarshalIndent(wi, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal identity: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	return nil
}

// PublicIdentity returns a copy of the identity with only public information.
// This is safe to send to HQ during enrollment.
func (wi *WorkerIdentity) PublicIdentity() *WorkerIdentity {
	return &WorkerIdentity{
		ID:           wi.ID,
		PublicKey:    wi.PublicKey,
		PublicKeyB64: wi.PublicKeyB64,
	}
}

// ToKeyPair converts the WorkerIdentity to a KeyPair for encryption operations.
func (wi *WorkerIdentity) ToKeyPair() *KeyPair {
	return &KeyPair{
		PublicKey:  wi.PublicKey,
		PrivateKey: wi.PrivateKey,
	}
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
