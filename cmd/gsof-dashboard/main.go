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
	"timing-analyzer/internal/gsof"
	"timing-analyzer/internal/gsofstats"
	"timing-analyzer/internal/stream"
)

//go:embed dashboard.html
var dashboardHTML []byte

var dashboardHTMLLive = func() []byte {
	return bytes.ReplaceAll(dashboardHTML, []byte("__GSOF_DASHBOARD_VERSION__"), []byte(Version))
}()

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Surrogate-Control", "no-store")
}

func main() {
	ipFlag := flag.String("ip", "udp", "tcp or udp (embedded CLI stream only; ignored when UI sessions own the stream)")
	host := flag.String("host", "", "Optional host for a single embedded TCP stream (forces tcp). If empty and the web UI owns streams (-embedded-stream=false or -hub), users set host/port in the browser.")
	port := flag.Int("port", 2101, "Port for embedded stream or default suggested in the connection form when UI sessions are enabled")
	webHost := flag.String("web-host", "127.0.0.1", "HTTP listen address. 0.0.0.0 or :: defaults to per-browser streams unless -embedded-stream is set (shared hosting, e.g. trimbletools.com).")
	webPort := flag.Int("web-port", 8080, "HTTP port for the dashboard")
	verbose := flag.Int("verbose", 0, "DCOL / stream verbosity: 0=off, 1=checksum/page warnings, 2=each GSOF sub-record payload as spaced hex (types 0x01/len/payload…), 3=full parser debug")
	showExpectedReserved := flag.Bool("show-expected-reserved-bits", false, "Include spec-cleared reserved flag rows (types 1/8/10) and the GSOF type 57 radio channel column; default hides them")
	embeddedStream := flag.Bool("embedded-stream", true, "Run one server-side GSOF transport from -ip/-host/-port. Omit or set false for multi-user mode: each browser starts its own session via the Web UI (see -hub).")
	hub := flag.Bool("hub", false, "Shorthand for shared hosting: -embedded-stream=false, -web-host=0.0.0.0 (each visitor configures TCP or UDP in the UI; use -advertise-host for public UDP)")
	maxUISessions := flag.Int("max-ui-sessions", 64, "Maximum concurrent UI-defined GSOF sessions (hub / -embedded-stream=false)")
	allowPrivateGSOF := flag.Bool("allow-private-gsof-targets", false, "Allow UI/API TCP targets that resolve to loopback or RFC1918 addresses (lab only)")
	advertiseHost := flag.String("advertise-host", "", "If set (e.g. trimbletools.com), UDP session API responses include this hostname so receivers can be aimed at the correct public address")
	ignoreTCPGSOFGap1 := flag.Bool("ignore-tcp-gsof-transmission-gap1", false, "TCP only: suppress Stats and parser warnings when exactly one GSOF transmission id is skipped between messages")
	flag.Parse()

	gsof.ShowExpectedReservedBits = *showExpectedReserved

	if *hub {
		*embeddedStream = false
		*webHost = "0.0.0.0"
	}

	embeddedStreamSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "embedded-stream" {
			embeddedStreamSet = true
		}
	})
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
		go stream.StartListener(cfg, packetChan)
		go func() {
			for pkt := range packetChan {
				for _, w := range pkt.StreamWarnings {
					embStats.AddWarning(w)
				}
				tcp := !strings.EqualFold(cfg.IP, "udp")
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					embStats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer, tcp, cfg.IgnoreTCPGSOFTransmissionGap1)
				}
			}
		}()
		go func() {
			t := time.NewTicker(500 * time.Millisecond)
			defer t.Stop()
			for range t.C {
				dash := embStats.BuildDashboard(cfg.IP, cfg.Port, Version, cfg.Host)
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
		h.handleAPIConfig(w, *embeddedStream, cfg)
	})
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		h.handleAPICreateSession(w, r, *embeddedStream, *verbose, *allowPrivateGSOF, strings.TrimSpace(*advertiseHost))
	})
	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		h.handleAPIDeleteSession(w, r, *embeddedStream)
	})
	mux.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) {
		h.serveSessionBranch(w, r, *embeddedStream, dashboardHTMLLive)
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if !*embeddedStream {
			http.NotFound(w, r)
			return
		}
		embBroker.ServeHTTP(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		setNoCacheHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-GSOF-Dashboard-Version", Version)
		_, _ = w.Write(dashboardHTMLLive)
	})

	webAddr := net.JoinHostPort(*webHost, strconv.Itoa(*webPort))
	srv := &http.Server{Addr: webAddr, Handler: mux}

	streamDesc := "none (UI sessions only)"
	if *embeddedStream {
		streamDesc = fmt.Sprintf("%s port %d", cfg.IP, cfg.Port)
		if cfg.Host != "" {
			streamDesc = fmt.Sprintf("tcp -> %s:%d", cfg.Host, cfg.Port)
		}
	}
	fmt.Fprintf(os.Stdout, "gsof-dashboard version %s\n  web UI:  http://%s\n  GSOF:    %s\n",
		Version, webAddr, streamDesc)
	if !*embeddedStream {
		fmt.Fprintf(os.Stdout, "  mode:    multi-user (open http://%s/ and set TCP or UDP in the header)\n", webAddr)
	}

	go func() {
		slog.Info("GSOF dashboard listening", "version", Version, "addr", webAddr, "stream", streamDesc, "embedded_stream", *embeddedStream)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
