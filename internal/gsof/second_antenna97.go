package gsof

import (
	"fmt"
	"math"
)

// SecondAntenna97Point is one GSOF type-97 sample for dashboard graphing (paired with type-0x01 TOW).
type SecondAntenna97Point struct {
	GPSTOWSec          float64 `json:"gps_tow_s"`
	LatDeg             float64 `json:"lat_deg"`
	LonDeg             float64 `json:"lon_deg"`
	HeightM            float64 `json:"height_m"`
	SigmaEastM         float64 `json:"sigma_east_m"`
	SigmaNorthM        float64 `json:"sigma_north_m"`
	SigmaUpM           float64 `json:"sigma_up_m"`
	SigmaHorizontalM   float64 `json:"sigma_horizontal_m"`
}

// ParseSecondAntenna97Point parses a type-0x61 payload into a point (GPSTOWSec must be set by caller).
func ParseSecondAntenna97Point(payload []byte) (SecondAntenna97Point, bool) {
	const need = 44
	if len(payload) < need {
		return SecondAntenna97Point{}, false
	}
	br := beReader{b: payload}
	_ = br.u16()
	_ = br.u32()
	_ = br.u8()
	_ = br.u8()
	latRad := br.f64()
	lonRad := br.f64()
	h := br.f64()
	sE := float64(br.f32())
	sN := float64(br.f32())
	sU := float64(br.f32())
	sh := math.Sqrt(sE*sE + sN*sN)
	return SecondAntenna97Point{
		LatDeg:           latRad * 180 / math.Pi,
		LonDeg:           lonRad * 180 / math.Pi,
		HeightM:          h,
		SigmaEastM:       sE,
		SigmaNorthM:      sN,
		SigmaUpM:         sU,
		SigmaHorizontalM: sh,
	}, true
}

// decodeSecondAntennaPosition97 decodes GSOF type 0x61 (97): second-antenna position (WGS-84).
// Layout per OEM doc: u16 GPS week, u32 GPS time (ms), u8 position type, u8 source,
// f64 latitude (rad), f64 longitude (rad), f64 height (m), f32 sigma east/north/up (m).
func decodeSecondAntennaPosition97(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(97).Function)}
	const need = 2 + 4 + 1 + 1 + 8 + 8 + 8 + 4 + 4 + 4
	if len(payload) < need {
		return shortFields(Lookup(97).Function, payload, need)
	}
	br := beReader{b: payload}
	week := br.u16()
	towMs := br.u32()
	posType := br.u8()
	src := br.u8()
	latRad := br.f64()
	lonRad := br.f64()
	h := br.f64()
	sigE := br.f32()
	sigN := br.f32()
	sigU := br.f32()
	sigH := float32(math.Sqrt(float64(sigE)*float64(sigE) + float64(sigN)*float64(sigN)))
	latDeg := latRad * 180 / math.Pi
	lonDeg := lonRad * 180 / math.Pi
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", week)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(towMs)/1000.0)),
		kv("Position type", positionType38FixType(int(posType))),
		kv("Source", secondAntenna97SourceLabel(src)),
		kv("Latitude (DMS)", formatDMS(latDeg, true)),
		kv("Longitude (DMS)", formatDMS(lonDeg, false)),
		kv("Latitude (decimal °)", formatDecimalDegrees(latDeg)),
		kv("Longitude (decimal °)", formatDecimalDegrees(lonDeg)),
		kv("Height (m)", formatMeters3(h)),
		kv("Sigma east (m)", formatMeters5F(sigE)),
		kv("Sigma north (m)", formatMeters5F(sigN)),
		kv("Sigma up (m)", formatMeters5F(sigU)),
		kv("Sigma horizontal (m)", formatMeters5F(sigH)),
	)
	return out
}

func secondAntenna97SourceLabel(b byte) string {
	switch b {
	case 1:
		return "1 — Moving-base heading vector from the primary antenna"
	case 2:
		return "2 — Correction source used by the primary antenna (not yet supported)"
	default:
		return fmt.Sprintf("%d — unknown", b)
	}
}
