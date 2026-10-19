package websocket

import (
	"time"
)

const defaultStatsInterval = time.Minute

// Stats describes the stream at one moment.
type Stats struct {
	MissionID         string
	Connected         bool
	Resumable         bool   // the server supports resume
	Sent              uint64 // highest envelope written to the server
	Acked             uint64 // highest envelope the server acknowledged
	BufferedEnvelopes int    // envelopes held in memory
	BufferedBytes     int
	Evicted           uint64 // envelopes moved from memory to the spool
	Rejected          uint64 // envelopes refused because the buffer was full
	Spooling          bool   // the mission is kept on disk
	Reconnects        uint64 // replacement connections, since the process started
	Resumes           uint64 // reconnects that resumed the mission
	Restarts          uint64 // reconnects that found the mission lost by the server
	Recovered         uint64 // missions recovered from the spool
}

// Stats reports the current state of the stream.
func (b *Backend) Stats() Stats {
	b.mu.Lock()
	conn, missionID := b.conn, b.missionID
	b.mu.Unlock()

	entries, bytes, rejected := conn.log.stats()
	conn.mu.Lock()
	connected, resumable := conn.sock != nil, conn.resumable
	conn.mu.Unlock()
	sp := conn.log.currentSpool()
	return Stats{
		MissionID:         missionID,
		Connected:         connected,
		Resumable:         resumable,
		Sent:              conn.sent.Load(),
		Acked:             conn.acked.Load(),
		BufferedEnvelopes: entries,
		BufferedBytes:     bytes,
		Evicted:           conn.log.evictedCount(),
		Rejected:          rejected,
		Spooling:          sp != nil && sp.healthy(),
		Reconnects:        b.reconnects.Load() + conn.reconnects.Load(),
		Resumes:           b.resumes.Load() + conn.resumes.Load(),
		Restarts:          b.restarts.Load() + conn.restarts.Load(),
		Recovered:         b.recovered.Load(),
	}
}

// statsLoop logs the stream's state while a mission records, so operators
// see a struggling stream before the mission ends. A disconnected stream
// logs at warning level.
func (b *Backend) statsLoop(done <-chan struct{}) {
	interval := b.cfg.StatsInterval
	if interval <= 0 {
		interval = defaultStatsInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			b.logStats()
		}
	}
}

func (b *Backend) logStats() {
	s := b.Stats()
	if s.MissionID == "" {
		return
	}
	args := []any{
		"missionId", s.MissionID, "connected", s.Connected, "resumable", s.Resumable,
		"sent", s.Sent, "acked", s.Acked, "unacked", s.Sent - min(s.Acked, s.Sent),
		"bufferedEnvelopes", s.BufferedEnvelopes, "bufferedBytes", s.BufferedBytes,
		"evicted", s.Evicted, "rejected", s.Rejected, "spooling", s.Spooling,
		"reconnects", s.Reconnects, "resumes", s.Resumes, "restarts", s.Restarts,
	}
	if !s.Connected {
		b.logger().Warn("WebSocket stream disconnected, buffering", args...)
		return
	}
	b.logger().Info("WebSocket stream", args...)
}
