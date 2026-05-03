package gsof

import "math"

// AttitudePoint is one GSOF type-0x1B (27) sample for dashboard plotting.
// GPSTOWSec is from the record’s own GPS time-of-week field (milliseconds → seconds).
type AttitudePoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	PitchDeg  float64 `json:"pitch_deg"`
	YawDeg    float64 `json:"yaw_deg"`
	RollDeg   float64 `json:"roll_deg"`
	RangeM    float64 `json:"range_m"`
	// Variances are as on the wire (rad²) per Trimble OEM.
	PitchVarRad2 float64 `json:"pitch_var_rad2"`
	YawVarRad2   float64 `json:"yaw_var_rad2"`
	RollVarRad2  float64 `json:"roll_var_rad2"`
	// RangeVarM2 is range variance on the wire (typically m²; same field as decode “Range variance”).
	RangeVarM2 float64 `json:"range_var_m2"`
}

// ParseAttitudePoint parses GSOF type-27 payload (same layout as decodeAttitude).
func ParseAttitudePoint(payload []byte) (AttitudePoint, bool) {
	br := beReader{b: payload}
	need := 4 + 4 + 8*4 + 2 + 7*4
	if !br.ok(need) {
		return AttitudePoint{}, false
	}
	ms := br.u32()
	tow := float64(ms) / 1000.0
	_ = br.u8() // flags
	_ = br.u8() // nsv
	_ = br.u8() // mode
	_ = br.u8() // reserved
	pitch := br.f64()
	yaw := br.f64()
	roll := br.f64()
	rng := br.f64()
	_ = br.u16() // PDOP × 0.1
	pv := float64(br.f32())
	yv := float64(br.f32())
	rv := float64(br.f32())
	_ = br.f32() // cov PY
	_ = br.f32() // cov PR
	_ = br.f32() // cov YR
	rngVar := float64(br.f32())
	rad := 180.0 / math.Pi
	return AttitudePoint{
		GPSTOWSec:    JSONFloat(tow),
		PitchDeg:     JSONFloat(pitch * rad),
		YawDeg:       JSONFloat(yaw * rad),
		RollDeg:      JSONFloat(roll * rad),
		RangeM:       JSONFloat(rng),
		PitchVarRad2: JSONFloat(pv),
		YawVarRad2:   JSONFloat(yv),
		RollVarRad2:  JSONFloat(rv),
		RangeVarM2:   JSONFloat(rngVar),
	}, true
}
