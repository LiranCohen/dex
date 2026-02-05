package mesh

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/WebP2P/dexnet/client/local"
	"github.com/WebP2P/dexnet/tsnet"
)

// Client wraps the tsnet mesh client for HQ integration.
type Client struct {
	mu     sync.RWMutex
	server *tsnet.Server
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
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.server = &tsnet.Server{
		Hostname:   c.config.Hostname,
		Dir:        c.config.StateDir,
		ControlURL: c.config.ControlURL,
		AuthKey:    c.config.AuthKey,
		Logf:       c.logf,
	}

	if err := c.server.Start(); err != nil {
		return fmt.Errorf("mesh start failed: %w", err)
	}

	c.logf("mesh: connected to %s, waiting for IP...", c.config.ControlURL)

	// Wait for mesh IP assignment in background
	go c.waitForIP(ctx)

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

	if c.server != nil {
		c.logf("mesh: shutting down")
		err := c.server.Close()
		c.server = nil
		return err
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

	if c.server == nil {
		return &Status{
			Connected: false,
			IsHQ:      c.config.IsHQ,
		}
	}

	status := &Status{
		Connected: true,
		Hostname:  c.config.Hostname,
		IsHQ:      c.config.IsHQ,
		Online:    true,
	}

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
