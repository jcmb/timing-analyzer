package gsof

// SVBriefEntry is one row of GSOF type 13 (GPS SV brief): PRN and two flag bytes.
type SVBriefEntry struct {
	PRN    int `json:"prn"`
	Flags1 int `json:"flags1"`
	Flags2 int `json:"flags2"`
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
