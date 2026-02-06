package forgejo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// ForgejoVersion is the pinned Forgejo release version.
	ForgejoVersion = "9.0.3"

	// forgejoMirror is the primary download source (Dex-controlled mirror).
	forgejoMirror = "https://dl.enbox.id/forgejo"

	// forgejoUpstream is the fallback download source (Codeberg releases).
	forgejoUpstream = "https://codeberg.org/forgejo/forgejo/releases/download"
)

// Checksums for supported platforms, keyed by "os-arch".
// These must be updated whenever ForgejoVersion changes.
// Checksums from: https://codeberg.org/forgejo/forgejo/releases/tag/v9.0.3
var forgejoChecksums = map[string]string{
	"linux-amd64": "51b3a6c0b397c66bd4adfc482b7d582b1b60a53f3205486ada9e6357afb03ebb",
	"linux-arm64": "295677cffa6fab4535b626686ddea1e5eb5ca1a964c84f04167a7d381efe2aa0",
}

// ensureBinary downloads the Forgejo binary if it doesn't exist or
// if the existing binary fails checksum verification.
func (m *Manager) ensureBinary(ctx context.Context) error {
	binaryPath := m.config.GetBinaryPath()
	binDir := filepath.Dir(binaryPath)

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Check if binary exists and has correct checksum
	if fileExists(binaryPath) {
		expectedChecksum := platformChecksum()
		if expectedChecksum == "" || checksumMatches(binaryPath, expectedChecksum) {
			return nil // Binary exists and is valid (or no checksum to verify)
		}
		fmt.Println("Forgejo binary checksum mismatch, re-downloading...")
	}

	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	binaryName := fmt.Sprintf("forgejo-%s-%s", ForgejoVersion, platform)

	// Build download URLs: mirror first, upstream fallback
	urls := []string{
		fmt.Sprintf("%s/v%s/%s", forgejoMirror, ForgejoVersion, binaryName),
		fmt.Sprintf("%s/v%s/%s", forgejoUpstream, ForgejoVersion, binaryName),
	}

	fmt.Printf("Downloading Forgejo v%s for %s...\n", ForgejoVersion, platform)

	// Try each URL
	var lastErr error
	for _, url := range urls {
		if err := downloadFile(ctx, url, binaryPath); err != nil {
			lastErr = err
			fmt.Printf("  Failed from %s: %v\n", url, err)
			continue
		}

		// Verify checksum if available
		expectedChecksum := platformChecksum()
		if expectedChecksum != "" && !checksumMatches(binaryPath, expectedChecksum) {
			lastErr = fmt.Errorf("checksum mismatch after download from %s", url)
			_ = os.Remove(binaryPath)
			fmt.Printf("  Checksum mismatch from %s\n", url)
			continue
		}

		// Make executable
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make forgejo executable: %w", err)
		}

		fmt.Printf("Forgejo v%s downloaded successfully\n", ForgejoVersion)
		return nil
	}

	return fmt.Errorf("failed to download forgejo from all sources: %w", lastErr)
}

func platformChecksum() string {
	key := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	return forgejoChecksums[key]
}

func downloadFile(ctx context.Context, url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// Download to temp file first, then rename (atomic)
	tmpFile := dest + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmpFile) // Clean up if we don't rename
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("download interrupted: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	return os.Rename(tmpFile, dest)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func checksumMatches(path, expected string) bool {
	if expected == "" {
		fmt.Printf("Warning: no checksum configured for %s, skipping verification\n", filepath.Base(path))
		return true
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}

	actual := hex.EncodeToString(h.Sum(nil))
	return actual == expected
}
