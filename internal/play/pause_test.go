package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
)

func pauseGame(t *testing.T, speed float64) *scene {
	t.Helper()
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 10 * time.Second, End: 10 * time.Second, Kind: "tap"}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	g := &scene{chart: c, session: s, speed: speed, timeline: clock.NewTimeline(0)}
	if err := g.timeline.SetRate(0, speed); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestResumeCountdownFreezesGameTimeAtEveryRate(t *testing.T) {
	for _, speed := range []float64{.5, 1, 2} {
		g := pauseGame(t, speed)
		if err := g.pause(2 * time.Second); err != nil {
			t.Fatal(err)
		}
		frozen, score := g.position, g.session.Score
		g.resume(5 * time.Second)
		for i, want := range []int{3, 2, 1} {
			now := time.Duration(5+i) * time.Second
			if err := g.updateResume(now, true); err != nil {
				t.Fatal(err)
			}
			if !g.paused || g.countdown(now) != want || g.timeline.Position(now) != frozen || g.session.Score != score {
				t.Fatalf("rate %v, second %d: countdown or judgement advanced", speed, i)
			}
		}
		if err := g.updateResume(8*time.Second, true); err != nil {
			t.Fatal(err)
		}
		if g.paused || g.countdown(8*time.Second) != 0 || g.inputSince != 8*time.Second || g.timeline.Position(8*time.Second) != frozen || g.timeline.Position(9*time.Second) != frozen+time.Duration(speed*float64(time.Second)) {
			t.Fatalf("rate %v: resume jumped or used a wrong time base", speed)
		}
	}
}

func TestResumeCanBeCanceledAndDoesNotCatchUpAfterAStall(t *testing.T) {
	g := pauseGame(t, 1)
	if err := g.pause(time.Second); err != nil {
		t.Fatal(err)
	}
	g.resume(2 * time.Second)
	g.resume(3 * time.Second) // A repeated request does not extend the countdown.
	if g.resumeAt != 5*time.Second {
		t.Fatal("repeated resume changed the deadline")
	}
	if err := g.updateResume(3*time.Second, false); err != nil {
		t.Fatal(err)
	}
	if g.resumeAt != 0 || !g.paused {
		t.Fatal("focus loss did not cancel resume")
	}
	g.resume(6 * time.Second)
	if err := g.pause(7 * time.Second); err != nil {
		t.Fatal(err)
	}
	if g.resumeAt != 0 || !g.paused {
		t.Fatal("pause did not cancel resume")
	}
	g.resume(8 * time.Second)
	g.speed = 2 // Rate changes during the countdown leave it paused.
	if err := g.updateResume(30*time.Second, true); err != nil {
		t.Fatal(err)
	}
	if g.timeline.Position(30*time.Second) != time.Second || g.timeline.Position(31*time.Second) != 3*time.Second {
		t.Fatal("delayed update skipped game time or did not apply the new rate")
	}
}

