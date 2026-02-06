package auth

import "context"

type contextKey string

const sessionContextKey contextKey = "session"

// WithSession adds a session to the context.
func WithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

// SessionFromContext retrieves the session from the context.
// Returns nil if no session is present.
func SessionFromContext(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionContextKey).(*Session)
	return session
}
