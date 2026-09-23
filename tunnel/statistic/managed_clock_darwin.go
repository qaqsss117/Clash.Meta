//go:build darwin && cgo

package statistic

/*
#include <mach/mach_time.h>
static double managed_continuous_ns(void) {
    mach_timebase_info_data_t info;
    mach_timebase_info(&info);
    return (double)mach_continuous_time() * info.numer / info.denom;
}
*/
import "C"
import "time"

func continuousNow() time.Duration { return time.Duration(C.managed_continuous_ns()) }
