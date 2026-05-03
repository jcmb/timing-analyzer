package gsof

// VelocityPoint is one GSOF type-0x08 sample for dashboard graphing.
// GPSTOWSec is taken from the latest type-0x01 in the stream (same convention as type 7).
type VelocityPoint struct {
	GPSTOWSec           float64  `json:"gps_tow_s"`
	VelocityMPS         float64  `json:"velocity_mps"`
	VerticalVelocityMPS float64  `json:"vertical_velocity_mps"`
	HeadingDeg          float64  `json:"heading_deg"`
	LocalHeadingDeg     *float64 `json:"local_heading_deg,omitempty"`
}

// ParseVelocityGraphPoint parses type-0x08 payload: flags u8, vel f32, heading f32,
// vvel f32, and optionally local heading f32 when len(payload) >= 17.
func ParseVelocityGraphPoint(payload []byte) (p VelocityPoint, ok bool) {
	br := beReader{b: payload}
	if len(payload) < 13 || !br.ok(13) {
		return p, false
	}
	_ = br.u8()
	p.VelocityMPS = JSONFloat(float64(br.f32()))
	p.HeadingDeg = JSONFloat(float64(br.f32()))
	p.VerticalVelocityMPS = JSONFloat(float64(br.f32()))
	if len(payload) >= 17 && br.ok(4) {
		lh := JSONFloat(float64(br.f32()))
		p.LocalHeadingDeg = &lh
	}
	return p, true
}
