package gsof

import "encoding/binary"

// ParseCurrentUTCEpochSec returns seconds since an arbitrary GPS-style epoch week boundary
// from GSOF type-16 payload (milliseconds of week, UTF week number; first 9 bytes —
// Trimble OEM "Current UTC time" layout).
//
// Adjacent deltas match solution cadence; use alongside max-step filtering in the consumer.
func ParseCurrentUTCEpochSec(payload []byte) (epochSec float64, ok bool) {
	if len(payload) < 9 {
		return 0, false
	}
	towMs := binary.BigEndian.Uint32(payload[0:4])
	week := binary.BigEndian.Uint16(payload[4:6])
	return JSONFloat(float64(week)*604800.0 + float64(towMs)/1000.0), true
}
