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

// JSON format matching the frontend Setup form
type StartRequest struct {
	Host       string  `json:"host"`
	Port       int     `json:"port"`
	Mountpoint string  `json:"mountpoint"`
	Username   string  `json:"username"`
	Password   string  `json:"password"`
	Rate       float64 `json:"rate"`
	Jitter     string  `json:"jitter"`
	Decode     string  `json:"decode"`
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

	// Parse the jitter string (e.g. "10%" or "5ms")
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

	cfg := core.Config{
		IP:            "tcp", // Server is always an outgoing TCP/NTRIP client
		Host:          req.Host,
		Port:          req.Port,
		Mountpoint:    req.Mountpoint,
		Username:      req.Username,
		Password:      req.Password,
		RateHz:        req.Rate,
		JitterVal:     jitterVal,
		JitterPct:     jitterPct,
		Decode:        req.Decode,
		Verbose:       2,
		SessionID:     sessionID,
		TimeoutExit:   false, // Never crash the server on timeout, just log it and stop the session
	}

	// Create isolation sandbox for this specific web user
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

	slog.Info("Created new session", "session", sessionID, "target", req.Host)

	// Spin up isolated workers for this specific user
	// Note: We pass broker.Notifier into StartNTRIPClient so it can report 'fatal' connect errors
	go stream.StartNTRIPClient(ctx, cfg, packetChan, broker.Notifier)
	go timing.Run(ctx, cfg, packetChan, broker.Notifier)

	// Return the success payload to the browser so it can open the SSE socket
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

	// When ServeHTTP finishes (because the user closed the browser tab or clicked Stop),
	// we MUST clean up the server resources!
	defer func() {
		slog.Info("User disconnected, cleaning up session", "session", sessionID)
		session.Cancel() // This kills the network dialer and the timing engine goroutines
		manager.mu.Lock()
		delete(manager.sessions, sessionID) // Remove them from the active sessions map
		manager.mu.Unlock()
	}()

	// Hold the connection open and stream events to the browser
	session.Broker.ServeHTTP(w, r)
}

func main() {
	port := flag.Int("port", 8080, "HTTP port to run the web server on")
	basePath := flag.String("base-path", "/", "Base URL path (e.g., '/jitter')")
	flag.Parse()

	// Ensure the base path always starts and ends with a slash for clean routing
	path := *basePath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	// 1. Serve the Setup / Dashboard UI
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		// Only serve the UI on the exact base path, otherwise return 404
		if r.URL.Path != path && r.URL.Path != strings.TrimSuffix(path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(web.IndexServerHTML)
	})

	// 2. Serve the API Endpoint to start a stream
	http.HandleFunc(path+"api/start", handleStart)

	// 3. Serve the live Telemetry stream
	http.HandleFunc(path+"events", handleEvents)

	slog.Info("Starting TrimbleTools Multi-Tenant Server", "port", *port, "base_path", path)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		slog.Error("Server crashed", "error", err)
	}
}

