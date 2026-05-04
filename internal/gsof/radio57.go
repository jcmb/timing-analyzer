package gsof

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Radio57Row is one radio block from GSOF type 57 for table UIs (e.g. gsof-dashboard).
// Channel is populated only when ShowExpectedReservedBits is true at parse time.
type Radio57Row struct {
	Band           string `json:"band"`
	Channel        string `json:"channel,omitempty"`
	SignalStrength string `json:"signal_strength"`
	SignalBars     string `json:"signal_bars"`
	NoiseStrength  string `json:"noise_strength"`
	NoiseBars      string `json:"noise_bars"`
	ExtensionHex   string `json:"extension_hex,omitempty"`
	ShortBodyHex   string `json:"short_body_hex,omitempty"`
}

type radio57ParseResult struct {
	headerOK  bool
	week      uint16
	towMs     uint32
	nRadios   int
	rows      []Radio57Row
	parseNote string
}

func parseRadio57Result(payload []byte) radio57ParseResult {
	var r radio57ParseResult
	const header = 2 + 4 + 1
	if len(payload) < header {
		return r
	}
	includeChannel := ShowExpectedReservedBits
	br := beReader{b: payload}
	r.week = br.u16()
	r.towMs = br.u32()
	r.nRadios = int(br.u8())
	r.headerOK = true
	for i := 0; i < r.nRadios; i++ {
		if !br.ok(1) {
			r.parseNote = fmt.Sprintf("truncated before radio %d length byte", i)
			break
		}
		radLen := int(br.u8())
		if radLen < 1 {
			r.parseNote = fmt.Sprintf("invalid radio data length %d for radio %d", radLen, i)
			break
		}
		bodyLen := radLen - 1
		if !br.ok(bodyLen) {
			r.parseNote = fmt.Sprintf("truncated in radio %d group (need %d bytes after length)", i, bodyLen)
			break
		}
		if bodyLen < 8 {
			raw := make([]byte, bodyLen)
			for j := 0; j < bodyLen; j++ {
				raw[j] = br.u8()
			}
			row := Radio57Row{
				Band:           "—",
				SignalStrength: "—",
				SignalBars:     "—",
				NoiseStrength:  "—",
				NoiseBars:      "—",
				ShortBodyHex:   radio57Hex(raw),
			}
			if includeChannel {
				row.Channel = "—"
			}
			r.rows = append(r.rows, row)
			if r.parseNote == "" {
				r.parseNote = fmt.Sprintf("radio %d body shorter than 8 bytes (%d); see short_body_hex", i, bodyLen)
			}
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
		row := Radio57Row{
			Band:           radio57BandLabel(band),
			SignalStrength: radio57StrengthLabel(band, sig),
			SignalBars:     fmt.Sprintf("%d / 5", sigBars),
			NoiseStrength:  radio57NoiseLabel(noise),
			NoiseBars:      fmt.Sprintf("%d / 5", noiseBars),
		}
		if includeChannel {
			row.Channel = fmt.Sprintf("%d", ch)
		}
		if len(ext) > 0 {
			row.ExtensionHex = radio57Hex(ext)
		}
		r.rows = append(r.rows, row)
	}
	return r
}

// ParseRadio57Rows returns table rows for GSOF type 57; Channel is filled only when
// ShowExpectedReservedBits is true (same as gsof-dashboard -show-expected-reserved-bits).
func ParseRadio57Rows(payload []byte) ([]Radio57Row, string) {
	pr := parseRadio57Result(payload)
	return pr.rows, pr.parseNote
}

// decodeRadio57 decodes GSOF type 0x39 (57) radio info.
// Per-radio details are exposed as JSON rows via ParseRadio57Rows (radio_57) for table UIs.
func decodeRadio57(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(57).Function)}
	pr := parseRadio57Result(payload)
	if !pr.headerOK {
		return shortFields(Lookup(57).Function, payload, 2+4+1)
	}
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", pr.week)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(pr.towMs)/1000.0)),
	)
	if pr.week == 0 && pr.towMs == 0 {
		out = append(out, kv("GPS time availability", "Unavailable (week and ms are zero per specification)"))
	}
	out = append(out, kv("Radio count", fmt.Sprintf("%d", pr.nRadios)))
	if pr.parseNote != "" {
		out = append(out, kv("Parse", pr.parseNote))
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
