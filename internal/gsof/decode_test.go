package gsof

import "testing"

func TestDecode01(t *testing.T) {
	payload := make([]byte, 8)
	// TOW 1000 ms, week 2271 LE
	payload[0] = 0xE8
	payload[1] = 0x03
	payload[2] = 0x00
	payload[3] = 0x00
	payload[4] = 0xDF
	payload[5] = 0x08
	payload[6] = 0x00
	payload[7] = 0x00
	fields := Decode(1, payload)
	if len(fields) < 3 {
		t.Fatalf("fields %#v", fields)
	}
}

func TestCatalogOverviewURL(t *testing.T) {
	if Lookup(99).DocURL() != OverviewURL {
		t.Fatal("unknown should link to overview")
	}
}
