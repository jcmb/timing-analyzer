package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/gsofstats"
	"timing-analyzer/internal/stream"
)

const maxSessionCreateBodyBytes = 4096

var errTooManySessions = errors.New("too many concurrent UI sessions")

type hub struct {
	mu          sync.Mutex
	sessions    map[string]*gsofSession
	maxSessions int
}

type gsofSession struct {
	id     string
	cancel context.CancelFunc
	broker *gsofstats.JSONBroker
}

func newHub(maxSessions int) *hub {
	if maxSessions < 1 {
		maxSessions = 1
	}
	return &hub{
		sessions:    make(map[string]*gsofSession),
		maxSessions: maxSessions,
	}
}

func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validateTCPStreamHost(host string, allowPrivate bool) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return fmt.Errorf("host is required for TCP")
	}
	if len(h) > 253 {
		return fmt.Errorf("host is too long")
	}
	if !allowPrivate {
		if ip := parseLiteralIP(h); ip != nil {
			if isDisallowedStreamIP(ip) {
				return fmt.Errorf("host is a private, loopback, or link-local address (use -allow-private-gsof-targets to allow)")
			}
			return nil
		}
		ips, err := net.LookupIP(h)
		if err != nil {
			return fmt.Errorf("lookup host: %w", err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("host resolved to no addresses")
		}
		for _, ip := range ips {
			if isDisallowedStreamIP(ip) {
				return fmt.Errorf("host resolves to a private, loopback, or link-local address (use -allow-private-gsof-targets to allow)")
			}
		}
	}
	return nil
}

func parseLiteralIP(h string) net.IP {
	s := strings.TrimSpace(h)
	if strings.Count(s, ":") >= 2 {
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	}
	return net.ParseIP(s)
}

func isDisallowedStreamIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}

func validateStreamRequest(cfg core.Config, allowPrivate bool) error {
	proto := strings.ToLower(strings.TrimSpace(cfg.IP))
	switch proto {
	case "tcp":
		if cfg.Port < 1 || cfg.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		return validateTCPStreamHost(cfg.Host, allowPrivate)
	case "udp":
		if cfg.Port < 0 || cfg.Port > 65535 {
			return fmt.Errorf("invalid UDP port")
		}
		return nil
	default:
		return fmt.Errorf("transport must be tcp or udp")
	}
}

