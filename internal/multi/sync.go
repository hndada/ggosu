package multi

import (
	"fmt"
	"sort"
	"time"
)

type sample struct {
	at, offset, rtt time.Duration
}

// Clock estimates the server's process-relative monotonic clock. Keep this
// value on the client's game goroutine; it never changes OS or game clocks.
type Clock struct {
	samples []sample
}

type Estimate struct {
	Offset      time.Duration // server minus client
	RTT         time.Duration
	Uncertainty time.Duration // conservative network uncertainty, not measured output lag
	Count       int
}

func (c *Clock) Observe(s Stamp, received time.Duration) error {
	t1, t2, t3, t4 := time.Duration(s.ClientSentNS), time.Duration(s.ServerRecvNS), time.Duration(s.ServerSentNS), received
	const limit = 365 * 24 * time.Hour
	for _, t := range []time.Duration{t1, t2, t3, t4} {
		if t < 0 || t > limit {
			return fmt.Errorf("multi: clock timestamp outside session range")
		}
	}
	if t4 < t1 || t3 < t2 {
		return fmt.Errorf("multi: clock timestamps out of order")
	}
	rtt := (t4 - t1) - (t3 - t2)
	if rtt < 0 || rtt > 2*time.Second {
		return fmt.Errorf("multi: clock sample delayed or inconsistent (rtt=%s t1=%d t2=%d t3=%d t4=%d)", rtt, t1, t2, t3, t4)
	}
	offset := ((t2 - t1) + (t3 - t4)) / 2
	if len(c.samples) == 16 {
		copy(c.samples, c.samples[1:])
		c.samples = c.samples[:15]
	}
	c.samples = append(c.samples, sample{t4, offset, rtt})
	return nil
}

func (c *Clock) Estimate(now time.Duration) (Estimate, bool) {
	var recent []sample
	for _, s := range c.samples {
		if now >= s.at && now-s.at <= 30*time.Second {
			recent = append(recent, s)
		}
	}
	if len(recent) < 3 {
		return Estimate{}, false
	}
	sort.Slice(recent, func(i, j int) bool { return recent[i].rtt < recent[j].rtt })
	best := recent[:min(3, len(recent))]
	offsets := []time.Duration{best[0].offset, best[1].offset, best[2].offset}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	return Estimate{Offset: offsets[1], RTT: recent[0].rtt,
		Uncertainty: best[len(best)-1].rtt/2 + offsets[2] - offsets[0], Count: len(recent)}, true
}

// LocalStart freezes the clock mapping for a match. A late announcement is
// refused instead of silently starting a game behind its opponents.
func (e Estimate) LocalStart(serverNS int64, now time.Duration) (time.Duration, error) {
	at := time.Duration(serverNS) - e.Offset
	if e.Uncertainty > 100*time.Millisecond || at-now < 100*time.Millisecond {
		return 0, fmt.Errorf("multi: insufficient synchronization or start lead time")
	}
	return at, nil
}
