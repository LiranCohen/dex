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
	// There should be exactly one HQ per Campus network.
	IsHQ bool `yaml:"is_hq"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:    false,
		StateDir:   "/var/lib/dex/mesh",
		ControlURL: "https://central.enbox.id",
		IsHQ:       true, // dex server instances are typically HQ
	}
}
