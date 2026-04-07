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

// Config holds the application parameters
type Config struct {
	Protocol      string
	Host          string
	Port          int
	RateHz        float64
	JitterMs      float64
	TimeoutExit   bool
	Verbose       int
	WarmupPackets int
}

// PacketEvent encapsulates the metadata of a received network payload
type PacketEvent struct {
	BestTime   time.Time // The most accurate time available (Kernel if possible, else Go)
	GoTime     time.Time // The time recorded in user-space
	KernelTime time.Time // The time recorded by the OS kernel (zero if unavailable)
	Length     int
	RemoteAddr string
}

func main() {
	// 1. Parse Command-Line Flags
	protocol := flag.String("protocol", "udp", "tcp or udp")
	host := flag.String("host", "", "Optional host IP to connect to (for TCP client mode)")
	port := flag.Int("port", 2101, "Port to listen on or connect to")
	rate := flag.Float64("rate", 10.0, "Expected update rate in Hz")
	jitter := flag.Float64("jitter", 5.0, "Allowable jitter in ms")
	timeoutExit := flag.Bool("timeout-exit", true, "Exit with error if no data in 100 epochs")
	verbose := flag.Int("verbose", 0, "Verbosity level (set to 2 to log all packets)")
	warmup := flag.Int("warmup", 5, "Number of initial packets to ignore for jitter calculations")
	flag.Parse()

	cfg := Config{
		Protocol:      *protocol,
		Host:          *host,
		Port:          *port,
		RateHz:        *rate,
		JitterMs:      *jitter,
		TimeoutExit:   *timeoutExit,
		Verbose:       *verbose,
		WarmupPackets: *warmup,
	}

	packetChan := make(chan PacketEvent, 1000)

	// 2. Start the Network Listener / Connector
	go startListener(cfg, packetChan)

	// 3. Run the Timing Engine
	runTimingEngine(cfg, packetChan)
}

func startListener(cfg Config, packetChan chan<- PacketEvent) {
	switch cfg.Protocol {
	case "tcp":
		if cfg.Host != "" {
			// TCP Client Mode
			address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
			slog.Info("Starting TCP client mode", "target", address)

			for {
				// 3-second timeout so it doesn't hang indefinitely
				conn, err := net.DialTimeout("tcp", address, 3*time.Second)
				if err != nil {
					slog.Warn("Failed to connect to TCP host, retrying in 5s...", "error", err)
					time.Sleep(5 * time.Second)
					continue
				}
				// This blocks until the connection is closed
				handleTCPConn(conn, packetChan)
				slog.Warn("TCP connection lost, attempting to reconnect...")
				time.Sleep(1 * time.Second)
			}
		} else {
			// TCP Server Mode
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
					slog.Warn("Failed to accept TCP connection", "error", err)
					continue
				}
				go handleTCPConn(conn, packetChan)
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

		// Request OS Kernel Timestamping (macOS/Linux)
		rawConn, err := conn.SyscallConn()
		if err == nil {
			rawConn.Control(func(fd uintptr) {
				err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_TIMESTAMP, 1)
				if err == nil {
					slog.Info("Hardware/Kernel timestamping (SO_TIMESTAMP) enabled")
				}
			})
		}

		buf := make([]byte, 2048)
		oob := make([]byte, 1024)

		for {
			n, oobn, _, remoteAddr, err := conn.ReadMsgUDP(buf, oob)
			goTime := time.Now() // Capture user-space time immediately

			if err != nil {
				slog.Warn("UDP read error", "error", err)
				continue
			}

			var kernelTime time.Time
			bestTime := goTime // Default to Go user-space time

// Extract Kernel Timestamp if available
			if oobn > 0 {
				cmsgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
				if err == nil {
					for _, m := range cmsgs {
						// FIX: Check for SCM_TIMESTAMP instead of SO_TIMESTAMP
						if m.Header.Level == syscall.SOL_SOCKET && m.Header.Type == syscall.SCM_TIMESTAMP {
							// Unsafe cast OS Timeval to Go time
							tv := (*syscall.Timeval)(unsafe.Pointer(&m.Data[0]))
							kernelTime = time.Unix(int64(tv.Sec), int64(tv.Usec)*1000)
							bestTime = kernelTime // Upgrade to higher precision kernel time
						}
					}
				}
			}
			packetChan <- PacketEvent{
				BestTime:   bestTime,
				GoTime:     goTime,
				KernelTime: kernelTime,
				Length:     n,
				RemoteAddr: remoteAddr.String(),
			}
		}

	default:
		slog.Error("Unknown protocol. Use tcp or udp")
		os.Exit(1)
	}
}

func handleTCPConn(conn net.Conn, packetChan chan<- PacketEvent) {
	defer conn.Close()
	buf := make([]byte, 2048)
	remoteIP := conn.RemoteAddr().String()
	slog.Info("TCP connection established", "remote_addr", remoteIP)

	for {
		n, err := conn.Read(buf)
		goTime := time.Now()
		if err != nil {
			slog.Info("TCP connection closed", "remote_addr", remoteIP)
			return
		}

		packetChan <- PacketEvent{
			BestTime:   goTime,
			GoTime:     goTime,
			// KernelTime remains zero because TCP doesn't supply packet-boundary metadata
			Length:     n,
			RemoteAddr: remoteIP,
		}
	}
}

