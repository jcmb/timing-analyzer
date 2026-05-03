package gsof

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseAllSVDetailedEntriesOrderAndSNR(t *testing.T) {
	// count=2: wire PRN,sys,... then second row — sorted by system then PRN.
	// Row A: PRN 10, GLO(2) — 10,2,...  Row B: PRN 5, GPS(0) — 5,0,...
	payload := make([]byte, 1+2*10)
	payload[0] = 2
	off := 1
	putRow := func(prn, sys byte, f1, f2 byte, elev int8, az uint16, sn1, sn2, sn5 byte) {
		payload[off] = prn
		payload[off+1] = sys
		payload[off+2] = f1
		payload[off+3] = f2
		payload[off+4] = byte(elev)
		binary.BigEndian.PutUint16(payload[off+5:], az)
		payload[off+7] = sn1
		payload[off+8] = sn2
		payload[off+9] = sn5
		off += 10
	}
	putRow(10, 2, 1, 2, -5, 90, 8, 12, 4)
	putRow(5, 0, 3, 4, 30, 180, 20, 24, 8)
	n, rows := ParseAllSVDetailedEntries(payload)
	if n != 2 || len(rows) != 2 {
		t.Fatalf("n=%d len=%d", n, len(rows))
	}
	if rows[0].System != 0 || rows[0].PRN != 5 || rows[0].Flags1 != 3 {
		t.Fatalf("row0 %+v", rows[0])
	}
	if rows[1].System != 2 || rows[1].PRN != 10 {
		t.Fatalf("row1 %+v", rows[1])
	}
	if rows[0].Elev != 30 || rows[0].Azimuth != 180 || math.Abs(rows[0].SNRL1-5.0) > 1e-9 {
		t.Fatalf("row0 metrics %+v", rows[0])
	}
}

func TestParseAllSVDetailedType48HeaderAndSV(t *testing.T) {
	// Version 3, page-info 0x12 → page 1 of 2, one SV (same 10-byte row as type 34).
	payload := make([]byte, 3+10)
	payload[0] = 3
	payload[1] = 0x12
	payload[2] = 1
	off := 3
	payload[off] = 6
	payload[off+1] = 0
	payload[off+2] = 0x0a
	payload[off+3] = 0x0b
	payload[off+4] = 10
	binary.BigEndian.PutUint16(payload[off+5:], 270)
	payload[off+7] = 4
	payload[off+8] = 8
	payload[off+9] = 12
	hdr, n, rows := ParseAllSVDetailedType48(payload)
	if hdr.Version != 3 || hdr.PageCurrent != 1 || hdr.PageTotal != 2 {
		t.Fatalf("hdr %+v", hdr)
	}
	if n != 1 || len(rows) != 1 {
		t.Fatalf("n=%d len=%d", n, len(rows))
	}
	e := rows[0]
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
