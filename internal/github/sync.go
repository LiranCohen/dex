package github

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v68/github"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/workspace"
)

// SyncService coordinates GitHub Issue sync for Quests and Objectives
type SyncService struct {
	db         *db.DB
	appManager *AppManager
}

// NewSyncService creates a new GitHub sync service
func NewSyncService(database *db.DB, appManager *AppManager) *SyncService {
	return &SyncService{
		db:         database,
		appManager: appManager,
	}
}

// RepoInfo contains the owner and repo name for GitHub operations
type RepoInfo struct {
	Owner string
	Repo  string
}

// SyncQuestToIssue creates or updates a GitHub Issue for a Quest
func (s *SyncService) SyncQuestToIssue(ctx context.Context, questID string, repo RepoInfo, installationID int64) error {
	// Get the quest
	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return fmt.Errorf("quest not found: %s", questID)
	}

	// Get GitHub client for installation
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Get project for context
	project, err := s.db.GetProjectByID(quest.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	// Check if quest already has an issue
	if quest.GitHubIssueNumber.Valid {
		// Update existing issue
		return s.updateQuestIssue(ctx, client, quest, repo)
	}

	// Create new issue
	title := quest.GetTitle()
	if title == "" {
		title = fmt.Sprintf("Quest %s", questID)
	}

	// Get first message as description
	messages, err := s.db.GetQuestMessages(questID)
	description := ""
	if err == nil && len(messages) > 0 {
		for _, msg := range messages {
			if msg.Role == "user" {
				description = msg.Content
				break
			}
		}
	}

	body := FormatQuestIssueBody(description, projectName)

	issue, err := CreateQuestIssue(ctx, client, repo.Owner, repo.Repo, IssueOptions{
		Title: title,
		Body:  body,
	})
	if err != nil {
		return err
	}

	// Update quest with issue number
	if err := s.db.UpdateQuestGitHubIssue(questID, int64(issue.GetNumber())); err != nil {
		return fmt.Errorf("failed to update quest with issue number: %w", err)
	}

	return nil
}

