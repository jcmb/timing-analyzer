package dcol

// stubHandler emits a generic Message with Payload copied for unknown DCOL command bytes.
type stubHandler struct{}

func (stubHandler) Handle(_ *Parser, fr Frame, env Env) (*Message, error) {
	payload := append([]byte(nil), fr.Payload()...)
	return &Message{
		BestTime:      env.BestTime,
		GoTime:        env.GoTime,
		KernelTime:    env.KernelTime,
		Length:        fr.TotalLen,
		RemoteAddr:    env.RemoteAddr,
		PacketType:    int(fr.Type),
		Decoded:       true,
		IsLastInBurst: true,
		Payload:       payload,
	}, nil
}
