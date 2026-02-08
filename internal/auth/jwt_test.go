package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func generateTestKeyPair() (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	return pub, priv
}

func TestGenerateToken(t *testing.T) {
	pub, priv := generateTestKeyPair()
	config := &TokenConfig{
		Issuer:       "test-issuer",
		ExpiryHours:  24,
		SigningKey:   priv,
		VerifyingKey: pub,
	}

	t.Run("generates valid token", func(t *testing.T) {
		token, err := GenerateToken("user-123", config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == "" {
			t.Error("expected non-empty token")
		}
	})

	t.Run("different users get different tokens", func(t *testing.T) {
		token1, _ := GenerateToken("user-1", config)
		token2, _ := GenerateToken("user-2", config)
		if token1 == token2 {
			t.Error("expected different tokens for different users")
		}
	})
}

func TestValidateToken(t *testing.T) {
	pub, priv := generateTestKeyPair()
	config := &TokenConfig{
		Issuer:       "test-issuer",
		ExpiryHours:  24,
		SigningKey:   priv,
		VerifyingKey: pub,
	}

	t.Run("validates correct token", func(t *testing.T) {
		token, _ := GenerateToken("user-123", config)
		claims, err := ValidateToken(token, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if claims.UserID != "user-123" {
			t.Errorf("expected user ID 'user-123', got '%s'", claims.UserID)
		}
		if claims.Subject != "user-123" {
			t.Errorf("expected subject 'user-123', got '%s'", claims.Subject)
		}
		if claims.Issuer != "test-issuer" {
			t.Errorf("expected issuer 'test-issuer', got '%s'", claims.Issuer)
		}
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		_, err := ValidateToken("invalid-token", config)
		if err != ErrInvalidToken {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})

	t.Run("rejects expired token", func(t *testing.T) {
		expiredConfig := &TokenConfig{
			Issuer:       "test-issuer",
			ExpiryHours:  0, // Expires immediately
			SigningKey:   priv,
			VerifyingKey: pub,
		}
		token, _ := GenerateToken("user-123", expiredConfig)

		// Wait a moment for token to expire
		time.Sleep(10 * time.Millisecond)

		_, err := ValidateToken(token, config)
		if err != ErrExpiredToken {
			t.Errorf("expected ErrExpiredToken, got %v", err)
		}
	})

	t.Run("rejects token signed with different key", func(t *testing.T) {
		otherPub, otherPriv := generateTestKeyPair()
		otherConfig := &TokenConfig{
			Issuer:       "test-issuer",
			ExpiryHours:  24,
			SigningKey:   otherPriv,
			VerifyingKey: otherPub,
		}
		token, _ := GenerateToken("user-123", otherConfig)

		_, err := ValidateToken(token, config)
		if err != ErrInvalidToken {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})
}

func TestTokenClaims(t *testing.T) {
	pub, priv := generateTestKeyPair()
	config := &TokenConfig{
		Issuer:       "my-app",
		ExpiryHours:  48,
		SigningKey:   priv,
		VerifyingKey: pub,
	}

	token, _ := GenerateToken("user-456", config)
	claims, _ := ValidateToken(token, config)

	t.Run("has correct issuer", func(t *testing.T) {
		if claims.Issuer != "my-app" {
			t.Errorf("expected issuer 'my-app', got '%s'", claims.Issuer)
		}
	})

	t.Run("has issued at time", func(t *testing.T) {
		if claims.IssuedAt == nil {
			t.Error("expected IssuedAt to be set")
		}
	})

	t.Run("has expiration time", func(t *testing.T) {
		if claims.ExpiresAt == nil {
			t.Error("expected ExpiresAt to be set")
		}

		// Should expire in ~48 hours
		expectedExpiry := time.Now().Add(48 * time.Hour)
		if claims.ExpiresAt.Before(expectedExpiry.Add(-time.Minute)) {
			t.Error("expiration time is too early")
		}
		if claims.ExpiresAt.After(expectedExpiry.Add(time.Minute)) {
			t.Error("expiration time is too late")
		}
	})
}
