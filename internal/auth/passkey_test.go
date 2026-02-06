package auth

import (
	"sync"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestNewWebAuthn(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &PasskeyConfig{
			RPDisplayName: "Test HQ",
			RPID:          "example.com",
			RPOrigin:      "https://example.com",
		}

		wa, err := NewWebAuthn(cfg)
		if err != nil {
			t.Fatalf("failed to create WebAuthn: %v", err)
		}

		if wa == nil {
			t.Error("WebAuthn instance should not be nil")
		}
	})

	t.Run("missing RPID returns error", func(t *testing.T) {
		cfg := &PasskeyConfig{
			RPDisplayName: "Test HQ",
			RPID:          "",
			RPOrigin:      "https://example.com",
		}

		_, err := NewWebAuthn(cfg)
		if err == nil {
			t.Error("expected error for missing RPID")
		}
	})
}

func TestWebAuthnUser(t *testing.T) {
	cred := webauthn.Credential{
		ID:        []byte("test-credential-id"),
		PublicKey: []byte("test-public-key"),
	}

	user := NewWebAuthnUser("user-123", "alice@example.com", []webauthn.Credential{cred})

	t.Run("WebAuthnID", func(t *testing.T) {
		id := user.WebAuthnID()
		if string(id) != "user-123" {
			t.Errorf("unexpected ID: %s", string(id))
		}
	})

	t.Run("WebAuthnName", func(t *testing.T) {
		name := user.WebAuthnName()
		if name != "alice@example.com" {
			t.Errorf("unexpected name: %s", name)
		}
	})

	t.Run("WebAuthnDisplayName", func(t *testing.T) {
		displayName := user.WebAuthnDisplayName()
		if displayName != "alice@example.com" {
			t.Errorf("unexpected display name: %s", displayName)
		}
	})

	t.Run("WebAuthnIcon", func(t *testing.T) {
		icon := user.WebAuthnIcon()
		if icon != "" {
			t.Errorf("expected empty icon, got: %s", icon)
		}
	})

	t.Run("WebAuthnCredentials", func(t *testing.T) {
		creds := user.WebAuthnCredentials()
		if len(creds) != 1 {
			t.Fatalf("expected 1 credential, got %d", len(creds))
		}
		if string(creds[0].ID) != "test-credential-id" {
			t.Errorf("unexpected credential ID: %s", string(creds[0].ID))
		}
	})

	t.Run("AddCredential", func(t *testing.T) {
		newCred := webauthn.Credential{
			ID:        []byte("new-credential-id"),
			PublicKey: []byte("new-public-key"),
		}
		user.AddCredential(newCred)

		creds := user.WebAuthnCredentials()
		if len(creds) != 2 {
			t.Fatalf("expected 2 credentials, got %d", len(creds))
		}
	})
}

