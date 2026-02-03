package session

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/tools"
	"github.com/lirancohen/promptloom"
	"gopkg.in/yaml.v3"
)

// PromptContext provides context for rendering hat prompts
type PromptContext struct {
	Task               *db.Task
	Session            *ActiveSession
	Toolbelt           []ToolbeltService
	Project            *ProjectContext
	Tools              []string
	RefinedPrompt      string             // From planning phase - included in system prompt
	ToolDescriptions   string             // Formatted tool descriptions for hat context
	ProjectHints       string             // Loaded from .dexhints, AGENTS.md, etc.
	ProjectMemories    string             // Formatted memory section from previous sessions
	PredecessorContext string             // Handoff from predecessor task in dependency chain
	Language           tools.ProjectType  // Detected programming language
}

// ProjectContext provides project-level context for prompts
type ProjectContext struct {
	Name         string
	RepoPath     string
	GitHubOwner  string
	GitHubRepo   string
	IsNewProject bool // True if this is a new project without an existing git repo
}

// ToolbeltService represents a toolbelt service status
type ToolbeltService struct {
	Name   string
	Status string
}

// PromptLoader loads and assembles hat prompts using PromptLoom
type PromptLoader struct {
	promptsDir         string
	registry           *promptloom.Registry
	assembler          *promptloom.Assembler
	languageGuidelines map[string]string // language name -> guidelines content
}

// languageFile represents a language guidelines YAML file
type languageFile struct {
	Name         string `yaml:"name"`
	Instructions string `yaml:"instructions"`
}

// NewPromptLoader creates a prompt loader for the given prompts directory
func NewPromptLoader(promptsDir string) *PromptLoader {
	return &PromptLoader{
		promptsDir: promptsDir,
		registry:   promptloom.NewRegistry(),
	}
}

