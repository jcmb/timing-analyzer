package gsof

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseDOPPoint(t *testing.T) {
	payload := make([]byte, 16)
	binary.BigEndian.PutUint32(payload[0:], math.Float32bits(2.5))
	binary.BigEndian.PutUint32(payload[4:], math.Float32bits(1.25))
	binary.BigEndian.PutUint32(payload[8:], math.Float32bits(0.75))
	binary.BigEndian.PutUint32(payload[12:], math.Float32bits(3))
	pt, ok := ParseDOPPoint(payload)
	if !ok {
		t.Fatal("expected ok")
	}
	if pt.PDOP != 2.5 || pt.HDOP != 1.25 || pt.TDOP != 0.75 || pt.VDOP != 3 {
		t.Fatalf("%+v", pt)
	}
}

func TestParseSigmaPointSigmaH(t *testing.T) {
	payload := make([]byte, 38)
	off := 0
	put := func(v float32) {
		binary.BigEndian.PutUint32(payload[off:], math.Float32bits(v))
		off += 4
	}
	put(0.1)  // POSITION_RMS
	put(3)    // SIGMA_EAST
	put(4)    // SIGMA_NORTH
	put(0)    // COVAR
	put(0.05) // SIGMA_UP
	put(0)
	put(0)
	put(0)
	put(0)
	binary.BigEndian.PutUint16(payload[off:], 1)
	pt, ok := ParseSigmaPoint(payload)
	if !ok {
		t.Fatal("expected ok")
	}
	wantH := 5.0
	if math.Abs(pt.SigmaH-wantH) > 1e-9 {
		t.Fatalf("sigma_h want %v got %v", wantH, pt.SigmaH)
	}
	if math.Abs(pt.PositionRMS-0.1) > 1e-6 || math.Abs(pt.SigmaEast-3) > 1e-9 || math.Abs(pt.SigmaNorth-4) > 1e-9 || math.Abs(pt.SigmaUp-0.05) > 1e-6 {
		t.Fatalf("%+v", pt)
	}
}
