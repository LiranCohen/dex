package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMemorySessionStore(t *testing.T) {
	store := NewMemorySessionStore()

	t.Run("set and get session", func(t *testing.T) {
		session := &Session{
			ID:        "test-session-1",
			UserID:    "user-123",
			Email:     "test@example.com",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		}

		if err := store.Set(session); err != nil {
			t.Fatalf("failed to set session: %v", err)
		}

		got, err := store.Get("test-session-1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		if got.ID != session.ID {
			t.Errorf("session ID mismatch: got %s, want %s", got.ID, session.ID)
		}
		if got.UserID != session.UserID {
			t.Errorf("user ID mismatch: got %s, want %s", got.UserID, session.UserID)
		}
		if got.Email != session.Email {
			t.Errorf("email mismatch: got %s, want %s", got.Email, session.Email)
		}
	})

	t.Run("get nonexistent session", func(t *testing.T) {
		_, err := store.Get("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent session")
		}
	})

	t.Run("get expired session", func(t *testing.T) {
		session := &Session{
			ID:        "expired-session",
			UserID:    "user-456",
			Email:     "expired@example.com",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			ExpiresAt: time.Now().Add(-time.Hour), // Already expired
		}

		if err := store.Set(session); err != nil {
			t.Fatalf("failed to set session: %v", err)
		}

		_, err := store.Get("expired-session")
		if err == nil {
			t.Error("expected error for expired session")
		}
	})

	t.Run("delete session", func(t *testing.T) {
		session := &Session{
			ID:        "to-delete",
			UserID:    "user-789",
			Email:     "delete@example.com",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		}

		if err := store.Set(session); err != nil {
			t.Fatalf("failed to set session: %v", err)
		}

		if err := store.Delete("to-delete"); err != nil {
			t.Fatalf("failed to delete session: %v", err)
		}

		_, err := store.Get("to-delete")
		if err == nil {
			t.Error("expected error after deletion")
		}
	})
}

func TestSessionManager(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		mgr := NewSessionManager(SessionManagerConfig{})

		if mgr.cookieName != "hq_session" {
			t.Errorf("unexpected cookie name: %s", mgr.cookieName)
		}
		if mgr.maxAge != 7*24*time.Hour {
			t.Errorf("unexpected max age: %v", mgr.maxAge)
		}
		if mgr.store == nil {
			t.Error("store should not be nil")
		}
	})

	t.Run("custom configuration", func(t *testing.T) {
		store := NewMemorySessionStore()
		mgr := NewSessionManager(SessionManagerConfig{
			Store:      store,
			CookieName: "custom_session",
			MaxAge:     24 * time.Hour,
			Secure:     true,
		})

		if mgr.cookieName != "custom_session" {
			t.Errorf("unexpected cookie name: %s", mgr.cookieName)
		}
		if mgr.maxAge != 24*time.Hour {
			t.Errorf("unexpected max age: %v", mgr.maxAge)
		}
		if !mgr.secure {
			t.Error("expected secure to be true")
		}
	})

	t.Run("create session", func(t *testing.T) {
		mgr := NewSessionManager(SessionManagerConfig{})

		session, err := mgr.CreateSession("user-123", "test@example.com")
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		if session.ID == "" {
			t.Error("session ID should not be empty")
		}
		if session.UserID != "user-123" {
			t.Errorf("user ID mismatch: got %s, want user-123", session.UserID)
		}
		if session.Email != "test@example.com" {
			t.Errorf("email mismatch: got %s, want test@example.com", session.Email)
		}
		if session.ExpiresAt.Before(time.Now()) {
			t.Error("session should not be expired")
		}
	})

	t.Run("validate session", func(t *testing.T) {
		mgr := NewSessionManager(SessionManagerConfig{})

		created, err := mgr.CreateSession("user-123", "test@example.com")
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		validated, err := mgr.ValidateSession(created.ID)
		if err != nil {
			t.Fatalf("failed to validate session: %v", err)
		}

		if validated.ID != created.ID {
			t.Errorf("session ID mismatch: got %s, want %s", validated.ID, created.ID)
		}
	})

	t.Run("delete session", func(t *testing.T) {
		mgr := NewSessionManager(SessionManagerConfig{})

		session, err := mgr.CreateSession("user-123", "test@example.com")
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		if err := mgr.DeleteSession(session.ID); err != nil {
			t.Fatalf("failed to delete session: %v", err)
		}

		_, err = mgr.ValidateSession(session.ID)
		if err == nil {
			t.Error("expected error validating deleted session")
		}
	})
}

