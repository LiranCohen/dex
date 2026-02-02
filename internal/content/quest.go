package content

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const conversationFileName = "conversation.md"

// ConversationMessage represents a single message in a quest conversation
type ConversationMessage struct {
	Role      string    // "user" or "assistant"
	Content   string    // Message content
	Timestamp time.Time // When the message was sent
}

// WriteQuestConversation writes the quest conversation to a git file
func (m *Manager) WriteQuestConversation(questID string, messages []ConversationMessage) error {
	content := FormatConversation(messages)
	path := filepath.Join(QuestContentPath(questID), conversationFileName)
	return m.writeFile(path, content)
}

// AppendQuestMessage appends a message to the quest conversation file
func (m *Manager) AppendQuestMessage(questID string, msg ConversationMessage) error {
	path := filepath.Join(QuestContentPath(questID), conversationFileName)

	// Read existing content
	existing, err := m.readFile(path)
	if err != nil {
		return fmt.Errorf("failed to read conversation: %w", err)
	}

	// If file doesn't exist, create with header
	if existing == "" {
		existing = "# Quest Conversation\n\n"
	}

	// Append new message
	formatted := formatMessage(msg)
	newContent := existing + formatted

	return m.writeFile(path, newContent)
}

// ReadQuestConversation reads and returns the raw conversation content
func (m *Manager) ReadQuestConversation(questID string) (string, error) {
	path := filepath.Join(QuestContentPath(questID), conversationFileName)
	return m.readFile(path)
}

// QuestConversationExists checks if a quest conversation file exists
func (m *Manager) QuestConversationExists(questID string) bool {
	path := filepath.Join(QuestContentPath(questID), conversationFileName)
	return m.fileExists(path)
}

// InitQuestContent creates the initial content files for a quest
func (m *Manager) InitQuestContent(questID, title string) error {
	content := fmt.Sprintf("# Quest Conversation\n\n**Started:** %s\n\n---\n\n",
		time.Now().Format("2006-01-02 15:04"))

	if title != "" {
		content = fmt.Sprintf("# %s\n\n**Started:** %s\n\n---\n\n",
			title, time.Now().Format("2006-01-02 15:04"))
	}

	path := filepath.Join(QuestContentPath(questID), conversationFileName)
	return m.writeFile(path, content)
}

// GetQuestContentPath returns the full path to quest content directory
func (m *Manager) GetQuestContentPath(questID string) string {
	return filepath.Join(m.basePath, QuestContentPath(questID))
}

// GetQuestConversationPath returns the full path to the quest conversation file
func (m *Manager) GetQuestConversationPath(questID string) string {
	return filepath.Join(m.basePath, QuestContentPath(questID), conversationFileName)
}

// AddQuestTask appends a task reference to the quest conversation
// This is called when a task is created from a quest objective
func (m *Manager) AddQuestTask(questID, taskID, title string) error {
	path := filepath.Join(QuestContentPath(questID), conversationFileName)

	// Read existing content
	existing, err := m.readFile(path)
	if err != nil {
		return fmt.Errorf("failed to read conversation: %w", err)
	}

	// If file doesn't exist, initialize it first
	if existing == "" {
		if err := m.InitQuestContent(questID, ""); err != nil {
			return err
		}
		existing, _ = m.readFile(path)
	}

	// Append task reference
	taskRef := fmt.Sprintf("\n> **Task Created:** [%s](#%s)\n\n---\n\n", title, taskID)
	return m.writeFile(path, existing+taskRef)
}

// CompleteQuestContent marks the quest as complete and adds a summary
func (m *Manager) CompleteQuestContent(questID, summary string) error {
	path := filepath.Join(QuestContentPath(questID), conversationFileName)

	// Read existing content
	existing, err := m.readFile(path)
	if err != nil {
		return fmt.Errorf("failed to read conversation: %w", err)
	}

	if existing == "" {
		return fmt.Errorf("conversation not found for quest %s", questID)
	}

	// Append completion section
	completion := fmt.Sprintf(`
---

## Quest Completed

**Completed:** %s

### Summary

%s
`, time.Now().Format("2006-01-02 15:04"), summary)

	return m.writeFile(path, existing+completion)
}

// WriteQuestTasksList writes a summary of all tasks created by a quest
func (m *Manager) WriteQuestTasksList(questID string, tasks []QuestTask) error {
	path := filepath.Join(QuestContentPath(questID), "tasks.md")

	var sb strings.Builder
	sb.WriteString("# Quest Tasks\n\n")

	for _, task := range tasks {
		checkbox := "[ ]"
		if task.Completed {
			checkbox = "[x]"
		}
		sb.WriteString(fmt.Sprintf("- %s [%s](#%s) - %s\n", checkbox, task.Title, task.ID, task.Status))
	}

	sb.WriteString(fmt.Sprintf("\n---\nLast updated: %s\n", time.Now().Format("2006-01-02 15:04")))

	return m.writeFile(path, sb.String())
}

// QuestTask represents a task reference in a quest
type QuestTask struct {
	ID        string
	Title     string
	Status    string
	Completed bool
}

// FormatConversation formats messages as markdown
func FormatConversation(messages []ConversationMessage) string {
	var sb strings.Builder
	sb.WriteString("# Quest Conversation\n\n")

	for _, msg := range messages {
		sb.WriteString(formatMessage(msg))
	}

	return sb.String()
}

// formatMessage formats a single message as markdown
func formatMessage(msg ConversationMessage) string {
	var sb strings.Builder

	// Role header
	role := "User"
	if msg.Role == "assistant" {
		role = "Dex"
	}

	sb.WriteString(fmt.Sprintf("## %s", role))
	if !msg.Timestamp.IsZero() {
		sb.WriteString(fmt.Sprintf(" _%s_", msg.Timestamp.Format("15:04")))
	}
	sb.WriteString("\n\n")

	// Content
	sb.WriteString(msg.Content)
	sb.WriteString("\n\n---\n\n")

	return sb.String()
}
