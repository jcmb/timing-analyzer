package dcol

// handler98 decodes CMR-style DCOL type 0x98.
type handler98 struct{}

func (handler98) Handle(_ *Parser, fr Frame, env Env) (*Message, error) {
	payloadLen := fr.PayloadLen
	var subType int
	if payloadLen >= 1 {
		pl := fr.Payload()
		subType = int(pl[0])
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
		Version:       0,
		StationID:     0,
		IsLastInBurst: true,
	}, nil
}
