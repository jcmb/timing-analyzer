package gsof

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParsePositionTimeTOWSec(t *testing.T) {
	p := []byte{0, 0, 0x03, 0xe8} // 1000 ms
	sec, ok := ParsePositionTimeTOWSec(p)
	if !ok || sec != 1.0 {
		t.Fatalf("got %v ok=%v", sec, ok)
	}
	_, ok = ParsePositionTimeTOWSec([]byte{})
	if ok {
		t.Fatal("expected false")
	}
}

func TestParseTangentPlaneENU(t *testing.T) {
	p := make([]byte, 24)
	binary.BigEndian.PutUint64(p[0:8], math.Float64bits(1.25))
	binary.BigEndian.PutUint64(p[8:16], math.Float64bits(-2.5))
	binary.BigEndian.PutUint64(p[16:24], math.Float64bits(3.0))
	de, dn, du, ok := ParseTangentPlaneENU(p)
	if !ok {
		t.Fatal("ok")
	}
	if de != 1.25 || dn != -2.5 || du != 3.0 {
		t.Fatalf("de,dn,du = %v,%v,%v", de, dn, du)
	}
}
