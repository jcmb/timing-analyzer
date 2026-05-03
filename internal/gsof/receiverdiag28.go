package gsof

// ReceiverDiagnosticsPoint is one GSOF type-0x1C (28) receiver diagnostics sample vs GPS TOW (from latest type-0x01).
type ReceiverDiagnosticsPoint struct {
	GPSTOWSec          float64 `json:"gps_tow_s"`
	LinkIntegrityPct   float64 `json:"link_integrity_pct"`
	CommonL1SVs        int     `json:"common_l1_svs"`
	CommonL2SVs        int     `json:"common_l2_svs"`
	DatalinkLatencySec float64 `json:"datalink_latency_s"`
	DiffSVsInUse       int     `json:"diff_svs_in_use"`
}

// ParseReceiverDiagnosticsPoint parses the 18-byte type-28 payload (same layout as decodeReceiverDiagnostics).
func ParseReceiverDiagnosticsPoint(payload []byte) (ReceiverDiagnosticsPoint, bool) {
	const need = 18
	if len(payload) < need {
		return ReceiverDiagnosticsPoint{}, false
	}
	link100 := payload[6]
	commonL1 := int(payload[9])
	commonL2 := int(payload[10])
	datalinkLatencyTenths := payload[11]
	diffSVs := int(payload[13])
	linkPct := float64(link100) * 100.0 / 255.0
	latencySec := float64(datalinkLatencyTenths) / 10.0
	return ReceiverDiagnosticsPoint{
		LinkIntegrityPct:   linkPct,
		CommonL1SVs:        commonL1,
		CommonL2SVs:        commonL2,
		DatalinkLatencySec: latencySec,
		DiffSVsInUse:       diffSVs,
	}, true
}
