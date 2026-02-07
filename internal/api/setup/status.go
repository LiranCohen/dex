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
	CurrentStep string `json:"current_step"`
	Steps       []Step `json:"steps"`

	// Status flags
	PasskeyRegistered bool `json:"passkey_registered"`
	AnthropicKeySet   bool `json:"anthropic_key_set"`
	SetupComplete     bool `json:"setup_complete"`

	// Access info
	AccessMethod string `json:"access_method,omitempty"`
	PermanentURL string `json:"permanent_url,omitempty"`

	// Workspace info (for Forgejo)
	WorkspaceReady bool   `json:"workspace_ready"`
	WorkspacePath  string `json:"workspace_path,omitempty"`
	WorkspaceURL   string `json:"workspace_url,omitempty"`
	WorkspaceError string `json:"workspace_error,omitempty"`
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
func DetermineCurrentStep(progress *db.OnboardingProgress, hasPasskey bool, hasAnthropicKey bool) string {
	// If onboarding is marked complete, return complete
	if progress.CompletedAt.Valid {
		return db.OnboardingStepComplete
	}

	// Check from the beginning
	if !hasPasskey {
		return db.OnboardingStepPasskey
	}

	if !hasAnthropicKey {
		return db.OnboardingStepAnthropic
	}

	return db.OnboardingStepComplete
}
