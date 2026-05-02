package gsof

import (
	"fmt"
	"time"
)

// decodePositionType38 decodes GSOF type 38 (Position Type Information).
// OEM layout (≥26 bytes, optional 27th when frame flag bit 7 set):
// https://receiverhelp.trimble.com/oem-gnss/gsof-messages-position-type.html
// Legacy 11-byte payloads (float + flags + correction age + network) are still decoded for older captures.
func decodePositionType38(payload []byte) []Field {
	if len(payload) >= 26 {
		frame0 := payload[12]
		need := 26
		if bitOn(frame0, 7) != 0 {
			need = 27
		}
		if len(payload) < need {
			return shortFields(Lookup(38).Function, payload, need)
		}
		return decodePositionType38OEM(payload)
	}
	if len(payload) == 11 {
		return decodePositionType38Legacy(payload)
	}
	return shortFields(Lookup(38).Function, payload, 26)
}

func decodePositionType38Legacy(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(11) {
		return shortFields(Lookup(38).Function, payload, 11)
	}
	esc := br.f32()
	sol := br.u8()
	rtk := br.u8()
	corr := br.f32()
	net := br.u8()
	return []Field{
		kv("Summary", Lookup(38).Function+" (11-byte legacy layout)"),
		kv("Error scale", fmt.Sprintf("%g", esc)),
		{
			Label:  "Solution flags",
			Value:  fmt.Sprintf("0x%02X · %08b", sol, sol),
			Detail: decodePositionType38SolutionFlagsDetail(sol),
		},
		decodePositionType38RTKField(rtk),
		kv("Correction age (s)", fmt.Sprintf("%.2f", corr)),
		{
			Label:  "Network flags",
			Value:  fmt.Sprintf("0x%02X · %08b", net, net),
			Detail: decodePositionType38NetworkFlagsDetail(net),
		},
		kv("Note", "Full OEM record is ≥26 bytes (reserved, Network flags 2, frame, ITRF epoch, …)."),
	}
}

func decodePositionType38OEM(payload []byte) []Field {
	br := beReader{b: payload}
	if !br.ok(26) {
		return shortFields(Lookup(38).Function, payload, 26)
	}
	out := []Field{kv("Summary", Lookup(38).Function)}

	res := br.b[br.i : br.i+4]
	br.i += 4
	if ShowExpectedReservedBits {
		out = append(out, kv("Reserved (bytes 0–3)", hexPreview(res, 8)))
	}

	sol := br.u8()
	out = append(out, Field{
		Label:  "Solution flags",
		Value:  fmt.Sprintf("0x%02X · %08b", sol, sol),
		Detail: decodePositionType38SolutionFlagsDetail(sol),
	})

	rtk := br.u8()
	out = append(out, decodePositionType38RTKField(rtk))

	corr := br.f32()
	out = append(out, kv("Correction age (s)", fmt.Sprintf("%.2f", corr)))

	net := br.u8()
	out = append(out, Field{
		Label:  "Network flags",
		Value:  fmt.Sprintf("0x%02X · %08b", net, net),
		Detail: decodePositionType38NetworkFlagsDetail(net),
	})

	net2 := br.u8()
	out = append(out, Field{
		Label:  "Network flags 2",
		Value:  fmt.Sprintf("0x%02X · %08b", net2, net2),
		Detail: decodePositionType38NetworkFlags2Detail(net2),
	})

	frame0 := br.u8()
	var frame1 byte
	hasFrame1 := bitOn(frame0, 7) != 0
	if hasFrame1 {
		frame1 = br.u8()
	}
	frameVal := fmt.Sprintf("0x%02X · %08b", frame0, frame0)
	if hasFrame1 {
		frameVal += fmt.Sprintf(" + 0x%02X · %08b", frame1, frame1)
	}
	out = append(out, Field{
		Label:  "Frame flag",
		Value:  frameVal,
		Detail: decodePositionType38FrameFlagDetail(frame0, frame1, hasFrame1),
	})

	itrf := br.i16()
	b01 := frame0 & 0x03
	itrfLine := formatITRFEpoch38(itrf, b01)
	out = append(out, kv("ITRF epoch (raw)", fmt.Sprintf("%d (×0.01 year from 2005-01-01 UTC)", itrf)))
	out = append(out, kv("ITRF epoch (interpreted)", itrfLine))

	plate := br.u8()
	out = append(out, kv("Tectonic plate", positionType38TectonicPlate(int(plate))))

	rtxMin := br.i32()
	out = append(out, kv("RTX STD SUB minutes left", formatRTXSubscriptionMinutes(rtxMin)))

	pole := br.u8()
	out = append(out, kv("Pole wobble status", yesNo(bitOn(pole, 0))))

	poleDist := br.f32()
	out = append(out, kv("Pole wobble distance (m)", fmt.Sprintf("%.3f", poleDist)))

	pos := br.u8()
	out = append(out, kv("Position fix type", positionType38FixType(int(pos))))

	if br.i < len(payload) {
		out = append(out, kv("Trailing payload (hex)", hexPreview(payload[br.i:], 64)))
	}
	return out
}

