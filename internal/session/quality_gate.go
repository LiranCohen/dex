package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/tools"
)

// QualityGate validates code quality before allowing task completion
type QualityGate struct {
	workDir    string
	projectCfg *tools.ProjectConfig // Cached after first detection
	activity   *ActivityRecorder
}

// NewQualityGate creates a new QualityGate for the given work directory
func NewQualityGate(workDir string, activity *ActivityRecorder) *QualityGate {
	return &QualityGate{
		workDir:  workDir,
		activity: activity,
	}
}

// TaskCompleteOpts configures the task completion validation
type TaskCompleteOpts struct {
	Summary   string
	SkipTests bool
	SkipLint  bool
	SkipBuild bool
}

// GateResult contains the outcome of quality gate validation
type GateResult struct {
	Passed   bool         `json:"passed"`
	Tests    *CheckResult `json:"tests,omitempty"`
	Lint     *CheckResult `json:"lint,omitempty"`
	Build    *CheckResult `json:"build,omitempty"`
	Feedback string       `json:"feedback"`
}

// CheckResult contains the outcome of a single quality check
type CheckResult struct {
	Passed     bool   `json:"passed"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skip_reason,omitempty"`
}

// getProjectConfig returns the cached project config, detecting if needed
func (g *QualityGate) getProjectConfig() *tools.ProjectConfig {
	if g.projectCfg == nil {
		g.projectCfg = tools.DetectProject(g.workDir)
	}
	return g.projectCfg
}

// Validate runs all quality gate checks and returns the result
func (g *QualityGate) Validate(ctx context.Context, opts TaskCompleteOpts) *GateResult {
	cfg := g.getProjectConfig()

	result := &GateResult{
		Passed: true,
	}

	// Run tests
	if !opts.SkipTests {
		result.Tests = g.runTests(ctx, cfg)
		if !result.Tests.Passed && !result.Tests.Skipped {
			result.Passed = false
		}
	} else {
		result.Tests = &CheckResult{Skipped: true, SkipReason: "skipped by request"}
	}

	// Run lint
	if !opts.SkipLint {
		result.Lint = g.runLint(ctx, cfg)
		if !result.Lint.Passed && !result.Lint.Skipped {
			result.Passed = false
		}
	} else {
		result.Lint = &CheckResult{Skipped: true, SkipReason: "skipped by request"}
	}

	// Run build
	if !opts.SkipBuild {
		result.Build = g.runBuild(ctx, cfg)
		if !result.Build.Passed && !result.Build.Skipped {
			result.Passed = false
		}
	} else {
		result.Build = &CheckResult{Skipped: true, SkipReason: "skipped by request"}
	}

	// Build feedback message
	result.Feedback = g.buildFeedback(result)

	return result
}

// runTests runs the project's test suite
func (g *QualityGate) runTests(ctx context.Context, cfg *tools.ProjectConfig) *CheckResult {
	cmd, ok := cfg.GetTestCommand()
	if !ok {
		return &CheckResult{
			Passed:     true,
			Skipped:    true,
			SkipReason: "no test command detected for project type: " + string(cfg.Type),
		}
	}

	return g.runCommand(ctx, cmd, "tests", 300)
}

// runLint runs the project's linter
func (g *QualityGate) runLint(ctx context.Context, cfg *tools.ProjectConfig) *CheckResult {
	cmd, ok := cfg.GetLintCommand()
	if !ok {
		return &CheckResult{
			Passed:     true,
			Skipped:    true,
			SkipReason: "no lint command detected for project type: " + string(cfg.Type),
		}
	}

	return g.runCommand(ctx, cmd, "lint", 120)
}

// runBuild runs the project's build command
func (g *QualityGate) runBuild(ctx context.Context, cfg *tools.ProjectConfig) *CheckResult {
	cmd, ok := cfg.GetBuildCommand()
	if !ok {
		return &CheckResult{
			Passed:     true,
			Skipped:    true,
			SkipReason: "no build command detected for project type: " + string(cfg.Type),
		}
	}

	return g.runCommand(ctx, cmd, "build", 300)
}

