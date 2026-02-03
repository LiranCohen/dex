// Package auth provides HTTP handlers for authentication operations.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/auth"
)

// challengeEntry holds a challenge and its expiry time.
type challengeEntry struct {
	Challenge string
	ExpiresAt time.Time
}

// Handler handles authentication-related HTTP requests.
type Handler struct {
	deps         *core.Deps
	challenges   map[string]challengeEntry
	challengesMu sync.RWMutex
}

// New creates a new auth handler.
func New(deps *core.Deps) *Handler {
	return &Handler{
		deps:       deps,
		challenges: make(map[string]challengeEntry),
	}
}

// RegisterRoutes registers all auth routes on the given group.
// These are all public routes (no auth required).
//   - POST /auth/challenge
//   - POST /auth/verify
//   - POST /auth/refresh
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.POST("/auth/challenge", h.HandleChallenge)
	g.POST("/auth/verify", h.HandleVerify)
	g.POST("/auth/refresh", h.HandleRefresh)
}

// HandleChallenge generates and returns a random challenge for authentication.
// POST /api/v1/auth/challenge
func (h *Handler) HandleChallenge(c echo.Context) error {
	// Generate 32 random bytes
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate challenge")
	}

	challenge := hex.EncodeToString(challengeBytes)
	expiresAt := time.Now().Add(5 * time.Minute)

	// Store challenge with TTL
	h.challengesMu.Lock()
	h.challenges[challenge] = challengeEntry{
		Challenge: challenge,
		ExpiresAt: expiresAt,
	}
	h.challengesMu.Unlock()

	return c.JSON(http.StatusOK, map[string]any{
		"challenge":  challenge,
		"expires_in": 300,
	})
}

// HandleVerify verifies a signed challenge and returns a JWT.
// POST /api/v1/auth/verify
func (h *Handler) HandleVerify(c echo.Context) error {
	var req struct {
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
		Challenge string `json:"challenge"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.PublicKey == "" || req.Signature == "" || req.Challenge == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "public_key, signature, and challenge are required")
	}

	// Validate challenge exists and not expired
	h.challengesMu.Lock()
	entry, exists := h.challenges[req.Challenge]
	if exists {
		delete(h.challenges, req.Challenge) // One-time use
	}
	h.challengesMu.Unlock()

	if !exists {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired challenge")
	}
	if time.Now().After(entry.ExpiresAt) {
		return echo.NewHTTPError(http.StatusUnauthorized, "challenge expired")
	}

	// Decode public key and signature from hex
	publicKey, err := hex.DecodeString(req.PublicKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid public_key format")
	}
	signature, err := hex.DecodeString(req.Signature)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid signature format")
	}
	challengeBytes, err := hex.DecodeString(req.Challenge)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid challenge format")
	}

	// Verify signature
	if !auth.Verify(challengeBytes, signature, publicKey) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}

	// Get or create user
	user, _, err := h.deps.DB.GetOrCreateUser(req.PublicKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get or create user")
	}

	// Update last login
	_ = h.deps.DB.UpdateUserLastLogin(user.ID)

	// Generate JWT
	if h.deps.TokenConfig == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "token configuration not available")
	}
	token, err := auth.GenerateToken(user.ID, h.deps.TokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"user_id": user.ID,
	})
}

// HandleRefresh refreshes an existing JWT token.
// POST /api/v1/auth/refresh
func (h *Handler) HandleRefresh(c echo.Context) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}

	if h.deps.TokenConfig == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "token configuration not available")
	}

	newToken, err := auth.RefreshToken(req.Token, h.deps.TokenConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "failed to refresh token")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"token": newToken,
	})
}
