package dcol

import (
	"fmt"
	"time"
)

// Logger receives verbose parser diagnostics (optional).
type Logger interface {
	Printf(format string, args ...interface{})
}

// StdLogger prints via fmt.Printf, matching historical timing-analyzer DCOL behavior.
type StdLogger struct{}

func (StdLogger) Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Env carries per-chunk decode context; keep it free of application-specific types so other repos can depend only on this module.
type Env struct {
	Verbose int
	// RemoteAddr is an opaque endpoint label (e.g. from net.Addr.String()).
	RemoteAddr string
	// TransportIsUDP is true when the stream is UDP (affects GSOF transmission-gap warnings).
	TransportIsUDP bool
	// IgnoreTCPGSOFTransmissionGap1 mirrors timing-analyzer core.Config: on TCP, suppress gap warning when exactly one transmission id was skipped.
	IgnoreTCPGSOFTransmissionGap1 bool

	BestTime   time.Time
	GoTime     time.Time
	KernelTime time.Time

	Logger Logger
}

func (e Env) logf(verboseMin int, format string, args ...interface{}) {
	if e.Verbose < verboseMin || e.Logger == nil {
		return
	}
	e.Logger.Printf(format, args...)
}
