package core

import "time"

type Config struct {
	IP            string
	Host          string
	Port          int
	WebPort       int
	RateHz        float64
	JitterVal     float64
	JitterPct     bool
	TimeoutExit   bool
	Verbose       int
	WarmupPackets int
	Decode        string
	CSVFile       string

	// New fields for NTRIP and Multi-Tenant Sessions
	Mountpoint string
	Username   string
	Password   string
	SessionID  string

	// IgnoreTCPGSOFTransmissionGap1, when true on a TCP stream, suppresses warnings for
	// exactly one missing GSOF transmission number between successive messages (some senders
	// skip one counter in multi-page bursts). Larger gaps still warn. UDP behavior is unchanged.
	IgnoreTCPGSOFTransmissionGap1 bool
}

type PacketEvent struct {
	BestTime      time.Time
	GoTime        time.Time
	KernelTime    time.Time
	Length        int
	RemoteAddr    string
	PacketType    int
	Decoded       bool
	IsCMR         bool
	PacketSubType int
	Version       int
	StationID     int
	IsLastInBurst bool
	GSOFBuffer    []byte // The reassembled multi-page payload
	// SequenceNumber is the GSOF 0x40 transmission number (payload byte 4) when
	// PacketType is 0x40; otherwise zero. It is not a monotonic transport sequence.
	SequenceNumber uint8
	// StreamWarnings are parser-side issues (e.g. undecoded bytes after sync) to merge into stats.
	StreamWarnings []string `json:"-"`
}

type TelemetryEvent struct {
	Timestamp     string `json:"timestamp"`
	DisplayKey    string `json:"display_key"`
	Count         uint64 `json:"count"`
	ActualDeltaMs int64  `json:"actual_delta_ms"`
	ExpectedMs    int64  `json:"expected_ms"`
	OSDelayUs     int64  `json:"os_delay_us"`
	IsUDP         bool   `json:"is_udp"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type LogEntry struct {
	Level         string
	Time          time.Time
	Event         string
	DisplayKey    string
	Count         uint64
	Delta         int64
	Expected      int64
	IP            string
	Length        int
	IsUDP         bool
	HasKernelTime bool
	OSDelayUs     int64
	IsCMR         bool
	PktType       int
	CMRVer        int
	StationID     int
	HasAdj        bool
	AdjDelta      int64
	MissedPackets int
	Message       string
	PrintConsole  bool
	WriteCSV      bool
}

func GetNiceName(displayKey string) string {
	switch displayKey {
	case "0x93-0":
		return "CMR GPS"
	case "0x93-1":
		return "CMR Base LLH"
	case "0x93-2":
		return "CMR Base Name"
	case "0x93-3":
		return "CMR GLN-STD"
	case "0x93-4":
		return "GPS Delta"
	case "0x94":
		return "CMR+ Base"
	case "0x98-0":
		return "CMR GLONASS"
	case "0x98-1":
		return "CMR Time"
	case "0x98-4":
		return "GLN Delta"
	case "0x40":
		return "GSOF"
	case "0x57":
		return "RAWDATA"
	default:
		return displayKey
	}
}
