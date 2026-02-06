package certs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/challenge"
)

// Ensure DexDNSProvider implements the lego Provider interface.
var _ challenge.Provider = (*DexDNSProvider)(nil)

// DexDNSProvider implements lego's dns01.Provider interface.
// It communicates with Dex Central to set/delete DNS TXT records.
type DexDNSProvider struct {
	coordURL   string // Central coordination server URL
	apiToken   string // API token for authentication
	baseDomain string // e.g., "enbox.id"
	client     *http.Client
}

// DexDNSProviderConfig holds configuration for the Dex DNS provider.
type DexDNSProviderConfig struct {
	CoordURL   string // Central coordination server URL
	APIToken   string // API token for authentication with Central
	BaseDomain string // Base domain (default: "enbox.id")
}

// NewDexDNSProvider creates a new DNS provider that uses Dex Central.
func NewDexDNSProvider(cfg DexDNSProviderConfig) *DexDNSProvider {
	baseDomain := cfg.BaseDomain
	if baseDomain == "" {
		baseDomain = "enbox.id"
	}

	return &DexDNSProvider{
		coordURL:   cfg.CoordURL,
		apiToken:   cfg.APIToken,
		baseDomain: baseDomain,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Present creates the DNS TXT record for the ACME challenge.
// domain is in the form "_acme-challenge.myapp.alice.enbox.id"
func (d *DexDNSProvider) Present(domain, token, keyAuth string) error {
	// Extract hostname and namespace from the challenge domain
	// domain: _acme-challenge.myapp.alice.enbox.id
	// We need: hostname=myapp, namespace=alice
	hostname, namespace, ok := d.parseChallengeDomain(domain)
	if !ok {
		return fmt.Errorf("invalid challenge domain: %s", domain)
	}

	payload := map[string]interface{}{
		"hostname":  hostname,
		"namespace": namespace,
		"token":     keyAuth, // This is what Let's Encrypt expects in the TXT record
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST",
		d.coordURL+"/api/v1/dns/acme-challenge",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("set challenge failed: status %d", resp.StatusCode)
	}

	return nil
}

// CleanUp removes the DNS TXT record after the challenge.
func (d *DexDNSProvider) CleanUp(domain, token, keyAuth string) error {
	hostname, namespace, ok := d.parseChallengeDomain(domain)
	if !ok {
		// Best effort cleanup
		return nil
	}

	payload := map[string]interface{}{
		"hostname":  hostname,
		"namespace": namespace,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil // Best effort
	}

	req, err := http.NewRequest("DELETE",
		d.coordURL+"/api/v1/dns/acme-challenge",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil // Best effort
	}

	req.Header.Set("Content-Type", "application/json")
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil // Best effort
	}
	_ = resp.Body.Close()

	return nil
}

// Timeout returns the timeout and check interval for DNS propagation.
func (d *DexDNSProvider) Timeout() (timeout, interval time.Duration) {
	// Wait up to 3 minutes for DNS propagation, checking every 10 seconds
	return 3 * time.Minute, 10 * time.Second
}

// parseChallengeDomain extracts hostname and namespace from an ACME challenge domain.
// Input: _acme-challenge.myapp.alice.enbox.id
// Output: myapp, alice, true
func (d *DexDNSProvider) parseChallengeDomain(domain string) (hostname, namespace string, ok bool) {
	// Remove _acme-challenge. prefix
	const prefix = "_acme-challenge."
	if !strings.HasPrefix(domain, prefix) {
		return "", "", false
	}
	fullHostname := strings.TrimPrefix(domain, prefix)

	// Remove base domain suffix
	suffix := "." + d.baseDomain
	if !strings.HasSuffix(fullHostname, suffix) {
		return "", "", false
	}
	withoutBase := strings.TrimSuffix(fullHostname, suffix)

	// Split into hostname.namespace
	parts := strings.SplitN(withoutBase, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	return parts[0], parts[1], true
}
