package piano

import (
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func o2Chart(notes int) *chart.Chart {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := 1; i <= notes; i++ {
		at := time.Duration(i) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Kind: "tap"})
	}
	return c
}

func o2Session(t *testing.T, c *chart.Chart, m chart.Mod) *Session {
	t.Helper()
	s, err := NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	s.SetO2Jam(m)
	return s
}

func tap(t *testing.T, s *Session, at time.Duration) {
	t.Helper()
	if err := s.Input(at, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Input(at+time.Millisecond, 0, false); err != nil {
		t.Fatal(err)
	}
}

// At 120 BPM a beat is 500 ms: KOOL ±66.4 ms, COOL ±166 ms, GOOD ±265.6 ms.
func TestO2JamBeatWindows(t *testing.T) {
	offsets := []time.Duration{-60, 66, -70, 165, -170, 265}
	want := []Grade{Kool, Kool, Cool, Cool, Good, Good}
	c := o2Chart(len(offsets))
	s := o2Session(t, c, chart.Mod{O2Jam: "normal"})
	for i, d := range offsets {
		tap(t, s, time.Duration(i+1)*time.Second+d*time.Millisecond)
	}
	for i, r := range s.Results {
		if r.Grade != want[i] {
			t.Fatal(i, r.Grade, want[i])
		}
	}
	// Outside 0.8 beat the press is ignored and the note misses after the window.
	s = o2Session(t, o2Chart(1), chart.Mod{O2Jam: "normal"})
	tap(t, s, time.Second-267*time.Millisecond)
	if err := s.Advance(time.Second + 265*time.Millisecond); err != nil || len(s.Results) != 0 {
		t.Fatal("early press judged", s.Results, err)
	}
	if err := s.Advance(time.Second + 266*time.Millisecond); err != nil || len(s.Results) != 1 || s.Results[0].Grade != Miss {
		t.Fatal("late note did not miss", s.Results, err)
	}
	// Doubling the BPM halves every window.
	c = o2Chart(1)
	c.Timing[0].BPM = 240
	s = o2Session(t, c, chart.Mod{O2Jam: "normal"})
	tap(t, s, time.Second+40*time.Millisecond)
	if s.Results[0].Grade != Cool {
		t.Fatal("240 BPM window", s.Results[0].Grade)
	}
}

func TestO2JamLifeEndsTheGame(t *testing.T) {
	for _, tc := range []struct {
		level  string
		misses int
	}{{"easy", 17}, {"normal", 25}, {"hard", 34}} {
		s := o2Session(t, o2Chart(40), chart.Mod{O2Jam: tc.level})
		if err := s.Advance(50 * time.Second); err != nil {
			t.Fatal(err)
		}
		if !s.Health.Failed || len(s.Results) != tc.misses || s.Health.Life != 0 || s.Health.Value != 0 {
			t.Fatal(tc.level, len(s.Results), s.Health)
		}
	}
	// Hard recovers half as much: 48 of 48000 is 0.1%.
	s := o2Session(t, o2Chart(2), chart.Mod{O2Jam: "hard"})
	if err := s.Advance(time.Second + 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	tap(t, s, 2*time.Second)
	if s.Health.Life != 48000-1440+48 || s.Health.Value != 9710 {
		t.Fatal(s.Health)
	}
}

func TestO2JamGoodBreaksCombo(t *testing.T) {
	s := o2Session(t, o2Chart(3), chart.Mod{O2Jam: "easy"})
	tap(t, s, time.Second)
	tap(t, s, 2*time.Second+200*time.Millisecond)
	tap(t, s, 3*time.Second)
	if s.Score.Good != 1 || s.Score.Combo != 1 || s.Score.MaxCombo != 1 || s.Health.Life != 24000-240+96 {
		t.Fatal(s.Score, s.Health)
	}
	if err := s.Score.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestO2JamPills(t *testing.T) {
	c := o2Chart(17)
	off := o2Session(t, c, chart.Mod{O2Jam: "easy"})
	on := o2Session(t, c, chart.Mod{O2Jam: "easy", Pills: true})
	for _, s := range []*Session{off, on} {
		for i := 1; i <= 15; i++ {
			tap(t, s, time.Duration(i)*time.Second)
		}
	}
	if off.Pills() != -1 || on.Pills() != 1 {
		t.Fatal(off.Pills(), on.Pills())
	}
	tap(t, on, 16*time.Second+200*time.Millisecond)
	tap(t, on, 17*time.Second+200*time.Millisecond)
	if on.Pills() != 0 || on.Results[15].Grade != Cool || on.Results[16].Grade != Good || on.Score.Combo != 0 || on.Score.MaxCombo != 16 {
		t.Fatal(on.Pills(), on.Results[15:], on.Score)
	}
}

func TestO2JamReplayRecord(t *testing.T) {
	c := o2Chart(20)
	m := chart.Mod{O2Jam: "normal", Pills: true}
	s := o2Session(t, c, m)
	var events []Event
	for i := 1; i <= 18; i++ {
		at := time.Duration(i)*time.Second + time.Duration(i%3)*90*time.Millisecond
		events = append(events, Event{At: at, Down: true}, Event{At: at + time.Millisecond})
		tap(t, s, at)
	}
	if err := s.Advance(25 * time.Second); err != nil {
		t.Fatal(err)
	}
	replay, err := NewChartReplay(c, Rules)
	if err != nil {
		t.Fatal(err)
	}
	replay.Mods, replay.Health, replay.HealthGauge = m, 2, O2JamGauge
	replay.Duration, replay.Events = 25*time.Second, events
	record := Record{Version: RecordVersion, ID: "o2jam", Date: time.Now(), Rate: 1, Mode: "live", Completed: true, Replay: replay}
	got, _, err := record.Recalculate(c)
	if err != nil || got.Score != s.Score || got.Health == nil || *got.Health != s.Health {
		t.Fatal(got.Score, s.Score, got.Health, s.Health, err)
	}
	if err := SaveRecord(filepath.Join(t.TempDir(), "record.json"), got); err != nil {
		t.Fatal(err)
	}
	if _, ok := PP(got); Summarize([]Record{got})[0].BestPoints != nil || ok {
		t.Fatal("event mod record entered best points or pp")
	}
	replay.Mods.O2Jam = "hard"
	if replay.Validate() == nil {
		t.Fatal("O2Jam level and health disagree")
	}
}
