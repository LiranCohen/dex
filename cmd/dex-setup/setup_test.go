package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPINVerification(t *testing.T) {
	server := &SetupServer{
		state: SetupState{
			Phase: PhasePin,
		},
		pinVerifier: NewPINVerifier("123456"),
		done:        make(chan struct{}),
		dataDir:     t.TempDir(),
		dexPort:     8080,
	}

	t.Run("health check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()

		server.handleHealth(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}
	})

	t.Run("get initial state", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
		w := httptest.NewRecorder()

		server.handleGetState(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}

		var state SetupState
		_ = json.NewDecoder(w.Body).Decode(&state)

		if state.Phase != PhasePin {
			t.Errorf("Expected phase pin, got %s", state.Phase)
		}
	})

	t.Run("verify PIN with wrong PIN", func(t *testing.T) {
		body := map[string]string{"pin": "000000"}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/verify-pin", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleVerifyPIN(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("verify PIN with correct PIN", func(t *testing.T) {
		body := map[string]string{"pin": "123456"}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/verify-pin", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleVerifyPIN(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Check state advanced to mesh_setup
		req = httptest.NewRequest(http.MethodGet, "/api/state", nil)
		w = httptest.NewRecorder()
		server.handleGetState(w, req)

		var state SetupState
		_ = json.NewDecoder(w.Body).Decode(&state)

		if state.Phase != PhaseMeshSetup {
			t.Errorf("Expected phase mesh_setup, got %s", state.Phase)
		}
		if !state.PINVerified {
			t.Error("Expected PINVerified to be true")
		}
	})
}

func TestMethodNotAllowed(t *testing.T) {
	server := &SetupServer{
		state: SetupState{
			Phase: PhasePin,
		},
		pinVerifier: NewPINVerifier("123456"),
		done:        make(chan struct{}),
		dataDir:     t.TempDir(),
		dexPort:     8080,
	}

	t.Run("state POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/state", nil)
		w := httptest.NewRecorder()

		server.handleGetState(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", w.Code)
		}
	})

	t.Run("verify-pin GET not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/verify-pin", nil)
		w := httptest.NewRecorder()

		server.handleVerifyPIN(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", w.Code)
		}
	})

	t.Run("complete GET not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/complete", nil)
		w := httptest.NewRecorder()

		server.handleComplete(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", w.Code)
		}
	})
}

func TestPINVerifier(t *testing.T) {
	t.Run("correct PIN", func(t *testing.T) {
		v := NewPINVerifier("123456")
		err := v.Verify("123456")
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("wrong PIN", func(t *testing.T) {
		v := NewPINVerifier("123456")
		err := v.Verify("000000")
		if err != ErrInvalidPIN {
			t.Errorf("Expected ErrInvalidPIN, got %v", err)
		}
	})

	t.Run("rate limiting", func(t *testing.T) {
		v := NewPINVerifier("123456")
		v.maxAttempts = 3 // Lower for testing

		// Make 3 wrong attempts
		for i := 0; i < 3; i++ {
			_ = v.Verify("000000")
		}

		// Next attempt should be rate limited
		err := v.Verify("123456")
		if err != ErrRateLimited {
			t.Errorf("Expected ErrRateLimited, got %v", err)
		}
	})

	t.Run("empty PIN config", func(t *testing.T) {
		v := NewPINVerifier("")
		err := v.Verify("123456")
		if err != ErrPINNotSet {
			t.Errorf("Expected ErrPINNotSet, got %v", err)
		}
	})
}
