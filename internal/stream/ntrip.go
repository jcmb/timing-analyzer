package stream

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/parser"
)

// StartNTRIPClient handles standard TCP streams and NTRIP casters
func StartNTRIPClient(ctx context.Context, cfg core.Config, packetChan chan<- core.PacketEvent, telemetryChan chan<- core.TelemetryEvent) {
	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	slog.Info("Starting Web TCP/NTRIP client", "target", address, "session", cfg.SessionID)

	dcolParser := &parser.DCOLParser{}

	// Helper to push connection errors directly to the Web UI
	reportFatal := func(msg string) {
		if telemetryChan != nil {
			select {
			case telemetryChan <- core.TelemetryEvent{
				Timestamp:  time.Now().Format("15:04:05.000000"),
				DisplayKey: "SYSTEM",
				Status:     "fatal", // Custom status for the UI to catch
				Message:    msg,
			}:
			default:
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			slog.Warn("Failed to connect", "error", err, "session", cfg.SessionID)
			reportFatal(fmt.Sprintf("Connection Failed: %v", err))
			return // Exit immediately so the user can fix their setup
		}

		// Perform NTRIP Handshake if a mountpoint is requested
		if cfg.Mountpoint != "" {

			// DEBUG LOG: Print exact auth details to terminal
			slog.Info("Executing NTRIP v2 Handshake",
				"host_header", address,
				"mountpoint", cfg.Mountpoint,
				"username", cfg.Username,
				"password", cfg.Password,
				"session", cfg.SessionID,
			)

			// Construct HTTP/1.1 Request with the mandatory NTRIP v2 headers
			req := fmt.Sprintf("GET /%s HTTP/1.1\r\n", cfg.Mountpoint)
			req += fmt.Sprintf("Host: %s\r\n", address)
			req += "Ntrip-Version: Ntrip/2.0\r\n"
			req += "User-Agent: NTRIP TimingAnalyzer/2.0\r\n"
			req += "Accept: */*\r\n"
			req += "Connection: close\r\n"

			if cfg.Username != "" {
				auth := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
				req += fmt.Sprintf("Authorization: Basic %s\r\n", auth)
			}
			req += "\r\n"

			conn.Write([]byte(req))

			reader := bufio.NewReader(conn)
			line, err := reader.ReadString('\n')
			if err != nil {
				slog.Error("Failed to read NTRIP response", "error", err, "session", cfg.SessionID)
				reportFatal("Failed to read NTRIP response from caster")
				conn.Close()
				return
			}

			// Check for both HTTP/1.1 200 OK (v2) and ICY 200 OK (fallback v1)
			if !strings.Contains(line, "200 OK") && !strings.Contains(line, "ICY 200 OK") {
				// Clean up the HTTP prefix for a nicer UI error (e.g., "HTTP/1.1 401 Unauthorized" -> "401 Unauthorized")
				errMsg := strings.TrimSpace(line)
				errMsg = strings.TrimPrefix(errMsg, "HTTP/1.1 ")
				errMsg = strings.TrimPrefix(errMsg, "HTTP/1.0 ")

				slog.Error("NTRIP connection rejected", "response", errMsg, "session", cfg.SessionID)
				reportFatal(fmt.Sprintf("Caster Rejected Connection: %s", errMsg))
				conn.Close()
				return // Exit immediately on 401/404 errors
			}
			slog.Info("NTRIP connected successfully", "mountpoint", cfg.Mountpoint, "session", cfg.SessionID)

			// Consume remaining HTTP headers before jumping into binary processing
			for {
				headerLine, err := reader.ReadString('\n')
				if err != nil || headerLine == "\r\n" || headerLine == "\n" {
					break
				}
			}
		}

		buf := make([]byte, 2048)
		for {
			// Check if the user closed the browser tab
			select {
			case <-ctx.Done():
				conn.Close()
				return
			default:
			}

			// 10 second timeout on read so we don't hang forever if the caster stops sending
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			n, err := conn.Read(buf)
			goTime := time.Now()

			if err != nil {
				slog.Info("Connection closed or timed out", "error", err, "session", cfg.SessionID)
				conn.Close()
				break // Break inner loop to trigger a reconnect or exit
			}

			if cfg.Decode == "dcol" || cfg.Decode == "mb-cmr" {
				dcolParser.Process(buf[:n], goTime, goTime, time.Time{}, cfg.Host, cfg.Verbose, packetChan)
			} else {
				packetChan <- core.PacketEvent{
					BestTime:      goTime,
					GoTime:        goTime,
					Length:        n,
					RemoteAddr:    cfg.Host,
					PacketType:    -1,
					IsLastInBurst: true,
				}
			}
		}

		// Small sleep before attempting to reconnect a dropped stream
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}
