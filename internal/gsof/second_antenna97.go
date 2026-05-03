package gsof

import (
	"fmt"
	"math"
)

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
