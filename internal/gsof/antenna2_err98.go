package gsof

import (
	"fmt"
)

// decodeAntenna2ErrorEstimates98 decodes GSOF type 0x62 (98): error estimates for antenna 2.
// Layout matches type 11 (VCV) plus trailing u8 source: f32 RMS, six f32 VCV terms (m²), f32 unit variance,
// u16 number of epochs, u8 source (same semantics as type 97 second-antenna source).
func decodeAntenna2ErrorEstimates98(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(98).Function)}
	const need = 8*4 + 2 + 1 // 35
	if len(payload) < need {
		return shortFields(Lookup(98).Function, payload, need)
	}
	br := beReader{b: payload}
	vcv := []string{"VCV_xx", "VCV_xy", "VCV_xz", "VCV_yy", "VCV_yz", "VCV_zz"}
	out = append(out, kv("POSITION_RMS (m)", formatMeters5F(br.f32())))
	for _, l := range vcv {
		out = append(out, kv(l+" (m²)", formatM2_5(br.f32())))
	}
	out = append(out, kv("UNIT_VARIANCE", formatFloat32_5(br.f32())))
	out = append(out, kv("NUMBER_OF_EPOCHS", fmt.Sprintf("%d", br.u16())))
	out = append(out, kv("Source", secondAntenna97SourceLabel(br.u8())))
	return out
}
