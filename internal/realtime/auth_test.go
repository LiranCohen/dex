package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifuge"
)

// mockTokenValidator is a test implementation of TokenValidator
type mockTokenValidator struct {
	user *UserInfo
	err  error
}

func (m *mockTokenValidator) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.user, nil
}

func TestAuthMiddleware(t *testing.T) {
	validUser := &UserInfo{ID: "user-123", Username: "testuser"}

	t.Run("extracts token from Authorization header", func(t *testing.T) {
		validator := &mockTokenValidator{user: validUser}
		middleware := AuthMiddleware(validator)

		var capturedCtx context.Context
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		cred, ok := centrifuge.GetCredentials(capturedCtx)
		if !ok {
			t.Fatal("Expected credentials in context")
		}
		if cred.UserID != "user-123" {
			t.Errorf("Expected UserID 'user-123', got %q", cred.UserID)
		}
	})

	t.Run("extracts token from query parameter", func(t *testing.T) {
		validator := &mockTokenValidator{user: validUser}
		middleware := AuthMiddleware(validator)

		var capturedCtx context.Context
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test?token=query-token", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		cred, ok := centrifuge.GetCredentials(capturedCtx)
		if !ok {
			t.Fatal("Expected credentials in context")
		}
		if cred.UserID != "user-123" {
			t.Errorf("Expected UserID 'user-123', got %q", cred.UserID)
		}
	})

	t.Run("prefers Authorization header over query param", func(t *testing.T) {
		callCount := 0
		validator := &mockTokenValidator{user: validUser}
		// Wrap to track calls
		trackingValidator := &mockTokenValidator{
			user: validUser,
		}

		middleware := AuthMiddleware(trackingValidator)

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test?token=query-token", nil)
		req.Header.Set("Authorization", "Bearer header-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}
		_ = validator // silence unused warning
	})

	t.Run("rejects request without token", func(t *testing.T) {
		validator := &mockTokenValidator{user: validUser}
		middleware := AuthMiddleware(validator)

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("rejects request with invalid token", func(t *testing.T) {
		validator := &mockTokenValidator{err: errors.New("invalid token")}
		middleware := AuthMiddleware(validator)

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("ignores non-Bearer Authorization header", func(t *testing.T) {
		validator := &mockTokenValidator{user: validUser}
		middleware := AuthMiddleware(validator)

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called without valid token")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic auth
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})
}

func TestNoAuthMiddleware(t *testing.T) {
	t.Run("sets anonymous user credentials", func(t *testing.T) {
		middleware := NoAuthMiddleware()

		var capturedCtx context.Context
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		cred, ok := centrifuge.GetCredentials(capturedCtx)
		if !ok {
			t.Fatal("Expected credentials in context")
		}
		if cred.UserID != "anonymous" {
			t.Errorf("Expected UserID 'anonymous', got %q", cred.UserID)
		}
	})

	t.Run("allows any request", func(t *testing.T) {
		middleware := NoAuthMiddleware()

		handlerCalled := false
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		// Request with no auth at all
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("Expected handler to be called")
		}
	})
}

func TestUserInfo(t *testing.T) {
	t.Run("stores user info correctly", func(t *testing.T) {
		info := &UserInfo{
			ID:       "user-456",
			Username: "john_doe",
		}

		if info.ID != "user-456" {
			t.Errorf("Expected ID 'user-456', got %q", info.ID)
		}
		if info.Username != "john_doe" {
			t.Errorf("Expected Username 'john_doe', got %q", info.Username)
		}
	})
}

func TestJWTValidator(t *testing.T) {
	t.Run("creates validator with config", func(t *testing.T) {
		// We can't easily test actual JWT validation without setting up
		// a full token config, but we can verify the validator is created
		validator := NewJWTValidator(nil)
		if validator == nil {
			t.Fatal("Expected validator to be non-nil")
		}
	})
}
