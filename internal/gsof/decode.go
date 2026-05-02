package gsof

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// GSOF record payloads use big-endian multi-byte fields (struct prefix '>' in
// https://github.com/jcmb/DCOL/blob/master/Public/GSOF.py). Trimble OEM topic:
// https://receiverhelp.trimble.com/oem-gnss/gsof-messages-overview.html
//
// Payload is the GSOF record body only (after per-record type and length bytes).

// Field is one decoded row for UI / JSON.
// Detail is optional nested rows (e.g. bit meanings under "Flags 1").
type Field struct {
	Label  string  `json:"label"`
	Value  string  `json:"value"`
	Detail []Field `json:"detail,omitempty"`
}

func kv(label, value string) Field {
	return Field{Label: label, Value: value}
}

var gnssSystemNames = []string{
	"GPS", "SBAS", "GLONASS", "GALILEO", "QZSS", "BEIDOU", "IRNSS",
	"R7", "R8", "R9", "OMNISTAR",
}

func gnssName(idx int) string {
	if idx >= 0 && idx < len(gnssSystemNames) {
		return gnssSystemNames[idx]
	}
	return fmt.Sprintf("GNSS(%d)", idx)
}

// Decode returns human-readable fields for the payload of one GSOF record.
func Decode(msgType int, payload []byte) []Field {
	if len(payload) == 0 {
		return []Field{kv("Payload", "(empty)")}
	}
	switch msgType {
	case 1:
		return decode01(payload)
	case 2:
		return decodeLatLonHeight(payload)
	case 3:
		return decodeECEFPosition(payload)
	case 4:
		return decodeLocalDatum(payload)
	case 5:
		return decodeLocalZone(payload)
	case 6:
		return decode3xF64(6, "dX (m)", "dY (m)", "dZ (m)", payload)
	case 7:
		return decode3xF64(7, "dE (m)", "dN (m)", "dU (m)", payload)
	case 8:
		return decodeVelocity(payload)
	case 9:
		return decodePDOP(payload)
	case 10:
		return decodeClock(payload)
	case 11:
		return decodeVCV(payload)
	case 12:
		return decodeSigma(payload)
	case 13:
		return decodeSVBrief(payload)
	case 14:
		return decodeSVDetailed(payload)
	case 15:
		return decodeSerial(payload)
	case 16:
		return decodeCurrentTime(payload)
	case 26:
		return decodePositionTimeUTC(payload)
	case 27:
		return decodeAttitude(payload)
	case 28:
		return decodeMasterReceiver(payload)
	case 33:
		return decodeAllSVBrief(payload)
	case 34:
		return decodeAllSVDetailed(payload)
	case 35:
		return decodeReceivedBase(payload)
	case 37:
		return decodeBatteryMemory(payload)
	case 38:
		return decodeRTKErrorScale(payload)
	case 40:
		return decodeLBandStatus(payload)
	case 41:
		return decodeBasePositionQuality(payload)
	case 48:
		return decodeMultiPageAllSV(payload)
	default:
		return decodeGeneric(msgType, payload)
	}
}

type beReader struct {
	b []byte
	i int
}

func (r *beReader) ok(n int) bool { return r.i+n <= len(r.b) }

func (r *beReader) u8() byte {
	v := r.b[r.i]
	r.i++
	return v
}

func (r *beReader) u16() uint16 {
	v := binary.BigEndian.Uint16(r.b[r.i:])
	r.i += 2
	return v
}

func (r *beReader) u32() uint32 {
	v := binary.BigEndian.Uint32(r.b[r.i:])
	r.i += 4
	return v
}

func (r *beReader) f32() float32 {
	v := binary.BigEndian.Uint32(r.b[r.i:])
	r.i += 4
	return math.Float32frombits(v)
}

func (r *beReader) f64() float64 {
	v := binary.BigEndian.Uint64(r.b[r.i:])
	r.i += 8
	return math.Float64frombits(v)
}

func (r *beReader) str8() string {
	if !r.ok(8) {
		return ""
	}
	s := string(r.b[r.i : r.i+8])
	r.i += 8
	return strings.TrimRight(s, "\x00")
}

func hexPreview(b []byte, max int) string {
	if len(b) <= max {
		return hex.EncodeToString(b)
	}
	return hex.EncodeToString(b[:max]) + fmt.Sprintf("… (%d more bytes)", len(b)-max)
}

func decodeGeneric(msgType int, payload []byte) []Field {
	m := Lookup(msgType)
	return []Field{
		kv("Summary", m.Function),
		kv("Payload length (bytes)", fmt.Sprintf("%d", len(payload))),
		kv("Payload (hex)", hexPreview(payload, 96)),
		kv("Note", "No decoder layout wired for this type; see Trimble GSOF documentation."),
	}
}

