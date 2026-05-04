package parser

import (
	"fmt"
	"strings"
	"time"
	"timing-analyzer/internal/core"
	"timing-analyzer/internal/gsof"
)

// hexBytesSpaced returns each byte as two uppercase hex digits, separated by spaces (e.g. "1A 2B FF").
func hexBytesSpaced(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(b)*3 - 1)
	for i, v := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmt.Sprintf("%02X", v))
	}
	return sb.String()
}

// printGSOFSubRecordsVerbose2 logs each sub-record in a reassembled GSOF buffer: type, length byte, and full record bytes as spaced hex.
func printGSOFSubRecordsVerbose2(gsofBuffer []byte) {
	gsofBuffer = gsof.FlattenGSOFBuffer(gsofBuffer)
	ptr := 0
	for ptr < len(gsofBuffer) {
		if ptr+2 > len(gsofBuffer) {
			fmt.Printf("[VERBOSE2] GSOF sub-record truncated at offset %d (remainder %d bytes): %s\n",
				ptr, len(gsofBuffer)-ptr, hexBytesSpaced(gsofBuffer[ptr:]))
			return
		}
		recType := gsofBuffer[ptr]
		recLen := int(gsofBuffer[ptr+1])
		endIdx := ptr + 2 + recLen
		if endIdx > len(gsofBuffer) {
			fmt.Printf("[VERBOSE2] GSOF sub-record type=0x%02X len=%d incomplete (need %d bytes, %d remain): %s\n",
				recType, recLen, endIdx-ptr, len(gsofBuffer)-ptr, hexBytesSpaced(gsofBuffer[ptr:]))
			return
		}
		recBytes := gsofBuffer[ptr:endIdx]
		fmt.Printf("[VERBOSE2] GSOF sub-record type=0x%02X len=%d packet_hex=%s\n",
			recType, recLen, hexBytesSpaced(recBytes))
		ptr = endIdx
	}
}

// GSOFSession maintains the state of a multi-page GSOF message reassembly.
type GSOFSession struct {
	Data             []byte
	TotalPages       uint8
	PagesSeen        uint8
	ExpectedNextPage uint8
}

// maxDCOLBufWithoutSTX bounds how much we retain while waiting for 0x02 (STX). TCP can split
// a DCOL frame across reads so a chunk may contain no STX; discarding the buffer in that
// case loses bytes and breaks reassembly. UDP datagrams usually begin at STX, so this
// matters most for TCP streams.
const maxDCOLBufWithoutSTX = 1 << 20 // 1 MiB

type DCOLParser struct {
	buf                   []byte
	gsofAssembler         map[uint8]*GSOFSession // Map of transmission number to reassembly session
	synced                bool                   // true after at least one complete DCOL frame was emitted
	pendingStreamWarnings []string
}

func (p *DCOLParser) noteUndecodedAfterSync(n int, reason, remoteAddr string, verbose int) {
	if !p.synced || n <= 0 {
		return
	}
	msg := fmt.Sprintf("[%s] WARNING: %d undecoded byte(s) after DCOL sync (%s) remote=%s",
		time.Now().Format("15:04:05"), n, reason, remoteAddr)
	p.pendingStreamWarnings = append(p.pendingStreamWarnings, msg)
	if verbose >= 2 {
		fmt.Printf("[VERBOSE2] %s\n", msg)
	}
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
			if len(p.buf) > maxDCOLBufWithoutSTX {
				if verbose >= 1 {
					fmt.Printf("[WARN] DCOL buffer %d bytes without STX (0x02); discarding to resync.\n", len(p.buf))
				}
				p.noteUndecodedAfterSync(len(p.buf), "discarded without STX (buffer cap)", remoteAddr, verbose)
				p.buf = nil
			}
			return
		}
		if stxIdx > 0 {
			p.noteUndecodedAfterSync(stxIdx, "leading bytes before STX", remoteAddr, verbose)
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
			p.noteUndecodedAfterSync(1, "invalid ETX (slipped 1 byte)", remoteAddr, verbose)
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
			p.noteUndecodedAfterSync(1, "checksum mismatch (slipped 1 byte)", remoteAddr, verbose)
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
				if verbose >= 2 {
					printGSOFSubRecordsVerbose2(gsofBuffer)
				}
				if verbose >= 3 {
					fmt.Printf("[DEBUG] GSOF FULL BUFFER: %X\n", gsofBuffer)
					flat := gsof.FlattenGSOFBuffer(gsofBuffer)
					ptr := 0
					for ptr < len(flat)-1 {
						recType := flat[ptr]
						recLen := flat[ptr+1]
						endIdx := ptr + 2 + int(recLen)
						if endIdx > len(flat) {
							fmt.Printf("[DEBUG]   - SUB-MESSAGE OVERRUN: Type 0x%02X needs %d bytes, but only %d remain\n", recType, recLen, len(flat)-ptr-2)
							break
						}
						fmt.Printf("[DEBUG]   - SUB-MESSAGE: Type 0x%02X, Len %d, Data [%X]\n", recType, recLen, flat[ptr+2:endIdx])
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

		var gsofCopy []byte
		if len(gsofBuffer) > 0 {
			gsofCopy = append([]byte(nil), gsofBuffer...)
		}
		warnings := append([]string(nil), p.pendingStreamWarnings...)
		p.pendingStreamWarnings = p.pendingStreamWarnings[:0]
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
			GSOFBuffer:     gsofCopy,
			SequenceNumber: gsofSeq,
			StreamWarnings: warnings,
		}
		p.synced = true
		p.buf = p.buf[totalExpectedLen:]
	}
}
