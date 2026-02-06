package certs

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

const (
	// RenewalThreshold is how long before expiry to renew certificates.
	RenewalThreshold = 30 * 24 * time.Hour // 30 days

	// RenewalCheckInterval is how often to check for certificates needing renewal.
	RenewalCheckInterval = 12 * time.Hour
)

// Manager handles ACME certificate issuance and renewal for HQ.
type Manager struct {
	stateDir    string
	coordURL    string
	apiToken    string
	email       string
	baseDomain  string
	staging     bool
	store       *Store
	client      *lego.Client
	dnsProvider *DexDNSProvider
	logf        func(format string, args ...any)

	mu          sync.Mutex
	initialized bool
	stopCh      chan struct{}
	running     bool
}

// Config holds configuration for the certificate manager.
type Config struct {
	StateDir   string // Directory to store certificates and account key
	CoordURL   string // Central coordination server URL
	APIToken   string // API token for authentication with Central
	Email      string // ACME account email
	BaseDomain string // Base domain (default: "enbox.id")
	Staging    bool   // Use Let's Encrypt staging environment
	Logf       func(format string, args ...any)
}

// NewManager creates a new certificate manager.
func NewManager(cfg Config) *Manager {
	baseDomain := cfg.BaseDomain
	if baseDomain == "" {
		baseDomain = "enbox.id"
	}

	logf := cfg.Logf
	if logf == nil {
		logf = func(format string, args ...any) {}
	}

	return &Manager{
		stateDir:   cfg.StateDir,
		coordURL:   cfg.CoordURL,
		apiToken:   cfg.APIToken,
		email:      cfg.Email,
		baseDomain: baseDomain,
		staging:    cfg.Staging,
		store:      NewStore(),
		logf:       logf,
		stopCh:     make(chan struct{}),
	}
}

// Store returns the certificate store for use with TLS.
func (m *Manager) Store() *Store {
	return m.store
}

// Start initializes the manager and starts the renewal loop.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.mu.Unlock()

	// Initialize lazily when first cert is requested
	go m.renewalLoop(ctx)

	return nil
}

// Stop stops the renewal loop.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	close(m.stopCh)
	m.running = false
	return nil
}

// ObtainCert gets or renews a certificate for the hostname.
// fullHostname should be the complete hostname (e.g., "myapp.alice.enbox.id").
func (m *Manager) ObtainCert(fullHostname string) (*tls.Certificate, error) {
	// Check for existing valid certificate in store
	if m.store.HasCertificate(fullHostname) {
		cert, _ := m.store.GetCertificate(&tls.ClientHelloInfo{ServerName: fullHostname})
		if cert != nil && !m.shouldRenew(cert) {
			return cert, nil
		}
	}

	// Check for existing valid certificate on disk
	certPath := m.certPath(fullHostname)
	keyPath := m.keyPath(fullHostname)

	if cert, err := m.loadCert(certPath, keyPath); err == nil {
		if !m.shouldRenew(cert) {
			// Certificate is still valid
			m.store.SetCertificate(fullHostname, cert)
			return cert, nil
		}
		m.logf("certs: certificate for %s expiring soon, renewing", fullHostname)
	}

	// Need to obtain/renew certificate
	cert, err := m.obtainNewCert(fullHostname)
	if err != nil {
		return nil, fmt.Errorf("obtain certificate for %s: %w", fullHostname, err)
	}

	// Save certificate
	if err := m.saveCert(cert, certPath, keyPath); err != nil {
		m.logf("certs: failed to save certificate for %s: %v", fullHostname, err)
	}

	// Update store
	m.store.SetCertificate(fullHostname, cert)

	return cert, nil
}

// ObtainCertForEndpoint gets or renews a certificate for a short hostname and namespace.
// Example: ObtainCertForEndpoint("myapp", "alice") → certificate for myapp.alice.enbox.id
func (m *Manager) ObtainCertForEndpoint(hostname, namespace string) (*tls.Certificate, error) {
	fullHostname := hostname + "." + namespace + "." + m.baseDomain
	return m.ObtainCert(fullHostname)
}

