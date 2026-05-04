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
	headingIP := flag.String("heading-ip", "udp", "Heading receiver: tcp or udp")
	headingHost := flag.String("heading-host", "", "Heading receiver: TCP dial host (optional)")
	headingPort := flag.Int("heading-port", 2101, "Heading receiver: UDP listen or TCP port")

	mbIP := flag.String("moving-base-ip", "udp", "Moving base: tcp or udp (ignored when -moving-base-port is 0)")
	mbHost := flag.String("moving-base-host", "", "Moving base: TCP dial host (optional)")
	mbPort := flag.Int("moving-base-port", 0, "Moving base: UDP listen or TCP port; 0 = disabled (not required when GSOF type 41 is on the heading stream)")

	matchMax := flag.Float64("match-max-tow-delta-sec", 0.25, "Max GPS TOW gap (s, week-wrapped) between heading epoch and reference (type 41 or moving-base type 1)")
	rangeTol := flag.Float64("range-check-tolerance-m", 0, "If > 0, flag rows where |slant range − reference| exceeds this (metres)")
	expectedRange := flag.Float64("expected-range-m", 0, "Fixed reference range in metres (overrides type-27 when > 0)")
	rangeFromAtt := flag.Bool("range-ref-from-attitude", false, "When -expected-range-m is 0 and tolerance > 0, use GSOF type-27 range at nearest TOW (heading stream when using type 41; moving-base stream when using moving-base LLH)")

	webHost := flag.String("web-host", "127.0.0.1", "HTTP listen address")
	webPort := flag.Int("web-port", 8091, "HTTP port for UI and /events SSE")
	verbose := flag.Int("verbose", 0, "DCOL verbosity (same as gsof-dashboard)")
	ignoreGap1 := flag.Bool("ignore-tcp-gsof-transmission-gap1", false, "TCP: suppress warnings for a single skipped GSOF transmission id")
	flag.Parse()

	mbEnabled := *mbPort != 0
	cfgHeading := streamCfg(*headingIP, *headingHost, *headingPort, *verbose, *ignoreGap1)
	var cfgMB core.Config
	if mbEnabled {
		cfgMB = streamCfg(*mbIP, *mbHost, *mbPort, *verbose, *ignoreGap1)
	}

	eng := gsofbaseline.NewEngine(gsofbaseline.EngineConfig{
		MatchMaxTowDeltaSec:    *matchMax,
		RangeCheckTolM:         *rangeTol,
		ExpectedRangeM:         *expectedRange,
		RangeRefFromAttitude:   *rangeFromAtt,
		MovingBaseConfigured:   mbEnabled,
	})

	chHeading := make(chan core.PacketEvent, 2000)
	var chMB chan core.PacketEvent
	if mbEnabled {
		chMB = make(chan core.PacketEvent, 2000)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := stream.StartListenerContext(ctx, cfgHeading, chHeading, nil)
		if err != nil && ctx.Err() == nil {
			slog.Error("heading listener", "error", err)
			os.Exit(1)
		}
	}()
	if mbEnabled {
		go func() {
			err := stream.StartListenerContext(ctx, cfgMB, chMB, nil)
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

	fmt.Fprintf(os.Stdout, "gsof-baseline version %s\n  web UI:  http://%s\n  heading: %s\n",
		Version, addr, describeStream(cfgHeading))
	if mbEnabled {
		fmt.Fprintf(os.Stdout, "  moving base: %s\n", describeStream(cfgMB))
	} else {
		fmt.Fprintf(os.Stdout, "  moving base: (disabled — enable with -moving-base-port, or use GSOF type 41 on heading stream)\n")
	}
	if *rangeTol > 0 {
		ref := "type-27 (heading or moving base, see -range-ref-from-attitude)"
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
