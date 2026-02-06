package mesh

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/lirancohen/dex/internal/certs"
)

const (
	// TunnelKeepaliveInterval is how often keepalive messages are sent.
	TunnelKeepaliveInterval = 30 * time.Second
	// TunnelKeepaliveTimeout is how long to wait for operations.
	TunnelKeepaliveTimeout = 10 * time.Second
	// TunnelHandshakeTimeout is how long to wait for handshake completion.
	TunnelHandshakeTimeout = 10 * time.Second
	// TunnelReconnectDelay is how long to wait before reconnecting after failure.
	TunnelReconnectDelay = 5 * time.Second
	// TunnelMaxReconnectDelay is the maximum reconnect delay with backoff.
	TunnelMaxReconnectDelay = 60 * time.Second
)

// TunnelConfig holds configuration for the tunnel client.
type TunnelConfig struct {
	// IngressAddr is the address of the Ingress tunnel listener (e.g., "ingress.enbox.id:9443").
	IngressAddr string
	// Token is the authentication token for this HQ.
	Token string
	// Endpoints is the list of endpoints to expose via Ingress.
	Endpoints []TunnelEndpoint
	// Logf is the logging function.
	Logf func(format string, args ...any)

	// ACME configuration for TLS termination (optional).
	// If nil or not enabled, TLS is passed through to local services.
	ACME *ACMESettings
	// CoordURL is the Central coordination server URL (for ACME DNS challenges).
	CoordURL string
	// APIToken is the API token for Central (for ACME DNS challenges).
	APIToken string
	// StateDir is the directory for persistent state (certificates, account key).
	StateDir string
	// BaseDomain is the base domain for ACME challenges (e.g., "enbox.id").
	BaseDomain string
}

// TunnelClient manages the tunnel connection from HQ to Ingress.
type TunnelClient struct {
	config TunnelConfig

	mu          sync.Mutex
	conn        net.Conn
	yamux       *yamux.Session
	control     net.Conn
	running     bool
	connected   bool
	closeCh     chan struct{}
	reconnectCh chan struct{}

	// certManager handles ACME certificate management (optional).
	certManager *certs.Manager
}

// NewTunnelClient creates a new tunnel client.
func NewTunnelClient(cfg TunnelConfig) *TunnelClient {
	if cfg.Logf == nil {
		cfg.Logf = func(format string, args ...any) {}
	}

	tc := &TunnelClient{
		config:      cfg,
		closeCh:     make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
	}

	// Initialize cert manager if ACME is enabled
	if cfg.ACME != nil && cfg.ACME.Enabled {
		certDir := cfg.ACME.CertDir
		if certDir == "" {
			certDir = cfg.StateDir
		}

		tc.certManager = certs.NewManager(certs.Config{
			StateDir:   certDir,
			CoordURL:   cfg.CoordURL,
			APIToken:   cfg.APIToken,
			Email:      cfg.ACME.Email,
			BaseDomain: cfg.BaseDomain,
			Staging:    cfg.ACME.Staging,
			Logf:       cfg.Logf,
		})
	}

	return tc
}

// Start begins the tunnel client, connecting to Ingress and maintaining the connection.
func (t *TunnelClient) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return errors.New("tunnel already running")
	}
	t.running = true
	t.closeCh = make(chan struct{})
	t.mu.Unlock()

	// Start cert manager if configured
	if t.certManager != nil {
		if err := t.certManager.Start(ctx); err != nil {
			return fmt.Errorf("cert manager start failed: %w", err)
		}

		// Pre-obtain certificates for all endpoints
		go t.preobtainCertificates()
	}

	// Start connection manager
	go t.connectionLoop(ctx)

	return nil
}

// preobtainCertificates requests certificates for all configured endpoints.
func (t *TunnelClient) preobtainCertificates() {
	if t.certManager == nil {
		return
	}

	for _, ep := range t.config.Endpoints {
		t.config.Logf("tunnel: obtaining certificate for %s", ep.Hostname)
		_, err := t.certManager.ObtainCert(ep.Hostname)
		if err != nil {
			t.config.Logf("tunnel: failed to obtain certificate for %s: %v", ep.Hostname, err)
		}
	}
}

// Stop gracefully shuts down the tunnel client.
func (t *TunnelClient) Stop() error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	t.running = false
	close(t.closeCh)
	t.mu.Unlock()

	// Stop cert manager
	if t.certManager != nil {
		_ = t.certManager.Stop()
	}

	// Close current connection if any
	t.closeConnection()

	return nil
}

