package session

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"text/template"

	"github.com/lirancohen/dex/internal/db"
)

// PromptContext provides context for rendering hat prompts
type PromptContext struct {
	Task     *db.Task
	Session  *ActiveSession
	Toolbelt []ToolbeltService
}

// ToolbeltService represents a toolbelt service status
type ToolbeltService struct {
	Name   string
	Status string
}

// PromptLoader loads and templates hat prompts
type PromptLoader struct {
	promptsDir string
	templates  map[string]*template.Template
}

// NewPromptLoader creates a prompt loader for the given prompts directory
func NewPromptLoader(promptsDir string) *PromptLoader {
	return &PromptLoader{
		promptsDir: promptsDir,
		templates:  make(map[string]*template.Template),
	}
}

// LoadAll loads all hat prompt templates from the prompts directory
func (p *PromptLoader) LoadAll() error {
	hatsDir := filepath.Join(p.promptsDir, "hats")

	entries, err := os.ReadDir(hatsDir)
	if err != nil {
		return fmt.Errorf("failed to read hats directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		hatName := entry.Name()[:len(entry.Name())-3] // Remove .md extension
		path := filepath.Join(hatsDir, entry.Name())

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read prompt %s: %w", hatName, err)
		}

		tmpl, err := template.New(hatName).Parse(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", hatName, err)
		}

		p.templates[hatName] = tmpl
	}

	// Validate all required hats have templates
	for _, hat := range ValidHats {
		if _, exists := p.templates[hat]; !exists {
			return fmt.Errorf("missing template for required hat: %s", hat)
		}
	}

	return nil
}

// Get returns the rendered prompt for a hat with the given context
func (p *PromptLoader) Get(hatName string, ctx *PromptContext) (string, error) {
	tmpl, exists := p.templates[hatName]
	if !exists {
		return "", fmt.Errorf("no prompt template for hat: %s", hatName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to render prompt for %s: %w", hatName, err)
	}

	return buf.String(), nil
}

// ListHats returns all available hat names (sorted for deterministic output)
func (p *PromptLoader) ListHats() []string {
	hats := make([]string, 0, len(p.templates))
	for name := range p.templates {
		hats = append(hats, name)
	}
	sort.Strings(hats)
	return hats
}

// HasHat checks if a hat prompt template exists
func (p *PromptLoader) HasHat(hatName string) bool {
	_, exists := p.templates[hatName]
	return exists
}

// Reload reloads all templates from disk
func (p *PromptLoader) Reload() error {
	p.templates = make(map[string]*template.Template)
	return p.LoadAll()
}

// ValidHats returns the list of valid hat names
var ValidHats = []string{
	"planner",
	"architect",
	"implementer",
	"reviewer",
	"tester",
	"debugger",
	"documenter",
	"devops",
	"conflict_manager",
}

// IsValidHat checks if the given hat name is valid
func IsValidHat(hat string) bool {
	return slices.Contains(ValidHats, hat)
}
