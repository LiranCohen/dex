package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskContentPath(t *testing.T) {
	got := TaskContentPath("task-abc123")
	want := filepath.Join("tasks", "task-abc123")
	if got != want {
		t.Errorf("TaskContentPath() = %q, want %q", got, want)
	}
}

func TestQuestContentPath(t *testing.T) {
	got := QuestContentPath("quest-xyz789")
	want := filepath.Join("quests", "quest-xyz789")
	if got != want {
		t.Errorf("QuestContentPath() = %q, want %q", got, want)
	}
}

func TestManager_WriteAndReadFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)

	// Test writing and reading
	content := "test content"
	path := "test/nested/file.txt"

	err = mgr.writeFile(path, content)
	if err != nil {
		t.Fatalf("writeFile() error = %v", err)
	}

	got, err := mgr.readFile(path)
	if err != nil {
		t.Fatalf("readFile() error = %v", err)
	}

	if got != content {
		t.Errorf("readFile() = %q, want %q", got, content)
	}
}

func TestManager_FileExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)

	// File doesn't exist
	if mgr.fileExists("nonexistent.txt") {
		t.Error("fileExists() should return false for nonexistent file")
	}

	// Create file
	err = mgr.writeFile("exists.txt", "content")
	if err != nil {
		t.Fatalf("writeFile() error = %v", err)
	}

	// File exists
	if !mgr.fileExists("exists.txt") {
		t.Error("fileExists() should return true for existing file")
	}
}

func TestManager_ReadNonexistentFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)

	// Reading nonexistent file should return empty string, not error
	got, err := mgr.readFile("nonexistent.txt")
	if err != nil {
		t.Fatalf("readFile() error = %v, want nil for nonexistent file", err)
	}
	if got != "" {
		t.Errorf("readFile() = %q, want empty string for nonexistent file", got)
	}
}

