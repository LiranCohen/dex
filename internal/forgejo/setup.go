package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// User account constants for Forgejo bootstrap.
const (
	AdminUsername = "dex-admin"
	AdminEmail    = "admin@hq.local"
	BotUsername   = "dex-bot"
	BotEmail      = "bot@hq.local"
)

// bootstrap performs first-run setup: creates admin and bot accounts,
// generates API tokens, and stores them in the Dex database.
func (m *Manager) bootstrap(ctx context.Context) error {
	// 1. Create admin user via Forgejo CLI
	adminPassword, err := generateSecret(16)
	if err != nil {
		return fmt.Errorf("failed to generate admin password: %w", err)
	}

	if err := m.cliCreateUser(ctx, AdminUsername, AdminEmail, adminPassword, true); err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	// Store admin password so the user can log into the Forgejo web UI
	if err := m.db.SetSecret(SecretKeyAdminPassword, adminPassword); err != nil {
		return fmt.Errorf("failed to store admin password: %w", err)
	}

	// 2. Generate admin API token via CLI
	adminToken, err := m.cliCreateToken(ctx, AdminUsername, "dex-admin-token")
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

	if err := m.apiCreateUser(ctx, adminToken, BotUsername, BotEmail, botPassword); err != nil {
		return fmt.Errorf("failed to create bot user: %w", err)
	}

	// 4. Generate bot API token via CLI
	botToken, err := m.cliCreateToken(ctx, BotUsername, "dex-bot-token")
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
	if err := m.apiAddOrgMember(ctx, adminToken, orgName, BotUsername); err != nil {
		return fmt.Errorf("failed to add bot to org: %w", err)
	}

	// 7. Configure OAuth2 SSO if OIDC issuer is set
	if m.config.OIDCIssuer != "" {
		if err := m.setupOAuth2SSO(ctx); err != nil {
			return fmt.Errorf("failed to setup OAuth2 SSO: %w", err)
		}
	}

	return nil
}

// setupOAuth2SSO configures Forgejo to use HQ as its OAuth2/OIDC provider.
func (m *Manager) setupOAuth2SSO(ctx context.Context) error {
	// Generate OAuth client secret
	oauthSecret, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("failed to generate OAuth secret: %w", err)
	}

	// Store the secret for later use when registering with OIDC provider
	if err := m.db.SetSecret(SecretKeyOAuthSecret, oauthSecret); err != nil {
		return fmt.Errorf("failed to store OAuth secret: %w", err)
	}

	// Use explicit OIDC endpoints instead of auto-discovery URL.
	// Auto-discovery requires the HQ OIDC provider to be reachable during bootstrap,
	// but the HQ HTTP server starts AFTER Forgejo bootstrap completes.
	// By using explicit endpoints, we avoid the chicken-and-egg problem.
	issuer := m.config.OIDCIssuer
	authURL := issuer + "/oidc/authorize"
	tokenURL := issuer + "/oidc/token"
	profileURL := issuer + "/oidc/userinfo"

	// Add OAuth2 authentication source via Forgejo CLI
	// Use explicit endpoint URLs to avoid needing to reach the discovery endpoint during bootstrap
	// Forgejo requires --use-custom-urls and uses --custom-* prefix for custom URLs
	_, err = m.runCLI(ctx,
		"admin", "auth", "add-oauth",
		"--name", "hq",
		"--provider", "openidConnect",
		"--key", OAuthClientID,
		"--secret", oauthSecret,
		"--use-custom-urls",
		"--custom-auth-url", authURL,
		"--custom-token-url", tokenURL,
		"--custom-profile-url", profileURL,
		"--scopes", "openid email profile",
	)
	if err != nil {
		return fmt.Errorf("failed to add OAuth2 source: %w", err)
	}

	// Delete the default "Local" authentication source (ID 1) to hide username/password form
	// This makes SSO the ONLY way to sign in
	_, err = m.runCLI(ctx, "admin", "auth", "delete", "--id", "1")
	if err != nil {
		// Log but don't fail - the local source might already be removed
		fmt.Printf("Note: Could not delete local auth source: %v\n", err)
	} else {
		fmt.Println("Deleted local authentication source (SSO-only mode)")
	}

	fmt.Println("OAuth2 SSO configured with HQ as identity provider")
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
	return m.apiAddOrgMember(ctx, adminToken, org, BotUsername)
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

	// Build clean environment with our vars taking precedence
	forgejoEnv := m.config.EnvVars()
	forgejoKeys := make(map[string]bool)
	for _, e := range forgejoEnv {
		if idx := strings.Index(e, "="); idx > 0 {
			forgejoKeys[e[:idx]] = true
		}
	}

	// Filter out keys we're setting, then append ours
	for _, e := range os.Environ() {
		if idx := strings.Index(e, "="); idx > 0 {
			if !forgejoKeys[e[:idx]] {
				cmd.Env = append(cmd.Env, e)
			}
		}
	}
	cmd.Env = append(cmd.Env, forgejoEnv...)

	// If running as root, run CLI as the same user that owns the database
	if os.Getuid() == 0 {
		runUser := m.config.RunUser
		if runUser == "" {
			runUser = "nobody"
		}

		u, err := user.Lookup(runUser)
		if err != nil {
			return "", fmt.Errorf("failed to lookup user %s: %w", runUser, err)
		}

		uid, _ := strconv.ParseUint(u.Uid, 10, 32)
		gid, _ := strconv.ParseUint(u.Gid, 10, 32)

		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
	}

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
