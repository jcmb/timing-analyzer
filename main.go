package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"
)

type Config struct {
	Protocol	  string
	Host		  string
	Port		  int
	RateHz		  float64
	JitterMs	  float64
	TimeoutExit	  bool
	Verbose		  int
	WarmupPackets int
	Decode		  string
}

type PacketEvent struct {
	BestTime	  time.Time
	GoTime		  time.Time
	KernelTime	  time.Time
	Length		  int
	RemoteAddr	  string
	PacketType	  int
	Decoded		  bool
	IsCMR		  bool
	PacketSubType int
	Version		  int
	StationID	  int
	IsLastInBurst bool
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

		out <- PacketEvent{
			BestTime:	   bestTime,
			GoTime:		   goTime,
			KernelTime:	   kernelTime,
			Length:		   totalExpectedLen,
			RemoteAddr:	   remoteAddr,
			PacketType:	   int(pktType),
			Decoded:	   true,
			IsCMR:		   isCMR,
			PacketSubType: subType,
			Version:	   version,
			StationID:	   stationID,
			IsLastInBurst: isLastInBurst,
		}

		p.buf = p.buf[totalExpectedLen:]
	}
}

func main() {
	protocol := flag.String("protocol", "udp", "tcp or udp")
	host := flag.String("host", "", "Optional host IP to connect to (implicitly forces tcp mode)")
	port := flag.Int("port", 2101, "Port to listen on or connect to")
	rate := flag.Float64("rate", 1.0, "Expected update rate in Hz")
	jitter := flag.Float64("jitter", 5.0, "Allowable jitter in ms")
	timeoutExit := flag.Bool("timeout-exit", true, "Exit with error if no data in 100 epochs")
	verbose := flag.Int("verbose", 0, "Verbosity level (1=warmup, 2=all packets, 3=parser debug)")
	warmup := flag.Int("warmup", 0, "Number of initial packets to ignore (0 to disable)")
	decode := flag.String("decode", "none", "Protocol decoder: 'none' or 'dcol'")
	flag.Parse()

	// If host is provided, automatically assume TCP
	if *host != "" {
		*protocol = "tcp"
	}

	cfg := Config{
		Protocol:	   *protocol,
		Host:		   *host,
		Port:		   *port,
		RateHz:		   *rate,
		JitterMs:	   *jitter,
		TimeoutExit:   *timeoutExit,
		Verbose:	   *verbose,
		WarmupPackets: *warmup,
		Decode:		   *decode,
	}

	packetChan := make(chan PacketEvent, 1000)

	go startListener(cfg, packetChan)
	runTimingEngine(cfg, packetChan)
}

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

			if cfg.Decode == "dcol" {
				if parsers[ip] == nil {
					parsers[ip] = &DCOLParser{}
				}
				parsers[ip].Process(buf[:n], bestTime, goTime, kernelTime, ip, cfg.Verbose, packetChan)
			} else {
				packetChan <- PacketEvent{
					BestTime:	   bestTime,
					GoTime:		   goTime,
					KernelTime:	   kernelTime,
					Length:		   n,
					RemoteAddr:	   ip,
					PacketType:	   -1,
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

		if cfg.Decode == "dcol" {
			parser.Process(buf[:n], goTime, goTime, time.Time{}, remoteIP, cfg.Verbose, packetChan)
		} else {
			packetChan <- PacketEvent{
				BestTime:	   goTime,
				GoTime:		   goTime,
				Length:		   n,
				RemoteAddr:	   remoteIP,
				PacketType:	   -1,
				IsLastInBurst: true,
			}
		}
	}
}

type TimingState struct {
	LastPacketTime time.Time
	PacketCount	   uint64
	PrevError	   time.Duration
}

func getNiceName(timingKey string) string {
	switch timingKey {
	case "0x93-0":
		return "CMR GPS"
	case "0x93-1":
		return "CMR Base LLH"
	case "0x93-2":
		return "CMR Base Name"
	case "0x93-3":
		return "CMR GLN-STD"
	case "0x94":
		return "CMR+ Base"
	case "0x98-1":
		return "CMR Time"
	case "0x40":
		return "GSOF"
	default:
		return timingKey
	}
}

func logAligned(level string, t time.Time, event string, timingKey string, count uint64, delta, expected int64, extra string) {
	timeStr := t.Format("15:04:05.000000")
	niceName := getNiceName(timingKey)

	var deltaStr, expStr string
	if delta == -1 {
		deltaStr = " ---"
		expStr = " ---"
	} else {
		deltaStr = fmt.Sprintf("%4d", delta)
		expStr = fmt.Sprintf("%4d", expected)
	}

	fmt.Printf("%s | %-5s | %-17s | Type: %-13s | Count: %-6d | Delta: %s ms | Exp: %s ms | %s\n",
		timeStr, level, event, niceName, count, deltaStr, expStr, extra)
}

