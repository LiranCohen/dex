package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"
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

	// Auto-detect state paths and control URL from enrollment config.
	// HQ mode: /opt/dex/config.json -> /opt/dex/mesh/
	// Client mode: ~/.dex/config.json -> ~/.dex/mesh/
	if *stateDir == "" && *statePath == "" {
		// Try HQ config first
		hqConfigPath := "/opt/dex/config.json"
		if hqCfg, err := LoadConfig(hqConfigPath); err == nil {
			cfg.StateDir = "/opt/dex/mesh"
			cfg.StatePath = filepath.Join(cfg.StateDir, "tailscaled.state")
			cfg.ControlURL = hqCfg.Mesh.ControlURL
			cfg.Hostname = hqCfg.Hostname
			if cfg.Hostname == "" {
				cfg.Hostname = "hq"
			}
			fmt.Fprintf(os.Stderr, "dex-meshd: using HQ enrollment (control=%s, hostname=%s)\n", cfg.ControlURL, cfg.Hostname)
		} else {
			// Try client config. When running as root (via sudo), resolve
			// the original user's home directory since enrollment state lives
			// in the user's ~/.dex/, not root's.
			clientDataDir := sudoAwareClientDataDir()
			clientConfigPath := filepath.Join(clientDataDir, "config.json")
			if clientCfg, err := LoadClientConfig(clientConfigPath); err == nil {
				cfg.StateDir = filepath.Join(clientDataDir, "mesh")
				cfg.StatePath = filepath.Join(cfg.StateDir, "tailscaled.state")
				cfg.ControlURL = clientCfg.Mesh.ControlURL
				cfg.Hostname = clientCfg.Hostname
				fmt.Fprintf(os.Stderr, "dex-meshd: using client enrollment (control=%s, hostname=%s)\n", cfg.ControlURL, cfg.Hostname)
			} else {
				return fmt.Errorf("no enrollment found. Run 'dex enroll' or 'dex client enroll' first.\nLooked in: %s, %s", hqConfigPath, clientConfigPath)
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

	// Validate that we have a control URL
	if cfg.ControlURL == "" {
		return fmt.Errorf("no control URL found in enrollment config; re-enroll with 'dex enroll' or 'dex client enroll'")
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

// sudoAwareClientDataDir returns the client data directory, resolving
// the original user's home when running as root via sudo. This is needed
// because the daemon requires root for TUN device creation, but the
// enrollment state lives in the original user's ~/.dex/ directory.
func sudoAwareClientDataDir() string {
	// If not root, use the normal default
	if os.Getuid() != 0 {
		return DefaultClientDataDir()
	}

	// Check SUDO_USER to find the original user
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return filepath.Join(u.HomeDir, ".dex")
		}
	}

	// Fallback: scan /Users/ on macOS for a .dex/config.json
	entries, err := os.ReadDir("/Users")
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "Shared" {
				continue
			}
			candidate := filepath.Join("/Users", e.Name(), ".dex")
			if _, err := os.Stat(filepath.Join(candidate, "config.json")); err == nil {
				return candidate
			}
		}
	}

	// Last resort: root's own home
	return DefaultClientDataDir()
}
