package mesh

// Config holds the mesh client configuration.
type Config struct {
	// Enabled determines whether mesh networking is active.
	Enabled bool `yaml:"enabled"`

	// Hostname is how this instance appears on the mesh network.
	Hostname string `yaml:"hostname"`

	// StateDir is the directory for persistent mesh state.
	StateDir string `yaml:"state_dir"`

	// ControlURL is the Central coordination service URL.
	// Production: https://central.enbox.id
	// For local development, use http://localhost:8080 or similar.
	ControlURL string `yaml:"control_url"`

	// AuthKey is a pre-auth key for automatic node registration.
	// Used primarily for worker/agent enrollment.
	// If empty, interactive authentication is required.
	AuthKey string `yaml:"auth_key"`

	// IsHQ marks this node as the HQ (headquarters) node.
	// There should be exactly one HQ per user.
	IsHQ bool `yaml:"is_hq"`

	// Tunnel holds configuration for the HQ-initiated tunnel to Ingress.
	Tunnel TunnelSettings `yaml:"tunnel"`
}

// TunnelSettings holds configuration for the HQ tunnel to Ingress.
type TunnelSettings struct {
	// Enabled determines whether the tunnel to Ingress is active.
	Enabled bool `yaml:"enabled"`

	// IngressAddr is the address of the Ingress tunnel listener.
	// Example: "ingress.enbox.id:9443"
	IngressAddr string `yaml:"ingress_addr"`

	// Token is the authentication token for this HQ.
	Token string `yaml:"token"`

	// Endpoints is the list of local services to expose via Ingress.
	Endpoints []EndpointMapping `yaml:"endpoints"`

	// ACME holds configuration for automatic TLS certificate management.
	ACME ACMESettings `yaml:"acme"`
}

// ACMESettings holds configuration for ACME certificate management.
type ACMESettings struct {
	// Enabled determines whether automatic certificate management is active.
	// When enabled, HQ terminates TLS and proxies plaintext to local services.
	// When disabled, TLS is passed through to local services.
	Enabled bool `yaml:"enabled"`

	// Email is the ACME account email for Let's Encrypt.
	Email string `yaml:"email"`

	// Staging uses Let's Encrypt staging environment (for testing).
	Staging bool `yaml:"staging"`

	// CertDir is the directory to store certificates.
	// Defaults to StateDir/certs if not specified.
	CertDir string `yaml:"cert_dir"`
}

// EndpointMapping maps a public hostname to a local port.
type EndpointMapping struct {
	// Hostname is the public hostname (e.g., "api.alice.enbox.id").
	Hostname string `yaml:"hostname"`
	// LocalPort is the local port to forward traffic to.
	LocalPort int `yaml:"local_port"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:    false,
		StateDir:   "/var/lib/dex/mesh",
		ControlURL: "https://central.enbox.id",
		IsHQ:       true, // dex server instances are typically HQ
		Tunnel: TunnelSettings{
			Enabled:     false,
			IngressAddr: "ingress.enbox.id:9443",
		},
	}
}
