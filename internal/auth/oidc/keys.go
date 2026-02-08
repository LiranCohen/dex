// Package oidc implements an OIDC provider for HQ authentication.
package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

const (
	// RSA key size for token signing
	rsaKeySize = 2048

	// Key file names
	privateKeyFile = "oidc_private.pem"
	publicKeyFile  = "oidc_public.pem"
)

// KeyPair holds the RSA keys used for signing JWT tokens.
type KeyPair struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	KeyID      string // Key ID for JWKS
}

// LoadOrGenerateKeyPair loads existing keys from disk or generates new ones.
func LoadOrGenerateKeyPair(dataDir string) (*KeyPair, error) {
	privPath := filepath.Join(dataDir, privateKeyFile)
	pubPath := filepath.Join(dataDir, publicKeyFile)

	// Try to load existing keys
	if _, err := os.Stat(privPath); err == nil {
		return loadKeyPair(privPath, pubPath)
	}

	// Generate new keys
	return generateAndSaveKeyPair(privPath, pubPath)
}

// loadKeyPair loads an RSA key pair from PEM files.
func loadKeyPair(privPath, pubPath string) (*KeyPair, error) {
	// Read private key
	privPEM, err := os.ReadFile(privPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, errors.New("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	keyID := generateKeyID(&privateKey.PublicKey)

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
		KeyID:      keyID,
	}, nil
}

// generateAndSaveKeyPair generates a new RSA key pair and saves to disk.
func generateAndSaveKeyPair(privPath, pubPath string) (*KeyPair, error) {
	// Ensure directory exists
	dir := filepath.Dir(privPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create key directory: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Encode public key
	pubBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	// Save files with secure permissions
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		return nil, fmt.Errorf("failed to save private key: %w", err)
	}
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		return nil, fmt.Errorf("failed to save public key: %w", err)
	}

	keyID := generateKeyID(&privateKey.PublicKey)

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
		KeyID:      keyID,
	}, nil
}

// generateKeyID creates a deterministic key ID from the public key.
func generateKeyID(pub *rsa.PublicKey) string {
	// Use first 8 bytes of modulus as key ID (base64url encoded)
	n := pub.N.Bytes()
	if len(n) > 8 {
		n = n[:8]
	}
	return base64.RawURLEncoding.EncodeToString(n)
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key.
type JWK struct {
	Kty string `json:"kty"`           // Key type (RSA)
	Use string `json:"use"`           // Key use (sig)
	Kid string `json:"kid"`           // Key ID
	Alg string `json:"alg"`           // Algorithm (RS256)
	N   string `json:"n"`             // RSA modulus
	E   string `json:"e"`             // RSA exponent
}

// JWKS returns the public key in JWKS format.
func (kp *KeyPair) JWKS() ([]byte, error) {
	jwks := JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Use: "sig",
				Kid: kp.KeyID,
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(kp.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(kp.PublicKey.E)).Bytes()),
			},
		},
	}

	return json.Marshal(jwks)
}

// GenerateKeyPair creates a new RSA key pair without saving to disk.
// Useful for testing.
func GenerateKeyPair() (*KeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	keyID := generateKeyID(&privateKey.PublicKey)

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
		KeyID:      keyID,
	}, nil
}
