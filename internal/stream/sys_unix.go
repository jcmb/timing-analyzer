//go:build !windows

package stream

import (
	"syscall"
	"time"
	"unsafe"
)

func enableKernelTimestamps(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_TIMESTAMP, 1)
}

func extractKernelTimestamp(oob []byte) (time.Time, bool) {
	cmsgs, err := syscall.ParseSocketControlMessage(oob)
	if err == nil {
		for _, m := range cmsgs {
			if m.Header.Level == syscall.SOL_SOCKET && (m.Header.Type == syscall.SCM_TIMESTAMP || m.Header.Type == syscall.SO_TIMESTAMP) {
				tv := (*syscall.Timeval)(unsafe.Pointer(&m.Data[0]))
				return time.Unix(int64(tv.Sec), int64(tv.Usec)*1000), true
			}
		}
	}
	return time.Time{}, false
}
