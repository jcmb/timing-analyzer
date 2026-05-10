package gsof

import (
	"strings"
	"testing"
)

func TestFormatLatLonHeightEllipsoidalLine(t *testing.T) {
	line := FormatLatLonHeightEllipsoidalLine(39.9398111111, -105.0800736111, 1650.878)
	if !strings.Contains(line, "ellipsoidal") {
		t.Fatalf("missing ellipsoidal: %q", line)
	}
	if !strings.Contains(line, "1650.878") {
		t.Fatalf("missing height: %q", line)
	}
	if !strings.Contains(line, "″") {
		t.Fatalf("missing seconds prime: %q", line)
	}
}
