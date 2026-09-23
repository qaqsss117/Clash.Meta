//go:build !linux && (!darwin || !cgo)

package statistic

import "time"

var continuousOrigin = time.Now()

func continuousNow() time.Duration { return time.Since(continuousOrigin) }
