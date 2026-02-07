package certs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDexDNSProvider_Present(t *testing.T) {
	var receivedRequest map[string]any
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/dns/acme-challenge" && r.Method == "POST" {
			receivedAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedRequest)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := NewDexDNSProvider(DexDNSProviderConfig{
		CoordURL:   server.URL,
		APIToken:   "test-token",
		BaseDomain: "enbox.id",
	})

	// Present a challenge - domain is the domain being certified, NOT the challenge domain
	keyAuth := "keyAuth123"
	err := provider.Present(
		"myapp.alice.enbox.id", // domain being certified
		"token",
		keyAuth,
	)
	require.NoError(t, err)

	// Verify the correct token value (base64url(sha256(keyAuth)))
	expectedHash := sha256.Sum256([]byte(keyAuth))
	expectedToken := base64.RawURLEncoding.EncodeToString(expectedHash[:])

	assert.Equal(t, "Bearer test-token", receivedAuth)
	assert.Equal(t, "myapp", receivedRequest["hostname"])
	assert.Equal(t, "alice", receivedRequest["namespace"])
	assert.Equal(t, expectedToken, receivedRequest["token"])
}

func TestDexDNSProvider_CleanUp(t *testing.T) {
	var receivedRequest map[string]any
	var receivedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/dns/acme-challenge" && r.Method == "DELETE" {
			receivedMethod = r.Method
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedRequest)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := NewDexDNSProvider(DexDNSProviderConfig{
		CoordURL:   server.URL,
		APIToken:   "test-token",
		BaseDomain: "enbox.id",
	})

	// Clean up a challenge - domain is the domain being certified
	err := provider.CleanUp(
		"myapp.alice.enbox.id",
		"token",
		"keyAuth123",
	)
	require.NoError(t, err)

	assert.Equal(t, "DELETE", receivedMethod)
	assert.Equal(t, "myapp", receivedRequest["hostname"])
	assert.Equal(t, "alice", receivedRequest["namespace"])
}

func TestDexDNSProvider_ParseDomain(t *testing.T) {
	provider := NewDexDNSProvider(DexDNSProviderConfig{
		BaseDomain: "enbox.id",
	})

	tests := []struct {
		name         string
		domain       string
		wantHostname string
		wantNS       string
		wantOK       bool
	}{
		{
			name:         "hostname and namespace",
			domain:       "myapp.alice.enbox.id",
			wantHostname: "myapp",
			wantNS:       "alice",
			wantOK:       true,
		},
		{
			name:         "namespace only",
			domain:       "alice.enbox.id",
			wantHostname: "",
			wantNS:       "alice",
			wantOK:       true,
		},
		{
			name:         "multiple hostname parts",
			domain:       "api.company.enbox.id",
			wantHostname: "api",
			wantNS:       "company",
			wantOK:       true,
		},
		{
			name:   "wrong base domain",
			domain: "myapp.alice.example.com",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostname, namespace, ok := provider.parseDomain(tt.domain)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantHostname, hostname)
				assert.Equal(t, tt.wantNS, namespace)
			}
		})
	}
}

func TestDexDNSProvider_Timeout(t *testing.T) {
	provider := NewDexDNSProvider(DexDNSProviderConfig{})

	timeout, interval := provider.Timeout()

	// Should have reasonable timeout and interval
	assert.True(t, timeout > 0, "timeout should be positive")
	assert.True(t, interval > 0, "interval should be positive")
	assert.True(t, timeout > interval, "timeout should be greater than interval")
}

func TestDexDNSProvider_PresentError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	provider := NewDexDNSProvider(DexDNSProviderConfig{
		CoordURL:   server.URL,
		BaseDomain: "enbox.id",
	})

	err := provider.Present(
		"myapp.alice.enbox.id",
		"token",
		"keyAuth123",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestDexDNSProvider_PresentInvalidDomain(t *testing.T) {
	provider := NewDexDNSProvider(DexDNSProviderConfig{
		CoordURL:   "http://localhost",
		BaseDomain: "enbox.id",
	})

	err := provider.Present(
		"invalid-domain.example.com",
		"token",
		"keyAuth123",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid domain")
}
