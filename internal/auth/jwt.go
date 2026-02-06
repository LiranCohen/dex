// Package auth provides authentication for Poindexter
package auth

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Claims represents the JWT claims for Poindexter authentication
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// TokenConfig holds JWT configuration
type TokenConfig struct {
	Issuer       string
	ExpiryHours  int
	RefreshHours int
	SigningKey   ed25519.PrivateKey
	VerifyingKey ed25519.PublicKey
}

// GenerateToken creates a new JWT for the given user ID
func GenerateToken(userID string, config *TokenConfig) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    config.Issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(config.ExpiryHours) * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(config.SigningKey)
}

// ValidateToken verifies a JWT and returns the claims if valid
func ValidateToken(tokenString string, config *TokenConfig) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, ErrInvalidToken
		}
		return config.VerifyingKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
