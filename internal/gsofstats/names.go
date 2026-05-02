package gsofstats

import "fmt"

// RecordName returns a short description for a GSOF record type byte.
func RecordName(recType int) string {
	if recType >= 0 && recType < len(recordNames) && recordNames[recType] != "" {
		return recordNames[recType]
	}
	return fmt.Sprintf("GSOF 0x%02X", recType)
}

var recordNames = []string{
	0:  "reserved",
	1:  "Position Time",
	2:  "Latitude Longitude Height",
	3:  "ECEF Position",
	4:  "Local Datum LLH Position",
	5:  "Local Zone ENU Position",
	6:  "ECEF Delta",
	7:  "Tangent Plane Delta",
	8:  "Velocity Data",
	9:  "PDOP Information",
	10: "Clock Information",
	11: "Position VCV Information",
	12: "Position Sigma Information",
	13: "SV Brief Information",
	14: "SV Detailed Information",
	15: "Receiver Serial Number",
	16: "Current Time",
	26: "Position Time UTC",
	27: "Attitude Information",
	33: "All SV Brief Information",
	34: "All SV Detailed Information",
	35: "Received Base Information",
	37: "Battery and Memory Information",
	40: "L-Band Status Information",
	41: "Base Position and Quality Indicator",
}
