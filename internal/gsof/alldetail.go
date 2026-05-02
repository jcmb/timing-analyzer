package gsof

import "sort"

// AllSVDetailedEntry is one row of GSOF type 34 (all systems SV detailed): same identity and
// flags as type 33, plus elevation, azimuth, and L1/L2/L5 SNR (quarter-dB units on the wire).
type AllSVDetailedEntry struct {
	System     int     `json:"system"`
	SystemName string  `json:"system_name"`
	PRN        int     `json:"prn"`
	Flags1     int     `json:"flags1"`
	Flags2     int     `json:"flags2"`
	Elev       int     `json:"elev"`
	Azimuth    int     `json:"azimuth"`
	SNRL1      float64 `json:"snr_l1"`
	SNRL2      float64 `json:"snr_l2"`
	SNRL5      float64 `json:"snr_l5"`
}

// ParseAllSVDetailedEntries reads GSOF type-34: count byte, then count × (PRN, GNSS system,
// flags1, flags2, elevation int8°, azimuth uint16°, SNR L1/L2/L5 as uint8 ÷ 4).
// Rows are sorted by GNSS system id then PRN (same display order as type 33).
func ParseAllSVDetailedEntries(payload []byte) (count int, rows []AllSVDetailedEntry) {
	if len(payload) < 1 {
		return 0, nil
	}
	br := beReader{b: payload}
	count = int(br.u8())
	for i := 0; i < count; i++ {
		if !br.ok(10) {
			return count, rows
		}
		prn := int(br.u8())
		sys := int(br.u8())
		f1 := int(br.u8())
		f2 := int(br.u8())
		elev := int(int8(br.u8()))
		az := int(br.u16())
		s1 := float64(br.u8()) / 4
		s2 := float64(br.u8()) / 4
		s5 := float64(br.u8()) / 4
		rows = append(rows, AllSVDetailedEntry{
			System:     sys,
			SystemName: gnssName(sys),
			PRN:        prn,
			Flags1:     f1,
			Flags2:     f2,
			Elev:       elev,
			Azimuth:    az,
			SNRL1:      s1,
			SNRL2:      s2,
			SNRL5:      s5,
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
