package gsofbaseline

import (
	"math"
	"testing"
)

func TestAngleDiffDegSigned(t *testing.T) {
	if d := AngleDiffDegSigned(10, 350); math.Abs(d-20) > 1e-9 {
		t.Fatalf("10 vs 350 want +20, got %v", d)
	}
	if d := AngleDiffDegSigned(90, 90); d != 0 {
		t.Fatalf("got %v", d)
	}
}
