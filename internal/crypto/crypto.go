// Package crypto provides encryption utilities for Dex secrets management.
// It implements two layers of protection:
//   - Layer 1: Application-level encryption for secrets at rest (AES-GCM)
//   - Layer 2: Age X25519 encryption for worker payload dispatch
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// MasterKeyEnvVar is the environment variable for the master encryption key.
	// If not set, a key will be derived from a generated key file.
	MasterKeyEnvVar = "DEX_MASTER_KEY"

	// KeySize is the size of encryption keys in bytes (256 bits).
	KeySize = 32

	// NonceSize is the size of AES-GCM nonces in bytes.
	NonceSize = 12

	// SaltSize is the size of PBKDF2 salt in bytes.
	SaltSize = 16

	// PBKDF2Iterations is the number of iterations for key derivation.
	PBKDF2Iterations = 100000
)

var (
	// ErrDecryptionFailed is returned when decryption fails (bad key or corrupted data).
	ErrDecryptionFailed = errors.New("decryption failed: invalid key or corrupted data")

	// ErrInvalidCiphertext is returned when ciphertext format is invalid.
	ErrInvalidCiphertext = errors.New("invalid ciphertext format")

	// ErrKeyNotInitialized is returned when encryption is attempted without a key.
	ErrKeyNotInitialized = errors.New("encryption key not initialized")
)

// MasterKey holds the derived master encryption key for secrets at rest.
type MasterKey struct {
	key  [KeySize]byte
	salt []byte
}

// NewMasterKey creates a MasterKey from a password/passphrase and salt.
// If salt is nil, a random salt is generated.
func NewMasterKey(password []byte, salt []byte) (*MasterKey, error) {
	if len(password) == 0 {
		return nil, errors.New("password cannot be empty")
	}

	if salt == nil {
		salt = make([]byte, SaltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	derived := pbkdf2.Key(password, salt, PBKDF2Iterations, KeySize, sha256.New)

	mk := &MasterKey{salt: salt}
	copy(mk.key[:], derived)

	return mk, nil
}

// NewMasterKeyFromEnv creates a MasterKey from the DEX_MASTER_KEY environment variable.
// If the env var is not set, it returns nil (no encryption).
func NewMasterKeyFromEnv() (*MasterKey, error) {
	keyStr := os.Getenv(MasterKeyEnvVar)
	if keyStr == "" {
		return nil, nil // No master key configured
	}

	// Decode base64 key (should be salt:key format)
	data, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid master key format: %w", err)
	}

	if len(data) < SaltSize+KeySize {
		return nil, errors.New("master key too short")
	}

	mk := &MasterKey{
		salt: data[:SaltSize],
	}
	copy(mk.key[:], data[SaltSize:SaltSize+KeySize])

	return mk, nil
}

// Salt returns the salt used for key derivation.
func (mk *MasterKey) Salt() []byte {
	return mk.salt
}

// Export returns the key in a format suitable for DEX_MASTER_KEY env var.
func (mk *MasterKey) Export() string {
	data := make([]byte, len(mk.salt)+KeySize)
	copy(data, mk.salt)
	copy(data[len(mk.salt):], mk.key[:])
	return base64.StdEncoding.EncodeToString(data)
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns base64-encoded ciphertext with nonce prepended.
func (mk *MasterKey) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(mk.key[:])
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext using AES-256-GCM.
func (mk *MasterKey) Decrypt(encoded string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(mk.key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// KeyPair represents an age X25519 identity for asymmetric encryption.
// This is used for encrypting secrets from HQ to workers.
type KeyPair struct {
	identity  *age.X25519Identity
	recipient *age.X25519Recipient
}

// GenerateKeyPair generates a new age X25519 identity.
func GenerateKeyPair() (*KeyPair, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}
	return &KeyPair{
		identity:  identity,
		recipient: identity.Recipient(),
	}, nil
}

// KeyPairFromPrivate reconstructs a KeyPair from a private key string.
// The private key should be in age format (AGE-SECRET-KEY-1...).
func KeyPairFromPrivate(privateKeyStr string) (*KeyPair, error) {
	identity, err := age.ParseX25519Identity(privateKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return &KeyPair{
		identity:  identity,
		recipient: identity.Recipient(),
	}, nil
}

// PublicKey returns the public key as a string (age format).
func (kp *KeyPair) PublicKey() string {
	return kp.recipient.String()
}

// PrivateKey returns the private key as a string (age format).
func (kp *KeyPair) PrivateKey() string {
	return kp.identity.String()
}

// PublicKeyBase64 returns the public key as a base64 string.
// This is for compatibility with existing code that expects base64.
func (kp *KeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString([]byte(kp.recipient.String()))
}

// PrivateKeyBase64 returns the private key as a base64 string.
// This is for compatibility with existing code that expects base64.
func (kp *KeyPair) PrivateKeyBase64() string {
	return base64.StdEncoding.EncodeToString([]byte(kp.identity.String()))
}

// RecipientFromPublicKey parses a public key string into an age recipient.
func RecipientFromPublicKey(publicKey string) (*age.X25519Recipient, error) {
	recipient, err := age.ParseX25519Recipient(publicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	return recipient, nil
}

// EncryptForRecipient encrypts a message for a specific recipient using age.
// Returns base64-encoded ciphertext.
func (kp *KeyPair) EncryptForRecipient(message []byte, recipientPublicKey string) (string, error) {
	recipient, err := age.ParseX25519Recipient(recipientPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid recipient public key: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return "", fmt.Errorf("failed to create encrypter: %w", err)
	}

	if _, err := w.Write(message); err != nil {
		return "", fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to close encrypter: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Decrypt decrypts a message that was encrypted for this identity.
func (kp *KeyPair) Decrypt(encoded string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	r, err := age.Decrypt(bytes.NewReader(ciphertext), kp.identity)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read decrypted message: %w", err)
	}

	return plaintext, nil
}

// EncryptForRecipientStatic encrypts without requiring a KeyPair (static function).
// This is useful when you only have the recipient's public key.
func EncryptForRecipientStatic(message []byte, recipientPublicKey string) (string, error) {
	recipient, err := age.ParseX25519Recipient(recipientPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid recipient public key: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return "", fmt.Errorf("failed to create encrypter: %w", err)
	}

	if _, err := w.Write(message); err != nil {
		return "", fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to close encrypter: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// GenerateMasterKey generates a new random master key suitable for DEX_MASTER_KEY.
func GenerateMasterKey() (*MasterKey, error) {
	password := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, password); err != nil {
		return nil, fmt.Errorf("failed to generate random password: %w", err)
	}
	return NewMasterKey(password, nil)
}

// ZeroKey securely zeroes a key in memory.
func ZeroKey(key []byte) {
	for i := range key {
		key[i] = 0
	}
}
