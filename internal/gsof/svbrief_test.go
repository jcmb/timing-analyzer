package gsof

import "testing"

func TestParseSVBriefEntries(t *testing.T) {
	payload := []byte{2, 10, 5, 3, 20, 0xFF, 0x01}
	n, rows := ParseSVBriefEntries(payload)
	if n != 2 || len(rows) != 2 {
		t.Fatalf("n=%d rows=%d", n, len(rows))
	}
	if rows[0].PRN != 10 || rows[0].Flags1 != 5 || rows[0].Flags2 != 3 {
		t.Fatalf("row0 %+v", rows[0])
	}
	if rows[1].PRN != 20 || rows[1].Flags1 != 0xFF || rows[1].Flags2 != 1 {
		t.Fatalf("row1 %+v", rows[1])
	}
}

func TestParseSVBriefEntriesTruncated(t *testing.T) {
	payload := []byte{3, 1, 2, 3}
	n, rows := ParseSVBriefEntries(payload)
	if n != 3 || len(rows) != 1 {
		t.Fatalf("n=%d len(rows)=%d", n, len(rows))
	}
}

func TestDecode13FieldsAndBinary(t *testing.T) {
	payload := []byte{1, 7, 0b10101010, 0b00000011}
	fields := Decode(13, payload)
	got := make(map[string]string)
	for _, f := range fields {
		got[f.Label] = f.Value
	}
	if got["SV count"] != "1" {
		t.Fatalf("count: %#v", got)
	}
	if got["SV 0 PRN"] != "7" {
		t.Fatalf("prn: %#v", got)
	}
	if got["SV 0 Flags 1 (binary)"] != "10101010" {
		t.Fatalf("f1: %q", got["SV 0 Flags 1 (binary)"])
	}
	if got["SV 0 Flags 2 (binary)"] != "00000011" {
		t.Fatalf("f2: %q", got["SV 0 Flags 2 (binary)"])
	}
}

func TestDecode13TruncatedParseLine(t *testing.T) {
	payload := []byte{2, 1, 2, 3}
	fields := Decode(13, payload)
	var sawParse bool
	for _, f := range fields {
		if f.Label == "Parse" {
			sawParse = true
			break
		}
	}
	if !sawParse {
		t.Fatal("expected Parse field for truncated list")
	}
}