func decode01(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(10) {
		return shortFields(Lookup(1).Function, payload, 10)
	}
	gpsTime := br.u32()
	week := br.u16()
	sv := br.u8()
	f1 := br.u8()
	f2 := br.u8()
	init := br.u8()
	towSec := float64(gpsTime) / 1000
	return []Field{
		kv("Summary", Lookup(1).Function),
		kv("GPS time of week", fmt.Sprintf("%.2f s", towSec)),
		kv("GPS week", fmt.Sprintf("%d", week)),
		kv("SVs used", fmt.Sprintf("%d", sv)),
		{
			Label:  "Flags 1",
			Value:  fmt.Sprintf("0x%02X · %08b", f1, f1),
			Detail: decodePositionFlags1(f1),
		},
		{
			Label:  "Flags 2",
			Value:  fmt.Sprintf("0x%02X · %08b", f2, f2),
			Detail: decodePositionFlags2(f2),
		},
		kv("Init counter", fmt.Sprintf("%d", init)),
	}
}

func yesNo(bit byte) string {
	if bit&1 != 0 {
		return "Yes"
	}
	return "No"
}

func bitOn(flags byte, n uint) byte { return (flags >> n) & 1 }

// ShowExpectedReservedBits, when true, includes reserved flag-bit rows that
// match the specification in type 1 / 8 / 10 flag decodes. The default (false)
// omits those rows so only unexpected violations appear. Set from CLI (e.g.
// gsof-dashboard) before calling Decode.
var ShowExpectedReservedBits bool

func appendReservedSetKV(out []Field, flags byte, n uint, label string) []Field {
	if bitOn(flags, n) == 1 && !ShowExpectedReservedBits {
		return out
	}
	return append(out, kv(label, reservedAlwaysSet(flags, n)))
}

func appendReservedClearKV(out []Field, flags byte, n uint, label string) []Field {
	if bitOn(flags, n) == 0 && !ShowExpectedReservedBits {
		return out
	}
	return append(out, kv(label, reservedAlwaysClear(flags, n)))
}

func reservedAlwaysSet(flags byte, n uint) string {
	if bitOn(flags, n) == 1 {
		return "Yes — set (expected)"
	}
	return "No — clear (unexpected)"
}

func reservedAlwaysClear(flags byte, n uint) string {
	if bitOn(flags, n) == 0 {
		return "Yes — clear (expected)"
	}
	return "No — set (unexpected)"
}

func decodePositionFlags1(flags byte) []Field {
	out := []Field{
		kv("Bit 0 — New position", yesNo(bitOn(flags, 0))),
		kv("Bit 1 — Clock fix for current position", yesNo(bitOn(flags, 1))),
		kv("Bit 2 — Horizontal coordinates from this position", yesNo(bitOn(flags, 2))),
		kv("Bit 3 — Height from this position", yesNo(bitOn(flags, 3))),
	}
	out = appendReservedSetKV(out, flags, 4, "Bit 4 — Reserved (always set)")
	out = append(out, kv("Bit 5 — Least squares position", yesNo(bitOn(flags, 5))))
	out = appendReservedClearKV(out, flags, 6, "Bit 6 — Reserved (always clear)")
	out = append(out, kv("Bit 7 — Filtered L1 pseudoranges", yesNo(bitOn(flags, 7))))
	return out
}

func decodePositionFlags2(flags byte) []Field {
	b0 := bitOn(flags, 0)
	b1 := bitOn(flags, 1)
	b2 := bitOn(flags, 2)
	diff := "Differential solution."
	if b0 == 0 {
		diff = "Autonomous or WAAS solution (not differential)."
	}
	method1 := "Code."
	if b1 == 1 {
		method1 = "Phase, including RTK (fix, float, or dithered), RTX, HP or XP OmniSTAR (VBS is not derived from phase)."
	}
	method2 := "RTK-Float, dithered RTK, or code-phase DGNSS; uncorrected position is autonomous if bit 0 = 0."
	if b2 == 1 {
		method2 = "Fixed integer phase (RTK-Fixed); uncorrected position is WAAS if bit 0 = 0."
	}
	omni := "OmniSTAR differential solution (including HP, XP, and VBS)."
	if bitOn(flags, 3) == 0 {
		omni = "OmniSTAR not active."
	}
	return []Field{
		kv("Bit 0 — Differential position", diff),
		kv("Bit 1 — Differential method (code vs phase)", method1),
		kv("Bit 2 — Differential method (float vs fixed)", method2),
		kv("Bit 3 — OmniSTAR solution", omni),
		kv("Bit 4 — Position with static constraint", yesNo(bitOn(flags, 4))),
		kv("Bit 5 — Network RTK solution", yesNo(bitOn(flags, 5))),
		kv("Bit 6 — Dithered RTK", yesNo(bitOn(flags, 6))),
		kv("Bit 7 — Beacon DGNSS", yesNo(bitOn(flags, 7))),
	}
}

func decode3xF64(msgType int, a, b, c string, payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(24) {
		return shortFields(Lookup(msgType).Function, payload, 24)
	}
	x := br.f64()
	y := br.f64()
	z := br.f64()
	return []Field{
		kv(a, formatMeters3(x)),
		kv(b, formatMeters3(y)),
		kv(c, formatMeters3(z)),
	}
}

