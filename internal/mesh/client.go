package mesh

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/WebP2P/dexnet/client/local"
	"github.com/WebP2P/dexnet/tsnet"
)

// Client wraps the tsnet mesh client for HQ integration.
type Client struct {
	mu     sync.RWMutex
	server *tsnet.Server
	tunnel *TunnelClient
	config Config
	logf   func(format string, args ...any)
}

// NewClient creates a new mesh client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		config: cfg,
		logf:   log.Printf,
	}
}

// SetLogf sets a custom logging function.
func (c *Client) SetLogf(logf func(format string, args ...any)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf = logf
}

// Start initializes and starts the mesh client.
// It connects to Central and joins the Campus network.
func (c *Client) Start(ctx context.Context) error {
	if !c.config.Enabled {
		c.logf("mesh: networking disabled")
		// Even if mesh is disabled, we can still use tunnel
		return c.startTunnel(ctx)
	}

	c.mu.Lock()

	c.server = &tsnet.Server{
		Hostname:   c.config.Hostname,
		Dir:        c.config.StateDir,
		ControlURL: c.config.ControlURL,
		AuthKey:    c.config.AuthKey,
		Logf:       c.logf,
	}

	if err := c.server.Start(); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("mesh start failed: %w", err)
	}

	c.logf("mesh: connected to %s, waiting for IP...", c.config.ControlURL)
	c.mu.Unlock()

	// Wait for mesh IP assignment in background
	go c.waitForIP(ctx)

	// Start tunnel if configured
	return c.startTunnel(ctx)
}

// startTunnel starts the tunnel client if configured.
func (c *Client) startTunnel(ctx context.Context) error {
	if !c.config.Tunnel.Enabled {
		return nil
	}

	if c.config.Tunnel.IngressAddr == "" {
		c.logf("tunnel: ingress address not configured")
		return nil
	}

	if len(c.config.Tunnel.Endpoints) == 0 {
		c.logf("tunnel: no endpoints configured")
		return nil
	}

	// Convert endpoint mappings to tunnel endpoints
	endpoints := make([]TunnelEndpoint, len(c.config.Tunnel.Endpoints))
	for i, ep := range c.config.Tunnel.Endpoints {
		endpoints[i] = TunnelEndpoint{
			Hostname:  ep.Hostname,
			LocalPort: ep.LocalPort,
		}
	}

	// Prepare ACME config if enabled
	var acmeSettings *ACMESettings
	if c.config.Tunnel.ACME.Enabled {
		acmeSettings = &c.config.Tunnel.ACME
	}

	c.mu.Lock()
	c.tunnel = NewTunnelClient(TunnelConfig{
		IngressAddr: c.config.Tunnel.IngressAddr,
		Token:       c.config.Tunnel.Token,
		Endpoints:   endpoints,
		Logf:        c.logf,
		ACME:        acmeSettings,
		CoordURL:    c.config.ControlURL,
		APIToken:    c.config.Tunnel.Token, // Use tunnel token for DNS API auth
		StateDir:    c.config.StateDir,
		BaseDomain:  "enbox.id", // TODO: make configurable
	})
	c.mu.Unlock()

	if err := c.tunnel.Start(ctx); err != nil {
		return fmt.Errorf("tunnel start failed: %w", err)
	}

	c.logf("tunnel: started with %d endpoints", len(endpoints))
	return nil
}

// waitForIP waits for the mesh IP to be assigned and logs it.
func (c *Client) waitForIP(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			c.logf("mesh: timeout waiting for IP assignment")
			return
		case <-ticker.C:
			c.mu.RLock()
			if c.server == nil {
				c.mu.RUnlock()
				return
			}
			ip4, _ := c.server.TailscaleIPs()
			c.mu.RUnlock()

			if ip4.IsValid() {
				c.logf("mesh: IP assigned: %s", ip4)
				return
			}
		}
	}
}

// Stop gracefully shuts down the mesh client.
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	// Stop tunnel first
	if c.tunnel != nil {
		c.logf("tunnel: shutting down")
		if err := c.tunnel.Stop(); err != nil {
			errs = append(errs, err)
		}
		c.tunnel = nil
	}

	// Stop mesh
	if c.server != nil {
		c.logf("mesh: shutting down")
		if err := c.server.Close(); err != nil {
			errs = append(errs, err)
		}
		c.server = nil
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// IsRunning returns true if the mesh client is running.
func (c *Client) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.server != nil
}

