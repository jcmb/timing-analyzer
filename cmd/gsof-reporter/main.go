package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/stream"
)

// Nagios Exit Codes
const (
	OK       = 0
	WARNING  = 1
	CRITICAL = 2
	UNKNOWN  = 3
)

type GSOFStats struct {
	mu             sync.Mutex
	counts         map[int]int
	hz             map[int]float64
	displayHz      map[int]string
	lastSeen       map[int]time.Time
	lastSeqSeen    map[int]uint8 // Track the last sequence number for EACH subtype
	lastSeq        uint8
	lastSeqTime    time.Time
	hasSeenType01  bool
	warnings       []string
	suppressSingle bool
}

var allowedPeriods = []float64{
	0.02, 0.05, 0.1, 0.2, 0.5, 1.0,
	2.0, 5.0, 10.0, 15.0, 30.0, 60.0, 300.0, 600.0,
}

func snapToNearestRate(delta float64) (float64, string) {
	closest := allowedPeriods[0]
	minDiff := math.Abs(delta - closest)
	for _, p := range allowedPeriods {
		diff := math.Abs(delta - p)
		if diff < minDiff {
			minDiff = diff
			closest = p
		}
	}
	if closest < 1.0 {
		hz := 1.0 / closest
		return hz, fmt.Sprintf("%.0f Hz", hz)
	}
	return 1.0 / closest, fmt.Sprintf("%.0f sec", closest)
}

func (s *GSOFStats) Update(seq uint8, buffer []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 1. Core Sequence Tracking (Transport Layer)
	if !s.lastSeqTime.IsZero() {
		expectedSeq := s.lastSeq + 1
		if seq != expectedSeq && seq != s.lastSeq {
			gap := (int(seq) + 256 - int(expectedSeq)) % 256
			if !(s.suppressSingle && gap == 1) {
				s.warnings = append(s.warnings, fmt.Sprintf("[%s] WARNING: Sequence Gap! Jumped %d to %d (Missed %d)",
					now.Format("15:04:05"), s.lastSeq, seq, gap))
			}
		}
	}
	s.lastSeq = seq
	s.lastSeqTime = now

	// 2. Identify subtypes and check for 0x01
	isType01Present := false
	var packetSubtypes []string
	ptr := 0

	// Temporary map to ensure we only count a subtype once per packet for rate calculations
	seenInThisPacket := make(map[int]bool)

	for ptr < len(buffer) {
		if ptr+2 > len(buffer) { break }
		recType := int(buffer[ptr])
		recLen := int(buffer[ptr+1])

		if recType == 0x01 { isType01Present = true }
		packetSubtypes = append(packetSubtypes, fmt.Sprintf("0x%02X", recType))

		s.counts[recType]++

		// RATE CALCULATION FIX:
		// Only update the timing/Hz if this is a NEW sequence for this subtype.
		// This prevents 20Hz reports when 10Hz packets contain two records of the same type.
		if !seenInThisPacket[recType] {
			if lastTime, exists := s.lastSeen[recType]; exists {
				delta := now.Sub(lastTime).Seconds()
				hzVal, hzStr := snapToNearestRate(delta)
				s.hz[recType] = hzVal
				s.displayHz[recType] = hzStr
			}
			s.lastSeen[recType] = now
			seenInThisPacket[recType] = true
		}

		ptr += 2 + recLen
	}

	if isType01Present {
		s.hasSeenType01 = true
	} else {
		s.warnings = append(s.warnings, fmt.Sprintf("[%s] WARNING: Packet (Seq %d) missing Type 0x01. Found: [%s]",
			now.Format("15:04:05"), seq, strings.Join(packetSubtypes, ", ")))
	}
}

func parseExpectedRates(path string) (map[int]float64, error) {
	rates := make(map[int]float64)
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") { continue }

		parts := strings.Fields(line)
		if len(parts) < 2 { continue }

		var subType int
		fmt.Sscanf(parts[0], "0x%x", &subType)
		if subType == 0 && parts[0] != "0" && parts[0] != "0x00" { fmt.Sscanf(parts[0], "%d", &subType) }

		rateStr := strings.ToLower(parts[1])
		var val float64
		if strings.HasSuffix(rateStr, "hz") {
			fmt.Sscanf(strings.TrimSuffix(rateStr, "hz"), "%f", &val)
		} else if strings.HasSuffix(rateStr, "s") {
			fmt.Sscanf(strings.TrimSuffix(rateStr, "s"), "%f", &val)
			if val > 0 { val = 1.0 / val }
		} else {
			fmt.Sscanf(rateStr, "%f", &val)
		}
		rates[subType] = val
	}
	return rates, nil
}