// updateQuestIssue updates an existing GitHub Issue for a Quest
func (s *SyncService) updateQuestIssue(ctx context.Context, client *github.Client, quest *db.Quest, repo RepoInfo) error {
	// Get tasks for this quest to update the issue body
	tasks, err := s.db.GetTasksByQuestID(quest.ID)
	if err != nil {
		return fmt.Errorf("failed to get quest tasks: %w", err)
	}

	// Build objectives section
	objectivesSection := "\n### Objectives\n\n"
	if len(tasks) == 0 {
		objectivesSection += "_Objectives will be linked here as they are created._\n"
	} else {
		for _, task := range tasks {
			checkbox := "[ ]"
			if task.Status == db.TaskStatusCompleted || task.Status == db.TaskStatusCompletedWithIssues {
				checkbox = "[x]"
			}
			if task.GitHubIssueNumber.Valid {
				objectivesSection += fmt.Sprintf("- %s #%d - %s\n", checkbox, task.GitHubIssueNumber.Int64, task.Title)
			} else {
				objectivesSection += fmt.Sprintf("- %s %s\n", checkbox, task.Title)
			}
		}
	}

	// Get project for context
	project, _ := s.db.GetProjectByID(quest.ProjectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	// Get first message as description
	messages, _ := s.db.GetQuestMessages(quest.ID)
	description := ""
	if len(messages) > 0 {
		for _, msg := range messages {
			if msg.Role == "user" {
				description = msg.Content
				break
			}
		}
	}

	body := fmt.Sprintf(`## Quest

%s

---

**Project:** %s
%s`, description, projectName, objectivesSection)

	return UpdateIssueBody(ctx, client, repo.Owner, repo.Repo, int(quest.GitHubIssueNumber.Int64), body)
}

// SyncObjectiveToIssue creates a GitHub Issue for a Task/Objective
func (s *SyncService) SyncObjectiveToIssue(ctx context.Context, taskID string, repo RepoInfo, installationID int64) error {
	// Get the task
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Skip if already has an issue
	if task.GitHubIssueNumber.Valid {
		return nil
	}

	// Get GitHub client for installation
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Get quest issue number if task has a quest
	questIssueNumber := 0
	if task.QuestID.Valid {
		quest, err := s.db.GetQuestByID(task.QuestID.String)
		if err == nil && quest != nil && quest.GitHubIssueNumber.Valid {
			questIssueNumber = int(quest.GitHubIssueNumber.Int64)
		}
	}

	// Get checklist items
	checklist, err := s.db.GetChecklistByTaskID(taskID)
	var checklistItems []string
	if err == nil && checklist != nil {
		items, err := s.db.GetChecklistItems(checklist.ID)
		if err == nil {
			for _, item := range items {
				checklistItems = append(checklistItems, item.Description)
			}
		}
	}

	description := ""
	if task.Description.Valid {
		description = task.Description.String
	}

	body := FormatObjectiveIssueBody(description, checklistItems)

	issue, err := CreateObjectiveIssue(ctx, client, repo.Owner, repo.Repo, IssueOptions{
		Title: task.Title,
		Body:  body,
	}, questIssueNumber)
	if err != nil {
		return err
	}

	// Update task with issue number
	if err := s.db.UpdateTaskGitHubIssue(taskID, int64(issue.GetNumber())); err != nil {
		return fmt.Errorf("failed to update task with issue number: %w", err)
	}

	// Link objective to quest issue if applicable
	if questIssueNumber > 0 {
		if err := LinkObjectiveToQuest(ctx, client, repo.Owner, repo.Repo, questIssueNumber, issue.GetNumber(), task.Title); err != nil {
			// Log but don't fail - the issue was created successfully
			fmt.Printf("warning: failed to link objective to quest: %v\n", err)
		}
	}

	return nil
}

// CompleteQuestIssue closes the quest's GitHub Issue with a summary
func (s *SyncService) CompleteQuestIssue(ctx context.Context, questID string, summary string, repo RepoInfo, installationID int64) error {
	// Get the quest
	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return fmt.Errorf("quest not found: %s", questID)
	}

	// Skip if no issue
	if !quest.GitHubIssueNumber.Valid {
		return nil
	}

	// Get GitHub client for installation
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Update issue one last time to show final state
	if err := s.updateQuestIssue(ctx, client, quest, repo); err != nil {
		// Log but continue to close
		fmt.Printf("warning: failed to update quest issue before closing: %v\n", err)
	}

	// Close with summary
	comment := fmt.Sprintf("## Quest Completed\n\n%s", summary)
	return CloseIssueWithComment(ctx, client, repo.Owner, repo.Repo, int(quest.GitHubIssueNumber.Int64), comment)
}

// ReopenQuestIssue reopens a quest's GitHub Issue
func (s *SyncService) ReopenQuestIssue(ctx context.Context, questID string, repo RepoInfo, installationID int64) error {
	// Get the quest
	quest, err := s.db.GetQuestByID(questID)
	if err != nil {
		return fmt.Errorf("failed to get quest: %w", err)
	}
	if quest == nil {
		return fmt.Errorf("quest not found: %s", questID)
	}

	// Skip if no issue
	if !quest.GitHubIssueNumber.Valid {
		return nil
	}

	// Get GitHub client for installation
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	return ReopenIssueWithComment(ctx, client, repo.Owner, repo.Repo, int(quest.GitHubIssueNumber.Int64), "Quest reopened.")
}

