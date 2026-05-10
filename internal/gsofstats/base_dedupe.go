package gsofstats

import (
	"fmt"
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

func validYesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func fieldsBaseStation41(rb gsof.ReceivedBaseInfo, ok35 bool, bp gsof.BasePositionQualityInfo, antennaDiffers bool) []gsof.Field {
	out := []gsof.Field{
		{Label: "Summary", Value: gsof.Lookup(41).Function},
		{Label: "Base station", Value: ""},
	}
	if ok35 {
		out = append(out,
			gsof.Field{Label: "Valid.", Value: validYesNo(rb.InfoValid)},
			gsof.Field{Label: "Base name", Value: rb.Name},
			gsof.Field{Label: "Base ID", Value: fmt.Sprintf("%d", rb.BaseID)},
		)
	}
	out = append(out,
		gsof.Field{
			Label: "Position",
			Value: gsof.FormatLatLonHeightEllipsoidalLine(bp.LatDeg, bp.LonDeg, bp.HeightM),
		},
		gsof.Field{Label: "Quality", Value: gsof.ShortBasePositionQuality(bp.Quality)},
	)
	if ok35 && antennaDiffers {
		out = append(out, gsof.Field{
			Label: "Antenna position",
			Value: gsof.FormatLatLonHeightEllipsoidalLine(rb.LatDeg, rb.LonDeg, rb.HeightM),
		})
	}
	return out
}

func fieldsReceivedBase35Standalone(rb gsof.ReceivedBaseInfo) []gsof.Field {
	return []gsof.Field{
		{Label: "Summary", Value: gsof.Lookup(35).Function},
		{Label: "Base station", Value: ""},
		{Label: "Valid.", Value: validYesNo(rb.InfoValid)},
		{Label: "Base name", Value: rb.Name},
		{Label: "Base ID", Value: fmt.Sprintf("%d", rb.BaseID)},
		{
			Label: "Position",
			Value: gsof.FormatLatLonHeightEllipsoidalLine(rb.LatDeg, rb.LonDeg, rb.HeightM),
		},
	}
}

func fieldsReceivedBase35WithType41(rb gsof.ReceivedBaseInfo) []gsof.Field {
	return []gsof.Field{
		{Label: "Summary", Value: gsof.Lookup(35).Function},
		{Label: "Base station", Value: ""},
		{Label: "Valid.", Value: validYesNo(rb.InfoValid)},
		{Label: "Base name", Value: rb.Name},
		{Label: "Base ID", Value: fmt.Sprintf("%d", rb.BaseID)},
		{
			Label: "Note",
			Value: "Position and quality are combined on the GSOF type 41 (base position and quality) card.",
		},
	}
}

// applyBaseStationFieldDedup reformats base station types 35 and 41 for the dashboard and removes
// duplicate lat/lon/height lines from second-antenna (97) when it matches the canonical base position.
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

	antennaDiffers := ok35 && ok41 &&
		!positionsCloseLLH(base41.LatDeg, base41.LonDeg, base41.HeightM, rb35.LatDeg, rb35.LonDeg, rb35.HeightM)

	for i := range rows {
		switch rows[i].Type {
		case 41:
			if !ok41 {
				continue
			}
			if ok35 {
				rows[i].Fields = fieldsBaseStation41(rb35, true, base41, antennaDiffers)
			} else {
				rows[i].Fields = fieldsBaseStation41(gsof.ReceivedBaseInfo{}, false, base41, false)
			}
		case 35:
			if !ok35 {
				continue
			}
			if ok41 {
				rows[i].Fields = fieldsReceivedBase35WithType41(rb35)
			} else {
				rows[i].Fields = fieldsReceivedBase35Standalone(rb35)
			}
		case 97:
			if !ok97 || !haveCanon {
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
