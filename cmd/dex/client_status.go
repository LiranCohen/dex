package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lirancohen/dex/internal/daemon"
	"github.com/lirancohen/dex/internal/mesh"
)

func runClientStatus(args []string) error {
	fs := flag.NewFlagSet("client status", flag.ExitOnError)
	dataDirFlag := fs.String("data-dir", "", "Data directory (default: ~/.dex)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dex client status [options]\n\n")
		fmt.Fprintf(os.Stderr, "Show mesh client connection status.\n\n")
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
			fmt.Println("Status: Not enrolled")
			fmt.Println()
			fmt.Println("Run 'dex client enroll' to enroll this device")
			return nil
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Dex Client Status")
	fmt.Println("─────────────────")
	fmt.Printf("Namespace: %s\n", config.Namespace)
	fmt.Printf("Hostname:  %s\n", config.Hostname)
	fmt.Printf("Control:   %s\n", config.Mesh.ControlURL)
	fmt.Println()

	// Get public domain from config, with fallback
	publicDomain := config.Domains.Public
	if publicDomain == "" {
		publicDomain = "enbox.id"
	}

	// Try to connect briefly to get status
	meshConfig := mesh.Config{
		Enabled:      true,
		Hostname:     config.Hostname,
		StateDir:     filepath.Join(dataDir, "mesh"),
		ControlURL:   config.Mesh.ControlURL,
		IsHQ:         false,
		PublicDomain: publicDomain,
	}

	meshClient := mesh.NewClient(meshConfig)
	meshClient.SetLogf(func(format string, args ...any) {
		// Suppress logs during status check
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := meshClient.Start(ctx); err != nil {
		fmt.Println("Status: Offline (failed to connect)")
		fmt.Printf("Error:  %v\n", err)
		return nil
	}
	defer func() { _ = meshClient.Stop() }()

	// Wait briefly for IP assignment
	var meshIP string
	for i := 0; i < 10; i++ {
		meshIP = meshClient.MeshIP()
		if meshIP != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	status := meshClient.Status()

	if status.Connected {
		fmt.Println("Status: Connected")
		if meshIP != "" {
			fmt.Printf("Mesh IP: %s\n", meshIP)
		}
		if status.Online {
			fmt.Println("Online:  Yes")
		}
	} else {
		fmt.Println("Status: Disconnected")
	}

	// Show peers
	peers := meshClient.Peers()
	if len(peers) > 0 {
		fmt.Println()
		fmt.Println("Peers:")
		for _, p := range peers {
			onlineStr := "offline"
			if p.Online {
				onlineStr = "online"
			}
			directStr := ""
			if p.Direct {
				directStr = " (direct)"
			}
			fmt.Printf("  %s (%s) - %s%s\n", p.Hostname, p.MeshIP, onlineStr, directStr)
		}
	}

	return nil
}

func runClientStop(args []string) error {
	fs := flag.NewFlagSet("client stop", flag.ExitOnError)
	dataDirFlag := fs.String("data-dir", "", "Data directory (default: ~/.dex)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dex client stop [options]\n\n")
		fmt.Fprintf(os.Stderr, "Stop the running mesh client.\n\n")
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

	// Check PID file
	pidFile := daemon.NewPIDFile(dataDir, "dex-client")
	if !pidFile.IsRunning() {
		fmt.Println("Client is not running")
		return nil
	}

	// Read PID for display
	pid, _ := pidFile.Read()
	fmt.Printf("Stopping client (PID %d)...\n", pid)

	// Send SIGTERM
	if err := pidFile.StopProcess(); err != nil {
		return fmt.Errorf("failed to stop client: %w", err)
	}

	// Wait for process to exit (up to 10 seconds)
	for i := 0; i < 20; i++ {
		if !pidFile.IsRunning() {
			fmt.Println("Client stopped")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("Client may still be shutting down")
	return nil
}