// CompleteObjectiveIssue closes the objective's GitHub Issue
func (s *SyncService) CompleteObjectiveIssue(ctx context.Context, taskID string, repo RepoInfo, installationID int64) error {
	// Get the task
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Skip if no issue
	if !task.GitHubIssueNumber.Valid {
		return nil
	}

	// Get GitHub client for installation
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	comment := "Objective completed."
	if task.PRNumber.Valid {
		comment = fmt.Sprintf("Objective completed. See PR #%d.", task.PRNumber.Int64)
	}

	return CloseIssueWithComment(ctx, client, repo.Owner, repo.Repo, int(task.GitHubIssueNumber.Int64), comment)
}

// CancelObjectiveIssue closes the objective's GitHub Issue as cancelled
func (s *SyncService) CancelObjectiveIssue(ctx context.Context, taskID string, repo RepoInfo, installationID int64) error {
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if !task.GitHubIssueNumber.Valid {
		return nil
	}

	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	return CloseIssueWithComment(ctx, client, repo.Owner, repo.Repo, int(task.GitHubIssueNumber.Int64), "Objective cancelled.")
}

// FailObjectiveIssue closes the objective's GitHub Issue as failed
func (s *SyncService) FailObjectiveIssue(ctx context.Context, taskID string, reason string, repo RepoInfo, installationID int64) error {
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if !task.GitHubIssueNumber.Valid {
		return nil
	}

	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	comment := "Objective failed."
	if reason != "" {
		comment = fmt.Sprintf("Objective failed: %s", reason)
	}

	return CloseIssueWithComment(ctx, client, repo.Owner, repo.Repo, int(task.GitHubIssueNumber.Int64), comment)
}

// UpdateObjectiveIssueChecklist updates the objective's GitHub Issue body with current checklist status
func (s *SyncService) UpdateObjectiveIssueChecklist(ctx context.Context, taskID string, repo RepoInfo, installationID int64) error {
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if !task.GitHubIssueNumber.Valid {
		return nil
	}

	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Get checklist items for this task
	checklist, err := s.db.GetChecklistByTaskID(taskID)
	if err != nil || checklist == nil {
		// No checklist, nothing to update
		return nil
	}

	items, err := s.db.GetChecklistItems(checklist.ID)
	if err != nil {
		return fmt.Errorf("failed to get checklist items: %w", err)
	}

	// Convert to ChecklistItemWithStatus
	var checklistItems []ChecklistItemWithStatus
	for _, item := range items {
		checklistItems = append(checklistItems, ChecklistItemWithStatus{
			Description: item.Description,
			Status:      item.Status,
		})
	}

	description := ""
	if task.Description.Valid {
		description = task.Description.String
	}

	body := FormatObjectiveIssueBodyWithStatus(description, checklistItems)

	return UpdateIssueBody(ctx, client, repo.Owner, repo.Repo, int(task.GitHubIssueNumber.Int64), body)
}

// AddObjectiveStatusComment adds a status comment to the objective's GitHub Issue
func (s *SyncService) AddObjectiveStatusComment(ctx context.Context, taskID string, status string, repo RepoInfo, installationID int64) error {
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if !task.GitHubIssueNumber.Valid {
		return nil
	}

	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	var comment string
	switch status {
	case "running":
		comment = "🚀 Work started on this objective."
	case "paused":
		comment = "⏸️ Work paused on this objective."
	case "resumed":
		comment = "▶️ Work resumed on this objective."
	default:
		comment = fmt.Sprintf("Status changed to: %s", status)
	}

	return AddIssueComment(ctx, client, repo.Owner, repo.Repo, int(task.GitHubIssueNumber.Int64), comment)
}

// LinkPRToObjective links a PR to the objective's GitHub Issue
func (s *SyncService) LinkPRToObjective(ctx context.Context, taskID string, prNumber int, repo RepoInfo, installationID int64) error {
	// Get the task
	task, err := s.db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Skip if no issue
	if !task.GitHubIssueNumber.Valid {
		return nil
	}

	// Get GitHub client for installation
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	return LinkPRToIssue(ctx, client, repo.Owner, repo.Repo, int(task.GitHubIssueNumber.Int64), prNumber)
}

