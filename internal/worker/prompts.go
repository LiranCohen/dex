package worker

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/lirancohen/dex/internal/tools"
	"github.com/lirancohen/promptloom"
	"gopkg.in/yaml.v3"
)

// Embedded prompts directory for self-contained worker binary
//
//go:embed prompts/components/*.yaml prompts/profiles/*.yaml prompts/languages/*.yaml
var promptsFS embed.FS

// WorkerPromptLoader loads prompts embedded in the worker binary.
// It uses PromptLoom for composition and templating.
type WorkerPromptLoader struct {
	registry           *promptloom.Registry
	assembler          *promptloom.Assembler
	languageGuidelines map[string]string
}

// WorkerPromptContext provides context for rendering hat prompts.
type WorkerPromptContext struct {
	// Objective information
	ObjectiveID         string
	ObjectiveTitle      string
	ObjectiveDescription string
	BranchName          string

	// Session context
	SessionID   string
	WorkDir     string
	Scratchpad  string

	// Project context
	ProjectName   string
	GitHubOwner   string
	GitHubRepo    string
	IsNewProject  bool

	// Tools available
	Tools            []string
	ToolDescriptions string

	// Checklist items (if any)
	Checklist []string

	// Optional refinements
	ProjectHints       string
	PredecessorContext string
	Language           tools.ProjectType
}

// languageFile represents a language guidelines YAML file.
type languageFile struct {
	Name         string `yaml:"name"`
	Instructions string `yaml:"instructions"`
}

// NewWorkerPromptLoader creates a new prompt loader using embedded prompts.
func NewWorkerPromptLoader() *WorkerPromptLoader {
	return &WorkerPromptLoader{
		registry:           promptloom.NewRegistry(),
		languageGuidelines: make(map[string]string),
	}
}

// LoadAll loads all prompt components and profiles from the embedded filesystem.
func (p *WorkerPromptLoader) LoadAll() error {
	// Get the prompts subdirectory
	promptsDir, err := fs.Sub(promptsFS, "prompts")
	if err != nil {
		return fmt.Errorf("failed to get prompts subdirectory: %w", err)
	}

	// Load components and profiles from the embedded filesystem
	if err := p.registry.LoadFromFS(promptsDir, "."); err != nil {
		return fmt.Errorf("failed to load prompts: %w", err)
	}

	// Validate all components and profiles
	if err := p.registry.ValidateStrict(); err != nil {
		return fmt.Errorf("prompt validation failed: %w", err)
	}

	// Create the assembler
	p.assembler = promptloom.NewAssembler(p.registry)

	// Load language guidelines
	if err := p.loadLanguageGuidelines(); err != nil {
		fmt.Printf("WorkerPromptLoader: warning: failed to load language guidelines: %v\n", err)
		// Don't fail - language guidelines are optional
	}

	// Verify required hats have profiles
	profiles := p.registry.ListProfiles()
	fmt.Printf("WorkerPromptLoader: loaded %d profiles\n", len(profiles))

	requiredHats := []string{"explorer", "planner", "designer", "creator", "critic", "editor", "resolver"}
	for _, hat := range requiredHats {
		if !slices.Contains(profiles, hat) {
			return fmt.Errorf("missing profile for required hat: %s", hat)
		}
	}

	fmt.Printf("WorkerPromptLoader: all required hats validated\n")
	return nil
}

// loadLanguageGuidelines loads language-specific guidelines from embedded files.
func (p *WorkerPromptLoader) loadLanguageGuidelines() error {
	entries, err := fs.ReadDir(promptsFS, "prompts/languages")
	if err != nil {
		return nil // No languages directory is OK
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := fs.ReadFile(promptsFS, "prompts/languages/"+entry.Name())
		if err != nil {
			fmt.Printf("WorkerPromptLoader: warning: failed to read %s: %v\n", entry.Name(), err)
			continue
		}

		var lf languageFile
		if err := yaml.Unmarshal(data, &lf); err != nil {
			fmt.Printf("WorkerPromptLoader: warning: failed to parse %s: %v\n", entry.Name(), err)
			continue
		}

		if lf.Name != "" && lf.Instructions != "" {
			p.languageGuidelines[lf.Name] = lf.Instructions
			fmt.Printf("WorkerPromptLoader: loaded language guidelines for %s\n", lf.Name)
		}
	}

	return nil
}

