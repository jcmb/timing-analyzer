package gsof

import "fmt"

// OverviewURL is the Trimble OEM GNSS help page summarizing supported GSOF messages.
// Source: https://receiverhelp.trimble.com/oem-gnss/gsof-messages-overview.html
const OverviewURL = "https://receiverhelp.trimble.com/oem-gnss/gsof-messages-overview.html"

const docBase = "https://receiverhelp.trimble.com/oem-gnss/"

// Message describes one GSOF record type listed on the overview page.
type Message struct {
	ID       int
	Title    string // Short title for UI
	Function string // One-line function from overview table
	DocPath  string // Relative to docBase; empty means overview only
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
	13: {13, "GPS SV brief info", "GPS space-vehicle brief information.", ""},
	14: {14, "Detailed satellite info", "Detailed satellite information.", ""},
	15: {15, "Receiver serial number", "Receiver serial number.", ""},
	16: {16, "Current UTC time", "Current UTC time.", ""},
	27: {27, "Attitude info", "Attitude information.", ""},
	33: {33, "All SV brief info", "SV brief information for all satellite systems.", "gsof-messages-all-sv-brief.html"},
	34: {34, "All SV detailed info", "Detailed satellite information for all tracked satellite systems.", ""},
	35: {35, "Base station info", "Received information about the base station.", ""},
	37: {37, "Battery and memory", "Receiver battery and memory status.", ""},
	38: {38, "Position type", "Position type information.", ""},
	40: {40, "L-Band status", "L-Band status information.", ""},
	41: {41, "Base position and quality", "Base station position and its quality.", "gsof-messages-base-position-quality-indicator.html"},
	48: {48, "All SV detailed (multi-page)", "Detailed satellite information for all tracked satellites in all systems.", "gsof-messages-multiple-page-detail-all-sv.html"},
	49: {49, "INS navigation", "INS navigation information.", ""},
	50: {50, "INS RMS", "INS RMS information.", ""},
	51: {51, "Event marker", "Event marker information.", ""},
	62: {62, "Code LLH", "Latitude, longitude, height from the code (pseudorange) solution.", "gsof-messages-code-position-llh.html"},
	70: {70, "Lat, long, MSL height", "Latitude, longitude, MSL height.", "gsof-messages-llmsl.html"},
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
		return docBase + m.DocPath
	}
	return OverviewURL
}

func fmtUnknownTitle(id int) string {
	return fmt.Sprintf("GSOF message %d (0x%02X)", id, id)
}
