package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_SetAndGetCertificate(t *testing.T) {
	store := NewStore()

	cert := createTestCert(t, "test.example.com")

	// Initially no certificate
	hello := &tls.ClientHelloInfo{ServerName: "test.example.com"}
	got, err := store.GetCertificate(hello)
	require.NoError(t, err)
	assert.Nil(t, got)

	// Set certificate
	store.SetCertificate("test.example.com", cert)

	// Now we should get it
	got, err = store.GetCertificate(hello)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, cert, got)
}

func TestStore_HasCertificate(t *testing.T) {
	store := NewStore()

	assert.False(t, store.HasCertificate("test.example.com"))

	cert := createTestCert(t, "test.example.com")
	store.SetCertificate("test.example.com", cert)

	assert.True(t, store.HasCertificate("test.example.com"))
	assert.False(t, store.HasCertificate("other.example.com"))
}

func TestStore_DeleteCertificate(t *testing.T) {
	store := NewStore()

	cert := createTestCert(t, "test.example.com")
	store.SetCertificate("test.example.com", cert)

	assert.True(t, store.HasCertificate("test.example.com"))

	store.DeleteCertificate("test.example.com")

	assert.False(t, store.HasCertificate("test.example.com"))
}

func TestStore_Hostnames(t *testing.T) {
	store := NewStore()

	cert1 := createTestCert(t, "host1.example.com")
	cert2 := createTestCert(t, "host2.example.com")
	cert3 := createTestCert(t, "host3.example.com")

	store.SetCertificate("host1.example.com", cert1)
	store.SetCertificate("host2.example.com", cert2)
	store.SetCertificate("host3.example.com", cert3)

	hostnames := store.Hostnames()
	assert.Len(t, hostnames, 3)
	assert.Contains(t, hostnames, "host1.example.com")
	assert.Contains(t, hostnames, "host2.example.com")
	assert.Contains(t, hostnames, "host3.example.com")
}

func TestStore_GetCertificateUnknownHost(t *testing.T) {
	store := NewStore()

	cert := createTestCert(t, "known.example.com")
	store.SetCertificate("known.example.com", cert)

	// Request unknown host - should return nil (fallback to default)
	hello := &tls.ClientHelloInfo{ServerName: "unknown.example.com"}
	got, err := store.GetCertificate(hello)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_Concurrent(t *testing.T) {
	store := NewStore()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cert := createTestCert(t, "test.example.com")
			store.SetCertificate("test.example.com", cert)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			hello := &tls.ClientHelloInfo{ServerName: "test.example.com"}
			_, _ = store.GetCertificate(hello)
			_ = store.HasCertificate("test.example.com")
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done
}

// createTestCert creates a self-signed certificate for testing.
func createTestCert(t *testing.T, hostname string) *tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: hostname,
		},
		DNSNames:              []string{hostname},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}
