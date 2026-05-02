package gsof

import (
	"fmt"
	"strings"
)

// lBandStatusPayloadBytes is the GSOF type 40 payload size (Trimble OEM L-Band status + ROS driver alignment).
// Source: https://receiverhelp.trimble.com/oem-gnss/gsof-messages-l-band.html
const lBandStatusPayloadBytes = 70

// decodeLBandStatus decodes GSOF record type 40 (L-Band status information).
func decodeLBandStatus(payload []byte) []Field {
	m := Lookup(40)
	if len(payload) < lBandStatusPayloadBytes {
		return shortFields(m.Function, payload, lBandStatusPayloadBytes)
	}
	br := beReader{b: payload}
	name := br.str5()
	nomMHz := br.f32()
	bitrate := br.u16()
	snr := br.f32()
	hpxpEngine := br.u8()
	hpxpLib := br.u8()
	vbsLib := br.u8()
	beam := br.u8()
	motion := br.u8()
	sigH := br.f32()
	sigV := br.f32()
	nmeaEnc := br.u8()
	iq := br.f32()
	ber := br.f32()
	totalUniqueWords := br.u32()
	wordsWithErr := br.u32()
	badWordBits := br.u32()
	viterbiSym := br.u32()
	viterbiCorr := br.u32()
	badMsgs := br.u32()
	measValid := br.u8()
	measHz := br.f64()

	satNote := ""
	if strings.HasPrefix(name, "Custo") {
		satNote = " (custom beam: first 5 characters of \"Custom\")"
	}
	nameDisp := name
	if nameDisp == "" {
		nameDisp = "(blank)"
	}

	out := []Field{
		kv("Summary", m.Function),
		kv("Satellite name", nameDisp+satNote),
		kv("Satellite frequency (MHz)", fmt.Sprintf("%g", nomMHz)),
		kv("Satellite bit rate (Hz)", fmt.Sprintf("%d", bitrate)),
		kv("SNR (C/No)", fmt.Sprintf("%g dB-Hz", snr)),
		kv("HP/XP subscribed engine", decodeLBandHPXPEngine(hpxpEngine)),
		kv("HP/XP library mode", decodeLBandLibraryMode(hpxpLib)),
		kv("VBS library mode", decodeLBandLibraryMode(vbsLib)),
		kv("Beam mode", decodeLBandBeamMode(beam)),
		kv("OmniSTAR motion", decodeLBandOmniMotion(motion)),
		kv("3-sigma horizontal precision threshold", fmt.Sprintf("%g", sigH)),
		kv("3-sigma vertical precision threshold", fmt.Sprintf("%g", sigV)),
		kv("NMEA encryption state", decodeLBandNMEAEnc(nmeaEnc)),
		kv("I/Q ratio", fmt.Sprintf("%g (mean I / mean Q power)", iq)),
		kv("Estimated bit error rate", fmt.Sprintf("%g", ber)),
		kv("Total unique words", fmt.Sprintf("%d (since last search)", totalUniqueWords)),
		kv("Total unique words with bit errors", fmt.Sprintf("%d", wordsWithErr)),
		kv("Total bad unique word bits", fmt.Sprintf("%d", badWordBits)),
		kv("Total Viterbi symbols", fmt.Sprintf("%d (wraps near 0xFFFFFF00 per OEM)", viterbiSym)),
		kv("Corrected Viterbi symbols", fmt.Sprintf("%d (resets with symbol count)", viterbiCorr)),
		kv("Bad messages", fmt.Sprintf("%d (non-zero flush byte)", badMsgs)),
		kv("MEAS frequency valid", decodeLBandMeasValid(measValid)),
		kv("MEAS frequency (Hz)", fmt.Sprintf("%g", measHz)),
	}
	if br.i < len(payload) {
		out = append(out, kv("Trailing payload (hex)", hexPreview(payload[br.i:], 64)))
	}
	return out
}

func decodeLBandHPXPEngine(b byte) string {
	switch b {
	case 0:
		return "0 — XP"
	case 1:
		return "1 — HP"
	case 2:
		return "2 — G2"
	case 3:
		return "3 — HP + G2"
	case 4:
		return "4 — HP + XP"
	case 0xFF:
		return "0xFF — Unknown"
	default:
		return fmt.Sprintf("%d — (reserved / unknown)", b)
	}
}

func decodeLBandLibraryMode(b byte) string {
	switch b {
	case 0:
		return "0 — Library inactive"
	case 1:
		return "1 — Library active"
	default:
		return fmt.Sprintf("%d — (unknown)", b)
	}
}

func decodeLBandBeamMode(b byte) string {
	switch b {
	case 0:
		return "0 — Off"
	case 1:
		return "1 — FFT initializing"
	case 2:
		return "2 — FFT running"
	case 3:
		return "3 — Search initializing"
	case 4:
		return "4 — Search running"
	case 5:
		return "5 — Track initializing"
	case 6:
		return "6 — Track searching"
	case 7:
		return "7 — Tracking"
	default:
		return fmt.Sprintf("%d — (unknown)", b)
	}
}

func decodeLBandOmniMotion(b byte) string {
	switch b {
	case 0:
		return "0 — Dynamic"
	case 1:
		return "1 — Static"
	case 2:
		return "2 — OmniSTAR not ready"
	case 0xFF:
		return "0xFF — Unknown"
	default:
		return fmt.Sprintf("%d — (unknown)", b)
	}
}

func decodeLBandNMEAEnc(b byte) string {
	switch b {
	case 0:
		return "0 — Encryption not applied to NMEA"
	case 1:
		return "1 — Encryption applied to NMEA"
	default:
		return fmt.Sprintf("%d — (unknown)", b)
	}
}

func decodeLBandMeasValid(b byte) string {
	switch b {
	case 0:
		return "0 — MEAS frequency may be out by a significant amount"
	case 1:
		return "1 — MEAS frequency is accurate"
	default:
		return fmt.Sprintf("%d — (unknown)", b)
	}
}
