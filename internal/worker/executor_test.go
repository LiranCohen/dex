package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lirancohen/dex/internal/tools"
)

func TestToolResult(t *testing.T) {
	t.Run("Success result", func(t *testing.T) {
		result := ToolResult{Output: "success", IsError: false}
		if result.IsError {
			t.Error("expected IsError to be false")
		}
		if result.Output != "success" {
			t.Errorf("unexpected output: %s", result.Output)
		}
	})

	t.Run("Error result", func(t *testing.T) {
		result := ToolResult{Output: "failed", IsError: true}
		if !result.IsError {
			t.Error("expected IsError to be true")
		}
	})
}

func TestTaskCompleteOpts(t *testing.T) {
	opts := TaskCompleteOpts{
		Summary:   "Completed task",
		SkipTests: true,
		SkipLint:  false,
		SkipBuild: true,
	}

	if opts.Summary != "Completed task" {
		t.Error("unexpected summary")
	}
	if !opts.SkipTests {
		t.Error("expected SkipTests to be true")
	}
	if opts.SkipLint {
		t.Error("expected SkipLint to be false")
	}
	if !opts.SkipBuild {
		t.Error("expected SkipBuild to be true")
	}
}

func TestGateResult(t *testing.T) {
	t.Run("Passed result", func(t *testing.T) {
		result := GateResult{
			Passed:     true,
			Output:     "All tests passed",
			DurationMs: 1500,
			Feedback:   "Quality gate passed",
		}

		if !result.Passed {
			t.Error("expected Passed to be true")
		}
		if result.Skipped {
			t.Error("expected Skipped to be false")
		}
	})

	t.Run("Skipped result", func(t *testing.T) {
		result := GateResult{
			Skipped:    true,
			SkipReason: "No test command configured",
		}

		if !result.Skipped {
			t.Error("expected Skipped to be true")
		}
		if result.SkipReason == "" {
			t.Error("expected SkipReason to be set")
		}
	})

	t.Run("Failed result", func(t *testing.T) {
		result := GateResult{
			Passed:     false,
			Output:     "Test failures",
			DurationMs: 500,
		}

		if result.Passed {
			t.Error("expected Passed to be false")
		}
	})
}

func TestNewWorkerToolExecutor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	t.Run("With GitHub token", func(t *testing.T) {
		executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "ghp_token123")

		if executor.workDir != tmpDir {
			t.Errorf("expected workDir %s, got %s", tmpDir, executor.workDir)
		}
		if executor.owner != "owner" {
			t.Errorf("expected owner 'owner', got %s", executor.owner)
		}
		if executor.repo != "repo" {
			t.Errorf("expected repo 'repo', got %s", executor.repo)
		}
		if executor.githubClient == nil {
			t.Error("expected githubClient to be set when token provided")
		}
		if executor.gitOps == nil {
			t.Error("expected gitOps to be set")
		}
	})

	t.Run("Without GitHub token", func(t *testing.T) {
		executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

		if executor.githubClient != nil {
			t.Error("expected githubClient to be nil when no token")
		}
	})
}

func TestWorkerToolExecutor_SetGitHubClient(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	if executor.githubClient != nil {
		t.Error("expected initial githubClient to be nil")
	}

	// Can't easily test SetGitHubClient without creating a real client
	// but we can verify the method exists and doesn't panic with nil
	executor.SetGitHubClient(nil)
}

func TestWorkerToolExecutor_SetQualityGate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	if executor.qualityGate != nil {
		t.Error("expected initial qualityGate to be nil")
	}

	qg := NewWorkerQualityGate(tmpDir)
	executor.SetQualityGate(qg)

	if executor.qualityGate != qg {
		t.Error("expected qualityGate to be set")
	}
}

func TestNewWorkerQualityGate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)

	if qg.workDir != tmpDir {
		t.Errorf("expected workDir %s, got %s", tmpDir, qg.workDir)
	}
	if qg.projectConfig == nil {
		t.Error("expected projectConfig to be detected")
	}
}

