package oidc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config holds configuration for the OIDC provider.
type Config struct {
	Issuer            string        // Base URL (e.g., https://hq.alice.enbox.id)
	AuthCodeLifetime  time.Duration // How long auth codes are valid (default: 5 min)
	AccessTokenLifetime time.Duration // How long access tokens are valid (default: 1 hour)
	IDTokenLifetime   time.Duration // How long ID tokens are valid (default: 1 hour)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(issuer string) Config {
	return Config{
		Issuer:            issuer,
		AuthCodeLifetime:  5 * time.Minute,
		AccessTokenLifetime: 1 * time.Hour,
		IDTokenLifetime:   1 * time.Hour,
	}
}

// Provider implements an OIDC provider for HQ.
type Provider struct {
	config   Config
	keyPair  *KeyPair

	mu           sync.RWMutex
	clients      map[string]*Client          // client_id -> Client
	authCodes    map[string]*AuthorizationCode // code -> AuthorizationCode
	accessTokens map[string]*AccessToken     // token -> AccessToken
}

// NewProvider creates a new OIDC provider.
func NewProvider(config Config, keyPair *KeyPair) *Provider {
	return &Provider{
		config:       config,
		keyPair:      keyPair,
		clients:      make(map[string]*Client),
		authCodes:    make(map[string]*AuthorizationCode),
		accessTokens: make(map[string]*AccessToken),
	}
}

