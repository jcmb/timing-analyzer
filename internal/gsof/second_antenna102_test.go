package gsof

import (
	"math"
	"testing"
)

func TestParseHeading102Point(t *testing.T) {
	pl := []byte{0x0F}
	pl = append(pl, f64beSlice(10911.505)...)
	pl = append(pl, f64beSlice(0)...)
	pl = append(pl, f64beSlice(0)...)
	pl = append(pl, f64beSlice(427.30256)...)
	pt, ok := ParseHeading102Point(pl)
	if !ok {
		t.Fatal("expected ok")
	}
	pt.GPSTOWSec = 123.4
	if pt.GPSTOWSec != 123.4 {
		t.Fatalf("tow %v", pt.GPSTOWSec)
	}
	if math.Abs(pt.HeadingGeodeticDeg-10911.505) > 1e-9 {
		t.Fatalf("geo %v", pt.HeadingGeodeticDeg)
	}
	if pt.HeadingDatumDeg != 0 || pt.HeadingGridDeg != 0 {
		t.Fatalf("datum/grid %+v", pt)
	}
	if math.Abs(pt.MagneticVariationDeg-427.30256) > 1e-9 {
		t.Fatalf("mag %v", pt.MagneticVariationDeg)
	}
}
