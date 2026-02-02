package tools

import (
	"os"
	"path/filepath"
)

// ProjectType represents the detected project type
type ProjectType string

const (
	ProjectTypeGo     ProjectType = "go"
	ProjectTypeNode   ProjectType = "node"
	ProjectTypeRust   ProjectType = "rust"
	ProjectTypePython ProjectType = "python"
	ProjectTypeMake   ProjectType = "make"
	ProjectTypeUnknown ProjectType = "unknown"
)

// ProjectConfig holds auto-detected project configuration
type ProjectConfig struct {
	Type     ProjectType // "go", "node", "python", "rust", "make", "unknown"
	TestCmd  string      // Command to run tests
	LintCmd  string      // Command to run linter
	BuildCmd string      // Command to build
	HasTests bool        // True if test command is available
	HasLint  bool        // True if lint command is available
	HasBuild bool        // True if build command is available
}

// projectIndicator defines how to detect a project type
type projectIndicator struct {
	files    []string    // Files that indicate this project type
	projType ProjectType
	testCmd  string
	lintCmd  string
	buildCmd string
}

// Detection rules ordered by priority (first match wins)
var projectIndicators = []projectIndicator{
	// Go project
	{
		files:    []string{"go.mod"},
		projType: ProjectTypeGo,
		testCmd:  "go test ./...",
		lintCmd:  "go vet ./...",
		buildCmd: "go build ./...",
	},
	// Node.js project
	{
		files:    []string{"package.json"},
		projType: ProjectTypeNode,
		testCmd:  "npm test",
		lintCmd:  "npm run lint",
		buildCmd: "npm run build",
	},
	// Rust project
	{
		files:    []string{"Cargo.toml"},
		projType: ProjectTypeRust,
		testCmd:  "cargo test",
		lintCmd:  "cargo clippy",
		buildCmd: "cargo build",
	},
	// Python project (pyproject.toml preferred)
	{
		files:    []string{"pyproject.toml"},
		projType: ProjectTypePython,
		testCmd:  "pytest",
		lintCmd:  "ruff check .",
		buildCmd: "",
	},
	// Python project (setup.py fallback)
	{
		files:    []string{"setup.py"},
		projType: ProjectTypePython,
		testCmd:  "pytest",
		lintCmd:  "ruff check .",
		buildCmd: "",
	},
	// Makefile project (generic)
	{
		files:    []string{"Makefile"},
		projType: ProjectTypeMake,
		testCmd:  "make test",
		lintCmd:  "make lint",
		buildCmd: "make build",
	},
}

// DetectProject auto-detects the project type and configuration
func DetectProject(workDir string) *ProjectConfig {
	for _, indicator := range projectIndicators {
		for _, file := range indicator.files {
			path := filepath.Join(workDir, file)
			if _, err := os.Stat(path); err == nil {
				config := &ProjectConfig{
					Type:     indicator.projType,
					TestCmd:  indicator.testCmd,
					LintCmd:  indicator.lintCmd,
					BuildCmd: indicator.buildCmd,
					HasTests: indicator.testCmd != "",
					HasLint:  indicator.lintCmd != "",
					HasBuild: indicator.buildCmd != "",
				}

				// Enhance detection based on project type
				enhanceConfig(workDir, config)

				return config
			}
		}
	}

	// No known project type detected
	return &ProjectConfig{
		Type:     ProjectTypeUnknown,
		HasTests: false,
		HasLint:  false,
		HasBuild: false,
	}
}

// enhanceConfig refines the config based on additional project files
func enhanceConfig(workDir string, config *ProjectConfig) {
	switch config.Type {
	case ProjectTypeGo:
		// Check for golangci-lint config
		lintConfigs := []string{".golangci.yml", ".golangci.yaml", ".golangci.json"}
		for _, cfg := range lintConfigs {
			if _, err := os.Stat(filepath.Join(workDir, cfg)); err == nil {
				config.LintCmd = "golangci-lint run"
				break
			}
		}

	case ProjectTypeNode:
		// Check for yarn or pnpm lock files
		if _, err := os.Stat(filepath.Join(workDir, "yarn.lock")); err == nil {
			config.TestCmd = "yarn test"
			config.LintCmd = "yarn lint"
			config.BuildCmd = "yarn build"
		} else if _, err := os.Stat(filepath.Join(workDir, "pnpm-lock.yaml")); err == nil {
			config.TestCmd = "pnpm test"
			config.LintCmd = "pnpm lint"
			config.BuildCmd = "pnpm build"
		} else if _, err := os.Stat(filepath.Join(workDir, "bun.lockb")); err == nil {
			config.TestCmd = "bun test"
			config.LintCmd = "bun run lint"
			config.BuildCmd = "bun run build"
		}

		// Check for TypeScript
		if _, err := os.Stat(filepath.Join(workDir, "tsconfig.json")); err == nil {
			// TypeScript projects typically have build step
			config.HasBuild = true
		}

	case ProjectTypePython:
		// Check for alternative test runners
		if _, err := os.Stat(filepath.Join(workDir, "tox.ini")); err == nil {
			config.TestCmd = "tox"
		}

		// Check for alternative linters
		if _, err := os.Stat(filepath.Join(workDir, ".flake8")); err == nil {
			config.LintCmd = "flake8"
		}
		if _, err := os.Stat(filepath.Join(workDir, "mypy.ini")); err == nil {
			config.LintCmd = "mypy ."
		}

	case ProjectTypeMake:
		// For Makefile projects, verify targets exist by checking the Makefile content
		// For now, assume standard targets exist
		break
	}
}

// CommandsAvailable returns which commands are actually available
// based on the project configuration
func (c *ProjectConfig) CommandsAvailable() map[string]bool {
	return map[string]bool{
		"test":  c.HasTests,
		"lint":  c.HasLint,
		"build": c.HasBuild,
	}
}

// GetTestCommand returns the test command if available
func (c *ProjectConfig) GetTestCommand() (string, bool) {
	if c.HasTests && c.TestCmd != "" {
		return c.TestCmd, true
	}
	return "", false
}

// GetLintCommand returns the lint command if available
func (c *ProjectConfig) GetLintCommand() (string, bool) {
	if c.HasLint && c.LintCmd != "" {
		return c.LintCmd, true
	}
	return "", false
}

// GetBuildCommand returns the build command if available
func (c *ProjectConfig) GetBuildCommand() (string, bool) {
	if c.HasBuild && c.BuildCmd != "" {
		return c.BuildCmd, true
	}
	return "", false
}
