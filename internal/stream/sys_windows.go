//go:build windows

package stream

import (
	"errors"
	"syscall"
	"time"
)

func enableKernelTimestamps(fd uintptr) error {
	return errors.New("kernel timestamping not supported on windows")
}

func enableUDPListenSocket(fd uintptr) error {
	if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}

func extractKernelTimestamp(oob []byte) (time.Time, bool) {
	return time.Time{}, false
}
