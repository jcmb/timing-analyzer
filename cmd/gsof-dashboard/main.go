package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/gsof"
	"timing-analyzer/internal/gsofstats"
	"timing-analyzer/internal/stream"
)

//go:embed dashboard.html
var dashboardHTML []byte

func normalizeHTTPBasePath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "/" {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return strings.TrimSuffix(s, "/")
}

func prepareDashboardHTML(version, basePath string) []byte {
	prefixJSON, err := json.Marshal(basePath)
	if err != nil {
		prefixJSON = []byte(`""`)
	}
	out := bytes.ReplaceAll(dashboardHTML, []byte("__GSOF_DASHBOARD_VERSION__"), []byte(version))
	out = bytes.ReplaceAll(out, []byte("__GSOF_BASE_PATH_JSON__"), prefixJSON)
	return out
}

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Surrogate-Control", "no-store")
}

func dashboardBrowserURLHost(bind string) string {
	switch strings.TrimSpace(bind) {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.TrimSpace(bind)
	}
}

func dashboardAppURL(bind string, port int, httpBasePath string) string {
	u := "http://" + net.JoinHostPort(dashboardBrowserURLHost(bind), strconv.Itoa(port))
	if httpBasePath != "" {
		u += httpBasePath
	}
	return u + "/"
}

