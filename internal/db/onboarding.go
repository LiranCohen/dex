package db

import (
	"database/sql"
	"fmt"
	"time"
)

// OnboardingStep represents the possible steps in the onboarding flow
const (
	OnboardingStepWelcome   = "welcome"
	OnboardingStepPasskey   = "passkey"
	OnboardingStepAnthropic = "anthropic"
	OnboardingStepComplete  = "complete"
)

// OnboardingProgress represents the current state of onboarding
type OnboardingProgress struct {
	CurrentStep              string
	PasskeyCompletedAt       sql.NullTime
	GitHubOrgName            sql.NullString
	GitHubOrgID              sql.NullInt64
	GitHubAppCompletedAt     sql.NullTime
	GitHubInstallCompletedAt sql.NullTime
	AnthropicCompletedAt     sql.NullTime
	CompletedAt              sql.NullTime
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// IsComplete returns true if onboarding is fully completed
func (p *OnboardingProgress) IsComplete() bool {
	return p.CompletedAt.Valid
}

// GetGitHubOrgName returns the org name or empty string
func (p *OnboardingProgress) GetGitHubOrgName() string {
	if p.GitHubOrgName.Valid {
		return p.GitHubOrgName.String
	}
	return ""
}

// GetGitHubOrgID returns the org ID or 0
func (p *OnboardingProgress) GetGitHubOrgID() int64 {
	if p.GitHubOrgID.Valid {
		return p.GitHubOrgID.Int64
	}
	return 0
}

// GetOnboardingProgress retrieves the current onboarding progress
// Creates a new record if none exists
func (db *DB) GetOnboardingProgress() (*OnboardingProgress, error) {
	var progress OnboardingProgress
	err := db.QueryRow(`
		SELECT current_step, passkey_completed_at, github_org_name, github_org_id,
		       github_app_completed_at, github_install_completed_at,
		       anthropic_completed_at, completed_at, created_at, updated_at
		FROM onboarding_progress WHERE id = 1
	`).Scan(
		&progress.CurrentStep, &progress.PasskeyCompletedAt, &progress.GitHubOrgName, &progress.GitHubOrgID,
		&progress.GitHubAppCompletedAt, &progress.GitHubInstallCompletedAt,
		&progress.AnthropicCompletedAt, &progress.CompletedAt,
		&progress.CreatedAt, &progress.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		// Create initial progress record
		now := time.Now()
		_, err = db.Exec(`
			INSERT INTO onboarding_progress (id, current_step, created_at, updated_at)
			VALUES (1, ?, ?, ?)
		`, OnboardingStepWelcome, now, now)
		if err != nil {
			return nil, fmt.Errorf("failed to create onboarding progress: %w", err)
		}

		return &OnboardingProgress{
			CurrentStep: OnboardingStepWelcome,
			CreatedAt:   now,
			UpdatedAt:   now,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get onboarding progress: %w", err)
	}

	return &progress, nil
}

// SetOnboardingStep updates the current step
func (db *DB) SetOnboardingStep(step string) error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE onboarding_progress
		SET current_step = ?, updated_at = ?
		WHERE id = 1
	`, step, now)
	if err != nil {
		return fmt.Errorf("failed to set onboarding step: %w", err)
	}
	return nil
}

// CompletePasskeyStep marks the passkey step as complete and advances to next step
func (db *DB) CompletePasskeyStep() error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE onboarding_progress
		SET passkey_completed_at = ?, current_step = ?, updated_at = ?
		WHERE id = 1
	`, now, OnboardingStepAnthropic, now)
	if err != nil {
		return fmt.Errorf("failed to complete passkey step: %w", err)
	}
	return nil
}

// CompleteAnthropicStep marks the Anthropic API key step as complete
func (db *DB) CompleteAnthropicStep() error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE onboarding_progress
		SET anthropic_completed_at = ?, current_step = ?, updated_at = ?
		WHERE id = 1
	`, now, OnboardingStepComplete, now)
	if err != nil {
		return fmt.Errorf("failed to complete Anthropic step: %w", err)
	}
	return nil
}

// CompleteOnboarding marks the entire onboarding as complete
func (db *DB) CompleteOnboarding() error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE onboarding_progress
		SET completed_at = ?, updated_at = ?
		WHERE id = 1
	`, now, now)
	if err != nil {
		return fmt.Errorf("failed to complete onboarding: %w", err)
	}
	return nil
}

// ResetOnboarding resets the onboarding progress to the beginning
func (db *DB) ResetOnboarding() error {
	_, err := db.Exec(`DELETE FROM onboarding_progress WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("failed to reset onboarding: %w", err)
	}
	return nil
}

// IsOnboardingComplete returns true if onboarding has been completed
func (db *DB) IsOnboardingComplete() bool {
	var completedAt sql.NullTime
	err := db.QueryRow(`SELECT completed_at FROM onboarding_progress WHERE id = 1`).Scan(&completedAt)
	return err == nil && completedAt.Valid
}

// AdvanceToStep moves to a specific step (for callbacks that need to skip ahead)
func (db *DB) AdvanceToStep(step string) error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE onboarding_progress
		SET current_step = ?, updated_at = ?
		WHERE id = 1
	`, step, now)
	if err != nil {
		return fmt.Errorf("failed to advance to step: %w", err)
	}
	return nil
}
