package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server is the REX WebSocket server
type Server struct {
	cfg       *Config
	allowlist *Allowlist
	executor  *Executor
	streamer  *Streamer
}

func NewServer(cfg *Config, al *Allowlist) *Server {
	return &Server{
		cfg:       cfg,
		allowlist: al,
		executor:  NewExecutor(al),
		streamer:  NewStreamer(al),
	}
}

func (s *Server) Start() error {
	go s.StartTCP()

	mux := http.NewServeMux()
	mux.HandleFunc("/rex", s.handleConnection)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "rex-node ok")
	})
	mux.HandleFunc("/", s.handleAgentHTTPProbe)

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	log.Printf("[rex] WebSocket listening on %s", addr)

	if s.cfg.TLS && s.cfg.CertFile != "" && s.cfg.KeyFile != "" {
		return http.ListenAndServeTLS(addr, s.cfg.CertFile, s.cfg.KeyFile, mux)
	}
	return http.ListenAndServe(addr, mux)
}

func (s *Server) StartTCP() {
	tcpAddr := fmt.Sprintf(":%d", s.cfg.TCPPort)
	var listener net.Listener
	var err error

	if s.cfg.TLS && s.cfg.CertFile != "" && s.cfg.KeyFile != "" {
		cert, errTls := tls.LoadX509KeyPair(s.cfg.CertFile, s.cfg.KeyFile)
		if errTls != nil {
			log.Printf("[rex-tcp] TLS error loading cert: %v", errTls)
			return
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		}
		listener, err = tls.Listen("tcp", tcpAddr, tlsConfig)
		if err != nil {
			log.Printf("[rex-tcp] TLS listen error on %s: %v", tcpAddr, err)
			return
		}
		log.Printf("[rex-tcp] RXP/2.0 TLS 1.3 Binary Server listening on %s", tcpAddr)
	} else {
		log.Printf("[rex-tcp] WARNING: Plaintext TCP listening on %s (TLS disabled)", tcpAddr)
		listener, err = net.Listen("tcp", tcpAddr)
		if err != nil {
			log.Printf("[rex-tcp] failed to listen on %s: %v", tcpAddr, err)
			return
		}
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[rex-tcp] accept error: %v", err)
			continue
		}
		go s.handleTCPConn(conn)
	}
}

