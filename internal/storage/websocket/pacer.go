package websocket

import (
	"time"
)

const (
	defaultResendBytesPerSec = 8 << 20
	pacerMaxBurstBytes       = 1 << 20 // at most this much is sent without waiting
)

// pacer spaces writes to a byte rate. It paces catch-up traffic (a backlog
// resent after a reconnect, a whole mission resent from the spool, missions
// recovered after a crash) so that many game servers recovering at once do
// not flood the web server. Live traffic is not paced.
type pacer struct {
	rate  float64 // bytes per second; <= 0 disables pacing
	burst float64 // bytes sent without waiting at the start
	start time.Time
	sent  float64
}

func newPacer(bytesPerSec int) *pacer {
	rate := float64(bytesPerSec)
	return &pacer{rate: rate, burst: min(pacerMaxBurstBytes, rate/4), start: time.Now()}
}

// wait blocks until n more bytes fit the rate. It returns false if done is
// closed first.
func (p *pacer) wait(n int, done <-chan struct{}, dead <-chan struct{}) bool {
	if p == nil || p.rate <= 0 {
		return true
	}
	p.sent += float64(n)
	allowed := time.Since(p.start).Seconds()*p.rate + p.burst
	if p.sent <= allowed {
		return true
	}
	delay := time.Duration((p.sent - allowed) / p.rate * float64(time.Second))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-done:
		return false
	case <-dead:
		return false
	}
}
