package parser

import (
	"fmt"
	"time"
	"timing-analyzer/internal/core"
)

// GSOFSession maintains the state of a multi-page GSOF message reassembly.
type GSOFSession struct {
	Data             []byte
	TotalPages       uint8
	PagesSeen        uint8
	ExpectedNextPage uint8
}

type DCOLParser struct {
	buf           []byte
	gsofAssembler map[uint8]*GSOFSession // Map of transmission number to reassembly session
}

func (p *DCOLParser) Process(data []byte, bestTime, goTime, kernelTime time.Time, remoteAddr string, verbose int, out chan<- core.PacketEvent) {
	p.buf = append(p.buf, data...)

	for len(p.buf) >= 6 {
		stxIdx := -1
		for i, b := range p.buf {
			if b == 0x02 { // Start of Transmission
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
		if p.buf[totalExpectedLen-1] != 0x03 { // End of Transmission
			p.buf = p.buf[1:]
			continue
		}

		var csum byte = 0
		for i := 1; i < totalExpectedLen-2; i++ {
			csum += p.buf[i]
		}
		if csum != p.buf[totalExpectedLen-2] { // Checksum validation
			if verbose >= 1 {
				fmt.Printf("[WARN] Checksum failed for packet type 0x%02X\n", pktType)
			}
			p.buf = p.buf[1:]
			continue
		}

		// Verbose Debug: Raw DCOL Packet
		if verbose >= 3 {
			fmt.Printf("\n[DEBUG] DCOL RX: Type=0x%02X, Len=%d, Hex: %X\n", pktType, payloadLen, p.buf[:totalExpectedLen])
		}

		var isCMR bool
		var subType, version, stationID int
		isLastInBurst := true
		var gsofBuffer []byte
		var gsofSeq uint8

		// Handle Trimble GSOF (0x40) Multi-Page Transport
		if pktType == 0x40 {
			if payloadLen < 3 {
				p.buf = p.buf[totalExpectedLen:]
				continue
			}

			transmissionNum := p.buf[4] // Byte 4: Transmission number
			pageIndex := p.buf[5]       // Byte 5: Page Index
			maxPages := p.buf[6]        // Byte 6: Max Pages
			gsofSeq = transmissionNum

			if p.gsofAssembler == nil {
				p.gsofAssembler = make(map[uint8]*GSOFSession)
			}

			session, exists := p.gsofAssembler[transmissionNum]
			if !exists {
				session = &GSOFSession{
					TotalPages:       maxPages,
					Data:             make([]byte, 0, int(payloadLen)*int(maxPages+1)),
					ExpectedNextPage: 0,
				}
				p.gsofAssembler[transmissionNum] = session
			}

			if verbose >= 3 {
				fmt.Printf("[DEBUG] GSOF Frag: Seq=%d, Page=%d, Max=%d\n", transmissionNum, pageIndex, maxPages)
			}

			// Validate page order
			if pageIndex == session.ExpectedNextPage {
				// Append GSOF record data (skipping transport bytes 4, 5, 6)
				session.Data = append(session.Data, p.buf[7:totalExpectedLen-2]...)
				session.ExpectedNextPage++
			} else {
				if verbose >= 1 {
					fmt.Printf("[ERROR] GSOF Page Out of Order: Seq=%d, Got=%d, Expected=%d. Dropping.\n", transmissionNum, pageIndex, session.ExpectedNextPage)
				}
				delete(p.gsofAssembler, transmissionNum)
				p.buf = p.buf[totalExpectedLen:]
				continue
			}

			// Reassembly is complete only when the page index reaches maxPages
			if pageIndex == session.TotalPages {
				gsofBuffer = session.Data
				if verbose >= 3 {
					fmt.Printf("[DEBUG] GSOF FULL BUFFER: %X\n", gsofBuffer)
					ptr := 0
					for ptr < len(gsofBuffer)-1 {
						recType := gsofBuffer[ptr]
						recLen := gsofBuffer[ptr+1]
						endIdx := ptr + 2 + int(recLen)
						if endIdx > len(gsofBuffer) {
							fmt.Printf("[DEBUG]   - SUB-MESSAGE OVERRUN: Type 0x%02X needs %d bytes, but only %d remain\n", recType, recLen, len(gsofBuffer)-ptr-2)
							break
						}
						fmt.Printf("[DEBUG]   - SUB-MESSAGE: Type 0x%02X, Len %d, Data [%X]\n", recType, recLen, gsofBuffer[ptr+2:endIdx])
						ptr = endIdx
					}
				}
				delete(p.gsofAssembler, transmissionNum)
			} else {
				// Wait for more fragments
				p.buf = p.buf[totalExpectedLen:]
				continue
			}

		} else if pktType == 0x93 { // CMR logic
			isCMR = true
			if payloadLen >= 2 {
				firstByte := p.buf[4]
				secondByte := p.buf[5]
				version = int((firstByte >> 5) & 0x07)
				stationID = int(firstByte & 0x1F)
				subType = int((secondByte >> 5) & 0x07)
			}
		} else if pktType == 0x98 { // CMR logic
			isCMR = true
			if payloadLen >= 1 {
				firstByte := p.buf[4]
				version = 0
				stationID = 0
				subType = int(firstByte)
			}
		}

		out <- core.PacketEvent{
			BestTime:       bestTime,
			GoTime:         goTime,
			KernelTime:     kernelTime,
			Length:         totalExpectedLen,
			RemoteAddr:     remoteAddr,
			PacketType:     int(pktType),
			Decoded:        true,
			IsCMR:          isCMR,
			PacketSubType:  subType,
			Version:        version,
			StationID:      stationID,
			IsLastInBurst:  isLastInBurst,
			GSOFBuffer:     gsofBuffer,
			SequenceNumber: gsofSeq,
		}
		p.buf = p.buf[totalExpectedLen:]
	}
}
