//go:build windows

package stream

import (
	"errors"
	"net"
	"time"
)

func enableUDPKernelTimestamps(*net.UDPConn) bool { return false }

func readUDPWithOOB(*net.UDPConn, []byte, []byte) (int, *net.UDPAddr, time.Time, bool, error) {
	return 0, nil, time.Time{}, false, errors.New("udp readmsg not used on windows")
}
