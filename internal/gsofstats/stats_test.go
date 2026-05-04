package gsofstats

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"
)

func TestTowDeltaSeconds_nominal(t *testing.T) {
	d, ok := towDeltaSeconds(100.0, 100.1)
	if !ok || math.Abs(d-0.1) > 1e-9 {
		t.Fatalf("want 0.1 got %v ok=%v", d, ok)
	}
}

func TestTowDeltaSeconds_weekWrap(t *testing.T) {
	d, ok := towDeltaSeconds(604799.5, 0.2)
	if !ok || math.Abs(d-0.7) > 1e-3 {
		t.Fatalf("want ~0.7 got %v ok=%v", d, ok)
	}
}

func TestTowDeltaSeconds_rejectNonPositiveOrHuge(t *testing.T) {
	if _, ok := towDeltaSeconds(5.0, 5.0); ok {
		t.Fatal("expected false for zero advance")
	}
	if _, ok := towDeltaSeconds(0, 50); ok {
		t.Fatal("expected false for gap > maxTowDeltaForEpoch")
	}
}

func TestStats_UpdateAndDashboard(t *testing.T) {
	s := NewStats(false)
	// One GSOF record: type 1, len 0 (no payload bytes)
	s.Update(1, []byte{0x01, 0x00}, false, false)
	d := s.BuildDashboard("udp", 2101, "test", "")
	if len(d.Records) != 1 {
		t.Fatalf("records %d", len(d.Records))
	}
	if d.Records[0].Type != 1 || d.Records[0].Count != 1 {
		t.Fatalf("row %+v", d.Records[0])
	}
	if d.Records[0].PayloadHex != "01 00" {
		t.Fatalf("payload_hex should be full sub-record type+len+body, got %q", d.Records[0].PayloadHex)
	}
	s2 := NewStats(false)
	// GSOF sub-record: type 1, 3-byte body 0xAA 0xBB 0xCC
	s2.Update(1, []byte{0x01, 0x03, 0xAA, 0xBB, 0xCC}, false, false)
	d2 := s2.BuildDashboard("udp", 2101, "test", "")
	if len(d2.Records) != 1 || d2.Records[0].PayloadHex != "01 03 AA BB CC" {
		t.Fatalf("payload_hex %q", d2.Records[0].PayloadHex)
	}
}

func TestStats_TCPNoSequenceGapWarning(t *testing.T) {
	s := NewStats(false)
	buf := []byte{0x01, 0x00}
	s.Update(1, buf, true, false)
	s.Update(10, buf, true, false)
	d := s.BuildDashboard("tcp", 2101, "", "")
	for _, w := range d.Warnings {
		if strings.Contains(w, "Sequence Gap") {
			t.Fatalf("TCP without ignore-tcp-gsof flag should not emit sequence-gap warnings; got %q", w)
		}
	}
}

func TestStats_TCPIgnoreGSOFGap1SuppressesSingleStep(t *testing.T) {
	s := NewStats(false)
	buf := []byte{0x01, 0x00}
	s.Update(1, buf, true, true)
	s.Update(3, buf, true, true)
	d := s.BuildDashboard("tcp", 2101, "", "")
	for _, w := range d.Warnings {
		if strings.Contains(w, "Sequence Gap") {
			t.Fatalf("expected single-step gap suppressed on TCP with ignore flag; got %q", w)
		}
	}
}

func TestStats_TCPIgnoreGSOFGap1StillWarnsMultiStep(t *testing.T) {
	s := NewStats(false)
	buf := []byte{0x01, 0x00}
	s.Update(1, buf, true, true)
	s.Update(4, buf, true, true)
	d := s.BuildDashboard("tcp", 2101, "", "")
	found := false
	for _, w := range d.Warnings {
		if strings.Contains(w, "Sequence Gap") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected sequence-gap warning for gap > 1 when ignore-tcp-gsof-transmission-gap1 is set")
	}
}

func TestStats_RateNotInflatedByMicroBurstWallClock(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := NewStats(false)
	buf := []byte{0x01, 0x0A, 0x00, 0x00, 0x13, 0x88, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00}
	s.Update(1, buf, false, false)
	time.Sleep(2 * time.Millisecond)
	s.Update(2, buf, false, false)
	time.Sleep(120 * time.Millisecond)
	s.Update(3, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 1 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("missing type 1 row")
	}
	if strings.HasPrefix(row.Rate, "50") {
		t.Fatalf("rate should not snap to 50 Hz after sub-5 ms burst; got %q", row.Rate)
	}
}

