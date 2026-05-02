package gsof

import "math"

// LLHPoint is one GSOF type-2 sample: latitude and longitude in decimal degrees, height in metres,
// with GPS time-of-week (seconds) from the latest type-0x01 at sample time.
type LLHPoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	LatDeg    float64 `json:"lat_deg"`
	LonDeg    float64 `json:"lon_deg"`
	HeightM   float64 `json:"height_m"`
}

// ParseLLHDeg parses a type-0x02 payload (three big-endian float64: lat, lon, height in radians, radians, metres)
// and returns latitude and longitude in decimal degrees and height in metres.
func ParseLLHDeg(payload []byte) (latDeg, lonDeg, heightM float64, ok bool) {
	br := beReader{b: payload}
	if !br.ok(24) {
		return 0, 0, 0, false
	}
	latRad := br.f64()
	lonRad := br.f64()
	heightM = br.f64()
	return latRad * 180 / math.Pi, lonRad * 180 / math.Pi, heightM, true
}

// ParseLatLonDeg parses the first two float64 fields of a type-0x02 payload (radians)
// and returns latitude and longitude in decimal degrees (height not read; needs at least 16 bytes).
func ParseLatLonDeg(payload []byte) (latDeg, lonDeg float64, ok bool) {
	br := beReader{b: payload}
	if !br.ok(16) {
		return 0, 0, false
	}
	latRad := br.f64()
	lonRad := br.f64()
	return latRad * 180 / math.Pi, lonRad * 180 / math.Pi, true
}
