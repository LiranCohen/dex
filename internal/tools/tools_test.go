package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestToolSet(t *testing.T) {
	set := ReadOnlyTools()

	// Test Has
	if !set.Has("read_file") {
		t.Error("Expected read_file in ReadOnlyTools")
	}
	if set.Has("write_file") {
		t.Error("Did not expect write_file in ReadOnlyTools")
	}

	// Test Get
	tool := set.Get("read_file")
	if tool == nil {
		t.Fatal("Expected to get read_file tool")
	}
	if tool.Name != "read_file" {
		t.Errorf("Expected name 'read_file', got '%s'", tool.Name)
	}
	if !tool.ReadOnly {
		t.Error("Expected read_file to be ReadOnly")
	}

	// Test All
	all := set.All()
	if len(all) != 16 { // 10 read-only tools + 6 read-only mail/calendar tools
		t.Errorf("Expected 16 tools, got %d", len(all))
	}
}

func TestReadWriteToolSet(t *testing.T) {
	set := ReadWriteTools()

	// Should have both read and write tools
	if !set.Has("read_file") {
		t.Error("Expected read_file in ReadWriteTools")
	}
	if !set.Has("write_file") {
		t.Error("Expected write_file in ReadWriteTools")
	}
	if !set.Has("bash") {
		t.Error("Expected bash in ReadWriteTools")
	}

	// Count total tools
	all := set.All()
	if len(all) != 34 { // 10 read-only + 8 write + 4 quality gate + 12 mail/calendar tools
		t.Errorf("Expected 34 tools, got %d", len(all))
	}
}

func TestToAnthropicFormat(t *testing.T) {
	set := NewSet([]Tool{ReadFileTool()})
	format := set.ToAnthropicFormat()

	if len(format) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(format))
	}

	tool := format[0]
	if tool["name"] != "read_file" {
		t.Errorf("Expected name 'read_file', got '%v'", tool["name"])
	}
	if tool["description"] == nil || tool["description"] == "" {
		t.Error("Expected description to be set")
	}
	if tool["input_schema"] == nil {
		t.Error("Expected input_schema to be set")
	}
}

func TestExecutor(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "tools-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	exec := NewExecutor(tmpDir, ReadOnlyTools(), true)

	t.Run("read_file", func(t *testing.T) {
		result := exec.Execute(ctx, "read_file", map[string]any{"path": "test.txt"})
		if result.IsError {
			t.Errorf("Unexpected error: %s", result.Output)
		}
		if result.Output != "hello world" {
			t.Errorf("Expected 'hello world', got '%s'", result.Output)
		}
	})

	t.Run("read_file path traversal", func(t *testing.T) {
		result := exec.Execute(ctx, "read_file", map[string]any{"path": "../../../etc/passwd"})
		if !result.IsError {
			t.Error("Expected error for path traversal")
		}
	})

	t.Run("list_files", func(t *testing.T) {
		result := exec.Execute(ctx, "list_files", map[string]any{})
		if result.IsError {
			t.Errorf("Unexpected error: %s", result.Output)
		}
		if result.Output == "" {
			t.Error("Expected non-empty output")
		}
	})

	t.Run("glob", func(t *testing.T) {
		result := exec.Execute(ctx, "glob", map[string]any{"pattern": "**/*.txt"})
		if result.IsError {
			t.Errorf("Unexpected error: %s", result.Output)
		}
		if result.Output == "" {
			t.Error("Expected files to match")
		}
	})

	t.Run("write_file blocked in read-only", func(t *testing.T) {
		result := exec.Execute(ctx, "write_file", map[string]any{"path": "new.txt", "content": "test"})
		if !result.IsError {
			t.Error("Expected write_file to be blocked in read-only mode")
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		result := exec.Execute(ctx, "unknown_tool", map[string]any{})
		if !result.IsError {
			t.Error("Expected error for unknown tool")
		}
	})
}

func TestExecutorWriteMode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ctx := context.Background()
	exec := NewExecutor(tmpDir, ReadWriteTools(), false)

	t.Run("write_file", func(t *testing.T) {
		result := exec.Execute(ctx, "write_file", map[string]any{
			"path":    "new.txt",
			"content": "test content",
		})
		if result.IsError {
			t.Errorf("Unexpected error: %s", result.Output)
		}

		// Verify file was written
		content, err := os.ReadFile(filepath.Join(tmpDir, "new.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "test content" {
			t.Errorf("Expected 'test content', got '%s'", string(content))
		}
	})

	t.Run("bash", func(t *testing.T) {
		result := exec.Execute(ctx, "bash", map[string]any{"command": "echo hello"})
		if result.IsError {
			t.Errorf("Unexpected error: %s", result.Output)
		}
		if result.Output != "hello\n" {
			t.Errorf("Expected 'hello\\n', got '%s'", result.Output)
		}
	})

	t.Run("bash dangerous command", func(t *testing.T) {
		result := exec.Execute(ctx, "bash", map[string]any{"command": "rm -rf /"})
		if !result.IsError {
			t.Error("Expected dangerous command to be blocked")
		}
	})
}
