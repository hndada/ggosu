package main

import (
	"fmt"
	"math"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

// The first six trials and final chord trial observe 500..5500ms, excluding
// LN heads/tails. All use 24 tap onsets. The remaining trials measure all work.
func compareHolds(o difficulty.Options, p difficulty.Profile) ([]trial, error) {
	if len(p.Fingers) != 6 {
		return nil, fmt.Errorf("hold suite requires six fixed fingers")
	}
	ms := time.Millisecond
	cases := []struct {
		name   string
		chart  *chart.Chart
		middle bool
	}{
		{"tap A + taps B", heldTaps(nil, [][]int{{1}}, 200*ms), true},
		{"LN A + taps B", heldTaps([]int{0}, [][]int{{1}}, 200*ms), true},
		{"LN A + taps D", heldTaps([]int{0}, [][]int{{3}}, 200*ms), true},
		{"LN A + alternating BC", heldTaps([]int{0}, [][]int{{1}, {2}}, 200*ms), true},
		{"LN AC + taps B", heldTaps([]int{0, 2}, [][]int{{1}}, 200*ms), true},
		{"LN AD + taps B", heldTaps([]int{0, 3}, [][]int{{1}}, 200*ms), true},
		{"LN A / 1s", holdTail(time.Second, 0, 0), false},
		{"LN A / 4s", holdTail(4*time.Second, 0, 0), false},
		{"LN A -> A / 200ms", holdTail(time.Second, 200*ms, 0), false},
		{"LN A -> A / 40ms", holdTail(time.Second, 40*ms, 0), false},
		{"LN A -> B / overlap", holdTail(time.Second, 25*ms, 1), false},
		{"LN A + chord BC", heldTaps([]int{0}, [][]int{{1, 2}}, 400*ms), true},
	}
	var trials []trial
	for _, spec := range cases {
		labelChart(spec.chart, p, spec.name)
		r, err := difficulty.Analyze(spec.chart, p, o)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.name, err)
		}
		m := fullMeasurement(spec.chart, r)
		if spec.middle {
			start := time.Duration(math.Round(float64(500*ms) / o.Rate))
			end := time.Duration(math.Round(float64(5500*ms) / o.Rate))
			m = measure(spec.chart, r, start, end)
		}
		trials = append(trials, trial{Name: spec.name, Chart: spec.chart, Region: m, Report: r})
	}
	return trials, nil
}

func heldTaps(held []int, keys [][]int, period time.Duration) *chart.Chart {
	c := chart.New(6)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	if len(held) == 0 {
		c.Notes = append(c.Notes, chart.Note{Lane: 0, Kind: "tap"})
	}
	for _, lane := range held {
		c.Notes = append(c.Notes, chart.Note{At: 0, End: 6 * time.Second, Lane: lane, Kind: "hold"})
	}
	for step, at := 0, 600*time.Millisecond; at < 5400*time.Millisecond; step, at = step+1, at+period {
		for _, lane := range keys[step%len(keys)] {
			c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: "tap"})
		}
	}
	c.Normalize()
	return c
}

func holdTail(end, gap time.Duration, lane int) *chart.Chart {
	c := chart.New(6)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 0, End: end, Lane: 0, Kind: "hold"}}
	if gap > 0 {
		at := end + gap
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: "tap"})
	}
	c.Normalize()
	return c
}

// Match the original window grid, including initial preparation and final
// return. Whole-chart cases therefore reproduce Report.Difficulty exactly.
func fullMeasurement(c *chart.Chart, r *difficulty.Report) *measurement {
	if len(r.Segments) == 0 {
		return nil // No cost interval; the whole-chart report still has zero totals.
	}
	start, end := r.Segments[0].At-r.Origin, r.Segments[len(r.Segments)-1].End-r.Origin
	lo := start / r.Options.Window
	if start%r.Options.Window < 0 {
		lo--
	}
	hi := end / r.Options.Window
	if end%r.Options.Window > 0 {
		hi++
	}
	return measure(c, r, r.Origin+lo*r.Options.Window, r.Origin+hi*r.Options.Window)
}
