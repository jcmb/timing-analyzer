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
	if tow != "1.000000 s" || week != "2271" || svs != "5" {
		t.Fatalf("decode mismatch tow=%q week=%q svs=%q\n%s", tow, week, svs, fieldText(fields))
	}
	if flags1 == nil || len(flags1.Detail) != 8 {
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
	if height != "123.456789" {
		t.Fatalf("height: %q", height)
	}
}
