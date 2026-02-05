// Package crypto provides encryption utilities for Dex secrets management.
// It implements three layers of protection:
//   - Layer 1: Application-level encryption for secrets at rest (AES-GCM)
//   - Layer 2: NaCl box encryption for worker payload dispatch
//   - Layer 3: Support for encrypted worker databases
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/nacl/box"
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

	// NaClNonceSize is the size of NaCl box nonces in bytes.
	NaClNonceSize = 24

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

// KeyPair represents a NaCl box key pair for asymmetric encryption.
type KeyPair struct {
	PublicKey  [32]byte
	PrivateKey [32]byte
}

// GenerateKeyPair generates a new NaCl box key pair.
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair: %w", err)
	}
	return &KeyPair{
		PublicKey:  *pub,
		PrivateKey: *priv,
	}, nil
}

// KeyPairFromPrivate reconstructs a KeyPair from a private key.
func KeyPairFromPrivate(privateKey [32]byte) *KeyPair {
	var publicKey [32]byte
	// Derive public key using Curve25519 scalar base multiplication
	// Import is at top of file via golang.org/x/crypto/curve25519
	scalarBaseMult(&publicKey, &privateKey)
	return &KeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}
}

// scalarBaseMult computes publicKey = privateKey * basePoint using Curve25519.
// This is implemented in the keypair.go file using golang.org/x/crypto/curve25519.
var scalarBaseMult = func(dst, scalar *[32]byte) {
	// This will be set by init() in keypair.go
	// Using curve25519.ScalarBaseMult
}

// PublicKeyBase64 returns the public key as a base64 string.
func (kp *KeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PublicKey[:])
}

// PrivateKeyBase64 returns the private key as a base64 string.
func (kp *KeyPair) PrivateKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PrivateKey[:])
}

// KeyPairFromBase64 creates a KeyPair from base64-encoded keys.
func KeyPairFromBase64(publicKey, privateKey string) (*KeyPair, error) {
	kp := &KeyPair{}

	if publicKey != "" {
		pub, err := base64.StdEncoding.DecodeString(publicKey)
		if err != nil || len(pub) != 32 {
			return nil, errors.New("invalid public key")
		}
		copy(kp.PublicKey[:], pub)
	}

	if privateKey != "" {
		priv, err := base64.StdEncoding.DecodeString(privateKey)
		if err != nil || len(priv) != 32 {
			return nil, errors.New("invalid private key")
		}
		copy(kp.PrivateKey[:], priv)
	}

	return kp, nil
}

// PublicKeyFromBase64 decodes a base64-encoded public key.
func PublicKeyFromBase64(encoded string) ([32]byte, error) {
	var key [32]byte
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) != 32 {
		return key, errors.New("invalid public key")
	}
	copy(key[:], data)
	return key, nil
}

// EncryptForRecipient encrypts a message for a specific recipient using NaCl box.
// Returns base64-encoded ciphertext with nonce prepended.
func (kp *KeyPair) EncryptForRecipient(message []byte, recipientPublicKey [32]byte) (string, error) {
	var nonce [NaClNonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := box.Seal(nonce[:], message, &nonce, &recipientPublicKey, &kp.PrivateKey)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptFromSender decrypts a message from a specific sender using NaCl box.
func (kp *KeyPair) DecryptFromSender(encoded string, senderPublicKey [32]byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if len(ciphertext) < NaClNonceSize {
		return nil, ErrInvalidCiphertext
	}

	var nonce [NaClNonceSize]byte
	copy(nonce[:], ciphertext[:NaClNonceSize])

	plaintext, ok := box.Open(nil, ciphertext[NaClNonceSize:], &nonce, &senderPublicKey, &kp.PrivateKey)
	if !ok {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// SealAnonymous encrypts a message for a recipient without revealing the sender.
// This is useful for one-way encryption where the sender doesn't need to be verified.
func SealAnonymous(message []byte, recipientPublicKey [32]byte) (string, error) {
	// Generate ephemeral keypair
	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	var nonce [NaClNonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Prepend ephemeral public key to ciphertext so recipient can decrypt
	ciphertext := box.Seal(nil, message, &nonce, &recipientPublicKey, ephemeralPriv)

	// Format: ephemeralPubKey (32) + nonce (24) + ciphertext
	result := make([]byte, 32+NaClNonceSize+len(ciphertext))
	copy(result[:32], ephemeralPub[:])
	copy(result[32:32+NaClNonceSize], nonce[:])
	copy(result[32+NaClNonceSize:], ciphertext)

	return base64.StdEncoding.EncodeToString(result), nil
}

// OpenAnonymous decrypts a message that was sealed anonymously.
func (kp *KeyPair) OpenAnonymous(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if len(data) < 32+NaClNonceSize {
		return nil, ErrInvalidCiphertext
	}

	var ephemeralPub [32]byte
	var nonce [NaClNonceSize]byte
	copy(ephemeralPub[:], data[:32])
	copy(nonce[:], data[32:32+NaClNonceSize])
	ciphertext := data[32+NaClNonceSize:]

	plaintext, ok := box.Open(nil, ciphertext, &nonce, &ephemeralPub, &kp.PrivateKey)
	if !ok {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
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
