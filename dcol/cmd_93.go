package dcol

// handler93 decodes CMR-style DCOL type 0x93.
type handler93 struct{}

func (handler93) Handle(_ *Parser, fr Frame, env Env) (*Message, error) {
	payloadLen := fr.PayloadLen
	var subType, version, stationID int
	if payloadLen >= 2 {
		pl := fr.Payload()
		firstByte := pl[0]
		secondByte := pl[1]
		version = int((firstByte >> 5) & 0x07)
		stationID = int(firstByte & 0x1F)
		subType = int((secondByte >> 5) & 0x07)
	}
	return &Message{
		BestTime:      env.BestTime,
		GoTime:        env.GoTime,
		KernelTime:    env.KernelTime,
		Length:        fr.TotalLen,
		RemoteAddr:    env.RemoteAddr,
		PacketType:    int(fr.Type),
		Decoded:       true,
		IsCMR:         true,
		PacketSubType: subType,
		Version:       version,
		StationID:     stationID,
		IsLastInBurst: true,
	}, nil
}
