package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
)

func TestLocalDB_OpenClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenLocalDB(dbPath, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("failed to close db: %v", err)
	}

	// Should be able to reopen
	db2, err := OpenLocalDB(dbPath, nil)
	if err != nil {
		t.Fatalf("failed to reopen db: %v", err)
	}
	defer func() { _ = db2.Close() }()
}

func TestLocalDB_StoreAndGetObjective(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	payload := &ObjectivePayload{
		Objective: Objective{
			ID:          "obj-123",
			Title:       "Test Objective",
			Description: "Test description",
			Hat:         "explorer",
			BaseBranch:  "main",
			TokenBudget: 10000,
			Checklist:   []string{"item1", "item2"},
		},
		Project: Project{
			ID:          "proj-456",
			Name:        "test-project",
			GitHubOwner: "owner",
			GitHubRepo:  "repo",
		},
		DispatchedAt: time.Now(),
		HQPublicKey:  "age1...",
	}

	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}

	// Retrieve it
	obj, err := db.GetObjective("obj-123")
	if err != nil {
		t.Fatalf("failed to get objective: %v", err)
	}
	if obj == nil {
		t.Fatal("expected objective, got nil")
	}

	if obj.ID != "obj-123" {
		t.Errorf("expected ID obj-123, got %s", obj.ID)
	}
	if obj.Title != "Test Objective" {
		t.Errorf("expected title 'Test Objective', got %s", obj.Title)
	}
	if obj.Hat != "explorer" {
		t.Errorf("expected hat explorer, got %s", obj.Hat)
	}
	if len(obj.Checklist) != 2 {
		t.Errorf("expected 2 checklist items, got %d", len(obj.Checklist))
	}
}

func TestLocalDB_GetNonexistentObjective(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	obj, err := db.GetObjective("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj != nil {
		t.Error("expected nil for nonexistent objective")
	}
}

func TestLocalDB_UpdateObjectiveStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	payload := &ObjectivePayload{
		Objective: Objective{
			ID:    "obj-123",
			Title: "Test",
			Hat:   "explorer",
		},
		Project:      Project{ID: "proj-1"},
		DispatchedAt: time.Now(),
	}
	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}

	// Update status
	if err := db.UpdateObjectiveStatus("obj-123", "completed"); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
}

func TestLocalDB_Sessions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Store objective first (FK constraint)
	payload := &ObjectivePayload{
		Objective: Objective{ID: "obj-123", Title: "Test", Hat: "explorer"},
		Project:   Project{ID: "proj-1"},
	}
	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}

	// Create session
	if err := db.CreateSession("sess-123", "obj-123", "explorer"); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Update session status
	if err := db.UpdateSessionStatus("sess-123", "running"); err != nil {
		t.Fatalf("failed to update session status: %v", err)
	}

	// Increment iteration
	if err := db.IncrementSessionIteration("sess-123"); err != nil {
		t.Fatalf("failed to increment iteration: %v", err)
	}
}

func TestLocalDB_Activity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Store objective and session first
	payload := &ObjectivePayload{
		Objective: Objective{ID: "obj-123", Title: "Test", Hat: "explorer"},
		Project:   Project{ID: "proj-1"},
	}
	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}
	if err := db.CreateSession("sess-123", "obj-123", "explorer"); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Record activity
	event := &ActivityEvent{
		ID:           "act-1",
		SessionID:    "sess-123",
		ObjectiveID:  "obj-123",
		Iteration:    1,
		EventType:    "assistant_response",
		Content:      "test content",
		TokensInput:  100,
		TokensOutput: 50,
		Hat:          "explorer",
		CreatedAt:    time.Now(),
	}
	if err := db.RecordActivity(event); err != nil {
		t.Fatalf("failed to record activity: %v", err)
	}

	// Get unsynced activity
	events, err := db.GetUnsyncedActivity(10)
	if err != nil {
		t.Fatalf("failed to get unsynced activity: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 unsynced event, got %d", len(events))
	}

	// Mark as synced
	if err := db.MarkActivitySynced([]string{"act-1"}); err != nil {
		t.Fatalf("failed to mark activity synced: %v", err)
	}

	// Should be no more unsynced
	events, err = db.GetUnsyncedActivity(10)
	if err != nil {
		t.Fatalf("failed to get unsynced activity: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 unsynced events, got %d", len(events))
	}
}

