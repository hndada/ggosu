// Density summarizes chart heads and simultaneous holds on the 1x axis.
package chart

import (
	"slices"
	"time"
)

type DensityBin struct {
	Tap       int `json:"tap"`
	Hold      int `json:"hold"` // Hold and roll starts, counted once each.
	Lift      int `json:"lift"`
	Mine      int `json:"mine"`
	PeakHolds int `json:"peakHolds"`
}

type Density struct {
	Start float64      `json:"start"` // Seconds on the original 1x audio timeline.
	Step  float64      `json:"step"`  // Seconds per bin, at least 1; at most 600 bins.
	Bins  []DensityBin `json:"bins"`
	Lanes []int        `json:"lanes"` // Playable heads, excluding mines and fake notes.
	Total int          `json:"total"`
	Mines int          `json:"mines"`
}

func AnalyzeDensity(c *Chart) (Density, error) {
	if err := c.Validate(); err != nil {
		return Density{}, err
	}
	var start, end time.Duration
	for _, n := range c.Notes {
		if n.Kind != "fake" {
			start = min(start, n.At)
			end = max(end, n.End)
		}
	}
	step := max(time.Second, ((end-start)/time.Second+600)/600*time.Second)
	count := int((end-start)/step) + 1
	// The final head at an exact boundary belongs to a new bin.
	r := Density{Start: start.Seconds(), Step: step.Seconds(), Bins: make([]DensityBin, count), Lanes: make([]int, c.Lanes)}
	type event struct {
		at    time.Duration
		delta int
	}
	var events []event
	for _, n := range c.Notes {
		if n.Kind == "fake" {
			continue
		}
		b := &r.Bins[int((n.At-start)/step)]
		switch n.Kind {
		case "tap":
			b.Tap++
		case "lift":
			b.Lift++
		case "hold", "roll":
			b.Hold++
			events = append(events, event{n.At, 1}, event{n.End, -1})
		case "mine":
			b.Mine++
			r.Mines++
		}
		if n.Kind != "mine" {
			r.Lanes[n.Lane]++
			r.Total++
		}
	}
	slices.SortFunc(events, func(a, b event) int {
		if a.at < b.at {
			return -1
		}
		if a.at > b.at {
			return 1
		}
		return a.delta - b.delta
	})
	i, active := 0, 0
	for bin := range r.Bins {
		left, right := start+time.Duration(bin)*step, start+time.Duration(bin+1)*step
		for i < len(events) && events[i].at <= left {
			active += events[i].delta
			i++
		}
		peak := active
		for i < len(events) && events[i].at < right {
			at := events[i].at
			for i < len(events) && events[i].at == at {
				active += events[i].delta
				i++
			}
			peak = max(peak, active)
		}
		r.Bins[bin].PeakHolds = peak
	}
	return r, nil
}
