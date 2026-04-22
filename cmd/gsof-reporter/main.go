package main

import (
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

type GSOFStats struct {
	mu            sync.Mutex
	counts        map[int]int
	hz            map[int]float64
	displayHz     map[int]string
	lastSeen      map[int]time.Time
	lastSeq       uint8
	lastSeqTime   time.Time
	hasSeenType01 bool
	warnings      []string
	suppressSingle bool // Option to suppress single-sequence gaps
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

	// 1. Core Sequence Tracking
	if !s.lastSeqTime.IsZero() {
		expectedSeq := s.lastSeq + 1
		if seq != expectedSeq && seq != s.lastSeq {
			gap := (int(seq) + 256 - int(expectedSeq)) % 256

			// Option to suppress if gap is only 1 missed epoch
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
	for ptr < len(buffer) {
		if ptr+2 > len(buffer) { break }
		recType := int(buffer[ptr])
		recLen := int(buffer[ptr+1])

		if recType == 0x01 { isType01Present = true }
		packetSubtypes = append(packetSubtypes, fmt.Sprintf("0x%02X", recType))

		s.counts[recType]++
		if lastTime, exists := s.lastSeen[recType]; exists {
			delta := now.Sub(lastTime).Seconds()
			hzVal, hzStr := snapToNearestRate(delta)
			s.hz[recType] = hzVal
			s.displayHz[recType] = hzStr
		}
		s.lastSeen[recType] = now

		ptr += 2 + recLen
	}

	if isType01Present {
		s.hasSeenType01 = true
	} else {
		s.warnings = append(s.warnings, fmt.Sprintf("[%s] WARNING: Packet (Seq %d) missing Type 0x01 record. Found: [%s]",
			now.Format("15:04:05"), seq, strings.Join(packetSubtypes, ", ")))
	}
}

func main() {
	ipFlag := flag.String("ip", "udp", "Protocol: tcp or udp")
	host := flag.String("host", "", "Target IP")
	port := flag.Int("port", 2101, "Port")
	verbose := flag.Int("verbose", 0, "Verbosity level")
	suppress := flag.Bool("suppress-single", false, "Suppress warnings for single missed sequence numbers")
	flag.Parse()

	cfg := core.Config{IP: *ipFlag, Host: *host, Port: *port, Decode: "dcol", Verbose: *verbose}
	if cfg.Host != "" { cfg.IP = "tcp" }

	stats := &GSOFStats{
		counts:         make(map[int]int),
		hz:             make(map[int]float64),
		displayHz:      make(map[int]string),
		lastSeen:       make(map[int]time.Time),
		suppressSingle: *suppress,
	}
	packetChan := make(chan core.PacketEvent, 1000)

	go stream.StartListener(cfg, packetChan)

	go func() {
		time.Sleep(5 * time.Second)
		stats.mu.Lock()
		if !stats.hasSeenType01 {
			fmt.Printf("\n[FATAL] Heartbeat Failure: Type 0x01 not received within 5s.\n")
			os.Exit(1)
		}
		stats.mu.Unlock()
	}()

	go func() {
		for pkt := range packetChan {
			if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
				stats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer)
			}
		}
	}()

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
