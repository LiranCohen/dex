// Package auth provides authentication utilities for Poindexter
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

// OnboardingToken represents a short-lived token for mesh passkey onboarding
type OnboardingToken struct {
	TokenHash  [32]byte  // SHA-256 hash of the token
	UserID     string    // User this token is for
	MeshRPID   string    // Target mesh rpID (e.g., "alice.dex")
	MeshOrigin string    // Target mesh origin (e.g., "https://hq.alice.dex")
	CreatedAt  time.Time // When the token was created
	Used       bool      // Whether the token has been consumed
}

// OnboardingTokenStore manages short-lived onboarding tokens in memory
type OnboardingTokenStore struct {
	mu     sync.RWMutex
	tokens map[[32]byte]*OnboardingToken
	ttl    time.Duration
}

// NewOnboardingTokenStore creates a new token store with the given TTL
func NewOnboardingTokenStore(ttl time.Duration) *OnboardingTokenStore {
	store := &OnboardingTokenStore{
		tokens: make(map[[32]byte]*OnboardingToken),
		ttl:    ttl,
	}
	// Start background cleanup goroutine
	go store.cleanup()
	return store
}

// GenerateToken creates a new onboarding token for a user
// Returns the raw token (to be sent to the user) and any error
func (s *OnboardingTokenStore) GenerateToken(userID, meshRPID, meshOrigin string) (string, error) {
	// Generate 32 random bytes (256 bits of entropy)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}

	// Base64url encode for URL-safe transport
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// Store hash of token (not the token itself)
	tokenHash := sha256.Sum256(tokenBytes)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[tokenHash] = &OnboardingToken{
		TokenHash:  tokenHash,
		UserID:     userID,
		MeshRPID:   meshRPID,
		MeshOrigin: meshOrigin,
		CreatedAt:  time.Now(),
		Used:       false,
	}

	return rawToken, nil
}

// ValidateAndConsumeToken validates a token and marks it as used
// Returns the token data if valid, nil if invalid/expired/used
func (s *OnboardingTokenStore) ValidateAndConsumeToken(rawToken string) *OnboardingToken {
	// Decode the token
	tokenBytes, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil {
		return nil
	}

	// Hash to look up
	tokenHash := sha256.Sum256(tokenBytes)

	s.mu.Lock()
	defer s.mu.Unlock()

	token, exists := s.tokens[tokenHash]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Since(token.CreatedAt) > s.ttl {
		delete(s.tokens, tokenHash)
		return nil
	}

	// Check if already used
	if token.Used {
		return nil
	}

	// Mark as used (single-use)
	token.Used = true

	// Return a copy to avoid race conditions
	result := &OnboardingToken{
		TokenHash:  token.TokenHash,
		UserID:     token.UserID,
		MeshRPID:   token.MeshRPID,
		MeshOrigin: token.MeshOrigin,
		CreatedAt:  token.CreatedAt,
		Used:       true,
	}

	return result
}

// cleanup periodically removes expired tokens
func (s *OnboardingTokenStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for hash, token := range s.tokens {
			if now.Sub(token.CreatedAt) > s.ttl {
				delete(s.tokens, hash)
			}
		}
		s.mu.Unlock()
	}
}

// DefaultOnboardingTokenTTL is the default time-to-live for onboarding tokens (60 seconds)
const DefaultOnboardingTokenTTL = 60 * time.Second
