package gsof

import (
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
	var flags1 *Field
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
		}
	}
	if tow != "1.000000 s" || week != "2271" || svs != "5" {
		t.Fatalf("decode mismatch tow=%q week=%q svs=%q\n%s", tow, week, svs, fieldText(fields))
	}
	if flags1 == nil || len(flags1.Detail) != 8 {
		t.Fatalf("Flags 1 detail: %#v", flags1)
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