func main() {
	ipFlag := flag.String("ip", "udp", "Protocol: tcp or udp")
	host := flag.String("host", "", "Target IP")
	port := flag.Int("port", 2101, "Port")
	verbose := flag.Int("verbose", 0, "Verbosity level")
	suppress := flag.Bool("suppress-single", false, "Suppress single missed sequence warnings")
	nagios := flag.Bool("nagios", false, "Enable Nagios check mode")
	rateFile := flag.String("expected-rates", "", "Path to expected rates config file")
	strict := flag.Bool("strict", false, "Fail if unexpected subtypes are found")
	flag.Parse()

	cfg := core.Config{IP: *ipFlag, Host: *host, Port: *port, Decode: "dcol", Verbose: *verbose}
	if cfg.Host != "" { cfg.IP = "tcp" }

	stats := &GSOFStats{
		counts:      make(map[int]int),
		hz:          make(map[int]float64),
		displayHz:   make(map[int]string),
		lastSeen:    make(map[int]time.Time),
		lastSeqSeen: make(map[int]uint8),
		suppressSingle: *suppress,
	}
	packetChan := make(chan core.PacketEvent, 1000)

	go stream.StartListener(cfg, packetChan)

	go func() {
		for pkt := range packetChan {
			if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
				stats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer)
			}
		}
	}()

	if *nagios {
		expected, err := parseExpectedRates(*rateFile)
		if err != nil {
			fmt.Printf("UNKNOWN: Failed to read rate file: %v\n", err)
			os.Exit(UNKNOWN)
		}

		time.Sleep(5 * time.Second)
		stats.mu.Lock()
		defer stats.mu.Unlock()

		var errors []string
		for subType, expRate := range expected {
			actualRate, seen := stats.hz[subType]
			if !seen || actualRate == 0 {
				errors = append(errors, fmt.Sprintf("Type 0x%02X not found", subType))
				continue
			}
			if math.Abs(actualRate-expRate) > (expRate * 0.1) {
				errors = append(errors, fmt.Sprintf("Type 0x%02X rate mismatch (Exp: %.1fHz, Got: %.1fHz)", subType, expRate, actualRate))
			}
		}

		if *strict {
			for subType := range stats.counts {
				if _, ok := expected[subType]; !ok {
					errors = append(errors, fmt.Sprintf("Unexpected subtype 0x%02X", subType))
				}
			}
		}

		if len(errors) > 0 {
			fmt.Printf("CRITICAL: %s\n", strings.Join(errors, " | "))
			os.Exit(CRITICAL)
		}

		fmt.Println("OK: All GSOF rates within tolerance")
		os.Exit(OK)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		stats.mu.Lock()
		if stats.hasSeenType01 && time.Since(stats.lastSeqTime) > 10*time.Second {
			fmt.Printf("\n[FATAL] Stream Loss: GSOF transport timed out.\n")
			os.Exit(1)
		}

		if *verbose == 0 {
			fmt.Print("\033[H\033[2J")
			fmt.Println("==========================================================")
			fmt.Printf(" Trimble GSOF Reporter | Heartbeat: Type 0x01\n")
			fmt.Printf(" Mode: %-5s | Port: %-5d | Last Seq: %-3d\n", cfg.IP, cfg.Port, stats.lastSeq)
			fmt.Println("==========================================================")
			fmt.Printf("%-22s | %-12s | %-12s\n", "GSOF Record Type", "Count", "Rate")
			fmt.Println("----------------------------------------------------------")

			keys := make([]int, 0, len(stats.counts))
			for k := range stats.counts { keys = append(keys, k) }
			sort.Ints(keys)

			for _, subType := range keys {
				rateStr := stats.displayHz[subType]
				if rateStr == "" { rateStr = "calc..." }
				if stats.hz[subType] > 0 && time.Since(stats.lastSeen[subType]).Seconds() > (1.0/stats.hz[subType])*3.0 {
					rateStr = "stale"
				}
				fmt.Printf("Type 0x%02X (%-3d)       | %-12d | %-12s\n",
					subType, subType, stats.counts[subType], rateStr)
			}
		}

		if len(stats.warnings) > 0 {
			if *verbose == 0 { fmt.Println("\nRecent Warnings/Errors:") }
			start := 0
			if len(stats.warnings) > 10 { start = len(stats.warnings) - 10 }
			for i := start; i < len(stats.warnings); i++ {
				fmt.Printf(" %s\n", stats.warnings[i])
			}
			if *verbose > 0 { stats.warnings = nil }
		}
		stats.mu.Unlock()
	}
}
