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

func TestParseLLHDegHeight(t *testing.T) {
	payload := make([]byte, 24)
	binary.BigEndian.PutUint64(payload[0:], math.Float64bits(0))
	binary.BigEndian.PutUint64(payload[8:], math.Float64bits(0))
	binary.BigEndian.PutUint64(payload[16:], math.Float64bits(123.456))
	lat, lon, h, ok := ParseLLHDeg(payload)
	if !ok {
		t.Fatal("expected ok")
	}
	if lat != 0 || lon != 0 || math.Abs(h-123.456) > 1e-9 {
		t.Fatalf("got lat=%v lon=%v h=%v", lat, lon, h)
	}
}
