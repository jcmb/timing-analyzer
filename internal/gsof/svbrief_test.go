package gsof

import "testing"

func TestParseAllSVBriefEntriesSortsBySystemThenPRN(t *testing.T) {
	// count=2: GLONASS (2) PRN 10, then GPS (0) PRN 5 — wire order differs from display order.
	payload := []byte{2, 2, 10, 0x11, 0x22, 0, 5, 0x33, 0x44}
	n, rows := ParseAllSVBriefEntries(payload)
	if n != 2 || len(rows) != 2 {
		t.Fatalf("n=%d rows=%d", n, len(rows))
	}
	if rows[0].System != 0 || rows[0].PRN != 5 {
		t.Fatalf("row0 want GPS PRN5 got sys=%d prn=%d", rows[0].System, rows[0].PRN)
	}
	if rows[1].System != 2 || rows[1].PRN != 10 {
		t.Fatalf("row1 want GLO PRN10 got sys=%d prn=%d", rows[1].System, rows[1].PRN)
	}
	if rows[0].SystemName != "GPS" || rows[1].SystemName != "GLONASS" {
		t.Fatalf("names: %q %q", rows[0].SystemName, rows[1].SystemName)
	}
	if rows[0].Flags1 != 0x33 || rows[0].Flags2 != 0x44 {
		t.Fatalf("row0 flags: %02x %02x", rows[0].Flags1, rows[0].Flags2)
	}
}
