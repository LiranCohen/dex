// dex-setup is a temporary setup wizard that collects API keys.
// It's designed to run once during installation, served via tailscale serve.
// Authentication is handled via passkeys in the main application.
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
	"strings"
	"sync"
	"syscall"
)

//go:embed static/*
var staticFiles embed.FS

var (
	addr       = flag.String("addr", "127.0.0.1:9999", "Address to listen on")
	outputFile = flag.String("output", "/tmp/dex-setup-secrets.json", "Output file for secrets")
	doneFile   = flag.String("done", "/tmp/dex-setup-complete", "File to create when setup is complete")
	dexURL     = flag.String("url", "", "The dex URL to show after setup")
)

type SetupServer struct {
	mu      sync.Mutex
	secrets *Secrets
	done    chan struct{}
}

type Secrets struct {
	Anthropic string `json:"anthropic"`
	GitHub    string `json:"github"`
}

type SetupRequest struct {
	Anthropic string `json:"anthropic"`
	GitHub    string `json:"github"`
}

type SetupResponse struct {
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

type CompleteResponse struct {
	URL   string `json:"url"`
	Error string `json:"error,omitempty"`
}

func main() {
	flag.Parse()

	server := &SetupServer{
		done: make(chan struct{}),
	}

	mux := http.NewServeMux()

	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// API endpoints
	mux.HandleFunc("/api/setup", server.handleSetup)
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

		httpServer.Close()
	}()

	log.Printf("Setup wizard running on %s", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (s *SetupServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *SetupServer) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, SetupResponse{Error: "Invalid request"})
		return
	}

	// Validate inputs
	if !strings.HasPrefix(req.Anthropic, "sk-ant") {
		sendJSON(w, http.StatusBadRequest, SetupResponse{Error: "Invalid Anthropic key (should start with sk-ant)"})
		return
	}
	if !strings.HasPrefix(req.GitHub, "ghp_") && !strings.HasPrefix(req.GitHub, "github_pat_") {
		sendJSON(w, http.StatusBadRequest, SetupResponse{Error: "Invalid GitHub token (should start with ghp_ or github_pat_)"})
		return
	}

	s.mu.Lock()
	s.secrets = &Secrets{
		Anthropic: req.Anthropic,
		GitHub:    req.GitHub,
	}
	s.mu.Unlock()

	// Save to file for installer to pick up
	if err := s.saveSecrets(); err != nil {
		sendJSON(w, http.StatusInternalServerError, SetupResponse{Error: "Failed to save secrets"})
		return
	}

	sendJSON(w, http.StatusOK, SetupResponse{Success: true})
}

func (s *SetupServer) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create done file
	if err := os.WriteFile(*doneFile, []byte("done"), 0644); err != nil {
		sendJSON(w, http.StatusInternalServerError, CompleteResponse{Error: "Failed to signal completion"})
		return
	}

	url := *dexURL
	if url == "" {
		url = "https://dex.your-tailnet.ts.net"
	}

	sendJSON(w, http.StatusOK, CompleteResponse{URL: url})

	// Signal shutdown after response is sent
	go func() {
		close(s.done)
	}()
}

func (s *SetupServer) saveSecrets() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.secrets == nil {
		return os.ErrInvalid
	}

	data, err := json.MarshalIndent(s.secrets, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(*outputFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write with restrictive permissions
	if err := os.WriteFile(*outputFile, data, 0600); err != nil {
		return err
	}

	return nil
}

func sendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