func (s *Server) handleTCPConn(conn net.Conn) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[rex-tcp] client connected: %s", remoteAddr)

	var connMu sync.Mutex
	ptyMgr := NewPTYManager(conn, &connMu)
	defer ptyMgr.CloseAll()

	authenticated := false

	for {
		frame, err := ReadFrame(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("[rex-tcp] read error (%s): %v", remoteAddr, err)
			}
			return
		}

		if !authenticated && frame.Opcode != OpAuthHandshake {
			log.Printf("[rex-tcp] unauthenticated frame from %s", remoteAddr)
			connMu.Lock()
			_ = WriteFrame(conn, OpError, frame.StreamID, []byte("unauthenticated"))
			connMu.Unlock()
			return
		}

		switch frame.Opcode {
		case OpAuthHandshake:
			token := string(frame.Payload)
			if token != s.cfg.Token {
				log.Printf("[rex-tcp] auth failed for %s", remoteAddr)
				connMu.Lock()
				_ = WriteFrame(conn, OpError, frame.StreamID, []byte("invalid token"))
				connMu.Unlock()
				return
			}
			authenticated = true
			connMu.Lock()
			// Return RXP/2.5 OK together with the authoritative agent rules
			handshakeAck := fmt.Sprintf("RXP/2.5 OK\n%s", REXProtocolRule)
			_ = WriteFrame(conn, OpAuthHandshake, frame.StreamID, []byte(handshakeAck))
			connMu.Unlock()
			log.Printf("[rex-tcp] client authenticated (%s)", remoteAddr)

		case OpFastExec:
			cmdStr := string(frame.Payload)
			if s.allowlist != nil {
				allowed, reason := s.allowlist.IsCommandAllowed(cmdStr)
				if !allowed {
					log.Printf("[rex-tcp] fast exec denied for %s: %s", remoteAddr, reason)
					connMu.Lock()
					_ = WriteFrame(conn, OpError, frame.StreamID, []byte("command denied: "+reason))
					connMu.Unlock()
					continue
				}
			}
			go func(streamID uint16, cmd string) {
				res := s.executor.Run(cmd, 60)
				outBytes := []byte(res.Stdout)
				if res.Stderr != "" {
					if len(outBytes) > 0 {
						outBytes = append(outBytes, '\n')
					}
					outBytes = append(outBytes, []byte(res.Stderr)...)
				}
				connMu.Lock()
				_ = WriteFrame(conn, OpFastExec, streamID, outBytes)
				connMu.Unlock()
			}(frame.StreamID, cmdStr)

		case OpPTYSpawn:
			if s.allowlist != nil {
				allowed, reason := s.allowlist.IsCommandAllowed("bash")
				if !allowed {
					log.Printf("[rex-tcp] PTY spawn denied for %s: %s", remoteAddr, reason)
					connMu.Lock()
					_ = WriteFrame(conn, OpError, frame.StreamID, []byte("PTY spawn denied: "+reason))
					connMu.Unlock()
					continue
				}
			}

			cols, rows := uint16(80), uint16(24)
			if len(frame.Payload) >= 4 {
				cols = binary.BigEndian.Uint16(frame.Payload[0:2])
				rows = binary.BigEndian.Uint16(frame.Payload[2:4])
			}
			if err := ptyMgr.Spawn(frame.StreamID, cols, rows); err != nil {
				connMu.Lock()
				_ = WriteFrame(conn, OpError, frame.StreamID, []byte(err.Error()))
				connMu.Unlock()
			} else {
				connMu.Lock()
				_ = WriteFrame(conn, OpPTYSpawn, frame.StreamID, []byte("PTY_SPAWNED"))
				connMu.Unlock()
			}

		case OpPTYData:
			_ = ptyMgr.WriteStdin(frame.StreamID, frame.Payload)

		case OpPTYResize:
			if len(frame.Payload) >= 4 {
				cols := binary.BigEndian.Uint16(frame.Payload[0:2])
				rows := binary.BigEndian.Uint16(frame.Payload[2:4])
				_ = ptyMgr.Resize(frame.StreamID, cols, rows)
			}

		case OpPTYClose:
			ptyMgr.Close(frame.StreamID)

		case OpNativeFileOp:
			resBytes := HandleNativeFileOp(frame.Payload, s.allowlist)
			connMu.Lock()
			_ = WriteFrame(conn, OpNativeFileOp, frame.StreamID, resBytes)
			connMu.Unlock()

		case OpNativeSysInfo:
			info := collectSysinfo()
			infoBytes, _ := json.Marshal(info)
			connMu.Lock()
			_ = WriteFrame(conn, OpNativeSysInfo, frame.StreamID, infoBytes)
			connMu.Unlock()

		case OpPing:
			connMu.Lock()
			_ = WriteFrame(conn, OpPong, frame.StreamID, frame.Payload)
			connMu.Unlock()

		case OpAgentGuide:
			connMu.Lock()
			_ = WriteFrame(conn, OpAgentGuide, frame.StreamID, []byte(REXProtocolRule))
			connMu.Unlock()

		default:
			log.Printf("[rex-tcp] unknown opcode: 0x%02x", frame.Opcode)
		}
	}
}

