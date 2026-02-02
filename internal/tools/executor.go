package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Executor executes tools in the context of a working directory
type Executor struct {
	workDir  string
	toolSet  *Set
	readOnly bool // If true, only read-only tools are allowed
}

// NewExecutor creates a new Executor
func NewExecutor(workDir string, toolSet *Set, readOnly bool) *Executor {
	return &Executor{
		workDir:  workDir,
		toolSet:  toolSet,
		readOnly: readOnly,
	}
}

// WorkDir returns the working directory
func (e *Executor) WorkDir() string {
	return e.workDir
}

// ToolSet returns the tool set
func (e *Executor) ToolSet() *Set {
	return e.toolSet
}

// Execute runs a tool with the given input and returns the result
func (e *Executor) Execute(ctx context.Context, toolName string, input map[string]any) Result {
	start := time.Now()

	// Check if tool exists in our set
	tool := e.toolSet.Get(toolName)
	if tool == nil {
		return Result{
			Output:     fmt.Sprintf("Unknown tool: %s", toolName),
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// If we're in read-only mode, reject non-read-only tools
	if e.readOnly && !tool.ReadOnly {
		return Result{
			Output:     fmt.Sprintf("Tool %s is not available in read-only mode", toolName),
			IsError:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	var result Result
	switch toolName {
	// Read-only tools
	case "read_file":
		result = e.executeReadFile(input)
	case "list_files":
		result = e.executeListFiles(input)
	case "glob":
		result = e.executeGlob(input)
	case "grep":
		result = e.executeGrep(ctx, input)
	case "git_status":
		result = e.executeGitStatus()
	case "git_diff":
		result = e.executeGitDiff(input)
	case "git_log":
		result = e.executeGitLog(input)
	case "web_search":
		result = e.executeWebSearch(ctx, input)
	case "web_fetch":
		result = e.executeWebFetch(ctx, input)
	case "list_runtimes":
		result = e.executeListRuntimes()
	// Write tools
	case "bash":
		result = e.executeBash(ctx, input)
	case "write_file":
		result = e.executeWriteFile(input)
	case "git_init":
		result = e.executeGitInit(input)
	case "git_commit":
		result = e.executeGitCommit(input)
	case "git_remote_add":
		result = e.executeGitRemoteAdd(input)
	case "git_push":
		result = e.executeGitPush()
	default:
		result = Result{
			Output:  fmt.Sprintf("Tool %s not implemented in base executor", toolName),
			IsError: true,
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// Command blocklist patterns for security
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rm\s+(-[rf]+\s+)?/`),           // rm -rf /
	regexp.MustCompile(`(?i)>\s*/dev/`),                     // redirect to /dev/
	regexp.MustCompile(`(?i)sudo\s`),                        // sudo commands
	regexp.MustCompile(`(?i)chmod\s+777`),                   // chmod 777
	regexp.MustCompile(`(?i)mkfs\.`),                        // filesystem format
	regexp.MustCompile(`(?i)dd\s+.*of=/dev/`),               // dd to device
	regexp.MustCompile(`(?i):\(\)\s*\{\s*:\|\s*:\s*&\s*\}`), // fork bomb
}

func (e *Executor) isDangerousCommand(cmd string) bool {
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(cmd) {
			return true
		}
	}
	return false
}

// resolvePath safely resolves a relative path within the work directory
func (e *Executor) resolvePath(relativePath string) (string, error) {
	cleanPath := filepath.Clean(relativePath)

	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths not allowed: %s", relativePath)
	}

	fullPath := filepath.Join(e.workDir, cleanPath)

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	absWorkDir, err := filepath.Abs(e.workDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve work directory: %w", err)
	}

	if !strings.HasPrefix(absPath, absWorkDir+string(filepath.Separator)) && absPath != absWorkDir {
		return "", fmt.Errorf("path escapes work directory: %s", relativePath)
	}

	return fullPath, nil
}

// Read-only tool implementations

func (e *Executor) executeReadFile(input map[string]any) Result {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return Result{Output: "path is required", IsError: true}
	}

	fullPath, err := e.resolvePath(path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("Failed to read file: %v", err),
			IsError: true,
		}
	}

	return Result{Output: string(content), IsError: false}
}

func (e *Executor) executeListFiles(input map[string]any) Result {
	path := "."
	if p, ok := input["path"].(string); ok && p != "" {
		path = p
	}

	fullPath, err := e.resolvePath(path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	recursive := false
	if r, ok := input["recursive"].(bool); ok {
		recursive = r
	}

	var files []string

	if recursive {
		err = filepath.WalkDir(fullPath, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(e.workDir, p)
			if d.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return Result{
				Output:  fmt.Sprintf("Failed to list directory: %v", err),
				IsError: true,
			}
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			files = append(files, name)
		}
	}

	if err != nil {
		return Result{
			Output:  fmt.Sprintf("Failed to list files: %v", err),
			IsError: true,
		}
	}

	return Result{Output: strings.Join(files, "\n"), IsError: false}
}

func (e *Executor) executeGlob(input map[string]any) Result {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return Result{Output: "pattern is required", IsError: true}
	}

	var matches []string
	err := doublestar.GlobWalk(os.DirFS(e.workDir), pattern, func(path string, d os.DirEntry) error {
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("Glob failed: %v", err),
			IsError: true,
		}
	}

	if len(matches) == 0 {
		return Result{Output: "No files matched the pattern", IsError: false}
	}

	return Result{Output: strings.Join(matches, "\n"), IsError: false}
}

func (e *Executor) executeGrep(ctx context.Context, input map[string]any) Result {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return Result{Output: "pattern is required", IsError: true}
	}

	searchPath := "."
	if p, ok := input["path"].(string); ok && p != "" {
		searchPath = p
	}

	fullPath, err := e.resolvePath(searchPath)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	maxResults := 100
	if m, ok := input["max_results"].(float64); ok && m > 0 {
		maxResults = int(m)
	}

	// Build grep command
	args := []string{"-rn", "--color=never"}

	if include, ok := input["include"].(string); ok && include != "" {
		args = append(args, "--include="+include)
	}

	args = append(args, "-e", pattern, fullPath)

	cmd := exec.CommandContext(ctx, "grep", args...)
	output, err := cmd.Output()

	// grep returns exit code 1 when no matches found
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return Result{Output: "No matches found", IsError: false}
		}
		return Result{
			Output:  fmt.Sprintf("Grep failed: %v", err),
			IsError: true,
		}
	}

	// Limit results
	lines := strings.Split(string(output), "\n")
	if len(lines) > maxResults {
		lines = lines[:maxResults]
		lines = append(lines, fmt.Sprintf("... (truncated to %d results)", maxResults))
	}

	// Make paths relative to work directory
	var result []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		// grep output format: /path/to/file:line:content
		if strings.HasPrefix(line, e.workDir) {
			line = strings.TrimPrefix(line, e.workDir+"/")
		}
		result = append(result, line)
	}

	return Result{Output: strings.Join(result, "\n"), IsError: false}
}

func (e *Executor) executeGitStatus() Result {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("git status failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	if len(output) == 0 {
		return Result{Output: "Working directory clean", IsError: false}
	}

	return Result{Output: string(output), IsError: false}
}

func (e *Executor) executeGitDiff(input map[string]any) Result {
	args := []string{"diff"}

	if staged, ok := input["staged"].(bool); ok && staged {
		args = append(args, "--cached")
	}
	if path, ok := input["path"].(string); ok && path != "" {
		args = append(args, "--", path)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("git diff failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	if len(output) == 0 {
		return Result{Output: "No changes", IsError: false}
	}

	return Result{Output: string(output), IsError: false}
}

func (e *Executor) executeGitLog(input map[string]any) Result {
	maxCount := 20
	if m, ok := input["max_count"].(float64); ok && m > 0 {
		maxCount = int(m)
	}

	oneline := true
	if o, ok := input["oneline"].(bool); ok {
		oneline = o
	}

	args := []string{"log", fmt.Sprintf("-n%d", maxCount)}
	if oneline {
		args = append(args, "--oneline")
	}

	if path, ok := input["path"].(string); ok && path != "" {
		args = append(args, "--", path)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("git log failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return Result{Output: string(output), IsError: false}
}

func (e *Executor) executeWebSearch(ctx context.Context, input map[string]any) Result {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return Result{Output: "query is required", IsError: true}
	}

	// Use DuckDuckGo's lite HTML interface for basic search
	// This provides search results without requiring an API key
	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", strings.ReplaceAll(query, " ", "+"))

	cmd := exec.CommandContext(ctx, "curl", "-sL", "--max-time", "15", searchURL)
	output, err := cmd.Output()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("Web search failed: %v. Try using web_fetch with a specific URL instead.", err),
			IsError: true,
		}
	}

	// Extract text content and search results from HTML
	content := string(output)

	// Simple extraction of result links and snippets
	var results []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for result links (simplified extraction)
		if strings.Contains(line, "result-link") || strings.Contains(line, "result__a") {
			// Extract URL from href
			if idx := strings.Index(line, "href=\""); idx != -1 {
				start := idx + 6
				end := strings.Index(line[start:], "\"")
				if end > 0 {
					url := line[start : start+end]
					if strings.HasPrefix(url, "http") {
						results = append(results, url)
					}
				}
			}
		}
		// Also look for snippets
		if strings.Contains(line, "result__snippet") && !strings.Contains(line, "<") {
			results = append(results, "  "+line)
		}
	}

	if len(results) == 0 {
		return Result{
			Output:  fmt.Sprintf("Search completed for '%s' but couldn't extract structured results. Consider using web_fetch to access specific URLs directly.", query),
			IsError: false,
		}
	}

	return Result{
		Output:  fmt.Sprintf("Search results for '%s':\n%s", query, strings.Join(results[:min(len(results), 20)], "\n")),
		IsError: false,
	}
}

func (e *Executor) executeWebFetch(ctx context.Context, input map[string]any) Result {
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return Result{Output: "url is required", IsError: true}
	}

	// Use curl for simple fetch - could be replaced with proper HTTP client + HTML to text
	cmd := exec.CommandContext(ctx, "curl", "-sL", "--max-time", "30", url)
	output, err := cmd.Output()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("Failed to fetch URL: %v", err),
			IsError: true,
		}
	}

	content := string(output)
	// Truncate very long responses
	if len(content) > 50000 {
		content = content[:50000] + "\n... (truncated)"
	}

	return Result{Output: content, IsError: false}
}

// runtimeCheck represents a runtime to check for
type runtimeCheck struct {
	name    string
	cmd     string
	args    []string
	extract func(string) string // optional function to extract version
}

func (e *Executor) executeListRuntimes() Result {
	// List of runtimes to check
	runtimes := []runtimeCheck{
		// Languages
		{name: "Go", cmd: "go", args: []string{"version"}},
		{name: "Node.js", cmd: "node", args: []string{"--version"}},
		{name: "Bun", cmd: "bun", args: []string{"--version"}},
		{name: "Deno", cmd: "deno", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "Python", cmd: "python3", args: []string{"--version"}},
		{name: "Python 2", cmd: "python", args: []string{"--version"}},
		{name: "Ruby", cmd: "ruby", args: []string{"--version"}},
		{name: "PHP", cmd: "php", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "Perl", cmd: "perl", args: []string{"--version"}, extract: func(s string) string {
			// Extract version from verbose output
			if idx := strings.Index(s, "(v"); idx != -1 {
				end := strings.Index(s[idx:], ")")
				if end > 0 {
					return "perl " + s[idx+1:idx+end]
				}
			}
			return "perl installed"
		}},
		// Rust
		{name: "Rust (rustc)", cmd: "rustc", args: []string{"--version"}},
		{name: "Cargo", cmd: "cargo", args: []string{"--version"}},
		// C/C++
		{name: "GCC", cmd: "gcc", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "G++", cmd: "g++", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "Clang", cmd: "clang", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		// JVM
		{name: "Java", cmd: "java", args: []string{"-version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "Kotlin", cmd: "kotlin", args: []string{"-version"}},
		{name: "Scala", cmd: "scala", args: []string{"-version"}},
		{name: "Gradle", cmd: "gradle", args: []string{"--version"}, extract: func(s string) string {
			for _, line := range strings.Split(s, "\n") {
				if strings.HasPrefix(line, "Gradle ") {
					return strings.TrimSpace(line)
				}
			}
			return "gradle installed"
		}},
		{name: "Maven", cmd: "mvn", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		// Other
		{name: "Zig", cmd: "zig", args: []string{"version"}},
		{name: "Elixir", cmd: "elixir", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "Erlang", cmd: "erl", args: []string{"-eval", "erlang:display(erlang:system_info(otp_release)), halt().", "-noshell"}},
		// Package managers
		{name: "npm", cmd: "npm", args: []string{"--version"}, extract: func(s string) string { return "npm " + strings.TrimSpace(s) }},
		{name: "yarn", cmd: "yarn", args: []string{"--version"}, extract: func(s string) string { return "yarn " + strings.TrimSpace(s) }},
		{name: "pnpm", cmd: "pnpm", args: []string{"--version"}, extract: func(s string) string { return "pnpm " + strings.TrimSpace(s) }},
		{name: "pip", cmd: "pip3", args: []string{"--version"}, extract: func(s string) string {
			parts := strings.Fields(s)
			if len(parts) >= 2 {
				return "pip " + parts[1]
			}
			return s
		}},
		{name: "gem", cmd: "gem", args: []string{"--version"}, extract: func(s string) string { return "gem " + strings.TrimSpace(s) }},
		{name: "composer", cmd: "composer", args: []string{"--version"}, extract: func(s string) string {
			parts := strings.Fields(s)
			if len(parts) >= 3 {
				return parts[0] + " " + parts[2]
			}
			return s
		}},
		// Build tools
		{name: "Make", cmd: "make", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		{name: "CMake", cmd: "cmake", args: []string{"--version"}, extract: func(s string) string {
			lines := strings.Split(s, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return s
		}},
		// Containers
		{name: "Docker", cmd: "docker", args: []string{"--version"}},
		{name: "Podman", cmd: "podman", args: []string{"--version"}},
	}

	var found []string
	var notFound []string

	for _, rt := range runtimes {
		cmd := exec.Command(rt.cmd, rt.args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			version := strings.TrimSpace(string(output))
			if rt.extract != nil {
				version = rt.extract(version)
			}
			found = append(found, fmt.Sprintf("✓ %s: %s", rt.name, version))
		} else {
			notFound = append(notFound, rt.name)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Available Runtimes\n\n")

	if len(found) > 0 {
		for _, f := range found {
			sb.WriteString(f + "\n")
		}
	} else {
		sb.WriteString("No common runtimes detected.\n")
	}

	if len(notFound) > 0 {
		sb.WriteString("\n## Not Installed\n")
		sb.WriteString(strings.Join(notFound, ", "))
		sb.WriteString("\n")
	}

	return Result{Output: sb.String(), IsError: false}
}

// Write tool implementations

func (e *Executor) executeBash(ctx context.Context, input map[string]any) Result {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return Result{Output: "command is required", IsError: true}
	}

	if e.isDangerousCommand(command) {
		return Result{
			Output:  "Command blocked: potentially dangerous operation detected",
			IsError: true,
		}
	}

	timeoutSecs := 300
	if t, ok := input["timeout_seconds"].(float64); ok {
		timeoutSecs = int(t)
		if timeoutSecs > 300 {
			timeoutSecs = 300
		}
		if timeoutSecs < 1 {
			timeoutSecs = 1
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return Result{
				Output:  fmt.Sprintf("Command timed out after %d seconds", timeoutSecs),
				IsError: true,
			}
		}
		return Result{
			Output:  fmt.Sprintf("%s\nError: %v", string(output), err),
			IsError: true,
		}
	}

	return Result{Output: string(output), IsError: false}
}

func (e *Executor) executeWriteFile(input map[string]any) Result {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return Result{Output: "path is required", IsError: true}
	}

	content, ok := input["content"].(string)
	if !ok {
		return Result{Output: "content is required", IsError: true}
	}

	fullPath, err := e.resolvePath(path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Result{
			Output:  fmt.Sprintf("Failed to create directory: %v", err),
			IsError: true,
		}
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return Result{
			Output:  fmt.Sprintf("Failed to write file: %v", err),
			IsError: true,
		}
	}

	// Count lines for better feedback
	lineCount := strings.Count(content, "\n") + 1
	return Result{
		Output:  fmt.Sprintf("Successfully wrote %d bytes (%d lines) to %s", len(content), lineCount, path),
		IsError: false,
	}
}

func (e *Executor) executeGitInit(input map[string]any) Result {
	defaultBranch := "main"
	if branch, ok := input["default_branch"].(string); ok && branch != "" {
		defaultBranch = branch
	}

	cmd := exec.Command("git", "init", "-b", defaultBranch)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("git init failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return Result{
		Output:  fmt.Sprintf("Initialized git repository with default branch '%s'\n%s", defaultBranch, string(output)),
		IsError: false,
	}
}

func (e *Executor) executeGitCommit(input map[string]any) Result {
	message, ok := input["message"].(string)
	if !ok || message == "" {
		return Result{Output: "message is required", IsError: true}
	}

	// Stage files if specified
	if files, ok := input["files"].([]any); ok && len(files) > 0 {
		var paths []string
		for _, f := range files {
			if s, ok := f.(string); ok {
				paths = append(paths, s)
			}
		}
		if len(paths) > 0 {
			addCmd := exec.Command("git", append([]string{"add"}, paths...)...)
			addCmd.Dir = e.workDir
			if output, err := addCmd.CombinedOutput(); err != nil {
				return Result{
					Output:  fmt.Sprintf("Failed to stage files: %s: %v", string(output), err),
					IsError: true,
				}
			}
		}
	}

	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("git commit failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	// Extract commit hash from output
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "[") {
			// Format: [branch hash] message
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				hash := strings.TrimSuffix(parts[1], "]")
				return Result{
					Output:  fmt.Sprintf("Created commit %s", hash),
					IsError: false,
				}
			}
		}
	}

	return Result{Output: string(output), IsError: false}
}

func (e *Executor) executeGitRemoteAdd(input map[string]any) Result {
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return Result{Output: "url is required", IsError: true}
	}

	name := "origin"
	if n, ok := input["name"].(string); ok && n != "" {
		name = n
	}

	cmd := exec.Command("git", "remote", "add", name, url)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			updateCmd := exec.Command("git", "remote", "set-url", name, url)
			updateCmd.Dir = e.workDir
			updateOutput, updateErr := updateCmd.CombinedOutput()
			if updateErr != nil {
				return Result{
					Output:  fmt.Sprintf("git remote set-url failed: %s: %v", string(updateOutput), updateErr),
					IsError: true,
				}
			}
			return Result{
				Output:  fmt.Sprintf("Updated remote '%s' to %s", name, url),
				IsError: false,
			}
		}
		return Result{
			Output:  fmt.Sprintf("git remote add failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return Result{
		Output:  fmt.Sprintf("Added remote '%s' pointing to %s", name, url),
		IsError: false,
	}
}

func (e *Executor) executeGitPush() Result {
	// Get current branch
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = e.workDir
	branchOutput, err := branchCmd.Output()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("Failed to get current branch: %v", err),
			IsError: true,
		}
	}
	branch := strings.TrimSpace(string(branchOutput))

	cmd := exec.Command("git", "push", "-u", "origin", branch)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Output:  fmt.Sprintf("git push failed: %s: %v", string(output), err),
			IsError: true,
		}
	}

	return Result{
		Output:  fmt.Sprintf("Pushed branch %s to origin", branch),
		IsError: false,
	}
}
