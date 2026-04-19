package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
	"unsafe"
)

//go:embed index.html
var indexHTML []byte

type Config struct {
	Protocol      string
	Host          string
	Port          int
	WebPort       int
	RateHz        float64
	JitterMs      float64
	TimeoutExit   bool
	Verbose       int
	WarmupPackets int
	Decode        string
}

type PacketEvent struct {
	BestTime      time.Time
	GoTime        time.Time
	KernelTime    time.Time
	Length        int
	RemoteAddr    string
	PacketType    int
	Decoded       bool
	IsCMR         bool
	PacketSubType int
	Version       int
	StationID     int
	IsLastInBurst bool
}

// TelemetryEvent is the JSON payload sent to the Web Interface
type TelemetryEvent struct {
	Timestamp     string `json:"timestamp"`
	DisplayKey    string `json:"display_key"`
	Count         uint64 `json:"count"`
	ActualDeltaMs int64  `json:"actual_delta_ms"`
	ExpectedMs    int64  `json:"expected_ms"`
	Status        string `json:"status"` // "info", "warn", "error"
	Message       string `json:"message"`
}

// SSEBroker handles pushing real-time events to connected web browsers
type SSEBroker struct {
	Notifier       chan TelemetryEvent
	newClients     chan chan TelemetryEvent
	closingClients chan chan TelemetryEvent
	clients        map[chan TelemetryEvent]bool
}

func NewSSEBroker() *SSEBroker {
	broker := &SSEBroker{
		Notifier:       make(chan TelemetryEvent, 10),
		newClients:     make(chan chan TelemetryEvent),
		closingClients: make(chan chan TelemetryEvent),
		clients:        make(map[chan TelemetryEvent]bool),
	}
	go broker.listen()
	return broker
}

func (b *SSEBroker) listen() {
	for {
		select {
		case s := <-b.newClients:
			b.clients[s] = true
			slog.Info("Web UI Client Connected", "active_clients", len(b.clients))
		case s := <-b.closingClients:
			delete(b.clients, s)
			slog.Info("Web UI Client Disconnected", "active_clients", len(b.clients))
		case event := <-b.Notifier:
			for clientMessageChan := range b.clients {
				select {
				case clientMessageChan <- event:
				default:
					// If client is stuck, don't block the timing engine
				}
			}
		}
	}
}

