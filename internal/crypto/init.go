package crypto

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// MasterKeyFile is the filename for the auto-generated master key.
	MasterKeyFile = "master.key"

	// HQIdentityFile is the filename for HQ's keypair.
	HQIdentityFile = "hq_identity.json"
)

// EncryptionConfig holds the encryption configuration for a Dex instance.
type EncryptionConfig struct {
	// MasterKey is used for encrypting secrets at rest.
	// If nil, encryption is disabled.
	MasterKey *MasterKey

	// HQKeyPair is used for encrypting payloads to/from workers.
	// Only needed if workers are used.
	HQKeyPair *KeyPair

	// DataDir is the directory where encryption keys are stored.
	DataDir string
}

// InitEncryption initializes encryption for a Dex instance.
// It loads or creates the master key and HQ keypair.
//
// The master key is loaded in this order of precedence:
//  1. DEX_MASTER_KEY environment variable (if set)
//  2. {dataDir}/master.key file (if exists)
//  3. Auto-generated and saved to {dataDir}/master.key
//
// Set createIfMissing=false to disable auto-generation.
func InitEncryption(dataDir string, createIfMissing bool) (*EncryptionConfig, error) {
	config := &EncryptionConfig{
		DataDir: dataDir,
	}

	// Try to load master key from environment
	mk, err := NewMasterKeyFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load master key from env: %w", err)
	}

	if mk != nil {
		config.MasterKey = mk
	} else {
		// Try to load from file
		keyPath := filepath.Join(dataDir, MasterKeyFile)
		mk, err = loadMasterKeyFromFile(keyPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load master key from file: %w", err)
		}

		if mk != nil {
			config.MasterKey = mk
		} else if createIfMissing {
			// Generate new master key
			mk, err = GenerateMasterKey()
			if err != nil {
				return nil, fmt.Errorf("failed to generate master key: %w", err)
			}

			if err := saveMasterKeyToFile(mk, keyPath); err != nil {
				return nil, fmt.Errorf("failed to save master key: %w", err)
			}

			config.MasterKey = mk
		}
	}

	// Load or create HQ keypair for worker communication
	if config.MasterKey != nil {
		identityPath := filepath.Join(dataDir, HQIdentityFile)
		identity, err := EnsureWorkerIdentity(identityPath, "hq")
		if err != nil {
			return nil, fmt.Errorf("failed to initialize HQ identity: %w", err)
		}
		config.HQKeyPair = identity.ToKeyPair()
	}

	return config, nil
}

// loadMasterKeyFromFile loads a master key from a file.
func loadMasterKeyFromFile(path string) (*MasterKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// The file contains the exported key (base64 of salt+key)
	keyStr := string(data)
	if len(keyStr) == 0 {
		return nil, fmt.Errorf("empty master key file")
	}

	// Temporarily set env var to use existing parsing logic
	_ = os.Setenv(MasterKeyEnvVar, keyStr)
	defer func() { _ = os.Unsetenv(MasterKeyEnvVar) }()

	return NewMasterKeyFromEnv()
}

// saveMasterKeyToFile saves a master key to a file with restricted permissions.
func saveMasterKeyToFile(mk *MasterKey, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(path, []byte(mk.Export()), 0600); err != nil {
		return fmt.Errorf("failed to write master key file: %w", err)
	}

	return nil
}

// MigrateSecrets migrates plaintext secrets to encrypted format.
// This should be called after InitEncryption to encrypt any existing plaintext secrets.
type SecretsMigrator interface {
	MigrateToEncrypted() (int, error)
}

// GitHubMigrator migrates GitHub App config to encrypted format.
type GitHubMigrator interface {
	MigrateToEncrypted() error
}
