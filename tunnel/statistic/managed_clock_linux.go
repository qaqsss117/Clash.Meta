//go:build linux

package statistic

import (
	"golang.org/x/sys/unix"
	"time"
)

// CLOCK_BOOTTIME includes Android deep sleep and is independent of wall time.
func continuousNow() time.Duration {
	var value unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &value); err != nil {
		panic(err)
	}
	return time.Duration(value.Sec)*time.Second + time.Duration(value.Nsec)
}
