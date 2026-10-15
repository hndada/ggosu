package main

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/difficulty"
)

func closeCost(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-8*max(1, math.Abs(want)) {
		t.Fatalf("got %.12g, want %.12g", got, want)
	}
}

func TestLongNoteInteractions(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	trials, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(trials) != 12 {
		t.Fatal("missing long-note conditions")
	}
	for _, i := range []int{0, 1, 2, 3, 4, 5, 11} {
		m := trials[i].Region
		closeCost(t, m.NotesPerSecond, 4.8)
		closeCost(t, m.Movement, 48)
		if m.Notes[0] != 0 || m.Fingers[0].Movement != 0 {
			t.Fatal("middle interval must exclude LN head and tail motions")
		}
	}
	base, same, other := trials[0].Region, trials[1].Region, trials[2].Region
	closeCost(t, same.Fingers[0].Interaction, 9.6)
	closeCost(t, same.Fingers[1].Interaction, 12)
	if same.Fingers[0].Repeat <= 0 || same.Repeat <= base.Repeat || same.Difficulty <= base.Difficulty {
		t.Fatal("same-hand hold must add passive load and repeat cost")
	}
	closeCost(t, other.Interaction, 0)
	closeCost(t, other.Repeat, base.Repeat)
	closeCost(t, other.Difficulty, base.Difficulty)
	alt := trials[3].Region
	closeCost(t, alt.Interaction, same.Interaction)
	closeCost(t, alt.Fingers[0].Interaction, same.Fingers[0].Interaction)
	closeCost(t, alt.Fingers[0].Repeat, same.Fingers[0].Repeat)
	if alt.Repeat >= same.Repeat || alt.Difficulty >= same.Difficulty {
		t.Fatal("alternating tapping fingers should reduce concentrated repeat work")
	}
	two, split := trials[4].Region, trials[5].Region
	closeCost(t, two.Interaction, 43.2)
	closeCost(t, two.Fingers[0].Movement, 0)
	closeCost(t, two.Fingers[2].Movement, 0)
	closeCost(t, two.Fingers[0].Interaction, 9.6)
	closeCost(t, two.Fingers[2].Interaction, 9.6)
	if two.Difficulty <= same.Difficulty {
		t.Fatal("a second constrained same-hand finger should add recipient costs")
	}
	closeCost(t, split.Difficulty, same.Difficulty)
	closeCost(t, split.Fingers[3].Total(), 0)
	closeCost(t, trials[11].Region.Interaction, 24)
	// Matrix-disabled control leaves tapping speed/history unchanged and gives
	// the held finger no recipient cost. No standalone sustain cost is modeled.
	p.Pairs = nil
	control, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, control[1].Region.Fingers[0].Total(), 0)
	closeCost(t, control[1].Region.Difficulty, control[0].Region.Difficulty)
}

func TestLongNoteTailAndOverlap(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	trials, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	slow, fast := trials[8], trials[9]
	closeCost(t, slow.Region.Movement, 4)
	closeCost(t, fast.Region.Movement, 7)
	if fast.Region.Difficulty <= slow.Region.Difficulty {
		t.Fatal("40ms reset must compress release and next press")
	}
	for _, s := range fast.Report.Segments {
		if s.At >= time.Second && s.End <= 1040*time.Millisecond {
			closeCost(t, s.States[0].Speed, 2.5)
		}
	}
	var a, b float64
	for _, s := range trials[10].Report.Segments {
		if s.At >= time.Second && s.End <= 1025*time.Millisecond {
			if s.States[0].Action != difficulty.Release || s.States[1].Action != difficulty.Press {
				t.Fatal("tail overlap must use opposite moving states, not instantaneous note order")
			}
			a += s.Fingers[0].Interaction
			b += s.Fingers[1].Interaction
		}
	}
	closeCost(t, a, 0.175)
	closeCost(t, b, 0.175)
	// At an identical same-finger deadline no reset is available. This remains
	// an explicit POC constraint; judgement-window scheduling is not implemented.
	c := holdTail(time.Second, time.Nanosecond, 0)
	c.Notes[1].At = c.Notes[0].End
	c.Notes[1].End = c.Notes[1].At
	if _, err := difficulty.Analyze(c, p, o); err == nil {
		t.Fatal("a zero-duration reset must not be silently accepted")
	}
}

func TestLongNoteRecoveryAndRate(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	normal, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	o.HoldRecovery = 1
	free, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, free[1].Region.Movement, normal[1].Region.Movement)
	closeCost(t, free[1].Region.Interaction, normal[1].Region.Interaction)
	if free[1].Region.Fingers[0].Repeat >= normal[1].Region.Fingers[0].Repeat {
		t.Fatal("faster recovery must reduce the held recipient's repeat cost")
	}
	o = difficulty.DefaultOptions()
	o.Rate = 2
	fast, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 1, 2, 3, 4, 5, 11} {
		closeCost(t, fast[i].Region.NotesPerSecond, 9.6)
		if fast[i].Region.Notes[0] != 0 || fast[i].Region.Fingers[0].Movement != 0 {
			t.Fatal("rated observation must still exclude hold heads/tails")
		}
	}
	if fast[1].Region.Movement <= normal[1].Region.Movement {
		t.Fatal("rated repeated tap motions must compress")
	}
	if fast[1].Chart.Notes[0].End != 6*time.Second {
		t.Fatal("rate must not rewrite the exported source hold duration")
	}
}

func TestLongNoteZeroCostProfile(t *testing.T) {
	p := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1})
	p.Pairs = nil
	for i := range p.Fingers {
		p.Fingers[i].PressCost, p.Fingers[i].ReleaseCost = 0, 0
	}
	trials, err := compareHolds(difficulty.DefaultOptions(), p)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range trials {
		if c.Report.Strain != 0 || c.Report.Difficulty != 0 {
			t.Fatal("zero-cost profile must not create work or require a nonempty cost interval")
		}
	}
}

func TestLongNoteDurationAndAccounting(t *testing.T) {
	p, o := difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1}), difficulty.DefaultOptions()
	a, err := compareHolds(o, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{6, 7} {
		closeCost(t, a[i].Region.Movement, 2)
		closeCost(t, a[i].Region.Interaction, 0)
		closeCost(t, a[i].Region.Difficulty, a[i].Report.Difficulty)
	}
	if a[7].Region.Repeat >= a[6].Region.Repeat {
		t.Fatal("without sustain work, a longer isolated hold only allows more recovery before release")
	}
	// Document this limitation rather than manufacturing a length bonus.
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
