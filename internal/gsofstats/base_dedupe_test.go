package gsofstats

import "testing"

func TestPositionsCloseLLH(t *testing.T) {
	if !positionsCloseLLH(10, -50, 100, 10+5e-8, -50+5e-8, 100+5e-4) {
		t.Fatal("expected close")
	}
	if positionsCloseLLH(10, -50, 100, 11, -50, 100) {
		t.Fatal("expected not close (lat delta)")
	}
}
