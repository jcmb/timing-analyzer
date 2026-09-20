package gsofbaseline

import (
	"encoding/binary"
	"math"
	"testing"
)

func f32be(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func TestWalkGSOFPacket_DOPAndSigma(t *testing.T) {
	var buf []byte
	p1 := make([]byte, 10)
	binary.BigEndian.PutUint32(p1[0:4], 5000)
	buf = append(buf, gsofRec(1, p1)...)
	dop := append(append(append(f32be(1), f32be(2)...), f32be(3)...), f32be(4)...)
	buf = append(buf, gsofRec(9, dop)...)
	sig := append(f32be(0.1), f32be(3)...)
	sig = append(sig, f32be(4)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0.05)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, 0x00, 0x01)
	buf = append(buf, gsofRec(12, sig)...)

	w := WalkGSOFPacket(buf)
	if len(w.DOPPoints) != 1 {
		t.Fatalf("dop points %d", len(w.DOPPoints))
	}
	if w.DOPPoints[0].GPSTOWSec != 5 || w.DOPPoints[0].PDOP != 1 {
		t.Fatalf("dop %+v", w.DOPPoints[0])
	}
	if len(w.SigmaPoints) != 1 {
		t.Fatalf("sigma points %d", len(w.SigmaPoints))
	}
	if w.SigmaPoints[0].GPSTOWSec != 5 || math.Abs(w.SigmaPoints[0].SigmaH-5) > 1e-6 {
		t.Fatalf("sigma %+v", w.SigmaPoints[0])
	}
}

func TestEngine_DOPAndSigmaPerStream(t *testing.T) {
	eng := NewEngine(EngineConfig{MovingBaseConfigured: true})
	p1 := make([]byte, 10)
	binary.BigEndian.PutUint32(p1[0:4], 5000)
	dop := append(append(append(f32be(1.1), f32be(2.2)...), f32be(3.3)...), f32be(4.4)...)
	sig := append(f32be(0.1), f32be(1)...)
	sig = append(sig, f32be(2)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0.05)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, f32be(0)...)
	sig = append(sig, 0x00, 0x01)

	var bufH []byte
	bufH = append(bufH, gsofRec(1, p1)...)
	bufH = append(bufH, gsofRec(9, dop)...)
	bufH = append(bufH, gsofRec(12, sig)...)
	eng.IngestHeading(bufH)

	dop2 := append(append(append(f32be(5), f32be(6)...), f32be(7)...), f32be(8)...)
	var bufM []byte
	bufM = append(bufM, gsofRec(1, p1)...)
	bufM = append(bufM, gsofRec(9, dop2)...)
	eng.IngestMovingBase(bufM)

	s := eng.Snapshot("t")
	if len(s.HeadingDOPHistory) != 1 || math.Abs(s.HeadingDOPHistory[0].PDOP-1.1) > 1e-5 {
		t.Fatalf("heading dop %+v", s.HeadingDOPHistory)
	}
	if len(s.HeadingSigmaHistory) != 1 {
		t.Fatalf("heading sigma %+v", s.HeadingSigmaHistory)
	}
	if len(s.MovingBaseDOPHistory) != 1 || s.MovingBaseDOPHistory[0].PDOP != 5 {
		t.Fatalf("mb dop %+v", s.MovingBaseDOPHistory)
	}
}