// formatMeters3 formats a length in metres to three decimal places (default for (m) fields).
// Strictly positive values are prefixed with U+00A0 (NBSP) so they align with negatives in monospace UIs.
func formatMeters3(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	if v > 0 {
		return "\u00a0" + s
	}
	return s
}

// formatSignedDecimalNBSP formats v with prec decimal places; strictly positive values get a
// leading NBSP (same alignment trick as formatMeters3).
func formatSignedDecimalNBSP(v float64, prec int) string {
	s := fmt.Sprintf("%.*f", prec, v)
	if v > 0 {
		return "\u00a0" + s
	}
	return s
}

func formatSpeedMps(v float32) string {
	return formatSignedDecimalNBSP(float64(v), 3) + " m/s"
}

func formatSpeedKmh(v float32) string {
	return formatSignedDecimalNBSP(float64(v)*3.6, 3) + " km/h"
}

// decodeECEFPosition decodes GSOF type 3: '>d d d' — X, Y, Z in metres (big-endian).
func decodeECEFPosition(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(24) {
		return shortFields(Lookup(3).Function, payload, 24)
	}
	x := br.f64()
	y := br.f64()
	z := br.f64()
	return []Field{
		kv("X (m)", formatMeters3(x)),
		kv("Y (m)", formatMeters3(y)),
		kv("Z (m)", formatMeters3(z)),
	}
}

// decodeLatLonHeight decodes GSOF type 2: '>d d d' — latitude and longitude in radians
// (antenna phase center), height in metres (Trimble OEM / jcmb GSOF.py).
func decodeLatLonHeight(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(24) {
		return shortFields(Lookup(2).Function, payload, 24)
	}
	latRad := br.f64()
	lonRad := br.f64()
	h := br.f64()
	latDeg := latRad * 180 / math.Pi
	lonDeg := lonRad * 180 / math.Pi
	return []Field{
		kv("Latitude (DMS)", formatDMS(latDeg, true)),
		kv("Longitude (DMS)", formatDMS(lonDeg, false)),
		kv("Latitude (decimal °)", formatDecimalDegrees(latDeg)),
		kv("Longitude (decimal °)", formatDecimalDegrees(lonDeg)),
		kv("Height (m)", formatMeters3(h)),
	}
}

func formatDecimalDegrees(deg float64) string {
	return fmt.Sprintf("%.8f", deg)
}

// splitDMS breaks a non-negative degrees magnitude into ° ′ ″ (seconds with 5 dp).
func splitDMS(absDeg float64) (d, m int, sec float64) {
	const eps = 1e-11
	d = int(math.Floor(absDeg + eps))
	arcSec := (absDeg - float64(d)) * 3600
	m = int(math.Floor(arcSec/60 + eps))
	sec = arcSec - float64(m)*60
	if sec < 0 {
		sec = 0
	}
	return d, m, sec
}

// formatDMS renders signed decimal degrees as hemisphere + degrees° minutes′ seconds″.
func formatDMS(deg float64, isLat bool) string {
	var hemi string
	var abs float64
	if isLat {
		if deg >= 0 {
			hemi, abs = "N", deg
		} else {
			hemi, abs = "S", -deg
		}
	} else {
		if deg >= 0 {
			hemi, abs = "E", deg
		} else {
			hemi, abs = "W", -deg
		}
	}
	d, m, s := splitDMS(abs)
	// U+2032 prime (minutes), U+2033 double prime (seconds)
	return fmt.Sprintf("%s %d° %d′ %.5f″", hemi, d, m, s)
}

func formatDOP1(v float32) string {
	return fmt.Sprintf("%.1f", v)
}

func decodePDOP(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(16) {
		return shortFields(Lookup(9).Function, payload, 16)
	}
	return []Field{
		kv("PDOP", formatDOP1(br.f32())),
		kv("HDOP", formatDOP1(br.f32())),
		kv("TDOP", formatDOP1(br.f32())),
		kv("VDOP", formatDOP1(br.f32())),
	}
}

func decodeLocalDatum(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(8 + 8 + 8 + 8) {
		return shortFields(Lookup(4).Function, payload, 32)
	}
	datum := br.str8()
	return []Field{
		kv("Datum ID (8 chars)", datum),
		kv("Local latitude (deg)", fmt.Sprintf("%.9g", br.f64())),
		kv("Local longitude (deg)", fmt.Sprintf("%.9g", br.f64())),
		kv("Local height (m)", formatMeters3(br.f64())),
	}
}

func decodeLocalZone(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(8 + 8 + 8 + 8 + 8) {
		return shortFields(Lookup(5).Function, payload, 40)
	}
	return []Field{
		kv("Datum ID (8 chars)", br.str8()),
		kv("Zone ID (8 chars)", br.str8()),
		kv("Local north (m)", formatMeters3(br.f64())),
		kv("Local east (m)", formatMeters3(br.f64())),
		kv("Local elevation (m)", formatMeters3(br.f64())),
	}
}

