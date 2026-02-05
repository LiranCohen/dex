package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
)

// EncryptedSecretsStore provides encrypted storage for secrets.
// It wraps the basic secrets operations with encryption/decryption.
type EncryptedSecretsStore struct {
	db        *DB
	masterKey *crypto.MasterKey
}

// NewEncryptedSecretsStore creates a new encrypted secrets store.
// If masterKey is nil, secrets are stored in plaintext (backwards compatible).
func NewEncryptedSecretsStore(db *DB, masterKey *crypto.MasterKey) *EncryptedSecretsStore {
	return &EncryptedSecretsStore{
		db:        db,
		masterKey: masterKey,
	}
}

// SetSecret stores a secret, encrypting it if a master key is configured.
func (s *EncryptedSecretsStore) SetSecret(key, value string) error {
	now := time.Now()

	var storedValue string
	var encrypted bool

	if s.masterKey != nil {
		// Encrypt the value
		enc, err := s.masterKey.Encrypt([]byte(value))
		if err != nil {
			return fmt.Errorf("failed to encrypt secret: %w", err)
		}
		storedValue = enc
		encrypted = true
	} else {
		storedValue = value
		encrypted = false
	}

	_, err := s.db.Exec(`
		INSERT INTO secrets (key, value, encrypted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			encrypted = excluded.encrypted,
			updated_at = excluded.updated_at
	`, key, storedValue, encrypted, now, now)
	if err != nil {
		return fmt.Errorf("failed to set secret %s: %w", key, err)
	}
	return nil
}

// GetSecret retrieves and decrypts a secret.
// Returns empty string and nil error if not found.
func (s *EncryptedSecretsStore) GetSecret(key string) (string, error) {
	var value string
	var encrypted bool

	err := s.db.QueryRow(`
		SELECT value, encrypted FROM secrets WHERE key = ?
	`, key).Scan(&value, &encrypted)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", key, err)
	}

	if encrypted && s.masterKey != nil {
		decrypted, err := s.masterKey.Decrypt(value)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt secret %s: %w", key, err)
		}
		return string(decrypted), nil
	}

	// Return as-is if not encrypted or no master key
	return value, nil
}

// HasSecret returns true if a secret exists and is non-empty.
func (s *EncryptedSecretsStore) HasSecret(key string) bool {
	value, err := s.GetSecret(key)
	return err == nil && value != ""
}

// DeleteSecret removes a secret from the database.
func (s *EncryptedSecretsStore) DeleteSecret(key string) error {
	_, err := s.db.Exec(`DELETE FROM secrets WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("failed to delete secret %s: %w", key, err)
	}
	return nil
}

// ListSecretKeys returns all secret keys (not values) in the database.
func (s *EncryptedSecretsStore) ListSecretKeys() ([]string, error) {
	rows, err := s.db.Query(`SELECT key FROM secrets ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("failed to scan secret key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// GetAllSecrets returns all secrets as a map (for toolbelt initialization).
func (s *EncryptedSecretsStore) GetAllSecrets() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value, encrypted FROM secrets`)
	if err != nil {
		return nil, fmt.Errorf("failed to get all secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	secrets := make(map[string]string)
	for rows.Next() {
		var key, value string
		var encrypted bool
		if err := rows.Scan(&key, &value, &encrypted); err != nil {
			return nil, fmt.Errorf("failed to scan secret: %w", err)
		}

		if encrypted && s.masterKey != nil {
			decrypted, err := s.masterKey.Decrypt(value)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt secret %s: %w", key, err)
			}
			secrets[key] = string(decrypted)
		} else {
			secrets[key] = value
		}
	}
	return secrets, rows.Err()
}

// MigrateToEncrypted encrypts all plaintext secrets with the master key.
// This is idempotent - already encrypted secrets are skipped.
func (s *EncryptedSecretsStore) MigrateToEncrypted() (int, error) {
	if s.masterKey == nil {
		return 0, nil // No master key, nothing to migrate
	}

	// Get all plaintext secrets
	rows, err := s.db.Query(`SELECT key, value FROM secrets WHERE encrypted = 0 OR encrypted IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("failed to query plaintext secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type secret struct {
		key   string
		value string
	}
	var toMigrate []secret
	for rows.Next() {
		var s secret
		if err := rows.Scan(&s.key, &s.value); err != nil {
			return 0, fmt.Errorf("failed to scan secret: %w", err)
		}
		if s.value != "" {
			toMigrate = append(toMigrate, s)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// Encrypt each secret
	count := 0
	for _, sec := range toMigrate {
		encrypted, err := s.masterKey.Encrypt([]byte(sec.value))
		if err != nil {
			return count, fmt.Errorf("failed to encrypt secret %s: %w", sec.key, err)
		}

		_, err = s.db.Exec(`
			UPDATE secrets SET value = ?, encrypted = 1, updated_at = ? WHERE key = ?
		`, encrypted, time.Now(), sec.key)
		if err != nil {
			return count, fmt.Errorf("failed to update secret %s: %w", sec.key, err)
		}
		count++
	}

	return count, nil
}

// RotateMasterKey re-encrypts all secrets with a new master key.
func (s *EncryptedSecretsStore) RotateMasterKey(newKey *crypto.MasterKey) error {
	if s.masterKey == nil || newKey == nil {
		return fmt.Errorf("both old and new master keys must be provided")
	}

	// Get all secrets (decrypted with old key)
	secrets, err := s.GetAllSecrets()
	if err != nil {
		return fmt.Errorf("failed to get secrets for rotation: %w", err)
	}

	// Update to new key
	s.masterKey = newKey

	// Re-encrypt all secrets with new key
	for key, value := range secrets {
		if err := s.SetSecret(key, value); err != nil {
			return fmt.Errorf("failed to re-encrypt secret %s: %w", key, err)
		}
	}

	return nil
}
