// dex-setup is the setup wizard for Poindexter.
// It helps users configure mesh networking to connect HQ to the Campus network.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

var (
	addr       = flag.String("addr", "127.0.0.1:8081", "Address to listen on")
	pinFile    = flag.String("pin-file", "", "File containing the setup PIN")
	dataDir    = flag.String("data-dir", "/opt/dex", "Data directory for storing configuration")
	dexPort    = flag.Int("dex-port", 8080, "Port where dex will run")
)

// SetupPhase represents the current phase of setup
type SetupPhase string

const (
	PhasePin       SetupPhase = "pin"
	PhaseMeshSetup SetupPhase = "mesh_setup"
	PhaseComplete  SetupPhase = "complete"
)

// SetupState represents the current state of the setup wizard
type SetupState struct {
	Phase       SetupPhase `json:"phase"`
	PINVerified bool       `json:"pin_verified"`

	// Mesh setup state
	MeshHostname   string `json:"mesh_hostname,omitempty"`
	MeshControlURL string `json:"mesh_control_url,omitempty"`
	MeshConnected  bool   `json:"mesh_connected,omitempty"`
	MeshIP         string `json:"mesh_ip,omitempty"`

	Error string `json:"error,omitempty"`
}

// SetupServer handles the setup wizard endpoints
type SetupServer struct {
	mu          sync.RWMutex
	state       SetupState
	pinVerifier *PINVerifier
	done        chan struct{}
	dataDir     string
	dexPort     int
}

func main() {
	flag.Parse()

	// Load PIN from file
	pin := ""
	if *pinFile != "" {
		data, err := os.ReadFile(*pinFile)
		if err != nil {
			log.Fatalf("Failed to read PIN file: %v", err)
		}
		pin = string(data)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	server := &SetupServer{
		state: SetupState{
			Phase: PhasePin,
		},
		pinVerifier: NewPINVerifier(pin),
		done:        make(chan struct{}),
		dataDir:     *dataDir,
		dexPort:     *dexPort,
	}

	mux := http.NewServeMux()

	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// API endpoints
	mux.HandleFunc("/api/state", server.handleGetState)
	mux.HandleFunc("/api/verify-pin", server.handleVerifyPIN)
	mux.HandleFunc("/api/mesh/configure", server.handleMeshConfigure)
	mux.HandleFunc("/api/mesh/status", server.handleMeshStatus)
	mux.HandleFunc("/api/complete", server.handleComplete)
	mux.HandleFunc("/api/health", server.handleHealth)

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	// Handle shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-sigChan:
			log.Println("Received shutdown signal")
		case <-server.done:
			log.Println("Setup complete, shutting down")
		}

		_ = httpServer.Close()
	}()

	log.Printf("Setup wizard running on %s", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (s *SetupServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *SetupServer) handleGetState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	sendJSON(w, http.StatusOK, state)
}

func (s *SetupServer) handleVerifyPIN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	err := s.pinVerifier.Verify(req.PIN)
	if err != nil {
		remaining := s.pinVerifier.AttemptsRemaining()
		sendJSON(w, http.StatusUnauthorized, map[string]any{
			"error":             err.Error(),
			"attempts_remaining": remaining,
		})
		return
	}

	s.mu.Lock()
	s.state.PINVerified = true
	s.state.Phase = PhaseMeshSetup
	s.state.MeshControlURL = "https://central.dex.dev" // Default
	hostname, _ := os.Hostname()
	s.state.MeshHostname = hostname
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *SetupServer) handleMeshConfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	if !s.state.PINVerified {
		s.mu.Unlock()
		sendJSON(w, http.StatusForbidden, map[string]string{"error": "PIN not verified"})
		return
	}
	s.mu.Unlock()

	var req struct {
		Hostname   string `json:"hostname"`
		ControlURL string `json:"control_url"`
		AuthKey    string `json:"auth_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Use defaults if not provided
	if req.Hostname == "" {
		req.Hostname, _ = os.Hostname()
	}
	if req.ControlURL == "" {
		req.ControlURL = "https://central.dex.dev"
	}

	// Save mesh configuration
	meshConfig := map[string]any{
		"enabled":     true,
		"hostname":    req.Hostname,
		"control_url": req.ControlURL,
		"auth_key":    req.AuthKey,
		"state_dir":   filepath.Join(s.dataDir, "mesh"),
		"is_hq":       true,
	}

	configData, _ := json.MarshalIndent(meshConfig, "", "  ")
	configPath := filepath.Join(s.dataDir, "mesh-config.json")
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to save mesh configuration: " + err.Error(),
		})
		return
	}

	s.mu.Lock()
	s.state.MeshHostname = req.Hostname
	s.state.MeshControlURL = req.ControlURL
	s.state.Phase = PhaseComplete
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"hostname":    req.Hostname,
		"control_url": req.ControlURL,
	})
}

func (s *SetupServer) handleMeshStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"configured":  state.Phase == PhaseComplete,
		"hostname":    state.MeshHostname,
		"control_url": state.MeshControlURL,
		"connected":   state.MeshConnected,
		"mesh_ip":     state.MeshIP,
	})
}

func (s *SetupServer) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	if state.Phase != PhaseComplete {
		sendJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Setup not complete",
		})
		return
	}

	// Write completion file for installer
	completeFile := filepath.Join(s.dataDir, "setup-phase1-complete")
	if err := os.WriteFile(completeFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to signal completion",
		})
		return
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"hostname":     state.MeshHostname,
		"control_url":  state.MeshControlURL,
		"access_method": "mesh",
	})

	// Give time for response to be sent, then shut down
	go func() {
		time.Sleep(500 * time.Millisecond)
		close(s.done)
	}()
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
