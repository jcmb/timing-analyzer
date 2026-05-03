package gsof

import (
	"fmt"
	"strings"
)

// OverviewURL is the Trimble OEM GNSS help page summarizing supported GSOF messages.
// Source: https://receiverhelp.trimble.com/oem-gnss/gsof-messages-overview.html
const OverviewURL = "https://receiverhelp.trimble.com/oem-gnss/gsof-messages-overview.html"

const docBase = "https://receiverhelp.trimble.com/oem-gnss/"

// Message describes one GSOF record type listed on the overview page.
type Message struct {
	ID       int
	Title    string // Short title for UI
	Function string // One-line function from overview table
	// DocPath is normally a path under docBase (e.g. "gsof-messages-time.html").
	// If it starts with http:// or https://, DocURL returns it unchanged (external documentation).
	DocPath string
}

// Catalog is every message # listed on the GSOF messages overview (Trimble OEM GNSS).
var Catalog = map[int]Message{
	1:  {1, "Position time", "Position and time (antenna phase center).", "gsof-messages-time.html"},
	2:  {2, "Latitude, longitude, height", "Latitude, longitude, height.", "gsof-messages-llh.html"},
	3:  {3, "ECEF position", "Earth-centered, Earth-fixed position.", "gsof-messages-ecef.html"},
	4:  {4, "Local datum LLH", "Local datum position — latitude, longitude, height.", ""},
	5:  {5, "Local zone ENU", "Local zone north, east, and height (projection/calibration based).", ""},
	6:  {6, "ECEF delta", "Earth-centered, Earth-fixed delta position.", "gsof-messages-ecef-delta.html"},
	7:  {7, "Tangent plane delta", "Tangent plane delta.", "gsof-messages-tplane-enu.html"},
	8:  {8, "Velocity data", "Velocity data.", "gsof-messages-velocity.html"},
	9:  {9, "PDOP info", "PDOP information.", "gsof-messages-pdop.html"},
	10: {10, "Clock info", "Clock information.", "gsof-messages-clock-info.html"},
	11: {11, "Position VCV info", "Position variance-covariance (VCV) information.", "gsof-messages-position-vcv.html"},
	12: {12, "Position sigma", "Position sigma information.", "gsof-messages-sigma.html"},
	13: {13, "GPS SV brief info", "GPS space-vehicle brief information.", "gsof-messages-sv-brief.html"},
	14: {14, "Detailed satellite info", "Detailed satellite information.", "gsof-messages-sv-detail.html"},
	15: {15, "Receiver serial number", "Receiver serial number.", "gsof-messages-receiver-serial-no.html"},
	16: {16, "Current UTC time", "Current UTC time.", "gsof-messages-utc.html"},
	27: {27, "Attitude info", "Attitude information.", "gsof-messages-attitude.html"},
	28: {28, "Receiver diagnostics", "Receiver diagnostics (datalink, base flags, common SV counts).", ""},
	33: {33, "All SV brief info", "SV brief information for all satellite systems.", "gsof-messages-all-sv-brief.html"},
	34: {34, "All SV detailed info", "Detailed satellite information for all tracked satellite systems.", "gsof-messages-all-sv-detail.html"},
	35: {35, "Base station info", "Received information about the base station.", "gsof-messages-received-base-info.html"},
	37: {37, "Battery and memory", "Receiver battery and memory status.", "gsof-messages-batt-mem.html"},
	38: {38, "Position type", "Position type information.", "gsof-messages-position-type.html"},
	40: {40, "L-Band status", "L-Band status information.", "gsof-messages-l-band.html"},
	41: {41, "Base position and quality", "Base station position and its quality.", "gsof-messages-base-position-quality-indicator.html"},
	48: {48, "All SV detailed (multi-page)", "Detailed satellite information for all tracked satellites in all systems.", "gsof-messages-multiple-page-detail-all-sv.html"},
	49: {49, "INS navigation", "INS navigation information.", ""},
	50: {50, "INS RMS", "INS RMS information.", ""},
	51: {51, "Event marker", "Event marker information.", ""},
	57: {57, "Radio info", "Radio band, channel, signal and noise strength per datalink.", "https://docs.google.com/document/d/1H5RPgu3INoZ0NA1Pd48GDKvsqNaCHJgsjhKVxZi4lG4/edit?usp=sharing"},
	62: {62, "Code LLH", "Latitude, longitude, height from the code (pseudorange) solution.", "gsof-messages-code-position-llh.html"},
	70: {70, "Lat, long, MSL height", "Latitude, longitude, MSL height.", "gsof-messages-llmsl.html"},
	74: {74, "Position sigma (second antenna)", "Position RMS and sigmas for the second antenna RTK solution.", "https://docs.google.com/document/d/1_h1aBHjor4eH5aJ_3_nj8_BeTUGK3EoZKxik_9R9HCY/edit?usp=sharing"},
	91: {91, "NMA info", "Navigation Message Authentication (NMA) information.", "https://docs.google.com/document/d/1mxY_s34PX3jYNNM81WvM0gDJL_dQKDPsxqa5TdHiepM/edit?tab=t.0"},
	92: {92, "IonoGuard info", "IonoGuard ionospheric monitoring information.", "https://docs.google.com/document/d/1aIc38r95I3LCiIycIj_VmDws7jat2ed55j0Ve6U8tjM/edit?usp=sharing"},
	96: {96, "IonoGuard summary", "IonoGuard ionospheric summary information.", "https://docs.google.com/document/d/1FEliQDO_vcX1KZqz8pjy0DcXZNEfA1hXipYMjvKWbF4/edit?usp=sharing"},
	97: {97, "Second antenna position", "WGS-84 position and sigmas for the second antenna (GGA2-related).", "https://docs.google.com/document/d/1fdq0SSPibJn_rc_BbrpnKKWZni3BjRu4nbOACV_E_4o/edit?usp=sharing"},
	98: {98, "Error estimates (antenna 2)", "ECEF VCV and RMS for the second-antenna position solution.", "https://docs.google.com/document/d/1QDThFOoOE2KSvbEaMNZGwnPK16jEwmCe1Se7YgywvJo/edit?usp=sharing"},
}

// Lookup returns catalog metadata, or a synthetic entry for unknown record types.
func Lookup(id int) Message {
	if m, ok := Catalog[id]; ok {
		return m
	}
	return Message{
		ID:       id,
		Title:    fmtUnknownTitle(id),
		Function: "Message type not listed on the Trimble GSOF overview; see receiver documentation.",
		DocPath:  "",
	}
}

// DocURL returns the best help URL for this message (specific page or overview).
func (m Message) DocURL() string {
	if m.DocPath != "" {
		if strings.HasPrefix(m.DocPath, "http://") || strings.HasPrefix(m.DocPath, "https://") {
			return m.DocPath
		}
		return docBase + m.DocPath
	}
	return OverviewURL
}

func fmtUnknownTitle(id int) string {
	return fmt.Sprintf("GSOF message %d (0x%02X)", id, id)
}