func (s *Server) handleAgentHTTPProbe(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "rex-node ok")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	guide := map[string]interface{}{
		"service":     "REX Node Daemon (RXP/2.5 WarpGate)",
		"version":     "2.5.0",
		"status":      "online",
		"protocol":    "RXP (Remote EXecution Protocol)",
		"instruction": "This is an agent-native infrastructure runtime, NOT a REST API or SSH. Do not probe HTTP endpoints.",
		"quickstart": map[string]string{
			"exec_command":         "rex exec <SERVER_IP>:7444 -t <TOKEN> \"<command>\"",
			"interactive_terminal": "rex connect <SERVER_IP>:7444 -t <TOKEN>",
			"install_rex_cli":      "curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/install-cli.sh | bash",
			"python_sdk":           "from rex_client import RXPDirectClient",
		},
		"ports": map[string]interface{}{
			"binary_tcp_tls": s.cfg.TCPPort,
			"websocket_tls":  s.cfg.Port,
		},
		"documentation": "https://github.com/dalroot/rex",
	}

	_ = json.NewEncoder(w).Encode(guide)
}

func (s *Server) handleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[rex] upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("[rex] client connected: %s", r.RemoteAddr)

	if !s.doHandshake(conn) {
		return
	}

	s.handleSession(conn, r.RemoteAddr)
	log.Printf("[rex] client disconnected: %s", r.RemoteAddr)
}

func (s *Server) doHandshake(conn *websocket.Conn) bool {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	_, data, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[rex] handshake read error: %v", err)
		return false
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[rex] handshake invalid JSON: %v", err)
		return false
	}

	if msg["type"] != "handshake" {
		log.Printf("[rex] expected handshake, got: %v", msg["type"])
		return false
	}

	token, _ := msg["token"].(string)
	if token != s.cfg.Token {
		log.Printf("[rex] auth failed: invalid token")
		conn.WriteJSON(map[string]interface{}{
			"type":   "handshake_ack",
			"status": "error",
			"error":  "invalid token",
		})
		return false
	}

	hostname, _ := os.Hostname()
	conn.SetReadDeadline(time.Time{})

	conn.WriteJSON(map[string]interface{}{
		"type":         "handshake_ack",
		"status":       "ok",
		"node_id":      hostname,
		"os":           runtime.GOOS + "/" + runtime.GOARCH,
		"capabilities": []string{"exec", "fast_exec", "stream", "sysinfo", "file_write", "file_read"},
		"protocol":     "RXP/2.5",
		"agent_guide":  REXProtocolRule,
	})

	log.Printf("[rex] client authenticated (RXP/1.0 persistent session active)")
	return true
}

func (s *Server) handleSession(conn *websocket.Conn, remoteAddr string) {
	var mu sync.Mutex
	streams := make(map[string]context.CancelFunc)

	// Set Pong handler to keep persistent channel alive
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Time{})
		return nil
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[rex] session ended gracefully by client (%s)", remoteAddr)
			} else if websocket.IsUnexpectedCloseError(err) {
				log.Printf("[rex] connection dropped: %v", err)
			}
			for _, cancel := range streams {
				cancel()
			}
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[rex] invalid JSON frame: %v", err)
			continue
		}

		msgType, _ := msg["type"].(string)
		msgID, _ := msg["id"].(string)

		switch msgType {
		case "exec":
			go s.handleExec(conn, &mu, msg, msgID)

		case "stream":
			path, _ := msg["target"].(string)
			tailLines := 50
			if v, ok := msg["lines"].(float64); ok {
				tailLines = int(v)
			}
			ctx, cancel := context.WithCancel(context.Background())
			streams[msgID] = cancel
			go func() {
				s.streamer.StreamFile(ctx, conn, &mu, msgID, path, tailLines)
				delete(streams, msgID)
			}()

		case "stream_stop":
			if cancel, ok := streams[msgID]; ok {
				cancel()
				delete(streams, msgID)
			}

		case "sysinfo":
			go s.handleSysinfo(conn, &mu, msgID)

		case "file_write":
			go s.handleFileWrite(conn, &mu, msg, msgID)

		case "file_read":
			go s.handleFileRead(conn, &mu, msg, msgID)

		case "ping":
			_ = sendJSON(conn, &mu, map[string]interface{}{
				"type": "pong",
				"id":   msgID,
				"ts":   time.Now().UnixMilli(),
			})

		default:
			log.Printf("[rex] unknown frame type: %q", msgType)
			_ = sendJSON(conn, &mu, map[string]interface{}{
				"type":  "error",
				"id":    msgID,
				"error": fmt.Sprintf("unknown type: %s", msgType),
			})
		}
	}
}

