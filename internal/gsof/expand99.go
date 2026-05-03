package gsof

import "encoding/binary"

func decodeType99Wrapper(payload []byte) []Field {
	return []Field{
		kv("Summary", Lookup(99).Function),
		kv("Note", "Stats and dashboards expand type 99 into child records (types ≥100) before counting and decode; use those rows for extended payloads."),
		kv("Raw payload (hex)", hexPreview(payload, 96)),
	}
}

// FlattenGSOFBuffer expands type-0x63 (99) extended-record payloads into normal
// GSOF sub-records: one byte record type, one byte length, then payload bytes.
// Each extended block is u16 big-endian record type (≥100), u8 length, and
// exactly that many payload bytes (per Trimble extended-record layout inside type 99).
// Type 99 itself is removed from the stream; other record types are copied through.
//
// If a type-99 payload is truncated or contains an extended type <100, expansion
// stops for that 99 payload (remaining extended bytes are skipped). An incomplete
// outer GSOF record at the end of src is omitted from the output, matching a
// strict walk that would stop before it.
func FlattenGSOFBuffer(src []byte) []byte {
	first99 := -1
	for p := 0; p < len(src); {
		if p+2 > len(src) {
			break
		}
		t := src[p]
		n := int(src[p+1])
		end := p + 2 + n
		if end > len(src) {
			break
		}
		if t == 99 {
			first99 = p
			break
		}
		p = end
	}
	if first99 < 0 {
		return src
	}

	out := make([]byte, 0, len(src)+32)
	out = append(out, src[:first99]...)
	ptr := first99
	for ptr < len(src) {
		if ptr+2 > len(src) {
			break
		}
		recType := src[ptr]
		recLen := int(src[ptr+1])
		end := ptr + 2 + recLen
		if end > len(src) {
			break
		}
		if recType != 99 {
			out = append(out, src[ptr:end]...)
			ptr = end
			continue
		}
		pl := src[ptr+2 : end]
		off := 0
		for off+3 <= len(pl) {
			extType := int(binary.BigEndian.Uint16(pl[off : off+2]))
			extLen := int(pl[off+2])
			if extType < 100 || extType > 255 {
				break
			}
			if extLen < 0 || off+3+extLen > len(pl) {
				break
			}
			body := pl[off+3 : off+3+extLen]
			out = append(out, byte(extType), byte(extLen))
			out = append(out, body...)
			off += 3 + extLen
		}
		ptr = end
	}
	return out
}
