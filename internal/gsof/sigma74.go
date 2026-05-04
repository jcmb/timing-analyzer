package gsof

import (
	"fmt"
	"math"
)

// decodeSigma74 decodes GSOF type 0x4A (74): position sigma for the second antenna.
// Binary layout matches type 12 (nine big-endian float32 + u16 epochs); field semantics per OEM doc.
func decodeSigma74(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(9*4 + 2) {
		return shortFields(Lookup(74).Function, payload, 38)
	}
	prms := br.f32()
	se := br.f32()
	sn := br.f32()
	cov := br.f32()
	su := br.f32()
	maj := br.f32()
	minor := br.f32()
	orient := br.f32()
	uv := br.f32()
	epochs := br.u16()

	var out []Field
	out = append(out, kv("Summary", Lookup(74).Function))
	out = append(out, kv("POSITION_RMS (m)", formatSigma74MeterScalar(prms)))
	out = append(out, kv("SIGMA_EAST (m)", formatSigma74MeterScalar(se)))
	out = append(out, kv("SIGMA_NORTH (m)", formatSigma74MeterScalar(sn)))
	sigmaH := math.Sqrt(float64(se)*float64(se) + float64(sn)*float64(sn))
	out = append(out, kv("SIGMA_H (m)", formatSigma74MeterScalar(float32(sigmaH))))
	out = append(out, kv("COVAR_EAST_NORTH (dimensionless)", formatFloat32_5(cov)))
	out = append(out, kv("SIGMA_UP (m)", formatSigma74MeterScalar(su)))
	out = append(out, kv("SEMI_MAJOR_AXIS (m)", formatSigma74MeterScalar(maj)))
	out = append(out, kv("SEMI_MINOR_AXIS (m)", formatSigma74MeterScalar(minor)))
	odeg := float64(orient)
	out = append(out, kv("ORIENTATION (decimal °)", formatSignedDecimalNBSP(odeg, 8)))
	out = append(out, kv("ORIENTATION (DMS)", formatOrientationDMS(odeg)))
	out = append(out, kv("UNIT_VARIANCE", formatFloat32_5(uv)))
	out = append(out, kv("NUMBER_EPOCHS", fmt.Sprintf("%d", epochs)))
	if epochs != 0 {
		out = append(out, kv("NUMBER_EPOCHS note", "Documentation states this field is always zero for message 74"))
	}
	return out
}

// formatSigma74MeterScalar matches formatMeters5F alignment but omits the trailing " m" suffix (type 74 UI).
func formatSigma74MeterScalar(v float32) string {
	x := float64(v)
	s := fmt.Sprintf("%.5f", x)
	if x > 0 {
		return "\u00a0" + s
	}
	return s
}
