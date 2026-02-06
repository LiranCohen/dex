package oidc

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func setupTestProvider(t *testing.T) *Provider {
	t.Helper()

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	config := DefaultConfig("https://hq.test.enbox.id")
	return NewProvider(config, kp)
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("https://hq.test.enbox.id")

	if config.Issuer != "https://hq.test.enbox.id" {
		t.Errorf("unexpected issuer: %s", config.Issuer)
	}
	if config.AuthCodeLifetime != 5*time.Minute {
		t.Errorf("unexpected auth code lifetime: %v", config.AuthCodeLifetime)
	}
	if config.AccessTokenLifetime != 1*time.Hour {
		t.Errorf("unexpected access token lifetime: %v", config.AccessTokenLifetime)
	}
}

func TestClientRegistration(t *testing.T) {
	provider := setupTestProvider(t)

	t.Run("register valid client", func(t *testing.T) {
		client := &Client{
			ID:           "test-client",
			Secret:       "test-secret",
			RedirectURIs: []string{"https://app.test/callback"},
			Name:         "Test App",
		}

		err := provider.RegisterClient(client)
		if err != nil {
			t.Fatalf("failed to register client: %v", err)
		}

		got, err := provider.GetClient("test-client")
		if err != nil {
			t.Fatalf("failed to get client: %v", err)
		}

		if got.ID != client.ID {
			t.Errorf("client ID mismatch: %s != %s", got.ID, client.ID)
		}
	})

	t.Run("register client without ID", func(t *testing.T) {
		client := &Client{
			Secret:       "test-secret",
			RedirectURIs: []string{"https://app.test/callback"},
		}

		err := provider.RegisterClient(client)
		if err == nil {
			t.Error("expected error for client without ID")
		}
	})

	t.Run("register client without secret", func(t *testing.T) {
		client := &Client{
			ID:           "no-secret",
			RedirectURIs: []string{"https://app.test/callback"},
		}

		err := provider.RegisterClient(client)
		if err == nil {
			t.Error("expected error for client without secret")
		}
	})

	t.Run("register client without redirect URIs", func(t *testing.T) {
		client := &Client{
			ID:     "no-redirect",
			Secret: "test-secret",
		}

		err := provider.RegisterClient(client)
		if err == nil {
			t.Error("expected error for client without redirect URIs")
		}
	})

	t.Run("get nonexistent client", func(t *testing.T) {
		_, err := provider.GetClient("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent client")
		}
	})
}

func TestValidateClient(t *testing.T) {
	provider := setupTestProvider(t)

	client := &Client{
		ID:           "auth-client",
		Secret:       "correct-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	t.Run("valid credentials", func(t *testing.T) {
		got, err := provider.ValidateClient("auth-client", "correct-secret")
		if err != nil {
			t.Fatalf("validation failed: %v", err)
		}
		if got.ID != "auth-client" {
			t.Errorf("unexpected client ID: %s", got.ID)
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		_, err := provider.ValidateClient("auth-client", "wrong-secret")
		if err == nil {
			t.Error("expected error for wrong secret")
		}
	})

	t.Run("unknown client", func(t *testing.T) {
		_, err := provider.ValidateClient("unknown", "secret")
		if err == nil {
			t.Error("expected error for unknown client")
		}
	})
}

func TestClientValidateRedirectURI(t *testing.T) {
	client := &Client{
		ID:           "test",
		Secret:       "secret",
		RedirectURIs: []string{"https://app.test/callback", "https://app.test/oauth"},
	}

	t.Run("valid redirect URI", func(t *testing.T) {
		if !client.ValidateRedirectURI("https://app.test/callback") {
			t.Error("should accept registered redirect URI")
		}
	})

	t.Run("invalid redirect URI", func(t *testing.T) {
		if client.ValidateRedirectURI("https://evil.test/callback") {
			t.Error("should reject unregistered redirect URI")
		}
	})
}

