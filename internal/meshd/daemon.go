// Package meshd implements the dex mesh daemon, which runs a full
// WireGuard engine with TUN device, OS routes, and DNS configuration.
// This enables browsers and other OS-level applications to reach mesh
// nodes directly, unlike tsnet which only works within the process.
package meshd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/WebP2P/dexnet/control/controlclient"
	"github.com/WebP2P/dexnet/ipn"
	"github.com/WebP2P/dexnet/ipn/ipnlocal"
	"github.com/WebP2P/dexnet/ipn/ipnserver"
	"github.com/WebP2P/dexnet/ipn/store"
	"github.com/WebP2P/dexnet/ipn/store/mem"
	"github.com/WebP2P/dexnet/net/dns"
	"github.com/WebP2P/dexnet/net/netmon"
	"github.com/WebP2P/dexnet/net/netns"
	"github.com/WebP2P/dexnet/net/tsdial"
	"github.com/WebP2P/dexnet/net/tstun"
	"github.com/WebP2P/dexnet/safesocket"
	"github.com/WebP2P/dexnet/tsd"
	"github.com/WebP2P/dexnet/types/logger"
	"github.com/WebP2P/dexnet/types/logid"
	"github.com/WebP2P/dexnet/wgengine"
	"github.com/WebP2P/dexnet/wgengine/netstack"
	"github.com/WebP2P/dexnet/wgengine/router"

	// Register OS-specific router implementations (e.g., userspaceBSDRouter
	// on macOS/FreeBSD) so that router.New() works with real TUN devices.
	_ "github.com/WebP2P/dexnet/wgengine/router/osrouter"
)

const (
	// SocketPath is where the daemon listens for LocalAPI connections.
	SocketPath = "/var/run/dex-meshd.sock"

	// StatePath is the default state file location.
	StatePath = "/var/lib/dex/meshd.state"

	// StateDir is the default state directory.
	StateDir = "/var/lib/dex"
)

// Config holds daemon configuration.
type Config struct {
	// SocketPath is the LocalAPI socket path.
	SocketPath string

	// StatePath is the path to the persistent state file.
	StatePath string

	// StateDir is the directory for state files.
	StateDir string

	// TunName is the TUN device name ("utun" on macOS for auto-assignment).
	TunName string

	// ControlURL is the mesh control server URL (e.g., "https://central.enbox.id").
	// Required — the daemon needs this to connect to the correct control server.
	ControlURL string

	// Hostname is this node's hostname on the mesh network.
	Hostname string

	// Verbose sets the log verbosity level.
	Verbose int

	// Logf is the logging function. Defaults to log.Printf.
	Logf logger.Logf
}

// DefaultConfig returns a Config with platform-appropriate defaults.
func DefaultConfig() Config {
	tunName := "dex0"
	if runtime.GOOS == "darwin" {
		tunName = "utun"
	}
	return Config{
		SocketPath: SocketPath,
		StatePath:  StatePath,
		StateDir:   StateDir,
		TunName:    tunName,
		Logf:       log.Printf,
	}
}

// Run starts the mesh daemon and blocks until the context is cancelled
// or a signal is received.
func Run(ctx context.Context, cfg Config) error {
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}

	// Require root on macOS (TUN creation needs it)
	if runtime.GOOS == "darwin" && os.Getuid() != 0 {
		return fmt.Errorf("dex meshd requires root on macOS; use sudo or install as a LaunchDaemon")
	}

	// Ensure state directory exists
	if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	// Set up the system first — this creates the event bus that
	// netmon and other subsystems need.
	sys := tsd.NewSystem()
	sys.SocketPath = cfg.SocketPath

	// Create network monitor with the bus from the system
	nMon, err := netmon.New(sys.Bus.Get(), logf)
	if err != nil {
		return fmt.Errorf("netmon.New: %w", err)
	}
	sys.Set(nMon)

	// Clean up stale DNS/route config from previous crashes
	dns.CleanUp(logf, nMon, sys.Bus.Get(), sys.HealthTracker.Get(), cfg.TunName)
	router.CleanUp(logf, nMon, cfg.TunName)

	// Listen on the LocalAPI socket
	ln, err := safesocket.Listen(cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("safesocket.Listen(%s): %w", cfg.SocketPath, err)
	}
	logf("dex-meshd: listening on %s", cfg.SocketPath)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle signals for graceful shutdown
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)

	wgEngineCreated := make(chan struct{})
	go func() {
		var wgEngineClosed <-chan struct{}
		wgEngineCreatedLocal := wgEngineCreated
		for {
			select {
			case s := <-interrupt:
				logf("dex-meshd: got signal %v; shutting down", s)
				cancel()
				return
			case <-wgEngineClosed:
				logf("dex-meshd: wgengine closed; shutting down")
				cancel()
				return
			case <-wgEngineCreatedLocal:
				wgEngineClosed = sys.Engine.Get().Done()
				wgEngineCreatedLocal = nil
			case <-ctx.Done():
				return
			}
		}
	}()

	// Create the IPN server (serves LocalAPI)
	var logID logid.PublicID // zero = no logtail
	srv := ipnserver.New(logf, logID, sys.Bus.Get(), sys.NetMon.Get())

	// Start the backend in background
	go func() {
		lb, err := createBackend(ctx, logf, logID, sys, cfg)
		if err != nil {
			logf("dex-meshd: backend creation failed: %v", err)
			cancel()
			return
		}
		logf("dex-meshd: backend ready")

		// Set up prefs like tsnet does — explicitly pass ControlURL,
		// Hostname, and WantRunning so the backend connects to the
		// correct control server using the existing enrollment state.
		prefs := ipn.NewPrefs()
		prefs.WantRunning = true
		prefs.ControlURL = cfg.ControlURL
		prefs.Hostname = cfg.Hostname

		if err := lb.Start(ipn.Options{
			UpdatePrefs: prefs,
		}); err != nil {
			logf("dex-meshd: LocalBackend.Start: %v", err)
			lb.Shutdown()
			cancel()
			return
		}

		srv.SetLocalBackend(lb)
		close(wgEngineCreated)
	}()

	// Block on the IPN server (serves LocalAPI requests)
	err = srv.Run(ctx, ln)
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("ipnserver.Run: %w", err)
	}

	logf("dex-meshd: stopped")
	return nil
}

