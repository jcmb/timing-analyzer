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

const historySamplesMax = 2000

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
	// lastGPSTOWSec is the GPS time-of-week (s) from the latest type-0x01 in stream order.
	lastGPSTOWSec float64
	// tangentHistory holds recent type-0x07 samples paired with lastGPSTOWSec at decode time.
	tangentHistory []gsof.TangentPlanePoint
	// llhHistory holds recent type-0x02 lat/lon samples paired with lastGPSTOWSec at decode time.
	llhHistory []gsof.LLHPoint
	// dopHistory holds recent type-0x09 DOP samples paired with lastGPSTOWSec.
	dopHistory []gsof.DOPPoint
	// sigmaHistory holds recent type-0x0C sigma samples paired with lastGPSTOWSec.
	sigmaHistory []gsof.SigmaPoint
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

		if recType == 1 {
			if sec, ok := gsof.ParsePositionTimeTOWSec(pld); ok {
				s.lastGPSTOWSec = sec
			}
		}
		if recType == 7 {
			if de, dn, du, ok := gsof.ParseTangentPlaneENU(pld); ok {
				s.appendTangentPoint(gsof.TangentPlanePoint{
					GPSTOWSec: s.lastGPSTOWSec,
					DEm:       de,
					DNm:       dn,
					DUm:       du,
				})
			}
		}
		if recType == 2 {
			if lat, lon, h, ok := gsof.ParseLLHDeg(pld); ok {
				s.appendLLHPoint(gsof.LLHPoint{
					GPSTOWSec: s.lastGPSTOWSec,
					LatDeg:    lat,
					LonDeg:    lon,
					HeightM:   h,
				})
			}
		}
		if recType == 9 {
			if pt, ok := gsof.ParseDOPPoint(pld); ok {
				pt.GPSTOWSec = s.lastGPSTOWSec
				s.appendDOPPoint(pt)
			}
		}
		if recType == 12 {
			if pt, ok := gsof.ParseSigmaPoint(pld); ok {
				pt.GPSTOWSec = s.lastGPSTOWSec
				s.appendSigmaPoint(pt)
			}
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

func (s *Stats) appendTangentPoint(pt gsof.TangentPlanePoint) {
	s.tangentHistory = append(s.tangentHistory, pt)
	if len(s.tangentHistory) > historySamplesMax {
		s.tangentHistory = s.tangentHistory[len(s.tangentHistory)-historySamplesMax:]
	}
}

func (s *Stats) appendLLHPoint(pt gsof.LLHPoint) {
	s.llhHistory = append(s.llhHistory, pt)
	if len(s.llhHistory) > historySamplesMax {
		s.llhHistory = s.llhHistory[len(s.llhHistory)-historySamplesMax:]
	}
}

func (s *Stats) appendDOPPoint(pt gsof.DOPPoint) {
	s.dopHistory = append(s.dopHistory, pt)
	if len(s.dopHistory) > historySamplesMax {
		s.dopHistory = s.dopHistory[len(s.dopHistory)-historySamplesMax:]
	}
}

func (s *Stats) appendSigmaPoint(pt gsof.SigmaPoint) {
	s.sigmaHistory = append(s.sigmaHistory, pt)
	if len(s.sigmaHistory) > historySamplesMax {
		s.sigmaHistory = s.sigmaHistory[len(s.sigmaHistory)-historySamplesMax:]
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
	// SVBrief is populated for GSOF type 13 (GPS SV brief) for structured dashboard views.
	SVBrief []gsof.SVBriefEntry `json:"sv_brief,omitempty"`
	// AllSVBrief is populated for GSOF type 33 (all systems SV brief) for structured dashboard views.
	AllSVBrief []gsof.AllSVBriefEntry `json:"all_sv_brief,omitempty"`
	// SVDetailed is populated for GSOF type 14 (detailed satellite info).
	SVDetailed []gsof.SVDetailedEntry `json:"sv_detailed,omitempty"`
	// TangentHistory is populated for GSOF type 7 (tangent plane delta) for dashboard graphing.
	TangentHistory []gsof.TangentPlanePoint `json:"tangent_history,omitempty"`
	// LLHHistory is populated for GSOF type 2 (latitude / longitude) for dashboard graphing.
	LLHHistory []gsof.LLHPoint `json:"llh_history,omitempty"`
	// DOPHistory is populated for GSOF type 9 (DOP) for dashboard graphing.
	DOPHistory []gsof.DOPPoint `json:"dop_history,omitempty"`
	// SigmaHistory is populated for GSOF type 12 (position RMS / sigmas) for dashboard graphing.
	SigmaHistory []gsof.SigmaPoint `json:"sigma_history,omitempty"`
}

// DashboardPayload is JSON for the web UI / SSE.
type DashboardPayload struct {
	ServerTime       string      `json:"server_time"`
	LastSeq          int         `json:"last_seq"`
	Mode             string      `json:"mode"`
	Port             int         `json:"port"`
	Records          []RecordRow `json:"records"`
	Warnings         []string    `json:"warnings"`
	StreamOK                   bool   `json:"stream_ok"`
	DashboardVersion           string `json:"dashboard_version,omitempty"`
	ShowExpectedReservedBits   bool   `json:"show_expected_reserved_bits"`
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
		row := RecordRow{
			Type:     subType,
			TypeHex:  fmt.Sprintf("0x%02X", subType),
			Name:     meta.Title,
			Function: meta.Function,
			DocURL:   meta.DocURL(),
			Fields:   fields,
			Count:    s.counts[subType],
			Rate:     rateStr,
			Stale:    stale,
		}
		if subType == 13 {
			_, row.SVBrief = gsof.ParseSVBriefEntries(payload)
		}
		if subType == 33 {
			_, row.AllSVBrief = gsof.ParseAllSVBriefEntries(payload)
		}
		if subType == 14 {
			_, row.SVDetailed = gsof.ParseSVDetailedEntries(payload)
		}
		if subType == 7 && len(s.tangentHistory) > 0 {
			row.TangentHistory = append([]gsof.TangentPlanePoint(nil), s.tangentHistory...)
		}
		if subType == 2 && len(s.llhHistory) > 0 {
			row.LLHHistory = append([]gsof.LLHPoint(nil), s.llhHistory...)
		}
		if subType == 9 && len(s.dopHistory) > 0 {
			row.DOPHistory = append([]gsof.DOPPoint(nil), s.dopHistory...)
		}
		if subType == 12 && len(s.sigmaHistory) > 0 {
			row.SigmaHistory = append([]gsof.SigmaPoint(nil), s.sigmaHistory...)
		}
		rows = append(rows, row)
	}

	warn := append([]string(nil), s.warnings...)
	if len(warn) > 50 {
		warn = warn[len(warn)-50:]
	}

	return &DashboardPayload{
		ServerTime:                 now.Format(time.RFC3339Nano),
		LastSeq:                    int(s.lastSeq),
		Mode:                       mode,
		Port:                       port,
		Records:                    rows,
		Warnings:                   warn,
		StreamOK:                   streamOK,
		DashboardVersion:           dashboardVersion,
		ShowExpectedReservedBits:   gsof.ShowExpectedReservedBits,
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