// Initialize sets up the ACME client. Called lazily on first use.
func (m *Manager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return nil
	}

	// Create DNS provider
	m.dnsProvider = NewDexDNSProvider(DexDNSProviderConfig{
		CoordURL:   m.coordURL,
		APIToken:   m.apiToken,
		BaseDomain: m.baseDomain,
	})

	// Get or create account key
	accountKey, err := m.getOrCreateAccountKey()
	if err != nil {
		return fmt.Errorf("account key: %w", err)
	}

	user := &acmeUser{
		email: m.email,
		key:   accountKey,
	}

	// Create ACME client config
	config := lego.NewConfig(user)
	if m.staging {
		config.CADirURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	} else {
		config.CADirURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	config.Certificate.KeyType = certcrypto.EC256

	// Create client
	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("create ACME client: %w", err)
	}

	// Register if needed
	reg, err := client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return fmt.Errorf("ACME registration: %w", err)
	}
	user.Registration = reg

	// Configure DNS-01 challenge
	err = client.Challenge.SetDNS01Provider(m.dnsProvider,
		dns01.AddDNSTimeout(3*time.Minute),
	)
	if err != nil {
		return fmt.Errorf("set DNS provider: %w", err)
	}

	m.client = client
	m.initialized = true

	m.logf("certs: ACME client initialized (staging=%v, email=%s)", m.staging, m.email)

	return nil
}

// renewalLoop periodically checks for certificates needing renewal.
func (m *Manager) renewalLoop(ctx context.Context) {
	ticker := time.NewTicker(RenewalCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkRenewals()
		}
	}
}

// checkRenewals checks all certificates and renews those expiring soon.
func (m *Manager) checkRenewals() {
	hostnames := m.store.Hostnames()

	for _, hostname := range hostnames {
		cert, _ := m.store.GetCertificate(&tls.ClientHelloInfo{ServerName: hostname})
		if cert == nil {
			continue
		}

		if m.shouldRenew(cert) {
			m.logf("certs: renewing certificate for %s", hostname)
			if _, err := m.ObtainCert(hostname); err != nil {
				m.logf("certs: renewal failed for %s: %v", hostname, err)
			}
		}
	}
}

// obtainNewCert requests a new certificate from Let's Encrypt.
func (m *Manager) obtainNewCert(fullHostname string) (*tls.Certificate, error) {
	if err := m.Initialize(); err != nil {
		return nil, err
	}

	m.logf("certs: requesting certificate for %s (staging=%v)", fullHostname, m.staging)

	request := certificate.ObtainRequest{
		Domains: []string{fullHostname},
		Bundle:  true,
	}

	certificates, err := m.client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("obtain cert: %w", err)
	}

	// Parse certificate
	cert, err := tls.X509KeyPair(certificates.Certificate, certificates.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	m.logf("certs: certificate obtained for %s", fullHostname)

	return &cert, nil
}

// loadCert loads a certificate from disk.
func (m *Manager) loadCert(certPath, keyPath string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// saveCert saves a certificate to disk.
func (m *Manager) saveCert(cert *tls.Certificate, certPath, keyPath string) error {
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return err
	}

	// Encode certificate
	var certPEM []byte
	for _, c := range cert.Certificate {
		block := &pem.Block{Type: "CERTIFICATE", Bytes: c}
		certPEM = append(certPEM, pem.EncodeToMemory(block)...)
	}

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return err
	}

	// Encode private key
	keyBytes, err := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		return err
	}
	keyBlock := &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(keyBlock), 0600); err != nil {
		return err
	}

	return nil
}

// shouldRenew checks if a certificate should be renewed.
func (m *Manager) shouldRenew(cert *tls.Certificate) bool {
	if len(cert.Certificate) == 0 {
		return true
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}

	// Renew if less than threshold remaining
	return time.Until(x509Cert.NotAfter) < RenewalThreshold
}

// getOrCreateAccountKey returns the ACME account private key.
func (m *Manager) getOrCreateAccountKey() (*ecdsa.PrivateKey, error) {
	keyPath := filepath.Join(m.stateDir, "acme-account.key")

	// Try to load existing key
	if keyData, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(keyData)
		if block != nil {
			if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return key, nil
			}
		}
	}

	// Generate new key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Save key
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		return nil, err
	}

	m.logf("certs: created new ACME account key at %s", keyPath)

	return key, nil
}

// certPath returns the path for a certificate file.
func (m *Manager) certPath(hostname string) string {
	return filepath.Join(m.stateDir, "certs", hostname+".crt")
}

// keyPath returns the path for a key file.
func (m *Manager) keyPath(hostname string) string {
	return filepath.Join(m.stateDir, "certs", hostname+".key")
}

// acmeUser implements lego's registration.User interface.
type acmeUser struct {
	email        string
	key          *ecdsa.PrivateKey
	Registration *registration.Resource
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }
