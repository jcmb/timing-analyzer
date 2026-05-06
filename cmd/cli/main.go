package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/stream"
	"timing-analyzer/internal/telemetry"
	"timing-analyzer/internal/timing"
	"timing-analyzer/web"
)


const AppVersion = "v1.1.0"
const maxCLISessionBodyBytes = 8192

type cliSession struct {
	cancel context.CancelFunc
	broker *telemetry.SSEBroker
}

type cliHub struct {
	mu       sync.Mutex
	sessions map[string]*cliSession
}

type cliSessionReq struct {
	Transport string  `json:"transport"`
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	Rate      float64 `json:"rate"`
	Jitter    string  `json:"jitter"`
	Decode    string  `json:"decode"`
}

type cliSessionResp struct {
	ID            string `json:"id"`
	DashboardPath string `json:"dashboard_path"`
}

type cliConfigResp struct {
	EmbeddedStream bool    `json:"embedded_stream"`
	Transport      string  `json:"transport"`
	Host           string  `json:"host"`
	Port           int     `json:"port"`
	Rate           float64 `json:"rate"`
	Jitter         string  `json:"jitter"`
	Decode         string  `json:"decode"`
}

func newCLIHub() *cliHub {
	return &cliHub{sessions: make(map[string]*cliSession)}
}

func newCLISessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func parseJitter(v string) (float64, bool, error) {
	jitterStr := strings.TrimSpace(v)
	if strings.HasSuffix(jitterStr, "%") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(jitterStr, "%"), 64)
		return val, true, err
	}
	val, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(jitterStr), "ms"), 64)
	return val, false, err
}

// hubFlag implements flag.Value with strict parsing so typos like -hub=falsse fail instead of being ignored.
type hubFlag struct {
	v bool
}

func (h *hubFlag) String() string {
	if h.v {
		return "true"
	}
	return "false"
}

func (h *hubFlag) Set(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "t", "yes", "y", "on":
		h.v = true
		return nil
	case "false", "0", "f", "no", "n", "off":
		h.v = false
		return nil
	default:
		return fmt.Errorf("invalid value %q for -hub (use true or false, e.g. -hub=false)", s)
	}
}

func (h *hubFlag) IsBoolFlag() bool { return true }

func browserURLHost(bind string) string {
	switch bind {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return bind
	}
}

func cliWebAppURL(bind string, port int) string {
	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(browserURLHost(bind), strconv.Itoa(port)),
		Path:   "/",
	}
	return u.String()
}

func launchBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

// cliLaunchJSONPlaceholder is replaced once with JSON for the dashboard banner (see web/index.html).
const cliLaunchJSONPlaceholder = "__TA_CLI_LAUNCH_JSON__"

func embedCLIIntoIndexHTML(cfg core.Config, argv []string, jitterDisplay string, showCLIBanner bool) []byte {
	if !showCLIBanner {
		return bytes.Replace(web.IndexHTML, []byte(cliLaunchJSONPlaceholder), []byte("null"), 1)
	}
	payload := struct {
		Argv      string  `json:"argv"`
		Transport string  `json:"transport"`
		Host      string  `json:"host"`
		Port      int     `json:"port"`
		Rate      float64 `json:"rate"`
		Jitter    string  `json:"jitter"`
		Decode    string  `json:"decode"`
		Verbose   int     `json:"verbose"`
		Warmup    int     `json:"warmup"`
	}{
		Argv:      strings.Join(argv, " "),
		Transport: cfg.IP,
		Host:      cfg.Host,
		Port:      cfg.Port,
		Rate:      cfg.RateHz,
		Jitter:    jitterDisplay,
		Decode:    cfg.Decode,
		Verbose:   cfg.Verbose,
		Warmup:    cfg.WarmupPackets,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return web.IndexHTML
	}
	out := bytes.Replace(web.IndexHTML, []byte(cliLaunchJSONPlaceholder), raw, 1)
	return out
}

