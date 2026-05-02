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
	binary.BigEndian.PutUint32(payload[1:], math.Float32bits(1))   // horizontal m/s
	binary.BigEndian.PutUint32(payload[5:], math.Float32bits(90))  // heading
	binary.BigEndian.PutUint32(payload[9:], math.Float32bits(2))   // vertical m/s
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
	if got["Velocity"] != "1 m/s" || got["Velocity (km/h)"] != "3.6 km/h" {
		t.Fatalf("horizontal speed: %#v", got)
	}
	if got["Vertical velocity"] != "2 m/s" || got["Vertical velocity (km/h)"] != "7.2 km/h" {
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