func TestWorkerQualityGate_RunTests_NoTestCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)
	result := qg.RunTests(context.Background(), false, 60)

	if !result.Skipped {
		t.Error("expected tests to be skipped for empty project")
	}
	if result.SkipReason == "" {
		t.Error("expected skip reason to be set")
	}
}

func TestWorkerQualityGate_RunLint_NoLintCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)
	result := qg.RunLint(context.Background(), false)

	if !result.Skipped {
		t.Error("expected lint to be skipped for empty project")
	}
}

func TestWorkerQualityGate_RunBuild_NoBuildCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)
	result := qg.RunBuild(context.Background(), 60)

	if !result.Skipped {
		t.Error("expected build to be skipped for empty project")
	}
}

func TestWorkerQualityGate_Validate_AllSkipped(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)

	opts := TaskCompleteOpts{
		Summary:   "Completed the task",
		SkipTests: true,
		SkipLint:  true,
		SkipBuild: true,
	}

	result := qg.Validate(context.Background(), opts)

	if !result.Passed {
		t.Errorf("expected validation to pass when all checks skipped, got: %s", result.Feedback)
	}
	if result.Feedback == "" {
		t.Error("expected feedback message")
	}
}

func TestWorkerQualityGate_RunCommand_SimpleCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)

	// Run a simple command that should succeed
	result := qg.runCommand(context.Background(), "echo hello", 60)

	if !result.Passed {
		t.Errorf("expected command to pass, got: %s", result.Output)
	}
	if result.DurationMs < 0 {
		t.Error("expected positive duration")
	}
}

func TestWorkerQualityGate_RunCommand_FailingCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	qg := NewWorkerQualityGate(tmpDir)

	// Run a command that should fail
	result := qg.runCommand(context.Background(), "exit 1", 60)

	if result.Passed {
		t.Error("expected command to fail")
	}
}

func TestWorkerToolExecutor_ExecuteGitDiff_NoGitOps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")
	executor.gitOps = nil // Force nil

	result := executor.executeGitDiff(map[string]any{})

	if !result.IsError {
		t.Error("expected error when gitOps is nil")
	}
	if result.Output != "Git operations not configured" {
		t.Errorf("unexpected error message: %s", result.Output)
	}
}

func TestWorkerToolExecutor_ExecuteGitCommit_MissingMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	// Missing message
	result := executor.executeGitCommit(map[string]any{})
	if !result.IsError {
		t.Error("expected error when message is missing")
	}
	if result.Output != "message is required" {
		t.Errorf("unexpected error: %s", result.Output)
	}

	// Empty message
	result = executor.executeGitCommit(map[string]any{"message": ""})
	if !result.IsError {
		t.Error("expected error when message is empty")
	}
}

func TestWorkerToolExecutor_ExecuteGitCommit_NoGitOps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")
	executor.gitOps = nil

	result := executor.executeGitCommit(map[string]any{"message": "test"})
	if !result.IsError {
		t.Error("expected error when gitOps is nil")
	}
}

func TestWorkerToolExecutor_ExecuteGitPush_NoGitOps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")
	executor.gitOps = nil

	result := executor.executeGitPush(map[string]any{})
	if !result.IsError {
		t.Error("expected error when gitOps is nil")
	}
}