// runCommand executes a shell command and returns the result
func (g *QualityGate) runCommand(ctx context.Context, command, checkType string, timeoutSecs int) *CheckResult {
	start := time.Now()

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	cmd.Dir = g.workDir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start).Milliseconds()

	result := &CheckResult{
		Output:     string(output),
		DurationMs: duration,
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result.Output = fmt.Sprintf("Command timed out after %d seconds\n%s", timeoutSecs, result.Output)
		}
		result.Passed = false
	} else {
		result.Passed = true
	}

	// Truncate very long output
	if len(result.Output) > 10000 {
		result.Output = result.Output[:10000] + "\n... (output truncated)"
	}

	return result
}

// buildFeedback creates actionable feedback from the gate result
func (g *QualityGate) buildFeedback(result *GateResult) string {
	if result.Passed {
		return "QUALITY_PASSED: All quality checks passed successfully."
	}

	var issues []string

	if result.Tests != nil && !result.Tests.Passed && !result.Tests.Skipped {
		issues = append(issues, fmt.Sprintf("TESTS FAILED:\n%s", truncateForFeedback(result.Tests.Output, 2000)))
	}

	if result.Lint != nil && !result.Lint.Passed && !result.Lint.Skipped {
		issues = append(issues, fmt.Sprintf("LINT FAILED:\n%s", truncateForFeedback(result.Lint.Output, 2000)))
	}

	if result.Build != nil && !result.Build.Passed && !result.Build.Skipped {
		issues = append(issues, fmt.Sprintf("BUILD FAILED:\n%s", truncateForFeedback(result.Build.Output, 2000)))
	}

	return fmt.Sprintf("QUALITY_BLOCKED: Quality checks failed. Fix the following issues and try again:\n\n%s", strings.Join(issues, "\n\n"))
}

// truncateForFeedback truncates output for inclusion in feedback
func truncateForFeedback(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

// RunTests runs only the test suite (for standalone use)
func (g *QualityGate) RunTests(ctx context.Context, verbose bool, timeoutSecs int) *CheckResult {
	cfg := g.getProjectConfig()

	cmd, ok := cfg.GetTestCommand()
	if !ok {
		return &CheckResult{
			Passed:     true,
			Skipped:    true,
			SkipReason: "no test command detected for project type: " + string(cfg.Type),
		}
	}

	// Add verbose flag if supported and requested
	if verbose {
		switch cfg.Type {
		case tools.ProjectTypeGo:
			cmd = "go test -v ./..."
		case tools.ProjectTypeRust:
			cmd = "cargo test -- --nocapture"
		case tools.ProjectTypePython:
			cmd = "pytest -v"
		}
	}

	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}
	if timeoutSecs > 600 {
		timeoutSecs = 600
	}

	return g.runCommand(ctx, cmd, "tests", timeoutSecs)
}

// RunLint runs only the linter (for standalone use)
func (g *QualityGate) RunLint(ctx context.Context, fix bool) *CheckResult {
	cfg := g.getProjectConfig()

	cmd, ok := cfg.GetLintCommand()
	if !ok {
		return &CheckResult{
			Passed:     true,
			Skipped:    true,
			SkipReason: "no lint command detected for project type: " + string(cfg.Type),
		}
	}

	// Add fix flag if supported and requested
	if fix {
		switch cfg.Type {
		case tools.ProjectTypeGo:
			if strings.Contains(cmd, "golangci-lint") {
				cmd = "golangci-lint run --fix"
			}
		case tools.ProjectTypeNode:
			cmd = strings.Replace(cmd, "lint", "lint --fix", 1)
		case tools.ProjectTypePython:
			if strings.Contains(cmd, "ruff") {
				cmd = "ruff check --fix ."
			}
		case tools.ProjectTypeRust:
			cmd = "cargo clippy --fix --allow-dirty"
		}
	}

	return g.runCommand(ctx, cmd, "lint", 120)
}

// RunBuild runs only the build command (for standalone use)
func (g *QualityGate) RunBuild(ctx context.Context, timeoutSecs int) *CheckResult {
	cfg := g.getProjectConfig()

	cmd, ok := cfg.GetBuildCommand()
	if !ok {
		return &CheckResult{
			Passed:     true,
			Skipped:    true,
			SkipReason: "no build command detected for project type: " + string(cfg.Type),
		}
	}

	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}
	if timeoutSecs > 600 {
		timeoutSecs = 600
	}

	return g.runCommand(ctx, cmd, "build", timeoutSecs)
}

// GetProjectType returns the detected project type
func (g *QualityGate) GetProjectType() tools.ProjectType {
	return g.getProjectConfig().Type
}
