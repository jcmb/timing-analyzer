package gsof

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func TestDecode01(t *testing.T) {
	// jcmb GSOF.py: unpack_from('>L H B B B B', ...) — big-endian
	payload := []byte{
		0x00, 0x00, 0x03, 0xE8, // GPS TOW raw = 1000
		0x08, 0xDF, // week = 2271
		0x05,       // SVs used
		0x00, 0x00, // flags
		0x00, // init counter
	}
	fields := Decode(1, payload)
	if len(fields) < 4 {
		t.Fatalf("fields %#v", fields)
	}
	var tow, week, svs string
	var flags1, flags2 *Field
	for i := range fields {
		f := &fields[i]
		switch f.Label {
		case "GPS time of week":
			tow = f.Value
		case "GPS week":
			week = f.Value
		case "SVs used":
			svs = f.Value
		case "Flags 1":
			flags1 = f
		case "Flags 2":
			flags2 = f
		}
	}
	if tow != "1.00 s" || week != "2271" || svs != "5" {
		t.Fatalf("decode mismatch tow=%q week=%q svs=%q\n%s", tow, week, svs, fieldText(fields))
	}
	// Reserved bits at expected values are omitted by default (bit 6 clear);
	// bit 4 should be set but is clear in this payload → one reserved row.
	if flags1 == nil || len(flags1.Detail) != 7 {
		t.Fatalf("Flags 1 detail: %#v", flags1)
	}
	if flags2 == nil || len(flags2.Detail) != 8 {
		t.Fatalf("Flags 2 detail: %#v", flags2)
	}
}

func fieldText(fields []Field) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f.Label)
		b.WriteString("=")
		b.WriteString(f.Value)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestCatalogOverviewURL(t *testing.T) {
	if Lookup(99).DocURL() != OverviewURL {
		t.Fatal("unknown should link to overview")
	}
}

func TestCatalogDocURLs129(t *testing.T) {
	const base = "https://receiverhelp.trimble.com/oem-gnss/"
	if Lookup(1).DocURL() != base+"gsof-messages-time.html" {
		t.Fatalf("type 1 doc: %s", Lookup(1).DocURL())
	}
	if Lookup(2).DocURL() != base+"gsof-messages-llh.html" {
		t.Fatalf("type 2 doc: %s", Lookup(2).DocURL())
	}
	if Lookup(3).DocURL() != base+"gsof-messages-ecef.html" {
		t.Fatalf("type 3 doc: %s", Lookup(3).DocURL())
	}
	if Lookup(6).DocURL() != base+"gsof-messages-ecef-delta.html" {
		t.Fatalf("type 6 doc: %s", Lookup(6).DocURL())
	}
	if Lookup(7).DocURL() != base+"gsof-messages-tplane-enu.html" {
		t.Fatalf("type 7 doc: %s", Lookup(7).DocURL())
	}
	if Lookup(8).DocURL() != base+"gsof-messages-velocity.html" {
		t.Fatalf("type 8 doc: %s", Lookup(8).DocURL())
	}
	if Lookup(9).DocURL() != base+"gsof-messages-pdop.html" {
		t.Fatalf("type 9 doc: %s", Lookup(9).DocURL())
	}
	if Lookup(10).DocURL() != base+"gsof-messages-clock-info.html" {
		t.Fatalf("type 10 doc: %s", Lookup(10).DocURL())
	}
	if Lookup(11).DocURL() != base+"gsof-messages-position-vcv.html" {
		t.Fatalf("type 11 doc: %s", Lookup(11).DocURL())
	}
	if Lookup(12).DocURL() != base+"gsof-messages-sigma.html" {
		t.Fatalf("type 12 doc: %s", Lookup(12).DocURL())
	}
	if Lookup(13).DocURL() != base+"gsof-messages-sv-brief.html" {
		t.Fatalf("type 13 doc: %s", Lookup(13).DocURL())
	}
	if Lookup(33).DocURL() != base+"gsof-messages-all-sv-brief.html" {
		t.Fatalf("type 33 doc: %s", Lookup(33).DocURL())
	}
	if Lookup(34).DocURL() != base+"gsof-messages-all-sv-detail.html" {
		t.Fatalf("type 34 doc: %s", Lookup(34).DocURL())
	}
	if Lookup(48).DocURL() != base+"gsof-messages-multiple-page-detail-all-sv.html" {
		t.Fatalf("type 48 doc: %s", Lookup(48).DocURL())
	}
	if Lookup(35).DocURL() != base+"gsof-messages-received-base-info.html" {
		t.Fatalf("type 35 doc: %s", Lookup(35).DocURL())
	}
	if Lookup(37).DocURL() != base+"gsof-messages-batt-mem.html" {
		t.Fatalf("type 37 doc: %s", Lookup(37).DocURL())
	}
	if Lookup(15).DocURL() != base+"gsof-messages-receiver-serial-no.html" {
		t.Fatalf("type 15 doc: %s", Lookup(15).DocURL())
	}
	if Lookup(14).DocURL() != base+"gsof-messages-sv-detail.html" {
		t.Fatalf("type 14 doc: %s", Lookup(14).DocURL())
	}
	if Lookup(16).DocURL() != base+"gsof-messages-utc.html" {
		t.Fatalf("type 16 doc: %s", Lookup(16).DocURL())
	}
	if Lookup(38).DocURL() != base+"gsof-messages-position-type.html" {
		t.Fatalf("type 38 doc: %s", Lookup(38).DocURL())
	}
	if Lookup(40).DocURL() != base+"gsof-messages-l-band.html" {
		t.Fatalf("type 40 doc: %s", Lookup(40).DocURL())
	}
	if Lookup(41).DocURL() != base+"gsof-messages-base-position-quality-indicator.html" {
		t.Fatalf("type 41 doc: %s", Lookup(41).DocURL())
	}
	if Lookup(70).DocURL() != base+"gsof-messages-llmsl.html" {
		t.Fatalf("type 70 doc: %s", Lookup(70).DocURL())
	}
	const nma91 = "https://docs.google.com/document/d/1mxY_s34PX3jYNNM81WvM0gDJL_dQKDPsxqa5TdHiepM/edit?tab=t.0"
	if Lookup(91).DocURL() != nma91 {
		t.Fatalf("type 91 doc: %s", Lookup(91).DocURL())
	}
	const iono92 = "https://docs.google.com/document/d/1aIc38r95I3LCiIycIj_VmDws7jat2ed55j0Ve6U8tjM/edit?usp=sharing"
	if Lookup(92).DocURL() != iono92 {
		t.Fatalf("type 92 doc: %s", Lookup(92).DocURL())
	}
	const iono96 = "https://docs.google.com/document/d/1FEliQDO_vcX1KZqz8pjy0DcXZNEfA1hXipYMjvKWbF4/edit?usp=sharing"
	if Lookup(96).DocURL() != iono96 {
		t.Fatalf("type 96 doc: %s", Lookup(96).DocURL())
	}
}

