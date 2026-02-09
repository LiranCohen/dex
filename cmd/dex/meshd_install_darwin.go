//go:build darwin

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lirancohen/dex/internal/meshd"
)

const (
	daemonPlistPath = "/Library/LaunchDaemons/com.dex.meshd.plist"
	daemonBinPath   = "/usr/local/bin/dex"
	daemonService   = "com.dex.meshd"
)

// daemonPlist is the launchd plist for dex-meshd.
// It runs "dex meshd" as root at load time.
var daemonPlist = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>

  <key>Label</key>
  <string>%s</string>

  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>meshd</string>
  </array>

  <key>RunAtLoad</key>
  <true/>

  <key>StandardErrorPath</key>
  <string>/var/log/dex-meshd.log</string>

  <key>StandardOutPath</key>
  <string>/var/log/dex-meshd.log</string>

</dict>
</plist>
`, daemonService, daemonBinPath)

func runMeshdInstall(args []string) error {
	if len(args) > 0 {
		return errors.New("install takes no arguments")
	}

	if os.Getuid() != 0 {
		return fmt.Errorf("install requires root; use: sudo dex meshd install")
	}

	fmt.Println("Installing dex mesh daemon...")

	// Uninstall any existing daemon first (best effort)
	_ = uninstallDaemon()

	// Copy binary to system path if needed
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	same, err := sameFilePath(exe, daemonBinPath)
	if err != nil {
		return err
	}
	if !same {
		fmt.Printf("  Copying %s -> %s\n", exe, daemonBinPath)
		if err := copyBinaryFile(exe, daemonBinPath); err != nil {
			return fmt.Errorf("copying binary: %w", err)
		}
	} else {
		fmt.Printf("  Binary already at %s\n", daemonBinPath)
	}

	// Ensure socket directory exists
	socketDir := meshd.SocketDir()
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("creating socket dir: %w", err)
	}

	// Write plist
	fmt.Printf("  Writing plist: %s\n", daemonPlistPath)
	if err := os.WriteFile(daemonPlistPath, []byte(daemonPlist), 0644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	// Load and start
	fmt.Println("  Loading daemon...")
	if out, err := exec.Command("launchctl", "load", daemonPlistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %v\n%s", err, out)
	}

	fmt.Println("  Starting daemon...")
	if out, err := exec.Command("launchctl", "start", daemonService).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl start: %v\n%s", err, out)
	}

	fmt.Println("Mesh daemon installed and started.")
	fmt.Printf("  Socket: %s\n", meshd.SocketPath)
	fmt.Printf("  Logs:   /var/log/dex-meshd.log\n")
	return nil
}

func runMeshdUninstall(args []string) error {
	if len(args) > 0 {
		return errors.New("uninstall takes no arguments")
	}

	if os.Getuid() != 0 {
		return fmt.Errorf("uninstall requires root; use: sudo dex meshd uninstall")
	}

	fmt.Println("Uninstalling dex mesh daemon...")
	if err := uninstallDaemon(); err != nil {
		return err
	}
	fmt.Println("Mesh daemon uninstalled.")
	return nil
}

func uninstallDaemon() error {
	// Check if running
	if _, err := exec.Command("launchctl", "list", daemonService).Output(); err == nil {
		// Stop
		if out, err := exec.Command("launchctl", "stop", daemonService).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "  launchctl stop: %v\n%s", err, out)
		}
		// Unload
		if out, err := exec.Command("launchctl", "unload", daemonPlistPath).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "  launchctl unload: %v\n%s", err, out)
		}
	}

	// Remove plist
	if err := os.Remove(daemonPlistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing plist: %w", err)
	}

	// Remove socket
	_ = os.Remove(meshd.SocketPath)

	// Don't remove the binary — it's the main dex binary used for everything
	return nil
}

func copyBinaryFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmpBin := dst + ".tmp"
	f, err := os.Create(tmpBin)
	if err != nil {
		return err
	}
	srcf, err := os.Open(src)
	if err != nil {
		_ = f.Close()
		return err
	}
	_, err = io.Copy(f, srcf)
	_ = srcf.Close()
	if err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpBin, 0755); err != nil {
		return err
	}
	return os.Rename(tmpBin, dst)
}

func sameFilePath(a, b string) (bool, error) {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return ra == rb, nil
}