func TestFormatTaskSpec(t *testing.T) {
	spec := TaskSpec{
		Title:       "Test Task",
		Description: "This is a test task.",
		ProjectName: "test-project",
		QuestID:     "quest-123",
		CreatedAt:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	result := FormatTaskSpec(spec)

	// Check key elements are present
	if !strings.Contains(result, "# Test Task") {
		t.Error("FormatTaskSpec() should contain title header")
	}
	if !strings.Contains(result, "This is a test task.") {
		t.Error("FormatTaskSpec() should contain description")
	}
	if !strings.Contains(result, "**Project:** test-project") {
		t.Error("FormatTaskSpec() should contain project name")
	}
	if !strings.Contains(result, "#quest-123") {
		t.Error("FormatTaskSpec() should contain quest ID")
	}
}

func TestFormatChecklist(t *testing.T) {
	items := []ChecklistItem{
		{Description: "Required item 1", Status: ChecklistPending, Category: "must_have"},
		{Description: "Required item 2", Status: ChecklistDone, Category: "must_have"},
		{Description: "Optional item", Status: ChecklistInProgress, Category: "optional"},
	}

	result := FormatChecklist(items)

	// Check structure
	if !strings.Contains(result, "# Acceptance Criteria") {
		t.Error("FormatChecklist() should contain main header")
	}
	if !strings.Contains(result, "## Must Have") {
		t.Error("FormatChecklist() should contain must_have section")
	}
	if !strings.Contains(result, "## Optional") {
		t.Error("FormatChecklist() should contain optional section")
	}

	// Check items
	if !strings.Contains(result, "[ ] Required item 1") {
		t.Error("FormatChecklist() should have unchecked pending item")
	}
	if !strings.Contains(result, "[x] Required item 2") {
		t.Error("FormatChecklist() should have checked done item")
	}
	if !strings.Contains(result, "*(in progress)*") {
		t.Error("FormatChecklist() should have in progress marker")
	}
}

func TestParseChecklist(t *testing.T) {
	content := `# Acceptance Criteria

## Must Have

- [ ] First required item
- [x] Completed item
- [-] Skipped item *(skipped)*

## Optional

- [ ] Optional enhancement *(in progress)*
`

	items := ParseChecklist(content)

	if len(items) != 4 {
		t.Fatalf("ParseChecklist() returned %d items, want 4", len(items))
	}

	// Check categories
	mustHave := 0
	optional := 0
	for _, item := range items {
		if item.Category == "must_have" {
			mustHave++
		} else if item.Category == "optional" {
			optional++
		}
	}

	if mustHave != 3 {
		t.Errorf("ParseChecklist() found %d must_have items, want 3", mustHave)
	}
	if optional != 1 {
		t.Errorf("ParseChecklist() found %d optional items, want 1", optional)
	}

	// Check statuses
	if items[0].Status != ChecklistPending {
		t.Errorf("First item status = %v, want pending", items[0].Status)
	}
	if items[1].Status != ChecklistDone {
		t.Errorf("Second item status = %v, want done", items[1].Status)
	}
	if items[2].Status != ChecklistSkipped {
		t.Errorf("Third item status = %v, want skipped", items[2].Status)
	}
	if items[3].Status != ChecklistInProgress {
		t.Errorf("Fourth item status = %v, want in_progress", items[3].Status)
	}
}

func TestUpdateChecklistItemStatus(t *testing.T) {
	content := `# Acceptance Criteria

## Must Have

- [ ] Item to update
- [x] Already done

---
`

	// Update to in_progress
	updated := UpdateChecklistItemStatus(content, "Item to update", ChecklistInProgress)

	if !strings.Contains(updated, "[ ] Item to update *(in progress)*") {
		t.Error("UpdateChecklistItemStatus() should add in_progress marker")
	}

	// Update to done
	updated = UpdateChecklistItemStatus(content, "Item to update", ChecklistDone)

	if !strings.Contains(updated, "[x] Item to update") {
		t.Error("UpdateChecklistItemStatus() should change checkbox to done")
	}
}

func TestChecklistFromPlanningItems(t *testing.T) {
	mustHave := []string{"Required step 1", "Required step 2"}
	optional := []string{"Nice to have"}

	items := ChecklistFromPlanningItems(mustHave, optional)

	if len(items) != 3 {
		t.Fatalf("ChecklistFromPlanningItems() returned %d items, want 3", len(items))
	}

	// Check must_have items
	if items[0].Category != "must_have" || items[0].Description != "Required step 1" {
		t.Errorf("First item = %+v, want must_have 'Required step 1'", items[0])
	}
	if items[1].Category != "must_have" || items[1].Description != "Required step 2" {
		t.Errorf("Second item = %+v, want must_have 'Required step 2'", items[1])
	}

	// Check optional item
	if items[2].Category != "optional" || items[2].Description != "Nice to have" {
		t.Errorf("Third item = %+v, want optional 'Nice to have'", items[2])
	}

	// All should be pending
	for _, item := range items {
		if item.Status != ChecklistPending {
			t.Errorf("Item %q status = %v, want pending", item.Description, item.Status)
		}
	}
}

func TestManager_TaskContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)
	taskID := "task-test123"

	// Test InitTaskContent
	checklist := []ChecklistItem{
		{Description: "Test item", Status: ChecklistPending, Category: "must_have"},
	}

	err = mgr.InitTaskContent(taskID, "Test Title", "Test description", "test-project", "quest-1", checklist)
	if err != nil {
		t.Fatalf("InitTaskContent() error = %v", err)
	}

	// Verify content exists
	if !mgr.TaskContentExists(taskID) {
		t.Error("TaskContentExists() should return true after InitTaskContent()")
	}

	// Test ReadTaskSpec
	spec, err := mgr.ReadTaskSpec(taskID)
	if err != nil {
		t.Fatalf("ReadTaskSpec() error = %v", err)
	}
	if !strings.Contains(spec, "Test Title") {
		t.Error("ReadTaskSpec() should contain title")
	}

	// Test ReadTaskChecklist
	readItems, err := mgr.ReadTaskChecklist(taskID)
	if err != nil {
		t.Fatalf("ReadTaskChecklist() error = %v", err)
	}
	if len(readItems) != 1 {
		t.Errorf("ReadTaskChecklist() returned %d items, want 1", len(readItems))
	}

	// Test UpdateTaskChecklistItem
	err = mgr.UpdateTaskChecklistItem(taskID, "Test item", ChecklistDone)
	if err != nil {
		t.Fatalf("UpdateTaskChecklistItem() error = %v", err)
	}

	// Verify update
	readItems, _ = mgr.ReadTaskChecklist(taskID)
	if len(readItems) > 0 && readItems[0].Status != ChecklistDone {
		t.Error("UpdateTaskChecklistItem() should update status to done")
	}
}

