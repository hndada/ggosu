package main

import (
	"fmt"
	"math"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

// All chord trials share a six-second observation interval after preparation.
// A/B/C and D/E/F are the two hands in the example profile, not anatomical IDs.
func compareChords(o difficulty.Options, p difficulty.Profile) ([]trial, error) {
	if len(p.Fingers) != 6 {
		return nil, fmt.Errorf("chord suite requires six fixed fingers")
	}
	cases := []struct {
		name   string
		period time.Duration
		keys   [][]int
	}{
		{"AB / 400ms", 400 * time.Millisecond, [][]int{{0, 1}}},
		{"AB / 200ms", 200 * time.Millisecond, [][]int{{0, 1}}},
		{"AC / 200ms", 200 * time.Millisecond, [][]int{{0, 2}}},
		{"AD / 200ms", 200 * time.Millisecond, [][]int{{0, 3}}},
		{"AB-AC / 200ms", 200 * time.Millisecond, [][]int{{0, 1}, {0, 2}}},
		{"AB-DE / 200ms", 200 * time.Millisecond, [][]int{{0, 1}, {3, 4}}},
		{"AB / 100ms", 100 * time.Millisecond, [][]int{{0, 1}}},
		{"ABC / 200ms", 200 * time.Millisecond, [][]int{{0, 1, 2}}},
		{"ABD / 200ms", 200 * time.Millisecond, [][]int{{0, 1, 3}}},
		{"ABC / 300ms", 300 * time.Millisecond, [][]int{{0, 1, 2}}},
	}
	start := time.Duration(math.Round(float64(600*time.Millisecond) / o.Rate))
	end := time.Duration(math.Round(float64(6600*time.Millisecond) / o.Rate))
	var trials []trial
	for _, spec := range cases {
		c := chordChart(spec.period, spec.keys)
		labelChart(c, p, spec.name)
		r, err := difficulty.Analyze(c, p, o)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.name, err)
		}
		trials = append(trials, trial{Name: spec.name, Chart: c, Region: measure(c, r, start, end), Report: r})
	}
	return trials, nil
}

func chordChart(period time.Duration, keys [][]int) *chart.Chart {
	return chordCycles(period, keys, int(7200*time.Millisecond/period))
}

func chordCycles(period time.Duration, keys [][]int, cycles int) *chart.Chart {
	c := chart.New(6)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for step, at := 0, time.Duration(0); step < cycles; step, at = step+1, at+period {
		for _, lane := range keys[step%len(keys)] {
			c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: "tap"})
		}
	}
	c.Normalize()
	return c
}

func compareRecovery(o difficulty.Options, p difficulty.Profile) ([]trial, error) {
	if len(p.Fingers) != 6 {
		return nil, fmt.Errorf("recovery suite requires six fixed fingers")
	}
	var trials []trial
	for _, period := range []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1600 * time.Millisecond} {
		for _, keys := range [][][]int{{{0, 1}, {0, 2}}, {{0, 1}, {3, 4}}} {
			name := fmt.Sprintf("AB-AC / %dms", period.Milliseconds())
			if keys[1][0] == 3 {
				name = fmt.Sprintf("AB-DE / %dms", period.Milliseconds())
			}
			c := chordCycles(period, keys, 24)
			labelChart(c, p, name)
			r, err := difficulty.Analyze(c, p, o)
			if err != nil {
				return nil, err
			}
			start := time.Duration(math.Round(float64(2*period) / o.Rate))
			end := time.Duration(math.Round(float64(22*period) / o.Rate))
			trials = append(trials, trial{Name: name, Chart: c, Report: r, Region: measure(c, r, start, end)})
		}
	}
	return trials, nil
}