func runTimingEngine(cfg Config, packetChan <-chan PacketEvent) {
	baseExpectedPeriod := time.Duration(float64(time.Second) / cfg.RateHz)
	jitterDur := time.Duration(cfg.JitterMs * float64(time.Millisecond))

	timeoutDur := baseExpectedPeriod * 100

	slog.Info("Timing Engine Started",
		"base_rate_hz", cfg.RateHz,
		"base_expected_ms", baseExpectedPeriod.Milliseconds(),
		"allowed_jitter_ms", cfg.JitterMs,
		"decode_mode", cfg.Decode,
		"warmup_packets", cfg.WarmupPackets)

	fmt.Printf("\n%-15s | %-5s | %-17s | %-19s | %-13s | %-13s | %-13s | %s\n",
		"TIME", "LEVEL", "EVENT", "PKT TYPE", "BURST COUNT", "ACTUAL DELTA", "EXPECTED", "EXTRA DETAILS")
	fmt.Println("---------------------------------------------------------------------------------------------------------------------------------------")

	states := make(map[string]*TimingState)

	timeoutTimer := time.NewTimer(timeoutDur)
	timeoutTimer.Stop()

	for {
		select {
		case pkt := <-packetChan:

			timingKey := "RAW"
			if pkt.Decoded {
				if pkt.IsCMR {
					timingKey = fmt.Sprintf("0x%02X-%d", pkt.PacketType, pkt.PacketSubType)
				} else {
					timingKey = fmt.Sprintf("0x%02X", pkt.PacketType)
				}
			}

			state, exists := states[timingKey]
			if !exists {
				state = &TimingState{}
				states[timingKey] = state
			}

			if !pkt.IsLastInBurst {
				if cfg.Verbose >= 2 {
					extra := fmt.Sprintf("IP: %s | Len: %d | Burst Part (Ignored for Timing)", pkt.RemoteAddr, pkt.Length)
					logAligned("INFO", pkt.BestTime, "burst_part_rx", timingKey, state.PacketCount, -1, -1, extra)
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

			// --- Override expected period for specific CMR messages ---
			currentExpectedPeriod := baseExpectedPeriod
			if timingKey == "0x93-1" || timingKey == "0x93-2" {
				currentExpectedPeriod = 10 * time.Second
			}

			if cfg.Verbose >= 2 {
				extra := fmt.Sprintf("IP: %s | Len: %3d bytes | OS_Delay: %4dus", pkt.RemoteAddr, pkt.Length, osDelayUs)
				if pkt.IsCMR {
					extra += fmt.Sprintf(" | CMR Ver: %d, StaID: %d", pkt.Version, pkt.StationID)
				}

				if isFirstPacket {
					logAligned("INFO", pkt.BestTime, "packet_received", timingKey, state.PacketCount, -1, -1, extra+" (First Packet/Burst)")
				} else {
					logAligned("INFO", pkt.BestTime, "packet_received", timingKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)
				}
			}

			if isFirstPacket {
				continue
			}

			// Core math relies rigidly on currentExpectedPeriod
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
					logAligned("ERROR", pkt.BestTime, "missed_packet", timingKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)

					expectedMultiPeriod := currentExpectedPeriod * time.Duration(estimatedMissed+1)
					state.PrevError = delta - expectedMultiPeriod

				} else if delta < minExpected || delta > maxExpected {
					if adjustedDelta >= minExpected && adjustedDelta <= maxExpected {
						if cfg.Verbose >= 1 {
							extra := fmt.Sprintf("Adj Delta: %d ms (Compensated by previous offset)", adjustedDelta.Milliseconds())
							logAligned("INFO", pkt.BestTime, "jitter_suppressed", timingKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)
						}
						state.PrevError = 0
					} else {
						extra := fmt.Sprintf("Adj Delta: %d ms | Allowed Jitter: ±%.0f ms", adjustedDelta.Milliseconds(), cfg.JitterMs)
						logAligned("WARN", pkt.BestTime, "jitter_violation", timingKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), extra)
						state.PrevError = currentError
					}
				} else {
					state.PrevError = currentError
				}

			} else if cfg.Verbose >= 1 && state.PacketCount > 1 {
				logAligned("INFO", pkt.BestTime, "warmup_phase", timingKey, state.PacketCount, delta.Milliseconds(), currentExpectedPeriod.Milliseconds(), "Ignoring jitter during warmup")
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