// projectTypeToLanguage maps ProjectType to language guideline names.
func projectTypeToLanguage(pt tools.ProjectType) string {
	switch pt {
	case tools.ProjectTypeGo:
		return "go"
	case tools.ProjectTypeNode:
		return "typescript"
	case tools.ProjectTypeRust:
		return "rust"
	case tools.ProjectTypePython:
		return "python"
	default:
		return ""
	}
}

// Get returns the assembled prompt for a hat with the given context.
func (p *WorkerPromptLoader) Get(hatName string, ctx *WorkerPromptContext) (string, error) {
	if p.assembler == nil {
		return "", fmt.Errorf("prompt loader not initialized - call LoadAll first")
	}

	// Build PromptLoom context
	loomCtx := promptloom.NewContext()

	if ctx != nil {
		// Add objective context
		loomCtx.SetValue("task_id", ctx.ObjectiveID)
		loomCtx.SetValue("task_title", ctx.ObjectiveTitle)
		if ctx.ObjectiveDescription != "" {
			loomCtx.SetValue("task_description", ctx.ObjectiveDescription)
		}
		loomCtx.SetValue("branch_name", ctx.BranchName)

		// Add session context
		loomCtx.SetValue("worktree_path", ctx.WorkDir)
		loomCtx.SetValue("session_id", ctx.SessionID)

		// Add project context
		if ctx.ProjectName != "" {
			loomCtx.SetValue("project_name", ctx.ProjectName)
		}
		if ctx.GitHubOwner != "" {
			loomCtx.SetValue("github_owner", ctx.GitHubOwner)
			loomCtx.SetValue("github_repo", ctx.GitHubRepo)
			loomCtx.SetFlag("has_github", true)
		}
		if ctx.IsNewProject {
			loomCtx.SetFlag("is_new_project", true)
		}

		// Add tools list
		if len(ctx.Tools) > 0 {
			loomCtx.SetValue("tools", strings.Join(ctx.Tools, ", "))
		}

		// Add tool descriptions
		if ctx.ToolDescriptions != "" {
			loomCtx.SetValue("tool_descriptions", ctx.ToolDescriptions)
		}

		// Add project hints
		if ctx.ProjectHints != "" {
			loomCtx.SetValue("project_hints", ctx.ProjectHints)
			loomCtx.SetFlag("has_project_hints", true)
		}

		// Add predecessor context
		if ctx.PredecessorContext != "" {
			loomCtx.SetValue("predecessor_context", ctx.PredecessorContext)
			loomCtx.SetFlag("has_predecessor_context", true)
		}

		// Add language guidelines if detected
		if ctx.Language != "" && ctx.Language != tools.ProjectTypeUnknown {
			langName := projectTypeToLanguage(ctx.Language)
			if guidelines, ok := p.languageGuidelines[langName]; ok {
				loomCtx.SetValue("language_guidelines", guidelines)
				loomCtx.SetFlag("has_language_guidelines", true)
			}
		}
	}

	// Assemble the prompt
	prompt, err := p.assembler.Assemble(hatName, loomCtx)
	if err != nil {
		return "", fmt.Errorf("failed to assemble prompt for %s: %w", hatName, err)
	}

	return prompt, nil
}

// ListHats returns all available hat names (from profiles).
func (p *WorkerPromptLoader) ListHats() []string {
	if p.registry == nil {
		return nil
	}
	return p.registry.ListProfiles()
}

// HasHat checks if a hat profile exists.
func (p *WorkerPromptLoader) HasHat(hatName string) bool {
	if p.registry == nil {
		return false
	}
	return slices.Contains(p.registry.ListProfiles(), hatName)
}
