package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/gsofbaseline"
	"timing-analyzer/internal/stream"
)

const maxBaselineSessionCreateBodyBytes = 8192

type baselineHub struct {
	mu       sync.Mutex
	sessions map[string]*baselineSession
}

type baselineSession struct {
	cancel context.CancelFunc
	broker *gsofbaseline.JSONBroker
}

type baselineStreamRequest struct {
	Transport string `json:"transport"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
}

type baselineCreateSessionRequest struct {
	Heading    baselineStreamRequest  `json:"heading"`
	MovingBase *baselineStreamRequest `json:"moving_base,omitempty"`
}

type baselineCreateSessionResponse struct {
	ID            string `json:"id"`
	DashboardPath string `json:"dashboard_path"`
}

type baselineConfigResponse struct {
	EmbeddedStream bool                 `json:"embedded_stream"`
	DefaultHeading baselineStreamRequest `json:"default_heading"`
	DefaultMovingBase baselineStreamRequest `json:"default_moving_base"`
}

func newBaselineHub() *baselineHub {
	return &baselineHub{sessions: make(map[string]*baselineSession)}
}

func newBaselineSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func streamReqToCfg(req baselineStreamRequest, verbose int, ignoreGap1 bool) core.Config {
	return streamCfg(req.Transport, req.Host, req.Port, verbose, ignoreGap1)
}

func (h *baselineHub) remove(id string) {
	h.mu.Lock()
	s, ok := h.sessions[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.sessions, id)
	h.mu.Unlock()
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func validateBaselineStream(req baselineStreamRequest) error {
	t := strings.ToLower(strings.TrimSpace(req.Transport))
	if t != "tcp" && t != "udp" {
		return fmt.Errorf("transport must be tcp or udp")
	}
	if t == "tcp" && strings.TrimSpace(req.Host) == "" {
		return fmt.Errorf("tcp host is required")
	}
	if t == "tcp" && (req.Port < 1 || req.Port > 65535) {
		return fmt.Errorf("tcp port must be 1-65535")
	}
	if t == "udp" && (req.Port < 0 || req.Port > 65535) {
		return fmt.Errorf("udp port must be 0-65535")
	}
	return nil
}

func baselineConnectHTML(defaultHeading, defaultMoving baselineStreamRequest) []byte {
	return []byte(fmt.Sprintf(`<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>GSOF Baseline Setup</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 820px; margin: 2rem auto; padding: 0 1rem; background: #0f1419; color: #e6edf3; }
    .card { border: 1px solid #2d3a4d; border-radius: 8px; padding: 1rem; margin-bottom: 1rem; background: #1a2332; }
    h1,h2 { margin-top: 0; }
    .row { display: flex; gap: 0.75rem; margin-bottom: 0.6rem; }
    .row > div { flex: 1; }
    label { display:block; margin-bottom: 0.2rem; color: #8b9cb3; font-size: 0.9rem; }
    input,select { width: 100%%; box-sizing: border-box; padding: 0.5rem; background: #0f1419; color: #e6edf3; border: 1px solid #2d3a4d; border-radius: 6px; }
    button { padding: 0.55rem 0.9rem; border-radius: 6px; border: 1px solid #2d3a4d; background: #3d8bfd; color: #fff; cursor: pointer; }
    #err { color: #f85149; min-height: 1.2rem; }
  </style>
</head>
<body>
  <h1>GSOF Baseline Hub</h1>
  <p>Configure stream sources, then open a session dashboard.</p>
  <div class="card">
    <h2>Heading stream</h2>
    <div class="row">
      <div><label>Transport</label><select id="hTransport"><option value="tcp">TCP</option><option value="udp">UDP</option></select></div>
      <div><label>Host (TCP)</label><input id="hHost" placeholder="e.g. 192.0.2.10" /></div>
      <div><label>Port</label><input id="hPort" type="number" min="0" max="65535" value="%d" /></div>
    </div>
  </div>
  <div class="card">
    <h2>Moving base stream (optional)</h2>
    <div class="row">
      <div><label><input id="mbEnable" type="checkbox" /> Enable moving base stream</label></div>
    </div>
    <div class="row">
      <div><label>Transport</label><select id="mbTransport"><option value="tcp">TCP</option><option value="udp">UDP</option></select></div>
      <div><label>Host (TCP)</label><input id="mbHost" placeholder="e.g. 192.0.2.20" /></div>
      <div><label>Port</label><input id="mbPort" type="number" min="0" max="65535" value="%d" /></div>
    </div>
  </div>
  <button id="connect">Connect</button>
  <p id="err"></p>
  <script>
    const defaults = %s;
    document.getElementById("hTransport").value = defaults.default_heading.transport || "udp";
    document.getElementById("hHost").value = defaults.default_heading.host || "";
    document.getElementById("mbTransport").value = defaults.default_moving_base.transport || "udp";
    document.getElementById("mbHost").value = defaults.default_moving_base.host || "";
    document.getElementById("mbEnable").checked = (defaults.default_moving_base.port || 0) > 0;
    function syncHost(idTransport, idHost) {
      const tr = document.getElementById(idTransport).value;
      document.getElementById(idHost).disabled = tr === "udp";
    }
    ["hTransport","mbTransport"].forEach((id, i) => {
      const host = i === 0 ? "hHost" : "mbHost";
      document.getElementById(id).addEventListener("change", () => syncHost(id, host));
      syncHost(id, host);
    });
    document.getElementById("connect").addEventListener("click", async () => {
      const err = document.getElementById("err");
      err.textContent = "";
      const body = {
        heading: {
          transport: document.getElementById("hTransport").value,
          host: (document.getElementById("hHost").value || "").trim(),
          port: parseInt(document.getElementById("hPort").value, 10),
        },
      };
      if (document.getElementById("mbEnable").checked) {
        body.moving_base = {
          transport: document.getElementById("mbTransport").value,
          host: (document.getElementById("mbHost").value || "").trim(),
          port: parseInt(document.getElementById("mbPort").value, 10),
        };
      }
      try {
        const res = await fetch("/api/sessions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        const text = await res.text();
        if (!res.ok) throw new Error(text || res.statusText);
        const out = JSON.parse(text);
        window.location.href = out.dashboard_path;
      } catch (e) {
        err.textContent = String(e.message || e);
      }
    });
  </script>
</body>
</html>`, defaultHeading.Port, defaultMoving.Port, mustJSON(baselineConfigResponse{
		DefaultHeading:    defaultHeading,
		DefaultMovingBase: defaultMoving,
	})))
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (h *baselineHub) startSession(req baselineCreateSessionRequest, cfg EngineSessionDefaults) (*baselineCreateSessionResponse, error) {
	if err := validateBaselineStream(req.Heading); err != nil {
		return nil, fmt.Errorf("heading: %w", err)
	}
	if req.MovingBase != nil {
		if err := validateBaselineStream(*req.MovingBase); err != nil {
			return nil, fmt.Errorf("moving base: %w", err)
		}
	}
	id, err := newBaselineSessionID()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())

	eng := gsofbaseline.NewEngine(gsofbaseline.EngineConfig{
		MatchMaxTowDeltaSec:  cfg.MatchMaxTowDeltaSec,
		RangeCheckTolM:       cfg.RangeCheckTolM,
		ExpectedRangeM:       cfg.ExpectedRangeM,
		MovingBaseConfigured: req.MovingBase != nil,
	})

	chHeading := make(chan core.PacketEvent, 2000)
	go func() {
		_ = stream.StartListenerContext(ctx, streamReqToCfg(req.Heading, cfg.Verbose, cfg.IgnoreGap1), chHeading, nil)
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-chHeading:
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					eng.IngestHeading(pkt.GSOFBuffer)
				}
			}
		}
	}()

	if req.MovingBase != nil {
		chMB := make(chan core.PacketEvent, 2000)
		go func() {
			_ = stream.StartListenerContext(ctx, streamReqToCfg(*req.MovingBase, cfg.Verbose, cfg.IgnoreGap1), chMB, nil)
		}()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case pkt := <-chMB:
					if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
						eng.IngestMovingBase(pkt.GSOFBuffer)
					}
				}
			}
		}()
	}

	broker := gsofbaseline.NewJSONBroker()
	h.mu.Lock()
	h.sessions[id] = &baselineSession{cancel: cancel, broker: broker}
	h.mu.Unlock()
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				snap := eng.Snapshot(Version)
				data, err := json.Marshal(snap)
				if err == nil {
					broker.Publish(data)
				}
			}
		}
	}()

	return &baselineCreateSessionResponse{
		ID:            id,
		DashboardPath: "/s/" + id,
	}, nil
}

type EngineSessionDefaults struct {
	Verbose            int
	IgnoreGap1         bool
	MatchMaxTowDeltaSec float64
	RangeCheckTolM     float64
	ExpectedRangeM     float64
}

func (h *baselineHub) handleConfig(w http.ResponseWriter, embedded bool, heading, mb baselineStreamRequest) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(baselineConfigResponse{
		EmbeddedStream:    embedded,
		DefaultHeading:    heading,
		DefaultMovingBase: mb,
	})
}

func (h *baselineHub) handleCreateSession(w http.ResponseWriter, r *http.Request, embedded bool, cfg EngineSessionDefaults) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if embedded {
		http.Error(w, "ui sessions disabled while embedded stream is active", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBaselineSessionCreateBodyBytes+1))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body) > maxBaselineSessionCreateBodyBytes {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var req baselineCreateSessionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	out, err := h.startSession(req, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}

func (h *baselineHub) handleEvents(w http.ResponseWriter, r *http.Request, embedded bool, embeddedBroker *gsofbaseline.JSONBroker) {
	if embedded {
		embeddedBroker.ServeHTTP(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/s/")
	id = strings.TrimSuffix(id, "/events")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *baselineHub) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.remove(id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *baselineHub) serveSessionBranch(w http.ResponseWriter, r *http.Request, dashboardHTML []byte, brokerForID func(string) *gsofbaseline.JSONBroker) {
	path := strings.TrimPrefix(r.URL.Path, "/s/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "events" {
		b := brokerForID(id)
		if b == nil {
			http.NotFound(w, r)
			return
		}
		b.ServeHTTP(w, r)
		return
	}
	if len(parts) == 1 {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-GSOF-Baseline-Version", Version)
		_, _ = w.Write(dashboardHTML)
		return
	}
	http.NotFound(w, r)
}

func (h *baselineHub) brokerForID(id string) *gsofbaseline.JSONBroker {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[id]
	if s == nil {
		return nil
	}
	return s.broker
}

func baselineDefaultStreamReq(c core.Config) baselineStreamRequest {
	return baselineStreamRequest{
		Transport: strings.ToLower(strings.TrimSpace(c.IP)),
		Host:      strings.TrimSpace(c.Host),
		Port:      c.Port,
	}
}

func parseSessionIDFromPath(path string) (string, bool) {
	if !strings.HasPrefix(path, "/s/") {
		return "", false
	}
	p := strings.TrimPrefix(path, "/s/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", false
	}
	if strings.HasSuffix(p, "/events") {
		p = strings.TrimSuffix(p, "/events")
	}
	if p == "" || strings.Contains(p, "/") {
		return "", false
	}
	return p, true
}

func parsePort(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}
