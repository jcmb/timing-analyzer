package gsof

// Heading102Point is one GSOF type-0x66 (102) sample for dashboard graphing (paired with type-0x01 TOW).
// Wire layout matches decodeSecondAntennaHeading102: u8 status, four big-endian f64 values in degrees.
type Heading102Point struct {
	GPSTOWSec            float64 `json:"gps_tow_s"`
	HeadingGeodeticDeg   float64 `json:"heading_geodetic_deg"`
	HeadingDatumDeg      float64 `json:"heading_datum_deg"`
	HeadingGridDeg       float64 `json:"heading_grid_deg"`
	MagneticVariationDeg float64 `json:"magnetic_variation_deg"`
}

// ParseHeading102Point parses a type-0x66 inner payload into a point (GPSTOWSec must be set by caller).
func ParseHeading102Point(payload []byte) (Heading102Point, bool) {
	const need = 1 + 8 + 8 + 8 + 8
	if len(payload) < need {
		return Heading102Point{}, false
	}
	br := beReader{b: payload}
	_ = br.u8()
	return Heading102Point{
		HeadingGeodeticDeg:   JSONFloat(br.f64()),
		HeadingDatumDeg:      JSONFloat(br.f64()),
		HeadingGridDeg:       JSONFloat(br.f64()),
		MagneticVariationDeg: JSONFloat(br.f64()),
	}, true
}
