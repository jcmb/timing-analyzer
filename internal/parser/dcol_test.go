package parser

import (
	"strings"
	"testing"
	"time"

	"timing-analyzer/internal/core"
)

// buildDCOL40 builds a valid DCOL 0x40 frame with GSOF multi-page transport header + payload bytes.
func buildDCOL40(trans, page, max uint8, gsofPayload []byte) []byte {
	pl := append([]byte{trans, page, max}, gsofPayload...)
	n := 6 + len(pl)
	b := make([]byte, n)
	b[0] = 0x02
	b[1] = 0x00
	b[2] = 0x40
	b[3] = byte(len(pl))
	copy(b[4:], pl)
	var csum byte
	for i := 1; i < n-2; i++ {
		csum += b[i]
	}
	b[n-2] = csum
	b[n-1] = 0x03
	return b
}

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
	cfg := core.Config{Verbose: 0, IP: "tcp"}
	prefix := []byte{0xFF, 0xFE, 0xFD}
	// Minimal single-page GSOF 0x40 DCOL: type 1 len 0 inside reassembled buffer.
	frame := []byte{0x02, 0x00, 0x40, 0x05, 0x07, 0x00, 0x00, 0x01, 0x00, 0x4D, 0x03}
	p.Process(prefix, time.Time{}, time.Time{}, time.Time{}, "tcp:1", cfg, ch)
	if len(p.buf) != len(prefix) {
		t.Fatalf("expected prefix retained in buffer, got len %d want %d", len(p.buf), len(prefix))
	}
	p.Process(frame, time.Time{}, time.Time{}, time.Time{}, "tcp:1", cfg, ch)
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

// TestDCOL_UndecodedAfterSyncWarning verifies bytes between frames after the first
// decode produce StreamWarnings (dashboard path merges these via AddWarning).
func TestDCOL_UndecodedAfterSyncWarning(t *testing.T) {
	p := &DCOLParser{}
	ch := make(chan core.PacketEvent, 8)
	cfg := core.Config{Verbose: 0, IP: "tcp"}
	frame1 := []byte{0x02, 0x00, 0x40, 0x05, 0x07, 0x00, 0x00, 0x01, 0x00, 0x4D, 0x03}
	frame2 := []byte{0x02, 0x00, 0x40, 0x05, 0x08, 0x00, 0x00, 0x01, 0x00, 0x4E, 0x03}
	p.Process(frame1, time.Time{}, time.Time{}, time.Time{}, "tcp:1", cfg, ch)
	<-ch
	p.Process(append([]byte{0xAA, 0xBB}, frame2...), time.Time{}, time.Time{}, time.Time{}, "tcp:1", cfg, ch)
	ev := <-ch
	if len(ev.StreamWarnings) != 1 {
		t.Fatalf("want 1 stream warning, got %d: %v", len(ev.StreamWarnings), ev.StreamWarnings)
	}
	if ev.PacketType != 0x40 {
		t.Fatalf("packet type want 0x40 got %#x", ev.PacketType)
	}
}

func TestDCOL_GSOFTransmissionGapWarningUDP(t *testing.T) {
	p := &DCOLParser{}
	ch := make(chan core.PacketEvent, 8)
	cfg := core.Config{Verbose: 0, IP: "udp"}
	f1 := buildDCOL40(1, 0, 0, []byte{0x01, 0x00})
	f3 := buildDCOL40(3, 0, 0, []byte{0x01, 0x00})
	p.Process(f1, time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	<-ch
	p.Process(f3, time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	ev := <-ch
	found := false
	for _, w := range ev.StreamWarnings {
		if strings.Contains(w, "transmission gap") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected transmission gap stream warning, got %v", ev.StreamWarnings)
	}
}

func TestDCOL_GSOFTransmissionGapSuppressedTCPFlag(t *testing.T) {
	p := &DCOLParser{}
	ch := make(chan core.PacketEvent, 8)
	cfg := core.Config{Verbose: 0, IP: "tcp", IgnoreTCPGSOFTransmissionGap1: true}
	f1 := buildDCOL40(1, 0, 0, []byte{0x01, 0x00})
	f3 := buildDCOL40(3, 0, 0, []byte{0x01, 0x00})
	p.Process(f1, time.Time{}, time.Time{}, time.Time{}, "t:1", cfg, ch)
	<-ch
	p.Process(f3, time.Time{}, time.Time{}, time.Time{}, "t:1", cfg, ch)
	ev := <-ch
	for _, w := range ev.StreamWarnings {
		if strings.Contains(w, "transmission gap") {
			t.Fatalf("gap of 1 should be suppressed with ignore flag; got %q", w)
		}
	}
}

func TestDCOL_GSOFMissedPageDropsPartial(t *testing.T) {
	p := &DCOLParser{}
	ch := make(chan core.PacketEvent, 8)
	cfg := core.Config{Verbose: 0, IP: "udp"}
	pl := []byte{0x01, 0x00}
	p.Process(buildDCOL40(9, 0, 2, pl), time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	p.Process(buildDCOL40(9, 2, 2, pl), time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	select {
	case <-ch:
		t.Fatal("did not expect emit after skipped page")
	default:
	}
	if len(p.gsofAssembler) != 0 {
		t.Fatalf("expected assembler cleared, got %d sessions", len(p.gsofAssembler))
	}
}

func TestDCOL_GSOFDuplicatePageDropped(t *testing.T) {
	p := &DCOLParser{}
	ch := make(chan core.PacketEvent, 8)
	cfg := core.Config{Verbose: 0, IP: "udp"}
	pl := []byte{0x01, 0x00}
	f0 := buildDCOL40(4, 0, 1, pl)
	p.Process(f0, time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	p.Process(f0, time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	p.Process(buildDCOL40(4, 1, 1, pl), time.Time{}, time.Time{}, time.Time{}, "u:1", cfg, ch)
	<-ch // first duplicate dropped; complete after page 1
}
