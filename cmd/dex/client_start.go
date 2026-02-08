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

	"github.com/lirancohen/dex/internal/mesh"
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

	fmt.Println("Dex Client - Connecting to mesh network")
	fmt.Printf("  Namespace: %s\n", config.Namespace)
	fmt.Printf("  Hostname:  %s\n", config.Hostname)
	fmt.Printf("  Control:   %s\n", config.Mesh.ControlURL)
	fmt.Println()

	// Create mesh client with minimal configuration (no tunnel)
	meshConfig := mesh.Config{
		Enabled:    true,
		Hostname:   config.Hostname,
		StateDir:   filepath.Join(dataDir, "mesh"),
		ControlURL: config.Mesh.ControlURL,
		IsHQ:       false, // Client is not an HQ
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

	if !*foregroundFlag {
		// TODO: Implement background mode with PID file
		fmt.Println("Background mode not yet implemented")
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