func decodePositionType38SolutionFlagsDetail(sol byte) []Field {
	out := []Field{
		kv("Bit 0 — Wide Area / Network / VRS", yesNo(bitOn(sol, 0))),
		kv("Bit 1 — RTK fixed (clear = RTK float)", yesNo(bitOn(sol, 1))),
	}
	initBits := int(bitOn(sol, 3))<<1 | int(bitOn(sol, 2))
	var initSummary string
	switch initBits {
	case 0:
		initSummary = "Not checking"
	case 1:
		initSummary = "Checking initialization"
	case 2:
		initSummary = "Initialization passed"
	case 3:
		initSummary = "Initialization failed"
	}
	out = append(out, kv("Bits 3..2 — Initialization integrity", initSummary))
	for n := uint(4); n <= 7; n++ {
		out = appendReservedClearKV(out, sol, n, fmt.Sprintf("Bit %d — Reserved (clear)", n))
	}
	return out
}

func decodePositionType38NetworkFlagsDetail(net byte) []Field {
	v21 := (net >> 1) & 0x03
	var net21 string
	switch v21 {
	case 0:
		net21 = "RTCM v3 network messages not available or unknown (RTCM3Net not operational)"
	case 1:
		net21 = "Collecting RTCM v3 network messages; no complete cycle yet"
	case 2:
		net21 = "Full cycle collected; network message data insufficient for RTK network solutions"
	case 3:
		net21 = "RTCM v3 network collection complete; VRS epochs from V3 messages (network OK)"
	}
	out := []Field{
		kv("Bit 0 — New physical base available (clears after GETBASE 34h)", yesNo(bitOn(net, 0))),
		kv("Bits 2..1 — RTCM v3 network state", net21),
		kv("Bit 3 — GeoFence enabled and outside fence", yesNo(bitOn(net, 3))),
		kv("Bit 4 — RTK range limit enabled and exceeded", yesNo(bitOn(net, 4))),
		kv("Bit 5 — xFill operation", yesNo(bitOn(net, 5))),
		kv("Bit 6 — RTX position", yesNo(bitOn(net, 6))),
		kv("Bit 7 — RTX/xFill link down", yesNo(bitOn(net, 7))),
	}
	return out
}

func decodePositionType38NetworkFlags2Detail(n2 byte) []Field {
	return []Field{
		kv("Bit 0 — xFill ready to propagate RTK (or running)", yesNo(bitOn(n2, 0))),
		kv("Bit 1 — RTX Fast solution", yesNo(bitOn(n2, 1))),
		kv("Bit 2 — xFill–RTX offset known well enough to propagate RTK", yesNo(bitOn(n2, 2))),
		kv("Bit 3 — CMRxe being received", yesNo(bitOn(n2, 3))),
		kv("Bit 4 — RTX in a \"wet\" area", yesNo(bitOn(n2, 4))),
	}
}