func TestDecode91NMALayout(t *testing.T) {
	// Week 7, TOW 2000 ms, 1 NMA: OSNMA, GPS LNAV, N=1, auth=0x03, fail=0x01
	payload := []byte{
		0x00, 0x00, 0x00, 0x07,
		0x00, 0x00, 0x07, 0xd0,
		0x01,
		0x00, 0x00, 0x01,
		0x03,
		0x01,
	}
	fields := Decode(91, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Summary"], "Authentication") && !strings.Contains(got["Summary"], "NMA") {
		t.Fatalf("summary: %q", got["Summary"])
	}
	if got["GPS week"] != "7" || got["GPS time of week"] != "2.000 s" || got["NMA count"] != "1" {
		t.Fatalf("header: %#v", got)
	}
	if got["NMA 0 source"] != "0 — OSNMA" {
		t.Fatalf("source: %q", got["NMA 0 source"])
	}
	if !strings.Contains(got["NMA 0 authenticated mask (hex)"], "03") {
		t.Fatalf("auth mask: %q", got["NMA 0 authenticated mask (hex)"])
	}
	if !strings.Contains(got["NMA 0 failed mask (hex)"], "01") {
		t.Fatalf("fail mask: %q", got["NMA 0 failed mask (hex)"])
	}
}

func TestDecode91NMATruncated(t *testing.T) {
	fields := Decode(91, []byte{0, 0, 0, 1, 0, 0, 0, 0, 2, 0})
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Parse"], "truncated") {
		t.Fatalf("expected parse note: %#v", got)
	}
}

func TestDecode92IonoGuardShortPayload(t *testing.T) {
	fields := Decode(92, []byte{0x01, 0x02, 0x03})
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Parse"], "10") {
		t.Fatalf("parse: %#v", got)
	}
}

func TestDecode92IonoGuardLayout(t *testing.T) {
	// Week 2, TOW 0, RTK base, inside geofence, station orange, 1 SV: GPS PRN 12 yellow
	payload := []byte{
		0x00, 0x02,
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x02, 0x01,
		0x00, 12, 0x01,
	}
	fields := Decode(92, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["GPS week"] != "2" || got["GPS time of week"] != "0.000 s" {
		t.Fatalf("time header: %#v", got)
	}
	if !strings.Contains(got["IonoGuard source"], "RTK base") {
		t.Fatalf("source: %q", got["IonoGuard source"])
	}
	if got["SV count"] != "1" {
		t.Fatalf("count: %#v", got)
	}
	if got["SV 0 system"] != "GPS" || got["SV 0 PRN"] != "12" {
		t.Fatalf("sv0: %#v", got)
	}
	if !strings.Contains(got["SV 0 IonoGuard activity"], "Yellow") {
		t.Fatalf("sv0 metric: %q", got["SV 0 IonoGuard activity"])
	}
}

func TestDecode92IonoGuardTruncatedSV(t *testing.T) {
	payload := []byte{
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0xFF, 0x00, 0x01,
		0x00, 0x07,
	}
	fields := Decode(92, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Parse"], "truncated") {
		t.Fatalf("expected parse note: %#v", got)
	}
}

func TestDecode96IonoGuardShortPayload(t *testing.T) {
	fields := Decode(96, []byte{0x01, 0x02, 0x03})
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Parse"], "7") {
		t.Fatalf("parse: %#v", got)
	}
}

func TestDecode96IonoGuardLayout(t *testing.T) {
	// Wire: source, geofence, station, green, yellow, orange, red
	payload := []byte{255, 255, 1, 5, 2, 1, 0}
	fields := Decode(96, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["IonoGuard source"], "Invalid") {
		t.Fatalf("source: %q", got["IonoGuard source"])
	}
	if !strings.Contains(got["IonoGuard geofence"], "255") || !strings.Contains(got["IonoGuard geofence"], "Unknown") {
		t.Fatalf("geofence: %q", got["IonoGuard geofence"])
	}
	if !strings.Contains(got["Station IonoGuard activity"], "Yellow") {
		t.Fatalf("station: %q", got["Station IonoGuard activity"])
	}
	if got["Green SV count (all constellations)"] != "5" ||
		got["Yellow SV count (all constellations)"] != "2" ||
		got["Orange SV count (all constellations)"] != "1" ||
		got["Red SV count (all constellations)"] != "0" {
		t.Fatalf("counts: %#v", got)
	}
}