func TestLocalDB_TokenUsage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Store objective and session
	payload := &ObjectivePayload{
		Objective: Objective{ID: "obj-123", Title: "Test", Hat: "explorer"},
		Project:   Project{ID: "proj-1"},
	}
	if err := db.StoreObjective(payload); err != nil {
		t.Fatalf("failed to store objective: %v", err)
	}
	if err := db.CreateSession("sess-123", "obj-123", "explorer"); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Record multiple activities
	for i := 1; i <= 3; i++ {
		event := &ActivityEvent{
			ID:           "act-" + string(rune('0'+i)),
			SessionID:    "sess-123",
			ObjectiveID:  "obj-123",
			Iteration:    i,
			EventType:    "assistant_response",
			TokensInput:  100,
			TokensOutput: 50,
			CreatedAt:    time.Now(),
		}
		if err := db.RecordActivity(event); err != nil {
			t.Fatalf("failed to record activity %d: %v", i, err)
		}
	}

	// Get token usage
	input, output, err := db.GetObjectiveTokenUsage("obj-123")
	if err != nil {
		t.Fatalf("failed to get token usage: %v", err)
	}
	if input != 300 {
		t.Errorf("expected 300 input tokens, got %d", input)
	}
	if output != 150 {
		t.Errorf("expected 150 output tokens, got %d", output)
	}

	// Get iteration count
	count, err := db.GetObjectiveIterationCount("obj-123")
	if err != nil {
		t.Fatalf("failed to get iteration count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 iterations, got %d", count)
	}
}

func TestLocalDB_Secrets(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Store and retrieve secret (unencrypted mode)
	if err := db.StoreSecret("api_key", "secret-value"); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	value, err := db.GetSecret("api_key")
	if err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	if value != "secret-value" {
		t.Errorf("expected 'secret-value', got %s", value)
	}

	// Get nonexistent secret
	value, err = db.GetSecret("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "" {
		t.Errorf("expected empty string for nonexistent secret, got %s", value)
	}
}

func TestLocalDB_RecordSync(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.RecordSync("activity", "act-123", "ack-456"); err != nil {
		t.Fatalf("failed to record sync: %v", err)
	}
}

func TestLocalDB_MarkActivitySynced_EmptyList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Should handle empty list gracefully
	if err := db.MarkActivitySynced([]string{}); err != nil {
		t.Fatalf("failed to mark empty activity list synced: %v", err)
	}

	if err := db.MarkActivitySynced(nil); err != nil {
		t.Fatalf("failed to mark nil activity list synced: %v", err)
	}
}

// ====================
// Local DB Encryption Tests
// ====================

func TestLocalDB_SecretsWithEncryption(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a master key for encryption
	keyPath := filepath.Join(tmpDir, "master.key")
	masterKey, err := crypto.EnsureMasterKey(keyPath)
	if err != nil {
		t.Fatalf("failed to create master key: %v", err)
	}

	// Open DB with encryption enabled
	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), masterKey)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Store a secret (should be encrypted)
	secretValue := "super-secret-api-key-12345"
	if err := db.StoreSecret("api_key", secretValue); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	// Retrieve and decrypt
	retrieved, err := db.GetSecret("api_key")
	if err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	if retrieved != secretValue {
		t.Errorf("expected '%s', got '%s'", secretValue, retrieved)
	}

	// Verify it's actually encrypted in the DB by opening without encryption
	dbNoEnc, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db without encryption: %v", err)
	}
	defer func() { _ = dbNoEnc.Close() }()

	// Get the raw value (encrypted)
	rawValue, err := dbNoEnc.GetSecret("api_key")
	if err != nil {
		t.Fatalf("failed to get raw secret: %v", err)
	}

	// Raw value should NOT match the original (it's encrypted)
	if rawValue == secretValue {
		t.Error("secret should be encrypted in DB, but got plaintext")
	}
}

func TestLocalDB_SecretsUpdateWithEncryption(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	keyPath := filepath.Join(tmpDir, "master.key")
	masterKey, err := crypto.EnsureMasterKey(keyPath)
	if err != nil {
		t.Fatalf("failed to create master key: %v", err)
	}

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), masterKey)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Store initial value
	if err := db.StoreSecret("token", "value1"); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	// Update it
	if err := db.StoreSecret("token", "value2"); err != nil {
		t.Fatalf("failed to update secret: %v", err)
	}

	// Verify updated value
	value, err := db.GetSecret("token")
	if err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	if value != "value2" {
		t.Errorf("expected 'value2', got '%s'", value)
	}
}

// ====================
// Session State Tests (for crash recovery)
// ====================