// IsConnected returns whether the tunnel is currently connected.
func (t *TunnelClient) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// connectionLoop manages the connection lifecycle with automatic reconnection.
func (t *TunnelClient) connectionLoop(ctx context.Context) {
	delay := TunnelReconnectDelay

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.closeCh:
			return
		default:
		}

		// Attempt to connect
		err := t.connect(ctx)
		if err != nil {
			t.config.Logf("tunnel: connection failed: %v", err)
			t.closeConnection()

			// Wait before reconnecting with backoff
			select {
			case <-ctx.Done():
				return
			case <-t.closeCh:
				return
			case <-time.After(delay):
				// Increase delay with backoff, cap at max
				delay = delay * 2
				if delay > TunnelMaxReconnectDelay {
					delay = TunnelMaxReconnectDelay
				}
			}
			continue
		}

		// Reset delay on successful connection
		delay = TunnelReconnectDelay

		// Run the connection (blocks until disconnected)
		t.runConnection(ctx)

		// Connection ended, will reconnect
		t.closeConnection()
	}
}

// connect establishes the tunnel connection to Ingress.
func (t *TunnelClient) connect(ctx context.Context) error {
	t.config.Logf("tunnel: connecting to %s", t.config.IngressAddr)

	// Connect to Ingress
	dialer := &net.Dialer{Timeout: TunnelHandshakeTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", t.config.IngressAddr)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	// Create yamux client session
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = TunnelKeepaliveInterval
	cfg.ConnectionWriteTimeout = TunnelKeepaliveTimeout

	yamuxSession, err := yamux.Client(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("yamux client failed: %w", err)
	}

	// Open control stream
	controlStream, err := yamuxSession.Open()
	if err != nil {
		_ = yamuxSession.Close()
		_ = conn.Close()
		return fmt.Errorf("open control stream failed: %w", err)
	}

	// Perform handshake
	if err := t.handshake(controlStream); err != nil {
		_ = controlStream.Close()
		_ = yamuxSession.Close()
		_ = conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// Store connection state
	t.mu.Lock()
	t.conn = conn
	t.yamux = yamuxSession
	t.control = controlStream
	t.connected = true
	t.mu.Unlock()

	t.config.Logf("tunnel: connected to %s with %d endpoints", t.config.IngressAddr, len(t.config.Endpoints))

	return nil
}

// handshake performs the HELLO/HELLO_ACK exchange with Ingress.
func (t *TunnelClient) handshake(control net.Conn) error {
	// Set deadline for handshake
	if err := control.SetDeadline(time.Now().Add(TunnelHandshakeTimeout)); err != nil {
		return err
	}

	// Send HELLO
	hello := HelloMessage{
		Token:     t.config.Token,
		Endpoints: t.config.Endpoints,
	}
	if err := EncodeFrame(control, MsgHello, hello); err != nil {
		return fmt.Errorf("send HELLO failed: %w", err)
	}

	// Read HELLO_ACK
	frame, err := DecodeFrame(control)
	if err != nil {
		return fmt.Errorf("read HELLO_ACK failed: %w", err)
	}

	ack, err := ParseHelloAckMessage(frame)
	if err != nil {
		return err
	}

	if !ack.OK {
		return fmt.Errorf("registration rejected: %s", ack.Error)
	}

	// Clear deadline
	if err := control.SetDeadline(time.Time{}); err != nil {
		return err
	}

	return nil
}

// runConnection handles the connected state - keepalives and data streams.
func (t *TunnelClient) runConnection(ctx context.Context) {
	var wg sync.WaitGroup

	// Start keepalive sender
	wg.Add(1)
	go func() {
		defer wg.Done()
		t.keepaliveLoop(ctx)
	}()

	// Start control stream reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		t.controlReader(ctx)
	}()

	// Start data stream acceptor
	wg.Add(1)
	go func() {
		defer wg.Done()
		t.acceptDataStreams(ctx)
	}()

	// Wait for any goroutine to exit (indicates connection problem)
	wg.Wait()
}

// keepaliveLoop sends periodic keepalive messages.
func (t *TunnelClient) keepaliveLoop(ctx context.Context) {
	ticker := time.NewTicker(TunnelKeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.closeCh:
			return
		case <-ticker.C:
			t.mu.Lock()
			control := t.control
			t.mu.Unlock()

			if control == nil {
				return
			}

			if err := EncodeFrame(control, MsgKeepalive, KeepaliveMessage{}); err != nil {
				t.config.Logf("tunnel: keepalive send failed: %v", err)
				return
			}
		}
	}
}

// controlReader reads messages from the control stream.
func (t *TunnelClient) controlReader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.closeCh:
			return
		default:
		}

		t.mu.Lock()
		control := t.control
		t.mu.Unlock()

		if control == nil {
			return
		}

		frame, err := DecodeFrame(control)
		if err != nil {
			t.config.Logf("tunnel: control read error: %v", err)
			return
		}

		switch frame.Type {
		case MsgKeepalive:
			// Received keepalive from Ingress
		default:
			t.config.Logf("tunnel: unexpected message type: %s", frame.Type)
		}
	}
}