func TestDecode96IonoGuardTrailingBytes(t *testing.T) {
	payload := []byte{0, 0, 0, 0, 0, 0, 0, 0xFF}
	fields := Decode(96, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Parse"], "trailing") {
		t.Fatalf("parse: %#v", got)
	}
}

func TestDecode70LLMSL(t *testing.T) {
	payload := make([]byte, 24+5)
	binary.BigEndian.PutUint64(payload[0:], math.Float64bits(math.Pi/2))
	binary.BigEndian.PutUint64(payload[8:], math.Float64bits(-math.Pi/6))
	binary.BigEndian.PutUint64(payload[16:], math.Float64bits(1647.384))
	copy(payload[24:], []byte("EGM96"))
	fields := Decode(70, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["Geoid model"] != "EGM96" {
		t.Fatalf("model: %q", got["Geoid model"])
	}
	if !strings.Contains(got["MSL height (m)"], "1647.384") {
		t.Fatalf("height: %q", got["MSL height (m)"])
	}
	if !strings.Contains(got["Latitude (decimal °)"], "90.00000000") {
		t.Fatalf("lat: %q", got["Latitude (decimal °)"])
	}
}

func TestDecode02LatLonRad(t *testing.T) {
	payload := make([]byte, 24)
	binary.BigEndian.PutUint64(payload[0:], math.Float64bits(math.Pi/2))
	binary.BigEndian.PutUint64(payload[8:], math.Float64bits(-math.Pi/6))
	binary.BigEndian.PutUint64(payload[16:], math.Float64bits(123.456789))
	fields := Decode(2, payload)
	var latDec, latDMS, lonDec, lonDMS, height string
	for _, f := range fields {
		switch f.Label {
		case "Latitude (decimal °)":
			latDec = f.Value
		case "Latitude (DMS)":
			latDMS = f.Value
		case "Longitude (decimal °)":
			lonDec = f.Value
		case "Longitude (DMS)":
			lonDMS = f.Value
		case "Height (m)":
			height = f.Value
		}
	}
	if latDec != "90.00000000" {
		t.Fatalf("lat decimal: %q", latDec)
	}
	if !strings.HasPrefix(latDMS, "N 90°") || !strings.Contains(latDMS, "0.00000") {
		t.Fatalf("lat DMS: %q", latDMS)
	}
	if lonDec != "-30.00000000" {
		t.Fatalf("lon decimal: %q", lonDec)
	}
	if !strings.HasPrefix(lonDMS, "W 30°") {
		t.Fatalf("lon DMS: %q", lonDMS)
	}
	if height != "\u00a0123.457" {
		t.Fatalf("height: %q", height)
	}
}

func TestDecode11VCVUnitsAndDecimals(t *testing.T) {
	payload := make([]byte, 34)
	binary.BigEndian.PutUint32(payload[0:], math.Float32bits(1.25))
	for i := 1; i < 8; i++ {
		binary.BigEndian.PutUint32(payload[i*4:], math.Float32bits(-0.5))
	}
	binary.BigEndian.PutUint16(payload[32:], 42)
	fields := Decode(11, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["POSITION_RMS (m)"] != "\u00a01.25000 m" {
		t.Fatalf("POSITION_RMS: %q", got["POSITION_RMS (m)"])
	}
	if got["VCV_xx (m²)"] != "-0.50000 m²" {
		t.Fatalf("VCV_xx: %q", got["VCV_xx (m²)"])
	}
	if got["NUMBER_OF_EPOCHS"] != "42" {
		t.Fatalf("epochs: %q", got["NUMBER_OF_EPOCHS"])
	}
}

func TestDecode12SigmaUnitsAndOrientation(t *testing.T) {
	payload := make([]byte, 38)
	binary.BigEndian.PutUint32(payload[0:], math.Float32bits(2))
	binary.BigEndian.PutUint32(payload[4:], math.Float32bits(3))
	binary.BigEndian.PutUint32(payload[8:], math.Float32bits(4))
	binary.BigEndian.PutUint32(payload[12:], math.Float32bits(-1))
	binary.BigEndian.PutUint32(payload[16:], math.Float32bits(5))
	binary.BigEndian.PutUint32(payload[20:], math.Float32bits(6))
	binary.BigEndian.PutUint32(payload[24:], math.Float32bits(7))
	binary.BigEndian.PutUint32(payload[28:], math.Float32bits(45)) // °
	binary.BigEndian.PutUint32(payload[32:], math.Float32bits(1))
	binary.BigEndian.PutUint16(payload[36:], 3088)

	fields := Decode(12, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["POSITION_RMS (m)"] != "\u00a02.00000 m" {
		t.Fatalf("POSITION_RMS: %q", got["POSITION_RMS (m)"])
	}
	if got["COVAR_EAST_NORTH (m²)"] != "-1.00000 m²" {
		t.Fatalf("COVAR: %q", got["COVAR_EAST_NORTH (m²)"])
	}
	if got["ORIENTATION (decimal °)"] != "45.00000000" {
		t.Fatalf("orient dec: %q", got["ORIENTATION (decimal °)"])
	}
	if got["ORIENTATION (DMS)"] != "45° 0′ 0.00000″" {
		t.Fatalf("orient DMS: %q", got["ORIENTATION (DMS)"])
	}
	if got["UNIT_VARIANCE"] != "1.00000" {
		t.Fatalf("unit var: %q", got["UNIT_VARIANCE"])
	}
	if got["NUMBER_EPOCHS"] != "3088" {
		t.Fatalf("epochs: %q", got["NUMBER_EPOCHS"])
	}
	// √(3² + 4²) = 5 m
	if got["SIGMA_H (m)"] != "\u00a05.00000 m" {
		t.Fatalf("SIGMA_H: %q", got["SIGMA_H (m)"])
	}
}

func TestDecode09PDOPOneDecimal(t *testing.T) {
	payload := make([]byte, 16)
	binary.BigEndian.PutUint32(payload[0:], math.Float32bits(1.25))
	binary.BigEndian.PutUint32(payload[4:], math.Float32bits(2.5))
	binary.BigEndian.PutUint32(payload[8:], math.Float32bits(3.75))
	binary.BigEndian.PutUint32(payload[12:], math.Float32bits(4))
	fields := Decode(9, payload)
	want := map[string]string{
		"PDOP": "1.2", "HDOP": "2.5", "TDOP": "3.8", "VDOP": "4.0",
	}
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q (all %#v)", k, got[k], v, got)
		}
	}
}

func TestDecode33AllSVBriefFields(t *testing.T) {
	// One SV: PRN 7, GPS (0), flags1=0xAA, flags2=0xBB
	payload := []byte{0x01, 0x07, 0x00, 0xaa, 0xbb}
	fields := Decode(33, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["Summary"] != Lookup(33).Function {
		t.Fatalf("summary: %q", got["Summary"])
	}
	if got["SV count"] != "1" {
		t.Fatalf("count: %q", got["SV count"])
	}
	if got["SV 0 SV system"] != "GPS" || got["SV 0 PRN"] != "7" {
		t.Fatalf("row0: %#v", got)
	}
	if got["SV 0 Flags 1 (binary)"] != "10101010" || got["SV 0 Flags 2 (binary)"] != "10111011" {
		t.Fatalf("flags: %#v", got)
	}
}

func TestDecode48MultiPageHeader(t *testing.T) {
	// Version 2, page-info 0x34 → current page 3, total pages 4 (Trimble nibbles), zero SV rows.
	payload := []byte{2, 0x34, 0}
	fields := Decode(48, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["Format version"] != "2" {
		t.Fatalf("version: %#v", got)
	}
	if got["SV count (this page)"] != "0" {
		t.Fatalf("count: %#v", got)
	}
	if !strings.Contains(got["Page"], "3 of 4") || !strings.Contains(got["Page"], "0x34") {
		t.Fatalf("page field: %q", got["Page"])
	}
}

func TestDecode28ReceiverDiagnostics(t *testing.T) {
	m := Lookup(28)
	if m.Title != "Receiver diagnostics" {
		t.Fatalf("catalog title: %q", m.Title)
	}
	payload := make([]byte, 18)
	payload[5] = 0x80 // bit 7 set: Ref Station Info received
	payload[6] = 255  // link integrity → 100 %
	payload[9] = 11
	payload[10] = 12
	payload[11] = 25 // 2.5 s latency
	payload[13] = 8
	fields := Decode(28, payload)
	got := make(map[string]string)
	var base *Field
	for i := range fields {
		f := &fields[i]
		if f.Label == "Base flags" {
			base = f
			continue
		}
		got[f.Label] = f.Value
	}
	if got["Summary"] != m.Function {
		t.Fatalf("summary: %q", got["Summary"])
	}
	if strings.Contains(strings.Join(fieldsToLabels(fields), ","), "Reserved") {
		t.Fatalf("reserved rows should be hidden by default: %v", fieldsToLabels(fields))
	}
	if got["Link integrity (last 100 s)"] != "\u00a0100.0 %" {
		t.Fatalf("link %%: %q", got["Link integrity (last 100 s)"])
	}
	if got["Common L1 SVs"] != "11" || got["Common L2 SVs"] != "12" || got["Diff SVs in use"] != "8" {
		t.Fatalf("counts: %#v", got)
	}
	if got["Datalink latency"] != "2.5 s" {
		t.Fatalf("latency: %q", got["Datalink latency"])
	}
	if base == nil || len(base.Detail) != 1 {
		t.Fatalf("base flags detail: %#v", base)
	}
	if base.Detail[0].Label != "Bit 7 — Ref Station Info received" || base.Detail[0].Value != "Yes" {
		t.Fatalf("bit 7: %#v", base.Detail[0])
	}

	t.Cleanup(func() { ShowExpectedReservedBits = false })
	ShowExpectedReservedBits = true
	fields2 := Decode(28, payload)
	var sawReserved bool
	for _, f := range fields2 {
		if strings.HasPrefix(f.Label, "Reserved") {
			sawReserved = true
			break
		}
	}
	if !sawReserved {
		t.Fatalf("expected reserved rows with ShowExpectedReservedBits: %v", fieldsToLabels(fields2))
	}
}

func fieldsToLabels(fields []Field) []string {
	out := make([]string, len(fields))
	for i := range fields {
		out[i] = fields[i].Label
	}
	return out
}

func TestDecode10ClockFlags(t *testing.T) {
	payload := make([]byte, 17)
	payload[0] = 0x07 // bits 0–2 set
	fields := Decode(10, payload)
	var cf *Field
	for i := range fields {
		if fields[i].Label == "Clock flags" {
			cf = &fields[i]
			break
		}
	}
	// Bits 3–7 reserved and zero → hidden unless ShowExpectedReservedBits.
	if cf == nil || len(cf.Detail) != 3 {
		t.Fatalf("clock flags: %#v", cf)
	}
	if cf.Value != "0x07 · 00000111" {
		t.Fatalf("value: %q", cf.Value)
	}
}

func TestDecode08VelocityFlags(t *testing.T) {
	payload := make([]byte, 17)
	payload[0] = 0x05 // bits 0 and 2 set; 1 clear (Doppler, velocity valid, heading valid)
	fields := Decode(8, payload)
	var vf *Field
	for i := range fields {
		if fields[i].Label == "Velocity flags" {
			vf = &fields[i]
			break
		}
	}
	if vf == nil || len(vf.Detail) != 3 {
		t.Fatalf("velocity flags field: %#v", vf)
	}
	if vf.Value != "0x05 · 00000101" {
		t.Fatalf("value: %q", vf.Value)
	}
}

func TestDecode08VelocityFieldOrderAndUnits(t *testing.T) {
	payload := make([]byte, 17)
	payload[0] = 0x00
	binary.BigEndian.PutUint32(payload[1:], math.Float32bits(1))    // horizontal m/s
	binary.BigEndian.PutUint32(payload[5:], math.Float32bits(90))   // heading
	binary.BigEndian.PutUint32(payload[9:], math.Float32bits(2))    // vertical m/s
	binary.BigEndian.PutUint32(payload[13:], math.Float32bits(180)) // local heading
	fields := Decode(8, payload)
	var labels []string
	for _, f := range fields {
		if f.Label != "Velocity flags" {
			labels = append(labels, f.Label)
		}
	}
	wantLabels := []string{
		"Velocity", "Vertical velocity",
		"Velocity (km/h)", "Vertical velocity (km/h)",
		"Heading", "Local heading",
	}
	if len(labels) != len(wantLabels) {
		t.Fatalf("labels: %v want %v", labels, wantLabels)
	}
	for i := range wantLabels {
		if labels[i] != wantLabels[i] {
			t.Fatalf("label[%d]: got %q want %q (all %v)", i, labels[i], wantLabels[i], labels)
		}
	}
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["Velocity"] != "\u00a01.000 m/s" || got["Velocity (km/h)"] != "\u00a03.600 km/h" {
		t.Fatalf("horizontal speed: %#v", got)
	}
	if got["Vertical velocity"] != "\u00a02.000 m/s" || got["Vertical velocity (km/h)"] != "\u00a07.200 km/h" {
		t.Fatalf("vertical speed: %#v", got)
	}
	if got["Heading"] != "90" || got["Local heading"] != "180" {
		t.Fatalf("headings: %#v", got)
	}
}

func TestDecode03ECEFFormat(t *testing.T) {
	payload := make([]byte, 24)
	binary.BigEndian.PutUint64(payload[0:], math.Float64bits(1.25))
	binary.BigEndian.PutUint64(payload[8:], math.Float64bits(-2.5))
	binary.BigEndian.PutUint64(payload[16:], math.Float64bits(100))
	fields := Decode(3, payload)
	var x, y, z string
	for _, f := range fields {
		switch f.Label {
		case "X (m)":
			x = f.Value
		case "Y (m)":
			y = f.Value
		case "Z (m)":
			z = f.Value
		}
	}
	if x != "\u00a01.250" || y != "-2.500" || z != "\u00a0100.000" {
		t.Fatalf("ECEF m: x=%q y=%q z=%q", x, y, z)
	}
}

func TestDecode16CurrentTimeLikeType1Layout(t *testing.T) {
	payload := make([]byte, 9)
	binary.BigEndian.PutUint32(payload[0:], 5000) // 5.000 s
	binary.BigEndian.PutUint16(payload[4:], 2271)
	binary.BigEndian.PutUint16(payload[6:], 0)
	payload[8] = 0x03 // bits 0,1 set
	fields := Decode(16, payload)
	got := make(map[string]string)
	var tf *Field
	for i := range fields {
		got[fields[i].Label] = fields[i].Value
		if fields[i].Label == "Current time flags" {
			tf = &fields[i]
		}
	}
	if got["Summary"] == "" {
		t.Fatal("missing Summary")
	}
	if got["UTC time of week"] != "5.00 s" {
		t.Fatalf("tow: %q", got["UTC time of week"])
	}
	if got["UTC week"] != "2271" {
		t.Fatalf("week: %q", got["UTC week"])
	}
	if tf == nil || len(tf.Detail) != 2 {
		t.Fatalf("time flags detail len %v %#v", tf != nil, tf)
	}
}

func TestDecode16TimeFlagsVerboseReserved(t *testing.T) {
	t.Cleanup(func() { ShowExpectedReservedBits = false })
	ShowExpectedReservedBits = true

	payload := make([]byte, 9)
	binary.BigEndian.PutUint32(payload[0:], 0)
	binary.BigEndian.PutUint16(payload[4:], 0)
	binary.BigEndian.PutUint16(payload[6:], 0)
	payload[8] = 0x00
	fields := Decode(16, payload)
	var tf *Field
	for i := range fields {
		if fields[i].Label == "Current time flags" {
			tf = &fields[i]
			break
		}
	}
	if tf == nil || len(tf.Detail) != 8 {
		t.Fatalf("verbose time flags: %#v", tf)
	}
}

func TestDecode14SVDetailedRows(t *testing.T) {
	payload := []byte{
		1,
		2, 0x03, 0x0C, 250,
		0, 90,
		8, 12,
	}
	fields := Decode(14, payload)
	if len(fields) < 8 {
		t.Fatalf("fields %#v", fields)
	}
	_, rows := ParseSVDetailedEntries(payload)
	if len(rows) != 1 || rows[0].PRN != 2 || rows[0].Azimuth != 90 {
		t.Fatalf("parse %+v", rows)
	}
}

func TestDecode35ReceivedBase(t *testing.T) {
	payload := make([]byte, 35)
	payload[0] = 0x09 // version 1, bit 3 set (base info valid)
	copy(payload[1:9], []byte("BASE____"))
	binary.BigEndian.PutUint16(payload[9:], 0xabcd)
	latDeg := 40.7128
	lonDeg := -74.0060
	latRad := latDeg * math.Pi / 180
	lonRad := lonDeg * math.Pi / 180
	binary.BigEndian.PutUint64(payload[11:], math.Float64bits(latRad))
	binary.BigEndian.PutUint64(payload[19:], math.Float64bits(lonRad))
	binary.BigEndian.PutUint64(payload[27:], math.Float64bits(10.5))
	fields := Decode(35, payload)
	got := map[string]string{}
	var baseFlags *Field
	for i := range fields {
		f := &fields[i]
		if f.Label == "Base flags" {
			baseFlags = f
			continue
		}
		got[f.Label] = f.Value
	}
	if got["Summary"] != Lookup(35).Function {
		t.Fatalf("summary %q", got["Summary"])
	}
	if got["Base name"] != "BASE____" {
		t.Fatalf("name %q", got["Base name"])
	}
	if got["Base ID"] != "43981" {
		t.Fatalf("id %q", got["Base ID"])
	}
	if !strings.Contains(got["Base latitude (DMS)"], "N") || !strings.Contains(got["Base latitude (decimal °)"], "40.71280000") {
		t.Fatalf("lat: %q / %q", got["Base latitude (DMS)"], got["Base latitude (decimal °)"])
	}
	if !strings.Contains(got["Base longitude (DMS)"], "W") || !strings.Contains(got["Base longitude (decimal °)"], "-74.00600000") {
		t.Fatalf("lon: %q / %q", got["Base longitude (DMS)"], got["Base longitude (decimal °)"])
	}
	if baseFlags == nil || len(baseFlags.Detail) < 2 {
		t.Fatalf("base flags detail %#v", baseFlags)
	}
	if baseFlags.Detail[0].Label != "Bits 0–2 — Version" || baseFlags.Detail[0].Value != "1" {
		t.Fatalf("version row %#v", baseFlags.Detail[0])
	}
	if baseFlags.Detail[1].Label != "Bit 3 — Base information valid" || baseFlags.Detail[1].Value != "Yes" {
		t.Fatalf("valid row %#v", baseFlags.Detail[1])
	}
}

func TestDecode37BatteryMemory(t *testing.T) {
	payload := make([]byte, 10)
	binary.BigEndian.PutUint16(payload[0:], 88)
	// 3.25 h → 3 h 15 min
	binary.BigEndian.PutUint64(payload[2:], math.Float64bits(3.25))
	fields := Decode(37, payload)
	got := map[string]string{}
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["Summary"] != Lookup(37).Function {
		t.Fatalf("summary %q", got["Summary"])
	}
	if got["Battery capacity (%)"] != "88 %" {
		t.Fatalf("battery %q", got["Battery capacity (%)"])
	}
	if got["Memory left"] != "3 h 15 min" {
		t.Fatalf("memory %q", got["Memory left"])
	}
}

func TestDecodeReservedBitViolationStillShown(t *testing.T) {
	t.Cleanup(func() { ShowExpectedReservedBits = false })
	payload := make([]byte, 17)
	payload[0] = 0x0F // bits 0–3 set; bit 3 is reserved and must be zero → violation
	fields := Decode(10, payload)
	var cf *Field
	for i := range fields {
		if fields[i].Label == "Clock flags" {
			cf = &fields[i]
			break
		}
	}
	if cf == nil || len(cf.Detail) != 4 {
		t.Fatalf("want 3 non-reserved + 1 reserved violation, got %#v", cf)
	}
}

func TestDecodeShowExpectedReservedBits(t *testing.T) {
	t.Cleanup(func() { ShowExpectedReservedBits = false })
	ShowExpectedReservedBits = true

	payload := []byte{
		0x00, 0x00, 0x03, 0xE8,
		0x08, 0xDF,
		0x05,
		0x00, 0x00,
		0x00,
	}
	fields := Decode(1, payload)
	var flags1 *Field
	for i := range fields {
		if fields[i].Label == "Flags 1" {
			flags1 = &fields[i]
			break
		}
	}
	if flags1 == nil || len(flags1.Detail) != 8 {
		t.Fatalf("verbose flags1: %#v", flags1)
	}

	payload8 := make([]byte, 17)
	payload8[0] = 0x05
	fields8 := Decode(8, payload8)
	var vf *Field
	for i := range fields8 {
		if fields8[i].Label == "Velocity flags" {
			vf = &fields8[i]
			break
		}
	}
	if vf == nil || len(vf.Detail) != 8 {
		t.Fatalf("verbose velocity flags: %#v", vf)
	}
}

func TestDecodePositionType38Legacy(t *testing.T) {
	payload := make([]byte, 11)
	binary.BigEndian.PutUint32(payload[0:], math.Float32bits(1.5))
	payload[4] = 0x03 // bits 1,0 set: VRS + RTK fixed
	payload[5] = 0x02 // RTK condition low nibble = 2
	binary.BigEndian.PutUint32(payload[6:], math.Float32bits(12.5))
	payload[10] = 0xC0 // bits 7,6 set; bits 2,1 = 0
	fields := Decode(38, payload)
	got := make(map[string]string)
	var sol *Field
	for i := range fields {
		f := &fields[i]
		got[f.Label] = f.Value
		if f.Label == "Solution flags" {
			sol = f
		}
	}
	if !strings.Contains(got["Summary"], "legacy") {
		t.Fatalf("summary: %q", got["Summary"])
	}
	if got["Error scale"] != "1.5" {
		t.Fatalf("error scale: %q", got["Error scale"])
	}
	if !strings.HasPrefix(got["RTK condition"], "2 —") {
		t.Fatalf("rtk: %q", got["RTK condition"])
	}
	if sol == nil || len(sol.Detail) < 3 {
		t.Fatalf("solution flags detail: %#v", sol)
	}
}

func TestDecodePositionType38OEM(t *testing.T) {
	payload := make([]byte, 26)
	// reserved 0–3 left zero
	payload[4] = 0x03 // solution
	payload[5] = 0x01 // RTK condition 1
	binary.BigEndian.PutUint32(payload[6:], math.Float32bits(3))
	payload[10] = 0x06                                    // net: bits 2,1 = 3
	payload[11] = 0x01                                    // net2 bit 0
	payload[12] = 0x01                                    // frame: ITRF current epoch
	binary.BigEndian.PutUint16(payload[13:], uint16(100)) // +1.00 year → ~2006-01-01
	payload[15] = 7                                       // Australia
	binary.BigEndian.PutUint32(payload[16:], 0xFFFFFFFF)  // RTX minutes expired
	payload[20] = 0x00
	binary.BigEndian.PutUint32(payload[21:], math.Float32bits(0))
	payload[25] = 9 // full fixed RTK
	fields := Decode(38, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Summary"], "Position type") || strings.Contains(got["Summary"], "legacy") {
		t.Fatalf("summary: %q", got["Summary"])
	}
	if !strings.Contains(got["Position fix type"], "9 —") {
		t.Fatalf("fix type: %q", got["Position fix type"])
	}
	if !strings.Contains(got["Tectonic plate"], "Australia") {
		t.Fatalf("plate: %q", got["Tectonic plate"])
	}
	if !strings.Contains(got["RTX STD SUB minutes left"], "0xFFFFFFFF") {
		t.Fatalf("rtx min: %q", got["RTX STD SUB minutes left"])
	}
	if got["Correction age (s)"] != "3.00" {
		t.Fatalf("correction age: %q", got["Correction age (s)"])
	}
	if _, ok := got["Reserved (bytes 0–3)"]; ok {
		t.Fatalf("reserved row should be hidden without ShowExpectedReservedBits")
	}
}

func TestDecodePositionType38FixTypes52and53(t *testing.T) {
	for _, tc := range []struct {
		code byte
		want string
	}{
		{52, "52 — HAS"},
		{53, "53 — INS HAS"},
	} {
		payload := make([]byte, 26)
		payload[4] = 0x03
		payload[5] = 0x01
		binary.BigEndian.PutUint32(payload[6:], math.Float32bits(0))
		payload[10] = 0x06
		payload[11] = 0x01
		payload[12] = 0x01
		binary.BigEndian.PutUint16(payload[13:], 0)
		payload[15] = 0
		binary.BigEndian.PutUint32(payload[16:], 0)
		payload[20] = 0
		binary.BigEndian.PutUint32(payload[21:], math.Float32bits(0))
		payload[25] = tc.code
		fields := Decode(38, payload)
		var fix string
		for _, f := range fields {
			if f.Label == "Position fix type" {
				fix = f.Value
				break
			}
		}
		if fix != tc.want {
			t.Fatalf("code %d: got %q want %q", tc.code, fix, tc.want)
		}
	}
}

func TestDecodePositionType38OEMReservedVerbose(t *testing.T) {
	t.Cleanup(func() { ShowExpectedReservedBits = false })
	ShowExpectedReservedBits = true
	payload := make([]byte, 26)
	payload[4] = 0x00
	binary.BigEndian.PutUint32(payload[6:], math.Float32bits(0))
	payload[10] = 0x00
	payload[11] = 0x00
	payload[12] = 0x01
	binary.BigEndian.PutUint16(payload[13:], 0)
	payload[15] = 0
	binary.BigEndian.PutUint32(payload[16:], 0)
	payload[20] = 0
	binary.BigEndian.PutUint32(payload[21:], math.Float32bits(1.2345))
	payload[25] = 0
	fields := Decode(38, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if _, ok := got["Reserved (bytes 0–3)"]; !ok {
		t.Fatalf("expected reserved row with ShowExpectedReservedBits: %#v", got)
	}
	if got["Pole wobble distance (m)"] != "1.235" {
		t.Fatalf("pole dist: %q", got["Pole wobble distance (m)"])
	}
}

func TestDecodeLBand40(t *testing.T) {
	payload := make([]byte, lBandStatusPayloadBytes)
	copy(payload[0:5], []byte("Custo"))
	binary.BigEndian.PutUint32(payload[5:], math.Float32bits(1531.25))
	binary.BigEndian.PutUint16(payload[9:], 1200)
	binary.BigEndian.PutUint32(payload[11:], math.Float32bits(42.5))
	payload[15] = 1 // HP
	payload[16] = 1
	payload[17] = 0
	payload[18] = 7 // tracking
	payload[19] = 0 // dynamic
	binary.BigEndian.PutUint32(payload[20:], math.Float32bits(0.5))
	binary.BigEndian.PutUint32(payload[24:], math.Float32bits(1.0))
	payload[28] = 0
	binary.BigEndian.PutUint32(payload[29:], math.Float32bits(1.25))
	binary.BigEndian.PutUint32(payload[33:], math.Float32bits(1e-6))
	binary.BigEndian.PutUint32(payload[37:], 100)
	binary.BigEndian.PutUint32(payload[41:], 2)
	binary.BigEndian.PutUint32(payload[45:], 3)
	binary.BigEndian.PutUint32(payload[49:], 0xFFFFFF00-1)
	binary.BigEndian.PutUint32(payload[53:], 10)
	binary.BigEndian.PutUint32(payload[57:], 1)
	payload[61] = 1
	binary.BigEndian.PutUint64(payload[62:], math.Float64bits(1.542e9))

	fields := Decode(40, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["Summary"], "L-Band") {
		t.Fatalf("summary: %q", got["Summary"])
	}
	if !strings.Contains(got["Satellite name"], "Custo") {
		t.Fatalf("name: %q", got["Satellite name"])
	}
	if got["Satellite frequency (MHz)"] != "1531.25" {
		t.Fatalf("MHz: %q", got["Satellite frequency (MHz)"])
	}
	if got["Satellite bit rate (Hz)"] != "1200" {
		t.Fatalf("bitrate: %q", got["Satellite bit rate (Hz)"])
	}
	if !strings.HasPrefix(got["SNR (C/No)"], "42.5") {
		t.Fatalf("snr: %q", got["SNR (C/No)"])
	}
	if !strings.Contains(got["Beam mode"], "7 — Tracking") {
		t.Fatalf("beam: %q", got["Beam mode"])
	}
	if !strings.Contains(got["MEAS frequency (Hz)"], "1.542e+09") && !strings.Contains(got["MEAS frequency (Hz)"], "1542000000") {
		t.Fatalf("meas hz: %q", got["MEAS frequency (Hz)"])
	}
}

func TestDecode41BasePositionQuality(t *testing.T) {
	payload := make([]byte, 31)
	binary.BigEndian.PutUint32(payload[0:], 1234500) // 1234.50 s
	binary.BigEndian.PutUint16(payload[4:], 2300)
	latDeg := 37.7749
	lonDeg := -122.4194
	binary.BigEndian.PutUint64(payload[6:], math.Float64bits(latDeg*math.Pi/180))
	binary.BigEndian.PutUint64(payload[14:], math.Float64bits(lonDeg*math.Pi/180))
	binary.BigEndian.PutUint64(payload[22:], math.Float64bits(12.34))
	payload[30] = 5
	fields := Decode(41, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if !strings.Contains(got["GPS time of week"], "1234.50 s") {
		t.Fatalf("tow: %q", got["GPS time of week"])
	}
	if got["GPS week"] != "2300" {
		t.Fatalf("week: %q", got["GPS week"])
	}
	if !strings.Contains(got["Base latitude (DMS)"], "N") {
		t.Fatalf("lat DMS: %q", got["Base latitude (DMS)"])
	}
	if !strings.Contains(got["Base latitude (decimal °)"], "37.7749") {
		t.Fatalf("lat dec: %q", got["Base latitude (decimal °)"])
	}
	if !strings.Contains(got["Quality"], "5 —") || !strings.Contains(got["Quality"], "Location RTK") {
		t.Fatalf("quality: %q", got["Quality"])
	}
}

func TestDecodeLBand40Short(t *testing.T) {
	fields := Decode(40, []byte{1, 2, 3})
	if len(fields) < 2 {
		t.Fatal(fields)
	}
	if fields[0].Label != "Summary" {
		t.Fatalf("first: %#v", fields[0])
	}
	if !strings.Contains(fields[2].Value, "70") {
		t.Fatalf("parse line: %#v", fields[2])
	}
}

func TestDecodePositionType38OEMExtraFrameByte(t *testing.T) {
	payload := make([]byte, 27)
	payload[4] = 0x00
	payload[5] = 0x00
	binary.BigEndian.PutUint32(payload[6:], 0)
	payload[10] = 0x00
	payload[11] = 0x00
	payload[12] = 0x80 // bit 7: second frame byte follows
	payload[13] = 0x00
	binary.BigEndian.PutUint16(payload[14:], 0)
	payload[16] = 0
	binary.BigEndian.PutUint32(payload[17:], 0)
	payload[21] = 0
	binary.BigEndian.PutUint32(payload[22:], 0)
	payload[26] = 0
	fields := Decode(38, payload)
	if len(fields) < 5 {
		t.Fatalf("fields: %#v", fields)
	}
	var frame *Field
	for i := range fields {
		if fields[i].Label == "Frame flag" {
			frame = &fields[i]
			break
		}
	}
	if frame == nil || !strings.Contains(frame.Value, "+") {
		t.Fatalf("frame value: %#v", frame)
	}
}
