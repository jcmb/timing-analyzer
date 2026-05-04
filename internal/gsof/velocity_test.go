package gsof

import (
	"math"
	"testing"
)

func TestParseVelocityGraphPoint(t *testing.T) {
	// flags, vel=1, heading=90, vvel=2, local=180
	payload := []byte{
		0x05,
		0x3f, 0x80, 0x00, 0x00,
		0x42, 0xb4, 0x00, 0x00,
		0x40, 0x00, 0x00, 0x00,
		0x43, 0x34, 0x00, 0x00,
	}
	p, ok := ParseVelocityGraphPoint(payload)
	if !ok {
		t.Fatal("expected ok")
	}
	if p.VelocityMPS != 1 || p.VerticalVelocityMPS != 2 || p.HeadingDeg != 90 {
		t.Fatalf("values %+v", p)
	}
	if p.LocalHeadingDeg == nil || math.Abs(*p.LocalHeadingDeg-180) > 1e-5 {
		t.Fatalf("local heading %v", p.LocalHeadingDeg)
	}

	short := payload[:13]
	p2, ok2 := ParseVelocityGraphPoint(short)
	if !ok2 || p2.LocalHeadingDeg != nil {
		t.Fatalf("short payload: ok=%v local=%v", ok2, p2.LocalHeadingDeg)
	}
}
