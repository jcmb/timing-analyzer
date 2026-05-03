package gsof

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// decodeRadio57 decodes GSOF type 0x39 (57) radio info.
// Payload layout per OEM doc: u16 GPS week, u32 GPS time (ms), u8 number of radios, then per radio:
// u8 radio data length (bytes in this group including this byte; typically 9), then (length−1) bytes:
// u8 band, u8 channel, i16 signal (dBm), u8 signal bars, i16 noise (dBm), u8 noise bars, optional TBD/extension.
func decodeRadio57(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(57).Function)}
	const header = 2 + 4 + 1
	if len(payload) < header {
		return shortFields(Lookup(57).Function, payload, header)
	}
	br := beReader{b: payload}
	week := br.u16()
	towMs := br.u32()
	nRadios := int(br.u8())
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", week)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(towMs)/1000.0)),
	)
	if week == 0 && towMs == 0 {
		out = append(out, kv("GPS time availability", "Unavailable (week and ms are zero per specification)"))
	}
	out = append(out, kv("Radio count", fmt.Sprintf("%d", nRadios)))
	for i := 0; i < nRadios; i++ {
		if !br.ok(1) {
			out = append(out, kv("Parse", fmt.Sprintf("truncated before radio %d length byte", i)))
			return out
		}
		radLen := int(br.u8())
		if radLen < 1 {
			out = append(out, kv("Parse", fmt.Sprintf("invalid radio data length %d for radio %d", radLen, i)))
			return out
		}
		bodyLen := radLen - 1
		if !br.ok(bodyLen) {
			out = append(out, kv("Parse", fmt.Sprintf("truncated in radio %d group (need %d bytes after length)", i, bodyLen)))
			return out
		}
		pfx := fmt.Sprintf("Radio %d", i)
		if bodyLen < 8 {
			raw := make([]byte, bodyLen)
			for j := 0; j < bodyLen; j++ {
				raw[j] = br.u8()
			}
			out = append(out, kv(pfx+" raw (hex)", radio57Hex(raw)))
			out = append(out, kv("Parse", fmt.Sprintf("radio %d body shorter than 8 bytes (%d); raw shown as hex", i, bodyLen)))
			continue
		}
		band := br.u8()
		ch := br.u8()
		sig := br.i16()
		sigBars := int(br.u8())
		noise := br.i16()
		noiseBars := int(br.u8())
		extra := bodyLen - 8
		var ext []byte
		if extra > 0 {
			ext = make([]byte, extra)
			for j := 0; j < extra; j++ {
				ext[j] = br.u8()
			}
		}
		out = append(out,
			kv(pfx+" data length (bytes)", fmt.Sprintf("%d", radLen)),
			kv(pfx+" band", radio57BandLabel(band)),
			kv(pfx+" channel", fmt.Sprintf("%d", ch)),
			kv(pfx+" signal strength", radio57StrengthLabel(band, sig)),
			kv(pfx+" signal bars", fmt.Sprintf("%d / 5", sigBars)),
			kv(pfx+" noise strength", radio57NoiseLabel(noise)),
			kv(pfx+" noise bars", fmt.Sprintf("%d / 5", noiseBars)),
		)
		if len(ext) > 0 {
			out = append(out, kv(pfx+" extension (hex)", radio57Hex(ext)))
		}
	}
	return out
}

func radio57Hex(b []byte) string {
	if len(b) == 0 {
		return "—"
	}
	return strings.ToUpper(hex.EncodeToString(b))
}

func radio57BandLabel(b byte) string {
	switch b {
	case 0xFF:
		return "0xFF — No radio detected"
	case 0x01:
		return "0x01 — 450 MHz radio"
	case 0x02:
		return "0x02 — 900 MHz radio"
	case 0x03:
		return "0x03 — 220 MHz radio"
	case 0x04:
		return "0x04 — 2.4 GHz radio"
	case 0x05:
		return "0x05 — GPRS modem"
	default:
		return fmt.Sprintf("0x%02X — unknown", b)
	}
}

func radio57StrengthLabel(band byte, v int16) string {
	if v == 0x7FFF {
		return "not available (0x7FFF)"
	}
	if band == 0x05 && v == 0 {
		return "not available (0 dBm, GPRS modem)"
	}
	s := fmt.Sprintf("%d dBm", v)
	if v == -140 {
		s += " (Blade: possible no-packet-in-last-second sentinel)"
	}
	return s
}

func radio57NoiseLabel(v int16) string {
	if v == 0x7FFF {
		return "not available (0x7FFF)"
	}
	return fmt.Sprintf("%d dBm", v)
}
