package statistic

import (
	"testing"
	"time"
)

func TestManagedMeterCountsOnlyAssignedOutboundsAndSurvivesDisplayReset(t *testing.T) {
	old := Managed
	defer func() { Managed = old }()
	Managed = &ManagedMeter{}
	Managed.Begin(map[string]string{"upstream-7": "7"}, 1000, time.Minute)
	DefaultManager.PushUploaded("DIRECT", 400, Managed.Generation())
	DefaultManager.PushDownloaded("self-hosted", 500, Managed.Generation())
	DefaultManager.PushUploaded("upstream-7", 100, Managed.Generation())
	DefaultManager.PushDownloaded("upstream-7", 200, Managed.Generation())
	DefaultManager.ResetStatistic()
	totals, reason := Managed.Snapshot()
	if reason != "" || len(totals) != 1 || totals["7"] != [2]int64{100, 200} {
		t.Fatalf("wrong counters: %v %s", totals, reason)
	}
}

func TestManagedQuotaDeductsOnlyUnconfirmedBytes(t *testing.T) {
	m := &ManagedMeter{}
	m.Begin(map[string]string{"upstream-1": "1"}, 100, time.Minute)
	m.Add("upstream-1", 0, 60)
	m.Renew(40, time.Minute, map[string][2]int64{"1": {60, 0}})
	if !m.Allowed() {
		t.Fatal("acknowledged bytes charged twice")
	}
	m.Add("upstream-1", 1, 40)
	if m.Allowed() {
		t.Fatal("exhausted quota still authorized")
	}
}

func TestManagedExpiryAndRestartFailClosed(t *testing.T) {
	m := &ManagedMeter{}
	m.Require(true)
	if m.Allowed() {
		t.Fatal("cold start authorized without online lease")
	}
	m.Begin(map[string]string{}, 100, -time.Second)
	if m.Allowed() {
		t.Fatal("expired lease authorized")
	}
	m.Renew(100, time.Minute, nil)
	if m.Allowed() {
		t.Fatal("stopped lease revived by delayed response")
	}
	m.Begin(nil, 100, time.Minute)
	if !m.Allowed() {
		t.Fatal("online start denied")
	}
}

func TestManagedContinuousDeadlineIncludesSuspendedTime(t *testing.T) {
	m := &ManagedMeter{}
	m.Begin(nil, 100, time.Minute)
	m.continuousDeadline = continuousNow() - time.Second
	if m.Allowed() {
		t.Fatal("suspended time extended lease")
	}
	m.Renew(100, time.Minute, nil)
	if m.Allowed() {
		t.Fatal("late response revived deadline")
	}
}
