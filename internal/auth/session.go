// Package auth provides authentication and session management for HQ.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Session represents an authenticated user session.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionStore defines the interface for session storage.
type SessionStore interface {
	Get(id string) (*Session, error)
	Set(session *Session) error
	Delete(id string) error
}

// MemorySessionStore is an in-memory session store.
// Suitable for single-instance HQ deployments.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]*Session),
	}
}

// Get retrieves a session by ID.
func (s *MemorySessionStore) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

// Set stores a session.
func (s *MemorySessionStore) Set(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session
	return nil
}

// Delete removes a session.
func (s *MemorySessionStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)
	return nil
}

// SessionManager handles session creation and validation.
type SessionManager struct {
	store      SessionStore
	cookieName string
	maxAge     time.Duration
	secure     bool
}

// SessionManagerConfig contains configuration for the session manager.
type SessionManagerConfig struct {
	Store      SessionStore
	CookieName string
	MaxAge     time.Duration
	Secure     bool // Set to true for HTTPS
}

// NewSessionManager creates a new session manager.
func NewSessionManager(cfg SessionManagerConfig) *SessionManager {
	if cfg.Store == nil {
		cfg.Store = NewMemorySessionStore()
	}
	if cfg.CookieName == "" {
		cfg.CookieName = "hq_session"
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 7 * 24 * time.Hour // 7 days default
	}

	return &SessionManager{
		store:      cfg.Store,
		cookieName: cfg.CookieName,
		maxAge:     cfg.MaxAge,
		secure:     cfg.Secure,
	}
}

// CreateSession creates a new session for a user and returns it.
func (m *SessionManager) CreateSession(userID, email string) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	now := time.Now()
	session := &Session{
		ID:        id,
		UserID:    userID,
		Email:     email,
		CreatedAt: now,
		ExpiresAt: now.Add(m.maxAge),
	}

	if err := m.store.Set(session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	return session, nil
}

// ValidateSession checks if a session ID is valid and returns the session.
func (m *SessionManager) ValidateSession(sessionID string) (*Session, error) {
	return m.store.Get(sessionID)
}

// DeleteSession removes a session.
func (m *SessionManager) DeleteSession(sessionID string) error {
	return m.store.Delete(sessionID)
}

// SetSessionCookie sets the session cookie on the response.
func (m *SessionManager) SetSessionCookie(w http.ResponseWriter, session *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(m.maxAge.Seconds()),
	})
}

// ClearSessionCookie removes the session cookie.
func (m *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// GetSessionFromRequest retrieves and validates the session from a request.
func (m *SessionManager) GetSessionFromRequest(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(m.cookieName)
	if err != nil {
		return nil, fmt.Errorf("no session cookie: %w", err)
	}

	return m.ValidateSession(cookie.Value)
}

// Middleware returns an HTTP middleware that validates sessions.
// If the session is valid, it's added to the request context.
func (m *SessionManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.GetSessionFromRequest(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Add session to context
		ctx := WithSession(r.Context(), session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth is a middleware that requires authentication but allows the request through
// to handle the redirect to login.
func (m *SessionManager) RequireAuth(loginURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := m.GetSessionFromRequest(r)
			if err != nil {
				// Redirect to login with return URL
				redirectURL := loginURL + "?next=" + r.URL.RequestURI()
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}

			ctx := WithSession(r.Context(), session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
