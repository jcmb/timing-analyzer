package gsof

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func TestParseAttitudePointLayout(t *testing.T) {
	// 70-byte type-27 body: TOW ms, header bytes, 4×f64, u16, 7×f32
	var b []byte
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, 6000) // 6.000 s
	b = append(b, tmp...)
	b = append(b, 0, 0, 0, 0) // flags, nsv, mode, reserved
	b = append(b, f64beSlice(1.0)...)
	b = append(b, f64beSlice(0.0)...)
	b = append(b, f64beSlice(0.0)...)
	b = append(b, f64beSlice(10.0)...)
	b = append(b, 0, 10) // PDOP 1.0
	b = append(b, f32beSlice(0.25)...)
	b = append(b, f32beSlice(0.5)...)
	b = append(b, f32beSlice(0.75)...)
	b = append(b, f32beSlice(0)...)
	b = append(b, f32beSlice(0)...)
	b = append(b, f32beSlice(0)...)
	b = append(b, f32beSlice(2.25)...)
	if len(b) != 70 {
		t.Fatalf("payload len %d", len(b))
	}
	pt, ok := ParseAttitudePoint(b)
	if !ok {
		t.Fatal("ParseAttitudePoint failed")
	}
	if pt.GPSTOWSec != 6 {
		t.Fatalf("tow s %+v", pt)
	}
	wantPitch := 180.0 / math.Pi
	if math.Abs(pt.PitchDeg-wantPitch) > 1e-12 || pt.YawDeg != 0 || pt.RollDeg != 0 {
		t.Fatalf("angles %+v", pt)
	}
	if math.Abs(pt.RangeM-10) > 1e-12 {
		t.Fatalf("range %+v", pt)
	}
	if pt.PitchVarRad2 != 0.25 || pt.YawVarRad2 != 0.5 || pt.RollVarRad2 != 0.75 {
		t.Fatalf("var %+v", pt)
	}
	if math.Abs(pt.RangeVarM2-2.25) > 1e-6 {
		t.Fatalf("range var m2 %+v", pt)
	}
}

func TestDecode27DecimalDegreeLabels(t *testing.T) {
	fields := Decode(27, buildMinimalAttitudePayload())
	var pitchDec, yawDec, rollDec string
	for _, f := range fields {
		switch f.Label {
		case "Pitch (decimal °)":
			pitchDec = f.Value
		case "Yaw (decimal °)":
			yawDec = f.Value
		case "Roll (decimal °)":
			rollDec = f.Value
		}
	}
	if pitchDec == "" || yawDec == "" || rollDec == "" {
		t.Fatalf("missing decimal fields: pitch=%q yaw=%q roll=%q", pitchDec, yawDec, rollDec)
	}
	// 1 rad pitch → ~57.295780°
	if !strings.Contains(pitchDec, "57") {
		t.Fatalf("pitch value unexpected %q", pitchDec)
	}
}

func buildMinimalAttitudePayload() []byte {
	var b []byte
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, 0)
	b = append(b, tmp...)
	b = append(b, 0, 0, 0, 0)
	b = append(b, f64beSlice(1.0)...)
	b = append(b, f64beSlice(0.0)...)
	b = append(b, f64beSlice(0.0)...)
	b = append(b, f64beSlice(0.0)...)
	b = append(b, 0, 0)
	for i := 0; i < 7; i++ {
		b = append(b, f32beSlice(0)...)
	}
	return b
}

func f64beSlice(v float64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, math.Float64bits(v))
	return out
}

func f32beSlice(v float32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, math.Float32bits(v))
	return out
}
