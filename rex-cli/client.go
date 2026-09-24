package main

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// RXP/2.0 Magic bytes: 'R', 'X'
var MagicBytes = [2]byte{'R', 'X'}

const Version2 = 0x02

// RXP Opcodes
const (
	OpAuthHandshake byte = 0x01
	OpPTYSpawn      byte = 0x02
	OpPTYData       byte = 0x03
	OpPTYResize     byte = 0x04
	OpPTYClose      byte = 0x05
	OpNativeFileOp  byte = 0x06
	OpNativeSysInfo byte = 0x07
	OpPing          byte = 0x08
	OpPong          byte = 0x09
	OpAgentGuide    byte = 0x0A
	OpFastExec      byte = 0x0B
	OpError         byte = 0xFF
)

// Frame represents an RXP/2.0 binary packet header and payload
type Frame struct {
	Version  byte
	Opcode   byte
	StreamID uint16
	Payload  []byte
}

// ReadFrame reads an RXP/2.0 binary packet from an io.Reader
func ReadFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	if header[0] != MagicBytes[0] || header[1] != MagicBytes[1] {
		return nil, fmt.Errorf("invalid magic bytes: 0x%02x 0x%02x", header[0], header[1])
	}

	ver := header[2]
	opcode := header[3]
	streamID := binary.BigEndian.Uint16(header[4:6])
	payloadLen := binary.BigEndian.Uint16(header[6:8])

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("failed to read payload: %w", err)
		}
	}

	return &Frame{
		Version:  ver,
		Opcode:   opcode,
		StreamID: streamID,
		Payload:  payload,
	}, nil
}

// WriteFrame encodes and sends an RXP/2.0 binary frame to an io.Writer
func WriteFrame(w io.Writer, opcode byte, streamID uint16, payload []byte) error {
	payloadLen := len(payload)
	if payloadLen > 65535 {
		return fmt.Errorf("payload size exceeds max 65535 bytes")
	}

	buf := make([]byte, 8+payloadLen)
	buf[0] = MagicBytes[0]
	buf[1] = MagicBytes[1]
	buf[2] = Version2
	buf[3] = opcode
	binary.BigEndian.PutUint16(buf[4:6], streamID)
	binary.BigEndian.PutUint16(buf[6:8], uint16(payloadLen))

	if payloadLen > 0 {
		copy(buf[8:], payload)
	}

	_, err := w.Write(buf)
	return err
}

// Client represents a direct native connection to REX server over RXP/2.0
type Client struct {
	conn       net.Conn
	mu         sync.Mutex
	token      string
	insecure   bool
	streamChan map[uint16]chan *Frame
	streamMu   sync.RWMutex
	closed     bool
}

// Dial connects to REX server over TLS 1.3 or Raw TCP
func Dial(addr, token string, useTLS, insecure bool) (*Client, error) {
	var conn net.Conn
	var err error

	if useTLS {
		tlsConfig := &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: insecure,
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	client := &Client{
		conn:       conn,
		token:      token,
		insecure:   insecure,
		streamChan: make(map[uint16]chan *Frame),
	}

	// Authenticate via OpAuthHandshake
	if err := client.authenticate(); err != nil {
		conn.Close()
		return nil, err
	}

	// Start demux reader
	go client.readLoop()

	return client, nil
}

func (c *Client) authenticate() error {
	frame := Frame{
		Version:  Version2,
		Opcode:   OpAuthHandshake,
		StreamID: 0,
		Payload:  []byte(c.token),
	}

	c.mu.Lock()
	err := WriteFrame(c.conn, frame.Opcode, frame.StreamID, frame.Payload)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// Read ack
	ack, err := ReadFrame(c.conn)
	if err != nil {
		return fmt.Errorf("handshake read failed: %w", err)
	}

	if ack.Opcode == OpError {
		return fmt.Errorf("auth error: %s", string(ack.Payload))
	}

	if ack.Opcode != OpAuthHandshake {
		return fmt.Errorf("unexpected handshake response: 0x%02x", ack.Opcode)
	}

	return nil
}

func (c *Client) readLoop() {
	for {
		frame, err := ReadFrame(c.conn)
		if err != nil {
			c.Close()
			return
		}

		c.streamMu.RLock()
		ch, exists := c.streamChan[frame.StreamID]
		c.streamMu.RUnlock()

		if exists {
			ch <- frame
		}
	}
}

func (c *Client) RegisterStream(streamID uint16) chan *Frame {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	ch := make(chan *Frame, 128)
	c.streamChan[streamID] = ch
	return ch
}

func (c *Client) UnregisterStream(streamID uint16) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if ch, ok := c.streamChan[streamID]; ok {
		close(ch)
		delete(c.streamChan, streamID)
	}
}

func (c *Client) Send(opcode byte, streamID uint16, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("client closed")
	}
	return WriteFrame(c.conn, opcode, streamID, payload)
}

func (c *Client) Close() {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.conn.Close()
	}
	c.mu.Unlock()

	c.streamMu.Lock()
	for id, ch := range c.streamChan {
		close(ch)
		delete(c.streamChan, id)
	}
	c.streamMu.Unlock()
}
