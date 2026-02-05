package crypto

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMasterKey_NewAndEncryptDecrypt(t *testing.T) {
	password := []byte("test-password-12345")

	mk, err := NewMasterKey(password, nil)
	if err != nil {
		t.Fatalf("NewMasterKey failed: %v", err)
	}

	if mk.Salt() == nil || len(mk.Salt()) != SaltSize {
		t.Errorf("Salt should be %d bytes, got %d", SaltSize, len(mk.Salt()))
	}

	// Test encryption/decryption roundtrip
	plaintext := []byte("Hello, World! This is a secret message.")

	encrypted, err := mk.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == string(plaintext) {
		t.Error("Encrypted should differ from plaintext")
	}

	decrypted, err := mk.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestMasterKey_WithProvidedSalt(t *testing.T) {
	password := []byte("test-password")
	salt := make([]byte, SaltSize)
	for i := range salt {
		salt[i] = byte(i)
	}

	mk1, err := NewMasterKey(password, salt)
	if err != nil {
		t.Fatalf("NewMasterKey failed: %v", err)
	}

	mk2, err := NewMasterKey(password, salt)
	if err != nil {
		t.Fatalf("NewMasterKey failed: %v", err)
	}

	// Same password and salt should produce same key
	plaintext := []byte("test message")
	encrypted, _ := mk1.Encrypt(plaintext)

	// mk2 should be able to decrypt what mk1 encrypted
	decrypted, err := mk2.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt with same key failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("Decryption with same key should work")
	}
}

func TestMasterKey_Export(t *testing.T) {
	password := []byte("test-password")
	mk, err := NewMasterKey(password, nil)
	if err != nil {
		t.Fatalf("NewMasterKey failed: %v", err)
	}

	exported := mk.Export()
	if exported == "" {
		t.Error("Export should return non-empty string")
	}

	// Should be valid base64
	data, err := base64.StdEncoding.DecodeString(exported)
	if err != nil {
		t.Fatalf("Export should return valid base64: %v", err)
	}

	// Should be salt + key
	if len(data) != SaltSize+KeySize {
		t.Errorf("Exported data should be %d bytes, got %d", SaltSize+KeySize, len(data))
	}
}

func TestMasterKey_EmptyPassword(t *testing.T) {
	_, err := NewMasterKey(nil, nil)
	if err == nil {
		t.Error("NewMasterKey should fail with empty password")
	}

	_, err = NewMasterKey([]byte{}, nil)
	if err == nil {
		t.Error("NewMasterKey should fail with empty password")
	}
}

func TestMasterKey_DecryptInvalidData(t *testing.T) {
	mk, _ := NewMasterKey([]byte("password"), nil)

	// Not base64
	_, err := mk.Decrypt("not-valid-base64!")
	if err == nil {
		t.Error("Decrypt should fail on invalid base64")
	}

	// Valid base64 but too short
	_, err = mk.Decrypt(base64.StdEncoding.EncodeToString([]byte("short")))
	if err != ErrInvalidCiphertext {
		t.Errorf("Decrypt should return ErrInvalidCiphertext, got %v", err)
	}

	// Valid base64 but wrong key
	mk2, _ := NewMasterKey([]byte("different-password"), nil)
	encrypted, _ := mk.Encrypt([]byte("secret"))
	_, err = mk2.Decrypt(encrypted)
	if err != ErrDecryptionFailed {
		t.Errorf("Decrypt should return ErrDecryptionFailed with wrong key, got %v", err)
	}
}

func TestGenerateMasterKey(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	// Should work for encryption
	plaintext := []byte("test")
	encrypted, err := mk.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := mk.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("Roundtrip failed")
	}
}

func TestKeyPair_GenerateAndEncryptDecrypt(t *testing.T) {
	// Generate two keypairs (simulating HQ and Worker)
	hqKeyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	workerKeyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Verify public keys are in age format
	if !strings.HasPrefix(hqKeyPair.PublicKey(), "age1") {
		t.Errorf("Public key should start with 'age1', got %q", hqKeyPair.PublicKey())
	}

	// Verify private keys are in age format
	if !strings.HasPrefix(hqKeyPair.PrivateKey(), "AGE-SECRET-KEY-1") {
		t.Errorf("Private key should start with 'AGE-SECRET-KEY-1', got %q", hqKeyPair.PrivateKey())
	}

	// HQ encrypts for worker
	message := []byte("Secret objective payload")
	encrypted, err := hqKeyPair.EncryptForRecipient(message, workerKeyPair.PublicKey())
	if err != nil {
		t.Fatalf("EncryptForRecipient failed: %v", err)
	}

	// Worker decrypts
	decrypted, err := workerKeyPair.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, message) {
		t.Errorf("Decrypted mismatch: got %q, want %q", decrypted, message)
	}
}