// acceptDataStreams accepts incoming data streams from Ingress and routes them.
func (t *TunnelClient) acceptDataStreams(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.closeCh:
			return
		default:
		}

		t.mu.Lock()
		yamuxSession := t.yamux
		t.mu.Unlock()

		if yamuxSession == nil {
			return
		}

		// Accept data stream from Ingress
		stream, err := yamuxSession.Accept()
		if err != nil {
			t.config.Logf("tunnel: accept stream error: %v", err)
			return
		}

		// Handle stream in goroutine
		go t.handleDataStream(stream)
	}
}

// handleDataStream processes an incoming data stream from Ingress.
// It extracts the SNI from the TLS ClientHello to determine routing.
// If ACME is enabled, it terminates TLS and proxies plaintext to local services.
// If ACME is disabled, it passes TLS through to local services.
func (t *TunnelClient) handleDataStream(stream net.Conn) {
	defer func() { _ = stream.Close() }()

	// Set read deadline for SNI extraction
	if err := stream.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.config.Logf("tunnel: set deadline failed: %v", err)
		return
	}

	// Peek the TLS ClientHello to extract SNI
	hostname, peekedData, err := t.peekSNI(stream)
	if err != nil {
		t.config.Logf("tunnel: SNI extraction failed: %v", err)
		return
	}

	// Clear deadline
	if err := stream.SetReadDeadline(time.Time{}); err != nil {
		t.config.Logf("tunnel: clear deadline failed: %v", err)
		return
	}

	// Find the endpoint for this hostname
	var localPort int
	for _, ep := range t.config.Endpoints {
		if ep.Hostname == hostname {
			localPort = ep.LocalPort
			break
		}
	}

	if localPort == 0 {
		t.config.Logf("tunnel: no endpoint for hostname %s", hostname)
		return
	}

	localAddr := fmt.Sprintf("127.0.0.1:%d", localPort)

	// Choose TLS termination or passthrough based on ACME config
	if t.certManager != nil {
		t.handleWithTLSTermination(stream, peekedData, hostname, localAddr)
	} else {
		t.handleWithTLSPassthrough(stream, peekedData, localAddr, hostname)
	}
}

// handleWithTLSTermination terminates TLS at HQ and proxies plaintext to local service.
func (t *TunnelClient) handleWithTLSTermination(stream net.Conn, peekedData []byte, hostname, localAddr string) {
	// Get certificate for this hostname
	cert, err := t.certManager.ObtainCert(hostname)
	if err != nil {
		t.config.Logf("tunnel: failed to get certificate for %s: %v", hostname, err)
		return
	}

	// Create a connection that replays the peeked data
	bufferedConn := &bufferedConn{
		Conn:   stream,
		buffer: peekedData,
	}

	// Create TLS server connection
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsConn := tls.Server(bufferedConn, tlsConfig)

	// Perform TLS handshake
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.config.Logf("tunnel: set TLS deadline failed: %v", err)
		return
	}

	if err := tlsConn.Handshake(); err != nil {
		t.config.Logf("tunnel: TLS handshake failed for %s: %v", hostname, err)
		return
	}

	if err := tlsConn.SetDeadline(time.Time{}); err != nil {
		t.config.Logf("tunnel: clear TLS deadline failed: %v", err)
		return
	}

	// Connect to local service (plaintext)
	localConn, err := net.DialTimeout("tcp", localAddr, 5*time.Second)
	if err != nil {
		t.config.Logf("tunnel: dial local %s failed: %v", localAddr, err)
		return
	}
	defer func() { _ = localConn.Close() }()

	t.config.Logf("tunnel: TLS termination %s -> %s", hostname, localAddr)

	// Bidirectional proxy (decrypted traffic)
	t.proxyConnections(tlsConn, localConn)
}

// handleWithTLSPassthrough passes TLS through to local service.
func (t *TunnelClient) handleWithTLSPassthrough(stream net.Conn, peekedData []byte, localAddr, hostname string) {
	// Connect to local service
	localConn, err := net.DialTimeout("tcp", localAddr, 5*time.Second)
	if err != nil {
		t.config.Logf("tunnel: dial local %s failed: %v", localAddr, err)
		return
	}
	defer func() { _ = localConn.Close() }()

	t.config.Logf("tunnel: passthrough %s -> %s", hostname, localAddr)

	// Create buffered reader that replays peeked data
	bufferedReader := io.MultiReader(
		&bytesReader{data: peekedData},
		stream,
	)

	// Bidirectional proxy
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream -> Local (includes buffered data)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(localConn, bufferedReader)
		if tc, ok := localConn.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	// Local -> Stream
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stream, localConn)
		if tc, ok := stream.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	wg.Wait()
}

