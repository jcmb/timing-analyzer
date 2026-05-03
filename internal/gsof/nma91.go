package gsof

import (
	"fmt"
	"strings"
)

// decodeNMA91 decodes GSOF type 0x5B (91) Navigation Message Authentication (NMA) info.
// Layout: u16 GPS week, u32 GPS TOW (ms), u8 count, then count × (u8 source, u8 signal, u8 mask bytes N,
// N bytes authenticated mask, N bytes failed mask).
func decodeNMA91(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(91).Function)}
	const need = 2 + 4 + 1
	if len(payload) < need {
		return shortFields(Lookup(91).Function, payload, need)
	}
	br := beReader{b: payload}
	week := br.u16()
	towMs := br.u32()
	nmaCount := int(br.u8())
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", week)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(towMs)/1000.0)),
		kv("NMA count", fmt.Sprintf("%d", nmaCount)),
	)
	for k := 0; k < nmaCount; k++ {
		if !br.ok(3) {
			out = append(out, kv("Parse", fmt.Sprintf("truncated before NMA block %d", k)))
			return out
		}
		src := int(br.u8())
		sig := int(br.u8())
		n := int(br.u8())
		if n < 0 || n > len(payload) {
			out = append(out, kv("Parse", fmt.Sprintf("invalid mask size %d in NMA block %d", n, k)))
			return out
		}
		if !br.ok(2 * n) {
			out = append(out, kv("Parse", fmt.Sprintf("truncated in NMA block %d (need %d mask bytes)", k, 2*n)))
			return out
		}
		auth := make([]byte, n)
		fail := make([]byte, n)
		for i := 0; i < n; i++ {
			auth[i] = br.u8()
		}
		for i := 0; i < n; i++ {
			fail[i] = br.u8()
		}
		pfx := fmt.Sprintf("NMA %d", k)
		out = append(out,
			kv(pfx+" source", nma91SourceLabel(src)),
			kv(pfx+" signal", nma91SignalLabel(sig)),
			kv(pfx+" mask size (bytes)", fmt.Sprintf("%d", n)),
			kv(pfx+" authenticated mask (hex)", spacedHexUpper(auth)),
			kv(pfx+" failed mask (hex)", spacedHexUpper(fail)),
		)
	}
	return out
}

func spacedHexUpper(b []byte) string {
	if len(b) == 0 {
		return "—"
	}
	var sb strings.Builder
	sb.Grow(len(b)*3 - 1)
	for i, c := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmt.Sprintf("%02X", c))
	}
	return sb.String()
}

func nma91SourceLabel(v int) string {
	switch v {
	case 0:
		return "0 — OSNMA"
	case 1:
		return "1 — RTX-NMA"
	case 2:
		return "2 — QZNMA"
	default:
		return fmt.Sprintf("%d — unknown", v)
	}
}

func nma91SignalLabel(v int) string {
	names := []string{
		0:  "GPS LNAV (L1 C/A)",
		1:  "GPS CNAV (L2)",
		2:  "GPS CNAV (L5)",
		3:  "GPS CNAV2 (L1C)",
		4:  "Galileo I/NAV (E1)",
		5:  "Galileo F/NAV (E1)",
		6:  "BeiDou LNAV (B1I)",
		7:  "QZSS LNAV (L1 C/A)",
		8:  "QZSS LNAV (L1 C/B)",
		9:  "QZSS CNAV (L5)",
		10: "QZSS CNAV2 (L1C)",
	}
	if v >= 0 && v < len(names) {
		return fmt.Sprintf("%d — %s", v, names[v])
	}
	return fmt.Sprintf("%d — unknown", v)
}
