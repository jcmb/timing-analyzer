package gsofstats

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestTCPListenTrackerGraceDrop(t *testing.T) {
	tr := NewTCPListenTracker()
	var closed atomic.Int32
	tr.Register("192.0.2.1:5018", func() error {
		closed.Add(1)
		return nil
	})
	tr.Tick(time.Now().Add(tcpInboundGSOFGrace + time.Millisecond))
	if closed.Load() != 1 {
		t.Fatalf("expected connection closed once, got %d", closed.Load())
	}
	pend, drops := tr.SnapshotForDashboard()
	if len(pend) != 0 {
		t.Fatalf("pending: got %d want 0", len(pend))
	}
	if len(drops) != 1 || drops[0].Receiver != "192.0.2.1" {
		t.Fatalf("drops: %+v", drops)
	}
}

func TestTCPListenTrackerNotifyClearsActive(t *testing.T) {
	tr := NewTCPListenTracker()
	tr.Register("192.0.2.2:9", func() error { return errors.New("x") })
	tr.NotifyGSOF("192.0.2.2:9")
	tr.Tick(time.Now().Add(tcpInboundGSOFGrace + time.Second))
	pend, drops := tr.SnapshotForDashboard()
	if len(pend) != 0 || len(drops) != 0 {
		t.Fatalf("want no pending/drops after GSOF notify, got pend=%d drops=%d", len(pend), len(drops))
	}
}

func TestTCPListenTrackerHistoryPrune(t *testing.T) {
	tr := NewTCPListenTracker()
	tr.Register("192.0.2.3:1", func() error { return nil })
	tr.Tick(time.Now().Add(tcpInboundGSOFGrace + time.Millisecond))
	_, drops := tr.SnapshotForDashboard()
	if len(drops) != 1 {
		t.Fatalf("want 1 drop")
	}
	tr.Tick(time.Now().Add(tcpNoGSOFHistoryRetention + time.Minute))
	_, drops2 := tr.SnapshotForDashboard()
	if len(drops2) != 0 {
		t.Fatalf("after retention want 0 drops, got %d", len(drops2))
	}
}
