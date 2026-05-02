package gsof

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseLatLonDeg(t *testing.T) {
	payload := make([]byte, 24)
	binary.BigEndian.PutUint64(payload[0:], math.Float64bits(math.Pi/2))
	binary.BigEndian.PutUint64(payload[8:], math.Float64bits(-math.Pi/6))
	lat, lon, ok := ParseLatLonDeg(payload)
	if !ok {
		t.Fatal("expected ok")
	}
	if math.Abs(lat-90) > 1e-9 {
		t.Fatalf("lat deg: %v", lat)
	}
	if math.Abs(lon+30) > 1e-9 {
		t.Fatalf("lon deg: %v", lon)
	}
}
