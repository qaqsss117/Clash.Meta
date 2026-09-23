package statistic

import (
	"sync"
	"time"
)

// ManagedMeter is separate from the display counters and only counts user data
// flowing through explicitly assigned imported outbounds. Latency probes do not
// pass through the inbound connection trackers.
type ManagedMeter struct {
	mu                 sync.Mutex
	tags               map[string]string
	totals             map[string][2]int64
	confirmed          map[string][2]int64
	deadline           time.Time
	remaining          int64
	required           bool
	reason             string
	deadlineReason     string
	generation         uint64
	continuousDeadline time.Duration
	controlHosts       map[string]bool
}

var Managed = &ManagedMeter{}

func (m *ManagedMeter) Require(required bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.required = required
}

func (m *ManagedMeter) Begin(tags map[string]string, remaining int64, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.required = true
	m.generation++
	m.tags = tags
	m.totals = make(map[string][2]int64)
	m.confirmed = make(map[string][2]int64)
	m.remaining = remaining
	m.deadline = time.Now().Add(ttl)
	m.continuousDeadline = continuousNow() + ttl
	m.reason = ""
	m.deadlineReason = "authorization_timeout"
}

func (m *ManagedMeter) DeadlineReason(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if reason == "expired" || reason == "period_changed" {
		m.deadlineReason = reason
	} else {
		m.deadlineReason = "authorization_timeout"
	}
}

func (m *ManagedMeter) Renew(remaining int64, ttl time.Duration, confirmed map[string][2]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// A stopped lease cannot be revived; recovery needs an online Begin.
	if !m.allowedLocked() {
		return
	}
	m.remaining = remaining
	m.confirmed = confirmed
	m.deadline = time.Now().Add(ttl)
	m.continuousDeadline = continuousNow() + ttl
}

func (m *ManagedMeter) Add(tag string, direction int, bytes int64) {
	m.AddGeneration(m.Generation(), tag, direction, bytes)
}

func (m *ManagedMeter) Generation() uint64 { m.mu.Lock(); defer m.mu.Unlock(); return m.generation }

func (m *ManagedMeter) AddGeneration(generation uint64, tag string, direction int, bytes int64) {
	if bytes <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if generation != m.generation {
		return
	}
	id, ok := m.tags[tag]
	if !ok {
		return
	}
	total := m.totals[id]
	total[direction] += bytes
	m.totals[id] = total
}

func (m *ManagedMeter) Stop(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deadline = time.Time{}
	m.reason = reason
}

func (m *ManagedMeter) Allowed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allowedLocked()
}

func (m *ManagedMeter) allowedLocked() bool {
	if !m.required {
		return true
	}
	if m.reason != "" {
		return false
	}
	if m.deadline.IsZero() || !time.Now().Before(m.deadline) || continuousNow() >= m.continuousDeadline {
		m.reason = m.deadlineReason
		if m.reason == "" {
			m.reason = "authorization_timeout"
		}
		return false
	}
	var pending int64
	for id, total := range m.totals {
		ack := m.confirmed[id]
		for direction := 0; direction < 2; direction++ {
			if total[direction] > ack[direction] {
				pending += total[direction] - ack[direction]
			}
		}
	}
	if pending >= m.remaining {
		m.reason = "quota_exhausted"
		return false
	}
	return true
}

func (m *ManagedMeter) ControlHosts(hosts []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.controlHosts == nil {
		m.controlHosts = make(map[string]bool)
	}
	for _, host := range hosts {
		m.controlHosts[host] = true
	}
}
func (m *ManagedMeter) IsControl(host string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.controlHosts[host]
}

func (m *ManagedMeter) Snapshot() (map[string][2]int64, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowedLocked()
	copy := make(map[string][2]int64, len(m.totals))
	for id, value := range m.totals {
		copy[id] = value
	}
	return copy, m.reason
}

func (m *ManagedMeter) Remaining() (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	remaining := m.remaining
	for id, total := range m.totals {
		ack := m.confirmed[id]
		for d := 0; d < 2; d++ {
			if total[d] > ack[d] {
				remaining -= total[d] - ack[d]
			}
		}
	}
	if remaining < 0 {
		remaining = 0
	}
	return remaining, m.required && !m.deadline.IsZero()
}