func (b *SSEBroker) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan TelemetryEvent)
	b.newClients <- messageChan

	defer func() {
		b.closingClients <- messageChan
	}()

	notify := req.Context().Done()
	for {
		select {
		case <-notify:
			return
		case event := <-messageChan:
			jsonData, _ := json.Marshal(event)
			fmt.Fprintf(rw, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}

type DCOLParser struct {
	buf []byte
}

func (p *DCOLParser) Process(data []byte, bestTime, goTime, kernelTime time.Time, remoteAddr string, verbose int, out chan<- PacketEvent) {
	p.buf = append(p.buf, data...)

	for len(p.buf) >= 6 {
		stxIdx := -1
		for i, b := range p.buf {
			if b == 0x02 {
				stxIdx = i
				break
			}
		}

		if stxIdx == -1 {
			p.buf = nil
			return
		}

		if stxIdx > 0 {
			p.buf = p.buf[stxIdx:]
		}

		if len(p.buf) < 6 {
			return
		}

		pktType := p.buf[2]
		payloadLen := p.buf[3]
		totalExpectedLen := int(payloadLen) + 6

		if len(p.buf) < totalExpectedLen {
			return
		}

		if p.buf[totalExpectedLen-1] != 0x03 {
			p.buf = p.buf[1:]
			continue
		}

		var csum byte = 0
		for i := 1; i < totalExpectedLen-2; i++ {
			csum += p.buf[i]
		}

		if csum != p.buf[totalExpectedLen-2] {
			p.buf = p.buf[1:]
			continue
		}

		var isCMR bool
		var subType, version, stationID int
		isLastInBurst := true

		if pktType == 0x93 {
			isCMR = true
			if payloadLen >= 2 {
				firstByte := p.buf[4]
				secondByte := p.buf[5]
				version = int((firstByte >> 5) & 0x07)
				stationID = int(firstByte & 0x1F)
				subType = int((secondByte >> 5) & 0x07)
			}
		}

		if pktType == 0x98 {
			isCMR = true
			if payloadLen >= 1 {
				firstByte := p.buf[4]
				version = 0
				stationID = 0
				subType = int(firstByte)
			}
		}

		if pktType == 0x40 {
			if payloadLen >= 3 {
				pageIdx := p.buf[5]
				maxPageIdx := p.buf[6]
				isLastInBurst = (pageIdx == maxPageIdx)
			}
		}

		if pktType == 0x57 {
			if payloadLen >= 2 {
				pagingInfo := p.buf[5]
				pageIdx := (pagingInfo >> 4) & 0x0F
				maxPageIdx := pagingInfo & 0x0F
				isLastInBurst = (pageIdx == maxPageIdx)
			}
		}

		out <- PacketEvent{
			BestTime:      bestTime,
			GoTime:        goTime,
			KernelTime:    kernelTime,
			Length:        totalExpectedLen,
			RemoteAddr:    remoteAddr,
			PacketType:    int(pktType),
			Decoded:       true,
			IsCMR:         isCMR,
			PacketSubType: subType,
			Version:       version,
			StationID:     stationID,
			IsLastInBurst: isLastInBurst,
		}

		p.buf = p.buf[totalExpectedLen:]
	}
}

func main() {
	protocol := flag.String("protocol", "udp", "tcp or udp")
	host := flag.String("host", "", "Optional host IP to connect to (implicitly forces tcp mode)")
	port := flag.Int("port", 2101, "Port to listen on or connect to")
	webPort := flag.Int("web-port", 8080, "Port for the live web dashboard")
	rate := flag.Float64("rate", 1.0, "Expected update rate in Hz")
	jitter := flag.Float64("jitter", 5.0, "Allowable jitter in ms")
	timeoutExit := flag.Bool("timeout-exit", true, "Exit with error if no data in 100 epochs")
	verbose := flag.Int("verbose", 0, "Verbosity level (1=warmup, 2=all packets, 3=parser debug)")
	warmup := flag.Int("warmup", 0, "Number of initial packets to ignore (0 to disable)")
	decode := flag.String("decode", "none", "Protocol decoder: 'none', 'dcol', or 'mb-cmr'")
	flag.Parse()

	if *host != "" {
		*protocol = "tcp"
	}

	cfg := Config{
		Protocol:      *protocol,
		Host:          *host,
		Port:          *port,
		WebPort:       *webPort,
		RateHz:        *rate,
		JitterMs:      *jitter,
		TimeoutExit:   *timeoutExit,
		Verbose:       *verbose,
		WarmupPackets: *warmup,
		Decode:        *decode,
	}

	// Setup Telemetry Web Server
	broker := NewSSEBroker()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexHTML)
	})
	http.Handle("/events", broker)
	go func() {
		slog.Info("Starting Web Dashboard", "url", fmt.Sprintf("http://localhost:%d", cfg.WebPort))
		if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.WebPort), nil); err != nil {
			slog.Error("Web server failed", "error", err)
		}
	}()

	packetChan := make(chan PacketEvent, 1000)

	go startListener(cfg, packetChan)
	runTimingEngine(cfg, packetChan, broker) // Pass broker to the engine
}

