package main

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/difficulty"
)

func TestSharedFingerAndRecovery(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	trials, err := compareRecovery(o, p)
	if err != nil {
		t.Fatal(err)
	}
	previous := math.Inf(1)
	for i := 0; i < len(trials); i += 2 {
		a, b := trials[i].Region, trials[i+1].Region
		if math.Abs(a.Movement-b.Movement) > 1e-8 || math.Abs(a.Interaction-b.Interaction) > 1e-8 ||
			math.Abs(a.NotesPerSecond-b.NotesPerSecond) > 1e-8 {
			t.Fatal("shared/separate conditions must preserve original work, matrix and density")
		}
		if a.Repeat <= b.Repeat || a.Difficulty <= b.Difficulty {
			t.Fatal("shared A must retain more work and make AB-AC harder than AB-DE")
		}
		ratio := a.Difficulty / b.Difficulty
		if ratio >= previous {
			t.Fatal("adding rest must reduce the shared/separate difficulty ratio")
		}
		previous = ratio
	}
	if previous > 1.01 {
		t.Fatal("sufficient rest should make the two patterns converge")
	}
	// History disabled reproduces the original equality at every tested interval.
	o.RepeatGain = 0
	baseline, err := compareRecovery(o, p)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(baseline); i += 2 {
		a, b := baseline[i].Region, baseline[i+1].Region
		if a.Repeat != 0 || b.Repeat != 0 || math.Abs(a.Difficulty-b.Difficulty) > 1e-8 {
			t.Fatal("disabled repeat term must restore v1 accounting")
		}
	}
	// This separation is per-finger work history, not a hardcoded pattern or
	// same-hand pair bonus: removing the matrix must preserve its direction.
	p.Pairs = nil
	withoutMatrix, err := compareRecovery(difficulty.DefaultOptions(), p)
	if err != nil || withoutMatrix[0].Region.Difficulty <= withoutMatrix[1].Region.Difficulty {
		t.Fatal("shared-finger history should distinguish patterns even without matrix costs")
	}
}

func TestRecoveryMeasurementClipping(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	a, err := compareChords(o, p)
	if err != nil {
		t.Fatal(err)
	}
	o.Window = 7 * time.Millisecond
	b, err := compareChords(o, p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if math.Abs(a[i].Region.Repeat-b[i].Region.Repeat) > 1e-8 ||
			math.Abs(a[i].Region.Movement-b[i].Region.Movement) > 1e-8 {
			t.Fatal("cropping/rebinning must exactly integrate nonuniform repeat costs")
		}
	}
}
