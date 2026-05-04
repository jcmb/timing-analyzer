package gsof

import (
	"fmt"
	"strings"
)

// NMA91Entry is one NMA block for GSOF type 91; used by dashboards and ParseNMA91Entries.
type NMA91Entry struct {
	Source              string `json:"source"`
	Signal              string `json:"signal"`
	MaskSize            int    `json:"mask_size"`
	AuthenticatedBinary string `json:"authenticated_binary"`
	FailedBinary        string `json:"failed_binary"`
}

type nma91Parsed struct {
	week      uint16
	towMs     uint32
	nmaCount  int
	rows      []NMA91Entry
	parseNote string
}

func parseNMA91(payload []byte) (p nma91Parsed, headerOK bool) {
	const need = 2 + 4 + 1
	if len(payload) < need {
		return p, false
	}
	br := beReader{b: payload}
	p.week = br.u16()
	p.towMs = br.u32()
	p.nmaCount = int(br.u8())
	for k := 0; k < p.nmaCount; k++ {
		if !br.ok(3) {
			p.parseNote = fmt.Sprintf("truncated before NMA block %d", k)
			break
		}
		src := int(br.u8())
		sig := int(br.u8())
		n := int(br.u8())
		if n < 0 || n > len(payload) {
			p.parseNote = fmt.Sprintf("invalid mask size %d in NMA block %d", n, k)
			break
		}
		if !br.ok(2 * n) {
			p.parseNote = fmt.Sprintf("truncated in NMA block %d (need %d mask bytes)", k, 2*n)
			break
		}
		auth := make([]byte, n)
		fail := make([]byte, n)
		for i := 0; i < n; i++ {
			auth[i] = br.u8()
		}
		for i := 0; i < n; i++ {
			fail[i] = br.u8()
		}
		p.rows = append(p.rows, NMA91Entry{
			Source:              nma91SourceLabel(src),
			Signal:              nma91SignalLabel(sig),
			MaskSize:            n,
			AuthenticatedBinary: nma91MaskBytesBinary(auth),
			FailedBinary:        nma91MaskBytesBinary(fail),
		})
	}
	return p, true
}

// ParseNMA91Entries returns the declared NMA count and decoded rows (a prefix if truncated or invalid).
func ParseNMA91Entries(payload []byte) (declaredCount int, rows []NMA91Entry) {
	p, ok := parseNMA91(payload)
	if !ok {
		return 0, nil
	}
	return p.nmaCount, p.rows
}

// decodeNMA91 decodes GSOF type 0x5B (91) Navigation Message Authentication (NMA) info.
// Layout: u16 GPS week, u32 GPS TOW (ms), u8 count, then count × (u8 source, u8 signal, u8 mask bytes N,
// N bytes authenticated mask, N bytes failed mask). Per-block detail is in ParseNMA91Entries for table UIs.
func decodeNMA91(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(91).Function)}
	p, ok := parseNMA91(payload)
	if !ok {
		return shortFields(Lookup(91).Function, payload, 2+4+1)
	}
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", p.week)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(p.towMs)/1000.0)),
		kv("NMA count", fmt.Sprintf("%d", p.nmaCount)),
	)
	if p.parseNote != "" {
		out = append(out, kv("Parse", p.parseNote))
	}
	return out
}

// nma91MaskBytesBinary renders each mask byte as 8 bits (MSB first), bytes separated by a space.
func nma91MaskBytesBinary(b []byte) string {
	if len(b) == 0 {
		return "—"
	}
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = fmt.Sprintf("%08b", c)
	}
	return strings.Join(parts, " ")
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
