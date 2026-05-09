package dcol

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// GSOFExtendedUnknown is a synthetic record type (0xF3) used when bytes inside a
// GSOF type-0x63 (99) payload cannot be parsed as extended u16 type (100–255),
// u8 length, and body, or when there are leftover bytes after valid extended blocks.
const GSOFExtendedUnknown = 243

// ExpandedRecord is one logical GSOF sub-message after type-99 expansion.
type ExpandedRecord struct {
	MsgType int
	Inner   []byte
	Wire    []byte
}

// ExpandGSOFStream walks a reassembled GSOF buffer and expands type 99 into zero
// or more ExpandedRecord values. Type 99 is never emitted as its own record.
func ExpandGSOFStream(src []byte) []ExpandedRecord {
	var out []ExpandedRecord
	ptr := 0
	for ptr+2 <= len(src) {
		t := int(src[ptr])
		n := int(src[ptr+1])
		end := ptr + 2 + n
		if end > len(src) {
			break
		}
		wire := append([]byte(nil), src[ptr:end]...)
		if t != 99 {
			out = append(out, ExpandedRecord{
				MsgType: t,
				Inner:   append([]byte(nil), src[ptr+2:end]...),
				Wire:    wire,
			})
			ptr = end
			continue
		}
		pl := src[ptr+2 : end]
		off := 0
		if len(pl) < 3 {
			if len(pl) > 0 {
				out = append(out, ExpandedRecord{
					MsgType: GSOFExtendedUnknown,
					Inner:   append([]byte(nil), pl...),
					Wire:    wire,
				})
			}
			ptr = end
			continue
		}
		for off+3 <= len(pl) {
			extType := int(binary.BigEndian.Uint16(pl[off : off+2]))
			extLen := int(pl[off+2])
			if extType < 100 || extType > 255 {
				rem := pl[off:]
				if len(rem) > 0 {
					out = append(out, ExpandedRecord{
						MsgType: GSOFExtendedUnknown,
						Inner:   append([]byte(nil), rem...),
						Wire:    wire,
					})
				}
				off = len(pl)
				break
			}
			if off+3+extLen > len(pl) {
				rem := pl[off:]
				if len(rem) > 0 {
					out = append(out, ExpandedRecord{
						MsgType: GSOFExtendedUnknown,
						Inner:   append([]byte(nil), rem...),
						Wire:    wire,
					})
				}
				off = len(pl)
				break
			}
			body := pl[off+3 : off+3+extLen]
			out = append(out, ExpandedRecord{
				MsgType: extType,
				Inner:   append([]byte(nil), body...),
				Wire:    wire,
			})
			off += 3 + extLen
		}
		if off < len(pl) {
			rem := pl[off:]
			out = append(out, ExpandedRecord{
				MsgType: GSOFExtendedUnknown,
				Inner:   append([]byte(nil), rem...),
				Wire:    wire,
			})
		}
		ptr = end
	}
	return out
}

func containsType99(src []byte) bool {
	for p := 0; p+2 <= len(src); {
		t := src[p]
		n := int(src[p+1])
		end := p + 2 + n
		if end > len(src) {
			break
		}
		if t == 99 {
			return true
		}
		p = end
	}
	return false
}

func expandedToFlatBuffer(recs []ExpandedRecord) []byte {
	var out []byte
	for _, e := range recs {
		if e.MsgType == GSOFExtendedUnknown {
			inner := e.Inner
			for len(inner) > 0 {
				n := len(inner)
				if n > 255 {
					n = 255
				}
				out = append(out, byte(GSOFExtendedUnknown), byte(n))
				out = append(out, inner[:n]...)
				inner = inner[n:]
			}
			continue
		}
		if len(e.Inner) > 255 {
			out = append(out, byte(e.MsgType), 255)
			out = append(out, e.Inner[:255]...)
			continue
		}
		out = append(out, byte(e.MsgType), byte(len(e.Inner)))
		out = append(out, e.Inner...)
	}
	return out
}

// FlattenGSOFBuffer expands type-0x63 (99) extended-record payloads into normal
// GSOF sub-records (plus synthetic 0xF3 chunks for unparsed type-99 bytes).
// If src contains no type 99, src is returned unchanged (same slice).
func FlattenGSOFBuffer(src []byte) []byte {
	if !containsType99(src) {
		return src
	}
	return expandedToFlatBuffer(ExpandGSOFStream(src))
}

// FormatHexSpaced returns each byte as two uppercase hex digits, separated by spaces (e.g. "1A 2B FF").
func FormatHexSpaced(b []byte) string {
	return hexBytesSpaced(b)
}

func hexBytesSpaced(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(b)*3 - 1)
	for i, v := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmt.Sprintf("%02X", v))
	}
	return sb.String()
}

func debugPrintGSOFSubrecords(env Env, gsofBuffer []byte) {
	if env.Logger == nil {
		return
	}
	buf := FlattenGSOFBuffer(gsofBuffer)
	ptr := 0
	for ptr < len(buf) {
		if ptr+2 > len(buf) {
			env.Logger.Printf("[VERBOSE2] GSOF sub-record truncated at offset %d (remainder %d bytes): %s\n",
				ptr, len(buf)-ptr, hexBytesSpaced(buf[ptr:]))
			return
		}
		recType := buf[ptr]
		recLen := int(buf[ptr+1])
		endIdx := ptr + 2 + recLen
		if endIdx > len(buf) {
			env.Logger.Printf("[VERBOSE2] GSOF sub-record type=0x%02X len=%d incomplete (need %d bytes, %d remain): %s\n",
				recType, recLen, endIdx-ptr, len(buf)-ptr, hexBytesSpaced(buf[ptr:]))
			return
		}
		recBytes := buf[ptr:endIdx]
		env.Logger.Printf("[VERBOSE2] GSOF sub-record type=0x%02X len=%d packet_hex=%s\n",
			recType, recLen, hexBytesSpaced(recBytes))
		ptr = endIdx
	}
}

func debugPrintFlattenedSubmessages(env Env, gsofBuffer []byte) {
	if env.Logger == nil {
		return
	}
	flat := FlattenGSOFBuffer(gsofBuffer)
	ptr := 0
	for ptr < len(flat)-1 {
		recType := flat[ptr]
		recLen := flat[ptr+1]
		endIdx := ptr + 2 + int(recLen)
		if endIdx > len(flat) {
			env.Logger.Printf("[DEBUG]   - SUB-MESSAGE OVERRUN: Type 0x%02X needs %d bytes, but only %d remain\n", recType, recLen, len(flat)-ptr-2)
			break
		}
		env.Logger.Printf("[DEBUG]   - SUB-MESSAGE: Type 0x%02X, Len %d, Data [%X]\n", recType, recLen, flat[ptr+2:endIdx])
		ptr = endIdx
	}
}
