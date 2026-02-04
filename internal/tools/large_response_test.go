package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessLargeResponse_SmallResponse(t *testing.T) {
	input := "This is a small response"
	result := ProcessLargeResponse("test_tool", input)

	if result != input {
		t.Errorf("Small response should be unchanged, got: %s", result)
	}
}

func TestProcessLargeResponse_LargeResponse(t *testing.T) {
	// Clean up before and after test
	_ = CleanupTempResponses()
	defer func() { _ = CleanupTempResponses() }()

	// Create a response larger than the threshold
	input := strings.Repeat("x", LargeResponseThreshold+1000)
	result := ProcessLargeResponse("test_tool", input)

	// Should contain info about the temp file
	if !strings.Contains(result, "Tool response too large") {
		t.Error("Should indicate response was too large")
	}
	if !strings.Contains(result, "characters") {
		t.Error("Should mention character count")
	}
	if !strings.Contains(result, ".txt") {
		t.Error("Should mention the temp file path")
	}
	if !strings.Contains(result, "Preview") {
		t.Error("Should include a preview")
	}

	// Verify the temp file was created
	tempDir := GetTempResponseDir()
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("Expected temp file to be created")
	}

	// Verify the temp file contains the full content
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "test_tool_") {
			content, err := os.ReadFile(filepath.Join(tempDir, entry.Name()))
			if err != nil {
				t.Fatalf("Failed to read temp file: %v", err)
			}
			if len(content) != len(input) {
				t.Errorf("Temp file should contain full content, got %d bytes, want %d", len(content), len(input))
			}
		}
	}
}

func TestProcessLargeResponse_ExactThreshold(t *testing.T) {
	// Clean up before test
	_ = CleanupTempResponses()
	defer func() { _ = CleanupTempResponses() }()

	// Exactly at threshold should not create temp file
	input := strings.Repeat("x", LargeResponseThreshold)
	result := ProcessLargeResponse("test_tool", input)

	if result != input {
		t.Error("Response exactly at threshold should be unchanged")
	}
}

func TestTruncateWithWarning(t *testing.T) {
	input := strings.Repeat("x", 1000)

	result := truncateWithWarning(input, 100)

	if !strings.Contains(result, "TRUNCATED") {
		t.Error("Should contain truncation warning")
	}
	if !strings.Contains(result, "1000 characters") {
		t.Error("Should mention original length")
	}
}

func TestTruncateWithWarning_NoTruncationNeeded(t *testing.T) {
	input := "short string"
	result := truncateWithWarning(input, 100)

	if result != input {
		t.Error("Short string should be unchanged")
	}
}

func TestCleanupTempResponses(t *testing.T) {
	// Create temp directory and file
	tempDir := GetTempResponseDir()
	_ = os.MkdirAll(tempDir, 0755)

	testFile := filepath.Join(tempDir, "test_cleanup.txt")
	_ = os.WriteFile(testFile, []byte("test"), 0644)

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("Test file should exist before cleanup")
	}

	// Cleanup
	if err := CleanupTempResponses(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("Temp directory should be removed after cleanup")
	}
}

func TestCleanupOldTempResponses(t *testing.T) {
	// Create temp directory
	tempDir := GetTempResponseDir()
	_ = os.MkdirAll(tempDir, 0755)
	defer func() { _ = CleanupTempResponses() }()

	// Create an "old" file (we'll set its modtime to the past)
	oldFile := filepath.Join(tempDir, "old_file.txt")
	_ = os.WriteFile(oldFile, []byte("old"), 0644)

	// Set modtime to 2 hours ago
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(oldFile, oldTime, oldTime)

	// Create a "new" file
	newFile := filepath.Join(tempDir, "new_file.txt")
	_ = os.WriteFile(newFile, []byte("new"), 0644)

	// Cleanup files older than 1 hour
	if err := CleanupOldTempResponses(1 * time.Hour); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Old file should be gone
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("Old file should be removed")
	}

	// New file should still exist
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("New file should still exist")
	}
}

func TestCleanupTempResponses_NothingToClean(t *testing.T) {
	// Make sure temp dir doesn't exist
	tempDir := GetTempResponseDir()
	_ = os.RemoveAll(tempDir)

	// Should not error when nothing to clean
	if err := CleanupTempResponses(); err != nil {
		t.Errorf("Should not error when nothing to clean: %v", err)
	}
}
