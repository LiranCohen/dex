package oidc

import (
	"time"
)

// Client represents an OIDC client (e.g., Forgejo).
type Client struct {
	ID           string   `json:"client_id"`
	Secret       string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	Name         string   `json:"name"`
}

// ValidateRedirectURI checks if a redirect URI is registered for this client.
func (c *Client) ValidateRedirectURI(uri string) bool {
	for _, registered := range c.RedirectURIs {
		if registered == uri {
			return true
		}
	}
	return false
}

// AuthorizationCode represents an issued authorization code.
type AuthorizationCode struct {
	Code        string    `json:"code"`
	ClientID    string    `json:"client_id"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	RedirectURI string    `json:"redirect_uri"`
	Scopes      []string  `json:"scopes"`
	Nonce       string    `json:"nonce,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// IsExpired returns true if the authorization code has expired.
func (ac *AuthorizationCode) IsExpired() bool {
	return time.Now().After(ac.ExpiresAt)
}

// AccessToken represents an issued access token.
type AccessToken struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	ClientID  string    `json:"client_id"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired returns true if the access token has expired.
func (at *AccessToken) IsExpired() bool {
	return time.Now().After(at.ExpiresAt)
}

// TokenResponse is the response for the token endpoint.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// ErrorResponse is the error response format for OIDC endpoints.
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// Standard OIDC/OAuth2 error codes
const (
	ErrInvalidRequest          = "invalid_request"
	ErrUnauthorizedClient      = "unauthorized_client"
	ErrAccessDenied            = "access_denied"
	ErrUnsupportedResponseType = "unsupported_response_type"
	ErrInvalidScope            = "invalid_scope"
	ErrServerError             = "server_error"
	ErrInvalidClient           = "invalid_client"
	ErrInvalidGrant            = "invalid_grant"
	ErrUnsupportedGrantType    = "unsupported_grant_type"
)

// UserInfo represents the userinfo endpoint response.
type UserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
}

// DiscoveryDocument represents the OIDC discovery document.
type DiscoveryDocument struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	UserInfoEndpoint                 string   `json:"userinfo_endpoint"`
	JwksURI                          string   `json:"jwks_uri"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}
