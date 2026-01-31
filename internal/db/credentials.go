package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// CreateWebAuthnCredential stores a new WebAuthn credential
func (db *DB) CreateWebAuthnCredential(userID string, cred *webauthn.Credential) (*WebAuthnCredential, error) {
	id := uuid.New().String()
	now := time.Now()

	_, err := db.Exec(`
		INSERT INTO webauthn_credentials (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, cred.ID, cred.PublicKey, cred.AttestationType, cred.Authenticator.AAGUID, cred.Authenticator.SignCount, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	return &WebAuthnCredential{
		ID:              id,
		UserID:          userID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		CreatedAt:       now,
	}, nil
}

// GetWebAuthnCredentialsByUserID retrieves all credentials for a user
func (db *DB) GetWebAuthnCredentialsByUserID(userID string) ([]webauthn.Credential, error) {
	rows, err := db.Query(`
		SELECT credential_id, public_key, attestation_type, aaguid, sign_count
		FROM webauthn_credentials
		WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer rows.Close()

	var credentials []webauthn.Credential
	for rows.Next() {
		var credID, publicKey, aaguid []byte
		var attestationType string
		var signCount uint32

		if err := rows.Scan(&credID, &publicKey, &attestationType, &aaguid, &signCount); err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}

		credentials = append(credentials, webauthn.Credential{
			ID:              credID,
			PublicKey:       publicKey,
			AttestationType: attestationType,
			Authenticator: webauthn.Authenticator{
				AAGUID:    aaguid,
				SignCount: signCount,
			},
		})
	}

	return credentials, rows.Err()
}

// GetWebAuthnCredentialByCredentialID retrieves a credential by its credential ID
func (db *DB) GetWebAuthnCredentialByCredentialID(credentialID []byte) (*WebAuthnCredential, error) {
	var cred WebAuthnCredential
	err := db.QueryRow(`
		SELECT id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, created_at
		FROM webauthn_credentials
		WHERE credential_id = ?
	`, credentialID).Scan(&cred.ID, &cred.UserID, &cred.CredentialID, &cred.PublicKey, &cred.AttestationType, &cred.AAGUID, &cred.SignCount, &cred.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}
	return &cred, nil
}

// UpdateWebAuthnCredentialCounter updates the sign count for a credential
func (db *DB) UpdateWebAuthnCredentialCounter(credentialID []byte, newCount uint32) error {
	_, err := db.Exec(`
		UPDATE webauthn_credentials
		SET sign_count = ?
		WHERE credential_id = ?
	`, newCount, credentialID)
	if err != nil {
		return fmt.Errorf("failed to update credential counter: %w", err)
	}
	return nil
}

// HasAnyCredentials checks if any credentials exist (for first-time setup detection)
func (db *DB) HasAnyCredentials() (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to count credentials: %w", err)
	}
	return count > 0, nil
}

// HasAnyUsers checks if any users exist
func (db *DB) HasAnyUsers() (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to count users: %w", err)
	}
	return count > 0, nil
}

// GetFirstUser gets the first (and typically only) user for single-user mode
func (db *DB) GetFirstUser() (*User, error) {
	var user User
	err := db.QueryRow(`
		SELECT id, public_key, created_at, last_login_at
		FROM users
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&user.ID, &user.PublicKey, &user.CreatedAt, &user.LastLoginAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get first user: %w", err)
	}
	return &user, nil
}
