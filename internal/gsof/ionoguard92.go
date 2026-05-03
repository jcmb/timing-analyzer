package gsof

import "fmt"

// IonoGuardSVEntry is one SV row for GSOF type 92 (IonoGuard info); used by dashboards and ParseIonoGuard92SVEntries.
type IonoGuardSVEntry struct {
	SystemName string `json:"system_name"`
	PRN        int    `json:"prn"`
	Status     string `json:"status"`
}

type iono92Parsed struct {
	weekU16           uint16
	towMs             uint32
	src, geo, station int
	nSats             int
	rows              []IonoGuardSVEntry
	parseNote         string
}

func parseIonoGuard92(payload []byte) (p iono92Parsed, headerOK bool) {
	const need = 2 + 4 + 4
	if len(payload) < need {
		return p, false
	}
	br := beReader{b: payload}
	p.weekU16 = br.u16()
	p.towMs = br.u32()
	p.src = int(br.u8())
	p.geo = int(br.u8())
	p.station = int(br.u8())
	p.nSats = int(br.u8())
	for i := 0; i < p.nSats; i++ {
		if !br.ok(3) {
			p.parseNote = fmt.Sprintf("truncated before SV row %d", i)
			break
		}
		sys := int(br.u8())
		prn := int(br.u8())
		met := int(br.u8())
		p.rows = append(p.rows, IonoGuardSVEntry{
			SystemName: gnssName(sys),
			PRN:        prn,
			Status:     iono92ActivityShort(met),
		})
	}
	return p, true
}

// ParseIonoGuard92SVEntries returns the declared SV count from the payload and decoded SV rows
// (a prefix if the payload is truncated mid-row). For len(payload) < 10, returns (0, nil).
func ParseIonoGuard92SVEntries(payload []byte) (declaredCount int, rows []IonoGuardSVEntry) {
	p, ok := parseIonoGuard92(payload)
	if !ok {
		return 0, nil
	}
	return p.nSats, p.rows
}

// decodeIonoGuard92 decodes GSOF type 0x5C (92) IonoGuard info.
// Layout: u16 GPS week, u32 GPS TOW (ms), u8 source, u8 geofence status, u8 station activity,
// u8 SV count, then count × (u8 GNSS system, u8 PRN, u8 SV metric). Per-SV rows are exposed via
// ParseIonoGuard92SVEntries for table UIs (short activity labels without parenthetical notes).
func decodeIonoGuard92(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(92).Function)}
	p, ok := parseIonoGuard92(payload)
	if !ok {
		return shortFields(Lookup(92).Function, payload, 2+4+4)
	}
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", p.weekU16)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(p.towMs)/1000.0)),
		kv("Station IonoGuard activity", iono92ActivityLabel(p.station)),
		kv("IonoGuard source", iono92SourceLabel(p.src)),
		kv("IonoGuard geofence", iono92GeofenceLabel(p.geo)),
		kv("SV count", fmt.Sprintf("%d", p.nSats)),
	)
	if p.parseNote != "" {
		out = append(out, kv("Parse", p.parseNote))
	}
	return out
}

func iono92SourceLabel(v int) string {
	switch v {
	case 0:
		return "0 — Unknown — not set"
	case 1:
		return "1 — Broadcast from RTK base station"
	case 2:
		return "2 — Computed at the rover from base observation"
	case 3:
		return "3 — Broadcast from RTX"
	case 255:
		return "255 — Invalid"
	default:
		return fmt.Sprintf("%d — unknown", v)
	}
}

func iono92GeofenceLabel(v int) string {
	switch v {
	case 0:
		return "0 — Inside Iono geofence"
	case 1:
		return "1 — Outside Iono geofence"
	case 255:
		return "255 — Unknown (does not affect IonoGuard, e.g. Trimble base corrections)"
	default:
		return fmt.Sprintf("%d — unknown", v)
	}
}

func iono92ActivityLabel(v int) string {
	switch v {
	case 0:
		return "0 — Green (no or negligible activity)"
	case 1:
		return "1 — Yellow"
	case 2:
		return "2 — Orange"
	case 3:
		return "3 — Red (high activity)"
	default:
		return fmt.Sprintf("%d — unknown", v)
	}
}

// iono92ActivityShort is the SV-table status text (no parenthetical notes).
func iono92ActivityShort(v int) string {
	switch v {
	case 0:
		return "Green"
	case 1:
		return "Yellow"
	case 2:
		return "Orange"
	case 3:
		return "Red"
	default:
		return fmt.Sprintf("%d — unknown", v)
	}
}

// decodeIonoGuard96 decodes GSOF type 0x60 (96) IonoGuard summary.
// Layout per OEM spec (Google Doc linked from catalog): u8 source, u8 geofence, u8 station activity,
// u8 green SV count, u8 yellow SV count, u8 orange SV count, u8 red SV count (all constellations).
func decodeIonoGuard96(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(96).Function)}
	const need = 7
	if len(payload) < need {
		return shortFields(Lookup(96).Function, payload, need)
	}
	br := beReader{b: payload}
	src := int(br.u8())
	geo := int(br.u8())
	station := int(br.u8())
	green := int(br.u8())
	yellow := int(br.u8())
	orange := int(br.u8())
	red := int(br.u8())
	out = append(out,
		kv("Station IonoGuard activity", iono92ActivityLabel(station)),
		kv("IonoGuard source", iono92SourceLabel(src)),
		kv("IonoGuard geofence", iono92GeofenceLabel(geo)),
		kv("Green SV count (all constellations)", fmt.Sprintf("%d", green)),
		kv("Yellow SV count (all constellations)", fmt.Sprintf("%d", yellow)),
		kv("Orange SV count (all constellations)", fmt.Sprintf("%d", orange)),
		kv("Red SV count (all constellations)", fmt.Sprintf("%d", red)),
	)
	if br.i < len(payload) {
		out = append(out, kv("Parse", fmt.Sprintf("%d trailing byte(s) after 7-byte layout", len(payload)-br.i)))
	}
	return out
}
