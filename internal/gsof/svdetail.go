package gsof

// SVDetailedEntry is one row of GSOF type 14 (detailed satellite info), 8 bytes per SV.
type SVDetailedEntry struct {
	PRN    int     `json:"prn"`
	Flags1 int     `json:"flags1"`
	Flags2 int     `json:"flags2"`
	Elev    int     `json:"elev"`
	Azimuth int     `json:"azimuth"`
	SNRL1   float64 `json:"snr_l1"`
	SNRL2   float64 `json:"snr_l2"`
}

// ParseSVDetailedEntries reads type-14 payload: count byte, then count × 8-byte records.
func ParseSVDetailedEntries(payload []byte) (count int, rows []SVDetailedEntry) {
	if len(payload) < 1 {
		return 0, nil
	}
	br := beReader{b: payload}
	count = int(br.u8())
	for i := 0; i < count; i++ {
		if !br.ok(8) {
			return count, rows
		}
		prn := br.u8()
		f1 := br.u8()
		f2 := br.u8()
		elev := int8(br.u8())
		az := br.u16()
		snrL1 := br.u8()
		snrL2 := br.u8()
		rows = append(rows, SVDetailedEntry{
			PRN:    int(prn),
			Flags1: int(f1),
			Flags2:  int(f2),
			Elev:    int(elev),
			Azimuth: int(az),
			SNRL1:   float64(snrL1) / 4,
			SNRL2:   float64(snrL2) / 4,
		})
	}
	return count, rows
}
