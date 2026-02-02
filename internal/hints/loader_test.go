package hints

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader("/some/path")
	if loader == nil {
		t.Error("Expected non-nil loader")
	}
	if loader.workDir != "/some/path" {
		t.Errorf("Expected workDir to be /some/path, got %s", loader.workDir)
	}
}

func TestLoader_Load_NoFiles(t *testing.T) {
	// Create temp directory with no hint files
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	loader := NewLoader(tmpDir)
	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty content, got: %s", content)
	}
}

func TestLoader_Load_SingleFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a .dexhints file
	hintsContent := "# Test Hints\n\nSome project hints here."
	hintsPath := filepath.Join(tmpDir, ".dexhints")
	if err := os.WriteFile(hintsPath, []byte(hintsContent), 0644); err != nil {
		t.Fatalf("Failed to write hints file: %v", err)
	}

	loader := NewLoader(tmpDir)
	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(content, "Test Hints") {
		t.Error("Expected content to contain 'Test Hints'")
	}
	if !strings.Contains(content, "Project Context") {
		t.Error("Expected content to contain 'Project Context' header")
	}
}

func TestLoader_Load_MultipleFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple hint files
	files := map[string]string{
		".dexhints": "# Dex Hints\nDex specific content",
		"AGENTS.md": "# Agent Hints\nAgent specific content",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", name, err)
		}
	}

	loader := NewLoader(tmpDir)
	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(content, "Dex Hints") {
		t.Error("Expected content to contain 'Dex Hints'")
	}
	if !strings.Contains(content, "Agent Hints") {
		t.Error("Expected content to contain 'Agent Hints'")
	}
}

func TestLoader_ProcessImports_Simple(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create main hints file with import
	mainContent := `# Main Hints
@extra.md
## More content`

	extraContent := `# Extra Content
This is imported.`

	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "extra.md"), []byte(extraContent), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(content, "Extra Content") {
		t.Error("Expected imported content")
	}
	if !strings.Contains(content, "Imported: extra.md") {
		t.Error("Expected import comment")
	}
}

func TestLoader_ProcessImports_MissingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hints file with import to non-existent file
	mainContent := `# Main Hints
@missing.md
## More content`

	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// Missing imports result in a comment being added but the load should still succeed
	// The comment format is <!-- Import not found: missing.md -->
	if !strings.Contains(content, "Main Hints") {
		t.Error("Expected main content to still be present")
	}
	if !strings.Contains(content, "More content") {
		t.Error("Expected content after import to be present")
	}
}

func TestLoader_ProcessImports_MaxDepth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a chain of imports
	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte("@level1.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "level1.md"), []byte("@level2.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "level2.md"), []byte("@level3.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "level3.md"), []byte("@level4.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "level4.md"), []byte("Final content"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	loader.SetConfig(Config{
		Enabled:        true,
		MaxTotalSize:   50 * 1024,
		MaxImportDepth: 2, // Only 2 levels deep
	})

	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// At depth 2, level3.md should be included as raw @level4.md text, not resolved
	if strings.Contains(content, "Final content") {
		t.Error("Should not have resolved imports beyond max depth")
	}
}

func TestLoader_SizeLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a large hints file
	largeContent := strings.Repeat("x", 100*1024) // 100KB
	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte(largeContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a second file that should not be loaded due to size limit
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("Should not appear"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	loader.SetConfig(Config{
		Enabled:        true,
		MaxTotalSize:   50 * 1024, // 50KB limit
		MaxImportDepth: 3,
	})

	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// Second file should not be included
	if strings.Contains(content, "Should not appear") {
		t.Error("Expected size limit to prevent loading additional files")
	}
}

func TestLoader_Disabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a hints file
	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte("Content"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	loader.SetConfig(Config{
		Enabled: false,
	})

	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != "" {
		t.Error("Expected empty content when disabled")
	}
}

func TestFindGitRoot_NoRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	root := findGitRoot(tmpDir)
	if root != "" {
		t.Errorf("Expected empty string for non-git directory, got %s", root)
	}
}

func TestFindGitRoot_WithRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .git directory
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	root := findGitRoot(subDir)
	if root != tmpDir {
		t.Errorf("Expected git root to be %s, got %s", tmpDir, root)
	}
}

func TestLoader_DirectoryChain(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .git directory at root
	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create nested directories
	subDir := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create hints at different levels
	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte("root"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "a", ".dexhints"), []byte("level a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, ".dexhints"), []byte("level c"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(subDir)
	content, err := loader.Load()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// All hints should be loaded in order
	if !strings.Contains(content, "root") {
		t.Error("Expected root hints")
	}
	if !strings.Contains(content, "level a") {
		t.Error("Expected level a hints")
	}
	if !strings.Contains(content, "level c") {
		t.Error("Expected level c hints")
	}

	// Root should come first
	rootIdx := strings.Index(content, "root")
	aIdx := strings.Index(content, "level a")
	cIdx := strings.Index(content, "level c")

	if rootIdx > aIdx || aIdx > cIdx {
		t.Error("Expected hints in order: root, a, c")
	}
}

func TestLoader_Validate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hints-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a valid hints file
	if err := os.WriteFile(filepath.Join(tmpDir, ".dexhints"), []byte("Valid content"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	errors := loader.Validate()

	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d", len(errors))
	}
}