func runTimingEngine(cfg Config, packetChan <-chan PacketEvent) {
	expectedPeriod := time.Duration(float64(time.Second) / cfg.RateHz)
	jitterDur := time.Duration(cfg.JitterMs * float64(time.Millisecond))
	timeoutDur := expectedPeriod * 100

	slog.Info("Timing Engine Started",
		"rate_hz", cfg.RateHz,
		"expected_period_ms", expectedPeriod.Milliseconds(),
		"allowed_jitter_ms", cfg.JitterMs,
		"warmup_packets", cfg.WarmupPackets,
		"verbose_level", cfg.Verbose)

	var lastPacketTime time.Time
	var packetCount uint64
	var prevError time.Duration

	timeoutTimer := time.NewTimer(timeoutDur)
	timeoutTimer.Stop()

	for {
		select {
		case pkt := <-packetChan:
			packetCount++
			isFirstPacket := lastPacketTime.IsZero()
			var delta time.Duration

			if !isFirstPacket {
				// We use the highest precision time available for our math
				delta = pkt.BestTime.Sub(lastPacketTime)
			}

			lastPacketTime = pkt.BestTime

			if cfg.TimeoutExit {
				timeoutTimer.Reset(timeoutDur)
			}

			// Calculate delay between kernel reception and Go wake-up
			osDelayUs := pkt.GoTime.Sub(pkt.KernelTime).Microseconds()
			if pkt.KernelTime.IsZero() {
				osDelayUs = 0
			}

			// Verbose Level 2: Log every packet with timing specifics
			if cfg.Verbose >= 2 {
				logArgs := []any{
					"packet_count", packetCount,
					"remote_addr", pkt.RemoteAddr,
					"length", pkt.Length,
					"os_delay_us", osDelayUs,
					"go_time", pkt.GoTime.Format(time.RFC3339Nano),
				}

				if !pkt.KernelTime.IsZero() {
					logArgs = append(logArgs, "kernel_time", pkt.KernelTime.Format(time.RFC3339Nano))
				}

				if isFirstPacket {
					logArgs = append(logArgs, "info", "first_packet")
					slog.Info("packet_received", logArgs...)
				} else {
					logArgs = append(logArgs, "delta_ms", delta.Milliseconds())
					slog.Info("packet_received", logArgs...)
				}
			}

			if isFirstPacket {
				continue
			}

			minExpected := expectedPeriod - jitterDur
			maxExpected := expectedPeriod + jitterDur
			missedThreshold := (2 * expectedPeriod) - jitterDur

			if packetCount > uint64(cfg.WarmupPackets) {
				adjustedDelta := delta + prevError
				currentError := delta - expectedPeriod

				if delta >= missedThreshold {
					estimatedMissed := int((delta + jitterDur) / expectedPeriod) - 1
					if estimatedMissed < 1 {
						estimatedMissed = 1
					}

					slog.Error("missed_packet",
						"packet_count", packetCount,
						"time", pkt.BestTime.Format(time.RFC3339Nano),
						"remote_addr", pkt.RemoteAddr,
						"expected_ms", expectedPeriod.Milliseconds(),
						"actual_ms", delta.Milliseconds(),
						"estimated_missed_count", estimatedMissed)

					expectedMultiPeriod := expectedPeriod * time.Duration(estimatedMissed+1)
					prevError = delta - expectedMultiPeriod

				} else if delta < minExpected || delta > maxExpected {
					if adjustedDelta >= minExpected && adjustedDelta <= maxExpected {
						if cfg.Verbose >= 1 {
							slog.Info("jitter_suppressed",
								"packet_count", packetCount,
								"delta_ms", delta.Milliseconds(),
								"adjusted_delta_ms", adjustedDelta.Milliseconds(),
								"reason", "compensated by previous packet offset")
						}
						prevError = 0
					} else {
						slog.Warn("jitter_violation",
							"packet_count", packetCount,
							"time", pkt.BestTime.Format(time.RFC3339Nano),
							"remote_addr", pkt.RemoteAddr,
							"data_length_bytes", pkt.Length,
							"expected_ms", expectedPeriod.Milliseconds(),
							"actual_ms", delta.Milliseconds(),
							"adjusted_ms", adjustedDelta.Milliseconds(),
							"allowed_jitter_ms", cfg.JitterMs)

						prevError = currentError
					}
				} else {
					prevError = currentError
				}

			} else if cfg.Verbose >= 1 && packetCount > 1 {
				slog.Info("warmup_phase", "packet_count", packetCount, "delta_ms", delta.Milliseconds())
				prevError = 0
			}

		case <-timeoutTimer.C:
			if cfg.TimeoutExit {
				slog.Error("No data received for 100 epochs. Exiting.", "timeout_duration_sec", timeoutDur.Seconds())
				os.Exit(1)
			}
		}
	}
}
