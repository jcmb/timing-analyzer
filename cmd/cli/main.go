package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/stream"
	"timing-analyzer/internal/telemetry"
	"timing-analyzer/internal/timing"
	"timing-analyzer/web"
)


const AppVersion = "v1.1.0"

func main() {
	ipFlag := flag.String("ip", "udp", "tcp or udp")
	host := flag.String("host", "", "Optional host IP to connect to (implicitly forces tcp mode)")
	port := flag.Int("port", 2101, "Port to listen on or connect to")
	webPort := flag.Int("web-port", 8080, "Port for the live web dashboard")
	rate := flag.Float64("rate", 1.0, "Expected update rate in Hz")
	jitterFlag := flag.String("jitter", "10%", "Allowable jitter (e.g. '5ms' or '10%')")
	timeoutExit := flag.Bool("timeout-exit", true, "Exit with error if no data in 100 epochs")
	verbose := flag.Int("verbose", 0, "Verbosity level (1=warmup, 2=all packets, 3=parser debug)")
	warmup := flag.Int("warmup", 0, "Number of initial packets to ignore (0 to disable)")
	decode := flag.String("decode", "none", "Protocol decoder: 'none', 'dcol', or 'mb-cmr'")
	csvFile := flag.String("csv", "", "Output filename for CSV logging")
	flag.Parse()

    fmt.Printf("Starting Timing Analyzer CLI %s\n", AppVersion)

	if *host != "" {
		*ipFlag = "tcp"
	}

	csvFilename := *csvFile
	if csvFilename != "" && !strings.HasSuffix(strings.ToLower(csvFilename), ".csv") {
		csvFilename += ".csv"
	}

	jitterStr := strings.TrimSpace(*jitterFlag)
	var jitterVal float64
	var jitterPct bool
	if strings.HasSuffix(jitterStr, "%") {
		jitterPct = true
		val, err := strconv.ParseFloat(strings.TrimSuffix(jitterStr, "%"), 64)
		if err != nil {
			slog.Error("Invalid jitter percentage", "error", err)
			os.Exit(1)
		}
		jitterVal = val
	} else {
		val, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(jitterStr), "ms"), 64)
		if err != nil {
			slog.Error("Invalid jitter value", "error", err)
			os.Exit(1)
		}
		jitterVal = val
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

	// 1. Setup the UI Web Server
	broker := telemetry.NewSSEBroker()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(web.IndexHTML) // Using the exported variable from the web package!
	})
	http.Handle("/events", broker)
	go func() {
		slog.Info("Starting Web Dashboard", "url", fmt.Sprintf("http://localhost:%d", cfg.WebPort))
		if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.WebPort), nil); err != nil {
			slog.Error("Web server failed", "error", err)
		}
	}()

	// 2. Wire the plumbing
	packetChan := make(chan core.PacketEvent, 1000)

	// 3. Start the components
	go stream.StartListener(cfg, packetChan)
	timing.Run(context.Background(), cfg, packetChan, broker.Notifier)
}

