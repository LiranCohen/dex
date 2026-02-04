package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Large response handling constants
const (
	LargeResponseThreshold = 200_000 // characters - responses larger than this go to temp files
	PreviewLength          = 1000    // characters to show in preview
	TempDirName            = "dex_tool_responses"
)

// ProcessLargeResponse handles large tool outputs by writing to temp files.
// If the response is under the threshold, it's returned unchanged.
// If larger, it's written to a temp file and a summary with the path is returned.
//
// Inspired by Goose's large_response_handler.
func ProcessLargeResponse(toolName string, result string) string {
	if len(result) <= LargeResponseThreshold {
		return result
	}

	// Create temp directory if needed
	tempDir := filepath.Join(os.TempDir(), TempDirName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		// Fall back to truncation if we can't create temp dir
		return truncateWithWarning(result, LargeResponseThreshold)
	}

	// Generate unique filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.txt", toolName, timestamp)
	filePath := filepath.Join(tempDir, filename)

	// Write full response to temp file
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		// Fall back to truncation if write fails
		return truncateWithWarning(result, LargeResponseThreshold)
	}

	// Create preview
	preview := result
	if len(preview) > PreviewLength {
		preview = preview[:PreviewLength]
	}

	return fmt.Sprintf(`Tool response too large (%d characters). Full output saved to: %s

To examine the output:
- Use read_file tool to view the file
- Use grep tool to search within the file
- The file will be cleaned up when the session ends

Preview (first %d chars):
%s...`,
		len(result),
		filePath,
		PreviewLength,
		preview)
}

// truncateWithWarning truncates a string and adds a warning message
func truncateWithWarning(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return fmt.Sprintf("%s\n\n... [TRUNCATED: Response was %d characters, showing first %d. Could not write to temp file.]",
		s[:maxLen], len(s), maxLen)
}

// CleanupTempResponses removes all temp files created by this session.
// Should be called when a session ends.
func CleanupTempResponses() error {
	tempDir := filepath.Join(os.TempDir(), TempDirName)

	// Check if directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		return nil // Nothing to clean up
	}

	return os.RemoveAll(tempDir)
}

// CleanupOldTempResponses removes temp files older than the given duration.
// Useful for cleaning up orphaned files from crashed sessions.
func CleanupOldTempResponses(maxAge time.Duration) error {
	tempDir := filepath.Join(os.TempDir(), TempDirName)

	// Check if directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			filePath := filepath.Join(tempDir, entry.Name())
			_ = os.Remove(filePath) // Ignore errors for individual files
		}
	}

	return nil
}

// GetTempResponseDir returns the path to the temp response directory
func GetTempResponseDir() string {
	return filepath.Join(os.TempDir(), TempDirName)
}
