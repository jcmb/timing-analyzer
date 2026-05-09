package dcol

import (
	"strings"
	"testing"
)

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

func newTestParser() *Parser {
	reg := NewRegistry()
	RegisterPublic(reg)
	return NewParser(reg)
}

func TestDCOL_RetainsBytesWithoutSTX(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{Verbose: 0, RemoteAddr: "tcp:1", TransportIsUDP: false}
	prefix := []byte{0xFF, 0xFE, 0xFD}
	frame := []byte{0x02, 0x00, 0x40, 0x05, 0x07, 0x00, 0x00, 0x01, 0x00, 0x4D, 0x03}
	p.Process(prefix, env, emit)
	if len(p.buf) != len(prefix) {
		t.Fatalf("expected prefix retained in buffer, got len %d want %d", len(p.buf), len(prefix))
	}
	p.Process(frame, env, emit)
	if len(got) != 1 {
		t.Fatalf("expected one message, got %d", len(got))
	}
	if got[0].PacketType != 0x40 {
		t.Fatalf("packet type want 0x40 got %#x", got[0].PacketType)
	}
	if len(got[0].GSOFBuffer) == 0 {
		t.Fatal("empty GSOF buffer")
	}
}

func TestDCOL_UndecodedAfterSyncWarning(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{Verbose: 0, RemoteAddr: "tcp:1", TransportIsUDP: false}
	frame1 := []byte{0x02, 0x00, 0x40, 0x05, 0x07, 0x00, 0x00, 0x01, 0x00, 0x4D, 0x03}
	frame2 := []byte{0x02, 0x00, 0x40, 0x05, 0x08, 0x00, 0x00, 0x01, 0x00, 0x4E, 0x03}
	p.Process(frame1, env, emit)
	p.Process(append([]byte{0xAA, 0xBB}, frame2...), env, emit)
	if len(got) != 2 {
		t.Fatalf("want 2 messages, got %d", len(got))
	}
	ev := got[1]
	if len(ev.StreamWarnings) != 1 {
		t.Fatalf("want 1 stream warning, got %d: %v", len(ev.StreamWarnings), ev.StreamWarnings)
	}
	if ev.PacketType != 0x40 {
		t.Fatalf("packet type want 0x40 got %#x", ev.PacketType)
	}
}

func TestDCOL_GSOFTransmissionGapWarningUDP(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{Verbose: 0, RemoteAddr: "u:1", TransportIsUDP: true}
	f1 := buildDCOL40(1, 0, 0, []byte{0x01, 0x00})
	f3 := buildDCOL40(3, 0, 0, []byte{0x01, 0x00})
	p.Process(f1, env, emit)
	p.Process(f3, env, emit)
	if len(got) != 2 {
		t.Fatalf("want 2 messages got %d", len(got))
	}
	ev := got[1]
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

func TestDCOL_GSOFTransmissionGapTCPDefaultWarnsSingleSkip(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{Verbose: 0, RemoteAddr: "t:1", TransportIsUDP: false}
	f1 := buildDCOL40(1, 0, 0, []byte{0x01, 0x00})
	f3 := buildDCOL40(3, 0, 0, []byte{0x01, 0x00})
	p.Process(f1, env, emit)
	p.Process(f3, env, emit)
	ev := got[1]
	found := false
	for _, w := range ev.StreamWarnings {
		if strings.Contains(w, "transmission gap") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("TCP without ignore flag should warn for a single skipped transmission id; got %v", ev.StreamWarnings)
	}
}

func TestDCOL_GSOFTransmissionGapSuppressedTCPWithFlag(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{
		Verbose:                       0,
		RemoteAddr:                    "t:1",
		TransportIsUDP:                false,
		IgnoreTCPGSOFTransmissionGap1: true,
	}
	f1 := buildDCOL40(1, 0, 0, []byte{0x01, 0x00})
	f3 := buildDCOL40(3, 0, 0, []byte{0x01, 0x00})
	p.Process(f1, env, emit)
	p.Process(f3, env, emit)
	ev := got[1]
	for _, w := range ev.StreamWarnings {
		if strings.Contains(w, "transmission gap") {
			t.Fatalf("TCP with ignore flag should suppress single skipped id; got %q", w)
		}
	}
}

func TestDCOL_GSOFTransmissionGapTCPWarnsTwoSkippedEvenWithFlag(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{
		Verbose:                       0,
		RemoteAddr:                    "t:1",
		TransportIsUDP:                false,
		IgnoreTCPGSOFTransmissionGap1: true,
	}
	f1 := buildDCOL40(1, 0, 0, []byte{0x01, 0x00})
	f4 := buildDCOL40(4, 0, 0, []byte{0x01, 0x00})
	p.Process(f1, env, emit)
	p.Process(f4, env, emit)
	ev := got[1]
	found := false
	for _, w := range ev.StreamWarnings {
		if strings.Contains(w, "transmission gap") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("TCP should still warn when ≥2 transmission ids are skipped, even with ignore flag; got %v", ev.StreamWarnings)
	}
}

func TestDCOL_GSOFMissedPageDropsPartial(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{Verbose: 0, RemoteAddr: "u:1", TransportIsUDP: true}
	pl := []byte{0x01, 0x00}
	p.Process(buildDCOL40(9, 0, 2, pl), env, emit)
	p.Process(buildDCOL40(9, 2, 2, pl), env, emit)
	if len(got) != 0 {
		t.Fatal("did not expect emit after skipped page")
	}
	if len(p.gsofAssembler) != 0 {
		t.Fatalf("expected assembler cleared, got %d sessions", len(p.gsofAssembler))
	}
}

func TestDCOL_GSOFDuplicatePageDropped(t *testing.T) {
	p := newTestParser()
	var got []Message
	emit := func(m Message) { got = append(got, m) }
	env := Env{Verbose: 0, RemoteAddr: "u:1", TransportIsUDP: true}
	pl := []byte{0x01, 0x00}
	f0 := buildDCOL40(4, 0, 1, pl)
	p.Process(f0, env, emit)
	p.Process(f0, env, emit)
	p.Process(buildDCOL40(4, 1, 1, pl), env, emit)
	if len(got) != 1 {
		t.Fatalf("want one emit after duplicate dropped; got %d", len(got))
	}
}