// ... [startListener and handleTCPConn remain exactly the same as the previous version] ...
func startListener(cfg Config, packetChan chan<- PacketEvent) {
	switch cfg.Protocol {
	case "tcp":
		if cfg.Host != "" {
			address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
			slog.Info("Starting TCP client mode", "target", address)
			for {
				conn, err := net.DialTimeout("tcp", address, 3*time.Second)
				if err != nil {
					slog.Warn("Failed to connect to TCP host, retrying in 5s...", "error", err)
					time.Sleep(5 * time.Second)
					continue
				}
				handleTCPConn(conn, cfg, packetChan)
				slog.Warn("TCP connection lost, attempting to reconnect...")
				time.Sleep(1 * time.Second)
			}
		} else {
			address := fmt.Sprintf(":%d", cfg.Port)
			l, err := net.Listen("tcp", address)
			if err != nil {
				slog.Error("Failed to bind TCP port", "error", err)
				os.Exit(1)
			}
			slog.Info("Listening for TCP connections", "port", cfg.Port)
			for {
				conn, err := l.Accept()
				if err != nil {
					continue
				}
				go handleTCPConn(conn, cfg, packetChan)
			}
		}

	case "udp":
		address := fmt.Sprintf(":%d", cfg.Port)
		addr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			slog.Error("Failed to resolve UDP address", "error", err)
			os.Exit(1)
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			slog.Error("Failed to bind UDP port", "error", err)
			os.Exit(1)
		}
		slog.Info("Listening for UDP packets", "port", cfg.Port)

		rawConn, err := conn.SyscallConn()
		if err == nil {
			rawConn.Control(func(fd uintptr) {
				err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_TIMESTAMP, 1)
				if err == nil {
					slog.Info("Hardware/Kernel timestamping enabled")
				}
			})
		}

		buf := make([]byte, 2048)
		oob := make([]byte, 1024)
		parsers := make(map[string]*DCOLParser)

		for {
			n, oobn, _, remoteAddr, err := conn.ReadMsgUDP(buf, oob)
			goTime := time.Now()
			if err != nil {
				continue
			}

			ip := remoteAddr.String()
			var kernelTime time.Time
			bestTime := goTime

			if oobn > 0 {
				cmsgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
				if err == nil {
					for _, m := range cmsgs {
						if m.Header.Level == syscall.SOL_SOCKET && (m.Header.Type == syscall.SCM_TIMESTAMP || m.Header.Type == syscall.SO_TIMESTAMP) {
							tv := (*syscall.Timeval)(unsafe.Pointer(&m.Data[0]))
							kernelTime = time.Unix(int64(tv.Sec), int64(tv.Usec)*1000)
							bestTime = kernelTime
						}
					}
				}
			}

			if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
				if parsers[ip] == nil {
					parsers[ip] = &DCOLParser{}
				}
				parsers[ip].Process(buf[:n], bestTime, goTime, kernelTime, ip, cfg.Verbose, packetChan)
			} else {
				packetChan <- PacketEvent{
					BestTime:      bestTime,
					GoTime:        goTime,
					KernelTime:    kernelTime,
					Length:        n,
					RemoteAddr:    ip,
					PacketType:    -1,
					IsLastInBurst: true,
				}
			}
		}
	}
}

func handleTCPConn(conn net.Conn, cfg Config, packetChan chan<- PacketEvent) {
	defer conn.Close()
	buf := make([]byte, 2048)
	remoteIP := conn.RemoteAddr().String()
	slog.Info("TCP connection established", "remote_addr", remoteIP)

	parser := &DCOLParser{}

	for {
		n, err := conn.Read(buf)
		goTime := time.Now()
		if err != nil {
			slog.Info("TCP connection closed", "remote_addr", remoteIP)
			return
		}

		if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
			parser.Process(buf[:n], goTime, goTime, time.Time{}, remoteIP, cfg.Verbose, packetChan)
		} else {
			packetChan <- PacketEvent{
				BestTime:      goTime,
				GoTime:        goTime,
				Length:        n,
				RemoteAddr:    remoteIP,
				PacketType:    -1,
				IsLastInBurst: true,
			}
		}
	}
}


type TimingState struct {
	LastPacketTime time.Time
	PacketCount    uint64
	PrevError      time.Duration
}