func TestAuthorizationCodeFlow(t *testing.T) {
	provider := setupTestProvider(t)

	client := &Client{
		ID:           "flow-client",
		Secret:       "flow-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	t.Run("create and exchange auth code", func(t *testing.T) {
		authCode, err := provider.CreateAuthorizationCode(
			"flow-client",
			"user-123",
			"alice@example.com",
			"Alice",
			"https://app.test/callback",
			[]string{"openid", "profile", "email"},
			"test-nonce",
		)
		if err != nil {
			t.Fatalf("failed to create auth code: %v", err)
		}

		if authCode.Code == "" {
			t.Error("auth code should not be empty")
		}

		// Exchange code for tokens
		tokenResp, err := provider.ExchangeAuthorizationCode(
			authCode.Code,
			"flow-client",
			"https://app.test/callback",
		)
		if err != nil {
			t.Fatalf("failed to exchange auth code: %v", err)
		}

		if tokenResp.AccessToken == "" {
			t.Error("access token should not be empty")
		}
		if tokenResp.IDToken == "" {
			t.Error("ID token should not be empty")
		}
		if tokenResp.TokenType != "Bearer" {
			t.Errorf("unexpected token type: %s", tokenResp.TokenType)
		}
		if tokenResp.ExpiresIn <= 0 {
			t.Errorf("expires_in should be positive: %d", tokenResp.ExpiresIn)
		}
	})

	t.Run("auth code is single-use", func(t *testing.T) {
		authCode, _ := provider.CreateAuthorizationCode(
			"flow-client",
			"user-123",
			"alice@example.com",
			"Alice",
			"https://app.test/callback",
			[]string{"openid"},
			"",
		)

		// First exchange succeeds
		_, err := provider.ExchangeAuthorizationCode(
			authCode.Code,
			"flow-client",
			"https://app.test/callback",
		)
		if err != nil {
			t.Fatalf("first exchange failed: %v", err)
		}

		// Second exchange fails
		_, err = provider.ExchangeAuthorizationCode(
			authCode.Code,
			"flow-client",
			"https://app.test/callback",
		)
		if err == nil {
			t.Error("second exchange should fail (single-use)")
		}
	})

	t.Run("invalid code", func(t *testing.T) {
		_, err := provider.ExchangeAuthorizationCode(
			"invalid-code",
			"flow-client",
			"https://app.test/callback",
		)
		if err == nil {
			t.Error("should reject invalid code")
		}
	})

	t.Run("wrong client ID", func(t *testing.T) {
		authCode, _ := provider.CreateAuthorizationCode(
			"flow-client",
			"user-123",
			"alice@example.com",
			"Alice",
			"https://app.test/callback",
			[]string{"openid"},
			"",
		)

		_, err := provider.ExchangeAuthorizationCode(
			authCode.Code,
			"wrong-client",
			"https://app.test/callback",
		)
		if err == nil {
			t.Error("should reject wrong client ID")
		}
	})

	t.Run("wrong redirect URI", func(t *testing.T) {
		authCode, _ := provider.CreateAuthorizationCode(
			"flow-client",
			"user-123",
			"alice@example.com",
			"Alice",
			"https://app.test/callback",
			[]string{"openid"},
			"",
		)

		_, err := provider.ExchangeAuthorizationCode(
			authCode.Code,
			"flow-client",
			"https://wrong.test/callback",
		)
		if err == nil {
			t.Error("should reject wrong redirect URI")
		}
	})

	t.Run("expired code", func(t *testing.T) {
		// Create provider with very short auth code lifetime
		kp, _ := GenerateKeyPair()
		shortConfig := Config{
			Issuer:            "https://hq.test.enbox.id",
			AuthCodeLifetime:  1 * time.Millisecond,
			AccessTokenLifetime: 1 * time.Hour,
		}
		shortProvider := NewProvider(shortConfig, kp)
		_ = shortProvider.RegisterClient(client)

		authCode, _ := shortProvider.CreateAuthorizationCode(
			"flow-client",
			"user-123",
			"alice@example.com",
			"Alice",
			"https://app.test/callback",
			[]string{"openid"},
			"",
		)

		// Wait for expiration
		time.Sleep(5 * time.Millisecond)

		_, err := shortProvider.ExchangeAuthorizationCode(
			authCode.Code,
			"flow-client",
			"https://app.test/callback",
		)
		if err == nil {
			t.Error("should reject expired code")
		}
	})
}

