// dex-setup is the setup wizard for Poindexter.
// It runs on a temporary Cloudflare tunnel and helps users establish
// permanent access via Tailscale or Cloudflare named tunnels.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
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
	PhasePin             SetupPhase = "pin"
	PhaseAccessChoice    SetupPhase = "access_choice"
	PhaseTailscaleSetup  SetupPhase = "tailscale_setup"
	PhaseCloudflareSetup SetupPhase = "cloudflare_setup"
	PhaseComplete        SetupPhase = "complete"
)

// SetupState represents the current state of the setup wizard
type SetupState struct {
	Phase         SetupPhase `json:"phase"`
	PINVerified   bool       `json:"pin_verified"`
	AccessMethod  string     `json:"access_method,omitempty"` // "tailscale" or "cloudflare"
	PermanentURL  string     `json:"permanent_url,omitempty"`
	TailscaleAuth string     `json:"tailscale_auth_url,omitempty"`
	Zones         []Zone     `json:"zones,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// SetupServer handles the setup wizard endpoints
type SetupServer struct {
	mu           sync.RWMutex
	state        SetupState
	pinVerifier  *PINVerifier
	done         chan struct{}
	dataDir      string
	dexPort      int
	cfClient     *CloudflareClient
	tunnelInfo   *TunnelInfo
	tunnelCmd    *exec.Cmd // cloudflared process
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
	mux.HandleFunc("/api/choose-tailscale", server.handleChooseTailscale)
	mux.HandleFunc("/api/choose-cloudflare", server.handleChooseCloudflare)
	mux.HandleFunc("/api/tailscale/status", server.handleTailscaleStatus)
	mux.HandleFunc("/api/tailscale/auth-url", server.handleTailscaleAuthURL)
	mux.HandleFunc("/api/tailscale/configure", server.handleTailscaleConfigure)
	mux.HandleFunc("/api/cloudflare/validate", server.handleCloudflareValidate)
	mux.HandleFunc("/api/cloudflare/zones", server.handleCloudflareZones)
	mux.HandleFunc("/api/cloudflare/setup", server.handleCloudflareSetup)
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
	s.state.Phase = PhaseAccessChoice
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *SetupServer) handleChooseTailscale(w http.ResponseWriter, r *http.Request) {
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
	s.state.AccessMethod = "tailscale"
	s.state.Phase = PhaseTailscaleSetup
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *SetupServer) handleChooseCloudflare(w http.ResponseWriter, r *http.Request) {
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
	s.state.AccessMethod = "cloudflare"
	s.state.Phase = PhaseCloudflareSetup
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *SetupServer) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if tailscale is installed
	if err := CheckTailscaleInstalled(); err != nil {
		sendJSON(w, http.StatusOK, map[string]any{
			"installed": false,
			"connected": false,
			"error":     "Tailscale is not installed",
		})
		return
	}

	// Get full status for debugging
	status, err := GetTailscaleStatus()
	if err != nil {
		log.Printf("Tailscale status error: %v", err)
		sendJSON(w, http.StatusOK, map[string]any{
			"installed":     true,
			"connected":     false,
			"error":         err.Error(),
			"backend_state": "unknown",
		})
		return
	}

	// Log for debugging
	log.Printf("Tailscale status: BackendState=%s, DNSName=%s, Online=%v",
		status.BackendState, status.Self.DNSName, status.Self.Online)

	// Check connection status - Running means connected
	connected := status.BackendState == "Running"

	// Get DNS name and serve URL if connected
	var dnsName, serveURL string
	if connected {
		dnsName = strings.TrimSuffix(status.Self.DNSName, ".")
		if dnsName != "" {
			serveURL = fmt.Sprintf("https://%s", dnsName)
		}
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"installed":     true,
		"connected":     connected,
		"backend_state": status.BackendState,
		"dns_name":      dnsName,
		"serve_url":     serveURL,
		"online":        status.Self.Online,
	})
}

func (s *SetupServer) handleTailscaleAuthURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Hostname string `json:"hostname"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Hostname == "" {
		req.Hostname = "dex"
	}

	// Start tailscale authentication
	authURL, checkConnected, err := StartTailscaleAuth(req.Hostname)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// If already connected
	if authURL == "" && checkConnected != nil && checkConnected() {
		dnsName, _ := GetTailscaleDNSName()
		serveURL, _ := GetTailscaleServeURL(443)
		sendJSON(w, http.StatusOK, map[string]any{
			"already_connected": true,
			"dns_name":          dnsName,
			"serve_url":         serveURL,
		})
		return
	}

	s.mu.Lock()
	s.state.TailscaleAuth = authURL
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"auth_url": authURL,
	})
}

