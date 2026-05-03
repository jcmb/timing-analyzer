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
	out = append(out, kv("POSITION_RMS (m)", formatMeters5F(prms)))
	out = append(out, kv("SIGMA_EAST (m)", formatMeters5F(se)))
	out = append(out, kv("SIGMA_NORTH (m)", formatMeters5F(sn)))
	sigmaH := math.Sqrt(float64(se)*float64(se) + float64(sn)*float64(sn))
	out = append(out, kv("SIGMA_H (m)", formatMeters5F(float32(sigmaH))))
	out = append(out, kv("COVAR_EAST_NORTH (dimensionless)", formatFloat32_5(cov)))
	out = append(out, kv("SIGMA_UP (m)", formatMeters5F(su)))
	out = append(out, kv("SEMI_MAJOR_AXIS (m)", formatMeters5F(maj)))
	out = append(out, kv("SEMI_MINOR_AXIS (m)", formatMeters5F(minor)))
	odeg := float64(orient)
	out = append(out, kv("ORIENTATION (decimal °)", fmt.Sprintf("%.8f", odeg)))
	out = append(out, kv("ORIENTATION (DMS)", formatOrientationDMS(odeg)))
	out = append(out, kv("ORIENTATION note", "Semi-major axis orientation, degrees clockwise from true north"))
	out = append(out, kv("UNIT_VARIANCE", formatFloat32_5(uv)))
	out = append(out, kv("UNIT_VARIANCE note", "Over-determined solutions tend toward 1.0; <1 suggests a priori variances were too pessimistic"))
	out = append(out, kv("NUMBER_EPOCHS", fmt.Sprintf("%d", epochs)))
	if epochs != 0 {
		out = append(out, kv("NUMBER_EPOCHS note", "Documentation states this field is always zero for message 74"))
	}
	out = append(out, kv("Applicability", "Receivers with two antennas running two RTK engine instances (e.g. BX992); not valid for Cobra or two separate receivers connected together"))
	return out
}
