package main

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//go:embed index.html
var indexHTML []byte

type Config struct {
	IP            string
	Host          string
	Port          int
	WebPort       int
	RateHz        float64
	JitterVal     float64
	JitterPct     bool
	TimeoutExit   bool
	Verbose       int
	WarmupPackets int
	Decode        string
	CSVFile       string
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

type TelemetryEvent struct {
	Timestamp     string `json:"timestamp"`
	DisplayKey    string `json:"display_key"`
	Count         uint64 `json:"count"`
	ActualDeltaMs int64  `json:"actual_delta_ms"`
	ExpectedMs    int64  `json:"expected_ms"`
	OSDelayUs     int64  `json:"os_delay_us"`
	IsUDP         bool   `json:"is_udp"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

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

	cfg := Config{
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
	runTimingEngine(cfg, packetChan, broker)
}

func startListener(cfg Config, packetChan chan<- PacketEvent) {
	switch cfg.IP {
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

type LogEntry struct {
	Level         string
	Time          time.Time
	Event         string
	DisplayKey    string
	Count         uint64
	Delta         int64
	Expected      int64
	IP            string
	Length        int
	IsUDP         bool
	HasKernelTime bool
	OSDelayUs     int64
	IsCMR         bool
	PktType       int
	CMRVer        int
	StationID     int
	HasAdj        bool
	AdjDelta      int64
	MissedPackets int
	Message       string
	PrintConsole  bool
	WriteCSV      bool
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

// Emits payload to terminal, web UI, and clean CSV columns
func logAndEmit(broker *SSEBroker, csvWriter *csv.Writer, e LogEntry, allowedJitterMs float64) {
	timeStr := e.Time.Format("15:04:05.000000")
	niceName := getNiceName(e.DisplayKey)

	deltaCol := "---"
	expCol := "---"
	csvDelta := ""
	csvExp := ""
	if e.Delta != -1 {
		deltaCol = fmt.Sprintf("%4d ms", e.Delta)
		expCol = fmt.Sprintf("%4d ms", e.Expected)
		csvDelta = fmt.Sprintf("%d", e.Delta)
		csvExp = fmt.Sprintf("%d", e.Expected)
	}

	var extras []string
	if e.IP != "" {
		extras = append(extras, fmt.Sprintf("IP: %s", e.IP))
	}
	if e.Length > 0 {
		extras = append(extras, fmt.Sprintf("Len: %3d bytes", e.Length))
	}

	if e.IsUDP && e.Event != "burst_part_rx" {
		if e.HasKernelTime {
			extras = append(extras, fmt.Sprintf("OS_Delay: %4dus", e.OSDelayUs))
		} else {
			extras = append(extras, "OS_Delay:  N/A us")
		}
	}

	if e.IsCMR && e.PktType != 0x98 {
		extras = append(extras, fmt.Sprintf("CMR Ver: %d, StaID: %d", e.CMRVer, e.StationID))
	}

	csvAdj := ""
	if e.HasAdj {
		csvAdj = fmt.Sprintf("%d", e.AdjDelta)
		if e.Event == "jitter_violation" {
			extras = append(extras, fmt.Sprintf("Adj Delta: %4d ms", e.AdjDelta))
			extras = append(extras, fmt.Sprintf("Allowed Jitter: ±%.0f ms", allowedJitterMs))
		} else if e.Event == "jitter_suppressed" {
			extras = append(extras, fmt.Sprintf("Adj Delta: %4d ms", e.AdjDelta))
		}
	}

	if e.MissedPackets > 0 {
		extras = append(extras, fmt.Sprintf("Missed: %d packets", e.MissedPackets))
	}

	if e.Message != "" {
		extras = append(extras, e.Message)
	}

	extraStr := strings.Join(extras, " | ")

	if e.PrintConsole {
		fmt.Printf("%-15s | %-5s | %-17s | %-13s | %-6d | %-12s | %-12s | %s\n",
			timeStr, e.Level, e.Event, niceName, e.Count, deltaCol, expCol, extraStr)
	}

	if csvWriter != nil && e.WriteCSV {
		csvCMRVer := ""
		csvStaID := ""
		if e.IsCMR && e.PktType != 0x98 {
			csvCMRVer = fmt.Sprintf("%d", e.CMRVer)
			csvStaID = fmt.Sprintf("%d", e.StationID)
		}

		csvLen := ""
		if e.Length > 0 {
			csvLen = fmt.Sprintf("%d", e.Length)
		}

		csvOSDelay := ""
		if e.IsUDP && e.Event != "burst_part_rx" && e.HasKernelTime {
			csvOSDelay = fmt.Sprintf("%d", e.OSDelayUs)
		}

		csvMissed := ""
		if e.MissedPackets > 0 {
			csvMissed = fmt.Sprintf("%d", e.MissedPackets)
		}

		record := []string{
			timeStr, e.Level, e.Event, niceName, fmt.Sprintf("%d", e.Count),
			csvDelta, csvExp, csvAdj, csvMissed, e.IP, csvLen, csvOSDelay, csvCMRVer, csvStaID, e.Message,
		}
		csvWriter.Write(record)
		csvWriter.Flush()
	}

	statusStr := "info"
	if e.Level == "WARN" {
		statusStr = "warn"
	} else if e.Level == "ERROR" {
		statusStr = "error"
	}

	if broker != nil {
		broker.Notifier <- TelemetryEvent{
			Timestamp:     timeStr,
			DisplayKey:    niceName,
			Count:         e.Count,
			ActualDeltaMs: e.Delta,
			ExpectedMs:    e.Expected,
			OSDelayUs:     e.OSDelayUs,
			IsUDP:         e.IsUDP,
			Status:        statusStr,
			Message:       extraStr,
		}
	}
}

func runTimingEngine(cfg Config, packetChan <-chan PacketEvent, broker *SSEBroker) {
	baseExpectedPeriod := time.Duration(float64(time.Second) / cfg.RateHz)
	timeoutDur := baseExpectedPeriod * 100

	// Calculate and log the computed base jitter
	var baseJitterMs float64
	if cfg.JitterPct {
		baseJitterMs = float64(baseExpectedPeriod.Milliseconds()) * (cfg.JitterVal / 100.0)
	} else {
		baseJitterMs = cfg.JitterVal
	}

	var csvWriter *csv.Writer
	if cfg.CSVFile != "" {
		f, err := os.OpenFile(cfg.CSVFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			slog.Error("Failed to open CSV file for logging", "error", err)
		} else {
			defer f.Close()
			csvWriter = csv.NewWriter(f)
			info, _ := f.Stat()
			if info != nil && info.Size() == 0 {
				csvWriter.Write([]string{
					"TIME", "LEVEL", "EVENT", "PKT TYPE", "COUNT",
					"ACTUAL DELTA (ms)", "EXPECTED (ms)", "ADJ DELTA (ms)", "MISSED PACKETS",
					"IP", "LEN (bytes)", "OS DELAY (us)", "CMR VER", "STA ID", "MESSAGE",
				})
				csvWriter.Flush()
			}
			slog.Info("CSV logging enabled", "file", cfg.CSVFile)
		}
	}

	slog.Info("Timing Engine Started",
		"decode_mode", cfg.Decode,
		"warmup_packets", cfg.WarmupPackets,
		"base_rate_hz", cfg.RateHz,
		"computed_base_jitter_ms", fmt.Sprintf("%.2f", baseJitterMs))

	fmt.Printf("\n%-15s | %-5s | %-17s | %-13s | %-6s | %-12s | %-12s | %s\n",
		"TIME", "LEVEL", "EVENT", "PKT TYPE", "COUNT", "ACTUAL DELTA", "EXPECTED", "EXTRA DETAILS")
	fmt.Println("---------------------------------------------------------------------------------------------------------------------------------------")

	states := make(map[string]*TimingState)
	timeoutTimer := time.NewTimer(timeoutDur)
	timeoutTimer.Stop()

	isUDP := cfg.IP == "udp"

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

			osDelayUs := int64(0)
			hasKernelTime := false
			if !pkt.KernelTime.IsZero() {
				osDelayUs = pkt.GoTime.Sub(pkt.KernelTime).Microseconds()
				hasKernelTime = true
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

			var jitterDur time.Duration
			var allowedJitterMs float64
			if cfg.JitterPct {
				jitterDur = time.Duration(float64(currentExpectedPeriod) * (cfg.JitterVal / 100.0))
				allowedJitterMs = float64(jitterDur.Milliseconds())
			} else {
				jitterDur = time.Duration(cfg.JitterVal * float64(time.Millisecond))
				allowedJitterMs = cfg.JitterVal
			}

			baseEntry := LogEntry{
				Time:          pkt.BestTime,
				DisplayKey:    displayKey,
				Count:         state.PacketCount + 1,
				Delta:         -1,
				Expected:      currentExpectedPeriod.Milliseconds(),
				IP:            pkt.RemoteAddr,
				Length:        pkt.Length,
				IsUDP:         isUDP,
				HasKernelTime: hasKernelTime,
				OSDelayUs:     osDelayUs,
				IsCMR:         pkt.IsCMR,
				PktType:       pkt.PacketType,
				CMRVer:        pkt.Version,
				StationID:     pkt.StationID,
				PrintConsole:  true,
				WriteCSV:      true,
			}

			if !pkt.IsLastInBurst {
				if cfg.Verbose >= 2 {
					e := baseEntry
					e.Level = "INFO"
					e.Event = "burst_part_rx"
					e.Expected = -1
					e.Count = state.PacketCount
					logAndEmit(broker, csvWriter, e, allowedJitterMs)
				}
				continue
			}

			state.PacketCount++
			isFirstPacket := state.LastPacketTime.IsZero()
			var delta time.Duration

			if !isFirstPacket {
				delta = pkt.BestTime.Sub(state.LastPacketTime)
				baseEntry.Delta = delta.Milliseconds()
			}

			state.LastPacketTime = pkt.BestTime

			if cfg.TimeoutExit {
				timeoutTimer.Reset(timeoutDur)
			}

			eRx := baseEntry
			eRx.Level = "INFO"
			eRx.Event = "packet_received"
			if isFirstPacket {
				eRx.Expected = -1
				eRx.Message = "(First Packet)"
			}
			eRx.PrintConsole = cfg.Verbose >= 2
			eRx.WriteCSV = cfg.Verbose >= 2
			logAndEmit(broker, csvWriter, eRx, allowedJitterMs)

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

					e := baseEntry
					e.Level = "ERROR"
					e.Event = "missed_packet"
					e.MissedPackets = estimatedMissed
					logAndEmit(broker, csvWriter, e, allowedJitterMs)

					expectedMultiPeriod := currentExpectedPeriod * time.Duration(estimatedMissed+1)
					state.PrevError = delta - expectedMultiPeriod

				} else if delta < minExpected || delta > maxExpected {
					if adjustedDelta >= minExpected && adjustedDelta <= maxExpected {
						if cfg.Verbose >= 2 {
							e := baseEntry
							e.Level = "INFO"
							e.Event = "jitter_suppressed"
							e.HasAdj = true
							e.AdjDelta = adjustedDelta.Milliseconds()
							e.Message = "(Compensated by previous offset)"
							logAndEmit(broker, csvWriter, e, allowedJitterMs)
						}
						state.PrevError = 0
					} else {
						e := baseEntry
						e.Level = "WARN"
						e.Event = "jitter_violation"
						e.HasAdj = true
						e.AdjDelta = adjustedDelta.Milliseconds()
						logAndEmit(broker, csvWriter, e, allowedJitterMs)
						state.PrevError = currentError
					}
				} else {
					state.PrevError = currentError
				}

			} else if cfg.Verbose >= 1 && state.PacketCount > 1 {
				e := baseEntry
				e.Level = "INFO"
				e.Event = "warmup_phase"
				e.Message = "Ignoring jitter during warmup"
				logAndEmit(broker, csvWriter, e, allowedJitterMs)
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
