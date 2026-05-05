package gsofbaseline

import (
	"math"
	"testing"
)

func TestHaversineM_equatorOneDegree(t *testing.T) {
	// ~111.32 km per degree at equator (great circle)
	d := HaversineM(0, 0, 0, 1)
	if d < 111_000 || d > 112_000 {
		t.Fatalf("unexpected distance: %v", d)
	}
}

func TestInitialBearingDeg_cardinal(t *testing.T) {
	// North from (0,0) to (1,0)
	b := InitialBearingDeg(0, 0, 1, 0)
	if math.Abs(b-0) > 1e-6 {
		t.Fatalf("want ~0, got %v", b)
	}
	// East along equator
	b2 := InitialBearingDeg(0, 0, 0, 1)
	if math.Abs(b2-90) > 0.01 {
		t.Fatalf("want ~90, got %v", b2)
	}
}

func TestTowAbsDiffSeconds_wrap(t *testing.T) {
	d := TowAbsDiffSeconds(604800-1, 1)
	if d > 2 {
		t.Fatalf("want small wrap distance, got %v", d)
	}
}
