// Package auth provides authentication for Poindexter
package auth

import (
	"crypto/ed25519"
)

// Keypair represents an Ed25519 public/private key pair
type Keypair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// Sign creates a signature for the given message using the private key
func Sign(message []byte, privateKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privateKey, message)
}

// Verify checks if a signature is valid for the given message and public key
func Verify(message []byte, signature []byte, publicKey ed25519.PublicKey) bool {
	return ed25519.Verify(publicKey, message, signature)
}
