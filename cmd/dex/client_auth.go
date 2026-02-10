//go:build !notray

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WebP2P/dexnet/types/key"
	"github.com/lirancohen/dex/internal/meshd"
)

const (
	// callbackPortStart is the starting port for the local callback server.
	callbackPortStart = 19532
	// callbackPortRange is how many ports to try before giving up.
	callbackPortRange = 10
)

// authCallback holds the result of a browser auth callback.
type authCallback struct {
	Code  string
	State string
}

// authExchangeResponse is the response from POST /api/v1/auth/exchange.
type authExchangeResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Namespace string `json:"namespace"`
}

// enrollmentKeyResponse is the response from POST /api/v1/enrollment-keys.
type enrollmentKeyResponse struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Hostname       string `json:"hostname"`
	Namespace      string `json:"namespace"`
	Type           string `json:"type"`
	ExpiresAt      string `json:"expires_at"`
	InstallCommand string `json:"install_command"`
}

// callbackServer runs a local HTTP server that receives the OAuth callback
// from the browser after the user completes authentication on Central.
type callbackServer struct {
	listener net.Listener
	port     int
	state    string // CSRF state parameter
	resultCh chan authCallback
	server   *http.Server
}

// newCallbackServer starts a local HTTP server on localhost for receiving
// the auth callback. It tries ports starting from callbackPortStart.
func newCallbackServer() (*callbackServer, error) {
	// Generate random state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Find an available port
	var ln net.Listener
	var port int
	for i := 0; i < callbackPortRange; i++ {
		p := callbackPortStart + i
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		ln = l
		port = p
		break
	}
	if ln == nil {
		return nil, fmt.Errorf("could not find available port in range %d-%d", callbackPortStart, callbackPortStart+callbackPortRange)
	}

	cs := &callbackServer{
		listener: ln,
		port:     port,
		state:    state,
		resultCh: make(chan authCallback, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)
	mux.HandleFunc("/health", cs.handleHealth)

	cs.server = &http.Server{Handler: mux}

	go func() {
		if err := cs.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("Callback server error: %v", err)
		}
	}()

	return cs, nil
}

