package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupHandler(t *testing.T) {
	// Create temp files for output
	tmpDir := t.TempDir()
	secretsPath := filepath.Join(tmpDir, "secrets.json")
	donePath := filepath.Join(tmpDir, "done")

	// Set flags
	*outputFile = secretsPath
	*doneFile = donePath
	*dexURL = "https://dex.test.ts.net"

	server := &SetupServer{
		done: make(chan struct{}),
	}

	t.Run("health check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()

		server.handleHealth(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}
	})

	t.Run("setup with valid keys", func(t *testing.T) {
		body := SetupRequest{
			Anthropic: "sk-ant-api03-test-key",
			GitHub:    "ghp_testtoken123456789",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleSetup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp SetupResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Error != "" {
			t.Errorf("Unexpected error: %s", resp.Error)
		}

		if !resp.Success {
			t.Error("Expected success in response")
		}

		// Verify secrets file was created
		data, err := os.ReadFile(secretsPath)
		if err != nil {
			t.Fatalf("Failed to read secrets file: %v", err)
		}

		var secrets Secrets
		if err := json.Unmarshal(data, &secrets); err != nil {
			t.Fatalf("Failed to parse secrets: %v", err)
		}

		if secrets.Anthropic != body.Anthropic {
			t.Errorf("Anthropic key mismatch")
		}
		if secrets.GitHub != body.GitHub {
			t.Errorf("GitHub token mismatch")
		}
	})

	t.Run("setup with invalid anthropic key", func(t *testing.T) {
		body := SetupRequest{
			Anthropic: "invalid-key",
			GitHub:    "ghp_testtoken123456789",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleSetup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", w.Code)
		}

		var resp SetupResponse
		json.NewDecoder(w.Body).Decode(&resp)
		if !strings.Contains(resp.Error, "Anthropic") {
			t.Errorf("Expected Anthropic error, got: %s", resp.Error)
		}
	})

	t.Run("setup with invalid github token", func(t *testing.T) {
		body := SetupRequest{
			Anthropic: "sk-ant-api03-valid",
			GitHub:    "invalid-token",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleSetup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", w.Code)
		}

		var resp SetupResponse
		json.NewDecoder(w.Body).Decode(&resp)
		if !strings.Contains(resp.Error, "GitHub") {
			t.Errorf("Expected GitHub error, got: %s", resp.Error)
		}
	})

	t.Run("complete endpoint", func(t *testing.T) {
		// Reset done file
		os.Remove(donePath)

		req := httptest.NewRequest(http.MethodPost, "/api/complete", nil)
		w := httptest.NewRecorder()

		server.handleComplete(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}

		var resp CompleteResponse
		json.NewDecoder(w.Body).Decode(&resp)

		if resp.URL != *dexURL {
			t.Errorf("Expected URL %s, got %s", *dexURL, resp.URL)
		}

		// Verify done file was created
		if _, err := os.Stat(donePath); os.IsNotExist(err) {
			t.Error("Done file was not created")
		}
	})
}

func TestSetupMethodNotAllowed(t *testing.T) {
	server := &SetupServer{
		done: make(chan struct{}),
	}

	t.Run("setup GET not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/setup", nil)
		w := httptest.NewRecorder()

		server.handleSetup(w, req)

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