func TestStats_RateUsesSeqAdvanceAfterUDPLoss(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := NewStats(false)
	buf := []byte{0x01, 0x0A, 0x00, 0x00, 0x13, 0x88, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00}
	s.Update(1, buf, false, false)
	time.Sleep(520 * time.Millisecond)
	s.Update(6, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 1 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("missing type 1 row")
	}
	if row.Rate != "10 Hz" {
		t.Fatalf("with seq advance 5 and ~520 ms wall, want 10 Hz (missed packets in rate); got %q", row.Rate)
	}
}

func TestSnapToNearestRate_prefersShorterPeriodOnTie(t *testing.T) {
	hz, label := snapToNearestRate(0.15)
	if hz != 10 || label != "10 Hz" {
		t.Fatalf("0.15 s equidistant from 0.1 and 0.2 s buckets: want 10 Hz; got %v %s", hz, label)
	}
}

func TestStats_RateEMASmoothsSingleLongGap(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := NewStats(false)
	buf := []byte{0x01, 0x0A, 0x00, 0x00, 0x13, 0x88, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00}
	s.Update(1, buf, false, false)
	time.Sleep(105 * time.Millisecond)
	s.Update(2, buf, false, false)
	time.Sleep(105 * time.Millisecond)
	s.Update(3, buf, false, false)
	time.Sleep(215 * time.Millisecond)
	s.Update(4, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 1 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("missing type 1 row")
	}
	if strings.HasPrefix(row.Rate, "5") && strings.Contains(row.Rate, "Hz") {
		t.Fatalf("EMA should keep ~10 Hz after one ~2× gap; got %q", row.Rate)
	}
}

func TestStats_Type34AllSVDetailedJSON(t *testing.T) {
	s := NewStats(false)
	// Type 1 (TOW 5 s) then type 34: count=1, PRN=6, GPS, flags 0x0A/0x0B, elev=10, az=270, SNR bytes 4,8,12 → 1,2,3
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x22, 0x0B,
		0x01, 0x06, 0x00, 0x0a, 0x0b, 10, 0x01, 0x0e, 4, 8, 12,
	}
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 34 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 34 row")
	}
	if len(row.AllSVDetailed) != 1 {
		t.Fatalf("all_sv_detailed len %d", len(row.AllSVDetailed))
	}
	e := row.AllSVDetailed[0]
	if e.System != 0 || e.PRN != 6 || e.Elev != 10 || e.Azimuth != 270 {
		t.Fatalf("entry %+v", e)
	}
	if e.Flags1 != 0x0a || e.Flags2 != 0x0b {
		t.Fatalf("flags %+v", e)
	}
	if e.SNRL1 != 1 || e.SNRL2 != 2 || e.SNRL5 != 3 {
		t.Fatalf("snr %+v", e)
	}
}

func TestStats_Type48AllSVDetailedJSON(t *testing.T) {
	s := NewStats(false)
	// Type 1 TOW 5 s, type 48: version 1, page 1 of 2 (0x12), count=1, same SV row as type-34 test.
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x30, 0x0D,
		0x01, 0x12, 0x01,
		0x06, 0x00, 0x0a, 0x0b, 10, 0x01, 0x0e, 4, 8, 12,
	}
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 48 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 48 row")
	}
	if row.AllSV48Page == nil || row.AllSV48Page.Version != 1 || row.AllSV48Page.PageCurrent != 1 || row.AllSV48Page.PageTotal != 2 {
		t.Fatalf("page meta %+v", row.AllSV48Page)
	}
	if len(row.AllSVDetailed) != 1 {
		t.Fatalf("all_sv_detailed len %d", len(row.AllSVDetailed))
	}
	e := row.AllSVDetailed[0]
	if e.System != 0 || e.PRN != 6 || e.Elev != 10 || e.Azimuth != 270 {
		t.Fatalf("entry %+v", e)
	}
}

