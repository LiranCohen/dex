package api

import (
	"context"
	"fmt"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/github"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// ========== GitHub App Management ==========

// initGitHubApp initializes the GitHub App manager from database configuration.
// It also sets up the session manager's GitHub client fetcher if not already set.
// This method is safe to call multiple times - it will reinitialize if needed.
func (s *Server) initGitHubApp() error {
	config, err := s.db.GetGitHubAppConfig()
	if err != nil {
		return fmt.Errorf("failed to get GitHub App config: %w", err)
	}
	if config == nil {
		return fmt.Errorf("no GitHub App configured")
	}

	appManager, err := github.NewAppManager(&github.AppConfig{
		AppID:         config.AppID,
		AppSlug:       config.AppSlug,
		ClientID:      config.ClientID,
		ClientSecret:  config.ClientSecret,
		PrivateKeyPEM: config.PrivateKey,
		WebhookSecret: config.WebhookSecret,
	})
	if err != nil {
		return fmt.Errorf("failed to create GitHub App manager: %w", err)
	}

	s.githubAppMu.Lock()
	s.githubApp = appManager
	s.githubSyncService = github.NewSyncService(s.db, appManager)
	s.githubAppMu.Unlock()

	fmt.Printf("initGitHubApp: GitHub App manager initialized for %s\n", config.AppSlug)

	// Set up session manager fetcher if we have a session manager
	if s.sessionManager != nil {
		s.sessionManager.SetGitHubClientFetcher(func(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
			return s.GetToolbeltGitHubClient(ctx, login)
		})
		fmt.Println("initGitHubApp: Session manager GitHub client fetcher configured")
	}

	// Set up quest handler fetcher if we have a quest handler
	if s.questHandler != nil {
		s.questHandler.SetGitHubClientFetcher(func(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
			return s.GetToolbeltGitHubClient(ctx, login)
		})
		fmt.Println("initGitHubApp: Quest handler GitHub client fetcher configured")
	}

	return nil
}

// GetToolbeltGitHubClient returns a toolbelt.GitHubClient for the specified installation login.
// If login is empty, returns a client for the first available installation.
// This wraps the GitHub App installation token in a toolbelt-compatible client.
func (s *Server) GetToolbeltGitHubClient(ctx context.Context, login string) (*toolbelt.GitHubClient, error) {
	s.githubAppMu.RLock()
	appManager := s.githubApp
	s.githubAppMu.RUnlock()

	if appManager == nil {
		return nil, fmt.Errorf("GitHub App not configured")
	}

	// Get installation from database (need login and account type)
	var accountType string
	if login == "" {
		installations, err := s.db.ListGitHubInstallations()
		if err != nil {
			return nil, fmt.Errorf("failed to list installations: %w", err)
		}
		if len(installations) == 0 {
			return nil, fmt.Errorf("no GitHub App installations found")
		}
		login = installations[0].Login
		accountType = installations[0].AccountType
		fmt.Printf("GetToolbeltGitHubClient: using default installation %s (type: %s)\n", login, accountType)
	} else {
		// Look up account type for specified login
		installations, err := s.db.ListGitHubInstallations()
		if err != nil {
			return nil, fmt.Errorf("failed to list installations: %w", err)
		}
		for _, inst := range installations {
			if inst.Login == login {
				accountType = inst.AccountType
				break
			}
		}
	}

	// Get installation ID for login
	installID, err := appManager.GetInstallationIDForLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation for %s: %w", login, err)
	}

	// Get installation token
	token, err := appManager.GetInstallationToken(ctx, installID)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	// Create toolbelt client from token
	// accountType determines how repos are created (user vs org)
	return toolbelt.NewGitHubClientFromToken(token, login, accountType), nil
}

// ========== GitHub Sync Operations ==========
// These methods delegate to the handlers sync service which handles all the
// complexity of getting repo info, installation IDs, etc.

// syncQuestToGitHubIssue syncs a quest to a GitHub Issue
func (s *Server) syncQuestToGitHubIssue(questID string) {
	s.handlersSyncSvc.SyncQuestToGitHubIssue(questID)
}

// closeQuestGitHubIssue closes the GitHub Issue for a completed quest
func (s *Server) closeQuestGitHubIssue(questID string, summary *db.QuestSummary) {
	s.handlersSyncSvc.CloseQuestGitHubIssue(questID, summary)
}

// reopenQuestGitHubIssue reopens the GitHub Issue for a reopened quest
func (s *Server) reopenQuestGitHubIssue(questID string) {
	s.handlersSyncSvc.ReopenQuestGitHubIssue(questID)
}

// syncObjectiveToGitHubIssue syncs a task (objective) to a GitHub Issue
func (s *Server) syncObjectiveToGitHubIssue(taskID string) {
	s.handlersSyncSvc.SyncObjectiveToGitHubIssue(taskID)
}
