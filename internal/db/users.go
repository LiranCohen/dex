// Package db provides SQLite database access for Poindexter
package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID generates a new random ID for database entities
// Format: 8 random hex characters (e.g., "a1b2c3d4")
func NewID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewPrefixedID generates a new ID with a prefix (e.g., "user-a1b2c3d4")
func NewPrefixedID(prefix string) string {
	return prefix + "-" + NewID()
}

// CreateUser inserts a new user into the database with auto-generated ID
func (db *DB) CreateUser(publicKey string) (*User, error) {
	return db.CreateUserWithID(NewPrefixedID("user"), publicKey)
}

// CreateUserWithID inserts a new user with a specific ID
func (db *DB) CreateUserWithID(id, publicKey string) (*User, error) {
	user := &User{
		ID:        id,
		PublicKey: publicKey,
		CreatedAt: time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO users (id, public_key, created_at) VALUES (?, ?, ?)`,
		user.ID, user.PublicKey, user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(id string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		`SELECT id, public_key, created_at, last_login_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.PublicKey, &user.CreatedAt, &user.LastLoginAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByPublicKey retrieves a user by their public key
func (db *DB) GetUserByPublicKey(publicKey string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		`SELECT id, public_key, created_at, last_login_at FROM users WHERE public_key = ?`,
		publicKey,
	).Scan(&user.ID, &user.PublicKey, &user.CreatedAt, &user.LastLoginAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by public key: %w", err)
	}

	return user, nil
}

// UpdateUserLastLogin updates the last login time for a user
func (db *DB) UpdateUserLastLogin(id string) error {
	result, err := db.Exec(
		`UPDATE users SET last_login_at = ? WHERE id = ?`,
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update user last login: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user not found: %s", id)
	}

	return nil
}

// DeleteUser removes a user from the database
func (db *DB) DeleteUser(id string) error {
	result, err := db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user not found: %s", id)
	}

	return nil
}

// GetOrCreateUser retrieves a user by public key, creating one if it doesn't exist
func (db *DB) GetOrCreateUser(publicKey string) (*User, bool, error) {
	user, err := db.GetUserByPublicKey(publicKey)
	if err != nil {
		return nil, false, err
	}
	if user != nil {
		return user, false, nil // existing user
	}

	user, err = db.CreateUser(publicKey)
	if err != nil {
		return nil, false, err
	}
	return user, true, nil // new user
}