func TestStats_Type33AllSVBriefJSON(t *testing.T) {
	s := NewStats(false)
	// Type 33: count=1, PRN=4, system=0 (GPS), flags1=0x0F, flags2=0x30
	s.Update(1, []byte{0x21, 0x05, 0x01, 0x04, 0x00, 0x0F, 0x30}, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
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
	s.Update(1, []byte{0x0D, 0x04, 0x01, 0x05, 0x0F, 0x30}, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
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

func f32be(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
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
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
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

func TestStats_PositionTimeHistoryFromType1(t *testing.T) {
	s := NewStats(false)
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x09, 0x0A, 0x1B, 0x03,
	}
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 1 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 1 row")
	}
	if len(row.PositionTimeHistory) != 1 {
		t.Fatalf("position_time_history len %d", len(row.PositionTimeHistory))
	}
	p := row.PositionTimeHistory[0]
	if p.GPSTOWSec != 5 || p.SVsUsed != 9 || p.Flags1 != 0x0A || p.Flags2 != 0x1B || p.Axis1 != 3 {
		t.Fatalf("point %+v", p)
	}
}

func TestStats_SecondAntenna97HistoryFromType1And97(t *testing.T) {
	s := NewStats(false)
	// Type 1 TOW 5 s, type 97: week 1, TOW 0, pos 0, source 1, lat=0, lon=0, h=10, sigmas 3,4,5 → σ_H=5
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x61, 0x2C,
		0x00, 0x01, // GPS week 1 (BE)
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, // position type, source
	}
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(10)...)
	buf = append(buf, f32be(3)...)
	buf = append(buf, f32be(4)...)
	buf = append(buf, f32be(5)...)
	if want := 12 + 2 + 44; len(buf) != want {
		t.Fatalf("packet len %d want %d", len(buf), want)
	}
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row97 *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 97 {
			row97 = &d.Records[i]
			break
		}
	}
	if row97 == nil || len(row97.SecondAntenna97History) != 1 {
		t.Fatalf("row97 history: %+v", row97)
	}
	p := row97.SecondAntenna97History[0]
	if p.GPSTOWSec != 5 || p.HeightM != 10 || math.Abs(p.SigmaHorizontalM-5) > 1e-6 {
		t.Fatalf("point %+v", p)
	}
}

func TestStats_SecondAntenna102HistoryFromType1And102(t *testing.T) {
	s := NewStats(false)
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x66, 0x21,
		0x00,
	}
	buf = append(buf, f64be(10911.505)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(427.30256)...)
	if want := 12 + 2 + 33; len(buf) != want {
		t.Fatalf("packet len %d want %d", len(buf), want)
	}
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row102 *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 102 {
			row102 = &d.Records[i]
			break
		}
	}
	if row102 == nil || len(row102.SecondAntenna102History) != 1 {
		t.Fatalf("row102 history: %+v", row102)
	}
	p := row102.SecondAntenna102History[0]
	if p.GPSTOWSec != 5 || math.Abs(p.HeadingGeodeticDeg-10911.505) > 1e-6 || math.Abs(p.MagneticVariationDeg-427.30256) > 1e-6 {
		t.Fatalf("point %+v", p)
	}
}

func TestStats_Type99InvalidExtendedEmits243FullWireHex(t *testing.T) {
	s := NewStats(false)
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	pl := []byte{0x00, 0x05, 0x00} // extended type 5 (<100)
	buf = append(buf, 0x63, byte(len(pl)))
	buf = append(buf, pl...)
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row243 *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 243 {
			row243 = &d.Records[i]
			break
		}
	}
	if row243 == nil || row243.Count != 1 {
		t.Fatalf("243 row %+v", row243)
	}
	if !strings.HasPrefix(row243.PayloadHex, "63 ") {
		t.Fatalf("payload_hex should include type 99: %q", row243.PayloadHex)
	}
}

func TestStats_Type99ExpandedTo100No99Row(t *testing.T) {
	s := NewStats(false)
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	pl := make([]byte, 35)
	binary.BigEndian.PutUint16(pl[0:2], 100)
	pl[2] = 32
	copy(pl[3:11], []byte("LOCAL123"))
	binary.BigEndian.PutUint64(pl[11:19], math.Float64bits(math.Pi/2))
	binary.BigEndian.PutUint64(pl[19:27], math.Float64bits(0))
	binary.BigEndian.PutUint64(pl[27:35], math.Float64bits(42))
	buf = append(buf, 0x63, byte(len(pl)))
	buf = append(buf, pl...)
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row100 *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 99 {
			t.Fatalf("type 99 should not appear after expansion, got %+v", d.Records[i])
		}
		if d.Records[i].Type == 100 {
			row100 = &d.Records[i]
		}
	}
	if row100 == nil || row100.Count != 1 {
		t.Fatalf("type 100 row: %+v", row100)
	}
	var joined strings.Builder
	for _, f := range row100.Fields {
		joined.WriteString(f.Label)
		joined.WriteString(f.Value)
	}
	if !strings.Contains(joined.String(), "LOCAL123") {
		t.Fatalf("fields %s", joined.String())
	}
	if !strings.HasPrefix(row100.PayloadHex, "63 ") {
		t.Fatalf("type 100 payload_hex should include enclosing type 99: %q", row100.PayloadHex)
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
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
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

func TestStats_LLHMSLHistoryFromType1And70(t *testing.T) {
	s := NewStats(false)
	// Type 1 TOW 5 s, type 70: 24-byte body (lat/lon rad, MSL height m) — same numeric layout as type 2 for history.
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x46, 0x18,
	}
	buf = append(buf, f64be(math.Pi/2)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(100.0)...)
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 70 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 70 row")
	}
	if len(row.LLHHistory) != 1 {
		t.Fatalf("llh_history len %d", len(row.LLHHistory))
	}
	p := row.LLHHistory[0]
	if p.GPSTOWSec != 5.0 || math.Abs(p.LatDeg-90) > 1e-9 || p.LonDeg != 0 || math.Abs(p.HeightM-100) > 1e-9 {
		t.Fatalf("point %+v", p)
	}
}

