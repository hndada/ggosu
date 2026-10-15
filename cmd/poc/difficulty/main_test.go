package main

import (
	"math"
	"reflect"
	"testing"
	"time"

	"ggosu/internal/difficulty"
)

// Original movement/matrix controls deliberately disable the new history term.
func baselineOptions() difficulty.Options {
	o := difficulty.DefaultOptions()
	o.RepeatGain = 0
	return o
}

func TestComparison(t *testing.T) {
	trials, err := compare(baselineOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(trials) != 6 {
		t.Fatal("expected chord/trill controls and two hold trials")
	}
	for _, c := range trials[:4] {
		if math.Abs(c.Region.NotesPerSecond-10) > 1e-8 || !reflect.DeepEqual(c.Region.Notes, []int{20, 20}) {
			t.Fatal("comparison must measure equal density in complete interior cycles")
		}
		if math.Abs(c.Region.Movement-80) > 1e-8 {
			t.Fatal("matched finger movements must have equal base cost")
		}
	}
	if trials[1].Region.Difficulty <= trials[0].Region.Difficulty {
		t.Fatal("illustrative same-hand coefficients must separate the patterns")
	}
	if math.Abs(trials[2].Region.Difficulty-trials[3].Region.Difficulty) > 1e-8 {
		t.Fatal("separate-hand controls must be equal")
	}
	if trials[4].Region.Fingers[0].Movement != 0 || trials[4].Region.Fingers[0].Interaction <= 0 {
		t.Fatal("LN recipient should have interaction strain without a new stroke")
	}
	if trials[5].Region.Fingers[0].Total() != 0 {
		t.Fatal("separate-hand profile should not produce LN interference")
	}
}

func TestMeasurementCuts(t *testing.T) {
	o := baselineOptions()
	a, err := compare(o)
	if err != nil {
		t.Fatal(err)
	}
	o.Window = 7 * time.Millisecond
	b, err := compare(o)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if math.Abs(a[i].Region.Movement-b[i].Region.Movement) > 1e-8 ||
			math.Abs(a[i].Region.Interaction-b[i].Region.Interaction) > 1e-8 {
			t.Fatal("changing integration cuts must not duplicate or discard costs")
		}
	}
}

func TestHoldMeasurementIncludesPreparationAtHigherRate(t *testing.T) {
	o := baselineOptions()
	o.Rate = 2
	trials, err := compare(o)
	if err != nil {
		t.Fatal(err)
	}
	m := trials[4].Region
	if math.Abs(m.Movement-2) > 1e-8 || math.Abs(m.Fingers[0].Interaction-0.4) > 1e-8 {
		t.Fatal("a scaled crop must not discard part of the isolated tap preparation")
	}
}

func TestHands(t *testing.T) {
	ids, err := parseHands("0, 0, 1, 1")
	if err != nil || !reflect.DeepEqual(ids, []int{0, 0, 1, 1}) {
		t.Fatal("explicit mapping was not parsed")
	}
	for _, s := range []string{"", "0,-1", "0,x"} {
		if _, err := parseHands(s); err == nil {
			t.Fatalf("invalid mapping %q accepted", s)
		}
	}
}

func TestCustomComparisonProfile(t *testing.T) {
	p := difficulty.NewProfile([]int{0, 0})
	p.Pairs = nil
	trials, err := compareProfile(baselineOptions(), p)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(trials[0].Region.Difficulty-trials[1].Region.Difficulty) > 1e-8 {
		t.Fatal("custom zero-interaction profile must eliminate chord/trill separation")
	}
	if !reflect.DeepEqual(p.Fingers, difficulty.NewProfile([]int{0, 0}).Fingers) {
		t.Fatal("separate-hand control mutated the supplied profile")
	}
}
