package gsof

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func TestFlattenGSOFBufferNo99SameSlice(t *testing.T) {
	src := []byte{0x01, 0x00}
	out := FlattenGSOFBuffer(src)
	if !bytes.Equal(out, src) || len(out) != 2 {
		t.Fatalf("got % x", out)
	}
}

func TestFlattenGSOFBufferExpands99(t *testing.T) {
	// One extended type 100: u16 BE 100, u8 len 32, 8-char datum + 3×f64 (rad, rad, m) = 35 bytes inside type 99
	pl := make([]byte, 35)
	binary.BigEndian.PutUint16(pl[0:2], 100)
	pl[2] = 32
	copy(pl[3:11], []byte("MYDATUM\x00"))
	binary.BigEndian.PutUint64(pl[11:19], math.Float64bits(math.Pi/6))
	binary.BigEndian.PutUint64(pl[19:27], math.Float64bits(0))
	binary.BigEndian.PutUint64(pl[27:35], math.Float64bits(12.5))
	outer := append([]byte{0x63, byte(len(pl))}, pl...)
	out := FlattenGSOFBuffer(outer)
	// Flattened wire is [type][len][len bytes] → 1 + 1 + 32 = 34 bytes for type 100.
	if len(out) != 34 || out[0] != 0x64 || out[1] != 32 {
		t.Fatalf("flattened header % x full % x", out[:4], out)
	}
	fields := Decode(100, out[2:])
	if !strings.Contains(fieldText(fields), "MYDATUM") {
		t.Fatalf("%s", fieldText(fields))
	}
}

func TestFlattenGSOFBufferTwoExtendedInOne99(t *testing.T) {
	// Type 99 payload: [100,32,<32>] + [102,33,<33>]
	var b []byte
	ext100 := make([]byte, 35)
	binary.BigEndian.PutUint16(ext100[0:2], 100)
	ext100[2] = 32
	copy(ext100[3:11], []byte("ABCD1234"))
	binary.BigEndian.PutUint64(ext100[11:19], math.Float64bits(0))
	binary.BigEndian.PutUint64(ext100[19:27], math.Float64bits(0))
	binary.BigEndian.PutUint64(ext100[27:35], math.Float64bits(1))
	b = append(b, ext100...)
	ext102 := make([]byte, 36)
	binary.BigEndian.PutUint16(ext102[0:2], 102)
	ext102[2] = 33
	ext102[3] = 0x0F
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint64(ext102[4+i*8:12+i*8], math.Float64bits(float64(i+1)*0.01))
	}
	b = append(b, ext102...)
	outer := append([]byte{0x63, byte(len(b))}, b...)
	out := FlattenGSOFBuffer(outer)
	// First record 34 bytes (0x64 + 0x20 + 32), second starts at offset 34.
	if out[0] != 0x64 || out[34] != 0x66 || out[35] != 33 {
		t.Fatalf("want 100 then 102, got % x", out)
	}
}

func TestExpandGSOFStreamWireRetainsType99Wrapper(t *testing.T) {
	pl := make([]byte, 35)
	binary.BigEndian.PutUint16(pl[0:2], 100)
	pl[2] = 32
	copy(pl[3:11], []byte("MYDATUM\x00"))
	binary.BigEndian.PutUint64(pl[11:19], math.Float64bits(0))
	binary.BigEndian.PutUint64(pl[19:27], math.Float64bits(0))
	binary.BigEndian.PutUint64(pl[27:35], math.Float64bits(1))
	outer := append([]byte{0x63, byte(len(pl))}, pl...)
	ex := ExpandGSOFStream(outer)
	if len(ex) != 1 || ex[0].MsgType != 100 {
		t.Fatalf("got %+v", ex)
	}
	if len(ex[0].Wire) != len(outer) || ex[0].Wire[0] != 0x63 {
		t.Fatalf("wire % x want prefix 63", ex[0].Wire)
	}
}

func TestExpandGSOFStreamInvalidExtendedType243(t *testing.T) {
	// u16 extended type 5 (<100): entire remainder wrapped as unknown.
	pl := []byte{0x00, 0x05, 0x00}
	outer := append([]byte{0x63, byte(len(pl))}, pl...)
	ex := ExpandGSOFStream(outer)
	if len(ex) != 1 || ex[0].MsgType != GSOFExtendedUnknown {
		t.Fatalf("got %+v", ex)
	}
	if len(ex[0].Inner) != 3 || ex[0].Wire[0] != 0x63 {
		t.Fatalf("inner % x wire % x", ex[0].Inner, ex[0].Wire)
	}
}

func TestDecode102SecondAntennaHeading(t *testing.T) {
	pl := make([]byte, 33)
	pl[0] = 0x0A
	binary.BigEndian.PutUint64(pl[1:9], math.Float64bits(0.02))
	binary.BigEndian.PutUint64(pl[9:17], math.Float64bits(0.03))
	binary.BigEndian.PutUint64(pl[17:25], math.Float64bits(0.04))
	binary.BigEndian.PutUint64(pl[25:33], math.Float64bits(0.05))
	fields := Decode(102, pl)
	got := fieldText(fields)
	if !strings.Contains(got, "Heading geodetic north") || !strings.Contains(got, "0x0A") {
		t.Fatalf("%s", got)
	}
}