func TestStats_DOPAndSigmaHistoryFromType1Packet(t *testing.T) {
	s := NewStats(false)
	// Type 1 TOW 5 s, type 9 DOP 1..4, type 12 sigma (E=3 N=4 → σ_H=5)
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x09, 0x10,
	}
	buf = append(buf, f32be(1)...)
	buf = append(buf, f32be(2)...)
	buf = append(buf, f32be(3)...)
	buf = append(buf, f32be(4)...)
	buf = append(buf, 0x0C, 0x26)
	buf = append(buf, f32be(0.1)...)
	buf = append(buf, f32be(3)...)
	buf = append(buf, f32be(4)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0.05)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, 0x00, 0x01)
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row9, row12 *RecordRow
	for i := range d.Records {
		switch d.Records[i].Type {
		case 9:
			row9 = &d.Records[i]
		case 12:
			row12 = &d.Records[i]
		}
	}
	if row9 == nil || len(row9.DOPHistory) != 1 {
		t.Fatalf("dop row/history: %+v", row9)
	}
	d0 := row9.DOPHistory[0]
	if d0.GPSTOWSec != 5 || d0.PDOP != 1 || d0.HDOP != 2 || d0.TDOP != 3 || d0.VDOP != 4 {
		t.Fatalf("dop point %+v", d0)
	}
	if row12 == nil || len(row12.SigmaHistory) != 1 {
		t.Fatalf("sigma row/history: %+v", row12)
	}
	s0 := row12.SigmaHistory[0]
	if s0.GPSTOWSec != 5 || math.Abs(s0.SigmaH-5) > 1e-6 {
		t.Fatalf("sigma point %+v", s0)
	}
	wantH := math.Sqrt(float64(s0.SigmaEast)*float64(s0.SigmaEast) + float64(s0.SigmaNorth)*float64(s0.SigmaNorth))
	if math.Abs(s0.SigmaH-wantH) > 1e-9 {
		t.Fatalf("sigma_h inconsistent %+v", s0)
	}
}

func TestStats_Sigma74HistoryPairedWithType1TOW(t *testing.T) {
	s := NewStats(false)
	// Type 1 TOW 5 s, type 74 second-antenna sigma (same 38-byte layout as type 12: E=3 N=4 → σ_H=5).
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x4A, 0x26,
	}
	buf = append(buf, f32be(0.1)...)
	buf = append(buf, f32be(3)...)
	buf = append(buf, f32be(4)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0.05)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, f32be(0)...)
	buf = append(buf, 0x00, 0x01)
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row74 *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 74 {
			row74 = &d.Records[i]
			break
		}
	}
	if row74 == nil || len(row74.SigmaHistory) != 1 {
		t.Fatalf("sigma74 row/history: %+v", row74)
	}
	p := row74.SigmaHistory[0]
	if p.GPSTOWSec != 5 || math.Abs(p.SigmaH-5) > 1e-6 {
		t.Fatalf("sigma74 point %+v", p)
	}
}

func TestStats_AttitudeHistoryFromType1And27(t *testing.T) {
	s := NewStats(false)
	// Type 1 TOW 5 s, type 27 attitude (this record carries its own TOW: 7000 ms → 7 s).
	buf := []byte{
		0x01, 0x0A,
		0x00, 0x00, 0x13, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x1B, 0x46,
	}
	var towMs [4]byte
	binary.BigEndian.PutUint32(towMs[:], 7000)
	buf = append(buf, towMs[:]...)
	buf = append(buf, 0, 0, 0, 0)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(0)...)
	buf = append(buf, f64be(2.5)...)
	buf = append(buf, 0, 0)
	for i := 0; i < 7; i++ {
		buf = append(buf, f32be(0)...)
	}
	if len(buf) != 84 { // type-1 (12) + type-0x1B sub-record (2+70)
		t.Fatalf("packet len %d", len(buf))
	}
	s.Update(1, buf, false, false)
	d := s.BuildDashboard("udp", 2101, "", "")
	var row *RecordRow
	for i := range d.Records {
		if d.Records[i].Type == 27 {
			row = &d.Records[i]
			break
		}
	}
	if row == nil {
		t.Fatal("no type 27 row")
	}
	if len(row.AttitudeHistory) != 1 {
		t.Fatalf("attitude_history len %d", len(row.AttitudeHistory))
	}
	p := row.AttitudeHistory[0]
	if p.GPSTOWSec != 7 || math.Abs(p.RangeM-2.5) > 1e-12 {
		t.Fatalf("point %+v", p)
	}
}
