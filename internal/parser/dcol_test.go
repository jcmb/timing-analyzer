package parser

import (
	"testing"
	"time"

	"timing-analyzer/internal/core"
)

func TestHexBytesSpaced(t *testing.T) {
	if got := hexBytesSpaced(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
	if got := hexBytesSpaced([]byte{}); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := hexBytesSpaced([]byte{0x1A, 0x2B, 0xFF}); got != "1A 2B FF" {
		t.Fatalf("got %q", got)
	}
}

// TestDCOL_RetainsBytesWithoutSTX verifies TCP-style leading bytes without 0x02 are kept
// until a DCOL frame starting with STX arrives (UDP datagrams rarely need this).
func TestDCOL_RetainsBytesWithoutSTX(t *testing.T) {
	p := &DCOLParser{}
	ch := make(chan core.PacketEvent, 8)
	prefix := []byte{0xFF, 0xFE, 0xFD}
	// Minimal single-page GSOF 0x40 DCOL: type 1 len 0 inside reassembled buffer.
	frame := []byte{0x02, 0x00, 0x40, 0x05, 0x07, 0x00, 0x00, 0x01, 0x00, 0x4D, 0x03}
	p.Process(prefix, time.Time{}, time.Time{}, time.Time{}, "tcp:1", 0, ch)
	if len(p.buf) != len(prefix) {
		t.Fatalf("expected prefix retained in buffer, got len %d want %d", len(p.buf), len(prefix))
	}
	p.Process(frame, time.Time{}, time.Time{}, time.Time{}, "tcp:1", 0, ch)
	select {
	case ev := <-ch:
		if ev.PacketType != 0x40 {
			t.Fatalf("packet type want 0x40 got %#x", ev.PacketType)
		}
		if len(ev.GSOFBuffer) == 0 {
			t.Fatal("empty GSOF buffer")
		}
	default:
		t.Fatal("expected one reassembled GSOF event")
	}
}
