package gsof

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseReceivedBaseInfo_roundtripLayout(t *testing.T) {
	// flags=0x0F (valid+version), name "BASE1234", id=42, lat=0, lon=0, h=100
	raw := make([]byte, 35)
	raw[0] = 0x0F
	copy(raw[1:9], []byte("BASE1234"))
	binary.BigEndian.PutUint16(raw[9:11], 42)
	// lat, lon = 0 at 11..26; height 100 at 27..34
	binary.BigEndian.PutUint64(raw[27:35], math.Float64bits(100))
	info, ok := ParseReceivedBaseInfo(raw)
	if !ok {
		t.Fatal("parse failed")
	}
	if !info.InfoValid {
		t.Fatal("expected valid bit")
	}
	if info.Name != "BASE1234" {
		t.Fatalf("name %q", info.Name)
	}
	if info.BaseID != 42 {
		t.Fatalf("id %d", info.BaseID)
	}
	if info.HeightM < 99 || info.HeightM > 101 {
		t.Fatalf("height %v", info.HeightM)
	}
}

func TestParseBasePositionQualityInfo_minimal(t *testing.T) {
	raw := make([]byte, 31)
	binary.BigEndian.PutUint32(raw[0:4], 1000)
	binary.BigEndian.PutUint16(raw[4:6], 2092)
	raw[30] = 0
	_, ok := ParseBasePositionQualityInfo(raw)
	if !ok {
		t.Fatal("parse failed")
	}
}
