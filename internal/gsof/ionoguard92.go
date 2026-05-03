package gsof

import "fmt"

// decodeIonoGuard92 decodes GSOF type 0x5C (92) IonoGuard info.
// Layout: u16 GPS week, u32 GPS TOW (ms), u8 source, u8 geofence status, u8 station activity,
// u8 SV count, then count × (u8 GNSS system, u8 PRN, u8 SV metric).
func decodeIonoGuard92(payload []byte) []Field {
	out := []Field{kv("Summary", Lookup(92).Function)}
	const need = 2 + 4 + 4
	if len(payload) < need {
		return shortFields(Lookup(92).Function, payload, need)
	}
	br := beReader{b: payload}
	week := br.u16()
	towMs := br.u32()
	src := int(br.u8())
	geo := int(br.u8())
	station := int(br.u8())
	nSats := int(br.u8())
	out = append(out,
		kv("GPS week", fmt.Sprintf("%d", week)),
		kv("GPS time of week", fmt.Sprintf("%.3f s", float64(towMs)/1000.0)),
		kv("Station IonoGuard activity", iono92ActivityLabel(station)),
		kv("IonoGuard source", iono92SourceLabel(src)),
		kv("IonoGuard geofence", iono92GeofenceLabel(geo)),
		kv("SV count", fmt.Sprintf("%d", nSats)),
	)
	for i := 0; i < nSats; i++ {
		if !br.ok(3) {
			out = append(out, kv("Parse", fmt.Sprintf("truncated before SV row %d", i)))
			return out
		}
		sys := int(br.u8())
		prn := int(br.u8())
		met := int(br.u8())
		pfx := fmt.Sprintf("SV %d", i)
		out = append(out,
			kv(pfx+" system", gnssName(sys)),
			kv(pfx+" PRN", fmt.Sprintf("%d", prn)),
			kv(pfx+" IonoGuard activity", iono92ActivityLabel(met)),
		)
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