func TestManager_QuestContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)
	questID := "quest-test456"

	// Test InitQuestContent
	err = mgr.InitQuestContent(questID, "Test Quest")
	if err != nil {
		t.Fatalf("InitQuestContent() error = %v", err)
	}

	// Verify content exists
	if !mgr.QuestConversationExists(questID) {
		t.Error("QuestConversationExists() should return true after InitQuestContent()")
	}

	// Test AppendQuestMessage
	err = mgr.AppendQuestMessage(questID, ConversationMessage{
		Role:      "user",
		Content:   "Hello, this is a test message",
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("AppendQuestMessage() error = %v", err)
	}

	err = mgr.AppendQuestMessage(questID, ConversationMessage{
		Role:      "assistant",
		Content:   "Hello! How can I help you?",
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("AppendQuestMessage() error = %v", err)
	}

	// Test ReadQuestConversation
	conv, err := mgr.ReadQuestConversation(questID)
	if err != nil {
		t.Fatalf("ReadQuestConversation() error = %v", err)
	}

	if !strings.Contains(conv, "Hello, this is a test message") {
		t.Error("ReadQuestConversation() should contain user message")
	}
	if !strings.Contains(conv, "Hello! How can I help you?") {
		t.Error("ReadQuestConversation() should contain assistant message")
	}
	if !strings.Contains(conv, "## User") {
		t.Error("ReadQuestConversation() should contain User header")
	}
	if !strings.Contains(conv, "## Dex") {
		t.Error("ReadQuestConversation() should contain Dex header")
	}
}

func TestFormatConversation(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "user", Content: "Test user message", Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)},
		{Role: "assistant", Content: "Test assistant response", Timestamp: time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC)},
	}

	result := FormatConversation(messages)

	if !strings.Contains(result, "# Quest Conversation") {
		t.Error("FormatConversation() should contain main header")
	}
	if !strings.Contains(result, "## User _10:30_") {
		t.Error("FormatConversation() should contain user header with timestamp")
	}
	if !strings.Contains(result, "## Dex _10:31_") {
		t.Error("FormatConversation() should contain Dex header with timestamp")
	}
}

func TestManager_GetPaths(t *testing.T) {
	mgr := NewManager("/test/base")

	taskPath := mgr.GetTaskContentPath("task-123")
	if taskPath != "/test/base/tasks/task-123" {
		t.Errorf("GetTaskContentPath() = %q, want %q", taskPath, "/test/base/tasks/task-123")
	}

	questPath := mgr.GetQuestContentPath("quest-456")
	if questPath != "/test/base/quests/quest-456" {
		t.Errorf("GetQuestContentPath() = %q, want %q", questPath, "/test/base/quests/quest-456")
	}

	specPath := mgr.GetTaskSpecPath("task-123")
	if specPath != "/test/base/tasks/task-123/spec.md" {
		t.Errorf("GetTaskSpecPath() = %q, want %q", specPath, "/test/base/tasks/task-123/spec.md")
	}

	checklistPath := mgr.GetTaskChecklistPath("task-123")
	if checklistPath != "/test/base/tasks/task-123/checklist.md" {
		t.Errorf("GetTaskChecklistPath() = %q, want %q", checklistPath, "/test/base/tasks/task-123/checklist.md")
	}

	convPath := mgr.GetQuestConversationPath("quest-456")
	if convPath != "/test/base/quests/quest-456/conversation.md" {
		t.Errorf("GetQuestConversationPath() = %q, want %q", convPath, "/test/base/quests/quest-456/conversation.md")
	}
}

