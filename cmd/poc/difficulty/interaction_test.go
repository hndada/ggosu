package main

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/difficulty"
)

func TestInteractionMemoryWeight(t *testing.T) {
	p := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1})
	previous := math.Inf(1)
	for _, weight := range []float64{1, 0.5, 0.25, 0} {
		o := difficulty.DefaultOptions()
		o.InteractionLoad = weight
		trials, err := compareHolds(o, p)
		if err != nil {
			t.Fatal(err)
		}
		base, ln := trials[0].Region, trials[1].Region
		closeCost(t, ln.Movement, 48)
		closeCost(t, ln.Interaction, 21.6)
		closeCost(t, ln.Fingers[0].Interaction, 9.6)
		if ln.Repeat >= previous || ln.Difficulty <= base.Difficulty {
			t.Fatal("lower memory weight should reduce amplification while retaining instantaneous interference")
		}
		previous = ln.Repeat
		if ln.Repeat > 0.5*(ln.Movement+ln.Interaction) {
			t.Fatal("hold interaction history must remain a secondary cost")
		}
		chords, err := compareChords(o, p)
		if err != nil {
			t.Fatal(err)
		}
		if chords[4].Region.Difficulty <= chords[5].Region.Difficulty {
			t.Fatal("AB-AC > AB-DE must survive interaction-memory weighting")
		}
	}
}

func TestWeightedInteractionClipping(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	for _, weight := range []float64{0, 0.25, 1} {
		o.InteractionLoad, o.Window = weight, 200*time.Millisecond
		a, err := compareHolds(o, p)
		if err != nil {
			t.Fatal(err)
		}
		o.Window = 7 * time.Millisecond
		b, err := compareHolds(o, p)
		if err != nil {
			t.Fatal(err)
		}
		for i := range a {
			closeCost(t, a[i].Region.Movement, b[i].Region.Movement)
			closeCost(t, a[i].Region.Interaction, b[i].Region.Interaction)
			closeCost(t, a[i].Region.Repeat, b[i].Region.Repeat)
		}
	}
}
