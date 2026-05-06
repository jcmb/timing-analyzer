package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/stream"
	"timing-analyzer/internal/telemetry"
	"timing-analyzer/internal/timing"
	"timing-analyzer/web"
)

// Global Application Version
const AppVersion = "v1.3.3"

// webListenPortMin/Max restrict inbound listener ports for the multi-tenant web UI
// so sessions do not collide with other well-known services on the host.
const (
	webListenPortMin = 20000
	webListenPortMax = 29999
)

type Session struct {
	ID     string
	Cancel context.CancelFunc
	Broker *telemetry.SSEBroker
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

var manager = &SessionManager{
	sessions: make(map[string]*Session),
}

func generateSessionID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

type StartRequest struct {
	Mode       string  `json:"mode"`
	Host       string  `json:"host"`
	Port       int     `json:"port"`
	Mountpoint string  `json:"mountpoint"`
	Username   string  `json:"username"`
	Password   string  `json:"password"`
	Rate       float64 `json:"rate"`
	Jitter     string  `json:"jitter"`
	Decode     string  `json:"decode"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func probeTCPListenPort(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return err
	}
	return ln.Close()
}

func probeUDPListenPort(port int) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return err
	}
	c, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	return c.Close()
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case "tcp", "ntrip", "ibss", "tcp_listen", "udp_listen":
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid connection mode")
		return
	}

	jitterStr := strings.TrimSpace(req.Jitter)
	var jitterVal float64
	var jitterPct bool
	if strings.HasSuffix(jitterStr, "%") {
		jitterPct = true
		val, err := strconv.ParseFloat(strings.TrimSuffix(jitterStr, "%"), 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid jitter percentage")
			return
		}
		jitterVal = val
	} else {
		val, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(jitterStr), "ms"), 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid jitter value")
			return
		}
		jitterVal = val
	}

	switch mode {
	case "tcp_listen", "udp_listen":
		if req.Port < webListenPortMin || req.Port > webListenPortMax {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("listen port must be between %d and %d (inclusive)", webListenPortMin, webListenPortMax))
			return
		}
		if mode == "tcp_listen" {
			if err := probeTCPListenPort(req.Port); err != nil {
				writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("cannot listen on TCP port %d: %v", req.Port, err))
				return
			}
		} else {
			if err := probeUDPListenPort(req.Port); err != nil {
				writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("cannot listen on UDP port %d: %v", req.Port, err))
				return
			}
		}
	default:
		if strings.TrimSpace(req.Host) == "" || req.Port < 1 || req.Port > 65535 {
			writeJSONError(w, http.StatusBadRequest, "host and port are required")
			return
		}
	}

	sessionID := generateSessionID()

	cfg := core.Config{
		IP:          "tcp",
		Host:        strings.TrimSpace(req.Host),
		Port:        req.Port,
		Mountpoint:  strings.TrimSpace(req.Mountpoint),
		Username:    req.Username,
		Password:    req.Password,
		RateHz:      req.Rate,
		JitterVal:   jitterVal,
		JitterPct:   jitterPct,
		Decode:      req.Decode,
		Verbose:     0,
		SessionID:   sessionID,
		TimeoutExit: false,
	}

	switch mode {
	case "tcp_listen":
		cfg.IP = "tcp"
		cfg.Host = ""
		cfg.Mountpoint = ""
		cfg.Username = ""
		cfg.Password = ""
	case "udp_listen":
		cfg.IP = "udp"
		cfg.Host = ""
		cfg.Mountpoint = ""
		cfg.Username = ""
		cfg.Password = ""
	default:
		// outbound TCP/NTRIP/IBSS: cfg already uses tcp client + Host/Port/NTRIP fields
	}

	ctx, cancel := context.WithCancel(context.Background())
	packetChan := make(chan core.PacketEvent, 1000)
	broker := telemetry.NewSSEBroker()

	session := &Session{
		ID:     sessionID,
		Cancel: cancel,
		Broker: broker,
	}

	manager.mu.Lock()
	manager.sessions[sessionID] = session
	manager.mu.Unlock()

	switch mode {
	case "tcp_listen":
		slog.Info("Created new session", "session", sessionID, "mode", mode, "listen", "tcp", "port", req.Port)
	case "udp_listen":
		slog.Info("Created new session", "session", sessionID, "mode", mode, "listen", "udp", "port", req.Port)
	default:
		slog.Info("Created new session", "session", sessionID, "mode", mode, "target", req.Host)
	}

	switch mode {
	case "tcp_listen", "udp_listen":
		go func() {
			if err := stream.StartListenerContext(ctx, cfg, packetChan, nil); err != nil && ctx.Err() == nil {
				slog.Warn("listen session ended", "session", sessionID, "error", err)
			}
		}()
	default:
		// Outbound TCP/NTRIP/IBSS: StartNTRIPClient handles raw TCP when Mountpoint is empty.
		go stream.StartNTRIPClient(ctx, cfg, packetChan, broker.Notifier)
	}

	go timing.Run(ctx, cfg, packetChan, broker.Notifier)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"session_id": sessionID})
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	manager.mu.RLock()
	session, exists := manager.sessions[sessionID]
	manager.mu.RUnlock()

	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	defer func() {
		slog.Info("User disconnected, cleaning up session", "session", sessionID)
		session.Cancel()
		manager.mu.Lock()
		delete(manager.sessions, sessionID)
		manager.mu.Unlock()
	}()

	session.Broker.ServeHTTP(w, r)
}

func main() {
	port := flag.Int("port", 2102, "HTTP port to run the web server on")
	bindIP := flag.String("bind", "127.0.0.1", "IP to bind the server to (use 0.0.0.0 for public)")
	basePath := flag.String("base-path", "/", "Base URL path (e.g., '/jitter')")
	flag.Parse()

	path := *basePath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path && r.URL.Path != strings.TrimSuffix(path, "/") {
			http.NotFound(w, r)
			return
		}

		html := strings.ReplaceAll(string(web.IndexServerHTML), "{{VERSION}}", AppVersion)

		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Type", "text/html")

		w.Write([]byte(html))
	})

	http.HandleFunc(path+"chart.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(web.ChartJS)
	})

	http.HandleFunc(path+"api/start", handleStart)
	http.HandleFunc(path+"events", handleEvents)

	address := fmt.Sprintf("%s:%d", *bindIP, *port)
	slog.Info(fmt.Sprintf("Starting Timing Analyzer Web Server %s", AppVersion), "address", address, "base_path", path)

	ln, err := net.Listen("tcp", address)
	if err != nil {
		slog.Error("Listen failed", "error", err)
		return
	}

	srv := &http.Server{Handler: nil}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		slog.Error("Server crashed", "error", err)
	}
}
