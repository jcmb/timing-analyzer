package dcol

import (
	"bytes"
	"testing"
)

func TestFormatHexSpaced(t *testing.T) {
	if got := FormatHexSpaced(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
	if got := FormatHexSpaced([]byte{}); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := FormatHexSpaced([]byte{0x1A, 0x2B, 0xFF}); got != "1A 2B FF" {
		t.Fatalf("got %q", got)
	}
}

func TestFlattenGSOFBufferNo99SameSlice(t *testing.T) {
	src := []byte{1, 0}
	out := FlattenGSOFBuffer(src)
	if !bytes.Equal(out, src) || len(out) != len(src) || &out[0] != &src[0] {
		t.Fatalf("expected same slice back, got %v vs %v", out, src)
	}
}

func TestFlattenGSOFBufferExpands99(t *testing.T) {
	// One extended record: type 101, length 1, body 0x42 inside type 99.
	ext := []byte{0, 101, 1, 0x42}
	outer := []byte{99, byte(len(ext))}
	outer = append(outer, ext...)
	out := FlattenGSOFBuffer(outer)
	want := []byte{101, 1, 0x42}
	if !bytes.Equal(out, want) {
		t.Fatalf("got %v want %v", out, want)
	}
}

func TestFlattenGSOFBufferTwoExtendedInOne99(t *testing.T) {
	var ext []byte
	ext = append(ext, 0, 101, 1, 0x11)
	ext = append(ext, 0, 102, 2, 0x22, 0x33)
	outer := []byte{99, byte(len(ext))}
	outer = append(outer, ext...)
	out := FlattenGSOFBuffer(outer)
	want := []byte{101, 1, 0x11, 102, 2, 0x22, 0x33}
	if !bytes.Equal(out, want) {
		t.Fatalf("got %v want %v", out, want)
	}
}
