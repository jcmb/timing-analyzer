package gsofbaseline

import (
	"encoding/binary"
	"math"
	"testing"
)

// gsofRec builds one [type][len][payload] record.
func gsofRec(msgType byte, payload []byte) []byte {
	out := make([]byte, 2+len(payload))
	out[0] = msgType
	out[1] = byte(len(payload))
	copy(out[2:], payload)
	return out
}

func TestEngine_movingBasePairing(t *testing.T) {
	eng := NewEngine(EngineConfig{MatchMaxTowDeltaSec: 0.5, MovingBaseConfigured: true})
	// Moving base: TOW 5 s, at origin
	p1 := make([]byte, 10)
	binary.BigEndian.PutUint32(p1[0:4], 5000)
	p1[6] = 7
	var bufB []byte
	bufB = append(bufB, gsofRec(1, p1)...)
	lat0 := make([]byte, 8)
	lon0 := make([]byte, 8)
	h0 := make([]byte, 8)
	binary.BigEndian.PutUint64(h0, math.Float64bits(10.0))
	llh := append(append(append([]byte{}, lat0...), lon0...), h0...)
	bufB = append(bufB, gsofRec(2, llh)...)
	eng.IngestMovingBase(bufB)

	// Heading: same TOW, offset lon ~0.001° (~111 m)
	p1a := make([]byte, 10)
	binary.BigEndian.PutUint32(p1a[0:4], 5000)
	p1a[6] = 8
	lonOff := make([]byte, 8)
	binary.BigEndian.PutUint64(lonOff, math.Float64bits(0.001*math.Pi/180))
	llhA := append(append(append([]byte{}, lat0...), lonOff...), h0...)
	var bufA []byte
	bufA = append(bufA, gsofRec(1, p1a)...)
	bufA = append(bufA, gsofRec(2, llhA)...)
	eng.IngestHeading(bufA)

	s := eng.Snapshot("t")
	if len(s.Points) != 1 {
		t.Fatalf("points %d", len(s.Points))
	}
	p := s.Points[0]
	if p.ReferenceSource != "moving_base" {
		t.Fatalf("source %q", p.ReferenceSource)
	}
	if p.SVsHeading != 8 || p.SVsMovingBase != 7 {
		t.Fatalf("sv %+v", p)
	}
	if p.HorizM < 100 || p.HorizM > 120 {
		t.Fatalf("horiz %v", p.HorizM)
	}
}

func TestEngine_headingType41Target(t *testing.T) {
	eng := NewEngine(EngineConfig{MatchMaxTowDeltaSec: 0.5, MovingBaseConfigured: false})
	// Type 41: TOW 5 s, base at (0,0) height 0
	pl41 := make([]byte, 31)
	binary.BigEndian.PutUint32(pl41[0:4], 5000)
	binary.BigEndian.PutUint16(pl41[4:6], 2092)
	// lat=lon=0, h=0 already zeroed
	// Order type 1/2 before type 41 in the packet (ring must still match).
	p1 := make([]byte, 10)
	binary.BigEndian.PutUint32(p1[0:4], 5000)
	p1[6] = 3
	lat0 := make([]byte, 8)
	lonOff := make([]byte, 8)
	binary.BigEndian.PutUint64(lonOff, math.Float64bits(0.0005*math.Pi/180))
	hR := make([]byte, 8)
	binary.BigEndian.PutUint64(hR, math.Float64bits(5.0))
	llh := append(append(append([]byte{}, lat0...), lonOff...), hR...)
	var buf []byte
	buf = append(buf, gsofRec(1, p1)...)
	buf = append(buf, gsofRec(2, llh)...)
	buf = append(buf, gsofRec(41, pl41)...)

	eng.IngestHeading(buf)
	s := eng.Snapshot("t")
	if len(s.Points) != 1 {
		t.Fatalf("points %d", len(s.Points))
	}
	if s.Points[0].ReferenceSource != "type41" {
		t.Fatalf("source %q", s.Points[0].ReferenceSource)
	}
	if !s.HasHeadingType41Ring {
		t.Fatal("expected type41 ring")
	}
}
