package dcol

import (
	"fmt"
	"time"
)

// maxDCOLBufWithoutSTX bounds how much we retain while waiting for 0x02 (STX). TCP can split
// a DCOL frame across reads so a chunk may contain no STX; discarding the buffer in that
// case loses bytes and breaks reassembly. UDP datagrams usually begin at STX, so this
// matters most for TCP streams.
const maxDCOLBufWithoutSTX = 1 << 20 // 1 MiB

type gsofSession struct {
	Data             []byte
	TotalPages       uint8
	ExpectedNextPage uint8
}

// Parser buffers a byte stream, extracts DCOL frames, and dispatches to a Registry.
type Parser struct {
	buf                      []byte
	gsofAssembler            map[uint8]*gsofSession
	synced                   bool
	pendingStreamWarnings    []string
	lastCompletedGSOFXmit    uint8
	hasLastCompletedGSOFXmit bool
	reg                      *Registry
}

// NewParser constructs a parser that uses reg for command dispatch (unknown types use the stub handler).
func NewParser(reg *Registry) *Parser {
	return &Parser{reg: reg}
}

func (p *Parser) appendGSOFTransportWarning(msg string, env Env) {
	p.pendingStreamWarnings = append(p.pendingStreamWarnings, msg)
	env.logf(2, "[VERBOSE2] %s\n", msg)
}

func (p *Parser) noteUndecodedAfterSync(n int, reason, remoteAddr string, env Env) {
	if !p.synced || n <= 0 {
		return
	}
	msg := fmt.Sprintf("[%s] WARNING: %d undecoded byte(s) after DCOL sync (%s) remote=%s",
		time.Now().Format("15:04:05"), n, reason, remoteAddr)
	p.pendingStreamWarnings = append(p.pendingStreamWarnings, msg)
	env.logf(2, "[VERBOSE2] %s\n", msg)
}

// Process consumes data, emits at most one Message per frame via emit (may be zero calls if input is incomplete).
func (p *Parser) Process(data []byte, env Env, emit func(Message)) {
	if p.reg == nil {
		p.reg = NewRegistry()
		RegisterPublic(p.reg)
	}
	p.buf = append(p.buf, data...)

	for len(p.buf) >= 6 {
		stxIdx := -1
		for i, b := range p.buf {
			if b == 0x02 { // STX
				stxIdx = i
				break
			}
		}
		if stxIdx == -1 {
			if len(p.buf) > maxDCOLBufWithoutSTX {
				env.logf(1, "[WARN] DCOL buffer %d bytes without STX (0x02); discarding to resync.\n", len(p.buf))
				p.noteUndecodedAfterSync(len(p.buf), "discarded without STX (buffer cap)", env.RemoteAddr, env)
				p.buf = nil
			}
			return
		}
		if stxIdx > 0 {
			p.noteUndecodedAfterSync(stxIdx, "leading bytes before STX", env.RemoteAddr, env)
			p.buf = p.buf[stxIdx:]
		}
		if len(p.buf) < 6 {
			return
		}

		pktType := p.buf[2]
		payloadLen := p.buf[3]
		totalExpectedLen := int(payloadLen) + 6

		if len(p.buf) < totalExpectedLen {
			return
		}
		if p.buf[totalExpectedLen-1] != 0x03 { // ETX
			p.noteUndecodedAfterSync(1, "invalid ETX (slipped 1 byte)", env.RemoteAddr, env)
			p.buf = p.buf[1:]
			continue
		}

		var csum byte
		for i := 1; i < totalExpectedLen-2; i++ {
			csum += p.buf[i]
		}
		if csum != p.buf[totalExpectedLen-2] {
			env.logf(1, "[WARN] Checksum failed for packet type 0x%02X\n", pktType)
			p.noteUndecodedAfterSync(1, "checksum mismatch (slipped 1 byte)", env.RemoteAddr, env)
			p.buf = p.buf[1:]
			continue
		}

		if env.Verbose >= 3 && env.Logger != nil {
			env.Logger.Printf("\n[DEBUG] DCOL RX: Type=0x%02X, Len=%d, Hex: %X\n", pktType, payloadLen, p.buf[:totalExpectedLen])
		}

		fr := Frame{
			Type:       pktType,
			PayloadLen: payloadLen,
			TotalLen:   totalExpectedLen,
			Raw:        p.buf[:totalExpectedLen],
		}

		h := p.reg.lookup(pktType)
		msg, err := h.Handle(p, fr, env)
		if err != nil {
			env.logf(1, "[WARN] DCOL handler 0x%02X: %v\n", pktType, err)
			p.buf = p.buf[totalExpectedLen:]
			continue
		}
		if msg != nil {
			warnings := append([]string(nil), p.pendingStreamWarnings...)
			p.pendingStreamWarnings = p.pendingStreamWarnings[:0]
			msg.StreamWarnings = warnings
			emit(*msg)
			p.synced = true
		}

		p.buf = p.buf[totalExpectedLen:]
	}
}
