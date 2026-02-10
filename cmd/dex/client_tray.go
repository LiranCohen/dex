//go:build !notray

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/WebP2P/dexnet/client/local"
	"github.com/lirancohen/dex/internal/mesh"
	"github.com/lirancohen/dex/internal/meshd"
)

type trayState int

const (
	trayStateUnauthenticated trayState = iota // No config — show Sign Up / Sign In
	trayStateAuthenticating                   // Browser auth in progress
	trayStateEnrolling                        // Auto-enrolling with Central
	trayStateDisconnected                     // Enrolled but not connected
	trayStateConnecting                       // Mesh connection in progress
	trayStateConnected                        // On the mesh
	trayStateError                            // Something went wrong
)

type clientTray struct {
	mu         sync.Mutex
	state      trayState
	meshIP     string
	hqURL      string
	errorMsg   string
	meshClient *mesh.Client
	cancel     context.CancelFunc
	stopping   bool
	dataDir    string
	centralURL string
	authToken  string

	// Callback server for app-first auth flow
	callbackSrv *callbackServer

	// Menu items
	mStatus     *systray.MenuItem
	mSignUp     *systray.MenuItem
	mSignIn     *systray.MenuItem
	mConnect    *systray.MenuItem
	mDisconnect *systray.MenuItem
	mAddHQ      *systray.MenuItem
	mOpenHQ     *systray.MenuItem
	mDashboard  *systray.MenuItem
	mQuit       *systray.MenuItem
}

// runClientTray runs the client with a system tray icon
func runClientTray(args []string) error {
	dataDir := DefaultClientDataDir()

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	tray := &clientTray{
		dataDir:    dataDir,
		centralURL: DefaultCentralURL,
	}

	configPath := filepath.Join(dataDir, "config.json")
	config, err := LoadClientConfig(configPath)
	if err != nil {
		// No config — start in unauthenticated state
		tray.state = trayStateUnauthenticated
		log.Printf("No client config: %v", err)
	} else {
		tray.state = trayStateDisconnected

		// Use domain from config, with fallback for backwards compatibility
		publicDomain := config.Domains.Public
		if publicDomain == "" {
			publicDomain = "enbox.id"
		}
		tray.hqURL = fmt.Sprintf("https://hq.%s.%s", config.Namespace, publicDomain)

		if config.CentralURL != "" {
			tray.centralURL = config.CentralURL
		}
		if config.AuthToken != "" {
			tray.authToken = config.AuthToken
		}
	}

	systray.Run(tray.onReady, tray.onExit)
	return nil
}

func (t *clientTray) onReady() {
	t.mu.Lock()
	initialState := t.state
	t.mu.Unlock()

	// Set initial icon
	t.setIcon(initialState)
	systray.SetTitle("Dex")

	// Create menu items — all of them, then show/hide based on state
	t.mStatus = systray.AddMenuItem("Status: Initializing", "Current status")
	t.mStatus.Disable()

	systray.AddSeparator()

	t.mSignUp = systray.AddMenuItem("Sign Up", "Create a new Dex account")
	t.mSignIn = systray.AddMenuItem("Sign In", "Sign in to an existing account")
	t.mConnect = systray.AddMenuItem("Connect", "Connect to mesh network")
	t.mDisconnect = systray.AddMenuItem("Disconnect", "Disconnect from mesh network")

	systray.AddSeparator()

	t.mAddHQ = systray.AddMenuItem("Add HQ...", "Set up your HQ server")
	t.mOpenHQ = systray.AddMenuItem("Open HQ", "Open HQ in browser")
	t.mDashboard = systray.AddMenuItem("Open Dashboard", "Open Dex dashboard in browser")

	systray.AddSeparator()

	t.mQuit = systray.AddMenuItem("Quit", "Quit Dex Client")

	// Apply initial state to UI
	t.updateUI()

	// Handle menu clicks
	go t.handleClicks()

	// Auto-connect if already enrolled
	if initialState == trayStateDisconnected {
		go t.connect()
	}
}

func (t *clientTray) onExit() {
	t.disconnect()
	if t.callbackSrv != nil {
		t.callbackSrv.close()
	}
}

