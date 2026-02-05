package mesh

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"
)

func TestServiceProxy_ReverseProxy(t *testing.T) {
	// Create a backend HTTP server to proxy to
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "test")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello from backend")
	}))
	defer backend.Close()

	// Create a ServiceProxy with empty client (we'll wire the listener manually)
	sp := &ServiceProxy{
		logf: func(format string, args ...any) { t.Logf(format, args...) },
	}

	// Create a listener manually (simulating what mesh.Listen would return)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	proxyAddr := ln.Addr().String()

	// Create reverse proxy to backend (same logic as Expose)
	target, _ := url.Parse(backend.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	srv := &http.Server{Handler: proxy}

	sp.listeners = append(sp.listeners, ln)
	sp.servers = append(sp.servers, srv)

	go func() {
		_ = srv.Serve(ln)
	}()

	// Give the proxy a moment to start serving
	time.Sleep(50 * time.Millisecond)

	// Make a request through the proxy
	resp, err := http.Get("http://" + proxyAddr + "/test")
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from backend" {
		t.Errorf("body = %q, want %q", string(body), "hello from backend")
	}

	if resp.Header.Get("X-Backend") != "test" {
		t.Errorf("X-Backend header = %q, want %q", resp.Header.Get("X-Backend"), "test")
	}

	// Test Stop shuts down cleanly
	sp.Stop()

	// Verify listener is closed (connection should fail)
	time.Sleep(50 * time.Millisecond)
	_, err = net.DialTimeout("tcp", proxyAddr, 100*time.Millisecond)
	if err == nil {
		t.Error("expected connection refused after Stop(), but dial succeeded")
	}
}

func TestServiceProxy_Stop_Empty(t *testing.T) {
	sp := &ServiceProxy{
		logf: func(format string, args ...any) {},
	}
	// Should not panic on empty proxy
	sp.Stop()
}
