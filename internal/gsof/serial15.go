package gsof

// ParseSerial15 parses GSOF type 15 (receiver serial number): big-endian u32.
func ParseSerial15(payload []byte) (serial uint32, ok bool) {
	br := beReader{b: payload}
	if !br.ok(4) {
		return 0, false
	}
	return br.u32(), true
}
