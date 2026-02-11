package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// DexProfile holds the Dex personality data received from Central.
type DexProfile struct {
	Traits             []string        `json:"traits"`
	GreetingStyle      string          `json:"greeting_style"`
	Catchphrase        string          `json:"catchphrase"`
	Avatar             []byte          `json:"-"`
	AvatarURL          string          `json:"avatar_url,omitempty"`
	OnboardingMessages json.RawMessage `json:"onboarding_messages,omitempty"`
}

// GetDexProfile retrieves the stored Dex profile, or nil if none exists.
func (db *DB) GetDexProfile() (*DexProfile, error) {
	var (
		traitsJSON         sql.NullString
		greetingStyle      sql.NullString
		catchphrase        sql.NullString
		avatar             []byte
		avatarURL          sql.NullString
		onboardingMessages sql.NullString
	)

	err := db.QueryRow(`
		SELECT traits, greeting_style, catchphrase, avatar, avatar_url, onboarding_messages
		FROM dex_profile WHERE id = 1
	`).Scan(&traitsJSON, &greetingStyle, &catchphrase, &avatar, &avatarURL, &onboardingMessages)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dex profile: %w", err)
	}

	profile := &DexProfile{
		GreetingStyle: greetingStyle.String,
		Catchphrase:   catchphrase.String,
		Avatar:        avatar,
		AvatarURL:     avatarURL.String,
	}

	if traitsJSON.Valid && traitsJSON.String != "" {
		if err := json.Unmarshal([]byte(traitsJSON.String), &profile.Traits); err != nil {
			profile.Traits = nil
		}
	}

	if onboardingMessages.Valid && onboardingMessages.String != "" {
		profile.OnboardingMessages = json.RawMessage(onboardingMessages.String)
	}

	return profile, nil
}

// SaveDexProfile inserts or replaces the Dex profile (singleton row).
func (db *DB) SaveDexProfile(profile *DexProfile) error {
	traitsJSON, err := json.Marshal(profile.Traits)
	if err != nil {
		return fmt.Errorf("marshaling traits: %w", err)
	}

	var messagesStr string
	if len(profile.OnboardingMessages) > 0 {
		messagesStr = string(profile.OnboardingMessages)
	}

	_, err = db.Exec(`
		INSERT OR REPLACE INTO dex_profile (id, traits, greeting_style, catchphrase, avatar, avatar_url, onboarding_messages, created_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, string(traitsJSON), profile.GreetingStyle, profile.Catchphrase, profile.Avatar, profile.AvatarURL, messagesStr)
	if err != nil {
		return fmt.Errorf("failed to save dex profile: %w", err)
	}

	return nil
}

// UpdateDexAvatar updates just the avatar bytes for the stored profile.
func (db *DB) UpdateDexAvatar(avatar []byte) error {
	_, err := db.Exec(`UPDATE dex_profile SET avatar = ? WHERE id = 1`, avatar)
	if err != nil {
		return fmt.Errorf("failed to update dex avatar: %w", err)
	}
	return nil
}

// HasDexProfile returns true if a Dex profile has been stored.
func (db *DB) HasDexProfile() bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM dex_profile WHERE id = 1`).Scan(&count)
	return err == nil && count > 0
}
