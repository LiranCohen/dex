package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/WebP2P/dexnet/client/local"
	"github.com/lirancohen/dex/internal/daemon"
	"github.com/lirancohen/dex/internal/mesh"
	"github.com/lirancohen/dex/internal/meshd"
)

func runClientStart(args []string) error {
	fs := flag.NewFlagSet("client start", flag.ExitOnError)
	dataDirFlag := fs.String("data-dir", "", "Data directory (default: ~/.dex)")
	foregroundFlag := fs.Bool("foreground", true, "Run in foreground (default: true)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dex client start [options]\n\n")
		fmt.Fprintf(os.Stderr, "Start the mesh client and connect to the mesh network.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Determine data directory
	dataDir := *dataDirFlag
	if dataDir == "" {
		dataDir = os.Getenv("DEX_CLIENT_DATA_DIR")
	}
	if dataDir == "" {
		dataDir = DefaultClientDataDir()
	}

	// Load config
	configPath := filepath.Join(dataDir, "config.json")
	config, err := LoadClientConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not enrolled. Run 'dex client enroll' first")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if the mesh daemon is running — if so, use it instead of tsnet
	if meshd.IsRunning() {
		return runClientViaDaemon(config)
	}

	// No daemon running — fall back to userspace tsnet
	fmt.Println("Tip: Install the mesh daemon for full OS-level connectivity:")
	fmt.Println("  sudo dex meshd install")
	fmt.Println()

	// Handle background mode
	pidFile := daemon.NewPIDFile(dataDir, "dex-client")
	if !*foregroundFlag {
		// Check if already running
		if pidFile.IsRunning() {
			return fmt.Errorf("client already running (PID file: %s)", pidFile.Path)
		}

		// Fork to background
		isParent, err := daemon.Daemonize([]string{"client", "start", "--foreground", "--data-dir", dataDir})
		if err != nil {
			return fmt.Errorf("failed to daemonize: %w", err)
		}
		if isParent {
			fmt.Println("Client started in background")
			return nil
		}
		// Child continues below
	}

	// Write PID file (for both foreground and background modes)
	if err := pidFile.Write(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	defer func() { _ = pidFile.Remove() }()

	fmt.Println("Dex Client - Connecting to mesh network (userspace mode)")
	fmt.Printf("  Namespace: %s\n", config.Namespace)
	fmt.Printf("  Hostname:  %s\n", config.Hostname)
	fmt.Printf("  Control:   %s\n", config.Mesh.ControlURL)
	fmt.Println()

	// Get public domain from config, with fallback
	publicDomain := config.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}

	// Create mesh client with minimal configuration (no tunnel)
	meshConfig := mesh.Config{
		Enabled:      true,
		Hostname:     config.Hostname,
		StateDir:     filepath.Join(dataDir, "mesh"),
		ControlURL:   config.Mesh.ControlURL,
		IsHQ:         false, // Client is not an HQ
		PublicDomain: publicDomain,
	}

	meshClient := mesh.NewClient(meshConfig)
	meshClient.SetLogf(func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	})

	// Start mesh client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := meshClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start mesh client: %w", err)
	}

	fmt.Println("Connected to mesh network")

	// Wait for mesh IP
	var meshIP string
	for i := 0; i < 30; i++ {
		meshIP = meshClient.MeshIP()
		if meshIP != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if meshIP != "" {
		fmt.Printf("Mesh IP: %s\n", meshIP)
	}

	// Wait for interrupt signal
	fmt.Println()
	fmt.Println("Press Ctrl+C to disconnect")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println()
	fmt.Println("Shutting down...")

	if err := meshClient.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stop mesh client: %v\n", err)
	}

	fmt.Println("Disconnected from mesh network")
	return nil
}

// runClientViaDaemon connects to the running mesh daemon via its LocalAPI
// socket and monitors the mesh state. The daemon handles TUN device, OS routes,
// and DNS — so all we need to do here is report status and wait.
func runClientViaDaemon(config *ClientConfig) error {
	fmt.Println("Dex Client - Using mesh daemon for OS-level connectivity")
	fmt.Printf("  Namespace: %s\n", config.Namespace)
	fmt.Printf("  Hostname:  %s\n", config.Hostname)
	fmt.Printf("  Daemon:    %s\n", meshd.SocketPath)
	fmt.Println()

	lc := local.Client{
		Socket:        meshd.SocketPath,
		UseSocketOnly: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get initial status from the daemon
	status, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get daemon status: %w", err)
	}

	fmt.Printf("  State:     %s\n", status.BackendState)

	if len(status.TailscaleIPs) > 0 {
		fmt.Printf("  Mesh IP:   %s\n", status.TailscaleIPs[0])
	}
	if status.Self != nil && status.Self.DNSName != "" {
		fmt.Printf("  DNS Name:  %s\n", status.Self.DNSName)
	}

	// If not running yet, wait for it to come up
	if status.BackendState != "Running" {
		fmt.Printf("\n  Waiting for daemon to connect...\n")
		for i := 0; i < 60; i++ {
			time.Sleep(time.Second)
			status, err = lc.StatusWithoutPeers(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: lost connection to daemon: %v\n", err)
				break
			}
			if status.BackendState == "Running" {
				if len(status.TailscaleIPs) > 0 {
					fmt.Printf("  Mesh IP:   %s\n", status.TailscaleIPs[0])
				}
				break
			}
		}
	}

	fmt.Println()
	fmt.Println("Mesh daemon is handling connectivity. Browsers and apps can reach mesh nodes.")
	fmt.Println("Press Ctrl+C to exit (daemon continues running).")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println()
	fmt.Println("Client detached. Mesh daemon continues running in the background.")
	fmt.Println("To stop the daemon: sudo dex meshd uninstall")
	return nil
}
