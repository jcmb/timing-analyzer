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

var uiHTMLLive = func() []byte {
	return bytes.ReplaceAll(uiHTML, []byte("__VERSION__"), []byte(Version))
}()

func streamCfg(ip, host string, port int, verbose int, ignoreGap1 bool) core.Config {
	cfg := core.Config{
		IP:                            strings.ToLower(strings.TrimSpace(ip)),
		Host:                          strings.TrimSpace(host),
		Port:                          port,
		Decode:                        "dcol",
		Verbose:                       verbose,
		IgnoreTCPGSOFTransmissionGap1: ignoreGap1,
	}
	if cfg.Host != "" {
		cfg.IP = "tcp"
	}
	return cfg
}

func main() {
	rx1IP := flag.String("rx1-ip", "udp", "Stream 1: tcp or udp")
	rx1Host := flag.String("rx1-host", "", "Stream 1: TCP dial host (optional)")
	rx1Port := flag.Int("rx1-port", 2101, "Stream 1: UDP listen or TCP port")

	rx2IP := flag.String("rx2-ip", "udp", "Stream 2: tcp or udp")
	rx2Host := flag.String("rx2-host", "", "Stream 2: TCP dial host (optional)")
	rx2Port := flag.Int("rx2-port", 2102, "Stream 2: UDP listen or TCP port (must differ from stream 1 when both UDP bind locally)")

	matchMax := flag.Float64("match-max-tow-delta-sec", 0.25, "Max |GPS TOW type-1 stream1 − type-1 stream2| to pair epochs (seconds, week-wrapped)")
	rangeTol := flag.Float64("range-check-tolerance-m", 0, "If > 0, flag rows where |slant range − reference| exceeds this (metres)")
	expectedRange := flag.Float64("expected-range-m", 0, "Fixed reference range in metres (overrides type-27 when > 0)")
	rangeFromAtt := flag.Bool("range-ref-from-attitude", false, "When -expected-range-m is 0 and tolerance > 0, use GSOF type-27 range from stream 2 at nearest TOW")

	webHost := flag.String("web-host", "127.0.0.1", "HTTP listen address")
	webPort := flag.Int("web-port", 8091, "HTTP port for UI and /events SSE")
	verbose := flag.Int("verbose", 0, "DCOL verbosity (same as gsof-dashboard)")
	ignoreGap1 := flag.Bool("ignore-tcp-gsof-transmission-gap1", false, "TCP: suppress warnings for a single skipped GSOF transmission id")
	flag.Parse()

	cfg1 := streamCfg(*rx1IP, *rx1Host, *rx1Port, *verbose, *ignoreGap1)
	cfg2 := streamCfg(*rx2IP, *rx2Host, *rx2Port, *verbose, *ignoreGap1)

	eng := gsofbaseline.NewEngine(gsofbaseline.EngineConfig{
		MatchMaxTowDeltaSec:    *matchMax,
		RangeCheckTolM:       *rangeTol,
		ExpectedRangeM:       *expectedRange,
		RangeRefFromAttitude: *rangeFromAtt,
	})

	ch1 := make(chan core.PacketEvent, 2000)
	ch2 := make(chan core.PacketEvent, 2000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := stream.StartListenerContext(ctx, cfg1, ch1, nil)
		if err != nil && ctx.Err() == nil {
			slog.Error("stream1 listener", "error", err)
			os.Exit(1)
		}
	}()
	go func() {
		err := stream.StartListenerContext(ctx, cfg2, ch2, nil)
		if err != nil && ctx.Err() == nil {
			slog.Error("stream2 listener", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-ch1:
				for _, w := range pkt.StreamWarnings {
					slog.Debug("stream1 warn", "msg", w)
				}
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					eng.IngestA(pkt.GSOFBuffer)
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-ch2:
				for _, w := range pkt.StreamWarnings {
					slog.Debug("stream2 warn", "msg", w)
				}
				if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
					eng.IngestB(pkt.GSOFBuffer)
				}
			}
		}
	}()

	broker := gsofbaseline.NewJSONBroker()
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
				if err != nil {
					slog.Warn("json marshal", "error", err)
					continue
				}
				broker.Publish(data)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/events", broker.ServeHTTP)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-GSOF-Baseline-Version", Version)
		_, _ = w.Write(uiHTMLLive)
	})

	addr := net.JoinHostPort(*webHost, strconv.Itoa(*webPort))
	srv := &http.Server{Addr: addr, Handler: mux}

	fmt.Fprintf(os.Stdout, "gsof-baseline version %s\n  web UI:  http://%s\n  stream1: %s\n  stream2: %s\n",
		Version, addr, describeStream(cfg1), describeStream(cfg2))
	if *rangeTol > 0 {
		ref := "type-27 on stream2 (nearest TOW)"
		if *expectedRange > 0 {
			ref = fmt.Sprintf("fixed expected %.3f m", *expectedRange)
		} else if !*rangeFromAtt {
			ref = "none (set -range-ref-from-attitude or -expected-range-m)"
		}
		fmt.Fprintf(os.Stdout, "  range check: tolerance %.3f m · reference: %s\n", *rangeTol, ref)
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
