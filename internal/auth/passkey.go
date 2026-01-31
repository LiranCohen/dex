package auth

import (
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
