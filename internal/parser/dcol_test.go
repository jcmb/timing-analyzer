package parser

import "testing"

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