func TestWorkerToolExecutor_ExecuteGitRemoteAdd_MissingURL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	result := executor.executeGitRemoteAdd(map[string]any{})
	if !result.IsError {
		t.Error("expected error when URL is missing")
	}
	if result.Output != "url is required" {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestWorkerToolExecutor_ExecuteGitHubCreateRepo_NoClient(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	result := executor.executeGitHubCreateRepo(context.Background(), map[string]any{"name": "test"})
	if !result.IsError {
		t.Error("expected error when GitHub client is nil")
	}
	if result.Output != "GitHub client not configured" {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestWorkerToolExecutor_ExecuteGitHubCreateRepo_MissingName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Need to simulate a client
	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "fake-token")

	result := executor.executeGitHubCreateRepo(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
	if result.Output != "name is required" {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestWorkerToolExecutor_ExecuteGitHubCreatePR_NoClient(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	result := executor.executeGitHubCreatePR(context.Background(), map[string]any{"title": "test"})
	if !result.IsError {
		t.Error("expected error when GitHub client is nil")
	}
}

func TestWorkerToolExecutor_ExecuteGitHubCreatePR_MissingTitle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "fake-token")

	result := executor.executeGitHubCreatePR(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("expected error when title is missing")
	}
	if result.Output != "title is required" {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestWorkerToolExecutor_ExecuteTaskComplete_MissingSummary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	result := executor.executeTaskComplete(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("expected error when summary is missing")
	}
	if result.Output != "summary is required" {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestWorkerToolExecutor_Execute_DelegatesToBase(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	// read_file should delegate to base executor
	// Use relative path since base executor doesn't allow absolute paths outside workdir
	result := executor.Execute(context.Background(), "read_file", map[string]any{
		"path": "test.txt",
	})

	if result.IsError {
		t.Errorf("read_file failed: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("expected output from read_file")
	}
}

func TestWorkerToolExecutor_Execute_LargeResponseProcessing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	executor := NewWorkerToolExecutor(tmpDir, "owner", "repo", "")

	// Test with git_diff which returns no changes (small response)
	// This tests the large response processing code path doesn't break small responses
	result := executor.executeGitDiff(map[string]any{})

	// Will fail since not a git repo, but tests the code path
	if !result.IsError {
		// If it somehow succeeded, the output should be reasonable
		if len(result.Output) > 100000 {
			t.Error("output seems unexpectedly large")
		}
	}
}

// MockCommandRunner is a test double for CommandRunner.
type MockCommandRunner struct {
	// RunResults maps command strings to results
	RunResults map[string]*CommandResult
	// GitResults maps git args (joined by space) to results
	GitResults map[string]*CommandResult
	// RunCalls tracks calls to Run
	RunCalls []string
	// GitCalls tracks calls to RunGit
	GitCalls [][]string
	// DefaultResult returned when no specific result is configured
	DefaultResult *CommandResult
}

func NewMockCommandRunner() *MockCommandRunner {
	return &MockCommandRunner{
		RunResults: make(map[string]*CommandResult),
		GitResults: make(map[string]*CommandResult),
		RunCalls:   []string{},
		GitCalls:   [][]string{},
		DefaultResult: &CommandResult{
			Output:   "",
			ExitCode: 0,
			Err:      nil,
		},
	}
}

func (m *MockCommandRunner) Run(ctx context.Context, workDir, command string) *CommandResult {
	m.RunCalls = append(m.RunCalls, command)
	if result, ok := m.RunResults[command]; ok {
		return result
	}
	return m.DefaultResult
}

func (m *MockCommandRunner) RunGit(ctx context.Context, workDir string, args ...string) *CommandResult {
	m.GitCalls = append(m.GitCalls, args)
	key := strings.Join(args, " ")
	if result, ok := m.GitResults[key]; ok {
		return result
	}
	return m.DefaultResult
}

func TestWorkerQualityGate_RunTests_WithMock(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.RunResults["go test ./..."] = &CommandResult{
		Output:   "ok  	mypackage	0.123s",
		ExitCode: 0,
		Err:      nil,
	}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	// Configure project with test command
	qg.projectConfig = &tools.ProjectConfig{
		Type:     tools.ProjectTypeGo,
		HasTests: true,
		TestCmd:  "go test ./...",
	}

	result := qg.RunTests(context.Background(), false, 300)

	if !result.Passed {
		t.Errorf("expected tests to pass, got: %s", result.Output)
	}
	if len(mockRunner.RunCalls) != 1 {
		t.Errorf("expected 1 run call, got %d", len(mockRunner.RunCalls))
	}
}

func TestWorkerQualityGate_RunTests_Failure(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.RunResults["go test ./..."] = &CommandResult{
		Output:   "--- FAIL: TestSomething\nFAIL",
		ExitCode: 1,
		Err:      fmt.Errorf("exit status 1"),
	}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	qg.projectConfig = &tools.ProjectConfig{
		Type:     tools.ProjectTypeGo,
		HasTests: true,
		TestCmd:  "go test ./...",
	}

	result := qg.RunTests(context.Background(), false, 300)

	if result.Passed {
		t.Error("expected tests to fail")
	}
	if !strings.Contains(result.Output, "FAIL") {
		t.Errorf("expected failure output, got: %s", result.Output)
	}
}

func TestWorkerQualityGate_RunLint_WithMock(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.RunResults["golangci-lint run"] = &CommandResult{
		Output:   "",
		ExitCode: 0,
		Err:      nil,
	}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	qg.projectConfig = &tools.ProjectConfig{
		Type:    tools.ProjectTypeGo,
		HasLint: true,
		LintCmd: "golangci-lint run",
	}

	result := qg.RunLint(context.Background(), false)

	if !result.Passed {
		t.Errorf("expected lint to pass, got: %s", result.Output)
	}
}

func TestWorkerQualityGate_RunBuild_WithMock(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.RunResults["go build ./..."] = &CommandResult{
		Output:   "",
		ExitCode: 0,
		Err:      nil,
	}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	qg.projectConfig = &tools.ProjectConfig{
		Type:     tools.ProjectTypeGo,
		HasBuild: true,
		BuildCmd: "go build ./...",
	}

	result := qg.RunBuild(context.Background(), 300)

	if !result.Passed {
		t.Errorf("expected build to pass, got: %s", result.Output)
	}
}

func TestWorkerQualityGate_Validate_AllPass(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.RunResults["go test ./..."] = &CommandResult{Output: "ok", Err: nil}
	mockRunner.RunResults["golangci-lint run"] = &CommandResult{Output: "", Err: nil}
	mockRunner.RunResults["go build ./..."] = &CommandResult{Output: "", Err: nil}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	qg.projectConfig = &tools.ProjectConfig{
		Type:     tools.ProjectTypeGo,
		HasTests: true,
		TestCmd:  "go test ./...",
		HasLint:  true,
		LintCmd:  "golangci-lint run",
		HasBuild: true,
		BuildCmd: "go build ./...",
	}

	result := qg.Validate(context.Background(), TaskCompleteOpts{Summary: "Done"})

	if !result.Passed {
		t.Errorf("expected validation to pass, got: %s", result.Feedback)
	}
	if !strings.Contains(result.Feedback, "passed") {
		t.Errorf("expected passed message, got: %s", result.Feedback)
	}
}

func TestWorkerQualityGate_Validate_TestsFail(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.RunResults["go test ./..."] = &CommandResult{
		Output: "FAIL",
		Err:    fmt.Errorf("exit status 1"),
	}
	mockRunner.RunResults["golangci-lint run"] = &CommandResult{Output: "", Err: nil}
	mockRunner.RunResults["go build ./..."] = &CommandResult{Output: "", Err: nil}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	qg.projectConfig = &tools.ProjectConfig{
		Type:     tools.ProjectTypeGo,
		HasTests: true,
		TestCmd:  "go test ./...",
		HasLint:  true,
		LintCmd:  "golangci-lint run",
		HasBuild: true,
		BuildCmd: "go build ./...",
	}

	result := qg.Validate(context.Background(), TaskCompleteOpts{Summary: "Done"})

	if result.Passed {
		t.Error("expected validation to fail")
	}
	if !strings.Contains(result.Feedback, "Tests failed") {
		t.Errorf("expected tests failed message, got: %s", result.Feedback)
	}
}

func TestWorkerQualityGate_Validate_SkipTests(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	// Don't configure test result - it shouldn't be called
	mockRunner.RunResults["golangci-lint run"] = &CommandResult{Output: "", Err: nil}
	mockRunner.RunResults["go build ./..."] = &CommandResult{Output: "", Err: nil}

	qg := NewWorkerQualityGateWithRunner("/tmp", mockRunner)
	qg.projectConfig = &tools.ProjectConfig{
		Type:     tools.ProjectTypeGo,
		HasTests: true,
		TestCmd:  "go test ./...",
		HasLint:  true,
		LintCmd:  "golangci-lint run",
		HasBuild: true,
		BuildCmd: "go build ./...",
	}

	result := qg.Validate(context.Background(), TaskCompleteOpts{
		Summary:   "Done",
		SkipTests: true,
	})

	if !result.Passed {
		t.Errorf("expected validation to pass, got: %s", result.Feedback)
	}

	// Verify tests were not called
	for _, call := range mockRunner.RunCalls {
		if strings.Contains(call, "test") {
			t.Error("tests should not have been run")
		}
	}
}

func TestWorkerToolExecutor_GitRemoteAdd_WithMock(t *testing.T) {
	executor := NewWorkerToolExecutor("/tmp", "owner", "repo", "")

	mockRunner := NewMockCommandRunner()
	mockRunner.GitResults["remote add origin https://github.com/test/repo.git"] = &CommandResult{
		Output: "",
		Err:    nil,
	}
	executor.SetCommandRunner(mockRunner)

	result := executor.executeGitRemoteAdd(map[string]any{
		"url": "https://github.com/test/repo.git",
	})

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Added remote") {
		t.Errorf("expected added remote message, got: %s", result.Output)
	}
	if len(mockRunner.GitCalls) != 1 {
		t.Errorf("expected 1 git call, got %d", len(mockRunner.GitCalls))
	}
}

func TestWorkerToolExecutor_GitRemoteAdd_AlreadyExists(t *testing.T) {
	executor := NewWorkerToolExecutor("/tmp", "owner", "repo", "")

	mockRunner := NewMockCommandRunner()
	mockRunner.GitResults["remote add origin https://github.com/test/repo.git"] = &CommandResult{
		Output: "error: remote origin already exists.",
		Err:    fmt.Errorf("exit status 1"),
	}
	mockRunner.GitResults["remote set-url origin https://github.com/test/repo.git"] = &CommandResult{
		Output: "",
		Err:    nil,
	}
	executor.SetCommandRunner(mockRunner)

	result := executor.executeGitRemoteAdd(map[string]any{
		"url": "https://github.com/test/repo.git",
	})

	if result.IsError {
		t.Errorf("expected success after update, got error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Updated remote") {
		t.Errorf("expected updated remote message, got: %s", result.Output)
	}
	if len(mockRunner.GitCalls) != 2 {
		t.Errorf("expected 2 git calls (add, then set-url), got %d", len(mockRunner.GitCalls))
	}
}

func TestExecCommandRunner_Run(t *testing.T) {
	runner := NewExecCommandRunner()

	result := runner.Run(context.Background(), "/tmp", "echo hello")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result.Output)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestExecCommandRunner_Run_Failure(t *testing.T) {
	runner := NewExecCommandRunner()

	result := runner.Run(context.Background(), "/tmp", "exit 1")

	if result.Err == nil {
		t.Error("expected error for failed command")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestExecCommandRunner_RunGit(t *testing.T) {
	runner := NewExecCommandRunner()

	result := runner.RunGit(context.Background(), "/tmp", "--version")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if !strings.Contains(result.Output, "git version") {
		t.Errorf("expected git version in output, got: %s", result.Output)
	}
}

func TestWorkerToolExecutor_SetCommandRunner(t *testing.T) {
	executor := NewWorkerToolExecutor("/tmp", "owner", "repo", "")

	// Verify default runner is set
	if executor.cmdRunner == nil {
		t.Error("expected default cmdRunner to be set")
	}

	// Set mock runner
	mockRunner := NewMockCommandRunner()
	executor.SetCommandRunner(mockRunner)

	if executor.cmdRunner != mockRunner {
		t.Error("expected cmdRunner to be updated")
	}
}

func TestWorkerQualityGate_SetCommandRunner(t *testing.T) {
	qg := NewWorkerQualityGate("/tmp")

	// Verify default runner is set
	if qg.cmdRunner == nil {
		t.Error("expected default cmdRunner to be set")
	}

	// Set mock runner
	mockRunner := NewMockCommandRunner()
	qg.SetCommandRunner(mockRunner)

	if qg.cmdRunner != mockRunner {
		t.Error("expected cmdRunner to be updated")
	}
}