// createBackend creates the WireGuard engine, TUN device, router, DNS,
// and LocalBackend — the core of the mesh daemon.
func createBackend(ctx context.Context, logf logger.Logf, logID logid.PublicID, sys *tsd.System, cfg Config) (*ipnlocal.LocalBackend, error) {
	dialer := &tsdial.Dialer{Logf: logf}
	dialer.SetBus(sys.Bus.Get())
	sys.Set(dialer)

	// Create the TUN device
	logf("dex-meshd: creating TUN device %q", cfg.TunName)
	onlyNetstack := cfg.TunName == "userspace-networking"
	netns.SetEnabled(!onlyNetstack)

	wgConf := wgengine.Config{
		ListenPort:    0, // auto
		NetMon:        sys.NetMon.Get(),
		HealthTracker: sys.HealthTracker.Get(),
		Metrics:       sys.UserMetricsRegistry(),
		Dialer:        sys.Dialer.Get(),
		SetSubsystem:  sys.Set,
		ControlKnobs:  sys.ControlKnobs(),
		EventBus:      sys.Bus.Get(),
	}

	sys.HealthTracker.Get().SetMetricsRegistry(sys.UserMetricsRegistry())

	if !onlyNetstack {
		dev, devName, err := tstun.New(logf, cfg.TunName)
		if err != nil {
			tstun.Diagnose(logf, cfg.TunName, err)
			return nil, fmt.Errorf("tstun.New(%q): %w", cfg.TunName, err)
		}
		logf("dex-meshd: created TUN device %s", devName)

		r, err := router.New(logf, dev, sys.NetMon.Get(), sys.HealthTracker.Get(), sys.Bus.Get())
		if err != nil {
			_ = dev.Close()
			return nil, fmt.Errorf("router.New: %w", err)
		}

		d, err := dns.NewOSConfigurator(logf, sys.HealthTracker.Get(), sys.Bus.Get(), sys.PolicyClientOrDefault(), sys.ControlKnobs(), devName)
		if err != nil {
			_ = dev.Close()
			_ = r.Close()
			return nil, fmt.Errorf("dns.NewOSConfigurator: %w", err)
		}

		wgConf.Tun = dev
		wgConf.Router = r
		wgConf.DNS = d
		sys.Set(wgConf.Router)
	}

	// Create WireGuard engine
	e, err := wgengine.NewUserspaceEngine(logf, wgConf)
	if err != nil {
		return nil, fmt.Errorf("wgengine.NewUserspaceEngine: %w", err)
	}
	e = wgengine.NewWatchdog(e)
	sys.Set(e)

	// On macOS/FreeBSD, subnet routing goes through netstack
	netstackSubnetRouter := onlyNetstack
	if !onlyNetstack {
		switch runtime.GOOS {
		case "windows", "darwin", "freebsd", "openbsd", "solaris", "illumos":
			netstackSubnetRouter = true
		}
	}
	sys.NetstackRouter.Set(netstackSubnetRouter)

	// Open state store
	st, err := store.New(logf, cfg.StatePath)
	if err != nil {
		logf("dex-meshd: store.New(%s) failed: %v; using in-memory store", cfg.StatePath, err)
		st, _ = mem.New(logf, "")
	}
	sys.Set(st)

	// Start TUN wrapper if available
	if w, ok := sys.Tun.GetOK(); ok {
		w.Start()
	}

	// Initialize netstack (required for subnet routing on macOS)
	ns, err := netstack.Create(logf,
		sys.Tun.Get(),
		sys.Engine.Get(),
		sys.MagicSock.Get(),
		sys.Dialer.Get(),
		sys.DNSManager.Get(),
		sys.ProxyMapper(),
	)
	if err != nil {
		logf("dex-meshd: netstack.Create failed (non-fatal): %v", err)
	} else {
		sys.Set(ns)
		ns.ProcessLocalIPs = onlyNetstack
		ns.ProcessSubnets = netstackSubnetRouter
	}

	// Create LocalBackend with OS-neutral flag (same as tsnet) so the
	// profile manager reads _current-profile correctly on all platforms.
	lb, err := ipnlocal.NewLocalBackend(logf, logID, sys, controlclient.LocalBackendStartKeyOSNeutral)
	if err != nil {
		return nil, fmt.Errorf("ipnlocal.NewLocalBackend: %w", err)
	}
	lb.SetVarRoot(cfg.StateDir)

	return lb, nil
}

// IsRunning checks if the daemon is running by trying to connect to its socket.
func IsRunning() bool {
	return IsRunningAt(SocketPath)
}

// IsRunningAt checks if a daemon is running at the given socket path.
func IsRunningAt(socketPath string) bool {
	c, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// WaitForSocket waits up to timeout for the daemon socket to become available.
func WaitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsRunningAt(socketPath) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon socket %s not available after %v", socketPath, timeout)
}

// SocketDir returns the directory portion of the socket path.
func SocketDir() string {
	return filepath.Dir(SocketPath)
}
