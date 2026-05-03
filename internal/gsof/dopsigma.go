package gsof

import "math"

// DOPPoint is one GSOF type-9 sample (PDOP, HDOP, TDOP, VDOP) vs GPS TOW from the latest type-0x01.
type DOPPoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	PDOP      float64 `json:"pdop"`
	HDOP      float64 `json:"hdop"`
	TDOP      float64 `json:"tdop"`
	VDOP      float64 `json:"vdop"`
}

// ParseDOPPoint parses GSOF type-9 payload: four big-endian float32 (PDOP, HDOP, TDOP, VDOP).
func ParseDOPPoint(payload []byte) (DOPPoint, bool) {
	br := beReader{b: payload}
	if !br.ok(16) {
		return DOPPoint{}, false
	}
	return DOPPoint{
		PDOP: JSONFloat(float64(br.f32())),
		HDOP: JSONFloat(float64(br.f32())),
		TDOP: JSONFloat(float64(br.f32())),
		VDOP: JSONFloat(float64(br.f32())),
	}, true
}

// SigmaPoint is one GSOF type-12 sample; SigmaH is horizontal RMS √(σ_E² + σ_N²).
type SigmaPoint struct {
	GPSTOWSec   float64 `json:"gps_tow_s"`
	PositionRMS float64 `json:"position_rms_m"`
	SigmaEast   float64 `json:"sigma_east_m"`
	SigmaNorth  float64 `json:"sigma_north_m"`
	SigmaUp     float64 `json:"sigma_up_m"`
	SigmaH      float64 `json:"sigma_h_m"`
}

// ParseSigmaPoint parses GSOF type-12 or type-74 payload (same binary layout as decodeSigma / second-antenna sigma).
func ParseSigmaPoint(payload []byte) (SigmaPoint, bool) {
	br := beReader{b: payload}
	if !br.ok(9*4+2) {
		return SigmaPoint{}, false
	}
	prms := br.f32()
	se := br.f32()
	sn := br.f32()
	_ = br.f32() // COVAR_EAST_NORTH
	su := br.f32()
	_ = br.f32() // SEMI_MAJOR_AXIS
	_ = br.f32() // SEMI_MINOR_AXIS
	_ = br.f32() // ORIENTATION
	_ = br.f32() // UNIT_VARIANCE
	_ = br.u16() // NUMBER_EPOCHS
	se64 := float64(se)
	sn64 := float64(sn)
	return SigmaPoint{
		PositionRMS: JSONFloat(float64(prms)),
		SigmaEast:   JSONFloat(se64),
		SigmaNorth:  JSONFloat(sn64),
		SigmaUp:     JSONFloat(float64(su)),
		SigmaH:      JSONFloat(math.Sqrt(se64*se64 + sn64*sn64)),
	}, true
}
