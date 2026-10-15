package chart

import (
	"testing"
	"time"
)

func TestHeadsAndSimultaneousHolds(t *testing.T) {
	c := New(4)
	c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []Note{
		{At: -time.Second, End: -time.Second, Kind: "tap"},
		{At: 0, End: time.Second, Kind: "hold"},
		{At: 500 * time.Millisecond, End: 1500 * time.Millisecond, Lane: 1, Kind: "roll"},
		{At: time.Second, End: 2 * time.Second, Kind: "hold"},
		{At: 2 * time.Second, End: 2 * time.Second, Kind: "lift", Lane: 2},
		{At: 2 * time.Second, End: 2 * time.Second, Kind: "mine", Lane: 3},
		{At: 2 * time.Second, End: 2 * time.Second, Kind: "fake"},
	}
	r, err := AnalyzeDensity(c)
	if err != nil {
		t.Fatal(err)
	}
	if r.Start != -1 || r.Step != 1 || r.Total != 5 || r.Mines != 1 || len(r.Bins) != 4 {
		t.Fatalf("totals: %+v", r)
	}
	if r.Bins[1].Hold != 2 || r.Bins[1].PeakHolds != 2 || r.Bins[2].Hold != 1 || r.Bins[2].PeakHolds != 2 || r.Bins[3].PeakHolds != 0 || r.Bins[3].Lift != 1 || r.Bins[3].Mine != 1 {
		t.Fatalf("boundary, tail or fake counted incorrectly: %+v", r.Bins)
	}
	if r.Lanes[0] != 3 || r.Lanes[1] != 1 || r.Lanes[2] != 1 || r.Lanes[3] != 0 {
		t.Fatalf("lane totals: %v", r.Lanes)
	}
}

func TestLongChartsBoundGraphSize(t *testing.T) {
	c := New(1)
	c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []Note{{At: -MaxTime, End: MaxTime, Kind: "hold"}}
	r, err := AnalyzeDensity(c)
	if err != nil || len(r.Bins) > 600 || r.Total != 1 || r.Step <= 1 {
		t.Fatalf("unbounded distribution: %d bins, %+v, %v", len(r.Bins), r, err)
	}
	for _, b := range r.Bins[:len(r.Bins)-1] {
		if b.PeakHolds != 1 {
			t.Fatal("lost sustained hold in empty bins")
		}
	}
}