func TestAccessTokenValidation(t *testing.T) {
	provider := setupTestProvider(t)

	client := &Client{
		ID:           "token-client",
		Secret:       "token-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	authCode, _ := provider.CreateAuthorizationCode(
		"token-client",
		"user-456",
		"bob@example.com",
		"Bob",
		"https://app.test/callback",
		[]string{"openid", "profile"},
		"",
	)
	tokenResp, _ := provider.ExchangeAuthorizationCode(
		authCode.Code,
		"token-client",
		"https://app.test/callback",
	)

	t.Run("validate valid token", func(t *testing.T) {
		accessToken, err := provider.ValidateAccessToken(tokenResp.AccessToken)
		if err != nil {
			t.Fatalf("validation failed: %v", err)
		}

		if accessToken.UserID != "user-456" {
			t.Errorf("unexpected user ID: %s", accessToken.UserID)
		}
		if accessToken.Email != "bob@example.com" {
			t.Errorf("unexpected email: %s", accessToken.Email)
		}
	})

	t.Run("validate invalid token", func(t *testing.T) {
		_, err := provider.ValidateAccessToken("invalid-token")
		if err == nil {
			t.Error("should reject invalid token")
		}
	})
}

func TestUserInfo(t *testing.T) {
	provider := setupTestProvider(t)

	client := &Client{
		ID:           "userinfo-client",
		Secret:       "userinfo-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	authCode, _ := provider.CreateAuthorizationCode(
		"userinfo-client",
		"user-789",
		"charlie@example.com",
		"Charlie",
		"https://app.test/callback",
		[]string{"openid", "email"},
		"",
	)
	tokenResp, _ := provider.ExchangeAuthorizationCode(
		authCode.Code,
		"userinfo-client",
		"https://app.test/callback",
	)

	t.Run("get user info", func(t *testing.T) {
		userInfo, err := provider.GetUserInfo(tokenResp.AccessToken)
		if err != nil {
			t.Fatalf("failed to get user info: %v", err)
		}

		if userInfo.Sub != "user-789" {
			t.Errorf("unexpected sub: %s", userInfo.Sub)
		}
		if userInfo.Email != "charlie@example.com" {
			t.Errorf("unexpected email: %s", userInfo.Email)
		}
		if !userInfo.EmailVerified {
			t.Error("email should be verified")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := provider.GetUserInfo("invalid-token")
		if err == nil {
			t.Error("should reject invalid token")
		}
	})
}

func TestIDToken(t *testing.T) {
	provider := setupTestProvider(t)

	client := &Client{
		ID:           "idtoken-client",
		Secret:       "idtoken-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	authCode, _ := provider.CreateAuthorizationCode(
		"idtoken-client",
		"user-999",
		"dave@example.com",
		"Dave",
		"https://app.test/callback",
		[]string{"openid", "profile", "email"},
		"test-nonce-123",
	)
	tokenResp, _ := provider.ExchangeAuthorizationCode(
		authCode.Code,
		"idtoken-client",
		"https://app.test/callback",
	)

	t.Run("parse and verify ID token", func(t *testing.T) {
		// Parse without verification first to check claims
		token, _, err := jwt.NewParser().ParseUnverified(tokenResp.IDToken, &IDTokenClaims{})
		if err != nil {
			t.Fatalf("failed to parse ID token: %v", err)
		}

		claims := token.Claims.(*IDTokenClaims)

		if claims.Issuer != "https://hq.test.enbox.id" {
			t.Errorf("unexpected issuer: %s", claims.Issuer)
		}
		if claims.Subject != "user-999" {
			t.Errorf("unexpected subject: %s", claims.Subject)
		}
		if claims.Email != "dave@example.com" {
			t.Errorf("unexpected email: %s", claims.Email)
		}
		if claims.Name != "Dave" {
			t.Errorf("unexpected name: %s", claims.Name)
		}
		if claims.Nonce != "test-nonce-123" {
			t.Errorf("unexpected nonce: %s", claims.Nonce)
		}
		if !claims.EmailVerified {
			t.Error("email should be verified")
		}

		// Verify audience includes client ID
		aud := claims.Audience
		found := false
		for _, a := range aud {
			if a == "idtoken-client" {
				found = true
				break
			}
		}
		if !found {
			t.Error("audience should include client ID")
		}

		// Check kid header
		if token.Header["kid"] == "" {
			t.Error("ID token should have kid header")
		}
	})
}

func TestDiscoveryDocument(t *testing.T) {
	provider := setupTestProvider(t)

	doc := provider.DiscoveryDocument()

	if doc.Issuer != "https://hq.test.enbox.id" {
		t.Errorf("unexpected issuer: %s", doc.Issuer)
	}
	if doc.AuthorizationEndpoint != "https://hq.test.enbox.id/oauth/authorize" {
		t.Errorf("unexpected authorization endpoint: %s", doc.AuthorizationEndpoint)
	}
	if doc.TokenEndpoint != "https://hq.test.enbox.id/oauth/token" {
		t.Errorf("unexpected token endpoint: %s", doc.TokenEndpoint)
	}
	if doc.UserInfoEndpoint != "https://hq.test.enbox.id/oauth/userinfo" {
		t.Errorf("unexpected userinfo endpoint: %s", doc.UserInfoEndpoint)
	}
	if doc.JwksURI != "https://hq.test.enbox.id/oauth/jwks" {
		t.Errorf("unexpected JWKS URI: %s", doc.JwksURI)
	}

	// Check supported values
	if len(doc.ResponseTypesSupported) == 0 || doc.ResponseTypesSupported[0] != "code" {
		t.Error("should support 'code' response type")
	}
	if len(doc.IDTokenSigningAlgValuesSupported) == 0 || doc.IDTokenSigningAlgValuesSupported[0] != "RS256" {
		t.Error("should support RS256 signing")
	}
}

func TestCleanup(t *testing.T) {
	kp, _ := GenerateKeyPair()
	config := Config{
		Issuer:            "https://hq.test.enbox.id",
		AuthCodeLifetime:  1 * time.Millisecond,
		AccessTokenLifetime: 1 * time.Millisecond,
	}
	provider := NewProvider(config, kp)

	client := &Client{
		ID:           "cleanup-client",
		Secret:       "cleanup-secret",
		RedirectURIs: []string{"https://app.test/callback"},
	}
	_ = provider.RegisterClient(client)

	// Create some auth codes and tokens
	for i := 0; i < 5; i++ {
		authCode, _ := provider.CreateAuthorizationCode(
			"cleanup-client",
			"user",
			"test@example.com",
			"Test",
			"https://app.test/callback",
			[]string{"openid"},
			"",
		)
		_, _ = provider.ExchangeAuthorizationCode(
			authCode.Code,
			"cleanup-client",
			"https://app.test/callback",
		)
	}

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	// Cleanup should remove expired items
	provider.Cleanup()

	// Verify auth codes map is empty (codes were either exchanged or expired)
	provider.mu.RLock()
	authCodesLen := len(provider.authCodes)
	accessTokensLen := len(provider.accessTokens)
	provider.mu.RUnlock()

	if authCodesLen != 0 {
		t.Errorf("expected 0 auth codes after cleanup, got %d", authCodesLen)
	}
	if accessTokensLen != 0 {
		t.Errorf("expected 0 access tokens after cleanup, got %d", accessTokensLen)
	}
}
