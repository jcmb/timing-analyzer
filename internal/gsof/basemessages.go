package gsof

import (
	"fmt"
	"math"
)

// ReceivedBaseInfo is a parsed GSOF type 35 (received base station information) payload.
type ReceivedBaseInfo struct {
	Flags       byte    `json:"flags"`
	Name        string  `json:"name"`
	BaseID      uint16  `json:"base_id"`
	LatDeg      float64 `json:"lat_deg"`
	LonDeg      float64 `json:"lon_deg"`
	HeightM     float64 `json:"height_m"`
	InfoValid   bool    `json:"info_valid"` // bit 3 of flags
	FlagsDetail string  `json:"flags_detail,omitempty"`
}

// ParseReceivedBaseInfo parses GSOF type-35 payload (decodeReceivedBase layout).
func ParseReceivedBaseInfo(payload []byte) (ReceivedBaseInfo, bool) {
	br := beReader{b: payload}
	if !br.ok(1 + 8 + 2 + 8 + 8 + 8) {
		return ReceivedBaseInfo{}, false
	}
	fl := br.u8()
	name := br.str8()
	id := br.u16()
	latRad := br.f64()
	lonRad := br.f64()
	h := br.f64()
	latDeg := JSONFloat(latRad * 180 / math.Pi)
	lonDeg := JSONFloat(lonRad * 180 / math.Pi)
	return ReceivedBaseInfo{
		Flags:       fl,
		Name:        name,
		BaseID:      id,
		LatDeg:      latDeg,
		LonDeg:      lonDeg,
		HeightM:     JSONFloat(h),
		InfoValid:   (fl & 0x08) != 0,
		FlagsDetail: fmtBaseFlags35(fl),
	}, true
}

func fmtBaseFlags35(flags byte) string {
	ver := int(flags & 0x07)
	valid := (flags & 0x08) != 0
	return fmt.Sprintf("version=%d; base_info_valid=%v", ver, valid)
}

// BasePositionQualityInfo is a parsed GSOF type 41 (base position and quality) payload.
type BasePositionQualityInfo struct {
	GPSWeek   int     `json:"gps_week"`
	GPSTOWSec float64 `json:"gps_tow_s"`
	LatDeg    float64 `json:"lat_deg"`
	LonDeg    float64 `json:"lon_deg"`
	HeightM   float64 `json:"height_m"`
	Quality   byte    `json:"quality"`
	// QualityLabel matches decode.go formatBasePositionQuality semantics.
	QualityLabel string `json:"quality_label"`
}

// ParseBasePositionQualityInfo parses GSOF type-41 payload.
func ParseBasePositionQualityInfo(payload []byte) (BasePositionQualityInfo, bool) {
	br := beReader{b: payload}
	if !br.ok(4 + 2 + 8 + 8 + 8 + 1) {
		return BasePositionQualityInfo{}, false
	}
	gpsMS := br.u32()
	week := br.u16()
	latRad := br.f64()
	lonRad := br.f64()
	h := br.f64()
	qual := br.u8()
	towSec := JSONFloat(float64(gpsMS) / 1000.0)
	latDeg := JSONFloat(latRad * 180 / math.Pi)
	lonDeg := JSONFloat(lonRad * 180 / math.Pi)
	return BasePositionQualityInfo{
		GPSWeek:      int(week),
		GPSTOWSec:    towSec,
		LatDeg:       latDeg,
		LonDeg:       lonDeg,
		HeightM:      JSONFloat(h),
		Quality:      qual,
		QualityLabel: labelBasePositionQuality(qual),
	}, true
}

func labelBasePositionQuality(code byte) string {
	switch code {
	case 0:
		return "0 — Fix not available or invalid"
	case 1:
		return "1 — Autonomous GPS fix"
	case 2:
		return "2 — Differential SBAS or OmniSTAR VBS"
	case 4:
		return "4 — RTK Fixed, xFill"
	case 5:
		return "5 — OmniSTAR XP, OmniSTAR HP, CenterPoint RTX, Float RTK, or Location RTK"
	default:
		return fmt.Sprintf("code=%d", code)
	}
}
