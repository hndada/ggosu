package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

type measurement struct {
	At               time.Duration       `json:"at_ns"`
	End              time.Duration       `json:"end_ns"`
	Notes            []int               `json:"notes_per_finger"`
	NotesPerSecond   float64             `json:"notes_per_second"`
	PressesPerSecond []float64           `json:"presses_per_second_per_finger"`
	Movement         float64             `json:"movement"`
	Interaction      float64             `json:"interaction"`
	Repeat           float64             `json:"repeat"`
	Difficulty       float64             `json:"difficulty"`
	Fingers          []difficulty.Cost   `json:"fingers"`
	Windows          []difficulty.Window `json:"windows"`
}

func compare(o difficulty.Options) ([]trial, error) {
	return compareProfile(o, difficulty.NewProfile([]int{0, 0}))
}

func compareProfile(o difficulty.Options, profile difficulty.Profile) ([]trial, error) {
	if len(profile.Fingers) != 2 || profile.Fingers[0].Hand != profile.Fingers[1].Hand {
		return nil, fmt.Errorf("controlled comparison profile requires two fingers on the same hand")
	}
	var trials []trial
	period := 200 * time.Millisecond
	start := time.Duration(math.Round(float64(2*period) / o.Rate))
	end := time.Duration(math.Round(float64(22*period) / o.Rate))
	for _, hands := range [][]int{{0, 0}, {0, 1}} {
		for _, trill := range []bool{false, true} {
			c := fixture(trill, period, 24)
			p := comparisonProfile(profile, hands[0] != hands[1])
			r, err := difficulty.Analyze(c, p, o)
			if err != nil {
				return nil, err
			}
			name := "chord"
			if trill {
				name = "trill"
			}
			if hands[0] == hands[1] {
				name += " / same hand"
			} else {
				name += " / separate hands"
			}
			labelChart(c, p, name)
			trials = append(trials, trial{Name: name, Chart: c, Region: measure(c, r, start, end), Report: r})
		}
	}
	// A long hold plus another finger's isolated tap exposes passive-recipient
	// costs separately from the LN's own head and release.
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: 0, End: time.Second, Lane: 0, Kind: "hold"},
		{At: 400 * time.Millisecond, End: 400 * time.Millisecond, Lane: 1, Kind: "tap"},
	}
	for _, hands := range [][]int{{0, 0}, {0, 1}} {
		r, err := difficulty.Analyze(c, comparisonProfile(profile, hands[0] != hands[1]), o)
		if err != nil {
			return nil, err
		}
		name := "hold + tap / same hand"
		if hands[0] != hands[1] {
			name = "hold + tap / separate hands"
		}
		// Reference motion times do not scale with song rate. Find the actual
		// tap phases instead of scaling a fixed crop and truncating preparation.
		var from, until time.Duration
		found := false
		for _, s := range r.Segments {
			st := s.States[1]
			if st.Note != 1 || (st.Action != difficulty.Press && st.Action != difficulty.Release) {
				continue
			}
			if !found || s.At < from {
				from = s.At
			}
			until, found = max(until, s.End), true
		}
		labelChart(c, r.Profile, name)
		trials = append(trials, trial{Name: name, Chart: c, Region: measure(c, r, from, until), Report: r})
	}
	return trials, nil
}

func labelChart(c *chart.Chart, p difficulty.Profile, name string) {
	c.Metadata.Title = name
	c.Metadata.Difficulty = "controlled POC"
	c.Layout = make([]chart.Lane, c.Lanes)
	for i, f := range p.Fingers {
		c.Layout[i] = chart.Lane{Name: string(rune('A' + i)), Role: "key", Group: f.Hand}
	}
}

func comparisonProfile(p difficulty.Profile, separate bool) difficulty.Profile {
	if separate {
		p.Fingers = slices.Clone(p.Fingers)
		p.Fingers[0].Hand, p.Fingers[1].Hand = 0, 1
		p.Pairs = nil
	}
	return p
}

func fixture(trill bool, period time.Duration, cycles int) *chart.Chart {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := 0; i < cycles; i++ {
		for finger := range 2 {
			at := time.Duration(i) * period
			if trill && finger == 1 {
				at += period / 2
			}
			c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: finger, Kind: "tap"})
		}
	}
	c.Normalize()
	return c
}

// measure excludes preparation/tail transients and rebins clipped movement
// segments exactly, including when the selected window does not divide a cycle.
func measure(c *chart.Chart, r *difficulty.Report, start, end time.Duration) *measurement {
	m := &measurement{At: start, End: end, Notes: make([]int, c.Lanes),
		PressesPerSecond: make([]float64, c.Lanes), Fingers: make([]difficulty.Cost, c.Lanes)}
	seconds := (end - start).Seconds()
	for _, n := range c.Notes {
		at := time.Duration(math.Round(float64(n.At) / r.Options.Rate))
		if n.Kind != "fake" && at >= start && at < end {
			m.Notes[n.Lane]++
			m.NotesPerSecond += 1 / seconds
			m.PressesPerSecond[n.Lane] += 1 / seconds
		}
	}
	indices := make(map[int64]int)
	for _, s := range r.Segments {
		at, until := max(s.At, start), min(s.End, end)
		for at < until {
			id := int64((at - start) / r.Options.Window)
			windowAt := start + time.Duration(id)*r.Options.Window
			windowEnd := min(windowAt+r.Options.Window, end)
			partEnd := min(until, windowEnd)
			i, ok := indices[id]
			if !ok {
				i = len(m.Windows)
				indices[id] = i
				m.Windows = append(m.Windows, difficulty.Window{At: windowAt, End: windowEnd, Fingers: make([]difficulty.Cost, c.Lanes)})
			}
			w := &m.Windows[i]
			for finger, cost := range s.Costs(at, partEnd, r.Options) {
				base, pair, repeat := cost.Movement, cost.Interaction, cost.Repeat
				m.Fingers[finger].Movement += base
				m.Fingers[finger].Interaction += pair
				m.Fingers[finger].Repeat += repeat
				m.Movement += base
				m.Interaction += pair
				m.Repeat += repeat
				w.Fingers[finger].Movement += base
				w.Fingers[finger].Interaction += pair
				w.Fingers[finger].Repeat += repeat
				w.Total += cost.Total()
			}
			at = partEnd
		}
	}
	m.Difficulty = difficulty.Rank(m.Windows, r.Options.Weight)
	return m
}
