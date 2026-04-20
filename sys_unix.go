//go:build !windows

package main

import (
	"syscall"
	"time"
	"unsafe"
)

// enableKernelTimestamps requests the OS kernel to attach timestamps to UDP packets
func enableKernelTimestamps(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_TIMESTAMP, 1)
}

// extractKernelTimestamp parses the Out-Of-Band (OOB) data for the SCM_TIMESTAMP
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
