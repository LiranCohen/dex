package oidc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	if kp.PrivateKey == nil {
		t.Error("private key should not be nil")
	}
	if kp.PublicKey == nil {
		t.Error("public key should not be nil")
	}
	if kp.KeyID == "" {
		t.Error("key ID should not be empty")
	}
}

func TestLoadOrGenerateKeyPair(t *testing.T) {
	t.Run("generate new keys", func(t *testing.T) {
		tmpDir := t.TempDir()

		kp, err := LoadOrGenerateKeyPair(tmpDir)
		if err != nil {
			t.Fatalf("failed to generate key pair: %v", err)
		}

		if kp.PrivateKey == nil {
			t.Error("private key should not be nil")
		}

		// Verify files were created
		privPath := filepath.Join(tmpDir, privateKeyFile)
		pubPath := filepath.Join(tmpDir, publicKeyFile)

		if _, err := os.Stat(privPath); os.IsNotExist(err) {
			t.Error("private key file was not created")
		}
		if _, err := os.Stat(pubPath); os.IsNotExist(err) {
			t.Error("public key file was not created")
		}

		// Verify private key permissions
		info, _ := os.Stat(privPath)
		if info.Mode().Perm() != 0600 {
			t.Errorf("private key should have 0600 permissions, got %o", info.Mode().Perm())
		}
	})

	t.Run("load existing keys", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Generate keys first
		kp1, err := LoadOrGenerateKeyPair(tmpDir)
		if err != nil {
			t.Fatalf("failed to generate key pair: %v", err)
		}

		// Load the same keys
		kp2, err := LoadOrGenerateKeyPair(tmpDir)
		if err != nil {
			t.Fatalf("failed to load key pair: %v", err)
		}

		// Verify same key ID
		if kp1.KeyID != kp2.KeyID {
			t.Errorf("key IDs should match: %s != %s", kp1.KeyID, kp2.KeyID)
		}

		// Verify same public key modulus
		if kp1.PublicKey.N.Cmp(kp2.PublicKey.N) != 0 {
			t.Error("public key modulus should match")
		}
	})
}

func TestJWKS(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	jwksBytes, err := kp.JWKS()
	if err != nil {
		t.Fatalf("failed to generate JWKS: %v", err)
	}

	var jwks JWKS
	if err := json.Unmarshal(jwksBytes, &jwks); err != nil {
		t.Fatalf("failed to parse JWKS: %v", err)
	}

	if len(jwks.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(jwks.Keys))
	}

	key := jwks.Keys[0]

	if key.Kty != "RSA" {
		t.Errorf("expected RSA key type, got %s", key.Kty)
	}
	if key.Use != "sig" {
		t.Errorf("expected sig use, got %s", key.Use)
	}
	if key.Alg != "RS256" {
		t.Errorf("expected RS256 algorithm, got %s", key.Alg)
	}
	if key.Kid != kp.KeyID {
		t.Errorf("key ID mismatch: %s != %s", key.Kid, kp.KeyID)
	}
	if key.N == "" {
		t.Error("modulus should not be empty")
	}
	if key.E == "" {
		t.Error("exponent should not be empty")
	}
}

func TestGenerateKeyID(t *testing.T) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()

	// Different keys should have different IDs
	if kp1.KeyID == kp2.KeyID {
		t.Error("different keys should have different IDs")
	}

	// Same key should produce same ID
	id1 := generateKeyID(kp1.PublicKey)
	id2 := generateKeyID(kp1.PublicKey)
	if id1 != id2 {
		t.Error("same key should produce same ID")
	}
}
