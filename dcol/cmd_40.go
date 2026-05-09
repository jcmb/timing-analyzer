package dcol

import (
	"fmt"
	"time"
)

// handler40 reassembles Trimble GSOF transported as DCOL packet type 0x40 (multi-page).
type handler40 struct{}

func (handler40) Handle(p *Parser, fr Frame, env Env) (*Message, error) {
	payloadLen := fr.PayloadLen
	totalExpectedLen := fr.TotalLen
	raw := fr.Raw

	if payloadLen < 3 {
		return nil, nil
	}

	transmissionNum := raw[4]
	pageIndex := raw[5]
	maxPages := raw[6]
	gsofSeq := transmissionNum

	if p.gsofAssembler == nil {
		p.gsofAssembler = make(map[uint8]*gsofSession)
	}

	if sess, ok := p.gsofAssembler[transmissionNum]; ok && sess.ExpectedNextPage > 0 && pageIndex == 0 {
		env.logf(1, "[WARN] GSOF transmission %d: new page 0 while page %d was still expected; restarting assembly\n",
			transmissionNum, sess.ExpectedNextPage)
		delete(p.gsofAssembler, transmissionNum)
	}

	if pageIndex == 0 && p.hasLastCompletedGSOFXmit {
		d := (int(transmissionNum) - int(p.lastCompletedGSOFXmit) + 256) % 256
		if d > 1 {
			missed := d - 1
			tcp := !env.TransportIsUDP
			if tcp && env.IgnoreTCPGSOFTransmissionGap1 && missed == 1 {
				// suppress
			} else {
				msg := fmt.Sprintf("[%s] WARNING: GSOF transmission gap: ~%d missed id(s) between xmit %d and %d",
					time.Now().Format("15:04:05"), missed, p.lastCompletedGSOFXmit, transmissionNum)
				p.appendGSOFTransportWarning(msg, env)
			}
		}
	}

	session, exists := p.gsofAssembler[transmissionNum]
	if !exists {
		session = &gsofSession{
			TotalPages:       maxPages,
			Data:             make([]byte, 0, int(payloadLen)*int(maxPages+1)),
			ExpectedNextPage: 0,
		}
		p.gsofAssembler[transmissionNum] = session
	}

	env.logf(3, "[DEBUG] GSOF Frag: Seq=%d, Page=%d, Max=%d\n", transmissionNum, pageIndex, maxPages)

	if pageIndex < session.ExpectedNextPage {
		env.logf(1, "[WARN] GSOF duplicate or late page: xmit=%d page=%d (already past); dropping frame\n",
			transmissionNum, pageIndex)
		return nil, nil
	}
	if pageIndex > session.ExpectedNextPage {
		env.logf(1, "[WARN] GSOF missed page(s): xmit=%d got page=%d expected=%d; dropping partial assembly\n",
			transmissionNum, pageIndex, session.ExpectedNextPage)
		msg := fmt.Sprintf("[%s] WARNING: GSOF multi-page gap: transmission %d expected page %d, got %d (dropped partial)",
			time.Now().Format("15:04:05"), transmissionNum, session.ExpectedNextPage, pageIndex)
		p.appendGSOFTransportWarning(msg, env)
		delete(p.gsofAssembler, transmissionNum)
		return nil, nil
	}

	session.Data = append(session.Data, raw[7:totalExpectedLen-2]...)
	session.ExpectedNextPage++

	if pageIndex != session.TotalPages {
		return nil, nil
	}

	gsofBuffer := session.Data
	p.lastCompletedGSOFXmit = transmissionNum
	p.hasLastCompletedGSOFXmit = true
	if env.Verbose >= 2 {
		debugPrintGSOFSubrecords(env, gsofBuffer)
	}
	if env.Verbose >= 3 && env.Logger != nil {
		env.Logger.Printf("[DEBUG] GSOF FULL BUFFER: %X\n", gsofBuffer)
		debugPrintFlattenedSubmessages(env, gsofBuffer)
	}
	delete(p.gsofAssembler, transmissionNum)

	gsofCopy := append([]byte(nil), gsofBuffer...)
	return &Message{
		BestTime:       env.BestTime,
		GoTime:         env.GoTime,
		KernelTime:     env.KernelTime,
		Length:         totalExpectedLen,
		RemoteAddr:     env.RemoteAddr,
		PacketType:     int(fr.Type),
		Decoded:        true,
		IsLastInBurst:  true,
		GSOFBuffer:     gsofCopy,
		SequenceNumber: gsofSeq,
	}, nil
}
