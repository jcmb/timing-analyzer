package gsof

// PositionTimePoint is one GSOF type-1 (position time) sample for dashboard graphing.
// X-axis time is GPS time-of-week in seconds from the record. Axis1 is the OEM
// initialization counter (Trimble field "Initialized number"); the graph legend
// labels it "Axis 1 (init)".
type PositionTimePoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	SVsUsed   int     `json:"svs_used"`
	Axis1     int     `json:"axis1"`
	Flags1    int     `json:"flags1"`
	Flags2    int     `json:"flags2"`
}

// ParsePositionTimeGraphPoint parses the standard 10-byte type-1 payload for plotting.
func ParsePositionTimeGraphPoint(payload []byte) (p PositionTimePoint, ok bool) {
	if len(payload) < 10 {
		return p, false
	}
	sec, ok := ParsePositionTimeTOWSec(payload)
	if !ok {
		return p, false
	}
	p.GPSTOWSec = sec
	p.SVsUsed = int(payload[6])
	p.Flags1 = int(payload[7])
	p.Flags2 = int(payload[8])
	p.Axis1 = int(payload[9])
	return p, true
}