func (t *clientTray) handleClicks() {
	for {
		select {
		case <-t.mSignUp.ClickedCh:
			go t.startBrowserAuth("signup")
		case <-t.mSignIn.ClickedCh:
			go t.startBrowserAuth("login")
		case <-t.mConnect.ClickedCh:
			go t.connect()
		case <-t.mDisconnect.ClickedCh:
			go t.disconnect()
		case <-t.mAddHQ.ClickedCh:
			t.openAddHQ()
		case <-t.mOpenHQ.ClickedCh:
			t.openHQ()
		case <-t.mDashboard.ClickedCh:
			t.openDashboard()
		case <-t.mQuit.ClickedCh:
			t.quit()
			return
		}
	}
}

// startBrowserAuth launches the browser auth flow.
func (t *clientTray) startBrowserAuth(mode string) {
	t.mu.Lock()
	if t.state != trayStateUnauthenticated && t.state != trayStateError {
		t.mu.Unlock()
		return
	}
	t.state = trayStateAuthenticating
	t.errorMsg = ""
	t.mu.Unlock()
	t.updateUI()

	// Start callback server
	srv, err := newCallbackServer()
	if err != nil {
		t.setError(fmt.Sprintf("Failed to start auth server: %v", err))
		return
	}
	t.mu.Lock()
	t.callbackSrv = srv
	t.mu.Unlock()

	// Open browser to Central login/signup page with callback params
	loginURL := fmt.Sprintf("%s/login?callback_port=%d&state=%s",
		t.centralURL, srv.port, srv.state)
	openBrowser(loginURL)

	// Wait for callback (timeout after 10 minutes)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	callback, err := srv.waitForCallback(ctx)
	srv.close()
	t.mu.Lock()
	t.callbackSrv = nil
	t.mu.Unlock()

	if err != nil {
		t.setError("Authentication timed out or was cancelled")
		return
	}

	// Exchange auth code for JWT
	t.mu.Lock()
	t.state = trayStateEnrolling
	t.mu.Unlock()
	t.updateUI()

	exchangeResp, err := exchangeAuthCode(t.centralURL, callback.Code)
	if err != nil {
		t.setError(fmt.Sprintf("Auth exchange failed: %v", err))
		return
	}

	// Auto-enroll
	config, err := autoEnroll(t.centralURL, exchangeResp.Token, exchangeResp.Namespace, t.dataDir)
	if err != nil {
		t.setError(fmt.Sprintf("Enrollment failed: %v", err))
		return
	}

	// Update tray state with new config
	publicDomain := config.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}
	t.mu.Lock()
	t.hqURL = fmt.Sprintf("https://hq.%s.%s", config.Namespace, publicDomain)
	t.authToken = exchangeResp.Token
	t.mu.Unlock()

	// Install meshd (will prompt for sudo via osascript on macOS)
	installMeshdIfNeeded()

	// Connect to mesh
	t.mu.Lock()
	t.state = trayStateDisconnected
	t.mu.Unlock()
	t.connect()
}

func (t *clientTray) connect() {
	t.mu.Lock()
	if t.state == trayStateConnecting || t.state == trayStateConnected {
		t.mu.Unlock()
		return
	}
	t.state = trayStateConnecting
	t.errorMsg = ""
	t.mu.Unlock()

	t.updateUI()

	// Check if the mesh daemon is running — if so, use it
	if meshd.IsRunning() {
		t.connectViaDaemon()
		return
	}

	// If the daemon is installed but not yet running, wait for it
	if meshd.IsInstalled() {
		log.Printf("Mesh daemon is installed but not yet running, waiting for socket...")
		if err := meshd.WaitForSocket(meshd.SocketPath, 30*time.Second); err != nil {
			log.Printf("Timed out waiting for mesh daemon: %v", err)
			t.setError("Mesh daemon not responding — check /var/log/dex-meshd.log")
			return
		}
		t.connectViaDaemon()
		return
	}

	// No daemon installed — fall back to userspace tsnet
	t.connectViaTsnet()
}

