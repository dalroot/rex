package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// PTYSession manages a long-lived interactive shell process on the server
type PTYSession struct {
	StreamID uint16
	cmd      *exec.Cmd
	ptyFile  *os.File
	mu       sync.Mutex
	closed   bool
}

// PTYManager holds active server sessions for a client connection
type PTYManager struct {
	mu       sync.RWMutex
	sessions map[uint16]*PTYSession
	conn     net.Conn
	connMu   *sync.Mutex
}

// NewPTYManager initializes the PTY session manager for a net.Conn
func NewPTYManager(conn net.Conn, connMu *sync.Mutex) *PTYManager {
	return &PTYManager{
		sessions: make(map[uint16]*PTYSession),
		conn:     conn,
		connMu:   connMu,
	}
}

// Spawn creates a new persistent PTY shell (e.g. /bin/bash) on the Linux server
func (m *PTYManager) Spawn(streamID uint16, cols, rows uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[streamID]; exists {
		return fmt.Errorf("session with streamID %d already exists", streamID)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	winSize := &pty.Winsize{
		Cols: cols,
		Rows: rows,
	}
	if cols == 0 {
		winSize.Cols = 80
	}
	if rows == 0 {
		winSize.Rows = 24
	}

	ptyFile, err := pty.StartWithSize(cmd, winSize)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	sess := &PTYSession{
		StreamID: streamID,
		cmd:      cmd,
		ptyFile:  ptyFile,
	}

	m.sessions[streamID] = sess

	// Background reader for streaming PTY stdout/stderr directly over TCP
	go m.readLoop(sess)

	log.Printf("[rex-pty] spawned persistent server shell (streamID=%d, pid=%d)", streamID, cmd.Process.Pid)
	return nil
}

// WriteStdin feeds input bytes directly into the server PTY shell STDIN
func (m *PTYManager) WriteStdin(streamID uint16, data []byte) error {
	m.mu.RLock()
	sess, exists := m.sessions[streamID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session streamID %d not found", streamID)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.closed {
		return fmt.Errorf("session streamID %d is closed", streamID)
	}

	_, err := sess.ptyFile.Write(data)
	return err
}

// Resize updates the terminal size on the server
func (m *PTYManager) Resize(streamID uint16, cols, rows uint16) error {
	m.mu.RLock()
	sess, exists := m.sessions[streamID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session streamID %d not found", streamID)
	}

	return pty.Setsize(sess.ptyFile, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// Close terminates a server PTY session
func (m *PTYManager) Close(streamID uint16) {
	m.mu.Lock()
	sess, exists := m.sessions[streamID]
	if exists {
		delete(m.sessions, streamID)
	}
	m.mu.Unlock()

	if exists {
		sess.mu.Lock()
		if !sess.closed {
			sess.closed = true
			_ = sess.ptyFile.Close()
			if sess.cmd.Process != nil {
				_ = sess.cmd.Process.Kill()
			}
		}
		sess.mu.Unlock()
		log.Printf("[rex-pty] closed session streamID=%d", streamID)
	}
}

// CloseAll closes all active sessions for this connection
func (m *PTYManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for streamID, sess := range m.sessions {
		sess.mu.Lock()
		if !sess.closed {
			sess.closed = true
			_ = sess.ptyFile.Close()
			if sess.cmd.Process != nil {
				_ = sess.cmd.Process.Kill()
			}
		}
		sess.mu.Unlock()
		delete(m.sessions, streamID)
	}
}

func (m *PTYManager) readLoop(sess *PTYSession) {
	buf := make([]byte, 4096)
	for {
		n, err := sess.ptyFile.Read(buf)
		if n > 0 {
			m.connMu.Lock()
			_ = WriteFrame(m.conn, OpPTYData, sess.StreamID, buf[:n])
			m.connMu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[rex-pty] read error streamID=%d: %v", sess.StreamID, err)
			}
			break
		}
	}
	m.connMu.Lock()
	_ = WriteFrame(m.conn, OpPTYClose, sess.StreamID, nil)
	m.connMu.Unlock()
	m.Close(sess.StreamID)
}