type createSessionRequest struct {
	Transport string `json:"transport"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
}

type createSessionResponse struct {
	ID            string             `json:"id"`
	EventsPath    string             `json:"events_path"`
	DashboardPath string             `json:"dashboard_path"`
	Listen        *sessionListenDTO  `json:"listen,omitempty"`
	AdvertiseHost string             `json:"advertise_host,omitempty"`
}

type sessionListenDTO struct {
	Transport string `json:"transport"`
	Port      int    `json:"port"`
}

type configResponse struct {
	Version           string `json:"version"`
	EmbeddedStream    bool   `json:"embedded_stream"`
	UISessionsEnabled bool   `json:"ui_sessions_enabled"`
	// CLIStream* are optional hints to pre-fill the connection form when UISessionsEnabled.
	CLIStreamTransport string `json:"cli_stream_transport,omitempty"`
	CLIStreamHost      string `json:"cli_stream_host,omitempty"`
	CLIStreamPort      int    `json:"cli_stream_port,omitempty"`
}

func (h *hub) get(id string) (*gsofSession, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	return s, ok
}

func (h *hub) remove(id string) {
	h.mu.Lock()
	s, ok := h.sessions[id]
	if ok {
		delete(h.sessions, id)
	}
	h.mu.Unlock()
	if ok && s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (h *hub) startSession(parent context.Context, cfg core.Config, verbose int, allowPrivate bool, advertiseHost string) (*createSessionResponse, error) {
	if err := validateStreamRequest(cfg, allowPrivate); err != nil {
		return nil, err
	}

	h.mu.Lock()
	if len(h.sessions) >= h.maxSessions {
		h.mu.Unlock()
		return nil, fmt.Errorf("%w (limit %d)", errTooManySessions, h.maxSessions)
	}
	id, err := newSessionID()
	if err != nil {
		h.mu.Unlock()
		return nil, err
	}
	sctx, cancel := context.WithCancel(parent)
	stats := gsofstats.NewStats(false)
	broker := gsofstats.NewJSONBroker()
	ch := make(chan core.PacketEvent, 1000)
	s := &gsofSession{id: id, cancel: cancel, broker: broker}
	h.sessions[id] = s
	h.mu.Unlock()

	cfg.Verbose = verbose
	cfg.Decode = "dcol"

	udPortCh := make(chan int, 1)
	var udpCB func(int)
	if strings.EqualFold(cfg.IP, "udp") {
		udpCB = func(p int) {
			select {
			case udPortCh <- p:
			default:
			}
		}
	}

	go func() {
		if err := stream.StartListenerContext(sctx, cfg, ch, udpCB); err != nil {
			slog.Warn("session stream ended", "session", id, "error", err)
		}
	}()

	go func() {
		for {
			select {
			case <-sctx.Done():
				return
			case pkt := <-ch:
				for _, w := range pkt.StreamWarnings {
					stats.AddWarning(w)
				}
				tcp := !strings.EqualFold(cfg.IP, "udp")
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					stats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer, tcp)
				}
			}
		}
	}()

	if strings.EqualFold(cfg.IP, "udp") {
		select {
		case p := <-udPortCh:
			cfg.Port = p
		case <-time.After(3 * time.Second):
			h.remove(id)
			return nil, fmt.Errorf("timed out binding UDP socket")
		}
	}

	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-t.C:
				dash := stats.BuildDashboard(cfg.IP, cfg.Port, Version, cfg.Host)
				data, err := json.Marshal(dash)
				if err != nil {
					slog.Warn("session dashboard JSON marshal failed", "session", id, "error", err)
					continue
				}
				broker.Publish(data)
			}
		}
	}()

	out := &createSessionResponse{
		ID:            id,
		EventsPath:    "/s/" + id + "/events",
		DashboardPath: "/s/" + id + "/",
	}
	if advertiseHost != "" {
		out.AdvertiseHost = advertiseHost
	}
	if strings.EqualFold(cfg.IP, "udp") {
		out.Listen = &sessionListenDTO{Transport: "udp", Port: cfg.Port}
	}
	return out, nil
}

func (h *hub) handleAPIConfig(w http.ResponseWriter, embeddedStream bool, cfg core.Config) {
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	out := configResponse{
		Version:           Version,
		EmbeddedStream:    embeddedStream,
		UISessionsEnabled: !embeddedStream,
	}
	if !embeddedStream {
		proto := strings.ToLower(strings.TrimSpace(cfg.IP))
		if proto == "tcp" || proto == "udp" {
			out.CLIStreamTransport = proto
		}
		if h := strings.TrimSpace(cfg.Host); h != "" {
			out.CLIStreamHost = h
		}
		if cfg.Port > 0 || proto == "udp" {
			out.CLIStreamPort = cfg.Port
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (h *hub) handleAPICreateSession(w http.ResponseWriter, r *http.Request, embeddedStream bool, verbose int, allowPrivate bool, advertiseHost string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if embeddedStream {
		http.Error(w, "UI-defined streams are disabled while an embedded CLI stream is active (-embedded-stream=true). Use -embedded-stream=false or -hub for multi-user mode.", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSessionCreateBodyBytes+1))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body) > maxSessionCreateBodyBytes {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var req createSessionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	cfg := core.Config{Decode: "dcol"}
	switch strings.ToLower(strings.TrimSpace(req.Transport)) {
	case "tcp":
		cfg.IP = "tcp"
		cfg.Host = strings.TrimSpace(req.Host)
		cfg.Port = req.Port
	case "udp":
		cfg.IP = "udp"
		cfg.Host = ""
		cfg.Port = req.Port
	default:
		http.Error(w, `transport must be "tcp" or "udp"`, http.StatusBadRequest)
		return
	}

	// Session streams must outlive this HTTP request: r.Context() is cancelled as soon as the
	// POST response is sent, which would immediately tear down TCP dials and UDP listeners.
	resp, err := h.startSession(context.Background(), cfg, verbose, allowPrivate, advertiseHost)
	if err != nil {
		if errors.Is(err, errTooManySessions) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *hub) handleAPIDeleteSession(w http.ResponseWriter, r *http.Request, embeddedStream bool) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if embeddedStream {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	id = strings.TrimSuffix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if _, ok := h.get(id); !ok {
		http.NotFound(w, r)
		return
	}
	h.remove(id)
	setNoCacheHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

// serveSessionBranch handles GET /s/{id}/events (SSE) and GET /s/{id}/ or /s/{id} (HTML).
func (h *hub) serveSessionBranch(w http.ResponseWriter, r *http.Request, embeddedStream bool, dashboardHTML []byte) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if embeddedStream {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/s/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	id := parts[0]
	s, ok := h.get(id)
	if !ok {
		// Expired or unknown session: send users to the hub home (connection form) instead of a 404
		// when they refresh the dashboard page. SSE must stay 404 — EventSource cannot follow an
		// HTML redirect to / reliably.
		if len(parts) == 2 && parts[1] == "events" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if len(parts) == 2 && parts[1] == "events" {
		s.broker.ServeHTTP(w, r)
		return
	}
	if len(parts) == 1 {
		setNoCacheHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-GSOF-Dashboard-Version", Version)
		_, _ = w.Write(dashboardHTML)
		return
	}
	http.NotFound(w, r)
}
