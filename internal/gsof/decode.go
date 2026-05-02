package gsof

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// Field is one decoded row for UI / JSON.
type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Decode returns human-readable fields for the payload of one GSOF record (bytes after
// the type and length bytes). Layout follows Trimble OEM documentation where available;
// types 1, 16, 26, and 41 also align with offsets used in RTKLIB decode_gsof_* (little-endian).
func Decode(msgType int, payload []byte) []Field {
	if len(payload) == 0 {
		return []Field{{"Payload", "(empty)"}}
	}
	switch msgType {
	case 1:
		return decode01(payload)
	case 2:
		return decodeLLH(2, payload)
	case 3:
		return decodeLLH(3, payload)
	case 16:
		return decode16(payload)
	case 26:
		return decode26(payload)
	case 41:
		return decode41(payload)
	default:
		return decodeGeneric(msgType, payload)
	}
}

// decodeLLH shows the first three little-endian doubles (common for LLH / ECEF XYZ in OEM docs).
// Values are indicative only; confirm field order in the Trimble message ICD for this type.
func decodeLLH(msgType int, payload []byte) []Field {
	m := Lookup(msgType)
	out := []Field{{"Summary", m.Function}}
	if len(payload) < 24 {
		out = append(out,
			Field{"Payload length (bytes)", fmt.Sprintf("%d", len(payload))},
			Field{"Payload (hex)", hexPreview(payload, 64)},
			Field{"Parse", "Need ≥24 bytes for three IEEE-754 doubles (LE)."},
		)
		return out
	}
	a, _ := readF64LE(payload, 0)
	b, _ := readF64LE(payload, 8)
	c, _ := readF64LE(payload, 16)
	labelA, labelB, labelC := "Value A (double @0, LE)", "Value B (double @8, LE)", "Value C (double @16, LE)"
	if msgType == 2 {
		labelA, labelB, labelC = "Latitude (deg, LE @0)", "Longitude (deg, LE @8)", "Height (m, LE @16)"
	}
	if msgType == 3 {
		labelA, labelB, labelC = "X (m, LE @0)", "Y (m, LE @8)", "Z (m, LE @16)"
	}
	out = append(out,
		Field{labelA, fmt.Sprintf("%.9g", a)},
		Field{labelB, fmt.Sprintf("%.9g", b)},
		Field{labelC, fmt.Sprintf("%.9g", c)},
		Field{"Note", "Field names follow common Trimble layouts; verify ordering and units in the OEM GSOF message topic for this type."},
		Field{"Payload (hex)", hexPreview(payload, 64)},
	)
	return out
}

func decodeGeneric(msgType int, payload []byte) []Field {
	m := Lookup(msgType)
	out := []Field{
		{"Summary", m.Function},
		{"Payload length (bytes)", fmt.Sprintf("%d", len(payload))},
	}
	out = append(out, Field{"Payload (hex)", hexPreview(payload, 96)})
	out = append(out, Field{"Note", "Binary field layout is message-specific; see Trimble GSOF message documentation for this type."})
	return out
}

func hexPreview(b []byte, max int) string {
	if len(b) <= max {
		return hex.EncodeToString(b)
	}
	return hex.EncodeToString(b[:max]) + fmt.Sprintf("… (%d more bytes)", len(b)-max)
}

func readU8(b []byte, off int) (byte, bool) {
	if off+1 > len(b) {
		return 0, false
	}
	return b[off], true
}

func readI16LE(b []byte, off int) (int16, bool) {
	if off+2 > len(b) {
		return 0, false
	}
	return int16(binary.LittleEndian.Uint16(b[off:])), true
}

func readI32LE(b []byte, off int) (int32, bool) {
	if off+4 > len(b) {
		return 0, false
	}
	return int32(binary.LittleEndian.Uint32(b[off:])), true
}

func readF64LE(b []byte, off int) (float64, bool) {
	if off+8 > len(b) {
		return 0, false
	}
	u := binary.LittleEndian.Uint64(b[off:])
	return math.Float64frombits(u), true
}

// RTKLIB decode_gsof_1: I4(p+2), I2(p+6) with p[0]=type, p[1]=len → payload offsets 0 and 4.
func decode01(payload []byte) []Field {
	if len(payload) < 8 {
		return []Field{
			{"Summary", Lookup(1).Function},
			{"Payload length (bytes)", fmt.Sprintf("%d", len(payload))},
			{"Payload (hex)", hexPreview(payload, 64)},
			{"Parse", "Need ≥8 bytes for GPS TOW ms + week (LE)."},
		}
	}
	towMs, _ := readI32LE(payload, 0)
	week, _ := readI16LE(payload, 4)
	out := []Field{
		{"Summary", Lookup(1).Function},
		{"GPS time of week (ms, LE)", fmt.Sprintf("%d", towMs)},
		{"GPS week (LE)", fmt.Sprintf("%d", week)},
	}
	out = append(out, Field{"Payload (hex)", hexPreview(payload, 64)})
	return out
}

// RTKLIB decode_gsof_16: I4(p+2), I2(p+6), validity U1(p+10) bit0.
func decode16(payload []byte) []Field {
	out := []Field{{"Summary", Lookup(16).Function}}
	if len(payload) < 11 {
		out = append(out,
			Field{"Payload length (bytes)", fmt.Sprintf("%d", len(payload))},
			Field{"Payload (hex)", hexPreview(payload, 64)},
			Field{"Parse", "Need ≥11 bytes for time fields + validity byte."},
		)
		return out
	}
	towMs, _ := readI32LE(payload, 0)
	week, _ := readI16LE(payload, 4)
	v, _ := readU8(payload, 8)
	valid := (v & 1) != 0
	out = append(out,
		Field{"GPS time of week (ms, LE)", fmt.Sprintf("%d", towMs)},
		Field{"GPS week (LE)", fmt.Sprintf("%d", week)},
		Field{"Time valid (bit 0 of byte @ offset 8)", fmt.Sprintf("%v", valid)},
	)
	out = append(out, Field{"Payload (hex)", hexPreview(payload, 64)})
	return out
}

func decode26(payload []byte) []Field {
	if len(payload) < 8 {
		return []Field{
			{"Summary", Lookup(26).Function},
			{"Payload length (bytes)", fmt.Sprintf("%d", len(payload))},
			{"Payload (hex)", hexPreview(payload, 64)},
			{"Parse", "Need ≥8 bytes for UTC time fields (LE)."},
		}
	}
	towMs, _ := readI32LE(payload, 0)
	week, _ := readI16LE(payload, 4)
	out := []Field{
		{"Summary", Lookup(26).Function},
		{"UTC/GPS time of week (ms, LE)", fmt.Sprintf("%d", towMs)},
		{"Week (LE)", fmt.Sprintf("%d", week)},
		{"Payload (hex)", hexPreview(payload, 64)},
	}
	return out
}

func decode41(payload []byte) []Field {
	if len(payload) < 8 {
		return []Field{
			{"Summary", Lookup(41).Function},
			{"Payload length (bytes)", fmt.Sprintf("%d", len(payload))},
			{"Payload (hex)", hexPreview(payload, 64)},
			{"Parse", "Need ≥8 bytes for base quality fields (LE)."},
		}
	}
	towMs, _ := readI32LE(payload, 0)
	week, _ := readI16LE(payload, 4)
	out := []Field{
		{"Summary", Lookup(41).Function},
		{"Time of week (ms, LE)", fmt.Sprintf("%d", towMs)},
		{"Week (LE)", fmt.Sprintf("%d", week)},
		{"Payload (hex)", hexPreview(payload, 64)},
	}
	return out
}
