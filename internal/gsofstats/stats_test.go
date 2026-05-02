package gsofstats

import "testing"

func TestStats_UpdateAndDashboard(t *testing.T) {
	s := NewStats(false)
	// One GSOF record: type 1, len 0 (no payload bytes)
	s.Update(1, []byte{0x01, 0x00})
	d := s.BuildDashboard("udp", 2101, "test")
	if len(d.Records) != 1 {
		t.Fatalf("records %d", len(d.Records))
	}
	if d.Records[0].Type != 1 || d.Records[0].Count != 1 {
		t.Fatalf("row %+v", d.Records[0])
	}
}
