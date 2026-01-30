// Package auth provides authentication for Poindexter
package auth

import (
	"crypto/ed25519"
	"crypto/sha512"

	"golang.org/x/crypto/hkdf"
)

// Keypair represents an Ed25519 public/private key pair
type Keypair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// DeriveKeypair derives an Ed25519 keypair from a BIP39 mnemonic
// Uses HKDF to derive key material from the mnemonic seed
func DeriveKeypair(mnemonic string) (*Keypair, error) {
	// Convert mnemonic to seed (empty passphrase for Poindexter auth)
	seed := MnemonicToSeed(mnemonic, "")

	// Use HKDF to derive 32 bytes for Ed25519 seed
	hkdfReader := hkdf.New(sha512.New, seed, []byte("poindexter-auth"), []byte("ed25519-keypair"))
	ed25519Seed := make([]byte, ed25519.SeedSize)
	if _, err := hkdfReader.Read(ed25519Seed); err != nil {
		return nil, err
	}

	// Generate Ed25519 keypair from seed
	privateKey := ed25519.NewKeyFromSeed(ed25519Seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	return &Keypair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}, nil
}

// Sign creates a signature for the given message using the private key
func Sign(message []byte, privateKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privateKey, message)
}

// Verify checks if a signature is valid for the given message and public key
func Verify(message []byte, signature []byte, publicKey ed25519.PublicKey) bool {
	return ed25519.Verify(publicKey, message, signature)
}