// proxyConnections performs bidirectional proxying between two connections.
func (t *TunnelClient) proxyConnections(client, backend net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend
	go func() {
		defer wg.Done()
		_, _ = io.Copy(backend, client)
		if tc, ok := backend.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, backend)
		if tc, ok := client.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	wg.Wait()
}

// bufferedConn wraps a net.Conn and prepends buffered data to reads.
type bufferedConn struct {
	net.Conn
	buffer []byte
	pos    int
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if c.pos < len(c.buffer) {
		n := copy(p, c.buffer[c.pos:])
		c.pos += n
		return n, nil
	}
	return c.Conn.Read(p)
}

// peekSNI extracts the SNI hostname from a TLS ClientHello.
// Returns the hostname and the peeked bytes.
func (t *TunnelClient) peekSNI(conn net.Conn) (string, []byte, error) {
	// Read up to 16KB (max TLS record size)
	buf := make([]byte, 16384)
	n, err := conn.Read(buf)
	if err != nil {
		return "", nil, err
	}
	buf = buf[:n]

	// Check for TLS handshake record (0x16 = handshake)
	if len(buf) < 5 || buf[0] != 0x16 {
		return "", nil, errors.New("not a TLS handshake")
	}

	// Extract SNI from ClientHello
	sni, err := t.extractSNI(buf)
	if err != nil {
		return "", nil, err
	}

	// Return the peeked bytes
	return sni, buf, nil
}

// bytesReader is a simple reader over a byte slice.
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// extractSNI parses a TLS record to find the SNI hostname.
func (t *TunnelClient) extractSNI(data []byte) (string, error) {
	// TLS record header: type(1) + version(2) + length(2)
	if len(data) < 5 {
		return "", errors.New("data too short")
	}

	recordLen := int(binary.BigEndian.Uint16(data[3:5]))
	if len(data) < 5+recordLen {
		return "", errors.New("data too short for record")
	}

	handshake := data[5 : 5+recordLen]

	// Handshake header: type(1) + length(3)
	// ClientHello type is 0x01
	if len(handshake) < 4 || handshake[0] != 0x01 {
		return "", errors.New("not a ClientHello")
	}

	// Skip: type(1) + length(3) + version(2) + random(32) = 38 bytes
	pos := 38
	if len(handshake) < pos+1 {
		return "", errors.New("data too short")
	}

	// Skip session ID
	sessionIDLen := int(handshake[pos])
	pos += 1 + sessionIDLen

	// Skip cipher suites
	if len(handshake) < pos+2 {
		return "", errors.New("data too short")
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(handshake[pos:]))
	pos += 2 + cipherSuitesLen

	// Skip compression methods
	if len(handshake) < pos+1 {
		return "", errors.New("data too short")
	}
	compressionLen := int(handshake[pos])
	pos += 1 + compressionLen

	// Extensions
	if len(handshake) < pos+2 {
		return "", errors.New("no extensions")
	}
	extensionsLen := int(binary.BigEndian.Uint16(handshake[pos:]))
	pos += 2
	extensionsEnd := pos + extensionsLen

	// Parse extensions to find SNI (type 0x0000)
	for pos < extensionsEnd && len(handshake) >= pos+4 {
		extType := binary.BigEndian.Uint16(handshake[pos:])
		extLen := int(binary.BigEndian.Uint16(handshake[pos+2:]))
		pos += 4

		if extType == 0x0000 { // SNI extension
			return t.parseSNIExtension(handshake[pos : pos+extLen])
		}
		pos += extLen
	}

	return "", errors.New("SNI not found")
}

// parseSNIExtension parses the SNI extension data.
func (t *TunnelClient) parseSNIExtension(data []byte) (string, error) {
	// SNI format: list_length(2) + name_type(1) + name_length(2) + name
	if len(data) < 5 {
		return "", errors.New("SNI data too short")
	}

	nameType := data[2]
	nameLen := int(binary.BigEndian.Uint16(data[3:5]))

	if nameType != 0x00 { // host_name
		return "", errors.New("not a host_name")
	}

	if len(data) < 5+nameLen {
		return "", errors.New("SNI data too short")
	}

	return string(data[5 : 5+nameLen]), nil
}

// closeConnection closes the current connection.
func (t *TunnelClient) closeConnection() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.connected = false

	if t.control != nil {
		_ = t.control.Close()
		t.control = nil
	}

	if t.yamux != nil {
		_ = t.yamux.Close()
		t.yamux = nil
	}

	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
}
