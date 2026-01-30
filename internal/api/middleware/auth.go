// Package middleware provides HTTP middleware for the API
package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/auth"
)

// ContextKey type for context values
type ContextKey string

const (
	// UserIDKey is the context key for the authenticated user ID
	UserIDKey ContextKey = "user_id"
)

// JWTAuth creates middleware that validates JWT tokens
func JWTAuth(tokenConfig *auth.TokenConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get Authorization header
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}

			// Check for Bearer prefix
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header format")
			}

			tokenString := parts[1]

			// Validate token
			claims, err := auth.ValidateToken(tokenString, tokenConfig)
			if err != nil {
				if err == auth.ErrExpiredToken {
					return echo.NewHTTPError(http.StatusUnauthorized, "token expired")
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			// Store user ID in context
			c.Set(string(UserIDKey), claims.UserID)

			return next(c)
		}
	}
}

// GetUserID retrieves the authenticated user ID from context
func GetUserID(c echo.Context) string {
	if userID, ok := c.Get(string(UserIDKey)).(string); ok {
		return userID
	}
	return ""
}