func (s *Server) handleExec(conn *websocket.Conn, mu sync.Locker, msg map[string]interface{}, id string) {
	command, _ := msg["command"].(string)
	timeout := 30
	if v, ok := msg["timeout"].(float64); ok {
		timeout = int(v)
	}

	log.Printf("[rex] exec: %q", command)
	result := s.executor.Run(command, timeout)

	_ = sendJSON(conn, mu, map[string]interface{}{
		"type":        "exec_result",
		"id":          id,
		"exit_code":   result.ExitCode,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"duration_ms": result.DurationMs,
		"error":       result.Error,
	})
}

func (s *Server) handleSysinfo(conn *websocket.Conn, mu sync.Locker, id string) {
	info := collectSysinfo()
	info["type"] = "sysinfo_result"
	info["id"] = id
	_ = sendJSON(conn, mu, info)
}

func (s *Server) handleFileWrite(conn *websocket.Conn, mu sync.Locker, msg map[string]interface{}, id string) {
	path, _ := msg["path"].(string)
	dataStr, _ := msg["data"].(string)
	appendMode, _ := msg["append"].(bool)
	modeVal := 0644
	if m, ok := msg["mode"].(float64); ok && m > 0 {
		modeVal = int(m)
	}

	if s.allowlist != nil {
		allowed, reason := s.allowlist.IsCommandAllowed(fmt.Sprintf("write %s", path))
		if !allowed {
			_ = sendJSON(conn, mu, map[string]interface{}{
				"type":   "file_write_result",
				"id":     id,
				"status": "error",
				"error":  "security policy denied: " + reason,
			})
			return
		}
	}

	data, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		data = []byte(dataStr)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	f, err := os.OpenFile(path, flags, os.FileMode(modeVal))
	if err != nil {
		_ = sendJSON(conn, mu, map[string]interface{}{
			"type":   "file_write_result",
			"id":     id,
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	defer f.Close()

	n, err := f.Write(data)
	if err != nil {
		_ = sendJSON(conn, mu, map[string]interface{}{
			"type":   "file_write_result",
			"id":     id,
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	_ = sendJSON(conn, mu, map[string]interface{}{
		"type":          "file_write_result",
		"id":            id,
		"status":        "ok",
		"bytes_written": n,
	})
}

func (s *Server) handleFileRead(conn *websocket.Conn, mu sync.Locker, msg map[string]interface{}, id string) {
	path, _ := msg["path"].(string)

	if s.allowlist != nil && s.allowlist.Mode == LevelAllowlist && !s.allowlist.IsPathAllowed(path) {
		_ = sendJSON(conn, mu, map[string]interface{}{
			"type":   "file_read_result",
			"id":     id,
			"status": "error",
			"error":  "access to path denied by allowlist policy",
		})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		_ = sendJSON(conn, mu, map[string]interface{}{
			"type":   "file_read_result",
			"id":     id,
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	_ = sendJSON(conn, mu, map[string]interface{}{
		"type":   "file_read_result",
		"id":     id,
		"status": "ok",
		"data":   base64.StdEncoding.EncodeToString(data),
		"size":   len(data),
	})
}

func sendJSON(conn *websocket.Conn, mu interface{ Lock(); Unlock() }, v interface{}) error {
	mu.Lock()
	defer mu.Unlock()
	return conn.WriteJSON(v)
}