// Status returns the current mesh connection status.
func (c *Client) Status() *Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := &Status{
		IsHQ: c.config.IsHQ,
	}

	// Mesh status
	if c.server != nil {
		status.Connected = true
		status.Hostname = c.config.Hostname
		status.Online = true

		// Get mesh IP
		ip4, _ := c.server.TailscaleIPs()
		if ip4.IsValid() {
			status.MeshIP = ip4.String()
		}

		// Get additional status info via LocalClient
		lc, err := c.server.LocalClient()
		if err == nil {
			ctx := context.Background()
			if s, err := lc.StatusWithoutPeers(ctx); err == nil && s != nil && s.Self != nil {
				// Get DERP relay info from Self peer status
				if s.Self.Relay != "" {
					// Relay is a string like "nyc" or "1" representing the DERP region
					status.DERPRegion = 0 // We don't have numeric region here
				}
			}
		}
	}

	// Tunnel status
	if c.tunnel != nil {
		status.TunnelConnected = c.tunnel.IsConnected()
		status.TunnelEndpoints = len(c.config.Tunnel.Endpoints)
	}

	return status
}

// Peers returns the list of peers on the Campus network.
func (c *Client) Peers() []Peer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.server == nil {
		return nil
	}

	lc, err := c.server.LocalClient()
	if err != nil {
		return nil
	}

	ctx := context.Background()
	status, err := lc.Status(ctx)
	if err != nil || status == nil {
		return nil
	}

	peers := make([]Peer, 0, len(status.Peer))
	for _, p := range status.Peer {
		peer := Peer{
			Hostname: p.HostName,
			Online:   p.Online,
			Direct:   p.CurAddr != "",
		}

		if len(p.TailscaleIPs) > 0 {
			peer.MeshIP = p.TailscaleIPs[0].String()
		}

		if !p.LastSeen.IsZero() {
			peer.LastSeen = p.LastSeen.Format(time.RFC3339)
		}

		// Copy tags from views.Slice
		if p.Tags != nil && p.Tags.Len() > 0 {
			peer.Tags = make([]string, p.Tags.Len())
			for i := range p.Tags.Len() {
				peer.Tags[i] = p.Tags.At(i)
			}
		}

		peers = append(peers, peer)
	}

	return peers
}

// Dial connects to an address over the mesh network.
// The network parameter should be "tcp" or "udp".
// The address should be in the form "host:port" where host can be
// a mesh IP or a MagicDNS hostname.
func (c *Client) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.server == nil {
		return nil, fmt.Errorf("mesh not connected")
	}

	return c.server.Dial(ctx, network, addr)
}

// Listen creates a listener on the mesh network.
// The network parameter should be "tcp" or "udp".
// The address should be in the form ":port" for all interfaces
// or "host:port" for a specific interface.
func (c *Client) Listen(network, addr string) (net.Listener, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.server == nil {
		return nil, fmt.Errorf("mesh not connected")
	}

	return c.server.Listen(network, addr)
}

// LocalClient returns the local client for advanced operations.
// This provides access to the underlying tsnet client for
// operations not exposed by the Client wrapper.
func (c *Client) LocalClient() (*local.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.server == nil {
		return nil, fmt.Errorf("mesh not connected")
	}

	return c.server.LocalClient()
}

// MeshIP returns the current mesh IP address, or empty string if not connected.
func (c *Client) MeshIP() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.server == nil {
		return ""
	}

	ip4, _ := c.server.TailscaleIPs()
	if ip4.IsValid() {
		return ip4.String()
	}

	return ""
}

// RegisterOnce performs a one-shot mesh registration using the provided auth key.
// It starts tsnet, waits for successful registration (IP assignment), then stops.
// The mesh state is saved to stateDir for future connections without auth key.
// This is used during enrollment to consume the auth key immediately.
func RegisterOnce(controlURL, authKey, hostname, stateDir string, logf func(format string, args ...any)) error {
	if logf == nil {
		logf = log.Printf
	}

	if err := ensureDir(stateDir); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}

	server := &tsnet.Server{
		Hostname:   hostname,
		Dir:        stateDir,
		ControlURL: controlURL,
		AuthKey:    authKey,
		Logf:       logf,
	}

	logf("mesh: registering with %s as %s", controlURL, hostname)

	if err := server.Start(); err != nil {
		return fmt.Errorf("mesh registration failed: %w", err)
	}

	// Wait for IP assignment (indicates successful registration)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = server.Close()
			return fmt.Errorf("timeout waiting for mesh registration")
		case <-ticker.C:
			ip4, _ := server.TailscaleIPs()
			if ip4.IsValid() {
				logf("mesh: registered successfully, IP: %s", ip4)
				_ = server.Close()
				return nil
			}
		}
	}
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
