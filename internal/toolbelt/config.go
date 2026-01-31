// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all toolbelt service configurations
type Config struct {
	GitHub      *GitHubConfig      `yaml:"github,omitempty"`
	Fly         *FlyConfig         `yaml:"fly,omitempty"`
	Cloudflare  *CloudflareConfig  `yaml:"cloudflare,omitempty"`
	Neon        *NeonConfig        `yaml:"neon,omitempty"`
	Upstash     *UpstashConfig     `yaml:"upstash,omitempty"`
	Resend      *ResendConfig      `yaml:"resend,omitempty"`
	BetterStack *BetterStackConfig `yaml:"better_stack,omitempty"`
	Doppler     *DopplerConfig     `yaml:"doppler,omitempty"`
	MoneyDevKit *MoneyDevKitConfig `yaml:"moneydevkit,omitempty"`
	Anthropic   *AnthropicConfig   `yaml:"anthropic,omitempty"`
	Fal         *FalConfig         `yaml:"fal,omitempty"`
}

// GitHubConfig holds GitHub API configuration
type GitHubConfig struct {
	Token      string `yaml:"token"`
	DefaultOrg string `yaml:"default_org,omitempty"`
}

// FlyConfig holds Fly.io API configuration
type FlyConfig struct {
	Token         string `yaml:"token"`
	DefaultRegion string `yaml:"default_region,omitempty"`
}

// CloudflareConfig holds Cloudflare API configuration
type CloudflareConfig struct {
	APIToken  string `yaml:"api_token"`
	AccountID string `yaml:"account_id"`
}

// NeonConfig holds Neon serverless PostgreSQL configuration
type NeonConfig struct {
	APIKey        string `yaml:"api_key"`
	DefaultRegion string `yaml:"default_region,omitempty"`
}

// UpstashConfig holds Upstash Redis/Queue configuration
type UpstashConfig struct {
	Email       string `yaml:"email"`
	APIKey      string `yaml:"api_key"`
	QStashToken string `yaml:"qstash_token,omitempty"`
}

// ResendConfig holds Resend email API configuration
type ResendConfig struct {
	APIKey string `yaml:"api_key"`
}

// BetterStackConfig holds Better Stack monitoring configuration
type BetterStackConfig struct {
	APIToken string `yaml:"api_token"`
}

// DopplerConfig holds Doppler secrets management configuration
type DopplerConfig struct {
	Token string `yaml:"token"`
}

// MoneyDevKitConfig holds MoneyDevKit payments configuration
type MoneyDevKitConfig struct {
	APIKey        string `yaml:"api_key"`
	WebhookSecret string `yaml:"webhook_secret,omitempty"`
}

// AnthropicConfig holds Anthropic Claude API configuration
type AnthropicConfig struct {
	APIKey string `yaml:"api_key"`
}

// FalConfig holds fal.ai media generation configuration
type FalConfig struct {
	APIKey string `yaml:"api_key"`
}

// ServiceStatus represents the configuration status of a single service
type ServiceStatus struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	HasToken   bool   `json:"has_token"`
}

// Status returns the configuration status of all services
func (c *Config) Status() []ServiceStatus {
	return []ServiceStatus{
		{Name: "github", Configured: c.GitHub != nil, HasToken: c.GitHub != nil && c.GitHub.Token != ""},
		{Name: "fly", Configured: c.Fly != nil, HasToken: c.Fly != nil && c.Fly.Token != ""},
		{Name: "cloudflare", Configured: c.Cloudflare != nil, HasToken: c.Cloudflare != nil && c.Cloudflare.APIToken != ""},
		{Name: "neon", Configured: c.Neon != nil, HasToken: c.Neon != nil && c.Neon.APIKey != ""},
		{Name: "upstash", Configured: c.Upstash != nil, HasToken: c.Upstash != nil && c.Upstash.APIKey != ""},
		{Name: "resend", Configured: c.Resend != nil, HasToken: c.Resend != nil && c.Resend.APIKey != ""},
		{Name: "better_stack", Configured: c.BetterStack != nil, HasToken: c.BetterStack != nil && c.BetterStack.APIToken != ""},
		{Name: "doppler", Configured: c.Doppler != nil, HasToken: c.Doppler != nil && c.Doppler.Token != ""},
		{Name: "moneydevkit", Configured: c.MoneyDevKit != nil, HasToken: c.MoneyDevKit != nil && c.MoneyDevKit.APIKey != ""},
		{Name: "anthropic", Configured: c.Anthropic != nil, HasToken: c.Anthropic != nil && c.Anthropic.APIKey != ""},
		{Name: "fal", Configured: c.Fal != nil, HasToken: c.Fal != nil && c.Fal.APIKey != ""},
	}
}

// envVarPattern matches ${VAR_NAME} patterns for environment variable expansion
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars expands ${VAR_NAME} patterns in the input string with environment variable values
func expandEnvVars(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		if value := os.Getenv(varName); value != "" {
			return value
		}
		// Return empty string if env var is not set
		return ""
	})
}

// LoadConfig loads toolbelt configuration from the specified YAML file
// Environment variables referenced as ${VAR_NAME} are expanded
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read toolbelt config: %w", err)
	}

	// Expand environment variables in the raw YAML content
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse toolbelt config: %w", err)
	}

	return &config, nil
}

// LoadFromSecrets loads toolbelt configuration from a secrets.json file
// This is used for dex installations where API keys are stored during setup
func LoadFromSecrets(secretsPath string) (*Config, error) {
	data, err := os.ReadFile(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	var secrets map[string]string
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets file: %w", err)
	}

	config := &Config{}

	// Map secrets to config
	if token := secrets["github_token"]; token != "" {
		config.GitHub = &GitHubConfig{Token: token}
	}
	if key := secrets["anthropic_key"]; key != "" {
		config.Anthropic = &AnthropicConfig{APIKey: key}
	}

	return config, nil
}

// NewFromSecrets creates a Toolbelt from a secrets.json file
func NewFromSecrets(secretsPath string) (*Toolbelt, error) {
	config, err := LoadFromSecrets(secretsPath)
	if err != nil {
		return nil, err
	}
	return New(config)
}
