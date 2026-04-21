package parser

import (
	"time"
	"timing-analyzer/internal/core"
)

type DCOLParser struct {
	buf []byte
}

func (p *DCOLParser) Process(data []byte, bestTime, goTime, kernelTime time.Time, remoteAddr string, verbose int, out chan<- core.PacketEvent) {
	p.buf = append(p.buf, data...)

	for len(p.buf) >= 6 {
		stxIdx := -1
		for i, b := range p.buf {
			if b == 0x02 {
				stxIdx = i
				break
			}
		}
		if stxIdx == -1 {
			p.buf = nil
			return
		}
		if stxIdx > 0 {
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
		if p.buf[totalExpectedLen-1] != 0x03 {
			p.buf = p.buf[1:]
			continue
		}

		var csum byte = 0
		for i := 1; i < totalExpectedLen-2; i++ {
			csum += p.buf[i]
		}
		if csum != p.buf[totalExpectedLen-2] {
			p.buf = p.buf[1:]
			continue
		}

		var isCMR bool
		var subType, version, stationID int
		isLastInBurst := true

		if pktType == 0x93 {
			isCMR = true
			if payloadLen >= 2 {
				firstByte := p.buf[4]
				secondByte := p.buf[5]
				version = int((firstByte >> 5) & 0x07)
				stationID = int(firstByte & 0x1F)
				subType = int((secondByte >> 5) & 0x07)
			}
		}
		if pktType == 0x98 {
			isCMR = true
			if payloadLen >= 1 {
				firstByte := p.buf[4]
				version = 0
				stationID = 0
				subType = int(firstByte)
			}
		}
		if pktType == 0x40 {
			if payloadLen >= 3 {
				pageIdx := p.buf[5]
				maxPageIdx := p.buf[6]
				isLastInBurst = (pageIdx == maxPageIdx)
			}
		}
		if pktType == 0x57 {
			if payloadLen >= 2 {
				pagingInfo := p.buf[5]
				pageIdx := (pagingInfo >> 4) & 0x0F
				maxPageIdx := pagingInfo & 0x0F
				isLastInBurst = (pageIdx == maxPageIdx)
			}
		}

		out <- core.PacketEvent{
			BestTime:      bestTime,
			GoTime:        goTime,
			KernelTime:    kernelTime,
			Length:        totalExpectedLen,
			RemoteAddr:    remoteAddr,
			PacketType:    int(pktType),
			Decoded:       true,
			IsCMR:         isCMR,
			PacketSubType: subType,
			Version:       version,
			StationID:     stationID,
			IsLastInBurst: isLastInBurst,
		}
		p.buf = p.buf[totalExpectedLen:]
	}
}

