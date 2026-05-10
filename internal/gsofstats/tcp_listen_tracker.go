package gsofstats

import (
	"log/slog"
	"net"
	"sync"
	"time"
)

const tcpInboundGSOFGrace = 5 * time.Second

const tcpNoGSOFHistoryRetention = 15 * time.Minute

// TCPInboundPendingRow is an inbound TCP peer still within the GSOF grace window.
type TCPInboundPendingRow struct {
	Receiver    string `json:"receiver"`
	RemoteAddr  string `json:"remote_addr"`
	ConnectedAt string `json:"connected_at_rfc3339"`
}

// TCPInboundNoGSOFRow is a peer dropped for receiving no GSOF DCOL payload within the grace period.
type TCPInboundNoGSOFRow struct {
	Receiver    string `json:"receiver"`
	RemoteAddr  string `json:"remote_addr"`
	ConnectedAt string `json:"connected_at_rfc3339"`
	DroppedAt   string `json:"dropped_at_rfc3339"`
}

// TCPListenTracker tracks inbound TCP connections (listen mode) until the first GSOF frame.
// Peers without a GSOF payload within tcpInboundGSOFGrace are closed and logged for the dashboard.
type TCPListenTracker struct {
	mu      sync.Mutex
	active  map[string]*tcpInboundActive
	history []tcpInboundHistory
}

type tcpInboundActive struct {
	started  time.Time
	receiver string
	close    func() error
}

type tcpInboundHistory struct {
	receiver   string
	remoteAddr string
	connected  time.Time
	dropped    time.Time
}

// NewTCPListenTracker constructs a tracker for one TCP listen session.
func NewTCPListenTracker() *TCPListenTracker {
	return &TCPListenTracker{
		active: make(map[string]*tcpInboundActive),
	}
}

func receiverFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// Register records a new inbound connection; closeConn should close the underlying net.Conn.
func (t *TCPListenTracker) Register(remoteAddr string, closeConn func() error) {
	if t == nil || remoteAddr == "" || closeConn == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active[remoteAddr] = &tcpInboundActive{
		started:  time.Now(),
		receiver: receiverFromRemoteAddr(remoteAddr),
		close:    closeConn,
	}
}

// NotifyGSOF marks the peer as having delivered at least one GSOF DCOL frame (0x40 payload).
func (t *TCPListenTracker) NotifyGSOF(remoteAddr string) {
	if t == nil || remoteAddr == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.active, remoteAddr)
}

// InboundClosed removes the peer from the grace watch (clean disconnect or external close).
func (t *TCPListenTracker) InboundClosed(remoteAddr string) {
	if t == nil || remoteAddr == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.active, remoteAddr)
}

// Tick closes peers that exceeded the GSOF grace period, appends history, and prunes old rows.
func (t *TCPListenTracker) Tick(now time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	var dropKeys []string
	for key, p := range t.active {
		if now.Sub(p.started) >= tcpInboundGSOFGrace {
			dropKeys = append(dropKeys, key)
		}
	}
	for _, key := range dropKeys {
		p := t.active[key]
		if p == nil {
			continue
		}
		if err := p.close(); err != nil {
			slog.Debug("TCP inbound close after no GSOF", "remote", key, "error", err)
		}
		t.history = append(t.history, tcpInboundHistory{
			receiver:   p.receiver,
			remoteAddr: key,
			connected:  p.started,
			dropped:    now,
		})
		slog.Info("TCP inbound dropped: no GSOF within grace period", "remote", key, "receiver", p.receiver)
		delete(t.active, key)
	}

	cut := now.Add(-tcpNoGSOFHistoryRetention)
	out := t.history[:0]
	for _, h := range t.history {
		if h.dropped.After(cut) {
			out = append(out, h)
		}
	}
	t.history = out
}

// SnapshotForDashboard returns pending peers and recent drops (newest drops first).
func (t *TCPListenTracker) SnapshotForDashboard() (pending []TCPInboundPendingRow, dropped []TCPInboundNoGSOFRow) {
	if t == nil {
		return nil, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	for remote, p := range t.active {
		pending = append(pending, TCPInboundPendingRow{
			Receiver:    p.receiver,
			RemoteAddr:  remote,
			ConnectedAt: p.started.Format(time.RFC3339Nano),
		})
	}

	for i := len(t.history) - 1; i >= 0; i-- {
		h := t.history[i]
		dropped = append(dropped, TCPInboundNoGSOFRow{
			Receiver:    h.receiver,
			RemoteAddr:  h.remoteAddr,
			ConnectedAt: h.connected.Format(time.RFC3339Nano),
			DroppedAt:   h.dropped.Format(time.RFC3339Nano),
		})
	}
	return pending, dropped
}
