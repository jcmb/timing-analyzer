package gsofstats

import (
	"math"

	"timing-analyzer/internal/gsof"
)

const llhCloseDeg = 1e-7
const llhCloseHm = 1e-3

func positionsCloseLLH(aLat, aLon, aH, bLat, bLon, bH float64) bool {
	return math.Abs(aLat-bLat) < llhCloseDeg &&
		math.Abs(aLon-bLon) < llhCloseDeg &&
		math.Abs(aH-bH) < llhCloseHm
}

var stripReceivedBaseLLHLabels = map[string]struct{}{
	"Base latitude (DMS)":         {},
	"Base longitude (DMS)":        {},
	"Base latitude (decimal °)":   {},
	"Base longitude (decimal °)":  {},
	"Base height (m)":             {},
}

var stripSecondAntennaLLHLabels = map[string]struct{}{
	"Latitude (DMS)":        {},
	"Longitude (DMS)":       {},
	"Latitude (decimal °)":  {},
	"Longitude (decimal °)": {},
	"Height (m)":            {},
}

func filterFieldLabels(fields []gsof.Field, drop map[string]struct{}) []gsof.Field {
	if len(fields) == 0 {
		return fields
	}
	out := make([]gsof.Field, 0, len(fields))
	for _, f := range fields {
		if _, d := drop[f.Label]; d {
			continue
		}
		out = append(out, f)
	}
	return out
}

func insertFieldAfterLabel(fields []gsof.Field, afterLabel string, insert gsof.Field) []gsof.Field {
	out := make([]gsof.Field, 0, len(fields)+1)
	inserted := false
	for _, f := range fields {
		out = append(out, f)
		if !inserted && f.Label == afterLabel {
			out = append(out, insert)
			inserted = true
		}
	}
	if !inserted {
		out = append(out, insert)
	}
	return out
}

// applyBaseStationFieldDedup removes duplicate lat/lon/height lines when received-base (35)
// or second-antenna (97) matches the canonical base position (41, else 35).
func (s *Stats) applyBaseStationFieldDedup(rows []RecordRow) {
	base41, ok41 := gsof.ParseBasePositionQualityInfo(s.lastPayload[41])
	rb35, ok35 := gsof.ParseReceivedBaseInfo(s.lastPayload[35])
	pt97, ok97 := gsof.ParseSecondAntenna97Point(s.lastPayload[97])

	var cLat, cLon, cH float64
	haveCanon := false
	if ok41 {
		cLat, cLon, cH = base41.LatDeg, base41.LonDeg, base41.HeightM
		haveCanon = true
	} else if ok35 {
		cLat, cLon, cH = rb35.LatDeg, rb35.LonDeg, rb35.HeightM
		haveCanon = true
	}
	if !haveCanon {
		return
	}

	for i := range rows {
		switch rows[i].Type {
		case 35:
			if !ok41 || !ok35 {
				continue
			}
			if !positionsCloseLLH(base41.LatDeg, base41.LonDeg, base41.HeightM, rb35.LatDeg, rb35.LonDeg, rb35.HeightM) {
				continue
			}
			f := filterFieldLabels(rows[i].Fields, stripReceivedBaseLLHLabels)
			note := gsof.Field{
				Label: "Base position",
				Value: "Same lat/lon/height as GSOF type 41 (base position and quality) in this snapshot — duplicate coordinate lines omitted.",
			}
			rows[i].Fields = insertFieldAfterLabel(f, "Base ID", note)
		case 97:
			if !ok97 {
				continue
			}
			if !positionsCloseLLH(cLat, cLon, cH, pt97.LatDeg, pt97.LonDeg, pt97.HeightM) {
				continue
			}
			f := filterFieldLabels(rows[i].Fields, stripSecondAntennaLLHLabels)
			note := gsof.Field{
				Label: "Antenna position",
				Value: "Same lat/lon/height as the base position message (type 41, or type 35 if 41 is absent) in this snapshot — duplicate coordinate lines omitted.",
			}
			rows[i].Fields = insertFieldAfterLabel(f, "Source", note)
		}
	}
}
