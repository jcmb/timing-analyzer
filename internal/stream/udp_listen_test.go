package stream

import (
	"net"
	"testing"

	"timing-analyzer/internal/core"
)

func TestUDPListenBindAddr(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"", "0.0.0.0"},
		{"172.27.0.14", "172.27.0.14"},
		{"255.255.255.255", "0.0.0.0"},
		{"172.27.0.255", "0.0.0.0"},
	}
	for _, tc := range tests {
		got := udpListenBindAddr(core.Config{Host: tc.host})
		if got != tc.want {
			t.Fatalf("host %q: want %q got %q", tc.host, tc.want, got)
		}
	}
}

func TestPrepareUDPListenConn_broadcastRX(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := prepareUDPListenConn(conn); err != nil {
		t.Fatalf("prepareUDPListenConn: %v", err)
	}
}
