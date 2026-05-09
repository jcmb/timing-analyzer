package parser

import (
	"strings"
	"sync"
	"time"

	"github.com/gkirk/dcol"

	"timing-analyzer/internal/core"
)

// DCOLParser wraps the shared github.com/gkirk/dcol module with timing-analyzer core types.
type DCOLParser struct {
	// Registry, when non-nil, is used as-is for command dispatch (call dcol.RegisterPublic and any private Register from another module before first Process).
	// When nil, a registry with dcol.RegisterPublic is created automatically.
	Registry *dcol.Registry

	inner *dcol.Parser
	once  sync.Once
}

func (p *DCOLParser) parser() *dcol.Parser {
	p.once.Do(func() {
		reg := p.Registry
		if reg == nil {
			reg = dcol.NewRegistry()
			dcol.RegisterPublic(reg)
		}
		p.inner = dcol.NewParser(reg)
	})
	return p.inner
}

// Process feeds raw bytes into the DCOL framer and emits core.PacketEvent values on out.
func (p *DCOLParser) Process(data []byte, bestTime, goTime, kernelTime time.Time, remoteAddr string, cfg core.Config, out chan<- core.PacketEvent) {
	env := dcol.Env{
		Verbose:                       cfg.Verbose,
		RemoteAddr:                    remoteAddr,
		TransportIsUDP:                strings.EqualFold(cfg.IP, "udp"),
		IgnoreTCPGSOFTransmissionGap1: cfg.IgnoreTCPGSOFTransmissionGap1,
		BestTime:                      bestTime,
		GoTime:                        goTime,
		KernelTime:                    kernelTime,
		Logger:                        dcol.StdLogger{},
	}
	p.parser().Process(data, env, func(m dcol.Message) {
		out <- core.PacketEvent{
			BestTime:       m.BestTime,
			GoTime:         m.GoTime,
			KernelTime:     m.KernelTime,
			Length:         m.Length,
			RemoteAddr:     m.RemoteAddr,
			PacketType:     m.PacketType,
			Decoded:        m.Decoded,
			IsCMR:          m.IsCMR,
			PacketSubType:  m.PacketSubType,
			Version:        m.Version,
			StationID:      m.StationID,
			IsLastInBurst:  m.IsLastInBurst,
			GSOFBuffer:     m.GSOFBuffer,
			SequenceNumber: m.SequenceNumber,
			StreamWarnings: m.StreamWarnings,
		}
	})
}
