package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	// Hint shared caches / reverse proxies not to treat this as cacheable.
	w.Header().Set("Surrogate-Control", "no-store")
}

func main() {
	ipFlag := flag.String("ip", "udp", "tcp or udp")
	host := flag.String("host", "", "Optional host to connect to (forces tcp)")
	port := flag.Int("port", 2101, "Port to listen on or connect to")
	webPort := flag.Int("web-port", 8080, "Local HTTP port for the dashboard")
	verbose := flag.Int("verbose", 0, "DCOL / stream verbosity: 0=off, 1=checksum/page warnings, 2=each GSOF sub-record payload as spaced hex (types 0x01/len/payload…), 3=full parser debug")
	showExpectedReserved := flag.Bool("show-expected-reserved-bits", false, "In GSOF flag decodes (types 1/8/10), include reserved bits that match the spec; default hides them")
	flag.Parse()

	gsof.ShowExpectedReservedBits = *showExpectedReserved

	cfg := core.Config{
		IP:      *ipFlag,
		Host:    *host,
		Port:    *port,
		Decode:  "dcol",
		Verbose: *verbose,
	}
	if cfg.Host != "" {
		cfg.IP = "tcp"
	}

	stats := gsofstats.NewStats(false)
	broker := gsofstats.NewJSONBroker()
	packetChan := make(chan core.PacketEvent, 1000)

	go stream.StartListener(cfg, packetChan)
	go func() {
		for pkt := range packetChan {
			if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
				stats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer)
			}
		}
	}()

	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			dash := stats.BuildDashboard(cfg.IP, cfg.Port, Version)
			data, err := json.Marshal(dash)
			if err != nil {
				continue
			}
			broker.Publish(data)
		}
	}()

	mux := http.NewServeMux()
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
	mux.Handle("/events", broker)

	addr := fmt.Sprintf("127.0.0.1:%d", *webPort)
	srv := &http.Server{Addr: addr, Handler: mux}

	fmt.Fprintf(os.Stdout, "gsof-dashboard version %s\n  web UI:  http://%s\n  GSOF:    %s port %d\n",
		Version, addr, cfg.IP, cfg.Port)

	go func() {
		slog.Info("GSOF dashboard listening", "version", Version, "url", "http://"+addr, "stream", fmt.Sprintf("%s:%d", cfg.IP, cfg.Port))
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
