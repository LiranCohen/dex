package mesh

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunnelProtocolEncodeDecodeFrame(t *testing.T) {
	tests := []struct {
		name    string
		msgType MessageType
		payload any
	}{
		{
			name:    "hello message",
			msgType: MsgHello,
			payload: HelloMessage{
				Token: "test-token",
				Endpoints: []TunnelEndpoint{
					{Hostname: "app.example.com", LocalPort: 8080},
				},
			},
		},
		{
			name:    "hello ack success",
			msgType: MsgHelloAck,
			payload: HelloAckMessage{OK: true},
		},
		{
			name:    "hello ack error",
			msgType: MsgHelloAck,
			payload: HelloAckMessage{OK: false, Error: "invalid token"},
		},
		{
			name:    "keepalive",
			msgType: MsgKeepalive,
			payload: KeepaliveMessage{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Encode
			err := EncodeFrame(&buf, tt.msgType, tt.payload)
			require.NoError(t, err)

			// Decode
			frame, err := DecodeFrame(&buf)
			require.NoError(t, err)

			assert.Equal(t, tt.msgType, frame.Type)
			assert.NotEmpty(t, frame.Payload)
		})
	}
}

func TestTunnelProtocolMessageTypeString(t *testing.T) {
	assert.Equal(t, "HELLO", MsgHello.String())
	assert.Equal(t, "HELLO_ACK", MsgHelloAck.String())
	assert.Equal(t, "KEEPALIVE", MsgKeepalive.String())
	assert.Equal(t, "UNKNOWN(255)", MessageType(255).String())
}

func TestTunnelClientConfig(t *testing.T) {
	cfg := TunnelConfig{
		IngressAddr: "ingress.example.com:9443",
		Token:       "test-token",
		Endpoints: []TunnelEndpoint{
			{Hostname: "app.example.com", LocalPort: 8080},
		},
	}

	client := NewTunnelClient(cfg)
	assert.NotNil(t, client)
	assert.False(t, client.IsConnected())
}

// simulateIngress creates a mock Ingress server for testing.
func simulateIngress(t *testing.T, acceptHandshake bool, token string) (net.Listener, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer func() { _ = c.Close() }()

				// Create yamux server
				cfg := yamux.DefaultConfig()
				cfg.EnableKeepAlive = false
				yamuxSession, err := yamux.Server(c, cfg)
				if err != nil {
					return
				}
				defer func() { _ = yamuxSession.Close() }()

				// Accept control stream
				control, err := yamuxSession.Accept()
				if err != nil {
					return
				}
				defer func() { _ = control.Close() }()

				// Read HELLO
				frame, err := DecodeFrame(control)
				if err != nil {
					return
				}

				var hello HelloMessage
				_ = json.Unmarshal(frame.Payload, &hello)

				// Send HELLO_ACK
				if acceptHandshake && (token == "" || hello.Token == token) {
					_ = EncodeFrame(control, MsgHelloAck, HelloAckMessage{OK: true})
				} else {
					_ = EncodeFrame(control, MsgHelloAck, HelloAckMessage{OK: false, Error: "rejected"})
					return
				}

				// Keep connection open and handle keepalives
				for {
					frame, err := DecodeFrame(control)
					if err != nil {
						return
					}
					if frame.Type == MsgKeepalive {
						continue // Acknowledge keepalive, no response needed
					}
				}
			}(conn)
		}
	}()

	return ln, func() { _ = ln.Close() }
}

func TestTunnelClientConnectSuccess(t *testing.T) {
	// Start mock Ingress
	ln, cleanup := simulateIngress(t, true, "test-token")
	defer cleanup()

	// Create tunnel client
	client := NewTunnelClient(TunnelConfig{
		IngressAddr: ln.Addr().String(),
		Token:       "test-token",
		Endpoints: []TunnelEndpoint{
			{Hostname: "app.example.com", LocalPort: 8080},
		},
		Logf: t.Logf,
	})

	// Start client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Stop() }()

	// Wait for connection
	time.Sleep(100 * time.Millisecond)
	assert.True(t, client.IsConnected())
}

func TestTunnelClientConnectRejected(t *testing.T) {
	// Start mock Ingress that rejects
	ln, cleanup := simulateIngress(t, false, "")
	defer cleanup()

	// Create tunnel client
	client := NewTunnelClient(TunnelConfig{
		IngressAddr: ln.Addr().String(),
		Token:       "wrong-token",
		Endpoints: []TunnelEndpoint{
			{Hostname: "app.example.com", LocalPort: 8080},
		},
		Logf: t.Logf,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err) // Start succeeds, but connection will fail
	defer func() { _ = client.Stop() }()

	// Wait a bit - connection should fail
	time.Sleep(200 * time.Millisecond)
	assert.False(t, client.IsConnected())
}

func TestTunnelClientStop(t *testing.T) {
	ln, cleanup := simulateIngress(t, true, "")
	defer cleanup()

	client := NewTunnelClient(TunnelConfig{
		IngressAddr: ln.Addr().String(),
		Token:       "test-token",
		Endpoints: []TunnelEndpoint{
			{Hostname: "app.example.com", LocalPort: 8080},
		},
		Logf: t.Logf,
	})

	ctx := context.Background()
	err := client.Start(ctx)
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)
	assert.True(t, client.IsConnected())

	// Stop
	err = client.Stop()
	require.NoError(t, err)
	assert.False(t, client.IsConnected())
}

