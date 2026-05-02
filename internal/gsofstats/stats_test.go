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

func TestStats_Type13SVBriefJSON(t *testing.T) {
	s := NewStats(false)
	// Type 13: count=1, PRN=5, flags1=0x0F, flags2=0x30
	s.Update(1, []byte{0x0D, 0x04, 0x01, 0x05, 0x0F, 0x30})
	d := s.BuildDashboard("udp", 2101, "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 13 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 13 row")
	}
	if len(row.SVBrief) != 1 {
		t.Fatalf("sv_brief len %d", len(row.SVBrief))
	}
	if row.SVBrief[0].PRN != 5 || row.SVBrief[0].Flags1 != 0x0F || row.SVBrief[0].Flags2 != 0x30 {
		t.Fatalf("entry %+v", row.SVBrief[0])
	}
}