func launchDashboardBrowser(rawURL string) error {
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

func main() {
	ipFlag := flag.String("ip", "tcp", "tcp or udp (embedded CLI stream only; ignored when UI sessions own the stream)")
	udp := flag.Bool("udp", false, "Use UDP transport for the embedded stream/default UI session hint (default transport is TCP)")
	host := flag.String("host", "", "Optional host for a single embedded TCP stream (forces tcp). If empty and the web UI owns streams (-embedded-stream=false or -hub), users set host/port in the browser.")
	port := flag.Int("port", 5018, "Port for embedded stream or default suggested in the connection form when UI sessions are enabled")
	webHost := flag.String("web-host", "127.0.0.1", "HTTP listen address. 0.0.0.0 or :: defaults to per-browser streams unless -embedded-stream is set (shared hosting, e.g. trimbletools.com).")
	webPort := flag.Int("web-port", 8080, "HTTP port for the dashboard")
	openBrowserFlag := flag.Bool("open-browser", false, "Open the dashboard URL in the default browser after listen (default: on when -hub=false, off when -hub=true)")
	verbose := flag.Int("verbose", 0, "DCOL / stream verbosity: 0=off, 1=checksum/page warnings, 2=each GSOF sub-record payload as spaced hex (types 0x01/len/payload…), 3=full parser debug")
	showExpectedReserved := flag.Bool("show-expected-reserved-bits", false, "Include spec-cleared reserved flag rows (types 1/8/10) and the GSOF type 57 radio channel column; default hides them")
	embeddedStream := flag.Bool("embedded-stream", true, "Run one server-side GSOF transport from -ip/-host/-port. Omit or set false for multi-user mode: each browser starts its own session via the Web UI (see -hub).")
	hub := flag.Bool("hub", true, "Shorthand for shared hosting: -embedded-stream=false, -web-host=0.0.0.0 (each visitor configures TCP or UDP in the UI; use -advertise-host for public UDP)")
	maxUISessions := flag.Int("max-ui-sessions", 64, "Maximum concurrent UI-defined GSOF sessions (hub / -embedded-stream=false)")
	allowPrivateGSOF := flag.Bool("allow-private-gsof-targets", false, "In hub mode (-hub), allow UI/API TCP targets that resolve to loopback or RFC1918 (off by default). Non-hub runs always allow private targets.")
	advertiseHost := flag.String("advertise-host", "", "If set (e.g. trimbletools.com), UDP session API responses include this hostname so receivers can be aimed at the correct public address")
	ignoreTCPGSOFGap1 := flag.Bool("ignore-tcp-gsof-transmission-gap1", false, "TCP only: suppress Stats/parser warnings for a single skipped GSOF transmission id; applies to embedded streams and (with -embedded-stream=false) is merged into each browser-started session")
	httpBasePathFlag := flag.String("http-base-path", "", "Public URL path prefix when mounted under a site path (e.g. /GSOF). Same value is stripped from incoming HTTP paths and baked into the HTML for links/SSE. Empty = serve at site root. Env: GSOF_DASHBOARD_BASE_PATH.")
	flag.Parse()

	httpBasePath := normalizeHTTPBasePath(*httpBasePathFlag)
	if httpBasePath == "" {
		httpBasePath = normalizeHTTPBasePath(os.Getenv("GSOF_DASHBOARD_BASE_PATH"))
	}
	dashboardHTMLPrepared := prepareDashboardHTML(buildDisplayVersion(), httpBasePath)

	hubFlagExplicit := false
	openBrowserFlagSet := false
	allowPrivateFlagExplicit := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "hub":
			hubFlagExplicit = true
		case "open-browser":
			openBrowserFlagSet = true
		case "allow-private-gsof-targets":
			allowPrivateFlagExplicit = true
		}
	})
	// Windows/macOS: if -hub was not passed, assume a local desktop run (default hub=true):
	// open the browser and allow RFC1918/loopback TCP targets without an extra flag.
	desktopHubDefaults := (runtime.GOOS == "darwin" || runtime.GOOS == "windows") && !hubFlagExplicit

	openBrowser := *openBrowserFlag
	if !openBrowserFlagSet {
		openBrowser = !*hub
		if desktopHubDefaults {
			openBrowser = true
		}
	}

	// Hub/shared hosting: block private TCP targets unless -allow-private-gsof-targets.
	// Local (-hub=false): allow LAN / loopback targets without extra flags.
	var effectiveAllowPrivateGSOF bool
	if desktopHubDefaults && !allowPrivateFlagExplicit {
		effectiveAllowPrivateGSOF = true
	} else {
		effectiveAllowPrivateGSOF = !*hub || *allowPrivateGSOF
	}

	gsof.ShowExpectedReservedBits = *showExpectedReserved

	if *udp {
		*ipFlag = "udp"
	}

	embeddedStreamSet := false
	hubSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "embedded-stream" {
			embeddedStreamSet = true
		}
		if f.Name == "hub" {
			hubSet = true
		}
	})
	if *hub && (hubSet || !embeddedStreamSet) {
		*embeddedStream = false
		*webHost = "0.0.0.0"
	}
	wh := strings.TrimSpace(*webHost)
	if !*hub && !embeddedStreamSet && (wh == "0.0.0.0" || wh == "::") {
		// Bind-all without an explicit stream choice: prefer per-browser sessions so many users can share one process.
		*embeddedStream = false
	}

	cfg := core.Config{
		IP:                            strings.ToLower(strings.TrimSpace(*ipFlag)),
		Host:                          strings.TrimSpace(*host),
		Port:                          *port,
		Decode:                        "dcol",
		Verbose:                       *verbose,
		IgnoreTCPGSOFTransmissionGap1: *ignoreTCPGSOFGap1,
	}
	if cfg.Host != "" {
		cfg.IP = "tcp"
	}

	h := newHub(*maxUISessions)

	var embStats *gsofstats.Stats
	var embBroker *gsofstats.JSONBroker
	var packetChan chan core.PacketEvent

	if *embeddedStream {
		embStats = gsofstats.NewStats(false)
		embBroker = gsofstats.NewJSONBroker()
		packetChan = make(chan core.PacketEvent, 1000)
		var tcpInbound *gsofstats.TCPListenTracker
		if strings.EqualFold(cfg.IP, "tcp") && strings.TrimSpace(cfg.Host) == "" {
			tcpInbound = gsofstats.NewTCPListenTracker()
			embStats.SetTCPListenTracker(tcpInbound)
		}
		go func() {
			if err := stream.StartListenerContext(context.Background(), cfg, packetChan, nil, tcpInbound); err != nil {
				slog.Error("embedded stream listener failed", "error", err)
			}
		}()
		go func() {
			for pkt := range packetChan {
				for _, w := range pkt.StreamWarnings {
					embStats.AddWarning(w)
				}
				tcp := !strings.EqualFold(cfg.IP, "udp")
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					if tcpInbound != nil {
						tcpInbound.NotifyGSOF(pkt.RemoteAddr)
					}
					embStats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer, tcp, cfg.IgnoreTCPGSOFTransmissionGap1)
				}
			}
		}()
		go func() {
			t := time.NewTicker(500 * time.Millisecond)
			defer t.Stop()
			for range t.C {
				dash := embStats.BuildDashboard(cfg.IP, cfg.Port, Version, cfg.Host, true)
				data, err := json.Marshal(dash)
				if err != nil {
					slog.Warn("dashboard: JSON marshal failed (SSE not updated)", "error", err)
					continue
				}
				embBroker.Publish(data)
			}
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleAPIConfig(w, *embeddedStream, cfg, httpBasePath)
	})
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		h.handleAPICreateSession(w, r, *embeddedStream, *verbose, effectiveAllowPrivateGSOF, strings.TrimSpace(*advertiseHost), *ignoreTCPGSOFGap1, httpBasePath)
	})
	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		h.handleAPIDeleteSession(w, r, *embeddedStream)
	})
	mux.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) {
		h.serveSessionBranch(w, r, *embeddedStream, dashboardHTMLPrepared)
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("session"))
		if *embeddedStream {
			if q != "" {
				http.NotFound(w, r)
				return
			}
			embBroker.ServeHTTP(w, r)
			return
		}
		if q == "" {
			http.NotFound(w, r)
			return
		}
		s, ok := h.get(q)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.broker.ServeHTTP(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		setNoCacheHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-GSOF-Dashboard-Version", buildDisplayVersion())
		_, _ = w.Write(dashboardHTMLPrepared)
	})

	var handler http.Handler = mux
	if httpBasePath != "" {
		handler = http.StripPrefix(httpBasePath, mux)
	}

	webAddr := net.JoinHostPort(*webHost, strconv.Itoa(*webPort))
	srv := &http.Server{Addr: webAddr, Handler: handler}

	streamDesc := "none (UI sessions only)"
	if *embeddedStream {
		streamDesc = fmt.Sprintf("%s port %d", cfg.IP, cfg.Port)
		if cfg.Host != "" {
			streamDesc = fmt.Sprintf("tcp -> %s:%d", cfg.Host, cfg.Port)
		}
	}
	fmt.Fprintf(os.Stdout, "gsof-dashboard version %s\n  web UI:  http://%s\n  GSOF:    %s\n",
		buildDisplayVersion(), webAddr, streamDesc)
	if desktopHubDefaults {
		fmt.Fprintf(os.Stdout, "  note:    %s desktop — -hub omitted: opened browser (unless -open-browser=false) and private LAN TCP targets allowed (same as -allow-private-gsof-targets)\n", runtime.GOOS)
	}
	if httpBasePath != "" {
		fmt.Fprintf(os.Stdout, "  HTTP prefix: %s (incoming paths strip this; configure proxy to forward full path including prefix, or equivalent)\n", httpBasePath)
	}
	if !*embeddedStream {
		fmt.Fprintf(os.Stdout, "  mode:    multi-user (open http://%s%s/ and set TCP or UDP in the header)\n", webAddr, httpBasePath)
	}

	ln, err := net.Listen("tcp", webAddr)
	if err != nil {
		slog.Error("http listen failed", "addr", webAddr, "error", err)
		os.Exit(1)
	}

	if openBrowser {
		openURL := dashboardAppURL(*webHost, *webPort, httpBasePath)
		go func() {
			if err := launchDashboardBrowser(openURL); err != nil {
				slog.Warn("Could not open browser", "url", openURL, "error", err)
			} else {
				slog.Info("Opened browser", "url", openURL)
			}
		}()
	}

	go func() {
		slog.Info("GSOF dashboard listening", "version", buildDisplayVersion(), "addr", webAddr, "stream", streamDesc, "embedded_stream", *embeddedStream, "http_base_path", httpBasePath)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-ctx.Done()
	stop()
	shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
}
