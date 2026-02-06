package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_NewManager(t *testing.T) {
	mgr := NewManager(Config{
		StateDir:   "/tmp/test",
		CoordURL:   "https://central.example.com",
		APIToken:   "test-token",
		Email:      "test@example.com",
		BaseDomain: "example.com",
		Staging:    true,
	})

	assert.NotNil(t, mgr)
	assert.NotNil(t, mgr.store)
	assert.Equal(t, "example.com", mgr.baseDomain)
	assert.True(t, mgr.staging)
}

func TestManager_DefaultBaseDomain(t *testing.T) {
	mgr := NewManager(Config{
		StateDir: "/tmp/test",
		CoordURL: "https://central.example.com",
	})

	assert.Equal(t, "enbox.id", mgr.baseDomain)
}

func TestManager_Store(t *testing.T) {
	mgr := NewManager(Config{
		StateDir: "/tmp/test",
	})

	store := mgr.Store()
	assert.NotNil(t, store)
	assert.Equal(t, mgr.store, store)
}

func TestManager_ShouldRenew(t *testing.T) {
	mgr := NewManager(Config{})

	tests := []struct {
		name     string
		notAfter time.Time
		want     bool
	}{
		{
			name:     "expired certificate",
			notAfter: time.Now().Add(-24 * time.Hour),
			want:     true,
		},
		{
			name:     "expiring soon",
			notAfter: time.Now().Add(7 * 24 * time.Hour), // 7 days
			want:     true,
		},
		{
			name:     "expiring in 60 days",
			notAfter: time.Now().Add(60 * 24 * time.Hour),
			want:     false,
		},
		{
			name:     "expiring in exactly 30 days",
			notAfter: time.Now().Add(30 * 24 * time.Hour),
			want:     true, // threshold is < 30 days, so exactly 30 days is borderline
		},
		{
			name:     "expiring in 31 days",
			notAfter: time.Now().Add(31 * 24 * time.Hour),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := createTestCertWithExpiry(t, "test.example.com", tt.notAfter)
			got := mgr.shouldRenew(cert)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManager_ShouldRenewEmptyCert(t *testing.T) {
	mgr := NewManager(Config{})

	cert := &tls.Certificate{Certificate: [][]byte{}}
	assert.True(t, mgr.shouldRenew(cert))
}

func TestManager_LoadSaveCert(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "certs-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(Config{
		StateDir: tmpDir,
	})

	// Create a test certificate
	cert := createTestCertWithExpiry(t, "test.example.com", time.Now().Add(90*24*time.Hour))

	certPath := filepath.Join(tmpDir, "test.crt")
	keyPath := filepath.Join(tmpDir, "test.key")

	// Save certificate
	err = mgr.saveCert(cert, certPath, keyPath)
	require.NoError(t, err)

	// Verify files exist
	_, err = os.Stat(certPath)
	require.NoError(t, err)
	_, err = os.Stat(keyPath)
	require.NoError(t, err)

	// Load certificate
	loaded, err := mgr.loadCert(certPath, keyPath)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, len(cert.Certificate), len(loaded.Certificate))
}

func TestManager_GetOrCreateAccountKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "certs-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mgr := NewManager(Config{
		StateDir: tmpDir,
	})

	// First call should create new key
	key1, err := mgr.getOrCreateAccountKey()
	require.NoError(t, err)
	assert.NotNil(t, key1)

	// Verify key file exists
	keyPath := filepath.Join(tmpDir, "acme-account.key")
	_, err = os.Stat(keyPath)
	require.NoError(t, err)

	// Second call should load existing key
	key2, err := mgr.getOrCreateAccountKey()
	require.NoError(t, err)
	assert.NotNil(t, key2)

	// Keys should be the same
	assert.Equal(t, key1.D, key2.D)
}

func TestManager_CertPaths(t *testing.T) {
	mgr := NewManager(Config{
		StateDir: "/var/lib/dex/mesh",
	})

	certPath := mgr.certPath("test.example.com")
	keyPath := mgr.keyPath("test.example.com")

	assert.Equal(t, "/var/lib/dex/mesh/certs/test.example.com.crt", certPath)
	assert.Equal(t, "/var/lib/dex/mesh/certs/test.example.com.key", keyPath)
}

func TestManager_StopBeforeStart(t *testing.T) {
	mgr := NewManager(Config{})

	// Should not error if Stop called before Start
	err := mgr.Stop()
	assert.NoError(t, err)
}

// createTestCertWithExpiry creates a self-signed certificate with specific expiry.
func createTestCertWithExpiry(t *testing.T, hostname string, notAfter time.Time) *tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: hostname,
		},
		DNSNames:              []string{hostname},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	// Encode for proper parsing
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return &cert
}