func TestKeyPair_Base64Encoding(t *testing.T) {
	kp, _ := GenerateKeyPair()

	pubB64 := kp.PublicKeyBase64()
	privB64 := kp.PrivateKeyBase64()

	if pubB64 == "" || privB64 == "" {
		t.Error("Base64 encoding should return non-empty strings")
	}

	// Should be valid base64
	_, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		t.Errorf("PublicKeyBase64 should return valid base64: %v", err)
	}

	_, err = base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		t.Errorf("PrivateKeyBase64 should return valid base64: %v", err)
	}
}

func TestKeyPair_FromPrivate(t *testing.T) {
	original, _ := GenerateKeyPair()

	// Reconstruct from just private key
	reconstructed, err := KeyPairFromPrivate(original.PrivateKey())
	if err != nil {
		t.Fatalf("KeyPairFromPrivate failed: %v", err)
	}

	if reconstructed.PublicKey() != original.PublicKey() {
		t.Error("Public key should be derivable from private key")
	}

	// Invalid private key
	_, err = KeyPairFromPrivate("not-a-valid-key")
	if err == nil {
		t.Error("Should fail on invalid private key")
	}
}

func TestRecipientFromPublicKey(t *testing.T) {
	kp, _ := GenerateKeyPair()

	recipient, err := RecipientFromPublicKey(kp.PublicKey())
	if err != nil {
		t.Fatalf("RecipientFromPublicKey failed: %v", err)
	}

	if recipient.String() != kp.PublicKey() {
		t.Error("Recipient public key should match")
	}

	// Invalid inputs
	_, err = RecipientFromPublicKey("not-a-valid-key")
	if err == nil {
		t.Error("Should fail on invalid public key")
	}
}

func TestEncryptForRecipientStatic(t *testing.T) {
	recipient, _ := GenerateKeyPair()
	message := []byte("Test message for static encryption")

	// Encrypt without needing a sender keypair
	encrypted, err := EncryptForRecipientStatic(message, recipient.PublicKey())
	if err != nil {
		t.Fatalf("EncryptForRecipientStatic failed: %v", err)
	}

	// Recipient should be able to decrypt
	decrypted, err := recipient.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, message) {
		t.Errorf("Message mismatch: got %q, want %q", decrypted, message)
	}

	// Invalid recipient
	_, err = EncryptForRecipientStatic(message, "invalid-key")
	if err == nil {
		t.Error("Should fail with invalid recipient key")
	}
}

func TestKeyPair_DecryptInvalidData(t *testing.T) {
	kp, _ := GenerateKeyPair()

	// Not base64
	_, err := kp.Decrypt("not-valid-base64!")
	if err == nil {
		t.Error("Should fail on invalid base64")
	}

	// Valid base64 but invalid age ciphertext
	_, err = kp.Decrypt(base64.StdEncoding.EncodeToString([]byte("not-age-ciphertext")))
	if err == nil {
		t.Error("Should fail on invalid ciphertext")
	}

	// Encrypted for different recipient
	other, _ := GenerateKeyPair()
	encrypted, _ := EncryptForRecipientStatic([]byte("test"), other.PublicKey())
	_, err = kp.Decrypt(encrypted)
	if err != ErrDecryptionFailed {
		t.Errorf("Should return ErrDecryptionFailed for wrong recipient, got %v", err)
	}
}

func TestZeroKey(t *testing.T) {
	key := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	ZeroKey(key)

	for i, b := range key {
		if b != 0 {
			t.Errorf("Key byte %d should be zero, got %d", i, b)
		}
	}
}

func TestWorkerIdentity_CreateAndSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "identity.json")

	// Create new identity
	identity, err := NewWorkerIdentity("test-worker-1")
	if err != nil {
		t.Fatalf("NewWorkerIdentity failed: %v", err)
	}

	if identity.ID != "test-worker-1" {
		t.Errorf("ID mismatch: got %q, want %q", identity.ID, "test-worker-1")
	}

	// Verify public key is in age format
	if !strings.HasPrefix(identity.PublicKey(), "age1") {
		t.Errorf("Public key should start with 'age1', got %q", identity.PublicKey())
	}

	// Save
	if err := identity.Save(identityPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists with correct permissions
	info, err := os.Stat(identityPath)
	if err != nil {
		t.Fatalf("File should exist: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("File should have 0600 permissions, got %o", info.Mode().Perm())
	}

	// Load
	loaded, err := LoadWorkerIdentity(identityPath)
	if err != nil {
		t.Fatalf("LoadWorkerIdentity failed: %v", err)
	}

	if loaded.ID != identity.ID {
		t.Error("ID mismatch after load")
	}
	if loaded.PublicKey() != identity.PublicKey() {
		t.Error("PublicKey mismatch after load")
	}
}

func TestWorkerIdentity_PublicIdentity(t *testing.T) {
	identity, _ := NewWorkerIdentity("worker")

	pubOnly := identity.PublicIdentity()

	if pubOnly.ID != identity.ID {
		t.Error("ID should be copied")
	}
	if pubOnly.PublicKeyStr != identity.PublicKeyStr {
		t.Error("PublicKeyStr should be copied")
	}
	if pubOnly.PrivateKeyStr != "" {
		t.Error("PrivateKeyStr should be empty in public identity")
	}
}

func TestWorkerIdentity_ToKeyPair(t *testing.T) {
	identity, _ := NewWorkerIdentity("worker")
	kp := identity.ToKeyPair()

	if kp == nil {
		t.Fatal("ToKeyPair should return non-nil")
	}

	if kp.PublicKey() != identity.PublicKey() {
		t.Error("PublicKey mismatch")
	}

	// Should work for encryption
	message := []byte("test")
	hq, _ := GenerateKeyPair()

	encrypted, err := hq.EncryptForRecipient(message, kp.PublicKey())
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := kp.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(decrypted, message) {
		t.Error("Roundtrip failed")
	}
}

func TestWorkerIdentity_Decrypt(t *testing.T) {
	identity, _ := NewWorkerIdentity("worker")
	hq, _ := GenerateKeyPair()

	message := []byte("secret message for worker")
	encrypted, err := hq.EncryptForRecipient(message, identity.PublicKey())
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Worker decrypts directly using identity
	decrypted, err := identity.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, message) {
		t.Error("Decrypted mismatch")
	}
}

func TestEnsureWorkerIdentity_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "subdir", "identity.json")

	identity, err := EnsureWorkerIdentity(identityPath, "new-worker")
	if err != nil {
		t.Fatalf("EnsureWorkerIdentity failed: %v", err)
	}

	if identity.ID != "new-worker" {
		t.Errorf("ID mismatch: got %q", identity.ID)
	}

	// File should exist
	if _, err := os.Stat(identityPath); err != nil {
		t.Error("Identity file should exist")
	}
}

func TestEnsureWorkerIdentity_LoadsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "identity.json")

	// Create first
	first, _ := NewWorkerIdentity("original")
	first.Save(identityPath)

	// Ensure should load existing
	loaded, err := EnsureWorkerIdentity(identityPath, "different-id")
	if err != nil {
		t.Fatalf("EnsureWorkerIdentity failed: %v", err)
	}

	// Should have the original ID, not the new one
	if loaded.ID != "original" {
		t.Errorf("Should load existing identity, got ID %q", loaded.ID)
	}
	if loaded.PublicKey() != first.PublicKey() {
		t.Error("Should load existing keys")
	}
}

func TestLoadWorkerIdentity_NotFound(t *testing.T) {
	_, err := LoadWorkerIdentity("/nonexistent/path/identity.json")
	if err == nil {
		t.Error("Should fail for nonexistent file")
	}
}

func TestLoadWorkerIdentity_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")

	os.WriteFile(path, []byte("not json"), 0600)

	_, err := LoadWorkerIdentity(path)
	if err == nil {
		t.Error("Should fail for invalid JSON")
	}
}
