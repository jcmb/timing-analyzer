package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"timing-analyzer/internal/core"
	"timing-analyzer/internal/gsofstats"
	"timing-analyzer/internal/stream"
)

// Nagios Exit Codes
const (
	OK       = 0
	WARNING  = 1
	CRITICAL = 2
	UNKNOWN  = 3
)

func parseExpectedRates(path string) (map[int]float64, error) {
	rates := make(map[int]float64)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		var subType int
		fmt.Sscanf(parts[0], "0x%x", &subType)
		if subType == 0 && parts[0] != "0" && parts[0] != "0x00" {
			fmt.Sscanf(parts[0], "%d", &subType)
		}

		rateStr := strings.ToLower(parts[1])
		var val float64
		if strings.HasSuffix(rateStr, "hz") {
			fmt.Sscanf(strings.TrimSuffix(rateStr, "hz"), "%f", &val)
		} else if strings.HasSuffix(rateStr, "s") {
			fmt.Sscanf(strings.TrimSuffix(rateStr, "s"), "%f", &val)
			if val > 0 {
				val = 1.0 / val
			}
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
	suppress := flag.Bool("suppress-single", false, "Suppress single missed sequence warnings (UDP)")
	ignoreTCPGSOFGap1 := flag.Bool("ignore-tcp-gsof-transmission-gap1", false, "TCP only: enable GSOF transmission gap warnings but suppress when exactly one id is skipped")
	nagios := flag.Bool("nagios", false, "Enable Nagios check mode")
	rateFile := flag.String("expected-rates", "", "Path to expected rates config file")
	strict := flag.Bool("strict", false, "Fail if unexpected subtypes are found")
	flag.Parse()

	cfg := core.Config{
		IP:                            *ipFlag,
		Host:                          *host,
		Port:                          *port,
		Decode:                        "dcol",
		Verbose:                       *verbose,
		IgnoreTCPGSOFTransmissionGap1: *ignoreTCPGSOFGap1,
	}
	if cfg.Host != "" {
		cfg.IP = "tcp"
	}

	stats := gsofstats.NewStats(*suppress)
	packetChan := make(chan core.PacketEvent, 1000)

	go stream.StartListener(cfg, packetChan)

	go func() {
		for pkt := range packetChan {
			for _, w := range pkt.StreamWarnings {
				stats.AddWarning(w)
			}
			tcp := !strings.EqualFold(cfg.IP, "udp")
			if pkt.PacketType == 0x40 && len(pkt.GSOFBuffer) > 0 {
				stats.Update(uint8(pkt.SequenceNumber), pkt.GSOFBuffer, tcp, cfg.IgnoreTCPGSOFTransmissionGap1)
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
		hz, counts := stats.ExportNagios()

		var errors []string
		for subType, expRate := range expected {
			actualRate, seen := hz[subType]
			if !seen || actualRate == 0 {
				errors = append(errors, fmt.Sprintf("Type 0x%02X not found", subType))
				continue
			}
			if math.Abs(actualRate-expRate) > (expRate * 0.1) {
				errors = append(errors, fmt.Sprintf("Type 0x%02X rate mismatch (Exp: %.1fHz, Got: %.1fHz)", subType, expRate, actualRate))
			}
		}

		if *strict {
			for subType := range counts {
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
		if stats.StreamLost() {
			fmt.Printf("\n[FATAL] Stream Loss: GSOF transport timed out.\n")
			os.Exit(1)
		}

		dash := stats.BuildDashboard(cfg.IP, cfg.Port, "", cfg.Host)

		if *verbose == 0 {
			fmt.Print("\033[H\033[2J")
			fmt.Println("==========================================================")
			fmt.Printf(" Trimble GSOF Reporter | Heartbeat: Type 0x01\n")
			fmt.Printf(" Mode: %-5s | Port: %-5d | Last Seq: %-3d\n", cfg.IP, cfg.Port, dash.LastSeq)
			fmt.Println("==========================================================")
			fmt.Printf("%-22s | %-12s | %-12s\n", "GSOF Record Type", "Count", "Rate")
			fmt.Println("----------------------------------------------------------")

			for _, row := range dash.Records {
				fmt.Printf("Type %s (%-3d)       | %-12d | %-12s\n",
					row.TypeHex, row.Type, row.Count, row.Rate)
			}
		}

		if len(dash.Warnings) > 0 {
			if *verbose == 0 {
				fmt.Println("\nRecent Warnings/Errors:")
			}
			start := 0
			if len(dash.Warnings) > 10 {
				start = len(dash.Warnings) - 10
			}
			for i := start; i < len(dash.Warnings); i++ {
				fmt.Printf(" %s\n", dash.Warnings[i])
			}
			if *verbose > 0 {
				stats.ClearWarnings()
			}
		}
	}
}