func TestPasskeyVerifier(t *testing.T) {
	cfg := &PasskeyConfig{
		RPDisplayName: "Test HQ",
		RPID:          "example.com",
		RPOrigin:      "https://example.com",
	}

	t.Run("create verifier with credentials", func(t *testing.T) {
		cred := webauthn.Credential{
			ID:        []byte("test-cred"),
			PublicKey: []byte("test-key"),
		}
		user := NewWebAuthnUser("user-123", "alice@example.com", []webauthn.Credential{cred})

		verifier, err := NewPasskeyVerifier(cfg, user)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		if !verifier.HasCredential() {
			t.Error("verifier should have credentials")
		}
	})

	t.Run("create verifier without credentials", func(t *testing.T) {
		user := NewWebAuthnUser("user-123", "alice@example.com", nil)

		verifier, err := NewPasskeyVerifier(cfg, user)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		if verifier.HasCredential() {
			t.Error("verifier should not have credentials")
		}
	})

	t.Run("create verifier with nil user", func(t *testing.T) {
		verifier, err := NewPasskeyVerifier(cfg, nil)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		if verifier.HasCredential() {
			t.Error("verifier with nil user should not have credentials")
		}
	})

	t.Run("begin authentication without credentials", func(t *testing.T) {
		user := NewWebAuthnUser("user-123", "alice@example.com", nil)

		verifier, err := NewPasskeyVerifier(cfg, user)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		_, _, err = verifier.BeginAuthentication()
		if err == nil {
			t.Error("expected error when no credentials configured")
		}
	})

	t.Run("begin authentication with credentials", func(t *testing.T) {
		// Create a proper credential with minimum required fields
		cred := webauthn.Credential{
			ID:              []byte("test-credential-id"),
			PublicKey:       []byte("test-public-key"),
			AttestationType: "none",
		}
		user := NewWebAuthnUser("user-123", "alice@example.com", []webauthn.Credential{cred})

		verifier, err := NewPasskeyVerifier(cfg, user)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		options, sessionID, err := verifier.BeginAuthentication()
		if err != nil {
			t.Fatalf("failed to begin authentication: %v", err)
		}

		if options == nil {
			t.Error("options should not be nil")
		}
		if sessionID == "" {
			t.Error("session ID should not be empty")
		}

		// Verify session was stored (with proper locking)
		verifier.mu.RLock()
		_, ok := verifier.sessions[sessionID]
		verifier.mu.RUnlock()
		if !ok {
			t.Error("session should be stored in verifier")
		}
	})

	t.Run("finish authentication with invalid session", func(t *testing.T) {
		cred := webauthn.Credential{
			ID:        []byte("test-cred"),
			PublicKey: []byte("test-key"),
		}
		user := NewWebAuthnUser("user-123", "alice@example.com", []webauthn.Credential{cred})

		verifier, err := NewPasskeyVerifier(cfg, user)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		_, err = verifier.FinishAuthentication("invalid-session", nil)
		if err == nil {
			t.Error("expected error for invalid session")
		}
	})
}

func TestPasskeyVerifierCleanup(t *testing.T) {
	cfg := &PasskeyConfig{
		RPDisplayName: "Test HQ",
		RPID:          "example.com",
		RPOrigin:      "https://example.com",
	}

	cred := webauthn.Credential{
		ID:              []byte("test-credential-id"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
	}
	user := NewWebAuthnUser("user-123", "alice@example.com", []webauthn.Credential{cred})

	verifier, err := NewPasskeyVerifier(cfg, user)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	// Begin authentication to create a session
	_, sessionID, err := verifier.BeginAuthentication()
	if err != nil {
		t.Fatalf("failed to begin authentication: %v", err)
	}

	// Manually expire the session for testing
	verifier.mu.Lock()
	if session, ok := verifier.sessions[sessionID]; ok {
		session.expiresAt = time.Now().Add(-time.Hour)
	}
	verifier.mu.Unlock()

	// Cleanup should remove expired session
	verifier.Cleanup()

	verifier.mu.RLock()
	_, ok := verifier.sessions[sessionID]
	verifier.mu.RUnlock()

	if ok {
		t.Error("expired session should be cleaned up")
	}
}

func TestPasskeyVerifierConcurrency(t *testing.T) {
	cfg := &PasskeyConfig{
		RPDisplayName: "Test HQ",
		RPID:          "example.com",
		RPOrigin:      "https://example.com",
	}

	cred := webauthn.Credential{
		ID:              []byte("test-credential-id"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
	}
	user := NewWebAuthnUser("user-123", "alice@example.com", []webauthn.Credential{cred})

	verifier, err := NewPasskeyVerifier(cfg, user)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	// Run concurrent BeginAuthentication calls
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = verifier.BeginAuthentication()
		}()
	}

	// Run concurrent Cleanup calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			verifier.Cleanup()
		}()
	}

	wg.Wait()

	// If we get here without data race panic, the test passes
}

func TestGenerateRandomString(t *testing.T) {
	t.Run("generates correct length", func(t *testing.T) {
		for _, length := range []int{8, 16, 32} {
			s := generateRandomString(length)
			if len(s) != length {
				t.Errorf("expected length %d, got %d", length, len(s))
			}
		}
	})

	t.Run("generates unique strings", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			s := generateRandomString(16)
			if seen[s] {
				t.Errorf("duplicate string generated: %s", s)
			}
			seen[s] = true
		}
	})
}