func (cs *callbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	if state != cs.state {
		http.Error(w, "invalid state", http.StatusForbidden)
		return
	}

	// Send the result to the tray
	select {
	case cs.resultCh <- authCallback{Code: code, State: state}:
	default:
	}

	// Return a success page to the browser
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
  <title>Dex - Authenticated</title>
  <style>
    body { font-family: -apple-system, system-ui, sans-serif; display: flex; 
           justify-content: center; align-items: center; min-height: 100vh; 
           margin: 0; background: #0a0a0a; color: #e5e5e5; }
    .card { text-align: center; padding: 2rem; border-radius: 12px; 
            background: #171717; border: 1px solid #262626; max-width: 400px; }
    h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
    p { color: #a3a3a3; }
    .check { font-size: 3rem; margin-bottom: 1rem; }
  </style>
</head>
<body>
  <div class="card">
    <div class="check">&#10003;</div>
    <h1>Authentication Successful</h1>
    <p>You can close this tab and return to the Dex Client app.</p>
  </div>
</body>
</html>`)
}

func (cs *callbackServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

// waitForCallback blocks until a callback is received or the context is cancelled.
func (cs *callbackServer) waitForCallback(ctx context.Context) (*authCallback, error) {
	select {
	case result := <-cs.resultCh:
		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// close shuts down the callback server.
func (cs *callbackServer) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = cs.server.Shutdown(ctx)
}

// exchangeAuthCode exchanges a one-time auth code for a JWT session token.
func exchangeAuthCode(centralURL, code string) (*authExchangeResponse, error) {
	url := strings.TrimSuffix(centralURL, "/") + "/api/v1/auth/exchange"

	reqBody, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange failed: %s", strings.TrimSpace(string(body)))
	}

	var result authExchangeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

// createClientEnrollmentKey creates a client enrollment key via Central API.
func createClientEnrollmentKey(centralURL, authToken, hostname string) (*enrollmentKeyResponse, error) {
	url := strings.TrimSuffix(centralURL, "/") + "/api/v1/enrollment-keys"

	reqBody, err := json.Marshal(map[string]string{
		"hostname": hostname,
		"type":     "client",
	})
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("User-Agent", "dex-client/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create enrollment key request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create enrollment key failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result enrollmentKeyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

// autoEnroll performs the full enrollment flow: create enrollment key,
// generate machine key, call enroll API, save config.
func autoEnroll(centralURL, authToken, namespace, dataDir string) (*ClientConfig, error) {
	log.Printf("Auto-enrolling with Central...")

	// 1. Determine hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "client"
	}
	hostname = cleanHostname(hostname)

	// 2. Create a client enrollment key via Central API
	log.Printf("Creating client enrollment key for %q...", hostname)
	enrollKey, err := createClientEnrollmentKey(centralURL, authToken, hostname)
	if err != nil {
		return nil, fmt.Errorf("creating enrollment key: %w", err)
	}
	log.Printf("Got enrollment key for hostname %q", enrollKey.Hostname)

	// 3. Set up mesh state directory
	meshStateDir := filepath.Join(dataDir, "mesh")
	if err := os.RemoveAll(meshStateDir); err != nil {
		return nil, fmt.Errorf("cleaning mesh state: %w", err)
	}
	if err := os.MkdirAll(meshStateDir, 0755); err != nil {
		return nil, fmt.Errorf("creating mesh state dir: %w", err)
	}

	// 4. Generate machine key
	machineKey := key.NewMachine()
	machineKeyPublic := machineKey.Public()

	if err := saveClientMachineKey(meshStateDir, machineKey); err != nil {
		return nil, fmt.Errorf("saving machine key: %w", err)
	}

	// 5. Call enrollment API
	log.Printf("Enrolling with Central...")
	enrollResp, err := callClientEnrollmentAPI(centralURL, ClientEnrollmentRequest{
		Key:        enrollKey.Key,
		MachineKey: machineKeyPublic.String(),
		Hostname:   enrollKey.Hostname,
	})
	if err != nil {
		_ = os.RemoveAll(meshStateDir)
		return nil, fmt.Errorf("enrollment failed: %w", err)
	}

	// 6. Build and save config
	publicDomain := enrollResp.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}
	meshDomain := enrollResp.Domains.Mesh
	if meshDomain == "" {
		meshDomain = "dex"
	}

	config := &ClientConfig{
		Namespace:  enrollResp.Namespace,
		Hostname:   enrollResp.Hostname,
		CentralURL: centralURL,
		AuthToken:  authToken,
		Domains: ClientDomainConfig{
			Public: publicDomain,
			Mesh:   meshDomain,
		},
		Mesh: ClientMeshConfig{
			ControlURL: enrollResp.Mesh.ControlURL,
		},
	}

	configPath := filepath.Join(dataDir, "config.json")
	if err := config.SaveClientConfig(configPath); err != nil {
		return nil, fmt.Errorf("saving config: %w", err)
	}

	log.Printf("Enrolled as %s.%s", enrollResp.Hostname, enrollResp.Namespace)
	return config, nil
}

// installMeshdWithPrivileges installs the mesh daemon using an OS privilege prompt.
// On macOS, this uses osascript to show the native auth dialog (Touch ID / password).
func installMeshdWithPrivileges() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("meshd install only supported on macOS")
	}

	// Find the dex binary path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	log.Printf("Installing mesh daemon (requires administrator privileges)...")
	cmd := exec.Command("osascript", "-e",
		fmt.Sprintf(`do shell script "%s meshd install" with administrator privileges`, exe))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("meshd install failed: %w\n%s", err, string(output))
	}

	log.Printf("Mesh daemon installed")
	return nil
}

// installMeshdIfNeeded installs the mesh daemon if it's not already installed.
func installMeshdIfNeeded() {
	if runtime.GOOS != "darwin" {
		return
	}

	if meshd.IsInstalled() {
		log.Printf("Mesh daemon already installed")
		return
	}

	if err := installMeshdWithPrivileges(); err != nil {
		log.Printf("Warning: failed to install mesh daemon: %v", err)
		log.Printf("You can install it manually with: sudo dex meshd install")
	}
}
