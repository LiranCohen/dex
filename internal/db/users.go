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
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewPrefixedID generates a new ID with a prefix (e.g., "user-a1b2c3d4")
func NewPrefixedID(prefix string) string {
	return prefix + "-" + NewID()
}

// CreateUser inserts a new user into the database with auto-generated ID
func (db *DB) CreateUser(email string) (*User, error) {
	return db.CreateUserWithID(NewPrefixedID("user"), email)
}

// CreateUserWithID inserts a new user with a specific ID
func (db *DB) CreateUserWithID(id, email string) (*User, error) {
	user := &User{
		ID:        id,
		Email:     email,
		CreatedAt: time.Now(),
	}

	_, err := db.Exec(
		`INSERT INTO users (id, email, created_at) VALUES (?, ?, ?)`,
		user.ID, user.Email, user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(id string) (*User, error) {
	user := &User{}
	var email sql.NullString
	err := db.QueryRow(
		`SELECT id, email, created_at, last_login_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &email, &user.CreatedAt, &user.LastLoginAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	user.Email = email.String
	return user, nil
}

// GetUserByEmail retrieves a user by their email
func (db *DB) GetUserByEmail(email string) (*User, error) {
	user := &User{}
	var emailVal sql.NullString
	err := db.QueryRow(
		`SELECT id, email, created_at, last_login_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &emailVal, &user.CreatedAt, &user.LastLoginAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	user.Email = emailVal.String
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

// GetOrCreateUserByEmail retrieves a user by email, creating one if it doesn't exist
func (db *DB) GetOrCreateUserByEmail(email string) (*User, bool, error) {
	user, err := db.GetUserByEmail(email)
	if err != nil {
		return nil, false, err
	}
	if user != nil {
		return user, false, nil // existing user
	}

	user, err = db.CreateUser(email)
	if err != nil {
		return nil, false, err
	}
	return user, true, nil // new user
}
