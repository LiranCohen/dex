package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Secret keys used by the application
const (
	SecretKeyGitHubToken  = "github_token"
	SecretKeyAnthropicKey = "anthropic_key"
)

// SetSecret stores a secret in the database
func (db *DB) SetSecret(key, value string) error {
	now := time.Now()
	_, err := db.Exec(`
		INSERT INTO secrets (key, value, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value, now)
	if err != nil {
		return fmt.Errorf("failed to set secret %s: %w", key, err)
	}
	return nil
}

// GetSecret retrieves a secret from the database
// Returns empty string and nil error if not found
func (db *DB) GetSecret(key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM secrets WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", key, err)
	}
	return value, nil
}

// HasSecret returns true if a secret exists and is non-empty
func (db *DB) HasSecret(key string) bool {
	value, err := db.GetSecret(key)
	return err == nil && value != ""
}

// DeleteSecret removes a secret from the database
func (db *DB) DeleteSecret(key string) error {
	_, err := db.Exec(`DELETE FROM secrets WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("failed to delete secret %s: %w", key, err)
	}
	return nil
}

// ListSecretKeys returns all secret keys (not values) in the database
func (db *DB) ListSecretKeys() ([]string, error) {
	rows, err := db.Query(`SELECT key FROM secrets ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	defer rows.Close()

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

// MigrateSecretsFromFile migrates secrets from the old secrets.json file to the database
// Returns the number of secrets migrated, or an error
func (db *DB) MigrateSecretsFromFile(dataDir string) (int, error) {
	secretsFile := filepath.Join(dataDir, "secrets.json")

	// Check if file exists
	data, err := os.ReadFile(secretsFile)
	if os.IsNotExist(err) {
		return 0, nil // No file to migrate
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read secrets file: %w", err)
	}

	// Parse JSON
	var secrets map[string]string
	if err := json.Unmarshal(data, &secrets); err != nil {
		return 0, fmt.Errorf("failed to parse secrets file: %w", err)
	}

	// Migrate each secret to database
	count := 0
	for key, value := range secrets {
		if value == "" {
			continue
		}

		// Check if already exists in database
		existing, _ := db.GetSecret(key)
		if existing != "" {
			continue // Don't overwrite existing database values
		}

		if err := db.SetSecret(key, value); err != nil {
			return count, fmt.Errorf("failed to migrate secret %s: %w", key, err)
		}
		count++
	}

	// Optionally rename the old file to indicate it's been migrated
	if count > 0 {
		backupFile := secretsFile + ".migrated"
		os.Rename(secretsFile, backupFile)
	}

	return count, nil
}

// GetAllSecrets returns all secrets as a map (for toolbelt initialization)
func (db *DB) GetAllSecrets() (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM secrets`)
	if err != nil {
		return nil, fmt.Errorf("failed to get all secrets: %w", err)
	}
	defer rows.Close()

	secrets := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan secret: %w", err)
		}
		secrets[key] = value
	}
	return secrets, rows.Err()
}
