package gsofbaseline

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"timing-analyzer/internal/gsof"
)

func TestFormatHeading38Card_subset(t *testing.T) {
	payload := make([]byte, 26)
	payload[4] = 0x03
	payload[5] = 0x01
	binary.BigEndian.PutUint32(payload[6:], math.Float32bits(3))
	payload[10] = 0x06
	payload[11] = 0x01
	payload[12] = 0x01
	binary.BigEndian.PutUint16(payload[13:], 0)
	payload[15] = 0
	binary.BigEndian.PutUint32(payload[16:], 0)
	payload[20] = 0
	binary.BigEndian.PutUint32(payload[21:], math.Float32bits(0))
	payload[25] = 9
	fields := gsof.Decode(38, payload)
	got := FormatHeading38Card(fields)
	if !strings.Contains(got, "Position type: 9 —") {
		t.Fatalf("want position type line, got %q", got)
	}
	if !strings.Contains(got, "RTK condition:") {
		t.Fatalf("want RTK condition, got %q", got)
	}
	if !strings.Contains(got, "Correction age (s): 3.00") {
		t.Fatalf("want correction age, got %q", got)
	}
	if strings.Contains(got, "Tectonic plate") || strings.Contains(got, "Network flags") {
		t.Fatalf("unexpected extra fields in card text: %q", got)
	}
}
