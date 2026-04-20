//go:build windows

package main

import (
	"errors"
	"time"
)

// enableKernelTimestamps fails silently on Windows since it's unsupported
func enableKernelTimestamps(fd uintptr) error {
	return errors.New("kernel timestamping not supported on windows")
}

// extractKernelTimestamp simply returns false on Windows
func extractKernelTimestamp(oob []byte) (time.Time, bool) {
	return time.Time{}, false
}
