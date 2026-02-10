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
	trayStateDisconnected trayState = iota
	trayStateConnecting
	trayStateConnected
)

type clientTray struct {
	mu         sync.Mutex
	state      trayState
	meshIP     string
	hqURL      string
	meshClient *mesh.Client
	cancel     context.CancelFunc
	stopping   bool
	dataDir    string

	// Menu items
	mStatus     *systray.MenuItem
	mConnect    *systray.MenuItem
	mDisconnect *systray.MenuItem
	mOpenHQ     *systray.MenuItem
	mQuit       *systray.MenuItem
}

// runClientTray runs the client with a system tray icon
func runClientTray(args []string) error {
	dataDir := DefaultClientDataDir()
	configPath := filepath.Join(dataDir, "config.json")

	config, err := LoadClientConfig(configPath)
	if err != nil {
		return fmt.Errorf("no client configuration found. Run 'dex client enroll' first: %w", err)
	}

	// Use domain from config, with fallback for backwards compatibility
	publicDomain := config.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}

	tray := &clientTray{
		state:   trayStateDisconnected,
		hqURL:   fmt.Sprintf("https://hq.%s.%s", config.Namespace, publicDomain),
		dataDir: dataDir,
	}

	systray.Run(tray.onReady, tray.onExit)
	return nil
}

func (t *clientTray) onReady() {
	// Set initial icon
	t.setIcon(trayStateDisconnected)
	systray.SetTitle("Dex")
	systray.SetTooltip("Dex Client - Disconnected")

	// Create menu items
	t.mStatus = systray.AddMenuItem("Status: Disconnected", "Current connection status")
	t.mStatus.Disable()

	systray.AddSeparator()

	t.mConnect = systray.AddMenuItem("Connect", "Connect to mesh network")
	t.mDisconnect = systray.AddMenuItem("Disconnect", "Disconnect from mesh network")
	t.mDisconnect.Hide()

	systray.AddSeparator()

	t.mOpenHQ = systray.AddMenuItem("Open HQ", "Open HQ in browser")
	t.mOpenHQ.Disable()

	systray.AddSeparator()

	t.mQuit = systray.AddMenuItem("Quit", "Quit Dex Client")

	// Handle menu clicks
	go t.handleClicks()

	// Auto-connect on start
	go t.connect()
}

func (t *clientTray) onExit() {
	t.disconnect()
}

func (t *clientTray) handleClicks() {
	for {
		select {
		case <-t.mConnect.ClickedCh:
			go t.connect()
		case <-t.mDisconnect.ClickedCh:
			go t.disconnect()
		case <-t.mOpenHQ.ClickedCh:
			t.openHQ()
		case <-t.mQuit.ClickedCh:
			t.disconnect()
			systray.Quit()
			return
		}
	}
}

func (t *clientTray) connect() {
	t.mu.Lock()
	if t.state != trayStateDisconnected {
		t.mu.Unlock()
		return
	}
	t.state = trayStateConnecting
	t.mu.Unlock()

	t.updateUI()

	// Check if the mesh daemon is running — if so, use it
	if meshd.IsRunning() {
		t.connectViaDaemon()
		return
	}

	// If the daemon is installed but not yet running, wait for it
	// instead of falling through to tsnet (which will fail on root-owned state files).
	if meshd.IsInstalled() {
		log.Printf("Mesh daemon is installed but not yet running, waiting for socket...")
		if err := meshd.WaitForSocket(meshd.SocketPath, 30*time.Second); err != nil {
			log.Printf("Timed out waiting for mesh daemon: %v", err)
			t.mu.Lock()
			t.state = trayStateDisconnected
			t.mu.Unlock()
			t.updateUI()
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
		t.mu.Lock()
		t.state = trayStateDisconnected
		t.mu.Unlock()
		t.updateUI()
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
		t.state = trayStateDisconnected
		t.meshClient = nil
		t.cancel = nil
		t.mu.Unlock()
		t.updateUI()
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
	t.stopping = false
	t.mu.Unlock()

	t.updateUI()
}

func (t *clientTray) updateUI() {
	t.mu.Lock()
	state := t.state
	meshIP := t.meshIP
	t.mu.Unlock()

	t.setIcon(state)

	switch state {
	case trayStateDisconnected:
		systray.SetTooltip("Dex Client - Disconnected")
		t.mStatus.SetTitle("Status: Disconnected")
		t.mConnect.Show()
		t.mConnect.Enable()
		t.mDisconnect.Hide()
		t.mOpenHQ.Disable()

	case trayStateConnecting:
		systray.SetTooltip("Dex Client - Connecting...")
		t.mStatus.SetTitle("Status: Connecting...")
		t.mConnect.Hide()
		t.mDisconnect.Show()
		t.mDisconnect.Enable()
		t.mOpenHQ.Disable()

	case trayStateConnected:
		tooltip := "Dex Client - Connected"
		statusText := "Status: Connected"
		if meshIP != "" {
			tooltip = fmt.Sprintf("Dex Client - %s", meshIP)
			statusText = fmt.Sprintf("Status: Connected (%s)", meshIP)
		}
		systray.SetTooltip(tooltip)
		t.mStatus.SetTitle(statusText)
		t.mConnect.Hide()
		t.mDisconnect.Show()
		t.mDisconnect.Enable()
		t.mOpenHQ.Enable()
	}
}

func (t *clientTray) openHQ() {
	t.mu.Lock()
	url := t.hqURL
	t.mu.Unlock()

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
	case trayStateDisconnected:
		icon = generateIcon(color.RGBA{R: 180, G: 50, B: 50, A: 255}) // Red
	case trayStateConnecting:
		icon = generateIcon(color.RGBA{R: 150, G: 150, B: 150, A: 255}) // Gray
	case trayStateConnected:
		icon = generateIcon(color.RGBA{R: 50, G: 180, B: 50, A: 255}) // Green
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
