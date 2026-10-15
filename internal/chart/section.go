package chart

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

// Section keeps the notes that start in [start, end) for practice. As in
// beatoraja's PracticeModifier, notes outside the section turn into background
// sounds so the song still plays; audio from the end onward is dropped.
func (c *Chart) Section(start, end time.Duration) (*Chart, error) {
	if start >= end {
		return nil, fmt.Errorf("chart: practice section must end after it starts")
	}
	x := *c
	x.Notes = nil
	x.Audio = nil
	for _, n := range c.Notes {
		if n.At >= start && n.At < end {
			x.Notes = append(x.Notes, n)
			continue
		}
		if n.At < end && n.Kind != "mine" && n.Kind != "fake" {
			for _, s := range n.Sounds {
				x.Audio = append(x.Audio, Audio{At: n.At, Sound: s})
			}
		}
	}
	if len(x.Notes) == 0 {
		return nil, fmt.Errorf("chart: practice section has no notes")
	}
	for _, a := range c.Audio {
		if a.At < end {
			x.Audio = append(x.Audio, a)
		}
	}
	slices.SortStableFunc(x.Audio, func(a, b Audio) int { return cmp.Compare(a.At, b.At) })
	return &x, x.Validate()
}
