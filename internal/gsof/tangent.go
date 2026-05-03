package gsof

// TangentPlanePoint is one GSOF type-7 sample; X-axis time is GPS time-of-week
// in seconds from the most recent type-0x01 (position / time) record in the stream.
type TangentPlanePoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	DEm       float64 `json:"de_m"`
	DNm       float64 `json:"dn_m"`
	DUm       float64 `json:"du_m"`
}

// ParsePositionTimeTOWSec returns GPS time of week in seconds from type-0x01 payload
// (first field: milliseconds since start of GPS week, big-endian u32).
func ParsePositionTimeTOWSec(payload []byte) (sec float64, ok bool) {
	br := beReader{b: payload}
	if !br.ok(4) {
		return 0, false
	}
	ms := br.u32()
	return JSONFloat(float64(ms) / 1000), true
}

// ParseTangentPlaneENU parses GSOF type-0x07 tangent-plane deltas (metres).
func ParseTangentPlaneENU(payload []byte) (de, dn, du float64, ok bool) {
	br := beReader{b: payload}
	if !br.ok(24) {
		return 0, 0, 0, false
	}
	de = br.f64()
	dn = br.f64()
	du = br.f64()
	return JSONFloat(de), JSONFloat(dn), JSONFloat(du), true
}
