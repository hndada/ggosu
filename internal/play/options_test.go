package play

import (
	"slices"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestSnapDivisorFollowsLazer(t *testing.T) {
	ms := time.Millisecond
	// 120 BPM from 100 ms, then a scroll-only point that keeps the beat grid.
	timing := []chart.Timing{{At: 100 * ms, BPM: 120, Scroll: 1}, {At: 1000 * ms, BPM: 120, Scroll: 2}}
	for _, c := range []struct {
		at   time.Duration
		want int
	}{
		{100 * ms, 1}, {600 * ms, 1}, {350 * ms, 2}, {225 * ms, 4}, {162500 * time.Microsecond, 8},
		{100*ms + 500*ms/3, 3}, {100*ms + 500*ms/6, 6}, {1350 * ms, 2}, {-400 * ms, 1},
	} {
		if got := closestDivisor(timing, c.at); got != c.want {
			t.Errorf("%v: divisor %d, want %d", c.at, got, c.want)
		}
	}
	// A BPM change starts a new grid at its own time.
	timing = append(timing, chart.Timing{At: 1100 * ms, BPM: 60, Scroll: 1})
	if got := closestDivisor(timing, 2100*ms); got != 1 {
		t.Errorf("after the BPM change: divisor %d, want 1", got)
	}
	if divisorColour(1) != white || divisorColour(4) == divisorColour(2) {
		t.Error("beat colours must follow BindableBeatDivisor.GetColourFor")
	}
}

func TestJudgementShownFiltersByGrade(t *testing.T) {
	for _, c := range []struct {
		show string
		want []piano.Grade
	}{
		{"", []piano.Grade{piano.Kool, piano.Cool, piano.Good, piano.Miss}},
		{"cool", []piano.Grade{piano.Cool, piano.Good, piano.Miss}},
		{"good", []piano.Grade{piano.Good, piano.Miss}},
		{"miss", []piano.Grade{piano.Miss}},
		{"off", nil},
	} {
		var got []piano.Grade
		for _, g := range []piano.Grade{piano.Kool, piano.Cool, piano.Good, piano.Miss} {
			if judgementShown(g, c.show) {
				got = append(got, g)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%q shows %v, want %v", c.show, got, c.want)
		}
	}
}

func TestGuideTimesSkipMinesAndSoundChordsOnce(t *testing.T) {
	ms := time.Millisecond
	c := &chart.Chart{Notes: []chart.Note{
		{At: 100 * ms, End: 100 * ms, Kind: "tap"},
		{At: 100 * ms, End: 400 * ms, Lane: 1, Kind: "hold"},
		{At: 200 * ms, End: 200 * ms, Kind: "mine"},
		{At: 300 * ms, End: 300 * ms, Kind: "fake"},
	}}
	if got, want := guideTimes(c), []time.Duration{100 * ms, 100 * ms, 400 * ms}; !slices.Equal(got, want) {
		t.Fatalf("guide times %v, want %v", got, want)
	}
	// Without a bank nothing plays, but the cursor still passes due and stale guides.
	g := scene{guides: guideTimes(c), position: 450 * ms}
	if err := g.guideSound(); err != nil || g.guide != 3 {
		t.Fatalf("guide cursor %d: %v", g.guide, err)
	}
}

func TestAbortOnlyWhenTargetIsOutOfReach(t *testing.T) {
	ms := time.Millisecond
	c := &chart.Chart{Version: chart.Version, Lanes: 1, Timing: []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}, Notes: []chart.Note{
		{At: 1000 * ms, End: 1000 * ms, Kind: "tap"}, {At: 2000 * ms, End: 2000 * ms, Kind: "tap"},
	}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	g := scene{session: s, mode: "live", display: settings.Display{Abort: true, GhostScore: 900000}}
	if g.abortDue() {
		t.Fatal("a fresh play can still reach the target")
	}
	if err := s.Advance(1500 * ms); err != nil { // The first note is missed.
		t.Fatal(err)
	}
	if !g.abortDue() {
		t.Fatalf("max %d below the target must abort", s.Score.MaxPoints())
	}
	for _, off := range []*scene{
		{session: s, mode: "live", display: settings.Display{GhostScore: 900000}},
		{session: s, mode: "auto", display: g.display},
		{session: s, mode: "live", display: g.display, sectionEnd: time.Second},
	} {
		if off.abortDue() {
			t.Errorf("abort must stay off for %+v", off.display)
		}
	}
}

func TestJudgeOffsetMovesJudgementNotNotes(t *testing.T) {
	o := settings.Offsets{GlobalMS: 10, JudgeMS: 30}
	if o.Total() != 40*time.Millisecond || o.Judge() != 30*time.Millisecond {
		t.Fatalf("total %v, judge %v", o.Total(), o.Judge())
	}
	if (settings.Offsets{JudgeMS: 1001}).Validate() == nil {
		t.Error("judgement offset beyond 1000 ms must be rejected")
	}
}

func TestHitAndGuideSamplesDecode(t *testing.T) {
	kinds := []string{"guide"}
	for _, s := range settings.HitSounds {
		kinds = append(kinds, settings.Audio{HitSound: s}.HitKind())
	}
	for _, kind := range kinds {
		data, err := sounds.ReadFile("sounds/" + kind + ".ogg")
		if err != nil {
			t.Fatal(err)
		}
		if pcm, err := decode(data, ".ogg", maxPCM); err != nil || len(pcm) == 0 {
			t.Errorf("%s decoded to %d bytes: %v", kind, len(pcm), err)
		}
	}
}
