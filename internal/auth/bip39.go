// Package auth provides authentication for Poindexter
package auth

import (
	"github.com/tyler-smith/go-bip39"
)

// GeneratePassphrase generates a 24-word BIP39 mnemonic for primary authentication
func GeneratePassphrase() (string, error) {
	entropy, err := bip39.NewEntropy(256) // 256 bits = 24 words
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

// GenerateRecoveryPhrase generates a 12-word BIP39 mnemonic for recovery
func GenerateRecoveryPhrase() (string, error) {
	entropy, err := bip39.NewEntropy(128) // 128 bits = 12 words
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

// ValidateMnemonic checks if a mnemonic phrase is valid BIP39
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// MnemonicToSeed converts a mnemonic to a seed with optional passphrase
func MnemonicToSeed(mnemonic string, passphrase string) []byte {
	return bip39.NewSeed(mnemonic, passphrase)
}
