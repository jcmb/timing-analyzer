package timing

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"timing-analyzer/internal/core"
)

type TimingState struct {
	LastPacketTime time.Time
	PacketCount    uint64
	PrevError      time.Duration
}

func logAndEmit(telemetryChan chan<- core.TelemetryEvent, csvWriter *csv.Writer, e core.LogEntry, allowedJitterMs float64) {
	timeStr := e.Time.Format("15:04:05.000000")
	niceName := core.GetNiceName(e.DisplayKey)

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

	if telemetryChan != nil {
		telemetryChan <- core.TelemetryEvent{
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

func Run(cfg core.Config, packetChan <-chan core.PacketEvent, telemetryChan chan<- core.TelemetryEvent) {
	baseExpectedPeriod := time.Duration(float64(time.Second) / cfg.RateHz)
	timeoutDur := baseExpectedPeriod * 100

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

			baseEntry := core.LogEntry{
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
					logAndEmit(telemetryChan, csvWriter, e, allowedJitterMs)
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
			logAndEmit(telemetryChan, csvWriter, eRx, allowedJitterMs)

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
					logAndEmit(telemetryChan, csvWriter, e, allowedJitterMs)

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
							logAndEmit(telemetryChan, csvWriter, e, allowedJitterMs)
						}
						state.PrevError = 0
					} else {
						e := baseEntry
						e.Level = "WARN"
						e.Event = "jitter_violation"
						e.HasAdj = true
						e.AdjDelta = adjustedDelta.Milliseconds()
						logAndEmit(telemetryChan, csvWriter, e, allowedJitterMs)
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
				logAndEmit(telemetryChan, csvWriter, e, allowedJitterMs)
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
