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

// minWallDeltaForRateSample is the minimum wall-clock gap between successive
// observations of a subtype before we refresh its inferred rate (Hz). Bursts
// of multiple DCOL packets (e.g. TCP coalescing or channel backlog) can arrive
// a few milliseconds apart while the true GNSS cadence is much slower; without
// this floor, snapToNearestRate maps ~20 ms gaps to the 0.02 s bucket (50 Hz)
// and stale detection (which uses hz) then marks a healthy 10 Hz stream stale.
//
// For UDP loss, wall time spans many missed DCOL frames; we divide that wall time
// by the sequence advance since the subtype was last seen so each missed packet
// still counts toward cadence. We still require effectiveDelta (below) ≥ this
// floor so many distinct seq numbers delivered in one wall-clock burst (TCP)
// do not infer an unrealistically fast rate.
const minWallDeltaForRateSample = 0.005 // 5 ms — coalesce OS/network batching, keep ≤200 Hz measurable

// ratePeriodEMAAlpha weights each new effective-period sample in the EMA blend.
// Output cadence is assumed stable for the whole session; real changes are rare and
// may take several seconds to show in Hz/stale. A small α damps UDP batching jitter,
// single long gaps, and TCP bursts without chasing every interval.
const ratePeriodEMAAlpha = 0.04

// maxTowDeltaForEpoch is the largest type-0x01 TOW step (seconds) we treat as output
// cadence; larger gaps are treated as outages or discontinuous time, not epoch rate.
const maxTowDeltaForEpoch = 10.0

// payloadBytesToSpacedHex renders bytes as uppercase hex pairs separated by spaces
// (full GSOF sub-record on the wire: type, length, and payload, or entire type-99 wrapper for expanded types).
func payloadBytesToSpacedHex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(b)*3 - 1)
	for i, c := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmt.Sprintf("%02X", c))
	}
	return sb.String()
}

// Stats tracks GSOF record subtypes, inferred rates, and transport warnings.
type Stats struct {
	mu        sync.Mutex
	counts    map[int]int
	hz        map[int]float64
	displayHz map[int]string
	// ratePeriodEMA is an exponentially weighted mean period (s) per subtype for Hz inference.
	ratePeriodEMA map[int]float64
	lastSeen      map[int]time.Time
	// lastSeenSeq is the GSOF transmission number (Update seq arg) when lastSeen[recType] was updated (per subtype).
	lastSeenSeq    map[int]uint8
	lastSeq        uint8
	lastSeqTime    time.Time
	hasSeenType01  bool
	warnings       []string
	suppressSingle bool
	lastPayload    map[int][]byte // last GSOF record inner payload per type (for dashboard decode)
	lastRecordWire map[int][]byte // last full on-wire sub-record bytes for PayloadHex ([type][len][inner]; 100+ from 99 uses full [99][len][pl])
	// lastGPSTOWSec is the GPS time-of-week (s) from the latest type-0x01 in stream order.
	lastGPSTOWSec float64
	// prevEpochTOWSec / epochTowPeriodEMA: type-0x01 TOW deltas track output epoch cadence
	// (independent of UDP wall-clock batching). Used as a minimum Hz floor for all subtypes.
	prevEpochTOWSec      float64
	hasPrevEpochTOWSec   bool
	epochTowPeriodEMA    float64
	hasEpochTowPeriodEMA bool
	// tangentHistory holds recent type-0x07 samples paired with lastGPSTOWSec at decode time.
	tangentHistory []gsof.TangentPlanePoint
	// llhHistory holds recent type-0x02 lat/lon samples paired with lastGPSTOWSec at decode time.
	llhHistory []gsof.LLHPoint
	// llhMSLHistory holds recent type-0x46 (70) lat/lon/MSL height samples paired with lastGPSTOWSec (same point shape as type 2 for plotting).
	llhMSLHistory []gsof.LLHPoint
	// secondAnt97History holds recent type-0x61 (97) second-antenna position samples paired with lastGPSTOWSec.
	secondAnt97History []gsof.SecondAntenna97Point
	// secondAnt102History holds recent type-0x66 (102) second-antenna heading samples paired with lastGPSTOWSec.
	secondAnt102History []gsof.Heading102Point
	// dopHistory holds recent type-0x09 DOP samples paired with lastGPSTOWSec.
	dopHistory []gsof.DOPPoint
	// sigmaHistory holds recent type-0x0C sigma samples paired with lastGPSTOWSec.
	sigmaHistory []gsof.SigmaPoint
	// sigma74History holds recent type-0x4A (74) second-antenna sigma samples paired with lastGPSTOWSec (same point shape as type 12).
	sigma74History []gsof.SigmaPoint
	// attitudeHistory holds recent type-0x1B (27) attitude samples (time from each record).
	attitudeHistory []gsof.AttitudePoint
	// rxDiag28History holds recent type-0x1C (28) receiver diagnostics paired with lastGPSTOWSec.
	rxDiag28History []gsof.ReceiverDiagnosticsPoint
	// positionTimeHistory holds recent type-0x01 position-time samples (each carries its own GPS TOW).
	positionTimeHistory []gsof.PositionTimePoint
	// velocityHistory holds recent type-0x08 velocity samples paired with lastGPSTOWSec at decode time.
	velocityHistory []gsof.VelocityPoint
}

