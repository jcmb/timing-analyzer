package gsof

import (
	"math"
	"testing"
)

func TestJSONFloat(t *testing.T) {
	if g := JSONFloat(1.5); g != 1.5 {
		t.Fatalf("JSONFloat(1.5) = %v", g)
	}
	if g := JSONFloat(math.NaN()); g != 0 {
		t.Fatalf("JSONFloat(NaN) = %v want 0", g)
	}
	if g := JSONFloat(math.Inf(1)); g != 0 {
		t.Fatalf("JSONFloat(+Inf) = %v want 0", g)
	}
	if g := JSONFloat(math.Inf(-1)); g != 0 {
		t.Fatalf("JSONFloat(-Inf) = %v want 0", g)
	}
}