func TestPauseLogIncludesReasonsCountdownAndRealTime(t *testing.T) {
	g := pauseGame(t, 2)
	if err := g.pauseFor(time.Second, "focus"); err != nil {
		t.Fatal(err)
	}
	g.resume(2 * time.Second)
	if err := g.updateResume(3*time.Second, false); err != nil {
		t.Fatal(err)
	}
	g.resume(4 * time.Second)
	if err := g.updateResume(7*time.Second, true); err != nil {
		t.Fatal(err)
	}
	events := g.recording.Pauses
	want := []string{"pause", "resume-request", "resume-cancel", "resume-request", "resume"}
	if len(events) != len(want) {
		t.Fatal(events)
	}
	for i, e := range events {
		if e.Action != want[i] || e.At != 2*time.Second || e.Rate != 2 {
			t.Fatal(e)
		}
	}
	if events[0].Reason != "focus" || events[4].Elapsed != 7*time.Second {
		t.Fatal(events)
	}
	r, err := piano.NewChartReplay(g.chart, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	r.Duration, r.Pauses = 12*time.Second, events
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPauseDoesNotBreakHeldNoteOrDrainHealth(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: 5 * time.Second, Kind: "hold"}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	s.Health = piano.NewHealth(3, 0)
	if err := s.Input(time.Second, 0, true); err != nil {
		t.Fatal(err)
	}
	g := scene{chart: c, session: s, speed: 1, timeline: clock.NewTimeline(0), last: time.Second}
	before := s.Score
	if err := g.pauseFor(2*time.Second, "escape"); err != nil {
		t.Fatal(err)
	}
	if s.Score != before || !s.Active(0) || s.Health.Value != 10000 {
		t.Fatal("pause broke a long note")
	}
}

func TestResumePreservesHeldNoteThroughInput(t *testing.T) {
	for _, mode := range []string{"manual", "focus", "countdown", "released"} {
		t.Run(mode, func(t *testing.T) {
			c := chart.New(1)
			c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
			c.Notes = []chart.Note{{At: time.Second, End: 5 * time.Second, Kind: "hold"}}
			s, err := piano.NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			s.Health = piano.NewHealth(3, 0)
			g := &scene{chart: c, session: s, speed: 1, timeline: clock.NewTimeline(0),
				source: &keyboard.Source{}, bindings: []binding{{code: 'D'}}, physical: []bool{false}, focused: true, record: "test"}
			g.box.Push(keyboard.Event{At: time.Second, Key: 'D', Down: true})
			if err := g.readInput(time.Second, true); err != nil {
				t.Fatal(err)
			}
			before := s.Score
			if err := g.pauseFor(2*time.Second, mode); err != nil {
				t.Fatal(err)
			}
			if err := g.updateInput(2*time.Second, mode != "focus"); err != nil {
				t.Fatal(err)
			}
			if s.Score != before || !s.Active(0) || !s.Held[0] || s.Health.Value != 10000 {
				t.Fatal("paused input broke the held note")
			}
			g.resume(3 * time.Second)
			if mode == "released" {
				g.box.Push(keyboard.Event{At: 4 * time.Second, Key: 'D'})
			}
			if mode == "countdown" {
				// Release and re-hold queued on the final countdown frame.
				g.box.Push(keyboard.Event{At: 6 * time.Second, Key: 'D'})
				g.box.Push(keyboard.Event{At: 6 * time.Second, Key: 'D', Down: true})
			}
			if err := g.updateInput(6*time.Second, true); err != nil {
				t.Fatal(err)
			}
			if mode == "released" {
				if g.paused || s.Held[0] || s.Active(0) || s.Score.Miss != 1 {
					t.Fatal("resume retained a note without its key held")
				}
				if len(g.recording.Events) != 2 || g.recording.Events[1].At != 2*time.Second || g.recording.Events[1].Down {
					t.Fatalf("missing resume release: %+v", g.recording.Events)
				}
				return
			}
			if g.paused || s.Score != before || !s.Active(0) || !s.Held[0] {
				t.Fatal("resume broke the held note")
			}
			g.box.Push(keyboard.Event{At: 9 * time.Second, Key: 'D'})
			if err := g.readInput(9*time.Second, true); err != nil {
				t.Fatal(err)
			}
			if s.Score.Kool != 2 || s.Score.Miss != 0 || s.Health.Value != 10000 {
				t.Fatalf("tail after resume: %+v", s.Score)
			}
			if len(g.recording.Events) != 2 || g.recording.Events[1].At != 5*time.Second {
				t.Fatalf("pause changed recorded input: %+v", g.recording.Events)
			}
			if err := s.Advance(6 * time.Second); err != nil {
				t.Fatal(err)
			}
			r, err := piano.NewChartReplay(c, piano.Rules)
			if err != nil {
				t.Fatal(err)
			}
			r.Events, r.Duration, r.Pauses = g.recording.Events, 6*time.Second, g.recording.Pauses
			if err := r.Validate(); err != nil {
				t.Fatal(err)
			}
			replayed, err := piano.NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range r.Events {
				if err := replayed.Input(e.At, int(e.Key), e.Down); err != nil {
					t.Fatal(err)
				}
			}
			if err := replayed.Advance(r.Duration); err != nil || replayed.Score != s.Score {
				t.Fatalf("replay differs: %+v, %v", replayed.Score, err)
			}
		})
	}
}
