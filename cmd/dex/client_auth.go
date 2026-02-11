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
	Token      string                 `json:"token"`
	ExpiresAt  string                 `json:"expires_at"`
	Namespace  string                 `json:"namespace"`
	DexProfile *dexProfileFromCentral `json:"dex_profile,omitempty"`
}

// dexProfileFromCentral is the Dex personality data included in the auth exchange response
// from Central. The tray stores this and forwards it to HQ during bootstrap.
type dexProfileFromCentral struct {
	Traits             []string        `json:"traits"`
	GreetingStyle      string          `json:"greeting_style"`
	Catchphrase        string          `json:"catchphrase"`
	AvatarURL          string          `json:"avatar_url,omitempty"`
	OnboardingMessages json.RawMessage `json:"onboarding_messages,omitempty"`
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
		writeCallbackErrorHTML(w, "Invalid Request",
			"The authentication callback is missing required parameters. Please try signing in again from the Dex Client app.",
			http.StatusBadRequest)
		return
	}

	if state != cs.state {
		writeCallbackErrorHTML(w, "Invalid Session",
			"The authentication session has expired or is invalid. Please try signing in again from the Dex Client app.",
			http.StatusForbidden)
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
    .close-btn { margin-top: 1rem; padding: 0.5rem 1rem; background: #262626;
                 color: #e5e5e5; border: 1px solid #404040; border-radius: 6px;
                 cursor: pointer; font-size: 14px; }
    .close-btn:hover { background: #333; }
  </style>
</head>
<body>
  <div class="card">
    <div class="check">&#10003;</div>
    <h1>Authentication Successful</h1>
    <p>You can close this tab and return to the Dex Client app.</p>
    <button onclick="window.close()" class="close-btn">Close This Tab</button>
  </div>
  <script>setTimeout(function(){ window.close(); }, 2000);</script>
</body>
</html>`)
}

// writeCallbackErrorHTML writes a styled HTML error response for the auth callback.
func writeCallbackErrorHTML(w http.ResponseWriter, title, message string, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
  <title>Dex - Error</title>
  <style>
    body { font-family: -apple-system, system-ui, sans-serif; display: flex; 
           justify-content: center; align-items: center; min-height: 100vh; 
           margin: 0; background: #0a0a0a; color: #e5e5e5; }
    .card { text-align: center; padding: 2rem; border-radius: 12px; 
            background: #171717; border: 1px solid #262626; max-width: 400px; }
    h1 { font-size: 1.5rem; margin-bottom: 0.5rem; color: #ef4444; }
    p { color: #a3a3a3; }
    .icon { font-size: 3rem; margin-bottom: 1rem; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">&#10007;</div>
    <h1>%s</h1>
    <p>%s</p>
  </div>
</body>
</html>`, title, message)
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
func autoEnroll(centralURL, authToken, namespace, dataDir string, dexProfile *dexProfileFromCentral) (*ClientConfig, error) {
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

	// Store Dex profile from Central if available (for forwarding to HQ)
	if dexProfile != nil {
		config.DexProfile = &ClientDexProfile{
			Traits:             dexProfile.Traits,
			GreetingStyle:      dexProfile.GreetingStyle,
			Catchphrase:        dexProfile.Catchphrase,
			AvatarURL:          dexProfile.AvatarURL,
			OnboardingMessages: dexProfile.OnboardingMessages,
		}
	}

	configPath := filepath.Join(dataDir, "config.json")
	if err := config.SaveClientConfig(configPath); err != nil {
		return nil, fmt.Errorf("saving config: %w", err)
	}

	log.Printf("Enrolled as %s.%s", enrollResp.Hostname, enrollResp.Namespace)
	return config, nil
}

// forwardDexProfileToHQ sends the Dex personality data to HQ's bootstrap endpoint.
// This is best-effort — if HQ isn't reachable yet or the profile was already sent,
// the error is logged but doesn't block enrollment. HQ may not be running yet when
// this is called, so we retry a few times with backoff.
func forwardDexProfileToHQ(hqURL string, profile *ClientDexProfile) {
	url := strings.TrimSuffix(hqURL, "/") + "/api/v1/setup/dex-profile"

	payload := map[string]any{
		"traits":              profile.Traits,
		"greeting_style":      profile.GreetingStyle,
		"catchphrase":         profile.Catchphrase,
		"avatar_url":          profile.AvatarURL,
		"onboarding_messages": profile.OnboardingMessages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal Dex profile: %v", err)
		return
	}

	client := &http.Client{Timeout: 60 * time.Second}

	// Retry with backoff — HQ might not be up yet
	backoffs := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
	for i, wait := range backoffs {
		time.Sleep(wait)

		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("Dex profile forward attempt %d/%d failed: %v", i+1, len(backoffs), err)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Printf("Dex profile forwarded to HQ successfully")
			return
		}
		if resp.StatusCode == http.StatusConflict {
			log.Printf("Dex profile already exists on HQ")
			return
		}

		log.Printf("Dex profile forward attempt %d/%d: HTTP %d", i+1, len(backoffs), resp.StatusCode)
	}

	log.Printf("Warning: failed to forward Dex profile to HQ after %d attempts", len(backoffs))
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
	escapedExe := strings.ReplaceAll(exe, `\`, `\\`)
	escapedExe = strings.ReplaceAll(escapedExe, `"`, `\"`)
	cmd := exec.Command("osascript", "-e",
		fmt.Sprintf(`do shell script "%s meshd install" with administrator privileges`, escapedExe))
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