func TestLocalDB_SaveAndGetSessionState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Save session state
	state := &SessionState{
		SessionID:       "sess-123",
		ObjectiveID:     "obj-456",
		Hat:             "creator",
		Iteration:       5,
		TokensInput:     1000,
		TokensOutput:    500,
		Conversation:    `[{"role":"user","content":"hello"}]`,
		Scratchpad:      "notes here",
		ChecklistDone:   []string{"item1", "item2"},
		ChecklistFailed: []string{"item3"},
		HatHistory:      `[{"Hat":"explorer","StartedAt":"2024-01-01T00:00:00Z"}]`,
		TransitionCount: 2,
		PreviousHat:     "explorer",
		Status:          "running",
		WorkDir:         "/tmp/work",
	}

	if err := db.SaveSessionState(state); err != nil {
		t.Fatalf("failed to save session state: %v", err)
	}

	// Get incomplete session
	retrieved, err := db.GetIncompleteSession()
	if err != nil {
		t.Fatalf("failed to get incomplete session: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected session state, got nil")
	}

	// Verify fields
	if retrieved.SessionID != "sess-123" {
		t.Errorf("expected session ID 'sess-123', got '%s'", retrieved.SessionID)
	}
	if retrieved.ObjectiveID != "obj-456" {
		t.Errorf("expected objective ID 'obj-456', got '%s'", retrieved.ObjectiveID)
	}
	if retrieved.Hat != "creator" {
		t.Errorf("expected hat 'creator', got '%s'", retrieved.Hat)
	}
	if retrieved.Iteration != 5 {
		t.Errorf("expected iteration 5, got %d", retrieved.Iteration)
	}
	if retrieved.TokensInput != 1000 {
		t.Errorf("expected tokens input 1000, got %d", retrieved.TokensInput)
	}
	if len(retrieved.ChecklistDone) != 2 {
		t.Errorf("expected 2 checklist done items, got %d", len(retrieved.ChecklistDone))
	}
	if len(retrieved.ChecklistFailed) != 1 {
		t.Errorf("expected 1 checklist failed item, got %d", len(retrieved.ChecklistFailed))
	}
	if retrieved.TransitionCount != 2 {
		t.Errorf("expected transition count 2, got %d", retrieved.TransitionCount)
	}
	if retrieved.PreviousHat != "explorer" {
		t.Errorf("expected previous hat 'explorer', got '%s'", retrieved.PreviousHat)
	}
}

func TestLocalDB_GetIncompleteSession_None(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// No sessions saved - should return nil
	session, err := db.GetIncompleteSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session != nil {
		t.Error("expected nil for no incomplete sessions")
	}
}

func TestLocalDB_MarkSessionComplete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Save a running session
	state := &SessionState{
		SessionID:   "sess-123",
		ObjectiveID: "obj-456",
		Hat:         "creator",
		Status:      "running",
	}
	if err := db.SaveSessionState(state); err != nil {
		t.Fatalf("failed to save session state: %v", err)
	}

	// Verify it's found as incomplete
	session, err := db.GetIncompleteSession()
	if err != nil {
		t.Fatalf("failed to get incomplete session: %v", err)
	}
	if session == nil {
		t.Fatal("expected running session")
	}

	// Mark as complete
	if err := db.MarkSessionComplete("sess-123", "completed"); err != nil {
		t.Fatalf("failed to mark session complete: %v", err)
	}

	// Should no longer be found as incomplete
	session, err = db.GetIncompleteSession()
	if err != nil {
		t.Fatalf("failed to get incomplete session: %v", err)
	}
	if session != nil {
		t.Errorf("expected nil after marking complete, got session with status '%s'", session.Status)
	}
}

func TestLocalDB_DeleteSessionState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Save a session
	state := &SessionState{
		SessionID:   "sess-123",
		ObjectiveID: "obj-456",
		Hat:         "creator",
		Status:      "running",
	}
	if err := db.SaveSessionState(state); err != nil {
		t.Fatalf("failed to save session state: %v", err)
	}

	// Delete it
	if err := db.DeleteSessionState("sess-123"); err != nil {
		t.Fatalf("failed to delete session state: %v", err)
	}

	// Should be gone
	session, err := db.GetIncompleteSession()
	if err != nil {
		t.Fatalf("failed to get incomplete session: %v", err)
	}
	if session != nil {
		t.Error("expected nil after delete")
	}
}

func TestLocalDB_SessionStateUpdate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	db, err := OpenLocalDB(filepath.Join(tmpDir, "test.db"), nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Save initial state
	state := &SessionState{
		SessionID:   "sess-123",
		ObjectiveID: "obj-456",
		Hat:         "creator",
		Iteration:   1,
		Status:      "running",
	}
	if err := db.SaveSessionState(state); err != nil {
		t.Fatalf("failed to save session state: %v", err)
	}

	// Update with new iteration
	state.Iteration = 5
	state.TokensInput = 2000
	if err := db.SaveSessionState(state); err != nil {
		t.Fatalf("failed to update session state: %v", err)
	}

	// Verify update
	retrieved, err := db.GetIncompleteSession()
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if retrieved.Iteration != 5 {
		t.Errorf("expected iteration 5, got %d", retrieved.Iteration)
	}
	if retrieved.TokensInput != 2000 {
		t.Errorf("expected tokens input 2000, got %d", retrieved.TokensInput)
	}
}
