package gsof

import "testing"

func TestParseSVDetailedEntries(t *testing.T) {
	payload := []byte{
		1,
		3, 0x0a, 0x0b, 5,
		0x00, 0x64, 0x08, 0x0c, // azimuth 100 BE, SNR L1=8/4, L2=12/4
	}
	n, rows := ParseSVDetailedEntries(payload)
	if n != 1 || len(rows) != 1 {
		t.Fatalf("n=%d rows=%d", n, len(rows))
	}
	e := rows[0]
	if e.PRN != 3 || e.Flags1 != 0x0a || e.Flags2 != 0x0b || e.Elev != 5 || e.Azimuth != 100 {
		t.Fatalf("entry %+v", e)
	}
	if e.SNRL1 != 2.0 || e.SNRL2 != 3.0 {
		t.Fatalf("snr L1=%v L2=%v", e.SNRL1, e.SNRL2)
	}
}
