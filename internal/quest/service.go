// Package quest provides Quest conversation handling for Poindexter
package quest

import (
	"fmt"

	"github.com/lirancohen/dex/internal/content"
	"github.com/lirancohen/dex/internal/db"
)

// Service coordinates quest operations between database and git content
type Service struct {
	db *db.DB
}

// NewService creates a new quest service
func NewService(database *db.DB) *Service {
	return &Service{
		db: database,
	}
}

// GitOperations interface for git operations needed by quest service
type GitOperations interface {
	CommitQuestContent(dir, questID, message string) (string, error)
}

// InitQuestContent initializes git content files for a quest and updates the database
func (s *Service) InitQuestContent(questID, basePath, title string) error {
	// Create content manager for this path
	mgr := content.NewManager(basePath)

	// Initialize the quest content files
	if err := mgr.InitQuestContent(questID, title); err != nil {
		return fmt.Errorf("failed to init quest content: %w", err)
	}

	// Update the quest's conversation_path in the database
	conversationPath := content.QuestContentPath(questID)
	if err := s.db.UpdateQuestConversationPath(questID, conversationPath); err != nil {
		return fmt.Errorf("failed to update quest conversation path: %w", err)
	}

	return nil
}

// AppendMessage appends a message to the quest conversation in git files
func (s *Service) AppendMessage(questID, basePath, role, messageContent string) error {
	mgr := content.NewManager(basePath)

	msg := content.ConversationMessage{
		Role:    role,
		Content: messageContent,
	}

	return mgr.AppendQuestMessage(questID, msg)
}

// AppendMessageAndCommit appends a message and commits to git
func (s *Service) AppendMessageAndCommit(questID, basePath, role, messageContent string, git GitOperations) (string, error) {
	// Append the message
	if err := s.AppendMessage(questID, basePath, role, messageContent); err != nil {
		return "", err
	}

	// Commit the changes
	commitMsg := fmt.Sprintf("Update quest %s conversation", questID)
	return git.CommitQuestContent(basePath, questID, commitMsg)
}

// AddTaskToQuest adds a task reference to the quest conversation
func (s *Service) AddTaskToQuest(questID, basePath, taskID, taskTitle string) error {
	mgr := content.NewManager(basePath)
	return mgr.AddQuestTask(questID, taskID, taskTitle)
}

// CompleteQuest marks the quest as complete and adds a summary to the conversation
func (s *Service) CompleteQuest(questID, basePath, summary string) error {
	mgr := content.NewManager(basePath)

	// Add completion to content file
	if err := mgr.CompleteQuestContent(questID, summary); err != nil {
		return fmt.Errorf("failed to complete quest content: %w", err)
	}

	// Update database status
	if err := s.db.CompleteQuest(questID); err != nil {
		return fmt.Errorf("failed to complete quest in database: %w", err)
	}

	return nil
}

// CompleteQuestAndCommit completes the quest and commits the changes
func (s *Service) CompleteQuestAndCommit(questID, basePath, summary string, git GitOperations) (string, error) {
	// Complete the quest content
	if err := s.CompleteQuest(questID, basePath, summary); err != nil {
		return "", err
	}

	// Commit the changes
	commitMsg := fmt.Sprintf("Complete quest %s", questID)
	return git.CommitQuestContent(basePath, questID, commitMsg)
}

// UpdateQuestTasksList updates the tasks list file for a quest
func (s *Service) UpdateQuestTasksList(questID, basePath string) error {
	// Get all tasks for this quest
	tasks, err := s.db.GetTasksByQuestID(questID)
	if err != nil {
		return fmt.Errorf("failed to get quest tasks: %w", err)
	}

	// Convert to content format
	questTasks := make([]content.QuestTask, len(tasks))
	for i, task := range tasks {
		completed := task.Status == db.TaskStatusCompleted || task.Status == db.TaskStatusCompletedWithIssues
		questTasks[i] = content.QuestTask{
			ID:        task.ID,
			Title:     task.Title,
			Status:    task.Status,
			Completed: completed,
		}
	}

	// Write the tasks list
	mgr := content.NewManager(basePath)
	return mgr.WriteQuestTasksList(questID, questTasks)
}

// ReadConversation reads the quest conversation from git files
func (s *Service) ReadConversation(questID, basePath string) (string, error) {
	mgr := content.NewManager(basePath)
	return mgr.ReadQuestConversation(questID)
}

// GetQuestContentPath returns the full path to quest content for a given base path
func (s *Service) GetQuestContentPath(questID, basePath string) string {
	mgr := content.NewManager(basePath)
	return mgr.GetQuestContentPath(questID)
}

// SyncConversationToGit syncs all messages from database to git file
// This is useful for rebuilding the git file from database state
func (s *Service) SyncConversationToGit(questID, basePath string) error {
	// Get the quest
	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return fmt.Errorf("quest not found: %s", questID)
	}

	// Get all messages
	messages, err := s.db.GetQuestMessages(questID)
	if err != nil {
		return fmt.Errorf("failed to get quest messages: %w", err)
	}

	// Convert to content format
	contentMessages := make([]content.ConversationMessage, len(messages))
	for i, msg := range messages {
		contentMessages[i] = content.ConversationMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.CreatedAt,
		}
	}

	// Write full conversation
	mgr := content.NewManager(basePath)
	return mgr.WriteQuestConversation(questID, contentMessages)
}
