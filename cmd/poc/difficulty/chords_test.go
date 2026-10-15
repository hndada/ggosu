package main

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

func TestChordSuite(t *testing.T) {
	p := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1})
	trials, err := compareChords(baselineOptions(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(trials) != 10 {
		t.Fatal("missing chord conditions")
	}
	want := []struct{ density, movement, interaction float64 }{
		{5, 60, 3}, {10, 120, 6}, {10, 120, 6}, {10, 120, 0},
		{10, 120, 6}, {10, 120, 6}, {20, 320, 16},
		{15, 180, 18}, {15, 180, 6}, {10, 120, 12},
	}
	for i, c := range trials {
		if c.Chart == nil || len(c.Chart.Layout) != 6 {
			t.Fatal("the exact chart and fixed hand mapping must be inspectable")
		}
		if err := c.Chart.Validate(); err != nil {
			t.Fatal(err)
		}
		m := c.Region
		if math.Abs(m.NotesPerSecond-want[i].density) > 1e-8 ||
			math.Abs(m.Movement-want[i].movement) > 1e-8 ||
			math.Abs(m.Interaction-want[i].interaction) > 1e-8 {
			t.Fatalf("%s: unexpected density or movement/pair accounting: %+v", c.Name, m)
		}
	}
	if !(trials[0].Region.Difficulty < trials[1].Region.Difficulty && trials[1].Region.Difficulty < trials[6].Region.Difficulty) {
		t.Fatal("the same repeated chord should increase with speed")
	}
	// Equal example coefficients and no fatigue make these cases identical.
	// Preserve this limitation instead of inventing anatomical distinctions.
	for _, i := range []int{2, 4, 5} {
		if math.Abs(trials[i].Region.Difficulty-trials[1].Region.Difficulty) > 1e-8 {
			t.Fatal("uniform relaxed-motion profile must not invent finger identity or fatigue effects")
		}
	}
	if trials[7].Region.Difficulty <= trials[8].Region.Difficulty {
		t.Fatal("same-hand three-key chord has three pair costs; split chord has one")
	}
	// Same note density and base cost, different chord size and concentration.
	if trials[9].Region.Difficulty <= trials[1].Region.Difficulty {
		t.Fatal("chosen profile/window should preserve burst and extra-pair effects")
	}
	// Pair-specific coefficients can distinguish AB and AC without changing speed.
	p.Pairs[0].Costs[difficulty.Press][difficulty.Press] = difficulty.Share{A: 0.2, B: 0.2}
	p.Pairs[0].Costs[difficulty.Release][difficulty.Release] = difficulty.Share{A: 0.2, B: 0.2}
	custom, err := compareChords(baselineOptions(), p)
	if err != nil {
		t.Fatal(err)
	}
	if custom[1].Region.Difficulty <= custom[2].Region.Difficulty {
		t.Fatal("pair-specific profile coefficients were not applied")
	}
}

func TestExportedFixturesMatchAnalysis(t *testing.T) {
	trials, err := compare(baselineOptions())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := exportCharts(dir, trials); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "01.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	c, readErr := chart.Read(f)
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read exported chart: %v, %v", readErr, closeErr)
	}
	if !reflect.DeepEqual(c.Notes, trials[0].Chart.Notes) || !reflect.DeepEqual(c.Layout, trials[0].Chart.Layout) {
		t.Fatal("export differs from the analyzed fixture")
	}
	r, err := difficulty.Analyze(c, trials[0].Report.Profile, trials[0].Report.Options)
	if err != nil || math.Abs(r.Difficulty-trials[0].Report.Difficulty) > 1e-8 {
		t.Fatal("exported chart does not reproduce the analyzed result")
	}
	if err := exportCharts(dir, trials); err == nil {
		t.Fatal("export must not overwrite an existing chart")
	}
}