// RegisterClient adds an OIDC client.
func (p *Provider) RegisterClient(client *Client) error {
	if client.ID == "" {
		return errors.New("client_id is required")
	}
	if client.Secret == "" {
		return errors.New("client_secret is required")
	}
	if len(client.RedirectURIs) == 0 {
		return errors.New("at least one redirect_uri is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.clients[client.ID] = client
	return nil
}

// GetClient retrieves a registered client by ID.
func (p *Provider) GetClient(clientID string) (*Client, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	client, ok := p.clients[clientID]
	if !ok {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	return client, nil
}

// ValidateClient validates client credentials.
func (p *Provider) ValidateClient(clientID, clientSecret string) (*Client, error) {
	client, err := p.GetClient(clientID)
	if err != nil {
		return nil, err
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(client.Secret), []byte(clientSecret)) != 1 {
		return nil, errors.New("invalid client credentials")
	}

	return client, nil
}

// CreateAuthorizationCode creates a new authorization code.
func (p *Provider) CreateAuthorizationCode(
	clientID string,
	userID string,
	email string,
	displayName string,
	redirectURI string,
	scopes []string,
	nonce string,
) (*AuthorizationCode, error) {
	code, err := generateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth code: %w", err)
	}

	authCode := &AuthorizationCode{
		Code:        code,
		ClientID:    clientID,
		UserID:      userID,
		Email:       email,
		DisplayName: displayName,
		RedirectURI: redirectURI,
		Scopes:      scopes,
		Nonce:       nonce,
		ExpiresAt:   time.Now().Add(p.config.AuthCodeLifetime),
	}

	p.mu.Lock()
	p.authCodes[code] = authCode
	p.mu.Unlock()

	return authCode, nil
}

// ExchangeAuthorizationCode exchanges an authorization code for tokens.
// The code is consumed (single-use).
func (p *Provider) ExchangeAuthorizationCode(code, clientID, redirectURI string) (*TokenResponse, error) {
	p.mu.Lock()
	authCode, ok := p.authCodes[code]
	if ok {
		delete(p.authCodes, code) // Single-use
	}
	p.mu.Unlock()

	if !ok {
		return nil, errors.New(ErrInvalidGrant)
	}

	if authCode.IsExpired() {
		return nil, errors.New(ErrInvalidGrant)
	}

	if authCode.ClientID != clientID {
		return nil, errors.New(ErrInvalidGrant)
	}

	if authCode.RedirectURI != redirectURI {
		return nil, errors.New(ErrInvalidGrant)
	}

	// Generate access token
	accessToken, err := p.createAccessToken(authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	// Generate ID token
	idToken, err := p.createIDToken(authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to create ID token: %w", err)
	}

	return &TokenResponse{
		AccessToken: accessToken.Token,
		TokenType:   "Bearer",
		ExpiresIn:   int(p.config.AccessTokenLifetime.Seconds()),
		IDToken:     idToken,
		Scope:       scopesToString(authCode.Scopes),
	}, nil
}

// createAccessToken generates an opaque access token and stores it.
func (p *Provider) createAccessToken(authCode *AuthorizationCode) (*AccessToken, error) {
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	accessToken := &AccessToken{
		Token:     token,
		UserID:    authCode.UserID,
		Email:     authCode.Email,
		ClientID:  authCode.ClientID,
		Scopes:    authCode.Scopes,
		ExpiresAt: time.Now().Add(p.config.AccessTokenLifetime),
	}

	p.mu.Lock()
	p.accessTokens[token] = accessToken
	p.mu.Unlock()

	return accessToken, nil
}

// IDTokenClaims represents the claims in an ID token.
type IDTokenClaims struct {
	jwt.RegisteredClaims
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Nonce             string `json:"nonce,omitempty"`
}

// createIDToken generates a signed JWT ID token.
func (p *Provider) createIDToken(authCode *AuthorizationCode) (string, error) {
	now := time.Now()

	claims := IDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    p.config.Issuer,
			Subject:   authCode.UserID,
			Audience:  jwt.ClaimStrings{authCode.ClientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(p.config.IDTokenLifetime)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email:             authCode.Email,
		EmailVerified:     true,
		Name:              authCode.DisplayName,
		PreferredUsername: authCode.Email,
		Nonce:             authCode.Nonce,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = p.keyPair.KeyID

	return token.SignedString(p.keyPair.PrivateKey)
}

// ValidateAccessToken validates an access token and returns its data.
func (p *Provider) ValidateAccessToken(token string) (*AccessToken, error) {
	p.mu.RLock()
	accessToken, ok := p.accessTokens[token]
	p.mu.RUnlock()

	if !ok {
		return nil, errors.New("invalid access token")
	}

	if accessToken.IsExpired() {
		return nil, errors.New("access token expired")
	}

	return accessToken, nil
}

// GetUserInfo returns user information for a valid access token.
func (p *Provider) GetUserInfo(token string) (*UserInfo, error) {
	accessToken, err := p.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	return &UserInfo{
		Sub:               accessToken.UserID,
		Email:             accessToken.Email,
		EmailVerified:     true,
		Name:              accessToken.Email, // Use email as name if no display name
		PreferredUsername: accessToken.Email,
	}, nil
}

// DiscoveryDocument returns the OIDC discovery document.
func (p *Provider) DiscoveryDocument() *DiscoveryDocument {
	return &DiscoveryDocument{
		Issuer:                           p.config.Issuer,
		AuthorizationEndpoint:            p.config.Issuer + "/oauth/authorize",
		TokenEndpoint:                    p.config.Issuer + "/oauth/token",
		UserInfoEndpoint:                 p.config.Issuer + "/oauth/userinfo",
		JwksURI:                          p.config.Issuer + "/oauth/jwks",
		ResponseTypesSupported:           []string{"code"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ScopesSupported:                  []string{"openid", "profile", "email"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post"},
		ClaimsSupported:                  []string{"sub", "email", "email_verified", "name", "preferred_username"},
	}
}

// JWKS returns the JSON Web Key Set.
func (p *Provider) JWKS() ([]byte, error) {
	return p.keyPair.JWKS()
}

// Cleanup removes expired codes and tokens.
// Should be called periodically.
func (p *Provider) Cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	for code, ac := range p.authCodes {
		if now.After(ac.ExpiresAt) {
			delete(p.authCodes, code)
		}
	}

	for token, at := range p.accessTokens {
		if now.After(at.ExpiresAt) {
			delete(p.accessTokens, token)
		}
	}
}

// WriteError writes an OIDC error response.
func WriteError(w http.ResponseWriter, statusCode int, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:            errorCode,
		ErrorDescription: description,
	})
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// Helper functions

func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func scopesToString(scopes []string) string {
	result := ""
	for i, s := range scopes {
		if i > 0 {
			result += " "
		}
		result += s
	}
	return result
}
