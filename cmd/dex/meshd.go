package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lirancohen/dex/internal/meshd"
)

func printMeshdUsage() {
	fmt.Fprintf(os.Stderr, "Dex Mesh Daemon - Full mesh networking with TUN device\n\n")
	fmt.Fprintf(os.Stderr, "Usage: dex meshd [command] [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  (default)   Run the mesh daemon (requires root)\n")
	fmt.Fprintf(os.Stderr, "  install     Install as a system daemon (LaunchDaemon on macOS)\n")
	fmt.Fprintf(os.Stderr, "  uninstall   Remove the system daemon\n")
	fmt.Fprintf(os.Stderr, "  status      Check if the daemon is running\n")
	fmt.Fprintf(os.Stderr, "  help        Show this help message\n")
	fmt.Fprintf(os.Stderr, "\nThe mesh daemon creates a TUN device, OS routes, and DNS resolver\n")
	fmt.Fprintf(os.Stderr, "configuration so that browsers and all applications can reach\n")
	fmt.Fprintf(os.Stderr, "mesh nodes directly.\n")
}

func runMeshd(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			return runMeshdInstall(args[1:])
		case "uninstall":
			return runMeshdUninstall(args[1:])
		case "status":
			return runMeshdStatus(args[1:])
		case "help", "-h", "--help":
			printMeshdUsage()
			return nil
		}
	}

	// Default: run the daemon
	return runMeshdRun(args)
}

func runMeshdRun(args []string) error {
	fs := flag.NewFlagSet("meshd", flag.ExitOnError)
	socketPath := fs.String("socket", meshd.SocketPath, "LocalAPI socket path")
	statePath := fs.String("state", "", "State file path (default: auto-detect from enrollment)")
	stateDir := fs.String("state-dir", "", "State directory (default: auto-detect from enrollment)")
	tunName := fs.String("tun", "", "TUN device name (default: utun on macOS, dex0 on Linux)")
	verbose := fs.Int("verbose", 0, "Log verbosity level")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dex meshd [options]\n\n")
		fmt.Fprintf(os.Stderr, "Run the mesh daemon. Requires root for TUN device creation.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := meshd.DefaultConfig()

	if *socketPath != "" {
		cfg.SocketPath = *socketPath
	}
	if *tunName != "" {
		cfg.TunName = *tunName
	}
	cfg.Verbose = *verbose

	// Auto-detect state paths from enrollment config.
	// HQ mode: /opt/dex/mesh/
	// Client mode: ~/.dex/mesh/
	if *stateDir == "" && *statePath == "" {
		// Try HQ config first
		hqConfig := "/opt/dex/config.json"
		if _, err := os.Stat(hqConfig); err == nil {
			cfg.StateDir = "/opt/dex/mesh"
			cfg.StatePath = filepath.Join(cfg.StateDir, "tailscaled.state")
			fmt.Fprintf(os.Stderr, "dex-meshd: using HQ state: %s\n", cfg.StateDir)
		} else {
			// Try client config
			clientDataDir := DefaultClientDataDir()
			clientConfig := filepath.Join(clientDataDir, "config.json")
			if _, err := os.Stat(clientConfig); err == nil {
				cfg.StateDir = filepath.Join(clientDataDir, "mesh")
				cfg.StatePath = filepath.Join(cfg.StateDir, "tailscaled.state")
				fmt.Fprintf(os.Stderr, "dex-meshd: using client state: %s\n", cfg.StateDir)
			} else {
				return fmt.Errorf("no enrollment found. Run 'dex enroll' or 'dex client enroll' first.\nLooked in: %s, %s", hqConfig, clientConfig)
			}
		}
	} else {
		if *stateDir != "" {
			cfg.StateDir = *stateDir
		}
		if *statePath != "" {
			cfg.StatePath = *statePath
		} else {
			cfg.StatePath = filepath.Join(cfg.StateDir, "tailscaled.state")
		}
	}

	fmt.Fprintf(os.Stderr, "dex-meshd: starting (socket=%s, state=%s, tun=%s)\n",
		cfg.SocketPath, cfg.StatePath, cfg.TunName)

	return meshd.Run(context.Background(), cfg)
}

func runMeshdStatus(_ []string) error {
	if meshd.IsRunning() {
		fmt.Println("dex-meshd is running")
		fmt.Printf("  Socket: %s\n", meshd.SocketPath)
		return nil
	}
	fmt.Println("dex-meshd is not running")
	return nil
}
