package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestHistoricalComparisonDoesNotUseLatestScore(t *testing.T) {
	var h history
	h.Reset(-time.Second, piano.Score{})
	for _, p := range []point{{At: time.Second, Score: *kool(1)}, {At: 2 * time.Second, Score: *kool(2)}, {At: 3 * time.Second, Score: *kool(3)}} {
		if err := h.Append(p.At, p.Score); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Advance(4 * time.Second); err != nil {
		t.Fatal(err)
	}
	// A packet from 1.5s arrives when we are already at 4s.
	s, ok := h.At(1500 * time.Millisecond)
	if !ok || s.Points != 110000 {
		t.Fatalf("compared a future/latest score: %+v %v", s, ok)
	}
	if _, ok := h.At(5 * time.Second); ok {
		t.Fatal("score extrapolated beyond the processed watermark")
	}
	if _, ok := h.At(-2 * time.Second); ok {
		t.Fatal("score invented before the history start")
	}
	if err := h.Append(2*time.Second, *kool(4)); err == nil {
		t.Fatal("backwards history accepted")
	}
}

func TestSimultaneousChangesAndOffsetNormalization(t *testing.T) {
	c := &chart.Chart{Version: chart.Version, Lanes: 4, Timing: []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}},
		Notes: []chart.Note{{At: time.Second, End: time.Second, Lane: 0, Kind: "tap"}, {At: time.Second, End: time.Second, Lane: 1, Kind: "tap"}}}
	var histories [2]history
	for i, offset := range []time.Duration{0, 80 * time.Millisecond} {
		shifted, err := (settings.Offsets{GlobalMS: int(offset.Milliseconds())}).Apply(c)
		if err != nil {
			t.Fatal(err)
		}
		s, err := piano.NewSession(shifted)
		if err != nil {
			t.Fatal(err)
		}
		h := &histories[i]
		h.Reset(-time.Second, s.Score)
		s.OnScore = func(at time.Duration, score piano.Score) {
			if err := h.Append(at-offset, score); err != nil {
				t.Error(err)
			}
		}
		for lane := range 2 {
			if err := s.Input(time.Second+offset, lane, true); err != nil {
				t.Fatal(err)
			}
		}
		if err := h.Advance(2 * time.Second); err != nil {
			t.Fatal(err)
		}
		if len(h.Points()) != 2 { // zero + the final score at the simultaneous time
			t.Fatal("simultaneous score changes were not coalesced")
		}
	}
	a, _ := histories[0].At(time.Second)
	b, _ := histories[1].At(time.Second)
	if a != b || a.Points != piano.MaxScore {
		t.Fatalf("personal offset changed canonical comparison: %+v %+v", a, b)
	}
}

func TestMissHistoryUsesCommitTime(t *testing.T) {
	c := &chart.Chart{Version: chart.Version, Lanes: 1, Timing: []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}},
		Notes: []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	var h history
	h.Reset(0, s.Score)
	s.OnScore = func(at time.Duration, score piano.Score) { _ = h.Append(at, score) }
	if err := s.Advance(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := h.Advance(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	before, _ := h.At(1500 * time.Millisecond)
	after, _ := h.At(2 * time.Second)
	if before.Miss != 0 || after.Miss != 1 {
		t.Fatal("a late-processed MISS was falsely backdated into recorded history")
	}
}

func TestFinalHistoryCoverageRequiresEveryJudgement(t *testing.T) {
	var h history
	h.Reset(0, *kool(5))
	if err := h.Finish(10 * time.Second); err == nil {
		t.Fatal("incomplete score extrapolated to a later opponent timestamp")
	}
	if err := h.Append(time.Second, *kool(10)); err != nil {
		t.Fatal(err)
	}
	if err := h.Finish(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	s, ok := h.At(9 * time.Second)
	if !ok || s.Points != piano.MaxScore {
		t.Fatal("final score could not be compared across personal offsets")
	}
}

func kool(n int) *piano.Score {
	return &piano.Score{Kool: n, Total: n, Possible: 10, Combo: n, MaxCombo: n,
		Earned: int64(n) * 2200, Points: n * 110000, Flow: 50, Precision: 20}
}