// connectViaDaemon connects to the running mesh daemon via LocalAPI.
func (t *clientTray) connectViaDaemon() {
	lc := local.Client{
		Socket:        meshd.SocketPath,
		UseSocketOnly: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()

	// Poll for daemon status
	var meshIP string
	for i := 0; i < 30; i++ {
		status, err := lc.StatusWithoutPeers(ctx)
		if err != nil {
			log.Printf("Lost connection to daemon: %v", err)
			break
		}
		if status.BackendState == "Running" && len(status.TailscaleIPs) > 0 {
			meshIP = status.TailscaleIPs[0].String()
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}

	if meshIP == "" {
		t.setError("Daemon connected but no mesh IP assigned")
		cancel()
		return
	}

	t.mu.Lock()
	t.state = trayStateConnected
	t.meshIP = meshIP
	t.mu.Unlock()

	t.updateUI()

	// Wait for context cancellation
	<-ctx.Done()
}

// connectViaTsnet connects using an in-process tsnet server (userspace mode).
func (t *clientTray) connectViaTsnet() {
	configPath := filepath.Join(t.dataDir, "config.json")
	config, err := LoadClientConfig(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		t.setError("Not enrolled — run 'dex client enroll'")
		return
	}

	// Get public domain from config, with fallback
	publicDomain := config.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}

	meshConfig := mesh.Config{
		Enabled:      true,
		Hostname:     config.Hostname,
		StateDir:     filepath.Join(t.dataDir, "mesh"),
		ControlURL:   config.Mesh.ControlURL,
		IsHQ:         false, // Client is not HQ
		PublicDomain: publicDomain,
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.cancel = cancel
	t.meshClient = mesh.NewClient(meshConfig)
	t.mu.Unlock()

	if err := t.meshClient.Start(ctx); err != nil {
		log.Printf("Failed to start mesh client: %v", err)
		cancel()
		t.mu.Lock()
		t.meshClient = nil
		t.cancel = nil
		t.mu.Unlock()
		t.setError(fmt.Sprintf("Mesh failed: %v", err))
		return
	}

	// Wait for mesh IP
	var meshIP string
	for i := 0; i < 30; i++ {
		meshIP = t.meshClient.MeshIP()
		if meshIP != "" {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}

	if meshIP == "" {
		t.setError("Connected but no mesh IP assigned")
		return
	}

	t.mu.Lock()
	t.state = trayStateConnected
	t.meshIP = meshIP
	t.mu.Unlock()

	t.updateUI()

	// Wait for context cancellation
	<-ctx.Done()
}

func (t *clientTray) disconnect() {
	t.mu.Lock()
	if t.stopping {
		t.mu.Unlock()
		return
	}
	t.stopping = true

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	if t.meshClient != nil {
		if err := t.meshClient.Stop(); err != nil {
			log.Printf("Warning: failed to stop mesh client: %v", err)
		}
		t.meshClient = nil
	}

	t.state = trayStateDisconnected
	t.meshIP = ""
	t.errorMsg = ""
	t.stopping = false
	t.mu.Unlock()

	t.updateUI()
}

func (t *clientTray) quit() {
	t.disconnect()

	// On macOS, stop the mesh daemon if it's installed.
	if runtime.GOOS == "darwin" && meshd.IsInstalled() {
		log.Printf("Stopping mesh daemon via privilege prompt...")
		cmd := exec.Command("osascript", "-e",
			`do shell script "launchctl unload /Library/LaunchDaemons/com.dex.meshd.plist" with administrator privileges`)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to stop mesh daemon (user may have cancelled): %v", err)
		}
	}

	systray.Quit()
}

// setError transitions the tray to error state with a visible message.
func (t *clientTray) setError(msg string) {
	log.Printf("Tray error: %s", msg)
	t.mu.Lock()
	t.state = trayStateError
	t.errorMsg = msg
	t.mu.Unlock()
	t.updateUI()
}

func (t *clientTray) updateUI() {
	t.mu.Lock()
	state := t.state
	meshIP := t.meshIP
	errorMsg := t.errorMsg
	t.mu.Unlock()

	t.setIcon(state)

	// Hide all optional items first, then show what's needed
	t.mSignUp.Hide()
	t.mSignIn.Hide()
	t.mConnect.Hide()
	t.mDisconnect.Hide()
	t.mAddHQ.Hide()
	t.mOpenHQ.Hide()
	t.mDashboard.Hide()

	switch state {
	case trayStateUnauthenticated:
		systray.SetTooltip("Dex Client - Sign in to get started")
		t.mStatus.SetTitle("Not signed in")
		t.mSignUp.Show()
		t.mSignIn.Show()

	case trayStateAuthenticating:
		systray.SetTooltip("Dex Client - Waiting for browser...")
		t.mStatus.SetTitle("Waiting for browser sign-in...")

	case trayStateEnrolling:
		systray.SetTooltip("Dex Client - Setting up...")
		t.mStatus.SetTitle("Setting up your device...")

	case trayStateDisconnected:
		systray.SetTooltip("Dex Client - Disconnected")
		t.mStatus.SetTitle("Status: Disconnected")
		t.mConnect.Show()
		t.mConnect.Enable()
		t.mDashboard.Show()

	case trayStateConnecting:
		systray.SetTooltip("Dex Client - Connecting...")
		t.mStatus.SetTitle("Status: Connecting...")
		t.mDisconnect.Show()
		t.mDisconnect.Enable()

	case trayStateConnected:
		tooltip := "Dex Client - Connected"
		statusText := "Status: Connected"
		if meshIP != "" {
			tooltip = fmt.Sprintf("Dex Client - %s", meshIP)
			statusText = fmt.Sprintf("Status: Connected (%s)", meshIP)
		}
		systray.SetTooltip(tooltip)
		t.mStatus.SetTitle(statusText)
		t.mDisconnect.Show()
		t.mDisconnect.Enable()
		t.mAddHQ.Show()
		t.mOpenHQ.Show()
		t.mOpenHQ.Enable()
		t.mDashboard.Show()

	case trayStateError:
		statusText := "Error"
		if errorMsg != "" {
			statusText = fmt.Sprintf("Error: %s", errorMsg)
		}
		systray.SetTooltip(fmt.Sprintf("Dex Client - %s", statusText))
		t.mStatus.SetTitle(statusText)
		// Show appropriate retry options based on whether we have config
		configPath := filepath.Join(t.dataDir, "config.json")
		if _, err := os.Stat(configPath); err == nil {
			// Have config — show connect
			t.mConnect.Show()
			t.mConnect.Enable()
		} else {
			// No config — show sign up/in
			t.mSignUp.Show()
			t.mSignIn.Show()
		}
	}
}

func (t *clientTray) openHQ() {
	t.mu.Lock()
	url := t.hqURL
	t.mu.Unlock()

	if url == "" {
		return
	}
	openBrowser(url)
}

func (t *clientTray) openDashboard() {
	openBrowser(t.centralURL + "/dashboard")
}

func (t *clientTray) openAddHQ() {
	openBrowser(t.centralURL + "/onboarding")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		log.Printf("Cannot open browser on %s", runtime.GOOS)
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

func (t *clientTray) setIcon(state trayState) {
	var icon []byte
	switch state {
	case trayStateUnauthenticated:
		icon = generateIcon(color.RGBA{R: 150, G: 150, B: 150, A: 255}) // Gray
	case trayStateAuthenticating, trayStateEnrolling:
		icon = generateIcon(color.RGBA{R: 100, G: 150, B: 230, A: 255}) // Blue
	case trayStateDisconnected:
		icon = generateIcon(color.RGBA{R: 180, G: 50, B: 50, A: 255}) // Red
	case trayStateConnecting:
		icon = generateIcon(color.RGBA{R: 150, G: 150, B: 150, A: 255}) // Gray
	case trayStateConnected:
		icon = generateIcon(color.RGBA{R: 50, G: 180, B: 50, A: 255}) // Green
	case trayStateError:
		icon = generateIcon(color.RGBA{R: 230, G: 160, B: 30, A: 255}) // Orange/Amber
	}
	systray.SetIcon(icon)
}

// generateIcon creates a simple 22x22 PNG icon with the given color
func generateIcon(c color.Color) []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw a filled circle
	cx, cy := size/2, size/2
	radius := size/2 - 2

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}
