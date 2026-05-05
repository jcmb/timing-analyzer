package gsof

import (
	"encoding/binary"
	"testing"
)

func TestParseSerial15(t *testing.T) {
	p := make([]byte, 4)
	binary.BigEndian.PutUint32(p, 123456789)
	s, ok := ParseSerial15(p)
	if !ok || s != 123456789 {
		t.Fatalf("got %v ok=%v", s, ok)
	}
}
