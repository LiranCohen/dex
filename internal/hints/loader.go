// Package hints provides project-specific context loading for Dex
package hints

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lirancohen/dex/internal/security"
)

// HintFilenames defines supported hint file names in priority order
// Later files in the same directory override earlier ones
var HintFilenames = []string{
	".dexhints",
	"DEX.md",
	"AGENTS.md",
	"CLAUDE.md",
}

// DefaultConfig provides default configuration values
var DefaultConfig = Config{
	Enabled:        true,
	MaxTotalSize:   50 * 1024, // 50KB
	MaxImportDepth: 3,
}

// Config holds configuration for the hints loader
type Config struct {
	Enabled        bool
	MaxTotalSize   int
	MaxImportDepth int
}

// HintSection represents a loaded hint section with metadata
type HintSection struct {
	Source  string // Filename type (e.g., ".dexhints", "AGENTS.md", "global")
	Path    string // Full path to the file
	Content string // Loaded content
}

// Loader loads project hints from the directory hierarchy
type Loader struct {
	workDir string
	gitRoot string
	config  Config
}

// NewLoader creates a new hints loader for the given working directory
func NewLoader(workDir string) *Loader {
	return &Loader{
		workDir: workDir,
		gitRoot: findGitRoot(workDir),
		config:  DefaultConfig,
	}
}

// SetConfig updates the loader configuration
func (l *Loader) SetConfig(config Config) {
	l.config = config
}

// Load loads all hint files from the directory chain
// Returns formatted hints content ready for injection into prompts
func (l *Loader) Load() (string, error) {
	if !l.config.Enabled {
		return "", nil
	}

	var sections []HintSection
	totalSize := 0

	// 1. Load global hints
	globalPath := filepath.Join(os.Getenv("HOME"), ".config", "dex", "hints.md")
	if content, err := l.loadFile(globalPath); err == nil && content != "" {
		// Sanitize content
		content = security.SanitizeForPrompt(content)
		totalSize += len(content)
		if totalSize <= l.config.MaxTotalSize {
			sections = append(sections, HintSection{
				Source:  "global",
				Path:    globalPath,
				Content: content,
			})
		}
	}

	// 2. Walk from git root to workDir, loading hints
	dirsToCheck := l.getDirectoryChain()

	for _, dir := range dirsToCheck {
		for _, filename := range HintFilenames {
			path := filepath.Join(dir, filename)
			content, err := l.loadFile(path)
			if err != nil || content == "" {
				continue
			}

			// Process imports
			content, err = l.processImports(content, dir, 0)
			if err != nil {
				return "", fmt.Errorf("error processing imports in %s: %w", path, err)
			}

			// Sanitize content
			content = security.SanitizeForPrompt(content)

			// Check size limit
			totalSize += len(content)
			if totalSize > l.config.MaxTotalSize {
				break // Stop loading if we exceed the limit
			}

			sections = append(sections, HintSection{
				Source:  filename,
				Path:    path,
				Content: content,
			})
		}
	}

	return formatHintSections(sections), nil
}

// LoadSections loads hints and returns individual sections for inspection
func (l *Loader) LoadSections() ([]HintSection, error) {
	if !l.config.Enabled {
		return nil, nil
	}

	var sections []HintSection

	// 1. Load global hints
	globalPath := filepath.Join(os.Getenv("HOME"), ".config", "dex", "hints.md")
	if content, err := l.loadFile(globalPath); err == nil && content != "" {
		sections = append(sections, HintSection{
			Source:  "global",
			Path:    globalPath,
			Content: security.SanitizeForPrompt(content),
		})
	}

	// 2. Walk directory chain
	dirsToCheck := l.getDirectoryChain()

	for _, dir := range dirsToCheck {
		for _, filename := range HintFilenames {
			path := filepath.Join(dir, filename)
			content, err := l.loadFile(path)
			if err != nil || content == "" {
				continue
			}

			content, err = l.processImports(content, dir, 0)
			if err != nil {
				return nil, fmt.Errorf("error processing imports in %s: %w", path, err)
			}

			sections = append(sections, HintSection{
				Source:  filename,
				Path:    path,
				Content: security.SanitizeForPrompt(content),
			})
		}
	}

	return sections, nil
}

