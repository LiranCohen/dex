package db

import (
	"database/sql"
	"fmt"
	"time"
)

// GitHubAppConfig represents the GitHub App configuration stored in the database
type GitHubAppConfig struct {
	AppID         int64
	AppSlug       string
	ClientID      string
	ClientSecret  string
	PrivateKey    string
	WebhookSecret string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// GitHubInstallation represents a GitHub App installation
type GitHubInstallation struct {
	ID          int64
	AccountID   int64
	AccountType string
	Login       string
	CreatedAt   time.Time
}

// GetGitHubAppConfig retrieves the GitHub App configuration
// Returns nil, nil if no configuration exists
func (db *DB) GetGitHubAppConfig() (*GitHubAppConfig, error) {
	var config GitHubAppConfig
	err := db.QueryRow(`
		SELECT app_id, app_slug, client_id, client_secret, private_key,
		       COALESCE(webhook_secret, ''), created_at, updated_at
		FROM github_app_config WHERE id = 1
	`).Scan(
		&config.AppID, &config.AppSlug, &config.ClientID, &config.ClientSecret,
		&config.PrivateKey, &config.WebhookSecret, &config.CreatedAt, &config.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub App config: %w", err)
	}

	return &config, nil
}

// SaveGitHubAppConfig saves the GitHub App configuration (upsert)
func (db *DB) SaveGitHubAppConfig(config *GitHubAppConfig) error {
	now := time.Now()

	_, err := db.Exec(`
		INSERT INTO github_app_config (id, app_id, app_slug, client_id, client_secret, private_key, webhook_secret, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			app_id = excluded.app_id,
			app_slug = excluded.app_slug,
			client_id = excluded.client_id,
			client_secret = excluded.client_secret,
			private_key = excluded.private_key,
			webhook_secret = excluded.webhook_secret,
			updated_at = excluded.updated_at
	`, config.AppID, config.AppSlug, config.ClientID, config.ClientSecret,
		config.PrivateKey, config.WebhookSecret, now, now)

	if err != nil {
		return fmt.Errorf("failed to save GitHub App config: %w", err)
	}

	return nil
}

// DeleteGitHubAppConfig removes the GitHub App configuration
func (db *DB) DeleteGitHubAppConfig() error {
	_, err := db.Exec(`DELETE FROM github_app_config WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub App config: %w", err)
	}
	return nil
}

// GetGitHubInstallation retrieves an installation by login
func (db *DB) GetGitHubInstallation(login string) (*GitHubInstallation, error) {
	var install GitHubInstallation
	err := db.QueryRow(`
		SELECT id, account_id, account_type, login, created_at
		FROM github_installations WHERE login = ?
	`, login).Scan(
		&install.ID, &install.AccountID, &install.AccountType,
		&install.Login, &install.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub installation: %w", err)
	}

	return &install, nil
}

// GetGitHubInstallationByID retrieves an installation by ID
func (db *DB) GetGitHubInstallationByID(id int64) (*GitHubInstallation, error) {
	var install GitHubInstallation
	err := db.QueryRow(`
		SELECT id, account_id, account_type, login, created_at
		FROM github_installations WHERE id = ?
	`, id).Scan(
		&install.ID, &install.AccountID, &install.AccountType,
		&install.Login, &install.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub installation: %w", err)
	}

	return &install, nil
}

// SaveGitHubInstallation saves an installation (upsert by ID)
func (db *DB) SaveGitHubInstallation(install *GitHubInstallation) error {
	now := time.Now()

	_, err := db.Exec(`
		INSERT INTO github_installations (id, account_id, account_type, login, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			account_id = excluded.account_id,
			account_type = excluded.account_type,
			login = excluded.login
	`, install.ID, install.AccountID, install.AccountType, install.Login, now)

	if err != nil {
		return fmt.Errorf("failed to save GitHub installation: %w", err)
	}

	return nil
}

// ListGitHubInstallations returns all installations
func (db *DB) ListGitHubInstallations() ([]*GitHubInstallation, error) {
	rows, err := db.Query(`
		SELECT id, account_id, account_type, login, created_at
		FROM github_installations ORDER BY login
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list GitHub installations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var installs []*GitHubInstallation
	for rows.Next() {
		var install GitHubInstallation
		if err := rows.Scan(&install.ID, &install.AccountID, &install.AccountType,
			&install.Login, &install.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan installation: %w", err)
		}
		installs = append(installs, &install)
	}

	return installs, rows.Err()
}

// DeleteGitHubInstallation removes an installation by ID
func (db *DB) DeleteGitHubInstallation(id int64) error {
	_, err := db.Exec(`DELETE FROM github_installations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub installation: %w", err)
	}
	return nil
}

// HasGitHubApp returns true if a GitHub App is configured
func (db *DB) HasGitHubApp() bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM github_app_config WHERE id = 1`).Scan(&count)
	return err == nil && count > 0
}

// HasGitHubInstallation returns true if any installation exists
func (db *DB) HasGitHubInstallation() bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM github_installations`).Scan(&count)
	return err == nil && count > 0
}
