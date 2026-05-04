package stream

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/parser"
)

// runTCPOutboundClient dials cfg.Host:cfg.Port until ctx is cancelled: backoff on dial errors,
// reconnect after each session ends (EOF, reset, close). Uses TCP keepalives so dead peers are
// detected without relying only on application reads.
func runTCPOutboundClient(ctx context.Context, cfg core.Config, packetChan chan<- core.PacketEvent) error {
	host := strings.TrimSpace(cfg.Host)
	address := fmt.Sprintf("%s:%d", host, cfg.Port)
	slog.Info("Starting TCP client mode (auto-reconnect)", "target", address)

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	const reconnectPause = time.Second

	var mu sync.Mutex
	var cur net.Conn
	go func() {
		<-ctx.Done()
		mu.Lock()
		if cur != nil {
			_ = cur.Close()
		}
		mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Warn("TCP dial failed; retrying", "target", address, "error", err, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = time.Second

		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(30 * time.Second)
		}

		mu.Lock()
		cur = conn
		mu.Unlock()

		handleTCPConn(conn, cfg, packetChan)

		mu.Lock()
		cur = nil
		mu.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(reconnectPause):
		}
		slog.Warn("TCP session ended; reconnecting", "target", address)
	}
}

// StartListenerContext runs the same transports as StartListener until ctx is cancelled.
// It returns nil when ctx is done. A non-nil error means setup failed (e.g. bind).
// If onUDPLocalPort is non-nil, it is called once with the bound UDP port immediately after ListenUDP succeeds.
func StartListenerContext(ctx context.Context, cfg core.Config, packetChan chan<- core.PacketEvent, onUDPLocalPort func(port int)) error {
	proto := strings.ToLower(strings.TrimSpace(cfg.IP))
	switch proto {
	case "tcp":
		if cfg.Host != "" {
			return runTCPOutboundClient(ctx, cfg, packetChan)
		}
		// Bind IPv4 explicitly so receivers sending to a public IPv4 reach this process; a bare
		// ":port" often resolves to [::]:port only, which does not accept IPv4 UDP/TCP on many hosts.
		address := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
		l, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("tcp listen %s: %w", address, err)
		}
		go func() {
			<-ctx.Done()
			_ = l.Close()
		}()
		slog.Info("Listening for TCP connections", "port", cfg.Port)
		for {
			conn, err := l.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				continue
			}
			go handleTCPConn(conn, cfg, packetChan)
		}

	case "udp":
		// Use IPv4 any (udp4) so hub / embedded UDP listen matches receivers aimed at x.x.x.x:port.
		// ":port" commonly becomes [::]:port, which skips IPv4 datagrams on typical Linux/macOS setups.
		address := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
		addr, err := net.ResolveUDPAddr("udp4", address)
		if err != nil {
			return fmt.Errorf("udp resolve %s: %w", address, err)
		}
		conn, err := net.ListenUDP("udp4", addr)
		if err != nil {
			return fmt.Errorf("udp listen %s: %w", address, err)
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		if la := conn.LocalAddr(); la != nil {
			slog.Info("Listening for UDP packets", "addr", la.String())
			if onUDPLocalPort != nil {
				if u, ok := la.(*net.UDPAddr); ok && u != nil {
					onUDPLocalPort(u.Port)
				}
			}
		} else {
			slog.Info("Listening for UDP packets", "port", cfg.Port)
		}

		// Kernel SCM timestamps require recvmsg (ReadMsgUDP). On several platforms that path
		// returns immediate errors while ReadFromUDP works; we prefer reliable delivery for
		// DCOL/GSOF and use wall-clock time for UDP (same as TCP path).
		buf := make([]byte, 65536)
		parsers := make(map[string]*parser.DCOLParser)

		for {
			if ctx.Err() != nil {
				return nil
			}
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			goTime := time.Now()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				slog.Warn("UDP read failed", "error", err)
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(50 * time.Millisecond):
				}
				continue
			}
			if n <= 0 || remoteAddr == nil {
				continue
			}

			ip := remoteAddr.String()
			var kernelTime time.Time
			bestTime := goTime

			if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
				if parsers[ip] == nil {
					parsers[ip] = &parser.DCOLParser{}
				}
				parsers[ip].Process(buf[:n], bestTime, goTime, kernelTime, ip, cfg, packetChan)
			} else {
				packetChan <- core.PacketEvent{
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

	default:
		return fmt.Errorf("invalid -ip protocol (use tcp or udp): %q", cfg.IP)
	}
}

// StartListener runs until process exit on fatal bind errors (same as historical behavior).
// Cancel is not supported; use StartListenerContext for cancellable sessions.
func StartListener(cfg core.Config, packetChan chan<- core.PacketEvent) {
	go func() {
		err := StartListenerContext(context.Background(), cfg, packetChan, nil)
		if err != nil {
			slog.Error("stream listener failed", "error", err)
			os.Exit(1)
		}
	}()
}

func handleTCPConn(conn net.Conn, cfg core.Config, packetChan chan<- core.PacketEvent) {
	defer conn.Close()
	buf := make([]byte, 2048)
	remoteIP := conn.RemoteAddr().String()
	slog.Info("TCP connection established", "remote_addr", remoteIP)

	dcolParser := &parser.DCOLParser{}

	for {
		n, err := conn.Read(buf)
		goTime := time.Now()
		if err != nil {
			slog.Info("TCP connection closed", "remote_addr", remoteIP)
			return
		}
		if n == 0 {
			continue
		}

		if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
			dcolParser.Process(buf[:n], goTime, goTime, time.Time{}, remoteIP, cfg, packetChan)
		} else {
			packetChan <- core.PacketEvent{
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
