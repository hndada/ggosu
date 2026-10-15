package main

import (
	"fmt"
	"time"

	"ggosu/internal/keyboard"
)

type loadResult struct {
	Sent     uint64        `json:"posted"`
	Elapsed  time.Duration `json:"elapsed_ns"`
	Rate     float64       `json:"actual_posts_per_second"`
	Error    string        `json:"error,omitempty"`
	finished time.Time
}

// load posts batches paced at 1ms. This tests event throughput and the Windows
// message-loop handoff, not evenly spaced 125us hardware reports.
func load(stop <-chan struct{}, src *keyboard.Source, rate int, duration time.Duration) loadResult {
	start := src.Now()
	target := uint64(float64(rate) * duration.Seconds())
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	var r loadResult
	keys := [...]uint16{'D', 'F', 'J', 'K'}
	for r.Sent < target {
		select {
		case <-stop:
			r.Error = "load stopped"
			return r
		case <-ticker.C:
		}
		due := min(target, uint64(float64(rate)*(src.Now()-start).Seconds()))
		for r.Sent < due {
			key := keys[(r.Sent/2)%uint64(len(keys))]
			if err := src.Post(key, r.Sent%2 == 0); err != nil {
				r.Error = fmt.Sprintf("event %d: %v", r.Sent+1, err)
				return r
			}
			r.Sent++
		}
	}
	r.Elapsed = src.Now() - start
	r.Rate = float64(r.Sent) / r.Elapsed.Seconds()
	r.finished = time.Now()
	return r
}

func (g *game) startLoad() {
	r := g.box.report()
	g.baseReceived, g.baseHandled = r.Synthetic, g.synthHandled
	g.testStarted, g.testFinished, g.testPassed = true, false, false
	g.loadResult = nil
	g.loadDone = make(chan loadResult, 1)
	// Pass the channel and settings as values: the renderer may start another
	// completed test later, while the producer owns only this run's values.
	done, rate, duration := g.loadDone, g.opts.rate, g.opts.duration
	go func() { done <- load(g.stop, g.source, rate, duration) }()
}