func decodePositionType38FrameFlagDetail(frame0, frame1 byte, hasFrame1 bool) []Field {
	b01 := frame0 & 0x03
	var frame01 string
	switch b01 {
	case 0:
		frame01 = "Unknown / local (e.g. local site or not defined)"
	case 1:
		frame01 = "ITRF current epoch"
	case 2:
		frame01 = "ITRF fixed epoch"
	case 3:
		frame01 = "Unknown / local; position from RTX then frame-adjusted"
	}
	out := []Field{
		kv("Bits 1..0 — Reference frame", frame01),
	}
	for n := uint(2); n <= 6; n++ {
		out = appendReservedClearKV(out, frame0, n, fmt.Sprintf("Bit %d — Reserved (clear)", n))
	}
	out = append(out, kv("Bit 7 — Additional frame-flag byte follows", yesNo(bitOn(frame0, 7))))
	if hasFrame1 {
		out = append(out, kv("Following frame-flag byte", fmt.Sprintf("0x%02X · %08b", frame1, frame1)))
	}
	return out
}

func decodePositionType38RTKField(rtk byte) Field {
	low := rtk & 0x0F
	name := positionType38RTKCondition(int(low))
	val := fmt.Sprintf("%d — %s", low, name)
	if rtk != low {
		val += fmt.Sprintf(" (raw byte 0x%02X)", rtk)
	}
	return kv("RTK condition", val)
}

func positionType38RTKCondition(code int) string {
	switch code {
	case 0:
		return "New position computed"
	case 1:
		return "Unable to obtain a synced pair from both stations"
	case 2:
		return "Insufficient double-difference measurements"
	case 3:
		return "Reference position unavailable"
	case 4:
		return "Failed integer verification with fixed solution"
	case 5:
		return "Solution residual RMS over limit (rover) or pole wobbling (static)"
	case 6:
		return "PDOP exceeds PDOP mask (absolute positioning)"
	default:
		return fmt.Sprintf("Reserved / unknown (%d)", code)
	}
}

func formatITRFEpoch38(hundredths int16, frameBits01 byte) string {
	// 1/100 year since 2005-01-01 UTC; ignored for display semantics when "current epoch".
	if frameBits01 == 0x01 {
		return fmt.Sprintf("~%s (current epoch on output per OEM; raw ×0.01y = %d)", approxDateFromITRFHundredths(hundredths), hundredths)
	}
	return fmt.Sprintf("%s (×0.01 year from 2005-01-01 UTC)", approxDateFromITRFHundredths(hundredths))
}

func approxDateFromITRFHundredths(hundredths int16) string {
	// OEM: integer hundredths of a year from 2005-01-01 — approximate with mean Gregorian year.
	const daysPerHundredthYear = 365.2425 / 100.0
	t0 := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	d := time.Duration(float64(hundredths) * daysPerHundredthYear * 24 * float64(time.Hour))
	t := t0.Add(d)
	return t.UTC().Format("2006-01-02")
}

func formatRTXSubscriptionMinutes(v int32) string {
	if v == 0 {
		return "0 (hourly subscription not used)"
	}
	if v == -1 { // 0xFFFFFFFF as int32
		return "0xFFFFFFFF — minutes used or expired"
	}
	return fmt.Sprintf("%d", v)
}

func positionType38FixType(code int) string {
	if code >= 0 && code < len(positionType38FixNames) {
		s := positionType38FixNames[code]
		if s != "" {
			return fmt.Sprintf("%d — %s", code, s)
		}
	}
	return fmt.Sprintf("%d — unknown", code)
}

func positionType38TectonicPlate(code int) string {
	if code >= 0 && code < len(positionType38TectonicNames) {
		s := positionType38TectonicNames[code]
		if s != "" {
			return fmt.Sprintf("%d — %s", code, s)
		}
	}
	return fmt.Sprintf("%d — unknown", code)
}

