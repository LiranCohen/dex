package setup

import (
	"github.com/lirancohen/dex/internal/db"
)

// Step represents a single step in the onboarding flow
type Step struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // "pending", "current", "complete"
	Skippable bool   `json:"skippable"`
}

// SetupStatus represents the current setup status
type SetupStatus struct {
	CurrentStep   string `json:"current_step"`
	Steps         []Step `json:"steps"`
	GitHubOrg     string `json:"github_org,omitempty"`
	GitHubOrgID   int64  `json:"github_org_id,omitempty"`
	GitHubAppSlug string `json:"github_app_slug,omitempty"`
	WorkspaceURL  string `json:"workspace_url,omitempty"`

	// Legacy fields for backward compatibility during transition
	PasskeyRegistered    bool   `json:"passkey_registered"`
	GitHubTokenSet       bool   `json:"github_token_set"`
	GitHubAppSet         bool   `json:"github_app_set"`
	GitHubAuthMethod     string `json:"github_auth_method"`
	AnthropicKeySet      bool   `json:"anthropic_key_set"`
	SetupComplete        bool   `json:"setup_complete"`
	AccessMethod         string `json:"access_method,omitempty"`
	PermanentURL         string `json:"permanent_url,omitempty"`
	WorkspaceReady       bool   `json:"workspace_ready"`
	WorkspacePath        string `json:"workspace_path,omitempty"`
	WorkspaceGitHubReady bool   `json:"workspace_github_ready"`
	WorkspaceGitHubURL   string `json:"workspace_github_url,omitempty"`
	WorkspaceError       string `json:"workspace_error,omitempty"`
}

// StepDefinitions returns the ordered list of onboarding steps
func StepDefinitions() []struct {
	ID   string
	Name string
} {
	return []struct {
		ID   string
		Name string
	}{
		{ID: db.OnboardingStepWelcome, Name: "Welcome"},
		{ID: db.OnboardingStepPasskey, Name: "Security"},
		{ID: db.OnboardingStepGitHubOrg, Name: "GitHub Organization"},
		{ID: db.OnboardingStepGitHubApp, Name: "Create GitHub App"},
		{ID: db.OnboardingStepGitHubInstall, Name: "Install App"},
		{ID: db.OnboardingStepAnthropic, Name: "Anthropic API"},
		{ID: db.OnboardingStepComplete, Name: "Complete"},
	}
}

// BuildSteps builds the steps array with current status
func BuildSteps(progress *db.OnboardingProgress) []Step {
	definitions := StepDefinitions()
	steps := make([]Step, len(definitions))

	currentFound := false
	for i, def := range definitions {
		status := "pending"

		// Determine step status based on completion timestamps
		if isStepComplete(progress, def.ID) {
			status = "complete"
		} else if def.ID == progress.CurrentStep {
			status = "current"
			currentFound = true
		} else if currentFound {
			status = "pending"
		} else {
			// Steps before current that aren't complete
			status = "complete"
		}

		steps[i] = Step{
			ID:        def.ID,
			Name:      def.Name,
			Status:    status,
			Skippable: false,
		}
	}

	return steps
}

// isStepComplete checks if a specific step is complete
func isStepComplete(progress *db.OnboardingProgress, stepID string) bool {
	switch stepID {
	case db.OnboardingStepWelcome:
		// Welcome is complete if we've moved past it
		return progress.CurrentStep != db.OnboardingStepWelcome
	case db.OnboardingStepPasskey:
		return progress.PasskeyCompletedAt.Valid
	case db.OnboardingStepGitHubOrg:
		return progress.GitHubOrgName.Valid && progress.GitHubOrgName.String != ""
	case db.OnboardingStepGitHubApp:
		return progress.GitHubAppCompletedAt.Valid
	case db.OnboardingStepGitHubInstall:
		return progress.GitHubInstallCompletedAt.Valid
	case db.OnboardingStepAnthropic:
		return progress.AnthropicCompletedAt.Valid
	case db.OnboardingStepComplete:
		return progress.CompletedAt.Valid
	default:
		return false
	}
}

// DetermineCurrentStep determines what step the user should be on based on progress
// This is used to recover from incomplete state
func DetermineCurrentStep(progress *db.OnboardingProgress, hasPasskey bool, hasGitHubApp bool, hasInstallation bool, hasAnthropicKey bool) string {
	// If onboarding is marked complete, return complete
	if progress.CompletedAt.Valid {
		return db.OnboardingStepComplete
	}

	// Check from the beginning
	if !hasPasskey {
		return db.OnboardingStepPasskey
	}

	if progress.GitHubOrgName.String == "" {
		return db.OnboardingStepGitHubOrg
	}

	if !hasGitHubApp {
		return db.OnboardingStepGitHubApp
	}

	if !hasInstallation {
		return db.OnboardingStepGitHubInstall
	}

	if !hasAnthropicKey {
		return db.OnboardingStepAnthropic
	}

	return db.OnboardingStepComplete
}
