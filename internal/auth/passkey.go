package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// PasskeyConfig holds WebAuthn configuration
type PasskeyConfig struct {
	RPDisplayName string // Human-readable name shown to user
	RPID          string // Domain (set dynamically from request)
	RPOrigin      string // Full origin URL (set dynamically from request)
}

// NewWebAuthn creates a WebAuthn instance with the given config
func NewWebAuthn(cfg *PasskeyConfig) (*webauthn.WebAuthn, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: cfg.RPDisplayName,
		RPID:          cfg.RPID,
		RPOrigins:     []string{cfg.RPOrigin},
		// Require user verification (biometrics/PIN)
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform, // Use device's built-in authenticator
			ResidentKey:             protocol.ResidentKeyRequirementRequired,
			UserVerification:        protocol.VerificationRequired,
		},
	}

	return webauthn.New(wconfig)
}

// WebAuthnUser implements webauthn.User interface
type WebAuthnUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

// NewWebAuthnUser creates a new WebAuthn user
func NewWebAuthnUser(id string, name string, credentials []webauthn.Credential) *WebAuthnUser {
	return &WebAuthnUser{
		id:          []byte(id),
		name:        name,
		displayName: name,
		credentials: credentials,
	}
}

// WebAuthnID returns the user's ID
func (u *WebAuthnUser) WebAuthnID() []byte {
	return u.id
}

// WebAuthnName returns the user's name
func (u *WebAuthnUser) WebAuthnName() string {
	return u.name
}

// WebAuthnDisplayName returns the user's display name
func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.displayName
}

// WebAuthnCredentials returns the user's credentials
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

// WebAuthnIcon returns the user's icon URL (deprecated but required)
func (u *WebAuthnUser) WebAuthnIcon() string {
	return ""
}

// AddCredential adds a credential to the user
func (u *WebAuthnUser) AddCredential(cred webauthn.Credential) {
	u.credentials = append(u.credentials, cred)
}

// PasskeyVerifier handles WebAuthn authentication for the HQ owner.
type PasskeyVerifier struct {
	webauthn   *webauthn.WebAuthn
	user       *WebAuthnUser
	sessions   map[string]*webauthn.SessionData
}

// NewPasskeyVerifier creates a new passkey verifier for owner authentication.
func NewPasskeyVerifier(cfg *PasskeyConfig, user *WebAuthnUser) (*PasskeyVerifier, error) {
	w, err := NewWebAuthn(cfg)
	if err != nil {
		return nil, err
	}

	return &PasskeyVerifier{
		webauthn: w,
		user:     user,
		sessions: make(map[string]*webauthn.SessionData),
	}, nil
}

// HasCredential returns true if the user has passkey credentials.
func (v *PasskeyVerifier) HasCredential() bool {
	return v.user != nil && len(v.user.credentials) > 0
}

// BeginAuthentication starts the WebAuthn authentication flow.
// Returns options to send to the client and a session ID.
func (v *PasskeyVerifier) BeginAuthentication() (*protocol.CredentialAssertion, string, error) {
	if !v.HasCredential() {
		return nil, "", protocol.ErrBadRequest.WithDetails("no passkey credential configured")
	}

	options, session, err := v.webauthn.BeginLogin(v.user)
	if err != nil {
		return nil, "", err
	}

	// Generate session ID
	sessionID := generateRandomString(16)
	v.sessions[sessionID] = session

	return options, sessionID, nil
}

// FinishAuthentication completes the WebAuthn authentication flow.
// Returns the updated credential on success.
func (v *PasskeyVerifier) FinishAuthentication(sessionID string, response *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	session, ok := v.sessions[sessionID]
	if !ok {
		return nil, protocol.ErrBadRequest.WithDetails("invalid or expired session")
	}
	delete(v.sessions, sessionID)

	credential, err := v.webauthn.ValidateLogin(v.user, *session, response)
	if err != nil {
		return nil, err
	}

	return credential, nil
}

// ParseAssertionResponse parses a WebAuthn assertion response from the request body.
func ParseAssertionResponse(body []byte) (*protocol.ParsedCredentialAssertionData, error) {
	return protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
}

func generateRandomString(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:length]
}
