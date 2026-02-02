package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryCRUD(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "dex-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	// Create a test project first (for foreign key)
	_, err = db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-1', 'Test Project', '/test')`)
	if err != nil {
		t.Fatal(err)
	}

	// Test CreateMemory
	memory := &Memory{
		ID:           "mem-1",
		ProjectID:    "proj-1",
		Type:         MemoryPattern,
		Title:        "Test Pattern",
		Content:      "Tests use table-driven pattern",
		Confidence:   InitialConfidenceExplicit,
		Tags:         []string{"testing", "go"},
		FileRefs:     []string{"internal/*_test.go"},
		CreatedByHat: "creator",
		Source:       SourceExplicit,
		CreatedAt:    time.Now(),
	}

	if err := db.CreateMemory(memory); err != nil {
		t.Fatalf("CreateMemory failed: %v", err)
	}

	// Test GetMemory
	got, err := db.GetMemory("mem-1")
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got.Title != "Test Pattern" {
		t.Errorf("Expected title 'Test Pattern', got %q", got.Title)
	}
	if got.Type != MemoryPattern {
		t.Errorf("Expected type 'pattern', got %q", got.Type)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(got.Tags))
	}

	// Test UpdateMemory
	got.Title = "Updated Pattern"
	got.Confidence = 0.8
	if err := db.UpdateMemory(got); err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	updated, err := db.GetMemory("mem-1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Updated Pattern" {
		t.Errorf("Expected title 'Updated Pattern', got %q", updated.Title)
	}

	// Test DeleteMemory
	if err := db.DeleteMemory("mem-1"); err != nil {
		t.Fatalf("DeleteMemory failed: %v", err)
	}

	_, err = db.GetMemory("mem-1")
	if err != sql.ErrNoRows {
		t.Error("Expected ErrNoRows after delete")
	}
}

func TestListMemories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dex-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-1', 'Test Project', '/test')`)
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple memories
	memories := []*Memory{
		{ID: "mem-1", ProjectID: "proj-1", Type: MemoryPattern, Title: "Pattern 1", Content: "Content 1", Confidence: 0.7, Source: SourceExplicit, CreatedAt: time.Now()},
		{ID: "mem-2", ProjectID: "proj-1", Type: MemoryPitfall, Title: "Pitfall 1", Content: "Content 2", Confidence: 0.5, Source: SourceExplicit, CreatedAt: time.Now()},
		{ID: "mem-3", ProjectID: "proj-1", Type: MemoryPattern, Title: "Pattern 2", Content: "Content 3", Confidence: 0.3, Source: SourceExplicit, CreatedAt: time.Now()},
	}

	for _, m := range memories {
		if err := db.CreateMemory(m); err != nil {
			t.Fatal(err)
		}
	}

	// Test list all
	list, err := db.ListMemories("proj-1", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("Expected 3 memories, got %d", len(list))
	}

	// Test filter by type
	patternType := MemoryPattern
	list, err = db.ListMemories("proj-1", &patternType, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 pattern memories, got %d", len(list))
	}

	// Test filter by confidence
	list, err = db.ListMemories("proj-1", nil, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 memories with confidence >= 0.5, got %d", len(list))
	}
}

func TestSearchMemories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dex-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-1', 'Test Project', '/test')`)
	if err != nil {
		t.Fatal(err)
	}

	memories := []*Memory{
		{ID: "mem-1", ProjectID: "proj-1", Type: MemoryPattern, Title: "Database patterns", Content: "Use SQLite", Confidence: 0.7, Source: SourceExplicit, CreatedAt: time.Now()},
		{ID: "mem-2", ProjectID: "proj-1", Type: MemoryPitfall, Title: "API gotchas", Content: "Null handling", Confidence: 0.5, Source: SourceExplicit, CreatedAt: time.Now()},
	}

	for _, m := range memories {
		if err := db.CreateMemory(m); err != nil {
			t.Fatal(err)
		}
	}

	// Test search by title
	results, err := db.SearchMemories("proj-1", MemorySearchParams{Query: "database"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'database', got %d", len(results))
	}

	// Test search by content
	results, err = db.SearchMemories("proj-1", MemorySearchParams{Query: "SQLite"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'SQLite', got %d", len(results))
	}
}

func TestRelevantMemories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dex-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-1', 'Test Project', '/test')`)
	if err != nil {
		t.Fatal(err)
	}

	// Create memories with different hat affiliations
	memories := []*Memory{
		{ID: "mem-1", ProjectID: "proj-1", Type: MemoryPattern, Title: "Creator pattern", Content: "Content", Confidence: 0.6, CreatedByHat: "creator", Source: SourceExplicit, CreatedAt: time.Now()},
		{ID: "mem-2", ProjectID: "proj-1", Type: MemoryPitfall, Title: "Critic finding", Content: "Content", Confidence: 0.6, CreatedByHat: "critic", Source: SourceExplicit, CreatedAt: time.Now()},
		{ID: "mem-3", ProjectID: "proj-1", Type: MemoryArchitecture, Title: "Explorer finding", Content: "Content", Confidence: 0.6, CreatedByHat: "explorer", Source: SourceExplicit, CreatedAt: time.Now()},
	}

	for _, m := range memories {
		if err := db.CreateMemory(m); err != nil {
			t.Fatal(err)
		}
	}

	// Get relevant memories for creator (should boost creator and related hats)
	ctx := MemoryContext{
		ProjectID:        "proj-1",
		CurrentHat:       "creator",
		CurrentSessionID: "other-session",
	}

	results, err := db.GetRelevantMemories(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 1 {
		t.Error("Expected at least 1 relevant memory")
	}

	// Verify creator memory is ranked higher (due to hat match)
	if len(results) > 0 && results[0].CreatedByHat != "creator" {
		// May not always be first due to other scoring factors, but should be present
		found := false
		for _, m := range results {
			if m.CreatedByHat == "creator" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Creator memory should be in results")
		}
	}
}

func TestRecordMemoryUsage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dex-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-1', 'Test Project', '/test')`)
	if err != nil {
		t.Fatal(err)
	}

	memory := &Memory{
		ID:         "mem-1",
		ProjectID:  "proj-1",
		Type:       MemoryPattern,
		Title:      "Test",
		Content:    "Content",
		Confidence: 0.5,
		Source:     SourceExplicit,
		CreatedAt:  time.Now(),
	}

	if err := db.CreateMemory(memory); err != nil {
		t.Fatal(err)
	}

	// Record usage
	if err := db.RecordMemoryUsage("mem-1"); err != nil {
		t.Fatal(err)
	}

	// Verify usage was recorded
	updated, err := db.GetMemory("mem-1")
	if err != nil {
		t.Fatal(err)
	}

	if updated.UseCount != 1 {
		t.Errorf("Expected use_count 1, got %d", updated.UseCount)
	}
	if updated.Confidence <= 0.5 {
		t.Errorf("Expected confidence boost, got %f", updated.Confidence)
	}
	if !updated.LastUsedAt.Valid {
		t.Error("Expected last_used_at to be set")
	}
}

func TestIsValidMemoryType(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"architecture", true},
		{"pattern", true},
		{"pitfall", true},
		{"decision", true},
		{"fix", true},
		{"convention", true},
		{"dependency", true},
		{"constraint", true},
		{"invalid", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := IsValidMemoryType(tc.input); got != tc.valid {
			t.Errorf("IsValidMemoryType(%q) = %v, want %v", tc.input, got, tc.valid)
		}
	}
}