func TestManager_AddQuestTask(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)
	questID := "quest-task-test"

	// Initialize quest first
	err = mgr.InitQuestContent(questID, "Test Quest")
	if err != nil {
		t.Fatalf("InitQuestContent() error = %v", err)
	}

	// Add a task reference
	err = mgr.AddQuestTask(questID, "task-123", "Implement feature X")
	if err != nil {
		t.Fatalf("AddQuestTask() error = %v", err)
	}

	// Verify task reference was added
	conv, err := mgr.ReadQuestConversation(questID)
	if err != nil {
		t.Fatalf("ReadQuestConversation() error = %v", err)
	}

	if !strings.Contains(conv, "Implement feature X") {
		t.Error("AddQuestTask() should add task title to conversation")
	}
	if !strings.Contains(conv, "task-123") {
		t.Error("AddQuestTask() should add task ID to conversation")
	}
	if !strings.Contains(conv, "Task Created") {
		t.Error("AddQuestTask() should add 'Task Created' marker")
	}
}

func TestManager_CompleteQuestContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)
	questID := "quest-complete-test"

	// Initialize quest first
	err = mgr.InitQuestContent(questID, "Test Quest")
	if err != nil {
		t.Fatalf("InitQuestContent() error = %v", err)
	}

	// Add a message
	err = mgr.AppendQuestMessage(questID, ConversationMessage{
		Role:    "user",
		Content: "Test message",
	})
	if err != nil {
		t.Fatalf("AppendQuestMessage() error = %v", err)
	}

	// Complete the quest
	summary := "Successfully implemented the feature with all acceptance criteria met."
	err = mgr.CompleteQuestContent(questID, summary)
	if err != nil {
		t.Fatalf("CompleteQuestContent() error = %v", err)
	}

	// Verify completion was added
	conv, err := mgr.ReadQuestConversation(questID)
	if err != nil {
		t.Fatalf("ReadQuestConversation() error = %v", err)
	}

	if !strings.Contains(conv, "Quest Completed") {
		t.Error("CompleteQuestContent() should add 'Quest Completed' section")
	}
	if !strings.Contains(conv, summary) {
		t.Error("CompleteQuestContent() should include the summary")
	}
}

func TestManager_WriteQuestTasksList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)
	questID := "quest-tasks-list"

	// Write tasks list
	tasks := []QuestTask{
		{ID: "task-1", Title: "First task", Status: "completed", Completed: true},
		{ID: "task-2", Title: "Second task", Status: "running", Completed: false},
		{ID: "task-3", Title: "Third task", Status: "pending", Completed: false},
	}

	err = mgr.WriteQuestTasksList(questID, tasks)
	if err != nil {
		t.Fatalf("WriteQuestTasksList() error = %v", err)
	}

	// Read and verify
	tasksPath := filepath.Join(QuestContentPath(questID), "tasks.md")
	content, err := mgr.readFile(tasksPath)
	if err != nil {
		t.Fatalf("Failed to read tasks file: %v", err)
	}

	// Check content
	if !strings.Contains(content, "# Quest Tasks") {
		t.Error("WriteQuestTasksList() should have header")
	}
	if !strings.Contains(content, "[x] [First task]") {
		t.Error("WriteQuestTasksList() should show completed task with checkbox")
	}
	if !strings.Contains(content, "[ ] [Second task]") {
		t.Error("WriteQuestTasksList() should show incomplete task with empty checkbox")
	}
}

func TestManager_AddQuestTaskWithoutInit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(tmpDir)
	questID := "quest-no-init"

	// Add task without initializing - should auto-init
	err = mgr.AddQuestTask(questID, "task-456", "Auto init test")
	if err != nil {
		t.Fatalf("AddQuestTask() error = %v", err)
	}

	// Verify conversation was created
	if !mgr.QuestConversationExists(questID) {
		t.Error("AddQuestTask() should auto-init conversation if not exists")
	}

	conv, _ := mgr.ReadQuestConversation(questID)
	if !strings.Contains(conv, "Auto init test") {
		t.Error("AddQuestTask() should add task even after auto-init")
	}
}