func NewStats(suppressSingle bool) *Stats {
	return &Stats{
		counts:         make(map[int]int),
		hz:             make(map[int]float64),
		displayHz:      make(map[int]string),
		ratePeriodEMA:  make(map[int]float64),
		lastSeen:       make(map[int]time.Time),
		lastSeenSeq:    make(map[int]uint8),
		lastPayload:    make(map[int][]byte),
		lastRecordWire: make(map[int][]byte),
		suppressSingle: suppressSingle,
	}
}

var allowedPeriods = []float64{
	0.02, 0.05, 0.1, 0.2, 0.5, 1.0,
	2.0, 5.0, 10.0, 15.0, 30.0, 60.0, 300.0, 600.0,
}

func snapToNearestRate(delta float64) (float64, string) {
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return 0, "calc..."
	}
	closest := allowedPeriods[0]
	minDiff := math.Abs(delta - closest)
	for _, p := range allowedPeriods {
		diff := math.Abs(delta - p)
		// Prefer the shorter period (higher Hz) when distances tie so mild jitter
		// around 0.15 s does not flip an otherwise 10 Hz stream toward 5 Hz.
		if diff < minDiff || (math.Abs(diff-minDiff) <= 1e-12 && p < closest) {
			minDiff = diff
			closest = p
		}
	}
	if closest < 1.0 {
		hz := gsof.JSONFloat(1.0 / closest)
		return hz, fmt.Sprintf("%.0f Hz", hz)
	}
	return gsof.JSONFloat(1.0 / closest), fmt.Sprintf("%.0f sec", closest)
}

// towDeltaSeconds returns the GPS week–wrapped advance cur−prev in (0, maxTowDeltaForEpoch].
func towDeltaSeconds(prev, cur float64) (float64, bool) {
	const week = 604800.0
	d := cur - prev
	if d < -week/2 {
		d += week
	} else if d > week/2 {
		d -= week
	}
	if d <= 0 || d > maxTowDeltaForEpoch {
		return 0, false
	}
	return d, true
}

// epochMinHzFromTOWLocked returns a minimum stream rate (Hz) from type-0x01 TOW cadence.
// Caller must hold s.mu.
func (s *Stats) epochMinHzFromTOWLocked() float64 {
	if !s.hasEpochTowPeriodEMA || s.epochTowPeriodEMA <= 0 {
		return 0
	}
	return gsof.JSONFloat(1.0 / s.epochTowPeriodEMA)
}

