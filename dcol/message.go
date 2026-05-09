package dcol

import "time"

// Message is one decoded DCOL packet emitted to consumers (timestamps are passed through from Env).
type Message struct {
	BestTime       time.Time
	GoTime         time.Time
	KernelTime     time.Time
	Length         int
	RemoteAddr     string
	PacketType     int
	Decoded        bool
	IsCMR          bool
	PacketSubType  int
	Version        int
	StationID      int
	IsLastInBurst  bool
	GSOFBuffer     []byte
	SequenceNumber uint8
	StreamWarnings []string
	// Payload is a copy of the DCOL payload bytes (between the length byte and checksum), when useful for generic consumers.
	Payload []byte
}

// Frame is one validated DCOL frame (STX .. ETX inclusive).
type Frame struct {
	Type       byte
	PayloadLen byte
	TotalLen   int
	Raw        []byte
}

// Payload returns DCOL payload bytes (indices [4 : TotalLen-2]) referencing Raw.
func (f Frame) Payload() []byte {
	if f.TotalLen < 6 {
		return nil
	}
	return f.Raw[4 : f.TotalLen-2]
}
