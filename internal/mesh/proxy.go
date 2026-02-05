package mesh

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// ServiceProxy manages mesh listeners that reverse-proxy to local services.
type ServiceProxy struct {
	mu        sync.Mutex
	client    *Client
	listeners []net.Listener
	servers   []*http.Server
	logf      func(format string, args ...any)
}

// NewServiceProxy creates a new proxy manager for exposing local services on the mesh.
func NewServiceProxy(client *Client) *ServiceProxy {
	return &ServiceProxy{
		client: client,
		logf:   func(format string, args ...any) { fmt.Printf(format, args...) },
	}
}

// SetLogf sets a custom logging function.
func (sp *ServiceProxy) SetLogf(logf func(format string, args ...any)) {
	sp.logf = logf
}

// Expose creates a mesh listener on meshPort and reverse-proxies all traffic
// to the given local target URL (e.g., "http://127.0.0.1:3000").
// The name is used for logging only.
func (sp *ServiceProxy) Expose(name string, meshPort int, targetURL string) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("parse target URL %q: %w", targetURL, err)
	}

	ln, err := sp.client.Listen("tcp", fmt.Sprintf(":%d", meshPort))
	if err != nil {
		return fmt.Errorf("mesh listen on :%d for %s: %w", meshPort, name, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		sp.logf("mesh proxy %s: %v\n", name, err)
		w.WriteHeader(http.StatusBadGateway)
	}

	srv := &http.Server{Handler: proxy}

	sp.mu.Lock()
	sp.listeners = append(sp.listeners, ln)
	sp.servers = append(sp.servers, srv)
	sp.mu.Unlock()

	go func() {
		sp.logf("mesh: exposing %s on mesh port %d → %s\n", name, meshPort, targetURL)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			sp.logf("mesh proxy %s stopped: %v\n", name, err)
		}
	}()

	return nil
}

// Stop gracefully shuts down all proxy servers and closes listeners.
func (sp *ServiceProxy) Stop() {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	for i, srv := range sp.servers {
		if err := srv.Close(); err != nil {
			sp.logf("mesh proxy stop [%d]: %v\n", i, err)
		}
	}
	sp.servers = nil
	sp.listeners = nil
}
