package mesh

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MessageType represents the type of tunnel control message.
type MessageType byte

const (
	// MsgHello is sent by HQ to register endpoints with Ingress.
	MsgHello MessageType = 0x01
	// MsgHelloAck is sent by Ingress to confirm registration.
	MsgHelloAck MessageType = 0x02
	// MsgKeepalive is sent bidirectionally as a heartbeat.
	MsgKeepalive MessageType = 0x03
)

// String returns the string representation of the message type.
func (m MessageType) String() string {
	switch m {
	case MsgHello:
		return "HELLO"
	case MsgHelloAck:
		return "HELLO_ACK"
	case MsgKeepalive:
		return "KEEPALIVE"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", m)
	}
}

// TunnelEndpoint describes an endpoint that HQ wants to expose via Ingress.
type TunnelEndpoint struct {
	// Hostname is the public hostname for this endpoint (e.g., "api.alice.enbox.id").
	Hostname string `json:"hostname"`
	// LocalPort is the port on the HQ side to forward traffic to.
	LocalPort int `json:"local_port"`
}

// HelloMessage is sent by HQ to register its endpoints with Ingress.
type HelloMessage struct {
	// Token is the authentication token for this HQ.
	Token string `json:"token"`
	// Endpoints is the list of endpoints this HQ wants to expose.
	Endpoints []TunnelEndpoint `json:"endpoints"`
}

// HelloAckMessage is sent by Ingress to confirm registration.
type HelloAckMessage struct {
	// OK indicates whether registration was successful.
	OK bool `json:"ok"`
	// Error contains an error message if OK is false.
	Error string `json:"error,omitempty"`
}

// KeepaliveMessage is a heartbeat message.
type KeepaliveMessage struct{}

// Frame represents a protocol frame.
type Frame struct {
	Type    MessageType
	Payload []byte
}

// EncodeFrame writes a frame to the writer.
// Frame format: [Type:1B][Length:4B][Payload:JSON]
func EncodeFrame(w io.Writer, msgType MessageType, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Write type (1 byte)
	if _, err := w.Write([]byte{byte(msgType)}); err != nil {
		return fmt.Errorf("failed to write type: %w", err)
	}

	// Write length (4 bytes, big-endian)
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	if _, err := w.Write(length); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write payload
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write payload: %w", err)
	}

	return nil
}

// DecodeFrame reads a frame from the reader.
func DecodeFrame(r io.Reader) (*Frame, error) {
	// Read type (1 byte)
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, typeBuf); err != nil {
		return nil, fmt.Errorf("failed to read type: %w", err)
	}

	// Read length (4 bytes, big-endian)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// Sanity check length (max 1MB)
	if length > 1<<20 {
		return nil, fmt.Errorf("payload too large: %d bytes", length)
	}

	// Read payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("failed to read payload: %w", err)
	}

	return &Frame{
		Type:    MessageType(typeBuf[0]),
		Payload: payload,
	}, nil
}

// ParseHelloAckMessage parses a HelloAckMessage from a frame.
func ParseHelloAckMessage(f *Frame) (*HelloAckMessage, error) {
	if f.Type != MsgHelloAck {
		return nil, fmt.Errorf("expected HELLO_ACK, got %s", f.Type)
	}
	var msg HelloAckMessage
	if err := json.Unmarshal(f.Payload, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse HELLO_ACK: %w", err)
	}
	return &msg, nil
}
