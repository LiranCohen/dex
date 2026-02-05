// Package issuesync provides issue-sync services that route to GitHub or Forgejo.
package issuesync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/gitprovider"
	forgejoclient "github.com/lirancohen/dex/internal/gitprovider/forgejo"
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

// getForgejoClient returns a gitprovider.Provider for the Forgejo instance, or nil if unavailable.
func (s *SyncService) getForgejoClient() gitprovider.Provider {
	mgr := s.deps.ForgejoManager
	if mgr == nil || !mgr.IsRunning() {
		return nil
	}
	token, err := mgr.BotToken()
	if err != nil {
		return nil
	}
	return forgejoclient.New(mgr.BaseURL(), token)
}

// syncForgejoQuestIssue creates or updates a Forgejo issue for a quest.
func (s *SyncService) syncForgejoQuestIssue(ctx context.Context, questID, owner, repo string, provider gitprovider.Provider) {
	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("syncForgejoQuestIssue: failed to get quest %s: %v\n", questID, err)
		return
	}

	title := quest.GetTitle()
	if title == "" {
		title = fmt.Sprintf("Quest %s", questID[:8])
	}

	if quest.IssueNumber.Valid {
		// Update existing issue
		body := fmt.Sprintf("Quest: %s\nStatus: %s", title, quest.Status)
		if err := provider.UpdateIssue(ctx, owner, repo, int(quest.IssueNumber.Int64), gitprovider.UpdateIssueOpts{Body: &body}); err != nil {
			fmt.Printf("syncForgejoQuestIssue: failed to update issue for quest %s: %v\n", questID, err)
		}
		return
	}

	// Create new issue
	issue, err := provider.CreateIssue(ctx, owner, repo, gitprovider.CreateIssueOpts{
		Title:  title,
		Body:   fmt.Sprintf("Quest: %s\nStatus: %s", title, quest.Status),
		Labels: []string{"dex:quest"},
	})
	if err != nil {
		fmt.Printf("syncForgejoQuestIssue: failed to create issue for quest %s: %v\n", questID, err)
		return
	}

	if err := s.deps.DB.UpdateQuestIssueNumber(questID, int64(issue.Number)); err != nil {
		fmt.Printf("syncForgejoQuestIssue: failed to store issue number for quest %s: %v\n", questID, err)
	}
	fmt.Printf("syncForgejoQuestIssue: created issue #%d for quest %s on %s/%s\n", issue.Number, questID, owner, repo)
}

// syncForgejoObjectiveIssue creates a Forgejo issue for a task/objective.
func (s *SyncService) syncForgejoObjectiveIssue(ctx context.Context, taskID, owner, repo string, provider gitprovider.Provider) {
	task, err := s.deps.DB.GetTaskByID(taskID)
	if err != nil || task == nil {
		fmt.Printf("syncForgejoObjectiveIssue: failed to get task %s: %v\n", taskID, err)
		return
	}

	if task.IssueNumber.Valid {
		return // Already synced
	}

	body := task.GetDescription()
	if body == "" {
		body = fmt.Sprintf("Objective for task %s", taskID)
	}

	// Link to parent quest issue if available
	if task.QuestID.Valid {
		quest, qerr := s.deps.DB.GetQuestByID(task.QuestID.String)
		if qerr == nil && quest != nil && quest.IssueNumber.Valid {
			body += fmt.Sprintf("\n\nParent quest: #%d", quest.IssueNumber.Int64)
		}
	}

	issue, err := provider.CreateIssue(ctx, owner, repo, gitprovider.CreateIssueOpts{
		Title:  task.Title,
		Body:   body,
		Labels: []string{"dex:objective"},
	})
	if err != nil {
		fmt.Printf("syncForgejoObjectiveIssue: failed to create issue for task %s: %v\n", taskID, err)
		return
	}

	if err := s.deps.DB.UpdateTaskIssueNumber(taskID, int64(issue.Number)); err != nil {
		fmt.Printf("syncForgejoObjectiveIssue: failed to store issue number for task %s: %v\n", taskID, err)
	}
	fmt.Printf("syncForgejoObjectiveIssue: created issue #%d for task %s on %s/%s\n", issue.Number, taskID, owner, repo)
}

// closeForgejoIssue closes a Forgejo issue by number with an optional comment.
func (s *SyncService) closeForgejoIssue(ctx context.Context, owner, repo string, issueNumber int, comment string, provider gitprovider.Provider) {
	if comment != "" {
		if _, err := provider.AddComment(ctx, owner, repo, issueNumber, comment); err != nil {
			fmt.Printf("closeForgejoIssue: failed to add comment to #%d: %v\n", issueNumber, err)
		}
	}
	if err := provider.CloseIssue(ctx, owner, repo, issueNumber); err != nil {
		fmt.Printf("closeForgejoIssue: failed to close issue #%d: %v\n", issueNumber, err)
	}
}

