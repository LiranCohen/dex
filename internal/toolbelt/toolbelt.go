// Package toolbelt provides clients for external services used to build projects
package toolbelt

import (
	"context"
	"time"
)

// Toolbelt holds all service clients for building user projects.
// Each client is initialized if credentials are configured.
// Nil clients indicate the service is not configured.
type Toolbelt struct {
	config *Config

	// Service clients - nil if not configured
	GitHub      *GitHubClient
	Fly         *FlyClient
	Cloudflare  *CloudflareClient
	Neon        *NeonClient
	Upstash     *UpstashClient
	Resend      *ResendClient
	BetterStack *BetterStackClient
	Doppler     *DopplerClient
	MoneyDevKit *MoneyDevKitClient
	Anthropic   *AnthropicClient
	Fal         *FalClient
}

// New creates a new Toolbelt from the given configuration.
// Service clients are initialized based on available credentials.
func New(config *Config) (*Toolbelt, error) {
	t := &Toolbelt{
		config: config,
	}

	// Initialize GitHub client if configured
	if config != nil && config.GitHub != nil {
		t.GitHub = NewGitHubClient(config.GitHub)
	}

	// Initialize Fly client if configured
	if config != nil && config.Fly != nil {
		t.Fly = NewFlyClient(config.Fly)
	}

	// Initialize Cloudflare client if configured
	if config != nil && config.Cloudflare != nil {
		t.Cloudflare = NewCloudflareClient(config.Cloudflare)
	}

	// Initialize Neon client if configured
	if config != nil && config.Neon != nil {
		t.Neon = NewNeonClient(config.Neon)
	}

	// Initialize Upstash client if configured
	if config != nil && config.Upstash != nil {
		t.Upstash = NewUpstashClient(config.Upstash)
	}

	// Initialize Resend client if configured
	if config != nil && config.Resend != nil {
		t.Resend = NewResendClient(config.Resend)
	}

	// Initialize BetterStack client if configured
	if config != nil && config.BetterStack != nil {
		t.BetterStack = NewBetterStackClient(config.BetterStack)
	}

	// Initialize Doppler client if configured
	if config != nil && config.Doppler != nil {
		t.Doppler = NewDopplerClient(config.Doppler)
	}

	// Initialize MoneyDevKit client if configured
	if config != nil && config.MoneyDevKit != nil {
		t.MoneyDevKit = NewMoneyDevKitClient(config.MoneyDevKit)
	}

	// Initialize Anthropic client if configured
	if config != nil && config.Anthropic != nil {
		t.Anthropic = NewAnthropicClient(config.Anthropic)
	}

	// Initialize Fal client if configured
	if config != nil && config.Fal != nil {
		t.Fal = NewFalClient(config.Fal)
	}

	return t, nil
}

// NewFromFile loads toolbelt configuration from a file and creates a new Toolbelt.
func NewFromFile(path string) (*Toolbelt, error) {
	config, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return New(config)
}

// Config returns the toolbelt configuration.
func (t *Toolbelt) Config() *Config {
	return t.config
}

// TestResult holds the result of testing a single service connection
type TestResult struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Latency int64  `json:"latency_ms"`
}

// TestConnections tests all configured service clients and returns results.
// Only configured services are tested; unconfigured services are skipped.
func (t *Toolbelt) TestConnections(ctx context.Context) []TestResult {
	var results []TestResult

	// Test each configured client
	if t.GitHub != nil {
		results = append(results, t.testService(ctx, "github", t.GitHub.Ping))
	}
	if t.Fly != nil {
		results = append(results, t.testService(ctx, "fly", t.Fly.Ping))
	}
	if t.Cloudflare != nil {
		results = append(results, t.testService(ctx, "cloudflare", t.Cloudflare.Ping))
	}
	if t.Neon != nil {
		results = append(results, t.testService(ctx, "neon", t.Neon.Ping))
	}
	if t.Upstash != nil {
		results = append(results, t.testService(ctx, "upstash", t.Upstash.Ping))
	}
	if t.Resend != nil {
		results = append(results, t.testService(ctx, "resend", t.Resend.Ping))
	}
	if t.BetterStack != nil {
		results = append(results, t.testService(ctx, "better_stack", t.BetterStack.Ping))
	}
	if t.Doppler != nil {
		results = append(results, t.testService(ctx, "doppler", t.Doppler.Ping))
	}
	if t.MoneyDevKit != nil {
		results = append(results, t.testService(ctx, "moneydevkit", t.MoneyDevKit.Ping))
	}
	if t.Anthropic != nil {
		results = append(results, t.testService(ctx, "anthropic", t.Anthropic.Ping))
	}
	if t.Fal != nil {
		results = append(results, t.testService(ctx, "fal", t.Fal.Ping))
	}

	return results
}

// testService tests a single service and returns the result with timing
func (t *Toolbelt) testService(ctx context.Context, name string, ping func(context.Context) error) TestResult {
	start := time.Now()
	err := ping(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return TestResult{
			Name:    name,
			Success: false,
			Error:   err.Error(),
			Latency: latency,
		}
	}

	return TestResult{
		Name:    name,
		Success: true,
		Message: "connected",
		Latency: latency,
	}
}

// Status returns the configuration status of all services.
func (t *Toolbelt) Status() []ServiceStatus {
	if t.config == nil {
		return []ServiceStatus{
			{Name: "github", Configured: false, HasToken: false},
			{Name: "fly", Configured: false, HasToken: false},
			{Name: "cloudflare", Configured: false, HasToken: false},
			{Name: "neon", Configured: false, HasToken: false},
			{Name: "upstash", Configured: false, HasToken: false},
			{Name: "resend", Configured: false, HasToken: false},
			{Name: "better_stack", Configured: false, HasToken: false},
			{Name: "doppler", Configured: false, HasToken: false},
			{Name: "moneydevkit", Configured: false, HasToken: false},
			{Name: "anthropic", Configured: false, HasToken: false},
			{Name: "fal", Configured: false, HasToken: false},
		}
	}
	return t.config.Status()
}
