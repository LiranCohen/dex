package realtime

import (
	"context"
	"net/http"
	"strings"

	"github.com/centrifugal/centrifuge"
	"github.com/lirancohen/dex/internal/auth"
)

// TokenValidator validates authentication tokens and returns user info
type TokenValidator interface {
	ValidateToken(ctx context.Context, token string) (*UserInfo, error)
}

// UserInfo contains user information extracted from a valid token
type UserInfo struct {
	ID       string
	Username string
}

// JWTValidator implements TokenValidator using the auth package
type JWTValidator struct {
	config *auth.TokenConfig
}

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(config *auth.TokenConfig) *JWTValidator {
	return &JWTValidator{config: config}
}

// ValidateToken validates a JWT and returns user info
func (v *JWTValidator) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
	claims, err := auth.ValidateToken(token, v.config)
	if err != nil {
		return nil, err
	}
	return &UserInfo{
		ID:       claims.UserID,
		Username: claims.UserID, // Use UserID as username for now
	}, nil
}

// AuthMiddleware extracts auth from request and sets Centrifuge credentials
func AuthMiddleware(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header first, then query param
			token := ""
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// Fall back to query param (for WebSocket connections)
			if token == "" {
				token = r.URL.Query().Get("token")
			}

			if token == "" {
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			// Validate token and get user info
			user, err := validator.ValidateToken(r.Context(), token)
			if err != nil {
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			// Set Centrifuge credentials in context
			cred := &centrifuge.Credentials{
				UserID: user.ID,
			}
			ctx := centrifuge.SetCredentials(r.Context(), cred)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NoAuthMiddleware allows all connections (for development without auth)
func NoAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set a default user ID for unauthenticated connections
			cred := &centrifuge.Credentials{
				UserID: "anonymous",
			}
			ctx := centrifuge.SetCredentials(r.Context(), cred)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
