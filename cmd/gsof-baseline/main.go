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
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/gsofbaseline"
	"timing-analyzer/internal/stream"
)

//go:embed ui.html
var uiHTML []byte

func baselineDashboardHTML(eventsPath, basePath string) []byte {
	prefixJSON, err := json.Marshal(basePath)
	if err != nil {
		prefixJSON = []byte(`""`)
	}
	html := bytes.ReplaceAll(uiHTML, []byte("__VERSION__"), []byte(buildDisplayVersion()))
	html = bytes.ReplaceAll(html, []byte("__BASE_PATH_JSON__"), prefixJSON)
	html = bytes.ReplaceAll(html, []byte("__EVENTS_PATH__"), []byte(publicPath(basePath, eventsPath)))
	html = bytes.ReplaceAll(html, []byte("__CHART_JS_PATH__"), []byte(publicPath(basePath, "/assets/chart.umd.min.js")))
	html = bytes.ReplaceAll(html, []byte("__HAMMER_JS_PATH__"), []byte(publicPath(basePath, "/assets/hammer.min.js")))
	html = bytes.ReplaceAll(html, []byte("__CHART_ZOOM_JS_PATH__"), []byte(publicPath(basePath, "/assets/chartjs-plugin-zoom.min.js")))
	return html
}

func streamCfg(ip, host string, port int, verbose int, ignoreGap1 bool) core.Config {
	transport := strings.ToLower(strings.TrimSpace(ip))
	cfg := core.Config{
		Host:                          strings.TrimSpace(host),
		Port:                          port,
		Decode:                        "dcol",
		Verbose:                       verbose,
		IgnoreTCPGSOFTransmissionGap1: ignoreGap1,
	}
	switch transport {
	case "udp":
		cfg.IP = "udp"
	default:
		cfg.IP = "tcp"
	}
	// Non-UDP with host set is outbound TCP dial (legacy default when -heading-ip was omitted).
	if cfg.Host != "" && !strings.EqualFold(cfg.IP, "udp") {
		cfg.IP = "tcp"
	}
	return cfg
}