func TestTunnelClientDataStream(t *testing.T) {
	// Start a local "backend" service
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = backendLn.Close() }()

	backendPort := backendLn.Addr().(*net.TCPAddr).Port

	// Backend echoes data
	var backendWg sync.WaitGroup
	backendWg.Add(1)
	go func() {
		defer backendWg.Done()
		conn, err := backendLn.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(conn, conn) // Echo
	}()

	// Start Ingress-like server that opens data stream
	ingressLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ingressLn.Close() }()

	dataReceived := make(chan []byte, 1)

	go func() {
		conn, err := ingressLn.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		cfg := yamux.DefaultConfig()
		cfg.EnableKeepAlive = false
		yamuxSession, err := yamux.Server(conn, cfg)
		if err != nil {
			return
		}
		defer func() { _ = yamuxSession.Close() }()

		// Accept control stream
		control, err := yamuxSession.Accept()
		if err != nil {
			return
		}

		// Handle HELLO/HELLO_ACK
		frame, _ := DecodeFrame(control)
		var hello HelloMessage
		_ = json.Unmarshal(frame.Payload, &hello)
		_ = EncodeFrame(control, MsgHelloAck, HelloAckMessage{OK: true})

		// Wait for handshake to complete
		time.Sleep(100 * time.Millisecond)

		// Open data stream (like Ingress would)
		dataStream, err := yamuxSession.Open()
		if err != nil {
			return
		}
		defer func() { _ = dataStream.Close() }()

		// Send TLS ClientHello with SNI
		clientHello := buildTestClientHello("app.example.com")
		_, _ = dataStream.Write(clientHello)

		// Read echoed response
		buf := make([]byte, len(clientHello))
		n, _ := io.ReadFull(dataStream, buf)
		dataReceived <- buf[:n]
	}()

	// Create tunnel client
	client := NewTunnelClient(TunnelConfig{
		IngressAddr: ingressLn.Addr().String(),
		Token:       "test-token",
		Endpoints: []TunnelEndpoint{
			{Hostname: "app.example.com", LocalPort: backendPort},
		},
		Logf: t.Logf,
	})

	ctx := context.Background()
	err = client.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Stop() }()

	// Wait for data to flow
	select {
	case data := <-dataReceived:
		// Should receive the echoed ClientHello
		assert.NotEmpty(t, data)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for data")
	}

	backendWg.Wait()
}

// buildTestClientHello creates a minimal TLS ClientHello with SNI for testing.
func buildTestClientHello(hostname string) []byte {
	// This is a minimal valid TLS 1.2 ClientHello with SNI
	// Structure: TLS record header + Handshake header + ClientHello body with SNI extension

	hostnameBytes := []byte(hostname)
	hostnameLen := len(hostnameBytes)

	// SNI extension
	sniExtension := []byte{
		0x00, 0x00, // Extension type: SNI
		byte((hostnameLen + 5) >> 8), byte((hostnameLen + 5) & 0xff), // Extension length
		byte((hostnameLen + 3) >> 8), byte((hostnameLen + 3) & 0xff), // SNI list length
		0x00,                                             // Name type: host_name
		byte(hostnameLen >> 8), byte(hostnameLen & 0xff), // Name length
	}
	sniExtension = append(sniExtension, hostnameBytes...)

	// Extensions block
	extensions := make([]byte, 2)
	extensions[0] = byte(len(sniExtension) >> 8)
	extensions[1] = byte(len(sniExtension) & 0xff)
	extensions = append(extensions, sniExtension...)

	// ClientHello body (minimal)
	clientHello := []byte{
		0x03, 0x03, // Version: TLS 1.2
	}
	// Random (32 bytes)
	random := make([]byte, 32)
	clientHello = append(clientHello, random...)
	// Session ID (empty)
	clientHello = append(clientHello, 0x00)
	// Cipher suites (2 bytes length + 2 bytes for one suite)
	clientHello = append(clientHello, 0x00, 0x02, 0x00, 0x2f) // TLS_RSA_WITH_AES_128_CBC_SHA
	// Compression methods (1 byte length + 1 byte for null)
	clientHello = append(clientHello, 0x01, 0x00)
	// Extensions
	clientHello = append(clientHello, extensions...)

	// Handshake header
	handshakeHeader := []byte{
		0x01,                                                             // ClientHello
		0x00, byte(len(clientHello) >> 8), byte(len(clientHello) & 0xff), // Length (3 bytes)
	}
	handshake := append(handshakeHeader, clientHello...)

	// TLS record header
	recordHeader := []byte{
		0x16,       // Content type: Handshake
		0x03, 0x01, // Version: TLS 1.0 (for compatibility)
		byte(len(handshake) >> 8), byte(len(handshake) & 0xff), // Length
	}

	return append(recordHeader, handshake...)
}

func TestBuildTestClientHello(t *testing.T) {
	// Verify our test helper builds a parseable ClientHello
	clientHello := buildTestClientHello("test.example.com")

	// Should start with TLS handshake record type
	assert.Equal(t, byte(0x16), clientHello[0])

	// Should be parseable (at least check length)
	assert.Greater(t, len(clientHello), 50)
}
