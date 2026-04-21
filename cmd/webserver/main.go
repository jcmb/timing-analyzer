package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
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
	Verbose    bool    `json:"verbose"`
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	jitterStr := strings.TrimSpace(req.Jitter)
	var jitterVal float64
	var jitterPct bool
	if strings.HasSuffix(jitterStr, "%") {
		jitterPct = true
		val, err := strconv.ParseFloat(strings.TrimSuffix(jitterStr, "%"), 64)
		if err != nil {
			http.Error(w, "Invalid jitter percentage", http.StatusBadRequest)
			return
		}
		jitterVal = val
	} else {
		val, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(jitterStr), "ms"), 64)
		if err != nil {
			http.Error(w, "Invalid jitter value", http.StatusBadRequest)
			return
		}
		jitterVal = val
	}

	sessionID := generateSessionID()

	// Map the UI Verbose toggle to the Core Config level
	verboseLevel := 0
	if req.Verbose {
		verboseLevel = 2
	}

	cfg := core.Config{
		IP:            "tcp",
		Host:          req.Host,
		Port:          req.Port,
		Mountpoint:    req.Mountpoint,
		Username:      req.Username,
		Password:      req.Password,
		RateHz:        req.Rate,
		JitterVal:     jitterVal,
		JitterPct:     jitterPct,
		Decode:        req.Decode,
		Verbose:       verboseLevel,
		SessionID:     sessionID,
		TimeoutExit:   false,
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

	slog.Info("Created new session", "session", sessionID, "mode", req.Mode, "target", req.Host)

	// Use StartNTRIPClient for all web-based TCP/NTRIP/IBSS connections.
	// It handles raw TCP automatically if Mountpoint is empty and supports Context cancellation.
	go stream.StartNTRIPClient(ctx, cfg, packetChan, broker.Notifier)

	go timing.Run(ctx, cfg, packetChan, broker.Notifier)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"session_id": sessionID})
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

	if err := http.ListenAndServe(address, nil); err != nil {
		slog.Error("Server crashed", "error", err)
	}
}