func main() {
	headingIP := flag.String("heading-ip", "tcp", "Heading receiver: tcp or udp")
	headingHost := flag.String("heading-host", "172.27.0.14", "Heading receiver: TCP dial host (optional)")
	headingPort := flag.Int("heading-port", 6000, "Heading receiver: UDP listen or TCP port")

	mbIP := flag.String("moving-base-ip", "tcp", "Moving base: tcp or udp (ignored when -moving-base-port is 0)")
	mbHost := flag.String("moving-base-host", "172.27.0.14", "Moving base: TCP dial host (optional)")
	mbPort := flag.Int("moving-base-port", 6001, "Moving base: UDP listen or TCP port; 0 = disabled (not required when GSOF type 41 is on the heading stream)")

	matchMax := flag.Float64("match-max-tow-delta-sec", 0.25, "Max GPS TOW gap (s, week-wrapped) between heading epoch and reference (type 41 or moving-base type 1)")
	rangeTol := flag.Float64("range-check-tolerance", 0.01, "Metres: pass if |computed slant − reference| is at most this (0 disables range check)")
	expectedRange := flag.Float64("expected-range", 0, "Metres: fixed reference slant range; when > 0 overrides GSOF type-27. When 0, range check uses type-27 range from the heading stream when present (same TOW as type 1)")

	webHost := flag.String("web-host", "127.0.0.1", "HTTP listen address")
	webPort := flag.Int("web-port", 8091, "HTTP port for UI and /events SSE")
	embeddedStream := flag.Bool("embedded-stream", true, "Run one embedded heading stream (and optional moving base) from CLI flags; set false for per-browser session setup in the web UI")
	hub := flag.Bool("hub", true, "Shorthand for hub mode: -embedded-stream=false and -web-host=0.0.0.0 (configure streams from browser)")
	verbose := flag.Int("verbose", 0, "DCOL verbosity (same as gsof-dashboard)")
	ignoreGap1 := flag.Bool("ignore-tcp-gsof-transmission-gap1", false, "TCP: suppress warnings for a single skipped GSOF transmission id")
	basePathFlag := flag.String("base-path", "", "Public URL path prefix when mounted under a site path (e.g. /baseline). Baked into HTML for links/SSE and stripped from incoming HTTP paths. Must match proxy X-Forwarded-Prefix. Env: GSOF_BASELINE_BASE_PATH.")
	flag.Parse()

	httpBasePath := resolveHTTPBasePath(*basePathFlag, os.Getenv("GSOF_BASELINE_BASE_PATH"))

	tol := *rangeTol
	if *expectedRange > 0 && tol <= 0 {
		tol = 0.01
	}

	embeddedStreamSet := false
	hubSet := false
	cliHeadingSet := false
	cliMBSet := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "embedded-stream":
			embeddedStreamSet = true
		case "hub":
			hubSet = true
		case "heading-host", "heading-ip", "heading-port":
			cliHeadingSet = true
		case "moving-base-host", "moving-base-ip", "moving-base-port":
			cliMBSet = true
		}
	})
	if *hub && (hubSet || !embeddedStreamSet) {
		*embeddedStream = false
		*webHost = "0.0.0.0"
	}

	mbEnabled := *mbPort != 0
	cfgHeading := streamCfg(*headingIP, *headingHost, *headingPort, *verbose, *ignoreGap1)
	var cfgMB core.Config
	if mbEnabled {
		cfgMB = streamCfg(*mbIP, *mbHost, *mbPort, *verbose, *ignoreGap1)
	}

	addr := net.JoinHostPort(*webHost, strconv.Itoa(*webPort))
	if !*embeddedStream {
		h := newBaselineHub(httpBasePath)
		mux := http.NewServeMux()
		defaultMB := baselineStreamRequest{Transport: "udp", Port: 0}
		if mbEnabled {
			defaultMB = baselineDefaultStreamReq(cfgMB)
		}
		defaultHeading := baselineDefaultStreamReq(cfgHeading)
		lockConnectDefaults := cliHeadingSet && cliMBSet && mbEnabled
		hubCfg := baselineConfigResponse{
			DefaultHeading:      defaultHeading,
			DefaultMovingBase:   defaultMB,
			LockSavedConnection: lockConnectDefaults,
		}
		mux.HandleFunc("/assets/chart.umd.min.js", serveBaselineChartJS)
		mux.HandleFunc("/assets/hammer.min.js", serveBaselineHammerJS)
		mux.HandleFunc("/assets/chartjs-plugin-zoom.min.js", serveBaselineChartZoomJS)
		mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.handleConfig(w, false, hubCfg)
		})
		mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
			h.handleCreateSession(w, r, false, EngineSessionDefaults{
				Verbose:             *verbose,
				IgnoreGap1:          *ignoreGap1,
				MatchMaxTowDeltaSec: *matchMax,
				RangeCheckTolM:      tol,
				ExpectedRangeM:      *expectedRange,
			})
		})
		mux.HandleFunc("/api/sessions/", h.handleSessionDelete)
		mux.HandleFunc("/events", h.handleHubEvents)
		mux.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) {
			h.serveSessionBranch(w, r)
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(baselineConnectHTML(hubCfg, httpBasePath))
		})
		srv := &http.Server{Addr: addr, Handler: stripHTTPBasePath(httpBasePath, mux)}
		appURL := "http://" + net.JoinHostPort(*webHost, strconv.Itoa(*webPort))
		if httpBasePath != "" {
			appURL += httpBasePath
		}
		fmt.Fprintf(os.Stdout, "gsof-baseline version %s\n  web UI:  %s/\n  mode:    hub (configure streams in browser)\n", buildDisplayVersion(), appURL)
		if httpBasePath != "" {
			fmt.Fprintf(os.Stdout, "  HTTP prefix: %s (must match proxy X-Forwarded-Prefix; forward full path including prefix)\n", httpBasePath)
		}
		if lockConnectDefaults {
			fmt.Fprintf(os.Stdout, "  connect form: CLI heading + moving-base defaults (browser saved settings ignored)\n")
		}
		slog.Info("gsof-baseline hub listening", "addr", addr, "base_path", httpBasePath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http", "error", err)
			os.Exit(1)
		}
		return
	}

	eng := gsofbaseline.NewEngine(gsofbaseline.EngineConfig{
		MatchMaxTowDeltaSec:    *matchMax,
		RangeCheckTolM:         tol,
		ExpectedRangeM:         *expectedRange,
		MovingBaseConfigured:   mbEnabled,
		HeadingStream:          gsofbaseline.StreamEndpointFromConfig(cfgHeading),
		MovingBaseStream:       gsofbaseline.StreamEndpointFromConfig(cfgMB),
	})

	chHeading := make(chan core.PacketEvent, 2000)
	var chMB chan core.PacketEvent
	if mbEnabled {
		chMB = make(chan core.PacketEvent, 2000)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := stream.StartListenerContext(ctx, cfgHeading, chHeading, nil, nil)
		if err != nil && ctx.Err() == nil {
			slog.Error("heading listener", "error", err)
			os.Exit(1)
		}
	}()
	if mbEnabled {
		go func() {
			err := stream.StartListenerContext(ctx, cfgMB, chMB, nil, nil)
			if err != nil && ctx.Err() == nil {
				slog.Error("moving-base listener", "error", err)
				os.Exit(1)
			}
		}()
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-chHeading:
				for _, w := range pkt.StreamWarnings {
					slog.Debug("heading warn", "msg", w)
				}
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					eng.NoteHeadingRemoteAddr(pkt.RemoteAddr)
					eng.IngestHeading(pkt.GSOFBuffer)
				}
			}
		}
	}()
	if mbEnabled {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case pkt := <-chMB:
					for _, w := range pkt.StreamWarnings {
						slog.Debug("moving-base warn", "msg", w)
					}
					if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
						eng.NoteMovingBaseRemoteAddr(pkt.RemoteAddr)
						eng.IngestMovingBase(pkt.GSOFBuffer)
					}
				}
			}
		}()
	}

	broker := gsofbaseline.NewJSONBroker()
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				snap := eng.Snapshot(buildDisplayVersion())
				data, err := json.Marshal(snap)
				if err != nil {
					slog.Warn("json marshal", "error", err)
					continue
				}
				broker.Publish(data)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/assets/chart.umd.min.js", serveBaselineChartJS)
	mux.HandleFunc("/assets/hammer.min.js", serveBaselineHammerJS)
	mux.HandleFunc("/assets/chartjs-plugin-zoom.min.js", serveBaselineChartZoomJS)
	mux.HandleFunc("/events", broker.ServeHTTP)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-GSOF-Baseline-Version", buildDisplayVersion())
		_, _ = w.Write(baselineDashboardHTML("/events", httpBasePath))
	})

	srv := &http.Server{Addr: addr, Handler: stripHTTPBasePath(httpBasePath, mux)}

	fmt.Fprintf(os.Stdout, "gsof-baseline version %s\n  web UI:  http://%s\n  heading: %s\n",
		buildDisplayVersion(), addr, describeStream(cfgHeading))
	if mbEnabled {
		fmt.Fprintf(os.Stdout, "  moving base: %s\n", describeStream(cfgMB))
	} else {
		fmt.Fprintf(os.Stdout, "  moving base: (disabled — enable with -moving-base-port, or use GSOF type 41 on heading stream)\n")
	}
	if tol > 0 {
		ref := "GSOF type-27 (heading stream, same TOW as type 1 when available)"
		if *expectedRange > 0 {
			ref = fmt.Sprintf("fixed expected %.3f m", *expectedRange)
		}
		fmt.Fprintf(os.Stdout, "  range check: tolerance %.3f m · reference: %s\n", tol, ref)
	}

	go func() {
		slog.Info("gsof-baseline listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http", "error", err)
			os.Exit(1)
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-sigCtx.Done()
	stop()
	cancel()
	shCtx, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	_ = srv.Shutdown(shCtx)
}

func describeStream(c core.Config) string {
	if strings.EqualFold(c.IP, "udp") {
		return fmt.Sprintf("udp :%d", c.Port)
	}
	if c.Host != "" {
		return fmt.Sprintf("tcp -> %s:%d", c.Host, c.Port)
	}
	return fmt.Sprintf("tcp listen :%d", c.Port)
}