// getDirectoryChain returns directories from git root to workDir (root first)
func (l *Loader) getDirectoryChain() []string {
	dirs := []string{}

	current := l.workDir
	for {
		dirs = append([]string{current}, dirs...) // Prepend for root-first order

		if current == l.gitRoot || l.gitRoot == "" {
			break
		}

		parent := filepath.Dir(current)
		if parent == current {
			break // Reached filesystem root
		}
		current = parent
	}

	return dirs
}

// loadFile reads a file and returns its content, or empty string if not found
func (l *Loader) loadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

// Import regex matches @filename lines
var importRegex = regexp.MustCompile(`^@([^\s]+)$`)

// processImports recursively processes @filename imports
func (l *Loader) processImports(content string, baseDir string, depth int) (string, error) {
	if depth >= l.config.MaxImportDepth {
		return content, nil // Stop recursion at max depth
	}

	lines := strings.Split(content, "\n")
	var result strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if match := importRegex.FindStringSubmatch(trimmed); match != nil {
			importPath := match[1]

			// Security: prevent escaping git root
			fullPath := filepath.Join(baseDir, importPath)
			fullPath, err := filepath.Abs(fullPath)
			if err != nil {
				return "", fmt.Errorf("invalid import path: %s", importPath)
			}

			// Check that import doesn't escape the git root
			if l.gitRoot != "" && !strings.HasPrefix(fullPath, l.gitRoot) {
				return "", fmt.Errorf("import escapes repository: %s", importPath)
			}

			imported, err := l.loadFile(fullPath)
			if err != nil {
				// Warn but don't fail on missing imports
				result.WriteString(fmt.Sprintf("<!-- Import not found: %s -->\n", importPath))
				continue
			}

			if imported == "" {
				result.WriteString(fmt.Sprintf("<!-- Import empty: %s -->\n", importPath))
				continue
			}

			// Recursively process imports in imported file
			imported, err = l.processImports(imported, filepath.Dir(fullPath), depth+1)
			if err != nil {
				return "", err
			}

			result.WriteString(fmt.Sprintf("<!-- Imported: %s -->\n", importPath))
			result.WriteString(imported)
			result.WriteString("\n")
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return result.String(), nil
}

// formatHintSections formats loaded sections into a single string
func formatHintSections(sections []HintSection) string {
	if len(sections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Project Context\n\n")

	for _, section := range sections {
		sb.WriteString(fmt.Sprintf("<!-- From: %s -->\n", section.Path))
		sb.WriteString(section.Content)
		if !strings.HasSuffix(section.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// findGitRoot finds the git repository root from the given directory
func findGitRoot(dir string) string {
	current := dir
	for {
		gitDir := filepath.Join(current, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "" // Reached filesystem root, no git repo found
		}
		current = parent
	}
}

// Validate checks hint files for errors without loading them into prompts
func (l *Loader) Validate() []ValidationError {
	var errors []ValidationError

	// Check global hints
	globalPath := filepath.Join(os.Getenv("HOME"), ".config", "dex", "hints.md")
	if err := l.validateFile(globalPath); err != nil {
		errors = append(errors, *err)
	}

	// Check directory chain
	dirsToCheck := l.getDirectoryChain()
	for _, dir := range dirsToCheck {
		for _, filename := range HintFilenames {
			path := filepath.Join(dir, filename)
			if err := l.validateFile(path); err != nil {
				errors = append(errors, *err)
			}
		}
	}

	return errors
}

// ValidationError represents an error found during validation
type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// validateFile validates a single hint file
func (l *Loader) validateFile(path string) *ValidationError {
	content, err := l.loadFile(path)
	if err != nil {
		return &ValidationError{Path: path, Message: err.Error()}
	}
	if content == "" {
		return nil // File doesn't exist, that's OK
	}

	// Try to process imports
	_, err = l.processImports(content, filepath.Dir(path), 0)
	if err != nil {
		return &ValidationError{Path: path, Message: err.Error()}
	}

	// Check for dangerous unicode
	if hasDangerous, _ := security.HasDangerousUnicode(content); hasDangerous {
		return &ValidationError{Path: path, Message: "contains potentially dangerous unicode characters"}
	}

	return nil
}
