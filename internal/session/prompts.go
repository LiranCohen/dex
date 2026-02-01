package session

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/promptloom"
)

// PromptContext provides context for rendering hat prompts
type PromptContext struct {
	Task     *db.Task
	Session  *ActiveSession
	Toolbelt []ToolbeltService
	Project  *ProjectContext
	Tools    []string
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
	promptsDir string
	registry   *promptloom.Registry
	assembler  *promptloom.Assembler
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

// Get returns the assembled prompt for a hat with the given context
func (p *PromptLoader) Get(hatName string, ctx *PromptContext) (string, error) {
	if p.assembler == nil {
		return "", fmt.Errorf("prompt loader not initialized - call LoadAll first")
	}

	// Build PromptLoom context from our PromptContext
	loomCtx := promptloom.NewContext()

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

	// Add toolbelt services
	if len(ctx.Toolbelt) > 0 {
		var services []string
		for _, svc := range ctx.Toolbelt {
			services = append(services, fmt.Sprintf("- %s: %s", svc.Name, svc.Status))
		}
		loomCtx.SetValue("toolbelt", strings.Join(services, "\n"))
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
