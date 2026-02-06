package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

		// Check state advanced to mesh setup
		req = httptest.NewRequest(http.MethodGet, "/api/state", nil)
		w = httptest.NewRecorder()
		server.handleGetState(w, req)

		var state SetupState
		_ = json.NewDecoder(w.Body).Decode(&state)

		if state.Phase != PhaseMeshSetup {
			t.Errorf("Expected phase mesh_setup, got %s", state.Phase)
		}
	})
}

func TestMeshConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	server := &SetupServer{
		state: SetupState{
			Phase:       PhaseMeshSetup,
			PINVerified: true,
		},
		pinVerifier: NewPINVerifier("123456"),
		done:        make(chan struct{}),
		dataDir:     tmpDir,
		dexPort:     8080,
	}

	t.Run("configure mesh without PIN verified", func(t *testing.T) {
		server.state.PINVerified = false

		body := map[string]any{
			"hostname":    "my-hq",
			"control_url": "https://central.enbox.id",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/mesh/configure", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMeshConfigure(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected 403, got %d: %s", w.Code, w.Body.String())
		}

		server.state.PINVerified = true
	})

	t.Run("configure mesh with basic settings", func(t *testing.T) {
		body := map[string]any{
			"hostname":    "my-hq",
			"control_url": "https://central.enbox.id",
			"auth_key":    "tskey-auth-xxxx",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/mesh/configure", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMeshConfigure(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Check state
		if server.state.Phase != PhaseComplete {
			t.Errorf("Expected phase complete, got %s", server.state.Phase)
		}
		if server.state.MeshHostname != "my-hq" {
			t.Errorf("Expected hostname my-hq, got %s", server.state.MeshHostname)
		}

		// Verify config file was written
		configPath := filepath.Join(tmpDir, "mesh-config.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatalf("Failed to parse config: %v", err)
		}

		if config["hostname"] != "my-hq" {
			t.Errorf("Expected hostname my-hq in config, got %v", config["hostname"])
		}
		if config["is_hq"] != true {
			t.Errorf("Expected is_hq true in config, got %v", config["is_hq"])
		}
	})

	t.Run("configure mesh with tunnel settings", func(t *testing.T) {
		server.state.Phase = PhaseMeshSetup

		body := map[string]any{
			"hostname":            "my-hq",
			"control_url":         "https://central.enbox.id",
			"auth_key":            "tskey-auth-xxxx",
			"tunnel_enabled":      true,
			"tunnel_ingress_addr": "ingress.enbox.id:9443",
			"tunnel_token":        "hq-token-123",
			"tunnel_endpoints": []map[string]any{
				{"hostname": "api.alice.enbox.id", "local_port": 8080},
				{"hostname": "web.alice.enbox.id", "local_port": 3000},
			},
			"acme_enabled": true,
			"acme_email":   "admin@example.com",
			"acme_staging": true,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/mesh/configure", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMeshConfigure(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Check state
		if !server.state.TunnelEnabled {
			t.Error("Expected tunnel enabled in state")
		}
		if !server.state.ACMEEnabled {
			t.Error("Expected ACME enabled in state")
		}

		// Verify config file
		configPath := filepath.Join(tmpDir, "mesh-config.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatalf("Failed to parse config: %v", err)
		}

		tunnel, ok := config["tunnel"].(map[string]any)
		if !ok {
			t.Fatal("Expected tunnel config in file")
		}
		if tunnel["enabled"] != true {
			t.Errorf("Expected tunnel enabled in config, got %v", tunnel["enabled"])
		}
		if tunnel["token"] != "hq-token-123" {
			t.Errorf("Expected tunnel token in config, got %v", tunnel["token"])
		}

		acme, ok := tunnel["acme"].(map[string]any)
		if !ok {
			t.Fatal("Expected acme config in tunnel")
		}
		if acme["enabled"] != true {
			t.Errorf("Expected acme enabled in config, got %v", acme["enabled"])
		}
		if acme["email"] != "admin@example.com" {
			t.Errorf("Expected acme email in config, got %v", acme["email"])
		}
	})
}

func TestMeshStatus(t *testing.T) {
	server := &SetupServer{
		state: SetupState{
			Phase:          PhaseComplete,
			MeshHostname:   "my-hq",
			MeshControlURL: "https://central.enbox.id",
			MeshConnected:  true,
			MeshIP:         "10.200.1.1",
		},
		pinVerifier: NewPINVerifier("123456"),
		done:        make(chan struct{}),
		dataDir:     t.TempDir(),
		dexPort:     8080,
	}

	t.Run("get mesh status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/mesh/status", nil)
		w := httptest.NewRecorder()

		server.handleMeshStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}

		var status map[string]any
		_ = json.NewDecoder(w.Body).Decode(&status)

		if status["configured"] != true {
			t.Errorf("Expected configured true, got %v", status["configured"])
		}
		if status["hostname"] != "my-hq" {
			t.Errorf("Expected hostname my-hq, got %v", status["hostname"])
		}
		if status["connected"] != true {
			t.Errorf("Expected connected true, got %v", status["connected"])
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
