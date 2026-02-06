package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	adminUsername = "dex-admin"
	adminEmail   = "admin@hq.local"
	botUsername   = "dex-bot"
	botEmail     = "bot@hq.local"
)

// bootstrap performs first-run setup: creates admin and bot accounts,
// generates API tokens, and stores them in the Dex database.
func (m *Manager) bootstrap(ctx context.Context) error {
	// 1. Create admin user via Forgejo CLI
	adminPassword, err := generateSecret(16)
	if err != nil {
		return fmt.Errorf("failed to generate admin password: %w", err)
	}

	if err := m.cliCreateUser(ctx, adminUsername, adminEmail, adminPassword, true); err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	// Store admin password so the user can log into the Forgejo web UI
	if err := m.db.SetSecret(SecretKeyAdminPassword, adminPassword); err != nil {
		return fmt.Errorf("failed to store admin password: %w", err)
	}

	// 2. Generate admin API token via CLI
	adminToken, err := m.cliCreateToken(ctx, adminUsername, "dex-admin-token")
	if err != nil {
		return fmt.Errorf("failed to create admin token: %w", err)
	}

	// Store admin token immediately so we can use the API
	if err := m.db.SetSecret(SecretKeyAdminToken, adminToken); err != nil {
		return fmt.Errorf("failed to store admin token: %w", err)
	}

	// 3. Create bot user via API
	botPassword, err := generateSecret(16)
	if err != nil {
		return fmt.Errorf("failed to generate bot password: %w", err)
	}

	if err := m.apiCreateUser(ctx, adminToken, botUsername, botEmail, botPassword); err != nil {
		return fmt.Errorf("failed to create bot user: %w", err)
	}

	// 4. Generate bot API token via CLI
	botToken, err := m.cliCreateToken(ctx, botUsername, "dex-bot-token")
	if err != nil {
		return fmt.Errorf("failed to create bot token: %w", err)
	}

	// Store bot token
	if err := m.db.SetSecret(SecretKeyBotToken, botToken); err != nil {
		return fmt.Errorf("failed to store bot token: %w", err)
	}

	// 5. Create default organization so projects have a home
	orgName := m.config.GetDefaultOrgName()
	if err := m.apiCreateOrg(ctx, adminToken, orgName); err != nil {
		return fmt.Errorf("failed to create default org: %w", err)
	}

	// 6. Add bot user to the org so it can create repos and PRs
	if err := m.apiAddOrgMember(ctx, adminToken, orgName, botUsername); err != nil {
		return fmt.Errorf("failed to add bot to org: %w", err)
	}

	return nil
}

// CreateOrg creates a Forgejo organization using the admin API token.
func (m *Manager) CreateOrg(ctx context.Context, name string) error {
	adminToken, err := m.AdminToken()
	if err != nil {
		return err
	}
	return m.apiCreateOrg(ctx, adminToken, name)
}

// CreateRepo creates a repository in a Forgejo organization using the bot token.
func (m *Manager) CreateRepo(ctx context.Context, org, name string) error {
	botToken, err := m.BotToken()
	if err != nil {
		return err
	}
	return m.apiCreateOrgRepo(ctx, botToken, org, name)
}

// AddBotToOrg adds the bot user as an owner of the given organization.
func (m *Manager) AddBotToOrg(ctx context.Context, org string) error {
	adminToken, err := m.AdminToken()
	if err != nil {
		return err
	}
	return m.apiAddOrgMember(ctx, adminToken, org, botUsername)
}

// --- CLI helpers ---

func (m *Manager) cliCreateUser(ctx context.Context, username, email, password string, admin bool) error {
	args := []string{
		"admin", "user", "create",
		"--username", username,
		"--password", password,
		"--email", email,
	}
	if admin {
		args = append(args, "--admin")
	}
	args = append(args, "--must-change-password=false")

	_, err := m.runCLI(ctx, args...)
	return err
}

func (m *Manager) cliCreateToken(ctx context.Context, username, tokenName string) (string, error) {
	output, err := m.runCLI(ctx,
		"admin", "user", "generate-access-token",
		"--username", username,
		"--token-name", tokenName,
		"--scopes", "all",
		"--raw",
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (m *Manager) runCLI(ctx context.Context, args ...string) (string, error) {
	binaryPath := m.config.GetBinaryPath()

	fullArgs := append([]string{
		"--config", m.config.GetAppIniPath(),
		"--work-path", m.config.DataDir,
	}, args...)

	cmd := exec.CommandContext(ctx, binaryPath, fullArgs...)
	cmd.Env = append(cmd.Environ(),
		"FORGEJO_WORK_DIR="+m.config.DataDir,
		"FORGEJO_CUSTOM="+m.config.DataDir+"/custom",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("forgejo CLI failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// --- API helpers ---

func (m *Manager) apiCreateUser(ctx context.Context, adminToken, username, email, password string) error {
	body := map[string]interface{}{
		"username":             username,
		"email":                email,
		"password":             password,
		"must_change_password": false,
		"visibility":           "private",
	}
	_, err := m.apiRequest(ctx, adminToken, "POST", "/api/v1/admin/users", body)
	return err
}

func (m *Manager) apiCreateOrg(ctx context.Context, token, name string) error {
	body := map[string]interface{}{
		"username":   name,
		"visibility": "private",
	}
	_, err := m.apiRequest(ctx, token, "POST", "/api/v1/orgs", body)
	return err
}

func (m *Manager) apiCreateOrgRepo(ctx context.Context, token, org, name string) error {
	body := map[string]interface{}{
		"name":          name,
		"private":       true,
		"auto_init":     true,
		"default_branch": "main",
	}
	_, err := m.apiRequest(ctx, token, "POST", fmt.Sprintf("/api/v1/orgs/%s/repos", org), body)
	return err
}

func (m *Manager) apiAddOrgMember(ctx context.Context, token, org, username string) error {
	// Add user to the "Owners" team of the org.
	// First, list teams to find the Owners team ID.
	resp, err := m.apiRequest(ctx, token, "GET", fmt.Sprintf("/api/v1/orgs/%s/teams", org), nil)
	if err != nil {
		return fmt.Errorf("failed to list org teams: %w", err)
	}

	var teams []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp, &teams); err != nil {
		return fmt.Errorf("failed to parse teams response: %w", err)
	}

	var ownersTeamID int64
	for _, t := range teams {
		if t.Name == "Owners" {
			ownersTeamID = t.ID
			break
		}
	}
	if ownersTeamID == 0 {
		return fmt.Errorf("owners team not found for org %s", org)
	}

	// Add user to the team
	_, err = m.apiRequest(ctx, token, "PUT", fmt.Sprintf("/api/v1/teams/%d/members/%s", ownersTeamID, username), nil)
	return err
}

func (m *Manager) apiRequest(ctx context.Context, token, method, path string, body interface{}) ([]byte, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(data)
	}

	url := m.BaseURL() + path

	var req *http.Request
	var err error
	if reqBody != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, reqBody)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var respBody bytes.Buffer
	_, _ = respBody.ReadFrom(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API %s %s returned %d: %s", method, path, resp.StatusCode, respBody.String())
	}

	return respBody.Bytes(), nil
}
