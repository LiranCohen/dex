package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/realtime"
)

// SyncService handles GitHub synchronization operations.
// It wraps the github.SyncService with additional context about server state.
type SyncService struct {
	deps *core.Deps
}

// NewSyncService creates a new sync service.
func NewSyncService(deps *core.Deps) *SyncService {
	return &SyncService{deps: deps}
}

// getSyncConfig returns the GitHub sync configuration, or nil if not configured.
func (s *SyncService) getSyncConfig(ctx context.Context) *github.SyncConfig {
	appManager := s.deps.GetGitHubApp()
	if appManager == nil {
		return nil
	}

	progress, err := s.deps.DB.GetOnboardingProgress()
	if err != nil || progress == nil {
		return nil
	}

	orgName := progress.GetGitHubOrgName()
	if orgName == "" {
		return nil
	}

	installID, err := appManager.GetInstallationIDForLogin(ctx, orgName)
	if err != nil {
		fmt.Printf("getSyncConfig: failed to get installation ID: %v\n", err)
		return nil
	}

	return &github.SyncConfig{
		OrgName:        orgName,
		InstallationID: installID,
	}
}

// SyncQuestToGitHubIssue creates or updates a GitHub Issue for a quest.
func (s *SyncService) SyncQuestToGitHubIssue(questID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("SyncQuestToGitHubIssue: failed to get quest %s: %v\n", questID, err)
		return
	}

	workspaceRepo := syncConfig.GetWorkspaceRepo()
	repo, err := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
	if err != nil {
		fmt.Printf("SyncQuestToGitHubIssue: failed to get repo for quest %s: %v\n", questID, err)
		return
	}

	if err := syncService.EnsureRepoLabels(ctx, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("SyncQuestToGitHubIssue: failed to ensure labels for %s/%s: %v\n", repo.Owner, repo.Repo, err)
	}

	if err := syncService.SyncQuestToIssue(ctx, questID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("SyncQuestToGitHubIssue: failed to sync quest %s: %v\n", questID, err)
		return
	}

	fmt.Printf("SyncQuestToGitHubIssue: synced quest %s to %s/%s\n", questID, repo.Owner, repo.Repo)
}

// CloseQuestGitHubIssue closes the GitHub Issue for a completed quest.
func (s *SyncService) CloseQuestGitHubIssue(questID string, summary *db.QuestSummary) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("CloseQuestGitHubIssue: failed to get quest %s: %v\n", questID, err)
		return
	}

	workspaceRepo := syncConfig.GetWorkspaceRepo()
	repo, err := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
	if err != nil {
		fmt.Printf("CloseQuestGitHubIssue: failed to get repo for quest %s: %v\n", questID, err)
		return
	}

	summaryText := "Quest completed."
	if summary != nil {
		summaryText = fmt.Sprintf("Quest completed with %d/%d tasks completed.",
			summary.CompletedTasks, summary.TotalTasks)
		if summary.TotalDollarsUsed > 0 {
			summaryText += fmt.Sprintf(" Total cost: $%.4f", summary.TotalDollarsUsed)
		}
	}

	if err := syncService.CompleteQuestIssue(ctx, questID, summaryText, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("CloseQuestGitHubIssue: failed to close issue for quest %s: %v\n", questID, err)
		return
	}

	fmt.Printf("CloseQuestGitHubIssue: closed issue for quest %s\n", questID)
}

// ReopenQuestGitHubIssue reopens the GitHub Issue for a reopened quest.
func (s *SyncService) ReopenQuestGitHubIssue(questID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("ReopenQuestGitHubIssue: failed to get quest %s: %v\n", questID, err)
		return
	}

	workspaceRepo := syncConfig.GetWorkspaceRepo()
	repo, err := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
	if err != nil {
		fmt.Printf("ReopenQuestGitHubIssue: failed to get repo for quest %s: %v\n", questID, err)
		return
	}

	if err := syncService.ReopenQuestIssue(ctx, questID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("ReopenQuestGitHubIssue: failed to reopen issue for quest %s: %v\n", questID, err)
		return
	}

	fmt.Printf("ReopenQuestGitHubIssue: reopened issue for quest %s\n", questID)
}