// getForgejoProjectInfo returns owner, repo, and a Forgejo provider for a Forgejo project.
// Returns "", "", nil if the project is not Forgejo-backed or Forgejo is unavailable.
func (s *SyncService) getForgejoProjectInfo(project *db.Project) (string, string, gitprovider.Provider) {
	if project == nil || !project.IsForgejo() {
		return "", "", nil
	}
	provider := s.getForgejoClient()
	if provider == nil {
		return "", "", nil
	}
	owner := project.GetOwner()
	repo := project.GetRepo()
	if owner == "" || repo == "" {
		return "", "", nil
	}
	return owner, repo, provider
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

// SyncQuestToGitHubIssue creates or updates an issue for a quest.
// Routes to Forgejo or GitHub based on the quest's project provider.
func (s *SyncService) SyncQuestToGitHubIssue(questID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if this quest's project uses Forgejo
	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("SyncQuestToGitHubIssue: failed to get quest %s: %v\n", questID, err)
		return
	}
	project, err := s.deps.DB.GetProjectByID(quest.ProjectID)
	if err == nil && project != nil {
		if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
			s.syncForgejoQuestIssue(ctx, questID, owner, repo, provider)
			return
		}
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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

// CloseQuestGitHubIssue closes the issue for a completed quest.
func (s *SyncService) CloseQuestGitHubIssue(questID string, summary *db.QuestSummary) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("CloseQuestGitHubIssue: failed to get quest %s: %v\n", questID, err)
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

	// Forgejo path
	project, err := s.deps.DB.GetProjectByID(quest.ProjectID)
	if err == nil && project != nil {
		if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
			if quest.IssueNumber.Valid {
				s.closeForgejoIssue(ctx, owner, repo, int(quest.IssueNumber.Int64), summaryText, provider)
				fmt.Printf("CloseQuestGitHubIssue: closed Forgejo issue #%d for quest %s\n", quest.IssueNumber.Int64, questID)
			}
			return
		}
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	workspaceRepo := syncConfig.GetWorkspaceRepo()
	repo, rerr := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
	if rerr != nil {
		fmt.Printf("CloseQuestGitHubIssue: failed to get repo for quest %s: %v\n", questID, rerr)
		return
	}

	if err := syncService.CompleteQuestIssue(ctx, questID, summaryText, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("CloseQuestGitHubIssue: failed to close issue for quest %s: %v\n", questID, err)
		return
	}

	fmt.Printf("CloseQuestGitHubIssue: closed issue for quest %s\n", questID)
}

// ReopenQuestGitHubIssue reopens the issue for a reopened quest.
func (s *SyncService) ReopenQuestGitHubIssue(questID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	quest, err := s.deps.DB.GetQuestByID(questID)
	if err != nil || quest == nil {
		fmt.Printf("ReopenQuestGitHubIssue: failed to get quest %s: %v\n", questID, err)
		return
	}

	// Forgejo path
	project, err := s.deps.DB.GetProjectByID(quest.ProjectID)
	if err == nil && project != nil {
		if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
			if quest.IssueNumber.Valid {
				openState := "open"
				if err := provider.UpdateIssue(ctx, owner, repo, int(quest.IssueNumber.Int64), gitprovider.UpdateIssueOpts{State: &openState}); err != nil {
					fmt.Printf("ReopenQuestGitHubIssue: failed to reopen Forgejo issue #%d for quest %s: %v\n", quest.IssueNumber.Int64, questID, err)
				} else {
					fmt.Printf("ReopenQuestGitHubIssue: reopened Forgejo issue #%d for quest %s\n", quest.IssueNumber.Int64, questID)
				}
			}
			return
		}
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
		return
	}

	workspaceRepo := syncConfig.GetWorkspaceRepo()
	repo, rerr := syncService.GetRepoInfoForQuest(quest, workspaceRepo)
	if rerr != nil {
		fmt.Printf("ReopenQuestGitHubIssue: failed to get repo for quest %s: %v\n", questID, rerr)
		return
	}

	if err := syncService.ReopenQuestIssue(ctx, questID, repo, syncConfig.InstallationID); err != nil {
		fmt.Printf("ReopenQuestGitHubIssue: failed to reopen issue for quest %s: %v\n", questID, err)
		return
	}

	fmt.Printf("ReopenQuestGitHubIssue: reopened issue for quest %s\n", questID)
}