func decodeVelocity(payload []byte) []Field {
	br := beReader{b: payload}
	appendSpeedRows := func(out []Field, vel, vvel float32) []Field {
		out = append(out,
			kv("Velocity", formatSpeedMps(vel)),
			kv("Vertical velocity", formatSpeedMps(vvel)),
			kv("Velocity (km/h)", formatSpeedKmh(vel)),
			kv("Vertical velocity (km/h)", formatSpeedKmh(vvel)),
		)
		return out
	}
	if len(payload) >= 0x11 {
		if !br.ok(1 + 4*4) {
			return shortFields(Lookup(8).Function, payload, 17)
		}
		fl := br.u8()
		vel := br.f32()
		heading := br.f32()
		vvel := br.f32()
		localHeading := br.f32()
		out := []Field{velocityFlagsField(fl)}
		out = appendSpeedRows(out, vel, vvel)
		out = append(out,
			kv("Heading", fmt.Sprintf("%g", heading)),
			kv("Local heading", fmt.Sprintf("%g", localHeading)),
		)
		return out
	}
	if !br.ok(1 + 4*3) {
		return shortFields(Lookup(8).Function, payload, 13)
	}
	fl := br.u8()
	vel := br.f32()
	heading := br.f32()
	vvel := br.f32()
	out := []Field{velocityFlagsField(fl)}
	out = appendSpeedRows(out, vel, vvel)
	out = append(out,
		kv("Heading", fmt.Sprintf("%g", heading)),
		kv("Local heading", "(not present; payload length < 17 bytes)"),
	)
	return out
}

func velocityFlagsField(fl byte) Field {
	return Field{
		Label:  "Velocity flags",
		Value:  fmt.Sprintf("0x%02X · %08b", fl, fl),
		Detail: decodeVelocityFlags(fl),
	}
}

// decodeVelocityFlags documents GSOF type 8 velocity flags (first payload byte).
func decodeVelocityFlags(flags byte) []Field {
	v0 := "Not valid"
	if bitOn(flags, 0) == 1 {
		v0 = "Valid"
	}
	v1 := "Computed from Doppler"
	if bitOn(flags, 1) == 1 {
		v1 = "Computed from consecutive measurements"
	}
	v2 := "Heading data not valid"
	if bitOn(flags, 2) == 1 {
		v2 = "Heading data valid"
	}
	out := []Field{
		kv("Bit 0 — Velocity data validity", v0),
		kv("Bit 1 — Velocity computation", v1),
		kv("Bit 2 — Heading data validity", v2),
	}
	for n := uint(3); n <= 7; n++ {
		out = appendReservedClearKV(out, flags, n, fmt.Sprintf("Bit %d — Reserved (set to zero)", n))
	}
	return out
}

func decodeClock(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(1 + 8 + 8) {
		return shortFields(Lookup(10).Function, payload, 17)
	}
	fl := br.u8()
	return []Field{
		clockFlagsField(fl),
		kv("Clock offset", fmt.Sprintf("%g", br.f64())),
		kv("Frequency offset", fmt.Sprintf("%g", br.f64())),
	}
}

func clockFlagsField(fl byte) Field {
	return Field{
		Label:  "Clock flags",
		Value:  fmt.Sprintf("0x%02X · %08b", fl, fl),
		Detail: decodeClockFlags(fl),
	}
}

// decodeClockFlags documents GSOF type 10 clock flags (first payload byte).
func decodeClockFlags(flags byte) []Field {
	out := []Field{
		kv("Bit 0 — Clock offset valid", yesNo(bitOn(flags, 0))),
		kv("Bit 1 — Frequency offset valid", yesNo(bitOn(flags, 1))),
		kv("Bit 2 — Anywhere fix mode", yesNo(bitOn(flags, 2))),
	}
	for n := uint(3); n <= 7; n++ {
		out = appendReservedClearKV(out, flags, n, fmt.Sprintf("Bit %d — Reserved (set to zero)", n))
	}
	return out
}

// formatFloat32_5 formats a scalar with five fractional digits (dimensionless values).
func formatFloat32_5(v float32) string {
	return fmt.Sprintf("%.5f", float64(v))
}

// formatMeters5F formats a length in metres to five decimals; positive values get a NBSP prefix for alignment.
func formatMeters5F(v float32) string {
	x := float64(v)
	s := fmt.Sprintf("%.5f m", x)
	if x > 0 {
		return "\u00a0" + s
	}
	return s
}

// formatM2_5 formats a value in square metres to five decimals; positive values get a NBSP prefix.
func formatM2_5(v float32) string {
	x := float64(v)
	s := fmt.Sprintf("%.5f m²", x)
	if x > 0 {
		return "\u00a0" + s
	}
	return s
}

func formatOrientationDMS(deg float64) string {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}
	d, m, s := splitDMS(deg)
	return fmt.Sprintf("%d° %d′ %.5f″", d, m, s)
}

