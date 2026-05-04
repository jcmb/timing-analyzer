package gsof

import "testing"

func TestParsePositionTimeGraphPoint(t *testing.T) {
	// 10-byte layout: TOW ms = 5000, week 0, SV=7, F1=0x0A, F2=0x1B, init=0x03
	p := []byte{
		0x00, 0x00, 0x13, 0x88,
		0x00, 0x00,
		0x07, 0x0A, 0x1B, 0x03,
	}
	pt, ok := ParsePositionTimeGraphPoint(p)
	if !ok {
		t.Fatal("expected ok")
	}
	if pt.GPSTOWSec != 5 {
		t.Fatalf("tow %v", pt.GPSTOWSec)
	}
	if pt.SVsUsed != 7 || pt.Flags1 != 0x0A || pt.Flags2 != 0x1B || pt.Axis1 != 0x03 {
		t.Fatalf("fields %+v", pt)
	}
}