func TestSessionCookies(t *testing.T) {
	mgr := NewSessionManager(SessionManagerConfig{
		CookieName: "test_session",
		Secure:     true,
	})

	t.Run("set session cookie", func(t *testing.T) {
		session, _ := mgr.CreateSession("user-123", "test@example.com")

		recorder := httptest.NewRecorder()
		mgr.SetSessionCookie(recorder, session)

		cookies := recorder.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("expected 1 cookie, got %d", len(cookies))
		}

		cookie := cookies[0]
		if cookie.Name != "test_session" {
			t.Errorf("unexpected cookie name: %s", cookie.Name)
		}
		if cookie.Value != session.ID {
			t.Errorf("cookie value mismatch: got %s, want %s", cookie.Value, session.ID)
		}
		if !cookie.HttpOnly {
			t.Error("cookie should be HttpOnly")
		}
		if !cookie.Secure {
			t.Error("cookie should be Secure")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("unexpected SameSite: %v", cookie.SameSite)
		}
	})

	t.Run("clear session cookie", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mgr.ClearSessionCookie(recorder)

		cookies := recorder.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("expected 1 cookie, got %d", len(cookies))
		}

		cookie := cookies[0]
		if cookie.Value != "" {
			t.Errorf("cookie value should be empty, got %s", cookie.Value)
		}
		if cookie.MaxAge != -1 {
			t.Errorf("cookie MaxAge should be -1, got %d", cookie.MaxAge)
		}
	})

	t.Run("get session from request", func(t *testing.T) {
		session, _ := mgr.CreateSession("user-123", "test@example.com")

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "test_session",
			Value: session.ID,
		})

		got, err := mgr.GetSessionFromRequest(req)
		if err != nil {
			t.Fatalf("failed to get session from request: %v", err)
		}

		if got.ID != session.ID {
			t.Errorf("session ID mismatch: got %s, want %s", got.ID, session.ID)
		}
	})

	t.Run("get session from request without cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		_, err := mgr.GetSessionFromRequest(req)
		if err == nil {
			t.Error("expected error when no cookie present")
		}
	})

	t.Run("get session from request with invalid session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "test_session",
			Value: "invalid-session-id",
		})

		_, err := mgr.GetSessionFromRequest(req)
		if err == nil {
			t.Error("expected error for invalid session")
		}
	})
}

func TestSessionMiddleware(t *testing.T) {
	mgr := NewSessionManager(SessionManagerConfig{
		CookieName: "test_session",
	})

	t.Run("valid session passes through", func(t *testing.T) {
		session, _ := mgr.CreateSession("user-123", "test@example.com")

		handler := mgr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := SessionFromContext(r.Context())
			if sess == nil {
				t.Error("session should be in context")
				return
			}
			if sess.UserID != "user-123" {
				t.Errorf("unexpected user ID: %s", sess.UserID)
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "test_session",
			Value: session.ID,
		})

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", recorder.Code)
		}
	})

	t.Run("invalid session returns 401", func(t *testing.T) {
		handler := mgr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", recorder.Code)
		}
	})
}

func TestRequireAuthMiddleware(t *testing.T) {
	mgr := NewSessionManager(SessionManagerConfig{
		CookieName: "test_session",
	})

	t.Run("valid session passes through", func(t *testing.T) {
		session, _ := mgr.CreateSession("user-123", "test@example.com")

		handler := mgr.RequireAuth("/login")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := SessionFromContext(r.Context())
			if sess == nil {
				t.Error("session should be in context")
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{
			Name:  "test_session",
			Value: session.ID,
		})

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", recorder.Code)
		}
	})

	t.Run("missing session redirects to login", func(t *testing.T) {
		handler := mgr.RequireAuth("/login")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/protected?foo=bar", nil)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", recorder.Code)
		}

		location := recorder.Header().Get("Location")
		if location != "/login?next=/protected?foo=bar" {
			t.Errorf("unexpected redirect location: %s", location)
		}
	})
}

func TestSessionContext(t *testing.T) {
	t.Run("round trip session through context", func(t *testing.T) {
		session := &Session{
			ID:     "ctx-test",
			UserID: "user-ctx",
			Email:  "ctx@example.com",
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := WithSession(req.Context(), session)

		got := SessionFromContext(ctx)
		if got == nil {
			t.Fatal("session should not be nil")
		}
		if got.ID != session.ID {
			t.Errorf("session ID mismatch: got %s, want %s", got.ID, session.ID)
		}
	})

	t.Run("no session in context returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		got := SessionFromContext(req.Context())
		if got != nil {
			t.Error("expected nil session from empty context")
		}
	})
}

func TestGenerateSessionID(t *testing.T) {
	// Generate multiple IDs to check uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := generateSessionID()
		if err != nil {
			t.Fatalf("failed to generate session ID: %v", err)
		}

		if len(id) == 0 {
			t.Error("session ID should not be empty")
		}

		if ids[id] {
			t.Errorf("duplicate session ID generated: %s", id)
		}
		ids[id] = true
	}
}