// EnsureRepoLabels ensures the dex labels exist in the repo
func (s *SyncService) EnsureRepoLabels(ctx context.Context, repo RepoInfo, installationID int64) error {
	client, err := s.appManager.GetClientForInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	return EnsureLabelsExist(ctx, client, repo.Owner, repo.Repo)
}

// GetRepoInfoFromProject extracts owner/repo from a project's GitHub fields or remote URL
func GetRepoInfoFromProject(project *db.Project) (RepoInfo, bool) {
	// First try explicit GitHub fields
	if project.GitHubOwner.Valid && project.GitHubRepo.Valid {
		return RepoInfo{
			Owner: project.GitHubOwner.String,
			Repo:  project.GitHubRepo.String,
		}, true
	}

	// Try to parse from remote origin URL
	if project.RemoteOrigin.Valid {
		if owner, repo, ok := ParseGitHubURL(project.RemoteOrigin.String); ok {
			return RepoInfo{Owner: owner, Repo: repo}, true
		}
	}

	return RepoInfo{}, false
}

// ParseGitHubURL extracts owner and repo from a GitHub URL
// Supports: git@github.com:owner/repo.git, https://github.com/owner/repo.git, https://github.com/owner/repo
func ParseGitHubURL(url string) (owner, repo string, ok bool) {
	// SSH format: git@github.com:owner/repo.git
	sshRegex := regexp.MustCompile(`git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := sshRegex.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1], matches[2], true
	}

	// HTTPS format: https://github.com/owner/repo.git or https://github.com/owner/repo
	httpsRegex := regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := httpsRegex.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1], matches[2], true
	}

	return "", "", false
}

// GetWorkspaceRepoInfo returns the repo info for the Dex workspace repo
func GetWorkspaceRepoInfo(orgName string) RepoInfo {
	return RepoInfo{
		Owner: orgName,
		Repo:  workspace.WorkspaceRepoName(),
	}
}

// GetRepoInfoForQuest determines the appropriate repo for a quest's issues
// Uses the project's repo if available, otherwise falls back to workspace repo
func (s *SyncService) GetRepoInfoForQuest(quest *db.Quest, workspaceRepo RepoInfo) (RepoInfo, error) {
	// Get the project
	project, err := s.db.GetProjectByID(quest.ProjectID)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("failed to get project: %w", err)
	}

	// If project has GitHub info, use it
	if project != nil {
		if repo, ok := GetRepoInfoFromProject(project); ok {
			return repo, nil
		}
	}

	// Fall back to workspace repo
	if workspaceRepo.Owner == "" || workspaceRepo.Repo == "" {
		return RepoInfo{}, fmt.Errorf("no GitHub repo configured for project and no workspace repo available")
	}

	return workspaceRepo, nil
}

// SyncConfig holds configuration for GitHub sync operations
type SyncConfig struct {
	OrgName        string // GitHub organization name
	InstallationID int64  // GitHub App installation ID
}

// GetWorkspaceRepo returns the workspace repo info from config
func (c *SyncConfig) GetWorkspaceRepo() RepoInfo {
	return GetWorkspaceRepoInfo(c.OrgName)
}

// IsConfigured returns true if sync is properly configured
func (c *SyncConfig) IsConfigured() bool {
	return c.OrgName != "" && c.InstallationID > 0
}

// BuildSyncConfig builds sync configuration from onboarding progress and app settings
func BuildSyncConfig(progress *db.OnboardingProgress, installationID int64) *SyncConfig {
	if progress == nil {
		return &SyncConfig{}
	}

	return &SyncConfig{
		OrgName:        progress.GetGitHubOrgName(),
		InstallationID: installationID,
	}
}

// Helper to check if string contains github.com
func isGitHubURL(url string) bool {
	return strings.Contains(url, "github.com")
}
