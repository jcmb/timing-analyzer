package gsofbaseline

import "math"

const earthRadiusM = 6371008.8 // mean Earth radius (IUGG)

// HaversineM returns great-circle distance in metres between two WGS-84 positions (degrees).
func HaversineM(lat1Deg, lon1Deg, lat2Deg, lon2Deg float64) float64 {
	φ1 := lat1Deg * math.Pi / 180
	φ2 := lat2Deg * math.Pi / 180
	Δφ := (lat2Deg - lat1Deg) * math.Pi / 180
	Δλ := (lon2Deg - lon1Deg) * math.Pi / 180
	sΔφ := math.Sin(Δφ / 2)
	sΔλ := math.Sin(Δλ / 2)
	a := sΔφ*sΔφ + math.Cos(φ1)*math.Cos(φ2)*sΔλ*sΔλ
	a = math.Min(1, math.Max(0, a))
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

// InitialBearingDeg returns forward azimuth from point 1 to point 2 in degrees [0,360).
func InitialBearingDeg(lat1Deg, lon1Deg, lat2Deg, lon2Deg float64) float64 {
	φ1 := lat1Deg * math.Pi / 180
	φ2 := lat2Deg * math.Pi / 180
	Δλ := (lon2Deg - lon1Deg) * math.Pi / 180
	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)
	θ := math.Atan2(y, x) * 180 / math.Pi
	θ = math.Mod(θ+360, 360)
	return θ
}

// SlantM combines horizontal great-circle distance with height difference (metres).
func SlantM(horizM, h1M, h2M float64) float64 {
	dh := h2M - h1M
	return math.Sqrt(horizM*horizM + dh*dh)
}

// BaselineAttitudeDeg returns yaw, pitch, and roll (degrees) for the line of sight from
// the rover (point 1) to the reference (point 2) in a local ENU frame at the rover:
//   - yaw: horizontal azimuth [0, 360) — same as InitialBearingDeg
//   - pitch: elevation (+ when reference is above rover)
//   - roll: always 0 (not observable from LLH positions alone)
func BaselineAttitudeDeg(lat1Deg, lon1Deg, h1M, lat2Deg, lon2Deg, h2M float64) (yawDeg, pitchDeg, rollDeg float64) {
	horiz := HaversineM(lat1Deg, lon1Deg, lat2Deg, lon2Deg)
	yawDeg = InitialBearingDeg(lat1Deg, lon1Deg, lat2Deg, lon2Deg)
	pitchDeg = math.Atan2(h2M-h1M, horiz) * 180 / math.Pi
	return yawDeg, pitchDeg, 0
}