func (s *SetupServer) handleTailscaleConfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if connected
	connected, err := IsTailscaleConnected()
	if err != nil || !connected {
		sendJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Tailscale not connected",
		})
		return
	}

	// Configure tailscale serve
	if err := ConfigureTailscaleServe(s.dexPort, 443); err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to configure Tailscale Serve: %v", err),
		})
		return
	}

	// Get the permanent URL
	serveURL, err := GetTailscaleServeURL(443)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to get serve URL: %v", err),
		})
		return
	}

	// Write permanent URL file for installer
	if err := os.WriteFile(filepath.Join(s.dataDir, "permanent-url"), []byte(serveURL), 0644); err != nil {
		log.Printf("Warning: failed to write permanent-url file: %v", err)
	}

	// Write access method file
	if err := os.WriteFile(filepath.Join(s.dataDir, "access-method"), []byte("tailscale"), 0644); err != nil {
		log.Printf("Warning: failed to write access-method file: %v", err)
	}

	s.mu.Lock()
	s.state.PermanentURL = serveURL
	s.state.Phase = PhaseComplete
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"permanent_url": serveURL,
	})
}

func (s *SetupServer) handleCloudflareValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		APIToken string `json:"api_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	if req.APIToken == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "API token required"})
		return
	}

	// Validate the token
	client := NewCloudflareClient(req.APIToken)
	if err := client.ValidateToken(); err != nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid API token",
		})
		return
	}

	// Get account ID to verify permissions
	accountID, err := client.GetAccountID()
	if err != nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Failed to get account: " + err.Error(),
		})
		return
	}

	// Store the client for later use
	s.mu.Lock()
	s.cfClient = client
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"valid":      true,
		"account_id": accountID,
	})
}

func (s *SetupServer) handleCloudflareZones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	client := s.cfClient
	s.mu.RUnlock()

	if client == nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{
			"error": "API token not validated",
		})
		return
	}

	zones, err := client.GetZones()
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to get zones: " + err.Error(),
		})
		return
	}

	s.mu.Lock()
	s.state.Zones = zones
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"zones": zones,
	})
}

func (s *SetupServer) handleCloudflareSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TunnelName string `json:"tunnel_name"`
		Subdomain  string `json:"subdomain"`
		ZoneID     string `json:"zone_id"`
		ZoneName   string `json:"zone_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	if req.TunnelName == "" {
		req.TunnelName = "poindexter"
	}
	if req.Subdomain == "" {
		req.Subdomain = "dex"
	}

	s.mu.RLock()
	client := s.cfClient
	s.mu.RUnlock()

	if client == nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{
			"error": "API token not validated",
		})
		return
	}

	// Create the tunnel
	tunnelInfo, err := client.CreateTunnel(req.TunnelName, s.dataDir)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create tunnel: " + err.Error(),
		})
		return
	}

	// Configure routing and DNS
	permanentURL, err := client.ConfigureTunnelRoute(
		tunnelInfo.ID,
		req.Subdomain,
		req.ZoneID,
		req.ZoneName,
		s.dexPort,
	)
	if err != nil {
		// Try to clean up the tunnel
		_ = client.DeleteTunnel(tunnelInfo.ID)
		sendJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to configure tunnel: " + err.Error(),
		})
		return
	}

	tunnelInfo.TunnelURL = permanentURL

	// Save tunnel info for the main app
	tunnelData, _ := json.Marshal(tunnelInfo)
	if err := os.WriteFile(filepath.Join(s.dataDir, "cloudflare-tunnel.json"), tunnelData, 0600); err != nil {
		log.Printf("Warning: failed to save tunnel info: %v", err)
	}

	// Write permanent URL file for installer
	if err := os.WriteFile(filepath.Join(s.dataDir, "permanent-url"), []byte(permanentURL), 0644); err != nil {
		log.Printf("Warning: failed to write permanent-url file: %v", err)
	}

	// Write access method file
	if err := os.WriteFile(filepath.Join(s.dataDir, "access-method"), []byte("cloudflare"), 0644); err != nil {
		log.Printf("Warning: failed to write access-method file: %v", err)
	}

	// Start cloudflared to make the tunnel accessible immediately
	tunnelCmd, err := RunCloudflaredTunnel(tunnelInfo.CredPath, tunnelInfo.ID)
	if err != nil {
		log.Printf("Warning: failed to start cloudflared: %v", err)
		// Don't fail - the installer will start it as a service
	} else {
		log.Printf("Started cloudflared tunnel connector (PID: %d)", tunnelCmd.Process.Pid)
	}

	s.mu.Lock()
	s.tunnelInfo = tunnelInfo
	s.tunnelCmd = tunnelCmd
	s.state.PermanentURL = permanentURL
	s.state.Phase = PhaseComplete
	s.mu.Unlock()

	sendJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"tunnel_id":     tunnelInfo.ID,
		"permanent_url": permanentURL,
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
		"success":       true,
		"permanent_url": state.PermanentURL,
		"access_method": state.AccessMethod,
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