func decodeVCV(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(8*4 + 2) {
		return shortFields(Lookup(11).Function, payload, 34)
	}
	vcv := []string{"VCV_xx", "VCV_xy", "VCV_xz", "VCV_yy", "VCV_yz", "VCV_zz"}
	var out []Field
	out = append(out, kv("Summary", Lookup(11).Function))
	out = append(out, kv("POSITION_RMS (m)", formatMeters5F(br.f32())))
	for _, l := range vcv {
		out = append(out, kv(l+" (m²)", formatM2_5(br.f32())))
	}
	out = append(out, kv("UNIT_VARIANCE", formatFloat32_5(br.f32())))
	out = append(out, kv("NUMBER_OF_EPOCHS", fmt.Sprintf("%d", br.u16())))
	return out
}

func decodeSigma(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(9*4 + 2) {
		return shortFields(Lookup(12).Function, payload, 38)
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
	out = append(out, kv("Summary", Lookup(12).Function))
	out = append(out, kv("POSITION_RMS (m)", formatMeters5F(prms)))
	out = append(out, kv("SIGMA_EAST (m)", formatMeters5F(se)))
	out = append(out, kv("SIGMA_NORTH (m)", formatMeters5F(sn)))
	out = append(out, kv("COVAR_EAST_NORTH (m²)", formatM2_5(cov)))
	out = append(out, kv("SIGMA_UP (m)", formatMeters5F(su)))
	out = append(out, kv("SEMI_MAJOR_AXIS (m)", formatMeters5F(maj)))
	out = append(out, kv("SEMI_MINOR_AXIS (m)", formatMeters5F(minor)))
	odeg := float64(orient)
	out = append(out, kv("ORIENTATION (decimal °)", fmt.Sprintf("%.8f", odeg)))
	out = append(out, kv("ORIENTATION (DMS)", formatOrientationDMS(odeg)))
	out = append(out, kv("UNIT_VARIANCE", formatFloat32_5(uv)))
	out = append(out, kv("NUMBER_EPOCHS", fmt.Sprintf("%d", epochs)))
	return out
}

func decodeSVBrief(payload []byte) []Field {
	if len(payload) < 1 {
		return shortFields(Lookup(13).Function, payload, 1)
	}
	n, rows := ParseSVBriefEntries(payload)
	var out []Field
	out = append(out, kv("SV count", fmt.Sprintf("%d", n)))
	if len(rows) < n {
		out = append(out, kv("Parse", "truncated SV brief list"))
	}
	for i, e := range rows {
		out = append(out, kv(fmt.Sprintf("SV %d PRN", i), fmt.Sprintf("%d", e.PRN)))
		out = append(out, kv(fmt.Sprintf("SV %d Flags 1 (binary)", i), fmt.Sprintf("%08b", e.Flags1)))
		out = append(out, kv(fmt.Sprintf("SV %d Flags 2 (binary)", i), fmt.Sprintf("%08b", e.Flags2)))
	}
	return out
}

func decodeSVDetailed(payload []byte) []Field {
	if len(payload) < 1 {
		return shortFields(Lookup(14).Function, payload, 1)
	}
	n, rows := ParseSVDetailedEntries(payload)
	var out []Field
	out = append(out, kv("SV count", fmt.Sprintf("%d", n)))
	if len(rows) < n {
		out = append(out, kv("Parse", "truncated SV detailed list"))
	}
	for i, e := range rows {
		out = append(out, kv(fmt.Sprintf("SV %d PRN", i), fmt.Sprintf("%d", e.PRN)))
		out = append(out, kv(fmt.Sprintf("SV %d Flags 1 (binary)", i), fmt.Sprintf("%08b", e.Flags1)))
		out = append(out, kv(fmt.Sprintf("SV %d Flags 2 (binary)", i), fmt.Sprintf("%08b", e.Flags2)))
		out = append(out, kv(fmt.Sprintf("SV %d Elevation (°)", i), fmt.Sprintf("%d", e.Elev)))
		out = append(out, kv(fmt.Sprintf("SV %d Azimuth (°)", i), fmt.Sprintf("%d", e.Azimuth)))
		out = append(out, kv(fmt.Sprintf("SV %d L1 SNR", i), fmt.Sprintf("%.2f", e.SNRL1)))
		out = append(out, kv(fmt.Sprintf("SV %d L2 SNR", i), fmt.Sprintf("%.2f", e.SNRL2)))
	}
	return out
}

func decodeSerial(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(4) {
		return shortFields(Lookup(15).Function, payload, 4)
	}
	return []Field{kv("Serial number", fmt.Sprintf("%d", br.u32()))}
}

func timeUTCWeekInfoValidity(flags byte) string {
	if bitOn(flags, 0) == 1 {
		return "Valid"
	}
	return "Not valid"
}

func timeUTCOffsetValidity(flags byte) string {
	if bitOn(flags, 1) == 1 {
		return "Valid"
	}
	return "Not valid"
}

func decodeTimeFlags16(flags byte) []Field {
	out := []Field{
		kv("Bit 0 — Time information (week and millisecond of week) validity", timeUTCWeekInfoValidity(flags)),
		kv("Bit 1 — UTC offset validity", timeUTCOffsetValidity(flags)),
	}
	for n := uint(2); n <= 7; n++ {
		out = appendReservedClearKV(out, flags, n, fmt.Sprintf("Bit %d — Reserved (set to zero)", n))
	}
	return out
}

func decodeCurrentTime(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(4 + 2 + 2 + 1) {
		return shortFields(Lookup(16).Function, payload, 9)
	}
	towMs := br.u32()
	week := br.u16()
	utcOff := br.u16()
	fl := br.u8()
	return []Field{
		kv("Summary", Lookup(16).Function),
		kv("UTC time of week", fmt.Sprintf("%.2f s", float64(towMs)/1000)),
		kv("UTC week", fmt.Sprintf("%d", week)),
		kv("UTC offset", fmt.Sprintf("%d", utcOff)),
		{
			Label:  "Current time flags",
			Value:  fmt.Sprintf("0x%02X · %08b", fl, fl),
			Detail: decodeTimeFlags16(fl),
		},
	}
}

func decodePositionTimeUTC(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(4 + 2 + 1 + 1 + 1) {
		return shortFields(Lookup(26).Function, payload, 8)
	}
	t := br.u32()
	w := br.u16()
	return []Field{
		kv("Summary", Lookup(26).Function),
		kv("UTC time of week", fmt.Sprintf("%.2f s", float64(t)/1000)),
		kv("UTC week", fmt.Sprintf("%d", w)),
		kv("SVs", fmt.Sprintf("%d", br.u8())),
		kv("Flags 1", fmt.Sprintf("0x%02X", br.u8())),
		kv("Flags 2", fmt.Sprintf("0x%02X", br.u8())),
	}
}

// radToDMS converts radians to degrees/minutes/seconds for attitude display (after rad column).
func radToDMS(rad float64) string {
	deg := rad * 180 / math.Pi
	sign := ""
	if deg < 0 {
		sign = "-"
		deg = -deg
	}
	d := math.Floor(deg)
	mFloat := (deg - d) * 60
	m := math.Floor(mFloat)
	s := (mFloat - m) * 60
	return fmt.Sprintf("%s%.0f° %.0f′ %.5f″", sign, d, m, s)
}

func formatRad5(rad float64) string {
	return formatSignedDecimalNBSP(rad, 5) + " rad"
}

// decodeAttitudeFlags documents GSOF type 27 attitude flags (Trimble OEM GNSS).
func decodeAttitudeFlags(flags byte) []Field {
	return []Field{
		kv("Bit 0 — Calibrated", yesNo(bitOn(flags, 0))),
		kv("Bit 1 — Pitch valid", yesNo(bitOn(flags, 1))),
		kv("Bit 2 — Yaw valid", yesNo(bitOn(flags, 2))),
		kv("Bit 3 — Roll valid", yesNo(bitOn(flags, 3))),
		kv("Bit 4 — Scalar valid", yesNo(bitOn(flags, 4))),
		kv("Bit 5 — COBRA: diagnostic valid; non-COBRA: reserved", yesNo(bitOn(flags, 5))),
		kv("Bit 6 — COBRA: slave static; non-COBRA: reserved", yesNo(bitOn(flags, 6))),
		kv("Bit 7 — COBRA: error stats valid; non-COBRA: reserved", yesNo(bitOn(flags, 7))),
	}
}

func attitudeCalcModeField(mode byte) Field {
	val, det := attitudeCalcModeValue(mode)
	return Field{Label: "Calc mode", Value: val, Detail: det}
}

// attitudeCalcModeValue follows Trimble "Attitude calculation flags" (positioning mode).
func attitudeCalcModeValue(mode byte) (value string, detail []Field) {
	switch mode {
	case 0:
		return "0 — No position", []Field{kv("Attitude calculation flag", "No position (Trimble OEM GNSS)")}
	case 1:
		return "1 — Autonomous position", []Field{kv("Attitude calculation flag", "Autonomous position")}
	case 2:
		return "2 — RTK/Float position", []Field{kv("Attitude calculation flag", "RTK/Float position")}
	case 3:
		return "3 — RTK/Fix position", []Field{kv("Attitude calculation flag", "RTK/Fix position")}
	case 4:
		return "4 — DGPS position", []Field{kv("Attitude calculation flag", "DGPS position")}
	default:
		return fmt.Sprintf("%d — Not listed", mode), []Field{kv("See", "Trimble GSOF messages — Flags — Attitude calculation flags")}
	}
}

func decodeAttitude(payload []byte) []Field {
	br := beReader{b: payload}
	need := 4 + 4 + 8*4 + 2 + 7*4
	if !br.ok(need) {
		return shortFields(Lookup(27).Function, payload, need)
	}
	gpsT := br.u32()
	fl := br.u8()
	nsv := br.u8()
	mode := br.u8()
	res := br.u8()
	pitch := br.f64()
	yaw := br.f64()
	roll := br.f64()
	rng := br.f64()
	pdop10 := br.u16()
	pv := br.f32()
	yv := br.f32()
	rv := br.f32()
	covPY := br.f32()
	covPR := br.f32()
	covYR := br.f32()
	rngVar := br.f32()
	flagsField := Field{
		Label:  "Flags",
		Value:  fmt.Sprintf("0x%02X · %08b", fl, fl),
		Detail: decodeAttitudeFlags(fl),
	}
	out := []Field{
		kv("Summary", Lookup(27).Function),
		kv("GPS time of week", fmt.Sprintf("%.2f s", float64(gpsT)/1000)),
		kv("Num SVs", fmt.Sprintf("%d", nsv)),
		attitudeCalcModeField(mode),
		kv("Reserved", fmt.Sprintf("%d", res)),
		kv("Pitch (rad)", formatRad5(pitch)),
		kv("Pitch (DMS)", radToDMS(pitch)),
		kv("Yaw (rad)", formatRad5(yaw)),
		kv("Yaw (DMS)", radToDMS(yaw)),
		kv("Roll (rad)", formatRad5(roll)),
		kv("Roll (DMS)", radToDMS(roll)),
		kv("Range (m)", formatMeters3(rng)),
		kv("PDOP", fmt.Sprintf("%.1f", float64(pdop10)/10)),
		kv("Pitch variance (rad²)", formatSignedDecimalNBSP(float64(pv), 6)),
		kv("Yaw variance (rad²)", formatSignedDecimalNBSP(float64(yv), 6)),
		kv("Roll variance (rad²)", formatSignedDecimalNBSP(float64(rv), 6)),
		kv("Pitch–yaw covariance (rad²)", formatSignedDecimalNBSP(float64(covPY), 6)),
		kv("Pitch–roll covariance (rad²)", formatSignedDecimalNBSP(float64(covPR), 6)),
		kv("Yaw–roll covariance (rad²)", formatSignedDecimalNBSP(float64(covYR), 6)),
		kv("Range variance (m²)", formatSignedDecimalNBSP(float64(rngVar), 5)),
		flagsField,
	}
	return out
}

func decodeMasterReceiver(payload []byte) []Field {
	if len(payload) < 18 {
		return shortFields(Lookup(28).Function, payload, 18)
	}
	br := beReader{b: payload}
	rf := br.u8()
	ch := br.u8()
	tr := uint32(br.u8())<<16 | uint32(br.u8())<<8 | uint32(br.u8())
	bf := br.u8()
	l100 := br.u8()
	l1k := br.u8()
	l10k := br.u8()
	c1 := br.u8()
	c2 := br.u8()
	dlat := float64(br.u8()) / 10
	dtype := br.u8()
	dsv := br.u8()
	rtkp := br.u8()
	rtks := br.u8()
	posl := float64(br.u8()) / 10
	res := br.u8()
	return []Field{
		kv("Summary", Lookup(28).Function),
		kv("DIAG_RF_FLAGS", fmt.Sprintf("%d", rf)),
		kv("DIAG_CHANNELS", fmt.Sprintf("%d", ch)),
		kv("DIAG_TRACKING", fmt.Sprintf("0x%06x", tr)),
		kv("DIAG_BASE_FLAGS", fmt.Sprintf("%d", bf)),
		kv("DIAG_LINK_100", fmt.Sprintf("%d", l100)),
		kv("DIAG_LINK_1000", fmt.Sprintf("%d", l1k)),
		kv("DIAG_LINK_10000", fmt.Sprintf("%d", l10k)),
		kv("DIAG_COMMON_L1", fmt.Sprintf("%d", c1)),
		kv("DIAG_COMMON_L2", fmt.Sprintf("%d", c2)),
		kv("DIAG_DATALINK_LATENCY (/10)", fmt.Sprintf("%g", dlat)),
		kv("DIAG_DIFF_TYPE", fmt.Sprintf("%d", dtype)),
		kv("DIAG_DIFF_SVs", fmt.Sprintf("%d", dsv)),
		kv("DIAG_RTK_POS_FAULT", fmt.Sprintf("%d", rtkp)),
		kv("DIAG_RTK_SEARCH_FAULT", fmt.Sprintf("%d", rtks)),
		kv("DIAG_POS_LATENCY (/10)", fmt.Sprintf("%g", posl)),
		kv("DIAG_RESERVED", fmt.Sprintf("%d", res)),
	}
}

func decodeAllSVBrief(payload []byte) []Field {
	if len(payload) < 1 {
		return shortFields(Lookup(33).Function, payload, 1)
	}
	br := beReader{b: payload}
	n := int(br.u8())
	var out []Field
	out = append(out, kv("SV count", fmt.Sprintf("%d", n)))
	for i := 0; i < n; i++ {
		if !br.ok(4) {
			out = append(out, kv("Parse", "truncated"))
			break
		}
		out = append(out, kv(fmt.Sprintf("SV %d", i),
			fmt.Sprintf("%d %d %d %d", br.u8(), br.u8(), br.u8(), br.u8())))
	}
	return out
}

func decodeAllSVDetailed(payload []byte) []Field {
	if len(payload) < 1 {
		return shortFields(Lookup(34).Function, payload, 1)
	}
	br := beReader{b: payload}
	n := int(br.u8())
	var out []Field
	out = append(out, kv("SV count", fmt.Sprintf("%d", n)))
	for i := 0; i < n; i++ {
		if !br.ok(10) {
			out = append(out, kv("Parse", "truncated"))
			break
		}
		sv := br.u8()
		gnss := br.u8()
		flags1 := br.u8()
		flags2 := br.u8()
		elev := int8(br.u8())
		az := br.u16()
		snrL1 := br.u8()
		snrL2 := br.u8()
		snrL5 := br.u8()
		out = append(out, kv(fmt.Sprintf("SV %d", i),
			fmt.Sprintf("%s PRN=%d flags1=0x%02X flags2=0x%02X elev=%d° az=%d L1=%.2f L2=%.2f L5=%.2f",
				gnssName(int(gnss)), sv, flags1, flags2, elev, az, float64(snrL1)/4, float64(snrL2)/4, float64(snrL5)/4)))
	}
	return out
}

func decodeReceivedBase(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(1 + 8 + 2 + 8 + 8 + 8) {
		return shortFields(Lookup(35).Function, payload, 35)
	}
	fl := br.u8()
	name := br.str8()
	id := br.u16()
	lat := br.f64()
	lon := br.f64()
	h := br.f64()
	return []Field{
		kv("Flags", fmt.Sprintf("%d", fl)),
		kv("Base name", name),
		kv("Base ID", fmt.Sprintf("%d", id)),
		kv("Base lat", fmt.Sprintf("%g", lat)),
		kv("Base lon", fmt.Sprintf("%g", lon)),
		kv("Base height", formatMeters3(h)),
	}
}

func decodeBatteryMemory(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(2 + 8) {
		return shortFields(Lookup(37).Function, payload, 10)
	}
	return []Field{
		kv("Battery capacity", fmt.Sprintf("%d", br.u16())),
		kv("Memory left", fmt.Sprintf("%g", br.f64())),
	}
}

func decodeRTKErrorScale(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(4 + 1 + 1 + 4 + 1) {
		return shortFields(Lookup(38).Function, payload, 11)
	}
	return []Field{
		kv("Error scale", fmt.Sprintf("%g", br.f32())),
		kv("Solution flags", fmt.Sprintf("%d", br.u8())),
		kv("RTK condition", fmt.Sprintf("%d", br.u8())),
		kv("Correction age", fmt.Sprintf("%g", br.f32())),
		kv("Network flags", fmt.Sprintf("%d", br.u8())),
	}
}

func decodeLBandStatus(payload []byte) []Field {
	m := Lookup(40)
	if len(payload) < 4 {
		return shortFields(m.Function, payload, 4)
	}
	return []Field{
		kv("Summary", m.Function),
		kv("Payload length (bytes)", fmt.Sprintf("%d", len(payload))),
		kv("Note", "Full L-Band layout is long in GSOF.py; use hex for remainder until fully expanded."),
		kv("Payload (hex)", hexPreview(payload, 128)),
	}
}

func decodeBasePositionQuality(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(4 + 2 + 8 + 8 + 8 + 1) {
		return shortFields(Lookup(41).Function, payload, 31)
	}
	return []Field{
		kv("GPS time (raw)", fmt.Sprintf("%d", br.u32())),
		kv("GPS week", fmt.Sprintf("%d", br.u16())),
		kv("Base latitude", fmt.Sprintf("%g", br.f64())),
		kv("Base longitude", fmt.Sprintf("%g", br.f64())),
		kv("Base height", formatMeters3(br.f64())),
		kv("Quality", fmt.Sprintf("%d", br.u8())),
	}
}

func decodeMultiPageAllSV(payload []byte) []Field {
	if len(payload) < 3 {
		return shortFields(Lookup(48).Title, payload, 3)
	}
	return []Field{
		kv("Summary", Lookup(48).Function),
		kv("Multi-page version", fmt.Sprintf("%d", payload[0])),
		kv("Multi-page info", fmt.Sprintf("%d", payload[1])),
		kv("SVs in this page", fmt.Sprintf("%d", payload[2])),
		kv("Note", "Full decode aggregates pages; see Trimble multi-page all-SV topic."),
		kv("Payload (hex)", hexPreview(payload, 96)),
	}
}

func shortFields(summary string, payload []byte, need int) []Field {
	if summary == "" {
		summary = "Insufficient data for full decode"
	}
	return []Field{
		kv("Summary", summary),
		kv("Payload length (bytes)", fmt.Sprintf("%d", len(payload))),
		kv("Parse", fmt.Sprintf("need ≥%d bytes for this layout", need)),
		kv("Payload (hex)", hexPreview(payload, 64)),
	}
}
