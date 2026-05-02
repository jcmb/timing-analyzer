package gsof

import "sort"

// SVBriefEntry is one row of GSOF type 13 (GPS SV brief): PRN and two flag bytes.
type SVBriefEntry struct {
	PRN    int `json:"prn"`
	Flags1 int `json:"flags1"`
	Flags2 int `json:"flags2"`
}

// AllSVBriefEntry is one row of GSOF type 33 (all systems SV brief): GNSS system id, PRN, two flag bytes.
type AllSVBriefEntry struct {
	System     int    `json:"system"`
	SystemName string `json:"system_name"`
	PRN        int    `json:"prn"`
	Flags1     int    `json:"flags1"`
	Flags2     int    `json:"flags2"`
}

// ParseSVBriefEntries reads the type-13 payload: count byte, then count × (PRN, flags1, flags2).
// count is the declared number from the first byte; rows may be shorter if the payload is truncated.
func ParseSVBriefEntries(payload []byte) (count int, rows []SVBriefEntry) {
	if len(payload) < 1 {
		return 0, nil
	}
	br := beReader{b: payload}
	count = int(br.u8())
	for i := 0; i < count; i++ {
		if !br.ok(3) {
			return count, rows
		}
		prn, f1, f2 := br.u8(), br.u8(), br.u8()
		rows = append(rows, SVBriefEntry{PRN: int(prn), Flags1: int(f1), Flags2: int(f2)})
	}
	return count, rows
}

// ParseAllSVBriefEntries reads the type-33 payload: count byte, then count × (GNSS system, PRN, flags1, flags2).
// Rows are sorted by SV system id then PRN for stable display. count is the declared count; rows may be shorter if truncated.
func ParseAllSVBriefEntries(payload []byte) (count int, rows []AllSVBriefEntry) {
	if len(payload) < 1 {
		return 0, nil
	}
	br := beReader{b: payload}
	count = int(br.u8())
	for i := 0; i < count; i++ {
		if !br.ok(4) {
			return count, rows
		}
		sys := int(br.u8())
		prn := int(br.u8())
		f1 := int(br.u8())
		f2 := int(br.u8())
		rows = append(rows, AllSVBriefEntry{
			System:     sys,
			SystemName: gnssName(sys),
			PRN:        prn,
			Flags1:     f1,
			Flags2:     f2,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].System != rows[j].System {
			return rows[i].System < rows[j].System
		}
		return rows[i].PRN < rows[j].PRN
	})
	return count, rows
}