func cliConnectHTML(cfg cliConfigResp) []byte {
	b, _ := json.Marshal(cfg)
	return []byte(fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>Timing Analyzer Setup</title>
<style>
body{font-family:system-ui,sans-serif;max-width:760px;margin:2rem auto;padding:0 1rem;background:#121212;color:#e0e0e0}
.card{background:#1e1e1e;border:1px solid #333;border-radius:8px;padding:1rem}
.row{display:flex;gap:.6rem;margin-bottom:.6rem}.row>div{flex:1}
label{display:block;color:#aaa;font-size:.9rem;margin-bottom:.2rem}
input,select{width:100%%;box-sizing:border-box;background:#222;color:#e0e0e0;border:1px solid #444;border-radius:6px;padding:.5rem}
button{padding:.55rem .95rem;border-radius:6px;border:1px solid #444;background:#4CAF50;color:#fff;cursor:pointer}
#err{color:#F44336;min-height:1.2rem}
</style></head><body>
<h1>Timing Analyzer Hub</h1>
<p>Configure stream parameters and open a session dashboard.</p>
<div class="card">
  <div class="row">
    <div><label>Transport</label><select id="transport"><option value="tcp">TCP</option><option value="udp">UDP</option></select></div>
    <div><label>Host (TCP)</label><input id="host" placeholder="e.g. 192.0.2.10"></div>
    <div><label>Port</label><input id="port" type="number" min="0" max="65535"></div>
  </div>
  <div class="row">
    <div><label>Rate Hz</label><input id="rate" type="number" step="0.1"></div>
    <div><label>Jitter</label><input id="jitter" placeholder="e.g. 10%% or 5ms"></div>
    <div><label>Decode</label><select id="decode"><option value="none">none</option><option value="dcol">dcol</option><option value="mb-cmr">mb-cmr</option></select></div>
  </div>
  <button id="connect">Connect</button>
  <p id="err"></p>
</div>
<script>
const cfg=%s;
const LS_KEY="timing-analyzer-cli-hub-form";
transport.value=cfg.transport||"tcp";host.value=cfg.host||"";port.value=String(cfg.port||2101);
rate.value=String(cfg.rate||1.0);jitter.value=cfg.jitter||"10%%";decode.value=cfg.decode||"none";
function loadSavedForm(){
  try{
    const raw=localStorage.getItem(LS_KEY);
    if(!raw) return;
    const o=JSON.parse(raw);
    if(o.transport==="tcp"||o.transport==="udp") transport.value=o.transport;
    if(typeof o.host==="string") host.value=o.host;
    if(o.port!==undefined&&o.port!==null&&String(o.port)!=="") port.value=String(o.port);
    if(o.rate!==undefined&&o.rate!==null&&String(o.rate)!=="") rate.value=String(o.rate);
    if(typeof o.jitter==="string"&&o.jitter) jitter.value=o.jitter;
    if(o.decode==="none"||o.decode==="dcol"||o.decode==="mb-cmr") decode.value=o.decode;
  }catch(e){}
}
function saveForm(){
  try{
    localStorage.setItem(LS_KEY,JSON.stringify({
      transport:transport.value,
      host:host.value,
      port:port.value,
      rate:rate.value,
      jitter:jitter.value,
      decode:decode.value
    }));
  }catch(e){}
}
loadSavedForm();
function syncHost(){host.disabled=(transport.value==="udp");}
transport.addEventListener("change",syncHost);syncHost();
connect.addEventListener("click",async()=>{err.textContent="";
  const body={transport:transport.value,host:(host.value||"").trim(),port:parseInt(port.value,10),rate:parseFloat(rate.value),jitter:jitter.value,decode:decode.value};
  try{const r=await fetch("/api/sessions",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
    const t=await r.text();if(!r.ok) throw new Error(t||r.statusText); const out=JSON.parse(t); saveForm(); window.location.href=out.dashboard_path;
  }catch(e){err.textContent=String(e.message||e);}
});
</script></body></html>`, string(b)))
}

func main() {
	ipFlag := flag.String("ip", "tcp", "tcp or udp")
	udp := flag.Bool("udp", false, "Use UDP transport (default transport is TCP)")
	host := flag.String("host", "", "Optional host IP to connect to (implicitly forces tcp mode)")
	port := flag.Int("port", 2101, "Port to listen on or connect to")
	webPort := flag.Int("web-port", 8080, "Port for the live web dashboard")
	webHost := flag.String("web-host", "127.0.0.1", "HTTP listen address")
	embeddedStream := flag.Bool("embedded-stream", true, "Run one embedded stream from CLI flags; set false for per-browser sessions")
	hub := hubFlag{v: true}
	flag.Var(&hub, "hub", "Shorthand for hub mode: -embedded-stream=false and -web-host=0.0.0.0 (true|false; use -hub=false to disable)")
	rate := flag.Float64("rate", 1.0, "Expected update rate in Hz")
	jitterFlag := flag.String("jitter", "10%", "Allowable jitter (e.g. '5ms' or '10%')")
	timeoutExit := flag.Bool("timeout-exit", true, "Exit with error if no data in 100 epochs")
	verbose := flag.Int("verbose", 0, "Verbosity level (1=warmup, 2=all packets, 3=parser debug)")
	warmup := flag.Int("warmup", 0, "Number of initial packets to ignore (0 to disable)")
	decode := flag.String("decode", "none", "Protocol decoder: 'none', 'dcol', or 'mb-cmr'")
	csvFile := flag.String("csv", "", "Output filename for CSV logging")
	flag.Parse()
	if extras := flag.Args(); len(extras) > 0 {
		slog.Error("unexpected command-line arguments after flags (check -hub spelling; use -hub=false)", "args", extras)
		os.Exit(2)
	}

	fmt.Printf("Starting Timing Analyzer CLI %s\n", AppVersion)

	if *udp {
		*ipFlag = "udp"
	}
	var embeddedStreamSet, hubSet, udpSet, portSet, hostSet, ipSet bool
	cliFlagCount := 0
	flag.Visit(func(f *flag.Flag) {
		cliFlagCount++
		switch f.Name {
		case "embedded-stream":
			embeddedStreamSet = true
		case "hub":
			hubSet = true
		case "udp":
			udpSet = true
		case "port":
			portSet = true
		case "host":
			hostSet = true
		case "ip":
			ipSet = true
		}
	})
	streamFlagsFromCLI := udpSet || portSet || hostSet || ipSet
	// Prefer embedded (single stream from flags) when the user set transport/endpoint flags,
	// unless they explicitly opted into hub mode with -hub.
	forceHub := hub.v && (hubSet || (!embeddedStreamSet && !streamFlagsFromCLI))
	if forceHub {
		*embeddedStream = false
		*webHost = "0.0.0.0"
	}
	// Browser auto-open follows -hub only; -hub=false never opens a window (even with -udp/-port/etc.).
	autoOpenBrowser := hub.v

	if *host != "" {
		*ipFlag = "tcp"
	}

	csvFilename := *csvFile
	if csvFilename != "" && !strings.HasSuffix(strings.ToLower(csvFilename), ".csv") {
		csvFilename += ".csv"
	}

	jitterVal, jitterPct, err := parseJitter(*jitterFlag)
	if err != nil {
		slog.Error("Invalid jitter value", "error", err)
		os.Exit(1)
	}

	cfg := core.Config{
		IP:            *ipFlag,
		Host:          *host,
		Port:          *port,
		WebPort:       *webPort,
		RateHz:        *rate,
		JitterVal:     jitterVal,
		JitterPct:     jitterPct,
		TimeoutExit:   *timeoutExit,
		Verbose:       *verbose,
		WarmupPackets: *warmup,
		Decode:        *decode,
		CSVFile:       csvFilename,
	}
	cfg.IP = strings.ToLower(strings.TrimSpace(cfg.IP))
	cfg.Host = strings.TrimSpace(cfg.Host)

	indexHTMLBody := embedCLIIntoIndexHTML(cfg, os.Args, *jitterFlag, cliFlagCount > 0)

	if !*embeddedStream {
		h := newCLIHub()
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/s/") {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(cliConnectHTML(cliConfigResp{
					EmbeddedStream: false,
					Transport:      cfg.IP,
					Host:           cfg.Host,
					Port:           cfg.Port,
					Rate:           cfg.RateHz,
					Jitter:         *jitterFlag,
					Decode:         cfg.Decode,
				}))
				return
			}
			id := strings.TrimPrefix(r.URL.Path, "/s/")
			id = strings.TrimSuffix(id, "/")
			if strings.HasSuffix(id, "/events") {
				id = strings.TrimSuffix(id, "/events")
				h.mu.Lock()
				s := h.sessions[id]
				h.mu.Unlock()
				if s == nil {
					http.NotFound(w, r)
					return
				}
				s.broker.ServeHTTP(w, r)
				return
			}
			h.mu.Lock()
			_, ok := h.sessions[id]
			h.mu.Unlock()
			if !ok {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(indexHTMLBody)
		})
		mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(cliConfigResp{
				EmbeddedStream: false,
				Transport:      cfg.IP,
				Host:           cfg.Host,
				Port:           cfg.Port,
				Rate:           cfg.RateHz,
				Jitter:         *jitterFlag,
				Decode:         cfg.Decode,
			})
		})
		mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, maxCLISessionBodyBytes+1))
			if err != nil {
				http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
				return
			}
			if len(body) > maxCLISessionBodyBytes {
				http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
				return
			}
			var req cliSessionReq
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
			if req.Transport != "tcp" && req.Transport != "udp" {
				http.Error(w, `transport must be "tcp" or "udp"`, http.StatusBadRequest)
				return
			}
			if req.Transport == "tcp" && strings.TrimSpace(req.Host) == "" {
				http.Error(w, "host is required for tcp", http.StatusBadRequest)
				return
			}
			if req.Port < 0 || req.Port > 65535 {
				http.Error(w, "port must be 0-65535", http.StatusBadRequest)
				return
			}
			jv, jp, err := parseJitter(req.Jitter)
			if err != nil {
				http.Error(w, "invalid jitter: "+err.Error(), http.StatusBadRequest)
				return
			}
			id, err := newCLISessionID()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ctx, cancel := context.WithCancel(context.Background())
			broker := telemetry.NewSSEBroker()
			sessCfg := cfg
			sessCfg.IP = req.Transport
			sessCfg.Host = strings.TrimSpace(req.Host)
			sessCfg.Port = req.Port
			sessCfg.RateHz = req.Rate
			sessCfg.JitterVal = jv
			sessCfg.JitterPct = jp
			sessCfg.Decode = req.Decode
			sessCfg.SessionID = id
			sessCfg.TimeoutExit = false
			packetChan := make(chan core.PacketEvent, 1000)
			go func() {
				_ = stream.StartListenerContext(ctx, sessCfg, packetChan, nil)
			}()
			go timing.Run(ctx, sessCfg, packetChan, broker.Notifier)
			h.mu.Lock()
			h.sessions[id] = &cliSession{cancel: cancel, broker: broker}
			h.mu.Unlock()
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(cliSessionResp{ID: id, DashboardPath: "/s/" + id + "/"})
		})
		mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
			id = strings.TrimSuffix(id, "/")
			h.mu.Lock()
			s := h.sessions[id]
			delete(h.sessions, id)
			h.mu.Unlock()
			if s != nil && s.cancel != nil {
				s.cancel()
			}
			w.WriteHeader(http.StatusNoContent)
		})
		webAddr := net.JoinHostPort(*webHost, strconv.Itoa(cfg.WebPort))
		slog.Info("Starting Timing Analyzer Hub", "addr", webAddr)
		ln, err := net.Listen("tcp", webAddr)
		if err != nil {
			slog.Error("Listen failed", "addr", webAddr, "error", err)
			os.Exit(1)
		}
		if autoOpenBrowser {
			openURL := cliWebAppURL(*webHost, cfg.WebPort)
			go func() {
				if err := launchBrowser(openURL); err != nil {
					slog.Warn("Could not open browser", "url", openURL, "error", err)
				} else {
					slog.Info("Opened browser", "url", openURL)
				}
			}()
		}
		srv := &http.Server{Handler: mux}
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("Web server failed", "error", err)
		}
		return
	}

	if streamFlagsFromCLI {
		slog.Info("Embedded stream from CLI flags", "transport", cfg.IP, "host", cfg.Host, "port", cfg.Port,
			"rate_hz", cfg.RateHz, "jitter", *jitterFlag, "decode", cfg.Decode)
		fmt.Fprintln(os.Stdout, strings.Join(os.Args, " "))
		fmt.Fprintf(os.Stdout, "Web dashboard: %s\n", cliWebAppURL(*webHost, cfg.WebPort))
	}

	// 1. Setup the UI Web Server
	broker := telemetry.NewSSEBroker()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexHTMLBody)
	})
	mux.Handle("/events", broker)
	webAddr := net.JoinHostPort(*webHost, strconv.Itoa(cfg.WebPort))
	slog.Info("Starting Web Dashboard", "addr", webAddr)
	ln, err := net.Listen("tcp", webAddr)
	if err != nil {
		slog.Error("Listen failed", "addr", webAddr, "error", err)
		os.Exit(1)
	}
	if autoOpenBrowser {
		openURL := cliWebAppURL(*webHost, cfg.WebPort)
		go func() {
			if err := launchBrowser(openURL); err != nil {
				slog.Warn("Could not open browser", "url", openURL, "error", err)
			} else {
				slog.Info("Opened browser", "url", openURL)
			}
		}()
	}
	go func() {
		srv := &http.Server{Handler: mux}
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("Web server failed", "error", err)
		}
	}()

	// 2. Wire the plumbing
	packetChan := make(chan core.PacketEvent, 1000)

	// 3. Start the components
	go stream.StartListener(cfg, packetChan)
	timing.Run(context.Background(), cfg, packetChan, broker.Notifier)
}

