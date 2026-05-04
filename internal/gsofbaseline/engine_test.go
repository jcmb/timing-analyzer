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

func TestEngine_matchByTOW(t *testing.T) {
	eng := NewEngine(EngineConfig{MatchMaxTowDeltaSec: 0.5})
	// B: TOW 5 s, at origin
	p1 := make([]byte, 10)
	binary.BigEndian.PutUint32(p1[0:4], 5000) // 5 s
	p1[6] = 7
	var bufB []byte
	bufB = append(bufB, gsofRec(1, p1)...)
	lat0 := make([]byte, 8)
	lon0 := make([]byte, 8)
	h0 := make([]byte, 8)
	binary.BigEndian.PutUint64(h0, math.Float64bits(10.0))
	llh := append(append(append([]byte{}, lat0...), lon0...), h0...)
	bufB = append(bufB, gsofRec(2, llh)...)
	eng.IngestB(bufB)

	// A: same TOW, offset lon ~0.001° (~111 m)
	p1a := make([]byte, 10)
	binary.BigEndian.PutUint32(p1a[0:4], 5000)
	p1a[6] = 8
	lonOff := make([]byte, 8)
	binary.BigEndian.PutUint64(lonOff, math.Float64bits(0.001*math.Pi/180))
	llhA := append(append(append([]byte{}, lat0...), lonOff...), h0...)
	var bufA []byte
	bufA = append(bufA, gsofRec(1, p1a)...)
	bufA = append(bufA, gsofRec(2, llhA)...)
	eng.IngestA(bufA)

	s := eng.Snapshot("t")
	if len(s.Points) != 1 {
		t.Fatalf("points %d", len(s.Points))
	}
	if s.Points[0].SVsStream1 != 8 || s.Points[0].SVsStream2 != 7 {
		t.Fatalf("sv %+v", s.Points[0])
	}
	if s.Points[0].HorizM < 100 || s.Points[0].HorizM > 120 {
		t.Fatalf("horiz %v", s.Points[0].HorizM)
	}
}