// SyncObjectiveToGitHubIssue creates a GitHub Issue for an objective (task).
func (s *SyncService) SyncObjectiveToGitHubIssue(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("SyncObjectiveToGitHubIssue: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("SyncObjectiveToGitHubIssue: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.EnsureRepoLabels(ctx, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("SyncObjectiveToGitHubIssue: failed to ensure labels: %v\n", err)
	}

	if err := syncService.SyncObjectiveToIssue(ctx, taskID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("SyncObjectiveToGitHubIssue: failed to sync task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("SyncObjectiveToGitHubIssue: synced task %s to %s/%s\n", taskID, repo.Owner, repo.Repo)

	// Also update the quest issue
	if task.QuestID.Valid {
		quest, err := s.deps.DB.GetQuestByID(task.QuestID.String)
		if err == nil && quest != nil && quest.GitHubIssueNumber.Valid {
			workspaceRepo := syncConfig.GetWorkspaceRepo()
			questRepo, err := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
			if err == nil {
				if err := syncService.SyncQuestToIssue(ctx, quest.ID, questRepo, syncConfig.InstallationID); err != nil {
					fmt.Printf("SyncObjectiveToGitHubIssue: failed to update quest issue: %v\n", err)
				}
			}
		}
	}
}

// OnTaskCompleted handles task completion for GitHub sync and dependency unblocking.
func (s *SyncService) OnTaskCompleted(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Phase 1: Handle dependency unblocking
	s.handleTaskUnblocking(ctx, taskID)

	// Phase 2: GitHub sync
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("OnTaskCompleted: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("OnTaskCompleted: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.CompleteObjectiveIssue(ctx, taskID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("OnTaskCompleted: failed to close issue for task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("OnTaskCompleted: closed issue for task %s\n", taskID)

	// Update parent quest issue
	s.updateQuestIssueForTask(ctx, task, syncService, syncConfig)
}

// handleTaskUnblocking finds tasks that are ready to auto-start because the given task completed.
// Uses fully derived blocked state - tasks stay 'ready' and we query for those with auto_start=true
// and no more incomplete blockers.
func (s *SyncService) handleTaskUnblocking(ctx context.Context, completedTaskID string) {
	completedTask, err := s.deps.DB.GetTaskByID(completedTaskID)
	if err != nil || completedTask == nil {
		fmt.Printf("handleTaskUnblocking: failed to get completed task %s: %v\n", completedTaskID, err)
		return
	}

	tasksToAutoStart, err := s.deps.DB.GetTasksReadyToAutoStart(completedTaskID)
	if err != nil {
		fmt.Printf("handleTaskUnblocking: failed to get tasks ready to auto-start for %s: %v\n", completedTaskID, err)
		return
	}

	if len(tasksToAutoStart) == 0 {
		return
	}

	fmt.Printf("handleTaskUnblocking: %d tasks ready to auto-start after completion of %s\n", len(tasksToAutoStart), completedTaskID)

	var predecessorHandoff string
	if s.deps.GeneratePredecessorHandoff != nil && completedTask.WorktreePath.Valid && completedTask.WorktreePath.String != "" {
		predecessorHandoff = s.deps.GeneratePredecessorHandoff(completedTask)
	}

	for _, task := range tasksToAutoStart {
		// Broadcast task unblocked event for UI update
		if s.deps.Broadcaster != nil {
			s.deps.Broadcaster.PublishTaskEvent(realtime.EventTaskUnblocked, task.ID, map[string]any{
				"unblocked_by": completedTaskID,
				"quest_id":     task.QuestID.String,
				"title":        task.Title,
				"project_id":   task.ProjectID,
			})
		}

		// Auto-start the task
		if s.deps.StartTaskWithInheritance != nil {
			taskID := task.ID
			projectID := task.ProjectID
			inheritedWorktree := completedTask.GetWorktreePath()
			handoff := predecessorHandoff
			broadcaster := s.deps.Broadcaster
			go func() {
				startResult, err := s.deps.StartTaskWithInheritance(context.Background(), taskID, inheritedWorktree, handoff)
				if err != nil {
					fmt.Printf("handleTaskUnblocking: auto-start failed for task %s: %v\n", taskID, err)
					if broadcaster != nil {
						broadcaster.PublishTaskEvent(realtime.EventTaskAutoStartFailed, taskID, map[string]any{
							"error":      err.Error(),
							"project_id": projectID,
						})
					}
					return
				}

				fmt.Printf("handleTaskUnblocking: auto-started task %s (session %s) with inherited worktree from %s\n",
					taskID, startResult.SessionID, completedTaskID)

				if broadcaster != nil {
					broadcaster.PublishTaskEvent(realtime.EventTaskAutoStarted, taskID, map[string]any{
						"session_id":        startResult.SessionID,
						"worktree_path":     startResult.WorktreePath,
						"inherited_from":    completedTaskID,
						"predecessor_title": completedTask.Title,
						"project_id":        projectID,
					})
				}
			}()
		}
	}
}

// updateQuestIssueForTask updates the parent quest's GitHub issue.
func (s *SyncService) updateQuestIssueForTask(ctx context.Context, task *db.Task, syncService *github.SyncService, syncConfig *github.SyncConfig) {
	if !task.QuestID.Valid {
		return
	}

	quest, err := s.deps.DB.GetQuestByID(task.QuestID.String)
	if err != nil || quest == nil || !quest.GitHubIssueNumber.Valid {
		return
	}

	workspaceRepo := syncConfig.GetWorkspaceRepo()
	repo, err := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
	if err != nil {
		fmt.Printf("updateQuestIssueForTask: failed to get repo for quest %s: %v\n", quest.ID, err)
		return
	}

	if err := syncService.SyncQuestToIssue(ctx, quest.ID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("updateQuestIssueForTask: failed to update quest issue %s: %v\n", quest.ID, err)
		return
	}

	fmt.Printf("updateQuestIssueForTask: updated quest issue for %s\n", quest.ID)
}

// OnTaskFailed handles task failure for GitHub sync.
func (s *SyncService) OnTaskFailed(taskID string, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("OnTaskFailed: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("OnTaskFailed: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.FailObjectiveIssue(ctx, taskID, reason, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("OnTaskFailed: failed to fail issue for task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("OnTaskFailed: failed issue for task %s\n", taskID)

	s.updateQuestIssueForTask(ctx, task, syncService, syncConfig)
}

// OnPRCreated handles PR creation for GitHub sync.
func (s *SyncService) OnPRCreated(taskID string, prNumber int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("OnPRCreated: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("OnPRCreated: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.LinkPRToObjective(ctx, taskID, prNumber, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("OnPRCreated: failed to link PR to task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("OnPRCreated: linked PR #%d to task %s\n", prNumber, taskID)
}

// CancelObjectiveGitHubIssue closes the GitHub Issue for a cancelled task.
func (s *SyncService) CancelObjectiveGitHubIssue(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("CancelObjectiveGitHubIssue: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("CancelObjectiveGitHubIssue: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.CancelObjectiveIssue(ctx, taskID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("CancelObjectiveGitHubIssue: failed to cancel issue for task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("CancelObjectiveGitHubIssue: cancelled issue for task %s\n", taskID)

	s.updateQuestIssueForTask(ctx, task, syncService, syncConfig)
}

// UpdateObjectiveStatusSync adds a status comment to the objective's GitHub Issue.
func (s *SyncService) UpdateObjectiveStatusSync(taskID string, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("UpdateObjectiveStatusSync: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("UpdateObjectiveStatusSync: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.AddObjectiveStatusComment(ctx, taskID, status, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("UpdateObjectiveStatusSync: failed to add status comment for task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("UpdateObjectiveStatusSync: added %s status comment for task %s\n", status, taskID)
}

// UpdateObjectiveChecklistSync updates the objective's GitHub Issue with checklist progress.
func (s *SyncService) UpdateObjectiveChecklistSync(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("UpdateObjectiveChecklistSync: failed to get task %s: %v\n", taskID, err)
		return
	}

	project, err := s.deps.DB.GetProjectByID(task.ProjectID)
	if err != nil || project == nil {
		fmt.Printf("UpdateObjectiveChecklistSync: failed to get project for task %s: %v\n", taskID, err)
		return
	}

	repo, ok := github.GetRepoInfoFromProject(project)
	if !ok {
		repo = syncConfig.GetWorkspaceRepo()
	}

	if err := syncService.UpdateObjectiveIssueChecklist(ctx, taskID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("UpdateObjectiveChecklistSync: failed to update issue for task %s: %v\n", taskID, err)
		return
	}

	fmt.Printf("UpdateObjectiveChecklistSync: updated issue checklist for task %s\n", taskID)
}

// GeneratePredecessorHandoff creates a handoff summary for a completed task.
func GeneratePredecessorHandoff(dbInstance *db.DB, task *db.Task) string {
	var sb strings.Builder

	sb.WriteString("## Predecessor Task Completed\n\n")
	sb.WriteString(fmt.Sprintf("**Previous Task**: %s\n", task.Title))

	if task.Description.Valid && task.Description.String != "" {
		sb.WriteString(fmt.Sprintf("**Description**: %s\n", task.Description.String))
	}

	sb.WriteString("**Status**: Completed\n")

	if task.WorktreePath.Valid && task.WorktreePath.String != "" {
		sb.WriteString(fmt.Sprintf("**Working Directory**: %s\n", task.WorktreePath.String))
	}

	if task.BranchName.Valid && task.BranchName.String != "" {
		sb.WriteString(fmt.Sprintf("**Branch**: %s\n", task.BranchName.String))
	}

	if checklist, err := dbInstance.GetChecklistByTaskID(task.ID); err == nil && checklist != nil {
		if items, err := dbInstance.GetChecklistItems(checklist.ID); err == nil && len(items) > 0 {
			sb.WriteString("\n**Completed Work**:\n")
			for _, item := range items {
				if item.Status == db.ChecklistItemStatusDone {
					sb.WriteString(fmt.Sprintf("- [x] %s\n", item.Description))
				}
			}
		}
	}

	sb.WriteString("\n**Your Task**: Continue from where the previous task left off. Use the same working directory and build upon the completed work.\n")

	return sb.String()
}
