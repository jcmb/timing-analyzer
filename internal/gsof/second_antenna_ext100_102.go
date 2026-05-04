package gsof

import (
	"fmt"
	"math"
)

// decodeSecondAntennaLocalDatum100 decodes GSOF type 0x64 (100): second-antenna
// local-datum position (after type-99 expansion). Layout: 8-char datum ID,
// three big-endian doubles — latitude and longitude in radians, height in metres.
func decodeSecondAntennaLocalDatum100(payload []byte) []Field {
	const need = 8 + 8 + 8 + 8
	if len(payload) < need {
		return shortFields(Lookup(100).Function, payload, need)
	}
	br := beReader{b: payload}
	datum := br.str8()
	latRad := br.f64()
	lonRad := br.f64()
	h := br.f64()
	latDeg := latRad * 180 / math.Pi
	lonDeg := lonRad * 180 / math.Pi
	return []Field{
		kv("Summary", Lookup(100).Function),
		kv("Local datum ID (8 chars)", datum),
		kv("Latitude (DMS)", formatDMS(latDeg, true)),
		kv("Longitude (DMS)", formatDMS(lonDeg, false)),
		kv("Latitude (decimal °)", formatDecimalDegrees(latDeg)),
		kv("Longitude (decimal °)", formatDecimalDegrees(lonDeg)),
		kv("Height (m)", formatMeters3(h)),
	}
}

// decodeSecondAntennaLocalZone101 decodes GSOF type 0x65 (101): second-antenna
// local zone ENU-style coordinates (after type-99 expansion). Layout: 8-char
// local datum ID, 8-char local zone ID, three big-endian doubles — east and
// north in metres, height in the local datum (metres).
func decodeSecondAntennaLocalZone101(payload []byte) []Field {
	const need = 8 + 8 + 8 + 8 + 8
	if len(payload) < need {
		return shortFields(Lookup(101).Function, payload, need)
	}
	br := beReader{b: payload}
	datum := br.str8()
	zone := br.str8()
	east := br.f64()
	north := br.f64()
	height := br.f64()
	return []Field{
		kv("Summary", Lookup(101).Function),
		kv("Local datum ID (8 chars)", datum),
		kv("Local zone ID (8 chars)", zone),
		kv("Local zone east (m)", formatMeters3(east)),
		kv("Local zone north (m)", formatMeters3(north)),
		kv("Local datum height (m)", formatMeters3(height)),
	}
}

// decodeSecondAntennaHeading102 decodes GSOF type 0x66 (102): second-antenna
// heading (after type-99 expansion). Layout: u8 status flags, four big-endian
// doubles — geodetic, datum, and grid headings and magnetic variation (degrees).
func decodeSecondAntennaHeading102(payload []byte) []Field {
	const need = 1 + 8 + 8 + 8 + 8
	if len(payload) < need {
		return shortFields(Lookup(102).Function, payload, need)
	}
	br := beReader{b: payload}
	st := br.u8()
	geo := br.f64()
	datum := br.f64()
	grid := br.f64()
	mag := br.f64()
	degStr := func(d float64) string {
		return fmt.Sprintf("%.8g °", d)
	}
	return []Field{
		kv("Summary", Lookup(102).Function),
		{
			Label:  "Status flags",
			Value:  fmt.Sprintf("0x%02X · %08b", st, st),
			Detail: decodeSecondAntenna102StatusFlags(st),
		},
		kv("Heading geodetic north", degStr(geo)),
		kv("Heading datum north", degStr(datum)),
		kv("Heading grid north", degStr(grid)),
		kv("Magnetic variation", degStr(mag)),
	}
}

func decodeSecondAntenna102StatusFlags(st byte) []Field {
	labels := []struct {
		bit uint
		txt string
	}{
		{0, "Geodetic heading valid"},
		{1, "Datum heading valid"},
		{2, "Grid heading valid"},
		{3, "Magnetic variation valid"},
	}
	out := make([]Field, 0, len(labels))
	for _, row := range labels {
		out = append(out, kv(row.txt, yesNo(bitOn(st, row.bit))))
	}
	return out
}