func getNiceName(displayKey string) string {
	switch displayKey {
	case "0x93-0": return "CMR GPS"
	case "0x93-1": return "CMR Base LLH"
	case "0x93-2": return "CMR Base Name"
	case "0x93-3": return "CMR GLN-STD"
	case "0x93-4": return "GPS Delta"
	case "0x94":   return "CMR+ Base"
	case "0x98-0": return "CMR GLONASS"
	case "0x98-1": return "CMR Time"
	case "0x98-4": return "GLN Delta"
	case "0x40":   return "GSOF"
	case "0x57":   return "RAWDATA"
	default:       return displayKey
	}
}

// Emits payload to terminal AND web UI
func logAndEmit(broker *SSEBroker, level string, t time.Time, event string, displayKey string, count uint64, delta, expected int64, extra string) {
	timeStr := t.Format("15:04:05.000000")
	niceName := getNiceName(displayKey)

	var deltaStr, expStr string
	if delta == -1 {
		deltaStr = " ---"
		expStr = " ---"
	} else {
		deltaStr = fmt.Sprintf("%4d", delta)
		expStr = fmt.Sprintf("%4d", expected)
	}

	// Terminal Print
	fmt.Printf("%s | %-5s | %-17s | Type: %-13s | Count: %-6d | Delta: %s ms | Exp: %s ms | %s\n",
		timeStr, level, event, niceName, count, deltaStr, expStr, extra)

	// Web UI Broadcast
	statusStr := "info"
	if level == "WARN" {
		statusStr = "warn"
	} else if level == "ERROR" {
		statusStr = "error"
	}

	broker.Notifier <- TelemetryEvent{
		Timestamp:     timeStr,
		DisplayKey:    niceName,
		Count:         count,
		ActualDeltaMs: delta,
		ExpectedMs:    expected,
		Status:        statusStr,
		Message:       extra,
	}
}