// LoadAll loads all prompt components and profiles from the prompts directory
func (p *PromptLoader) LoadAll() error {
	fmt.Printf("PromptLoader.LoadAll: loading prompts from %s\n", p.promptsDir)

	// Create filesystem from the prompts directory
	fsys := os.DirFS(p.promptsDir)

	// Load components and profiles from the filesystem
	// The root path is "." since DirFS is already rooted at promptsDir
	if err := p.registry.LoadFromFS(fsys, "."); err != nil {
		return fmt.Errorf("failed to load prompts: %w", err)
	}

	// Validate all components and profiles
	if err := p.registry.ValidateStrict(); err != nil {
		return fmt.Errorf("prompt validation failed: %w", err)
	}

	// Create the assembler
	p.assembler = promptloom.NewAssembler(p.registry)

	// Load language guidelines
	p.languageGuidelines = make(map[string]string)
	if err := p.loadLanguageGuidelines(); err != nil {
		fmt.Printf("PromptLoader.LoadAll: warning: failed to load language guidelines: %v\n", err)
		// Don't fail on language loading - it's optional
	}

	// Verify all required hats have profiles
	profiles := p.registry.ListProfiles()
	fmt.Printf("PromptLoader.LoadAll: loaded %d profiles\n", len(profiles))

	for _, hat := range ValidHats {
		found := false
		for _, profile := range profiles {
			if profile == hat {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("missing profile for required hat: %s", hat)
		}
	}

	fmt.Printf("PromptLoader.LoadAll: all required hats validated\n")
	return nil
}

// loadLanguageGuidelines loads language-specific guidelines from the languages directory
func (p *PromptLoader) loadLanguageGuidelines() error {
	languagesDir := filepath.Join(p.promptsDir, "languages")

	entries, err := os.ReadDir(languagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No languages directory is OK
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(languagesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("PromptLoader: warning: failed to read %s: %v\n", path, err)
			continue
		}

		var lf languageFile
		if err := yaml.Unmarshal(data, &lf); err != nil {
			fmt.Printf("PromptLoader: warning: failed to parse %s: %v\n", path, err)
			continue
		}

		if lf.Name != "" && lf.Instructions != "" {
			p.languageGuidelines[lf.Name] = lf.Instructions
			fmt.Printf("PromptLoader: loaded language guidelines for %s\n", lf.Name)
		}
	}

	return nil
}

// projectTypeToLanguage maps ProjectType to language guideline names
func projectTypeToLanguage(pt tools.ProjectType) string {
	switch pt {
	case tools.ProjectTypeGo:
		return "go"
	case tools.ProjectTypeNode:
		return "typescript" // Node projects use TypeScript guidelines
	case tools.ProjectTypeRust:
		return "rust"
	case tools.ProjectTypePython:
		return "python"
	default:
		return ""
	}
}

// Get returns the assembled prompt for a hat with the given context
func (p *PromptLoader) Get(hatName string, ctx *PromptContext) (string, error) {
	if p.assembler == nil {
		return "", fmt.Errorf("prompt loader not initialized - call LoadAll first")
	}

	// Build PromptLoom context from our PromptContext
	loomCtx := promptloom.NewContext()

	// Only populate context if provided (can be nil for simple prompts like quest)
	if ctx != nil {
		// Add task context
		if ctx.Task != nil {
			loomCtx.SetValue("task_id", ctx.Task.ID)
			loomCtx.SetValue("task_title", ctx.Task.Title)
			if ctx.Task.Description.Valid {
				loomCtx.SetValue("task_description", ctx.Task.Description.String)
			}
			loomCtx.SetValue("branch_name", ctx.Task.GetBranchName())
		}

		// Add session context
		if ctx.Session != nil {
			loomCtx.SetValue("worktree_path", ctx.Session.WorktreePath)
			loomCtx.SetValue("session_id", ctx.Session.ID)
		}

		// Add project context
		if ctx.Project != nil {
			loomCtx.SetValue("project_name", ctx.Project.Name)
			loomCtx.SetValue("repo_path", ctx.Project.RepoPath)
			if ctx.Project.GitHubOwner != "" {
				loomCtx.SetValue("github_owner", ctx.Project.GitHubOwner)
				loomCtx.SetValue("github_repo", ctx.Project.GitHubRepo)
				loomCtx.SetFlag("has_github", true)
			}
			if ctx.Project.IsNewProject {
				loomCtx.SetFlag("is_new_project", true)
			}
		}

		// Add tools list
		if len(ctx.Tools) > 0 {
			loomCtx.SetValue("tools", strings.Join(ctx.Tools, ", "))
		}

		// Add refined prompt from planning phase
		if ctx.RefinedPrompt != "" {
			loomCtx.SetValue("refined_prompt", ctx.RefinedPrompt)
			loomCtx.SetFlag("has_refined_prompt", true)
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

		// Add project memories
		if ctx.ProjectMemories != "" {
			loomCtx.SetValue("project_memories", ctx.ProjectMemories)
			loomCtx.SetFlag("has_memories", true)
		}

		// Add predecessor context (for dependency chain handoffs)
		if ctx.PredecessorContext != "" {
			loomCtx.SetValue("predecessor_context", ctx.PredecessorContext)
			loomCtx.SetFlag("has_predecessor_context", true)
		}

		// Add toolbelt services
		if len(ctx.Toolbelt) > 0 {
			var services []string
			for _, svc := range ctx.Toolbelt {
				services = append(services, fmt.Sprintf("- %s: %s", svc.Name, svc.Status))
			}
			loomCtx.SetValue("toolbelt", strings.Join(services, "\n"))
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

// ListHats returns all available hat names (from profiles)
func (p *PromptLoader) ListHats() []string {
	if p.registry == nil {
		return nil
	}
	return p.registry.ListProfiles()
}

// HasHat checks if a hat profile exists
func (p *PromptLoader) HasHat(hatName string) bool {
	if p.registry == nil {
		return false
	}
	for _, profile := range p.registry.ListProfiles() {
		if profile == hatName {
			return true
		}
	}
	return false
}

// Reload reloads all prompts from disk
func (p *PromptLoader) Reload() error {
	p.registry = promptloom.NewRegistry()
	p.assembler = nil
	p.languageGuidelines = nil
	return p.LoadAll()
}

// ValidHats returns the list of valid hat names
// These are general-purpose roles that apply to any domain, not just software
var ValidHats = []string{
	"explorer", // Research, investigate, gather information
	"planner",  // Strategy, breakdown, sequencing
	"designer", // High-level structure, approach, architecture
	"creator",  // Build, write, implement the actual work
	"critic",   // Review, evaluate, check quality
	"editor",   // Refine, polish, document
	"resolver", // Handle conflicts, blockers, dependencies
}

// IsValidHat checks if the given hat name is valid
func IsValidHat(hat string) bool {
	return slices.Contains(ValidHats, hat)
}
