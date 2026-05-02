package gsofstats

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"timing-analyzer/internal/gsof"
)

// Stats tracks GSOF record subtypes, inferred rates, and transport warnings.
type Stats struct {
	mu             sync.Mutex
	counts         map[int]int
	hz             map[int]float64
	displayHz      map[int]string
	lastSeen       map[int]time.Time
	lastSeq        uint8
	lastSeqTime    time.Time
	hasSeenType01  bool
	warnings       []string
	suppressSingle bool
	lastPayload    map[int][]byte // last GSOF record payload per type (for dashboard decode)
}

func NewStats(suppressSingle bool) *Stats {
	return &Stats{
		counts:         make(map[int]int),
		hz:             make(map[int]float64),
		displayHz:      make(map[int]string),
		lastSeen:       make(map[int]time.Time),
		lastPayload:    make(map[int][]byte),
		suppressSingle: suppressSingle,
	}
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

// Update processes one reassembled GSOF payload and transport sequence number.
func (s *Stats) Update(seq uint8, buffer []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

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

	isType01Present := false
	var packetSubtypes []string
	ptr := 0
	seenInThisPacket := make(map[int]bool)

	for ptr < len(buffer) {
		if ptr+2 > len(buffer) {
			break
		}
		recType := int(buffer[ptr])
		recLen := int(buffer[ptr+1])

		if recType == 0x01 {
			isType01Present = true
		}
		packetSubtypes = append(packetSubtypes, fmt.Sprintf("0x%02X", recType))

		s.counts[recType]++

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

		if ptr+2+recLen > len(buffer) {
			break
		}
		pld := make([]byte, recLen)
		copy(pld, buffer[ptr+2:ptr+2+recLen])
		s.lastPayload[recType] = pld

		ptr += 2 + recLen
	}

	if isType01Present {
		s.hasSeenType01 = true
	} else {
		s.warnings = append(s.warnings, fmt.Sprintf("[%s] WARNING: Packet (Seq %d) missing Type 0x01. Found: [%s]",
			now.Format("15:04:05"), seq, strings.Join(packetSubtypes, ", ")))
	}
}

// RecordRow is one GSOF subtype row for dashboards and APIs.
type RecordRow struct {
	Type     int          `json:"type"`
	TypeHex  string       `json:"type_hex"`
	Name     string       `json:"name"`
	Function string       `json:"function"`
	DocURL   string       `json:"doc_url"`
	Fields   []gsof.Field `json:"fields"`
	Count    int          `json:"count"`
	Rate     string       `json:"rate"`
	Stale    bool         `json:"stale"`
}

// DashboardPayload is JSON for the web UI / SSE.
type DashboardPayload struct {
	ServerTime       string      `json:"server_time"`
	LastSeq          int         `json:"last_seq"`
	Mode             string      `json:"mode"`
	Port             int         `json:"port"`
	Records          []RecordRow `json:"records"`
	Warnings         []string    `json:"warnings"`
	StreamOK         bool        `json:"stream_ok"`
	DashboardVersion string      `json:"dashboard_version,omitempty"`
}

// BuildDashboard returns a snapshot for JSON/SSE (sorted by record type).
// dashboardVersion is the gsof-dashboard binary build (empty if not applicable).
func (s *Stats) BuildDashboard(mode string, port int, dashboardVersion string) *DashboardPayload {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	streamOK := !(s.hasSeenType01 && !s.lastSeqTime.IsZero() && now.Sub(s.lastSeqTime) > 10*time.Second)

	keys := make([]int, 0, len(s.counts))
	for k := range s.counts {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	rows := make([]RecordRow, 0, len(keys))
	for _, subType := range keys {
		rateStr := s.displayHz[subType]
		if rateStr == "" {
			rateStr = "calc..."
		}
		stale := false
		if s.hz[subType] > 0 && now.Sub(s.lastSeen[subType]).Seconds() > (1.0/s.hz[subType])*3.0 {
			rateStr = "stale"
			stale = true
		}
		meta := gsof.Lookup(subType)
		payload := s.lastPayload[subType]
		fields := gsof.Decode(subType, payload)
		rows = append(rows, RecordRow{
			Type:     subType,
			TypeHex:  fmt.Sprintf("0x%02X", subType),
			Name:     meta.Title,
			Function: meta.Function,
			DocURL:   meta.DocURL(),
			Fields:   fields,
			Count:    s.counts[subType],
			Rate:     rateStr,
			Stale:    stale,
		})
	}

	warn := append([]string(nil), s.warnings...)
	if len(warn) > 50 {
		warn = warn[len(warn)-50:]
	}

	return &DashboardPayload{
		ServerTime:       now.Format(time.RFC3339Nano),
		LastSeq:          int(s.lastSeq),
		Mode:             mode,
		Port:             port,
		Records:          rows,
		Warnings:         warn,
		StreamOK:         streamOK,
		DashboardVersion: dashboardVersion,
	}
}

// ExportNagios copies rate and count maps for Nagios checks (caller must not mutate).
func (s *Stats) ExportNagios() (hz map[int]float64, counts map[int]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hz = make(map[int]float64, len(s.hz))
	for k, v := range s.hz {
		hz[k] = v
	}
	counts = make(map[int]int, len(s.counts))
	for k, v := range s.counts {
		counts[k] = v
	}
	return hz, counts
}

// StreamLost returns true if Type 0x01 heartbeat was seen but stream timed out.
func (s *Stats) StreamLost() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSeenType01 || s.lastSeqTime.IsZero() {
		return false
	}
	return time.Since(s.lastSeqTime) > 10*time.Second
}

// WarningsTail returns up to the last n warnings (copy).
func (s *Stats) WarningsTail(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.warnings) == 0 {
		return nil
	}
	start := 0
	if len(s.warnings) > n {
		start = len(s.warnings) - n
	}
	out := make([]string, len(s.warnings)-start)
	copy(out, s.warnings[start:])
	return out
}

// ClearWarnings removes buffered warnings (e.g. verbose console mode).
func (s *Stats) ClearWarnings() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = nil
}
