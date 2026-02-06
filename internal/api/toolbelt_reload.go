package api

import (
	"fmt"
	"os"

	"github.com/lirancohen/dex/internal/db"
	"github.com/lirancohen/dex/internal/planning"
	"github.com/lirancohen/dex/internal/quest"
	"github.com/lirancohen/dex/internal/toolbelt"
)

// getDataDir returns the data directory path
func (s *Server) getDataDir() string {
	// Use configured baseDir first
	if s.baseDir != "" {
		return s.baseDir
	}
	// Fall back to environment variable
	if dir := os.Getenv("DEX_DATA_DIR"); dir != "" {
		return dir
	}
	return "/opt/dex"
}

// ReloadToolbelt reloads the toolbelt from database secrets and updates the session manager
// This is called after setup completes when API keys are first entered
func (s *Server) ReloadToolbelt() error {
	// First try to migrate any existing secrets from file to database
	dataDir := s.getDataDir()
	if count, err := s.db.MigrateSecretsFromFile(dataDir); err != nil {
		fmt.Printf("ReloadToolbelt: failed to migrate secrets from file: %v\n", err)
	} else if count > 0 {
		fmt.Printf("ReloadToolbelt: migrated %d secrets from file to database\n", count)
	}

	// Load secrets from database
	secrets, err := s.db.GetAllSecrets()
	if err != nil {
		return fmt.Errorf("failed to load secrets from database: %w", err)
	}

	fmt.Printf("ReloadToolbelt: loading from database (%d secrets)\n", len(secrets))

	// Build toolbelt config from secrets
	config := &toolbelt.Config{}
	if token := secrets[db.SecretKeyGitHubToken]; token != "" {
		config.GitHub = &toolbelt.GitHubConfig{Token: token}
	}
	if key := secrets[db.SecretKeyAnthropicKey]; key != "" {
		config.Anthropic = &toolbelt.AnthropicConfig{APIKey: key}
	}

	tb, err := toolbelt.New(config)
	if err != nil {
		return fmt.Errorf("failed to create toolbelt: %w", err)
	}

	s.toolbeltMu.Lock()
	s.toolbelt = tb
	s.toolbeltMu.Unlock()

	// Update session manager with new clients
	if tb.Anthropic != nil {
		fmt.Println("ReloadToolbelt: Anthropic client initialized, updating session manager")
		s.sessionManager.SetAnthropicClient(tb.Anthropic)

		// Update planner with new Anthropic client
		if s.planner == nil {
			s.planner = planning.NewPlanner(s.db, tb.Anthropic, s.broadcaster)
			s.planner.SetPromptLoader(s.sessionManager.GetPromptLoader())
			s.deps.Planner = s.planner
			fmt.Println("ReloadToolbelt: Planner created")
		}

		// Update quest handler with new Anthropic client
		if s.questHandler == nil {
			s.questHandler = quest.NewHandler(s.db, tb.Anthropic, s.broadcaster)
			s.questHandler.SetPromptLoader(s.sessionManager.GetPromptLoader())
			s.questHandler.SetBaseDir(s.getDataDir())
			s.deps.QuestHandler = s.questHandler
			fmt.Println("ReloadToolbelt: Quest handler created")
		}
	}

	// Log status
	status := tb.Status()
	configured := 0
	for _, svc := range status {
		if svc.HasToken {
			configured++
		}
	}
	fmt.Printf("ReloadToolbelt: %d/%d services configured\n", configured, len(status))

	return nil
}
