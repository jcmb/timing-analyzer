package stream

import (
	"fmt"
	"net"
	"strings"

	"timing-analyzer/internal/core"
)

func prepareUDPListenConn(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var ctrlErr error
	err = raw.Control(func(fd uintptr) {
		ctrlErr = enableUDPListenSocket(fd)
	})
	if err != nil {
		return err
	}
	return ctrlErr
}

// udpListenBindAddr returns the IPv4 bind address for UDP listen (udp4).
// Empty or broadcast/multicast host values bind all interfaces (0.0.0.0).
func udpListenBindAddr(cfg core.Config) string {
	h := strings.TrimSpace(cfg.Host)
	if h == "" {
		return "0.0.0.0"
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return "0.0.0.0"
	}
	if ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return "0.0.0.0"
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[3] == 255 {
		return "0.0.0.0"
	}
	return h
}

func udpListenAddress(cfg core.Config) (string, error) {
	bind := udpListenBindAddr(cfg)
	if cfg.Port < 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("udp port must be 0-65535")
	}
	return fmt.Sprintf("%s:%d", bind, cfg.Port), nil
}
