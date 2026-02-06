package certs

import (
	"crypto/tls"
	"sync"
)

// Store provides thread-safe storage for TLS certificates.
// It implements the GetCertificate function for tls.Config.
type Store struct {
	mu    sync.RWMutex
	certs map[string]*tls.Certificate // hostname → certificate
}

// NewStore creates a new certificate store.
func NewStore() *Store {
	return &Store{
		certs: make(map[string]*tls.Certificate),
	}
}

// GetCertificate returns a certificate for the given hostname.
// This is designed to be used with tls.Config.GetCertificate.
func (s *Store) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cert, ok := s.certs[hello.ServerName]; ok {
		return cert, nil
	}

	return nil, nil // Let TLS fall back to default
}

// SetCertificate stores a certificate for a hostname.
func (s *Store) SetCertificate(hostname string, cert *tls.Certificate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.certs[hostname] = cert
}

// HasCertificate checks if a certificate exists for a hostname.
func (s *Store) HasCertificate(hostname string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.certs[hostname]
	return ok
}

// DeleteCertificate removes a certificate for a hostname.
func (s *Store) DeleteCertificate(hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.certs, hostname)
}

// Hostnames returns a list of all hostnames with certificates.
func (s *Store) Hostnames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hostnames := make([]string, 0, len(s.certs))
	for h := range s.certs {
		hostnames = append(hostnames, h)
	}
	return hostnames
}
