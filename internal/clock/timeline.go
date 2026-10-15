package clock

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type segment struct {
	at, position time.Duration
	rate         float64
}

// Timeline maps shared counter timestamps to positions on the game's 1x time
// axis. Rate changes how fast that position advances, not its time base. A rate
// of zero pauses time. Keep it on the game goroutine; Counter remains the receiver's clock.
// The zero value is paused at game time zero.
type Timeline struct {
	segments []segment
}

func NewTimeline(at time.Duration) *Timeline {
	return &Timeline{segments: []segment{{at: at, rate: 1}}}
}

// Position uses the rate in effect at at, including for events consumed after
// a pause or speed change. Queries do not advance or modify the timeline.
func (t *Timeline) Position(at time.Duration) time.Duration {
	i := sort.Search(len(t.segments), func(i int) bool { return t.segments[i].at > at }) - 1
	if i < 0 {
		return 0
	}
	s := t.segments[i]
	delta := float64(at-s.at) * s.rate
	if delta >= float64(math.MaxInt64-int64(s.position)) {
		return time.Duration(math.MaxInt64)
	}
	return s.position + time.Duration(delta)
}

func (t *Timeline) Rate() float64 {
	if len(t.segments) == 0 {
		return 0
	}
	return t.segments[len(t.segments)-1].rate
}

// SetRate starts a continuous segment at at. Changes must be chronological.
// Reverse playback and seeking require a new timeline and game state.
func (t *Timeline) SetRate(at time.Duration, rate float64) error {
	if at < 0 || rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return fmt.Errorf("clock: invalid timestamp or playback rate")
	}
	if n := len(t.segments); n > 0 && at < t.segments[n-1].at {
		return fmt.Errorf("clock: rate change precedes the last change")
	}
	if rate != t.Rate() {
		t.segments = append(t.segments, segment{at: at, position: t.Position(at), rate: rate})
	}
	return nil
}
