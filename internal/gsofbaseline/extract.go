package gsofbaseline

import (
	"timing-analyzer/internal/gsof"
)

const maxRing = 2000

// EpochSample is one rover position epoch: GPS TOW from GSOF type 1 paired with type 2 LLH.
type EpochSample struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	LatDeg    float64 `json:"lat_deg"`
	LonDeg    float64 `json:"lon_deg"`
	HeightM   float64 `json:"height_m"`
	SVsUsed   int     `json:"svs_used"`
}

// AttitudeRangeSample is type 27 range (metres) at the record's own GPS TOW.
type AttitudeRangeSample struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	RangeM    float64 `json:"range_m"`
}

// PacketWalkResult holds the latest auxiliary records seen while walking one DCOL GSOF buffer.
type PacketWalkResult struct {
	Epochs         []EpochSample
	AttitudeRanges []AttitudeRangeSample
	Base35         *gsof.ReceivedBaseInfo
	Base41         *gsof.BasePositionQualityInfo
}

// WalkGSOFPacket walks one flattened GSOF payload like gsofstats.ExpandGSOFStream.
// Type 2 (LLH) is paired with the most recent type 1 TOW in the same packet (same semantics as Stats).
func WalkGSOFPacket(gsofBuffer []byte) PacketWalkResult {
	var out PacketWalkResult
	expanded := gsof.ExpandGSOFStream(gsofBuffer)
	var lastTOW float64
	var hasTOW bool
	var lastSV int
	for _, e := range expanded {
		rec := e.MsgType
		pld := e.Inner
		switch rec {
		case 1:
			if sec, ok := gsof.ParsePositionTimeTOWSec(pld); ok {
				lastTOW = sec
				hasTOW = true
			}
			if pt, ok := gsof.ParsePositionTimeGraphPoint(pld); ok {
				lastSV = pt.SVsUsed
			}
		case 2:
			if !hasTOW {
				continue
			}
			if lat, lon, h, ok := gsof.ParseLLHDeg(pld); ok {
				out.Epochs = append(out.Epochs, EpochSample{
					GPSTOWSec: lastTOW,
					LatDeg:    lat,
					LonDeg:    lon,
					HeightM:   h,
					SVsUsed:   lastSV,
				})
			}
		case 27:
			if ap, ok := gsof.ParseAttitudePoint(pld); ok {
				out.AttitudeRanges = append(out.AttitudeRanges, AttitudeRangeSample{
					GPSTOWSec: ap.GPSTOWSec,
					RangeM:    ap.RangeM,
				})
			}
		case 35:
			if b, ok := gsof.ParseReceivedBaseInfo(pld); ok {
				cp := b
				out.Base35 = &cp
			}
		case 41:
			if b, ok := gsof.ParseBasePositionQualityInfo(pld); ok {
				cp := b
				out.Base41 = &cp
			}
		}
	}
	return out
}
