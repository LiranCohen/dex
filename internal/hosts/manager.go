// Package hosts manages /etc/hosts entries for local loopback routing.
// This allows services on the same machine (like Forgejo and HQ) to
// communicate directly via localhost instead of going through the tunnel.
package hosts

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// hostsFile is the path to the system hosts file.
	hostsFile = "/etc/hosts"

	// markerStart marks the beginning of dex-managed entries.
	markerStart = "# BEGIN dex-managed entries - do not edit this block"

	// markerEnd marks the end of dex-managed entries.
	markerEnd = "# END dex-managed entries"

	// loopbackIP is the IP address for local routing.
	loopbackIP = "127.0.0.1"
)

// Manager handles /etc/hosts file modifications for local hostname routing.
type Manager struct {
	mu       sync.Mutex
	managed  map[string]bool // Currently managed hostnames
	disabled bool            // If true, don't modify hosts file
}

// NewManager creates a new hosts file manager.
func NewManager() *Manager {
	return &Manager{
		managed: make(map[string]bool),
	}
}

// SetDisabled disables hosts file modifications (useful for testing or non-root).
func (m *Manager) SetDisabled(disabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disabled = disabled
}

// SetHostnames updates the managed hostnames.
// This removes any previously managed hostnames and adds the new ones.
// Pass nil or empty slice to remove all managed entries.
func (m *Manager) SetHostnames(hostnames []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.disabled {
		return nil
	}

	// Build new set
	newManaged := make(map[string]bool)
	for _, h := range hostnames {
		if h != "" {
			newManaged[h] = true
		}
	}

	// Update the file
	if err := m.updateHostsFile(newManaged); err != nil {
		return err
	}

	m.managed = newManaged
	return nil
}

// Add adds a hostname to the managed set.
func (m *Manager) Add(hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.disabled || hostname == "" {
		return nil
	}

	if m.managed[hostname] {
		return nil // Already managed
	}

	newManaged := copyMap(m.managed)
	newManaged[hostname] = true

	if err := m.updateHostsFile(newManaged); err != nil {
		return err
	}

	m.managed = newManaged
	return nil
}

// Remove removes a hostname from the managed set.
func (m *Manager) Remove(hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.disabled || hostname == "" {
		return nil
	}

	if !m.managed[hostname] {
		return nil // Not managed
	}

	newManaged := copyMap(m.managed)
	delete(newManaged, hostname)

	if err := m.updateHostsFile(newManaged); err != nil {
		return err
	}

	m.managed = newManaged
	return nil
}

// Cleanup removes all dex-managed entries from the hosts file.
// This should be called on shutdown or startup to clean stale entries.
func (m *Manager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.disabled {
		return nil
	}

	// Remove all managed entries
	if err := m.updateHostsFile(nil); err != nil {
		return err
	}

	m.managed = make(map[string]bool)
	return nil
}

// ManagedHostnames returns the list of currently managed hostnames.
func (m *Manager) ManagedHostnames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]string, 0, len(m.managed))
	for h := range m.managed {
		result = append(result, h)
	}
	return result
}

// updateHostsFile rewrites the hosts file with the given managed hostnames.
// If hostnames is nil or empty, removes the managed block entirely.
func (m *Manager) updateHostsFile(hostnames map[string]bool) error {
	// Read existing content
	content, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Parse and filter out our managed block
	lines := strings.Split(string(content), "\n")
	var newLines []string
	inManagedBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == markerStart {
			inManagedBlock = true
			continue
		}
		if trimmed == markerEnd {
			inManagedBlock = false
			continue
		}
		if inManagedBlock {
			continue // Skip managed entries
		}

		newLines = append(newLines, line)
	}

	// Remove trailing empty lines
	for len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) == "" {
		newLines = newLines[:len(newLines)-1]
	}

	// Add new managed block if we have hostnames
	if len(hostnames) > 0 {
		newLines = append(newLines, "") // Blank line before block
		newLines = append(newLines, markerStart)
		for hostname := range hostnames {
			newLines = append(newLines, fmt.Sprintf("%s %s", loopbackIP, hostname))
		}
		newLines = append(newLines, markerEnd)
	}

	// Ensure file ends with newline
	newLines = append(newLines, "")

	// Write atomically: write to temp file, then rename
	newContent := strings.Join(newLines, "\n")

	// Create temp file in same directory for atomic rename
	dir := filepath.Dir(hostsFile)
	tmpFile, err := os.CreateTemp(dir, ".hosts.tmp.")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	// Write content
	if _, err := tmpFile.WriteString(newContent); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Preserve original permissions
	info, err := os.Stat(hostsFile)
	if err != nil {
		return fmt.Errorf("failed to stat hosts file: %w", err)
	}
	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, hostsFile); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// copyMap creates a shallow copy of a map.
func copyMap(m map[string]bool) map[string]bool {
	result := make(map[string]bool, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// ReadManagedHostnames reads the hosts file and returns any dex-managed hostnames.
// This is useful for checking what entries exist without loading them into memory.
func ReadManagedHostnames() ([]string, error) {
	file, err := os.Open(hostsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open hosts file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var hostnames []string
	inManagedBlock := false
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == markerStart {
			inManagedBlock = true
			continue
		}
		if line == markerEnd {
			inManagedBlock = false
			continue
		}

		if inManagedBlock && line != "" && !strings.HasPrefix(line, "#") {
			// Parse "127.0.0.1 hostname" format
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				hostnames = append(hostnames, fields[1])
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read hosts file: %w", err)
	}

	return hostnames, nil
}
