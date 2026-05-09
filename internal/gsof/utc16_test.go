package gsof

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseCurrentUTCEpochSec(t *testing.T) {
	// 9-byte type-16 prefix: TOW ms + week (BE); remaining bytes ignored by parser.
	payload := make([]byte, 9)
	binary.BigEndian.PutUint32(payload[0:4], 1000) // 1 s into week
	binary.BigEndian.PutUint16(payload[4:6], 2000) // week
	sec, ok := ParseCurrentUTCEpochSec(payload)
	if !ok {
		t.Fatal("expected ok")
	}
	want := float64(2000)*604800.0 + 1.0
	if math.Abs(sec-want) > 1e-9 {
		t.Fatalf("got %v want %v", sec, want)
	}
}
