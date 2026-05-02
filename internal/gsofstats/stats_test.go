package gsofstats

import (
	"encoding/binary"
	"math"
	"testing"
)

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

func TestStats_Type33AllSVBriefJSON(t *testing.T) {
	s := NewStats(false)
	// Type 33: count=1, system=0 (GPS), PRN=4, flags1=0x0F, flags2=0x30
	s.Update(1, []byte{0x21, 0x05, 0x01, 0x00, 0x04, 0x0F, 0x30})
	d := s.BuildDashboard("udp", 2101, "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 33 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 33 row")
	}
	if len(row.AllSVBrief) != 1 {
		t.Fatalf("all_sv_brief len %d", len(row.AllSVBrief))
	}
	e := row.AllSVBrief[0]
	if e.System != 0 || e.PRN != 4 || e.Flags1 != 0x0F || e.Flags2 != 0x30 || e.SystemName != "GPS" {
		t.Fatalf("entry %+v", e)
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

func f64be(v float64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.Float64bits(v))
	return b
}

func TestStats_TangentHistoryFromType1And7(t *testing.T) {
	s := NewStats(false)
	// Type 1: 10-byte payload (GPS TOW ms = 5000 → 5 s), then type 7: 24 bytes ENU.
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x07, 0x18,
	}
	buf = append(buf, f64be(0.1)...)
	buf = append(buf, f64be(-0.2)...)
	buf = append(buf, f64be(0.3)...)
	s.Update(1, buf)
	d := s.BuildDashboard("udp", 2101, "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 7 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 7 row")
	}
	if len(row.TangentHistory) != 1 {
		t.Fatalf("tangent_history len %d", len(row.TangentHistory))
	}
	p := row.TangentHistory[0]
	if p.GPSTOWSec != 5.0 || p.DEm != 0.1 || p.DNm != -0.2 || p.DUm != 0.3 {
		t.Fatalf("point %+v", p)
	}
}

func TestStats_LLHHistoryFromType1And2(t *testing.T) {
	s := NewStats(false)
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x02, 0x18,
	}
	buf = append(buf, f64be(math.Pi/2)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(100.0)...)
	s.Update(1, buf)
	d := s.BuildDashboard("udp", 2101, "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 2 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 2 row")
	}
	if len(row.LLHHistory) != 1 {
		t.Fatalf("llh_history len %d", len(row.LLHHistory))
	}
	p := row.LLHHistory[0]
	if p.GPSTOWSec != 5.0 || math.Abs(p.LatDeg-90) > 1e-9 || p.LonDeg != 0 || math.Abs(p.HeightM-100) > 1e-9 {
		t.Fatalf("point %+v", p)
	}
}