// Trimble OEM "Position Fix Type" table (type 38).
var positionType38FixNames = []string{
	0:  "No fix or old position fix",
	1:  "Full measurement autonomous",
	2:  "Propagated autonomous",
	3:  "Full differential SBAS",
	4:  "Propagated SBAS",
	5:  "Full differential",
	6:  "Propagated differential",
	7:  "Full float RTK",
	8:  "Propagated float RTK",
	9:  "Full fixed-ambiguity RTK",
	10: "Propagated fixed-ambiguity RTK",
	11: "OmniSTAR HP differential",
	12: "OmniSTAR XP differential",
	13: "Location-RTK (dithered RTK)",
	14: "OmniSTAR VBS differential",
	15: "Beacon differential",
	16: "OmniSTAR HP/XP",
	17: "OmniSTAR HP/G2",
	18: "OmniSTAR G2",
	19: "Synchronous RTX",
	20: "Low-latency RTX",
	21: "OmniSTAR multiple source",
	22: "OmniSTAR L1-only",
	23: "INS autonomous",
	24: "INS SBAS",
	25: "INS code-phase DGNSS or OmniSTAR-VBS",
	26: "INS RTX code-phase corrections",
	27: "INS RTX carrier-phase corrections",
	28: "INS OmniSTAR HP/XP/G2",
	29: "INS RTK (fixed or float)",
	30: "INS dead reckoning",
	31: "RTX code-phase corrections",
	32: "RTX Fast in sync mode",
	33: "RTX Fast in low-latency mode",
	34: "RESERVED",
	35: "RESERVED",
	36: "xFill-RTX",
	37: "Low-latency RTX-RangePoint",
	38: "Synchronous RTX-RangePoint",
	39: "Low-latency RTX-ViewPoint",
	40: "Synchronous RTX-ViewPoint",
	41: "Low-latency RTX-FieldPoint",
	42: "Synchronous RTX-FieldPoint",
	43: "OmniSTAR G2+ solution type",
	44: "OmniSTAR G4+ solution type",
	45: "RESERVED",
	46: "RESERVED",
	47: "RESERVED",
	48: "L1S SLAS",
	49: "INS xFill-RTX",
	50: "CLAS",
	51: "INS CLAS",
}

// NNR-Morvel56 plate indices (Trimble OEM type 38).
var positionType38TectonicNames = []string{
	0:  "Unknown",
	1:  "Aegean Sea",
	2:  "Altiplano",
	3:  "Amurian",
	4:  "Anatolia",
	5:  "Antarctica",
	6:  "Arabia",
	7:  "Australia",
	8:  "Balmoral Reef",
	9:  "Banda Sea",
	10: "Birds Head",
	11: "Burma",
	12: "Capricorn",
	13: "Caribbean",
	14: "Caroline",
	15: "Cocos",
	16: "Conway Reef",
	17: "Easter",
	18: "Eurasia",
	19: "Futuna",
	20: "Galapagos",
	21: "India",
	22: "Juan de Fuca",
	23: "Juan Fernandez",
	24: "Kermadec",
	25: "Lwandle",
	26: "Macquarie",
	27: "Manus",
	28: "Maoke",
	29: "Mariana",
	30: "Molucca Sea",
	31: "Nazca",
	32: "New Hebrides",
	33: "Niuafoou",
	34: "North America",
	35: "North Andes",
	36: "North Bismarck",
	37: "Nubia",
	38: "Okhotsk",
	39: "Okinawa",
	40: "Pacific",
	41: "Panama",
	42: "Philippine Sea",
	43: "Rivera",
	44: "Sandwich",
	45: "Scotia",
	46: "Shetland",
	47: "Solomon Sea",
	48: "Somalia",
	49: "South America",
	50: "South Bismarck",
	51: "Sunda",
	52: "Sur",
	53: "Timor",
	54: "Tonga",
	55: "Woodlark",
	56: "Yangtze",
}
