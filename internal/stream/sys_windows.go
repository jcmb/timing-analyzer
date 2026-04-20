//go:build windows

package stream

import (
	"errors"
	"time"
)

func enableKernelTimestamps(fd uintptr) error {
	return errors.New("kernel timestamping not supported on windows")
}

func extractKernelTimestamp(oob []byte) (time.Time, bool) {
	return time.Time{}, false
}
