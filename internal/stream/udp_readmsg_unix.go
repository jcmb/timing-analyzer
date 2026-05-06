//go:build unix && !windows

package stream

import (
	"errors"
	"log/slog"
	"net"
	"time"
)

// enableUDPKernelTimestamps requests SO_TIMESTAMP ancillary data on this UDP socket (recvmsg path).
func enableUDPKernelTimestamps(conn *net.UDPConn) bool {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	var ctrlErr error
	err = raw.Control(func(fd uintptr) {
		ctrlErr = enableKernelTimestamps(fd)
	})
	if err != nil || ctrlErr != nil {
		if ctrlErr != nil {
			slog.Debug("UDP SO_TIMESTAMP not enabled", "error", ctrlErr)
		}
		return false
	}
	return true
}

// readUDPWithOOB reads one datagram with control messages for kernel RX time (SO_TIMESTAMP / SCM_TIMESTAMP).
func readUDPWithOOB(conn *net.UDPConn, buf, oob []byte) (n int, addr *net.UDPAddr, kernel time.Time, hasK bool, err error) {
	n, oobn, _, u, err := conn.ReadMsgUDP(buf, oob)
	if err != nil {
		return 0, nil, time.Time{}, false, err
	}
	if u == nil {
		return n, nil, time.Time{}, false, errors.New("udp: nil address")
	}
	if oobn > 0 && oobn <= len(oob) {
		kernel, hasK = extractKernelTimestamp(oob[:oobn])
	}
	return n, u, kernel, hasK, nil
}