// SyncObjectiveToGitHubIssue creates an issue for an objective (task).
// Routes to Forgejo or GitHub based on the task's project provider.
func (s *SyncService) SyncObjectiveToGitHubIssue(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Forgejo path
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		s.syncForgejoObjectiveIssue(ctx, taskID, owner, repo, provider)
		// Also update parent quest issue on Forgejo
		if task.QuestID.Valid {
			s.syncForgejoQuestIssue(ctx, task.QuestID.String, owner, repo, provider)
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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
		if err == nil && quest != nil && quest.IssueNumber.Valid {
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

// OnTaskCompleted handles task completion for issue sync and dependency unblocking.
func (s *SyncService) OnTaskCompleted(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Phase 1: Handle dependency unblocking (always runs regardless of provider)
	s.handleTaskUnblocking(ctx, taskID)

	// Phase 2: Issue sync
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

	// Forgejo path
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		if task.IssueNumber.Valid {
			s.closeForgejoIssue(ctx, owner, repo, int(task.IssueNumber.Int64), "Task completed.", provider)
			fmt.Printf("OnTaskCompleted: closed Forgejo issue #%d for task %s\n", task.IssueNumber.Int64, taskID)
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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
	if err != nil || quest == nil || !quest.IssueNumber.Valid {
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

// OnTaskFailed handles task failure for issue sync.
func (s *SyncService) OnTaskFailed(taskID string, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Forgejo path
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		if task.IssueNumber.Valid {
			comment := fmt.Sprintf("Task failed: %s", reason)
			s.closeForgejoIssue(ctx, owner, repo, int(task.IssueNumber.Int64), comment, provider)
			fmt.Printf("OnTaskFailed: closed Forgejo issue #%d for task %s\n", task.IssueNumber.Int64, taskID)
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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

// OnPRCreated handles PR creation for issue sync.
func (s *SyncService) OnPRCreated(taskID string, prNumber int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Forgejo path: link PR to objective issue via comment
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		if task.IssueNumber.Valid {
			comment := fmt.Sprintf("Pull request !%d created for this objective.", prNumber)
			if _, err := provider.AddComment(ctx, owner, repo, int(task.IssueNumber.Int64), comment); err != nil {
				fmt.Printf("OnPRCreated: failed to link Forgejo PR to task %s: %v\n", taskID, err)
			} else {
				fmt.Printf("OnPRCreated: linked Forgejo PR !%d to task %s\n", prNumber, taskID)
			}
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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

// CancelObjectiveGitHubIssue closes the issue for a cancelled task.
func (s *SyncService) CancelObjectiveGitHubIssue(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Forgejo path
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		if task.IssueNumber.Valid {
			s.closeForgejoIssue(ctx, owner, repo, int(task.IssueNumber.Int64), "Task cancelled.", provider)
			fmt.Printf("CancelObjectiveGitHubIssue: closed Forgejo issue #%d for task %s\n", task.IssueNumber.Int64, taskID)
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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

// UpdateObjectiveStatusSync adds a status comment to the objective's issue.
func (s *SyncService) UpdateObjectiveStatusSync(taskID string, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Forgejo path
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		if task.IssueNumber.Valid {
			comment := fmt.Sprintf("Status: **%s**", status)
			if _, err := provider.AddComment(ctx, owner, repo, int(task.IssueNumber.Int64), comment); err != nil {
				fmt.Printf("UpdateObjectiveStatusSync: failed to add Forgejo comment for task %s: %v\n", taskID, err)
			}
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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

// UpdateObjectiveChecklistSync updates the objective's issue with checklist progress.
func (s *SyncService) UpdateObjectiveChecklistSync(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Forgejo path: update the issue body with current checklist
	if owner, repo, provider := s.getForgejoProjectInfo(project); provider != nil {
		if task.IssueNumber.Valid {
			checklist, cerr := s.deps.DB.GetChecklistByTaskID(taskID)
			if cerr != nil || checklist == nil {
				return
			}
			items, ierr := s.deps.DB.GetChecklistItems(checklist.ID)
			if ierr != nil || len(items) == 0 {
				return
			}
			var body string
			body = task.GetDescription() + "\n\n## Checklist\n"
			for _, item := range items {
				if item.Status == "done" {
					body += fmt.Sprintf("- [x] %s\n", item.Description)
				} else {
					body += fmt.Sprintf("- [ ] %s\n", item.Description)
				}
			}
			if err := provider.UpdateIssue(ctx, owner, repo, int(task.IssueNumber.Int64), gitprovider.UpdateIssueOpts{Body: &body}); err != nil {
				fmt.Printf("UpdateObjectiveChecklistSync: failed to update Forgejo issue for task %s: %v\n", taskID, err)
			}
		}
		return
	}

	// GitHub path
	syncConfig := s.getSyncConfig(ctx)
	if syncConfig == nil || !syncConfig.IsConfigured() {
		return
	}

	syncService := s.deps.GetGitHubSync()
	if syncService == nil {
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
