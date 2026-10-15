package difficulty

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestAttributeSumsToStrain(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for k := range 24 {
		at := time.Duration(k) * 90 * time.Millisecond
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: k % 3, Kind: "tap"})
	}
	c.Notes = append(c.Notes, chart.Note{At: 0, End: time.Second, Lane: 3, Kind: "hold"},
		chart.Note{At: 500 * time.Millisecond, End: 500 * time.Millisecond, Lane: 3, Kind: "fake"})
	c.Normalize()
	r := analyze(t, c, NewProfile([]int{0, 0, 0, 0}), DefaultOptions())
	notes := r.Attribute(len(c.Notes))
	var sum float64
	for i, n := range notes {
		var parts float64
		for _, p := range n.Parts {
			parts += p.Cost.Total()
		}
		if math.Abs(parts-n.Cost.Total()) > 1e-9 {
			t.Fatalf("note %d parts %v, total %v", i, parts, n.Cost.Total())
		}
		if c.Notes[i].Kind == "fake" && n.Cost.Total() != 0 {
			t.Fatalf("fake note %d has strain %v", i, n.Cost.Total())
		}
		if c.Notes[i].Kind != "fake" && n.Cost.Movement == 0 {
			t.Fatalf("note %d has no movement", i)
		}
		sum += n.Cost.Total()
	}
	if math.Abs(sum-r.Strain) > 1e-9*r.Strain {
		t.Fatalf("notes sum to %v, report strain %v", sum, r.Strain)
	}
}