// Update processes one reassembled GSOF payload. seq is the GSOF transmission number
// (0x40 payload byte 4) when present. Sequence-gap warnings run for TCP and UDP; a single
// missed id is suppressed on TCP only when ignoreTCPGSOFTransmissionGap1 is true, and on
// UDP only when suppressSingle is true.
func (s *Stats) Update(seq uint8, buffer []byte, tcpTransport, ignoreTCPGSOFTransmissionGap1 bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if !s.lastSeqTime.IsZero() {
		expectedSeq := s.lastSeq + 1
		if seq != expectedSeq && seq != s.lastSeq {
			gap := (int(seq) + 256 - int(expectedSeq)) % 256
			suppress := false
			if gap == 1 {
				if (!tcpTransport && s.suppressSingle) || (tcpTransport && ignoreTCPGSOFTransmissionGap1) {
					suppress = true
				}
			}
			if !suppress {
				s.warnings = append(s.warnings, fmt.Sprintf("[%s] WARNING: Sequence Gap! Jumped %d to %d (Missed %d)",
					now.Format("15:04:05"), s.lastSeq, seq, gap))
			}
		}
	}
	s.lastSeq = seq
	s.lastSeqTime = now

	expanded := gsof.ExpandGSOFStream(buffer)

	isType01Present := false
	var packetSubtypes []string
	seenInThisPacket := make(map[int]bool)

	for _, e := range expanded {
		recType := e.MsgType
		pld := e.Inner

		if recType == 0x01 {
			isType01Present = true
		}
		packetSubtypes = append(packetSubtypes, fmt.Sprintf("0x%02X", recType))

		s.counts[recType]++

		if !seenInThisPacket[recType] {
			prev, hadPrev := s.lastSeen[recType]
			prevSeq := s.lastSeenSeq[recType]
			s.lastSeen[recType] = now
			s.lastSeenSeq[recType] = seq
			if hadPrev {
				wallDelta := now.Sub(prev).Seconds()
				seqAdv := (int(seq) + 256 - int(prevSeq)) % 256
				if seqAdv < 1 {
					seqAdv = 1
				}
				effectiveDelta := wallDelta / float64(seqAdv)
				if wallDelta >= minWallDeltaForRateSample && effectiveDelta >= minWallDeltaForRateSample {
					prevEMA, hadEMA := s.ratePeriodEMA[recType]
					blend := effectiveDelta
					if hadEMA && prevEMA > 0 {
						blend = ratePeriodEMAAlpha*effectiveDelta + (1.0-ratePeriodEMAAlpha)*prevEMA
					}
					blend = gsof.JSONFloat(blend)
					s.ratePeriodEMA[recType] = blend
					hzVal, hzStr := snapToNearestRate(blend)
					s.hz[recType] = gsof.JSONFloat(hzVal)
					s.displayHz[recType] = hzStr
				}
			}
			seenInThisPacket[recType] = true
		}

		s.lastPayload[recType] = append([]byte(nil), pld...)
		s.lastRecordWire[recType] = append([]byte(nil), e.Wire...)

		if recType == 1 {
			if sec, ok := gsof.ParsePositionTimeTOWSec(pld); ok {
				if s.hasPrevEpochTOWSec {
					if d, okD := towDeltaSeconds(s.prevEpochTOWSec, sec); okD {
						if s.hasEpochTowPeriodEMA {
							s.epochTowPeriodEMA = gsof.JSONFloat(ratePeriodEMAAlpha*d + (1.0-ratePeriodEMAAlpha)*s.epochTowPeriodEMA)
						} else {
							s.epochTowPeriodEMA = gsof.JSONFloat(d)
							s.hasEpochTowPeriodEMA = true
						}
					}
				}
				s.prevEpochTOWSec = sec
				s.hasPrevEpochTOWSec = true
				s.lastGPSTOWSec = sec
			}
			if pt, ok := gsof.ParsePositionTimeGraphPoint(pld); ok {
				s.appendPositionTimePoint(pt)
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
		if recType == 70 {
			if lat, lon, h, ok := gsof.ParseLLHDeg(pld); ok {
				s.appendLLHMSLPoint(gsof.LLHPoint{
					GPSTOWSec: s.lastGPSTOWSec,
					LatDeg:    lat,
					LonDeg:    lon,
					HeightM:   h,
				})
			}
		}
		if recType == 97 {
			if pt, ok := gsof.ParseSecondAntenna97Point(pld); ok {
				pt.GPSTOWSec = s.lastGPSTOWSec
				s.appendSecondAntenna97Point(pt)
			}
		}
		if recType == 102 {
			if pt, ok := gsof.ParseHeading102Point(pld); ok {
				pt.GPSTOWSec = s.lastGPSTOWSec
				s.appendHeading102Point(pt)
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
		if recType == 74 {
			if pt, ok := gsof.ParseSigmaPoint(pld); ok {
				pt.GPSTOWSec = s.lastGPSTOWSec
				s.appendSigma74Point(pt)
			}
		}
		if recType == 27 {
			if pt, ok := gsof.ParseAttitudePoint(pld); ok {
				s.appendAttitudePoint(pt)
			}
		}
		if recType == 28 {
			if pt, ok := gsof.ParseReceiverDiagnosticsPoint(pld); ok {
				pt.GPSTOWSec = s.lastGPSTOWSec
				s.appendReceiverDiagnostics28Point(pt)
			}
		}
		if recType == 8 {
			if vpt, ok := gsof.ParseVelocityGraphPoint(pld); ok {
				vpt.GPSTOWSec = s.lastGPSTOWSec
				s.appendVelocityPoint(vpt)
			}
		}
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

func (s *Stats) appendLLHMSLPoint(pt gsof.LLHPoint) {
	s.llhMSLHistory = append(s.llhMSLHistory, pt)
	if len(s.llhMSLHistory) > historySamplesMax {
		s.llhMSLHistory = s.llhMSLHistory[len(s.llhMSLHistory)-historySamplesMax:]
	}
}

func (s *Stats) appendSecondAntenna97Point(pt gsof.SecondAntenna97Point) {
	s.secondAnt97History = append(s.secondAnt97History, pt)
	if len(s.secondAnt97History) > historySamplesMax {
		s.secondAnt97History = s.secondAnt97History[len(s.secondAnt97History)-historySamplesMax:]
	}
}

func (s *Stats) appendHeading102Point(pt gsof.Heading102Point) {
	s.secondAnt102History = append(s.secondAnt102History, pt)
	if len(s.secondAnt102History) > historySamplesMax {
		s.secondAnt102History = s.secondAnt102History[len(s.secondAnt102History)-historySamplesMax:]
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

func (s *Stats) appendSigma74Point(pt gsof.SigmaPoint) {
	s.sigma74History = append(s.sigma74History, pt)
	if len(s.sigma74History) > historySamplesMax {
		s.sigma74History = s.sigma74History[len(s.sigma74History)-historySamplesMax:]
	}
}

func (s *Stats) appendAttitudePoint(pt gsof.AttitudePoint) {
	s.attitudeHistory = append(s.attitudeHistory, pt)
	if len(s.attitudeHistory) > historySamplesMax {
		s.attitudeHistory = s.attitudeHistory[len(s.attitudeHistory)-historySamplesMax:]
	}
}

func (s *Stats) appendReceiverDiagnostics28Point(pt gsof.ReceiverDiagnosticsPoint) {
	s.rxDiag28History = append(s.rxDiag28History, pt)
	if len(s.rxDiag28History) > historySamplesMax {
		s.rxDiag28History = s.rxDiag28History[len(s.rxDiag28History)-historySamplesMax:]
	}
}

func (s *Stats) appendPositionTimePoint(pt gsof.PositionTimePoint) {
	s.positionTimeHistory = append(s.positionTimeHistory, pt)
	if len(s.positionTimeHistory) > historySamplesMax {
		s.positionTimeHistory = s.positionTimeHistory[len(s.positionTimeHistory)-historySamplesMax:]
	}
}

func (s *Stats) appendVelocityPoint(pt gsof.VelocityPoint) {
	s.velocityHistory = append(s.velocityHistory, pt)
	if len(s.velocityHistory) > historySamplesMax {
		s.velocityHistory = s.velocityHistory[len(s.velocityHistory)-historySamplesMax:]
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
	// PayloadHex is the GSOF sub-record body passed to Decode, as spaced hex (UI copy / export).
	PayloadHex string `json:"payload_hex"`
	// SVBrief is populated for GSOF type 13 (GPS SV brief) for structured dashboard views.
	SVBrief []gsof.SVBriefEntry `json:"sv_brief,omitempty"`
	// AllSVBrief is populated for GSOF type 33 (all systems SV brief) for structured dashboard views.
	AllSVBrief []gsof.AllSVBriefEntry `json:"all_sv_brief,omitempty"`
	// AllSVDetailed is populated for GSOF type 34 (all systems SV detailed) for structured dashboard views.
	AllSVDetailed []gsof.AllSVDetailedEntry `json:"all_sv_detailed,omitempty"`
	// AllSV48Page is set with type 48 (multi-page all-SV detailed); SV rows are in AllSVDetailed for this page only.
	AllSV48Page *gsof.AllSV48Page `json:"all_sv_48_page,omitempty"`
	// SVDetailed is populated for GSOF type 14 (detailed satellite info).
	SVDetailed []gsof.SVDetailedEntry `json:"sv_detailed,omitempty"`
	// IonoGuard92SV is populated for GSOF type 92 (IonoGuard info) for structured SV tables in the dashboard.
	IonoGuard92SV []gsof.IonoGuardSVEntry `json:"ionoguard_92_sv,omitempty"`
	// NMA91Rows is populated for GSOF type 91 (NMA info) for structured NMA block tables in the dashboard.
	NMA91Rows []gsof.NMA91Entry `json:"nma91_rows,omitempty"`
	// Radio57Rows is populated for GSOF type 57 (radio info) for structured radio tables in the dashboard.
	Radio57Rows []gsof.Radio57Row `json:"radio_57,omitempty"`
	// TangentHistory is populated for GSOF type 7 (tangent plane delta) for dashboard graphing.
	TangentHistory []gsof.TangentPlanePoint `json:"tangent_history,omitempty"`
	// LLHHistory is populated for GSOF type 2 (latitude / longitude / ellipsoidal height) or type 70 (lat/lon / MSL height) for dashboard graphing.
	LLHHistory []gsof.LLHPoint `json:"llh_history,omitempty"`
	// SecondAntenna97History is populated for GSOF type 97 (second-antenna position) for dashboard graphing.
	SecondAntenna97History []gsof.SecondAntenna97Point `json:"second_antenna_97_history,omitempty"`
	// SecondAntenna102History is populated for GSOF type 102 (second-antenna heading) for dashboard graphing.
	SecondAntenna102History []gsof.Heading102Point `json:"second_antenna_102_history,omitempty"`
	// DOPHistory is populated for GSOF type 9 (DOP) for dashboard graphing.
	DOPHistory []gsof.DOPPoint `json:"dop_history,omitempty"`
	// SigmaHistory is populated for GSOF type 12 or 74 (position RMS / sigmas) for dashboard graphing (same JSON shape).
	SigmaHistory []gsof.SigmaPoint `json:"sigma_history,omitempty"`
	// AttitudeHistory is populated for GSOF type 27 (attitude) for dashboard graphing.
	AttitudeHistory []gsof.AttitudePoint `json:"attitude_history,omitempty"`
	// ReceiverDiagnostics28History is populated for GSOF type 28 (receiver diagnostics) for dashboard graphing.
	ReceiverDiagnostics28History []gsof.ReceiverDiagnosticsPoint `json:"receiver_diag_28_history,omitempty"`
	// PositionTimeHistory is populated for GSOF type 1 (position time) for dashboard graphing.
	PositionTimeHistory []gsof.PositionTimePoint `json:"position_time_history,omitempty"`
	// VelocityHistory is populated for GSOF type 8 (velocity) for dashboard graphing.
	VelocityHistory []gsof.VelocityPoint `json:"velocity_history,omitempty"`
}

// DashboardPayload is JSON for the web UI / SSE.
type DashboardPayload struct {
	ServerTime               string      `json:"server_time"`
	LastSeq                  int         `json:"last_seq"`
	Mode                     string      `json:"mode"`
	Port                     int         `json:"port"`
	RemoteHost               string      `json:"remote_host,omitempty"`
	Records                  []RecordRow `json:"records"`
	Warnings                 []string    `json:"warnings"`
	StreamOK                 bool        `json:"stream_ok"`
	DashboardVersion         string      `json:"dashboard_version,omitempty"`
	ShowExpectedReservedBits bool        `json:"show_expected_reserved_bits"`
}

// BuildDashboard returns a snapshot for JSON/SSE (sorted by record type).
// dashboardVersion is the gsof-dashboard binary build (empty if not applicable).
// remoteHost is the TCP peer when mode is tcp (e.g. from -host); omit with "" for listen/UDP.
func (s *Stats) BuildDashboard(mode string, port int, dashboardVersion string, remoteHost string) *DashboardPayload {
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
	epochHz := s.epochMinHzFromTOWLocked()
	for _, subType := range keys {
		rateStr := s.displayHz[subType]
		if rateStr == "" {
			rateStr = "calc..."
		}
		effHz := s.hz[subType]
		if epochHz > effHz {
			effHz = epochHz
			rateStr = fmt.Sprintf("%.0f Hz", effHz)
		} else if effHz <= 0 && epochHz > 0 {
			effHz = epochHz
			rateStr = fmt.Sprintf("%.0f Hz", effHz)
		}
		stale := false
		if effHz > 0 && now.Sub(s.lastSeen[subType]).Seconds() > (1.0/effHz)*3.0 {
			rateStr = "stale"
			stale = true
		}
		meta := gsof.Lookup(subType)
		payload := s.lastPayload[subType]
		fields := gsof.Decode(subType, payload)
		wire := s.lastRecordWire[subType]
		if len(wire) == 0 {
			// Synthesize [type][len][payload] if wire was not recorded.
			wire = append([]byte{byte(subType), byte(len(payload))}, payload...)
		}
		row := RecordRow{
			Type:       subType,
			TypeHex:    fmt.Sprintf("0x%02X", subType),
			Name:       meta.Title,
			Function:   meta.Function,
			DocURL:     meta.DocURL(),
			Fields:     fields,
			Count:      s.counts[subType],
			Rate:       rateStr,
			Stale:      stale,
			PayloadHex: payloadBytesToSpacedHex(wire),
		}
		if subType == 13 {
			_, row.SVBrief = gsof.ParseSVBriefEntries(payload)
		}
		if subType == 91 {
			_, row.NMA91Rows = gsof.ParseNMA91Entries(payload)
		}
		if subType == 57 {
			row.Radio57Rows, _ = gsof.ParseRadio57Rows(payload)
		}
		if subType == 92 {
			_, row.IonoGuard92SV = gsof.ParseIonoGuard92SVEntries(payload)
		}
		if subType == 33 {
			_, row.AllSVBrief = gsof.ParseAllSVBriefEntries(payload)
		}
		if subType == 34 {
			_, row.AllSVDetailed = gsof.ParseAllSVDetailedEntries(payload)
		}
		if subType == 48 {
			hdr, _, rows := gsof.ParseAllSVDetailedType48(payload)
			row.AllSVDetailed = rows
			if len(payload) >= 3 {
				h := hdr
				row.AllSV48Page = &h
			}
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
		if subType == 70 && len(s.llhMSLHistory) > 0 {
			row.LLHHistory = append([]gsof.LLHPoint(nil), s.llhMSLHistory...)
		}
		if subType == 97 && len(s.secondAnt97History) > 0 {
			row.SecondAntenna97History = append([]gsof.SecondAntenna97Point(nil), s.secondAnt97History...)
		}
		if subType == 102 && len(s.secondAnt102History) > 0 {
			row.SecondAntenna102History = append([]gsof.Heading102Point(nil), s.secondAnt102History...)
		}
		if subType == 9 && len(s.dopHistory) > 0 {
			row.DOPHistory = append([]gsof.DOPPoint(nil), s.dopHistory...)
		}
		if subType == 12 && len(s.sigmaHistory) > 0 {
			row.SigmaHistory = append([]gsof.SigmaPoint(nil), s.sigmaHistory...)
		}
		if subType == 74 && len(s.sigma74History) > 0 {
			row.SigmaHistory = append([]gsof.SigmaPoint(nil), s.sigma74History...)
		}
		if subType == 27 && len(s.attitudeHistory) > 0 {
			row.AttitudeHistory = append([]gsof.AttitudePoint(nil), s.attitudeHistory...)
		}
		if subType == 28 && len(s.rxDiag28History) > 0 {
			row.ReceiverDiagnostics28History = append([]gsof.ReceiverDiagnosticsPoint(nil), s.rxDiag28History...)
		}
		if subType == 1 && len(s.positionTimeHistory) > 0 {
			row.PositionTimeHistory = append([]gsof.PositionTimePoint(nil), s.positionTimeHistory...)
		}
		if subType == 8 && len(s.velocityHistory) > 0 {
			row.VelocityHistory = append([]gsof.VelocityPoint(nil), s.velocityHistory...)
		}
		rows = append(rows, row)
	}

	warn := append([]string(nil), s.warnings...)
	if len(warn) > 50 {
		warn = warn[len(warn)-50:]
	}

	return &DashboardPayload{
		ServerTime:               now.Format(time.RFC3339Nano),
		LastSeq:                  int(s.lastSeq),
		Mode:                     mode,
		Port:                     port,
		RemoteHost:               remoteHost,
		Records:                  rows,
		Warnings:                 warn,
		StreamOK:                 streamOK,
		DashboardVersion:         dashboardVersion,
		ShowExpectedReservedBits: gsof.ShowExpectedReservedBits,
	}
}

// ExportNagios copies rate and count maps for Nagios checks (caller must not mutate).
func (s *Stats) ExportNagios() (hz map[int]float64, counts map[int]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	epochHz := s.epochMinHzFromTOWLocked()
	hz = make(map[int]float64, len(s.hz))
	for k, v := range s.hz {
		out := v
		if epochHz > out {
			out = epochHz
		}
		hz[k] = gsof.JSONFloat(out)
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

// AddWarning appends a dashboard-visible warning (e.g. from the DCOL parser).
func (s *Stats) AddWarning(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, msg)
}
