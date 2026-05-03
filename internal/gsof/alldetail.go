package gsof

import "sort"

// AllSV48Page is the version and page header on GSOF type 48 (multi-page all-SV detailed).
// Page info byte: bits 0–3 = total pages, bits 4–7 = current page (Trimble OEM, e.g. 0x12 → page 1 of 2).
type AllSV48Page struct {
	Version     int `json:"version"`
	PageCurrent int `json:"page_current"`
	PageTotal   int `json:"page_total"`
}

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

func sortAllSVDetailedBySystemPRN(rows []AllSVDetailedEntry) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].System != rows[j].System {
			return rows[i].System < rows[j].System
		}
		return rows[i].PRN < rows[j].PRN
	})
}

// parseAllSVDetailedRowsFromReader reads count × 10-byte SV rows from br (same layout as type 34).
func parseAllSVDetailedRowsFromReader(br *beReader, count int) []AllSVDetailedEntry {
	var rows []AllSVDetailedEntry
	for i := 0; i < count; i++ {
		if !br.ok(10) {
			return rows
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
	return rows
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
	rows = parseAllSVDetailedRowsFromReader(&br, count)
	sortAllSVDetailedBySystemPRN(rows)
	return count, rows
}

// ParseAllSVDetailedType48 reads GSOF type-48: format version, page-info byte, SV count on this page,
// then the same per-SV layout as type 34. Rows are sorted by GNSS system then PRN.
func ParseAllSVDetailedType48(payload []byte) (hdr AllSV48Page, count int, rows []AllSVDetailedEntry) {
	if len(payload) < 3 {
		return hdr, 0, nil
	}
	br := beReader{b: payload}
	hdr.Version = int(br.u8())
	pg := br.u8()
	hdr.PageTotal = int(pg & 0x0F)
	hdr.PageCurrent = int((pg >> 4) & 0x0F)
	count = int(br.u8())
	rows = parseAllSVDetailedRowsFromReader(&br, count)
	sortAllSVDetailedBySystemPRN(rows)
	return hdr, count, rows
}
