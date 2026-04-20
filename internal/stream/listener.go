package stream

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/parser"
)

func StartListener(cfg core.Config, packetChan chan<- core.PacketEvent) {
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
				if err := enableKernelTimestamps(fd); err == nil {
					slog.Info("Hardware/Kernel timestamping enabled")
				}
			})
		}

		buf := make([]byte, 2048)
		oob := make([]byte, 1024)
		parsers := make(map[string]*parser.DCOLParser)

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
				if kt, ok := extractKernelTimestamp(oob[:oobn]); ok {
					kernelTime = kt
					bestTime = kernelTime
				}
			}

			if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
				if parsers[ip] == nil {
					parsers[ip] = &parser.DCOLParser{}
				}
				parsers[ip].Process(buf[:n], bestTime, goTime, kernelTime, ip, cfg.Verbose, packetChan)
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
	}
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

		if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
			dcolParser.Process(buf[:n], goTime, goTime, time.Time{}, remoteIP, cfg.Verbose, packetChan)
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