func runTimingEngine(cfg Config, packetChan <-chan PacketEvent, broker *SSEBroker) {
	baseExpectedPeriod := time.Duration(float64(time.Second) / cfg.RateHz)
	jitterDur := time.Duration(cfg.JitterMs * float64(time.Millisecond))
	timeoutDur := baseExpectedPeriod * 100

	slog.Info("Timing Engine Started", "decode_mode", cfg.Decode, "warmup_packets", cfg.WarmupPackets)
	fmt.Printf("\n%-15s | %-5s | %-17s | %-19s | %-13s | %-13s | %-13s | %s\n", "TIME", "LEVEL", "EVENT", "PKT TYPE", "BURST COUNT", "ACTUAL DELTA", "EXPECTED", "EXTRA DETAILS")
	fmt.Println("---------------------------------------------------------------------------------------------------------------------------------------")

	states := make(map[string]*TimingState)
	timeoutTimer := time.NewTimer(timeoutDur)
	timeoutTimer.Stop()

	for {
		select {
		case pkt := <-packetChan:

			displayKey := "RAW"
			timingKey := "RAW"

			if pkt.Decoded {
				if pkt.IsCMR {
					displayKey = fmt.Sprintf("0x%02X-%d", pkt.PacketType, pkt.PacketSubType)
					timingKey = displayKey

					if cfg.Decode == "mb-cmr" {
						if displayKey == "0x93-0" || displayKey == "0x93-4" {
							timingKey = "0x93-MAIN"
						} else if displayKey == "0x98-0" || displayKey == "0x98-4" {
							timingKey = "0x98-MAIN"
						}
					}
				} else {
					displayKey = fmt.Sprintf("0x%02X", pkt.PacketType)
					timingKey = displayKey
				}
			}

			state, exists := states[timingKey]
			if !exists {
				state = &TimingState{}
				states[timingKey] = state
			}

			if !pkt.IsLastInBurst {
				if cfg.Verbose >= 2 {
					extra := fmt.Sprintf("IP: %s | Len: %d | Burst Part", pkt.RemoteAddr, pkt.Length)
					logAndEmit(broker, "INFO", pkt.BestTime, "burst_part_rx", displayKey, state.PacketCount, -1, -1, extra)
				}
				continue
			}

			state.PacketCount++
			isFirstPacket := state.LastPacketTime.IsZero()
			var delta time.Duration

			if !isFirstPacket {
				delta = pkt.BestTime.Sub(state.LastPacketTime)
			}

			state.LastPacketTime = pkt.BestTime

			if cfg.TimeoutExit {
				timeoutTimer.Reset(timeoutDur)
			}

			osDelayUs := pkt.GoTime.Sub(pkt.KernelTime).Microseconds()
			if pkt.KernelTime.IsZero() {
				osDelayUs = 0
			}

			currentExpectedPeriod := baseExpectedPeriod
			if cfg.Decode == "dcol" {
				if displayKey == "0x93-1" || displayKey == "0x93-2" {
					currentExpectedPeriod = 10 * time.Second
				}
			} else if cfg.Decode == "mb-cmr" {
				if displayKey == "0x93-2" {
					currentExpectedPeriod = 10 * time.Second
				} else if displayKey == "0x98-1" {
					currentExpectedPeriod = 1 * time.Second
				}
			}

			if cfg.Verbose >= 2 {
				extra := fmt.Sprintf("IP: %s | Len: %3d bytes | OS_Delay: %4dus", pkt.RemoteAddr, pkt.Length, osDelayUs)
				if pkt.IsCMR && pkt.PacketType != 0x98 {
					extra += fmt.Sprintf(" | CMR Ver: %d, StaID: %d", pkt.Version, pkt.StationID)
				}

				if isFirstPacket {
					logAndEmit(broker, "INFO", pkt.BestTime, "packet_received", displayKey, state.PacketCount, -1, -1, extra+" (First Packet)")
				} else {
					logAndEmit(broker, "INFO", pkt.BestTime, "packet_received", displayKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)
				}
			}

			if isFirstPacket {
				continue
			}

			minExpected := currentExpectedPeriod - jitterDur
			maxExpected := currentExpectedPeriod + jitterDur
			missedThreshold := (2 * currentExpectedPeriod) - jitterDur

			if state.PacketCount > uint64(cfg.WarmupPackets) {
				adjustedDelta := delta + state.PrevError
				currentError := delta - currentExpectedPeriod

				if delta >= missedThreshold {
					estimatedMissed := int((delta + jitterDur) / currentExpectedPeriod) - 1
					if estimatedMissed < 1 {
						estimatedMissed = 1
					}
					extra := fmt.Sprintf("Missed roughly %d packets!", estimatedMissed)
					logAndEmit(broker, "ERROR", pkt.BestTime, "missed_packet", displayKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)

					expectedMultiPeriod := currentExpectedPeriod * time.Duration(estimatedMissed+1)
					state.PrevError = delta - expectedMultiPeriod

				} else if delta < minExpected || delta > maxExpected {
					if adjustedDelta >= minExpected && adjustedDelta <= maxExpected {
						if cfg.Verbose >= 1 {
							extra := fmt.Sprintf("Adj Delta: %d ms (Compensated by previous offset)", adjustedDelta.Milliseconds())
							logAndEmit(broker, "INFO", pkt.BestTime, "jitter_suppressed", displayKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)
						}
						state.PrevError = 0
					} else {
						extra := fmt.Sprintf("Adj Delta: %d ms | Allowed Jitter: ±%.0f ms", adjustedDelta.Milliseconds(), cfg.JitterMs)
						logAndEmit(broker, "WARN", pkt.BestTime, "jitter_violation", displayKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)
						state.PrevError = currentError
					}
				} else {
					state.PrevError = currentError
				}

			} else if cfg.Verbose >= 1 && state.PacketCount > 1 {
				logAndEmit(broker, "INFO", pkt.BestTime, "warmup_phase", displayKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), "Ignoring jitter during warmup")
				state.PrevError = 0
			}

		case <-timeoutTimer.C:
			if cfg.TimeoutExit {
				slog.Error("No data received for 100 epochs. Exiting.")
				os.Exit(1)
			}
		}
	}
}
