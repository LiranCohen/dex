package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
)

// EncryptedGitHubStore provides encrypted storage for GitHub App credentials.
type EncryptedGitHubStore struct {
	db        *DB
	masterKey *crypto.MasterKey
}

// NewEncryptedGitHubStore creates a new encrypted GitHub store.
// If masterKey is nil, credentials are stored in plaintext (backwards compatible).
func NewEncryptedGitHubStore(db *DB, masterKey *crypto.MasterKey) *EncryptedGitHubStore {
	return &EncryptedGitHubStore{
		db:        db,
		masterKey: masterKey,
	}
}

// GetGitHubAppConfig retrieves and decrypts the GitHub App configuration.
func (s *EncryptedGitHubStore) GetGitHubAppConfig() (*GitHubAppConfig, error) {
	var config GitHubAppConfig
	var encrypted bool

	err := s.db.QueryRow(`
		SELECT app_id, app_slug, client_id, client_secret, private_key,
		       COALESCE(webhook_secret, ''), COALESCE(encrypted, 0), created_at, updated_at
		FROM github_app_config WHERE id = 1
	`).Scan(
		&config.AppID, &config.AppSlug, &config.ClientID, &config.ClientSecret,
		&config.PrivateKey, &config.WebhookSecret, &encrypted, &config.CreatedAt, &config.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub App config: %w", err)
	}

	// Decrypt sensitive fields if encrypted
	if encrypted && s.masterKey != nil {
		if config.ClientSecret != "" {
			decrypted, err := s.masterKey.Decrypt(config.ClientSecret)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt client secret: %w", err)
			}
			config.ClientSecret = string(decrypted)
		}

		if config.PrivateKey != "" {
			decrypted, err := s.masterKey.Decrypt(config.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt private key: %w", err)
			}
			config.PrivateKey = string(decrypted)
		}

		if config.WebhookSecret != "" {
			decrypted, err := s.masterKey.Decrypt(config.WebhookSecret)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt webhook secret: %w", err)
			}
			config.WebhookSecret = string(decrypted)
		}
	}

	return &config, nil
}

// SaveGitHubAppConfig saves and optionally encrypts the GitHub App configuration.
func (s *EncryptedGitHubStore) SaveGitHubAppConfig(config *GitHubAppConfig) error {
	now := time.Now()
	encrypted := false

	clientSecret := config.ClientSecret
	privateKey := config.PrivateKey
	webhookSecret := config.WebhookSecret

	if s.masterKey != nil {
		encrypted = true

		if clientSecret != "" {
			enc, err := s.masterKey.Encrypt([]byte(clientSecret))
			if err != nil {
				return fmt.Errorf("failed to encrypt client secret: %w", err)
			}
			clientSecret = enc
		}

		if privateKey != "" {
			enc, err := s.masterKey.Encrypt([]byte(privateKey))
			if err != nil {
				return fmt.Errorf("failed to encrypt private key: %w", err)
			}
			privateKey = enc
		}

		if webhookSecret != "" {
			enc, err := s.masterKey.Encrypt([]byte(webhookSecret))
			if err != nil {
				return fmt.Errorf("failed to encrypt webhook secret: %w", err)
			}
			webhookSecret = enc
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO github_app_config (id, app_id, app_slug, client_id, client_secret, private_key, webhook_secret, encrypted, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			app_id = excluded.app_id,
			app_slug = excluded.app_slug,
			client_id = excluded.client_id,
			client_secret = excluded.client_secret,
			private_key = excluded.private_key,
			webhook_secret = excluded.webhook_secret,
			encrypted = excluded.encrypted,
			updated_at = excluded.updated_at
	`, config.AppID, config.AppSlug, config.ClientID, clientSecret,
		privateKey, webhookSecret, encrypted, now, now)

	if err != nil {
		return fmt.Errorf("failed to save GitHub App config: %w", err)
	}

	return nil
}

// MigrateToEncrypted encrypts existing plaintext GitHub App config.
func (s *EncryptedGitHubStore) MigrateToEncrypted() error {
	if s.masterKey == nil {
		return nil
	}

	// Get current config (will be plaintext if not encrypted)
	config, err := s.db.GetGitHubAppConfig()
	if err != nil {
		return err
	}
	if config == nil {
		return nil // No config to migrate
	}

	// Check if already encrypted
	var encrypted bool
	s.db.QueryRow(`SELECT COALESCE(encrypted, 0) FROM github_app_config WHERE id = 1`).Scan(&encrypted)
	if encrypted {
		return nil // Already encrypted
	}

	// Re-save with encryption
	return s.SaveGitHubAppConfig(config)
}
