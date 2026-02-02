// Package content manages task and quest content stored in git files
package content

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager handles reading and writing content files in git repositories
type Manager struct {
	basePath string // Base path (repo or worktree)
}

// NewManager creates a new content manager for the given base path
func NewManager(basePath string) *Manager {
	return &Manager{
		basePath: basePath,
	}
}

// SetBasePath updates the base path (useful when switching between repo and worktree)
func (m *Manager) SetBasePath(basePath string) {
	m.basePath = basePath
}

// GetBasePath returns the current base path
func (m *Manager) GetBasePath() string {
	return m.basePath
}

// ensureDir creates a directory if it doesn't exist
func (m *Manager) ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// writeFile writes content to a file, creating parent directories as needed
func (m *Manager) writeFile(relativePath, content string) error {
	fullPath := filepath.Join(m.basePath, relativePath)

	// Ensure parent directory exists
	if err := m.ensureDir(filepath.Dir(fullPath)); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// readFile reads content from a file
func (m *Manager) readFile(relativePath string) (string, error) {
	fullPath := filepath.Join(m.basePath, relativePath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Return empty string for non-existent files
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(data), nil
}

// fileExists checks if a file exists
func (m *Manager) fileExists(relativePath string) bool {
	fullPath := filepath.Join(m.basePath, relativePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// TaskContentPath returns the relative path for task content
func TaskContentPath(taskID string) string {
	return filepath.Join("tasks", taskID)
}

// QuestContentPath returns the relative path for quest content
func QuestContentPath(questID string) string {
	return filepath.Join("quests", questID)
}
