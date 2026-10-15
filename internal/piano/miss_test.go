package piano

import (
	"reflect"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestSingleMissPositionAndFrameCadence(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := range 100 {
		at := time.Duration(i+1) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: i % 4, Kind: "tap"})
	}
	auto, err := Auto(c, Rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                          string
		miss, points, combo, maxCombo int
	}{
		{"first", 0, 889_000, 99, 99},
		{"middle", 49, 889_000, 50, 50},
		{"late", 75, 934_500, 24, 75},
		{"last", 99, 1_089_000, 0, 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := auto
			f.Events = nil
			at := c.Notes[tc.miss].At
			for _, e := range auto.Events {
				if e.At != at && e.At != at+time.Nanosecond {
					f.Events = append(f.Events, e)
				}
			}
			var want Score
			var results []Result
			for _, tick := range []time.Duration{time.Millisecond, 17 * time.Millisecond, 250 * time.Millisecond, 5 * time.Second} {
				s, err := NewSession(c)
				if err != nil {
					t.Fatal(err)
				}
				p, err := NewPlayer(f)
				if err != nil {
					t.Fatal(err)
				}
				var due []Event
				for now := time.Duration(0); ; now += tick {
					now = min(now, f.Duration)
					due, err = p.AppendDue(due[:0], now)
					if err != nil {
						t.Fatal(err)
					}
					for _, e := range due {
						if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
							t.Fatal(err)
						}
					}
					if err := s.Advance(now); err != nil {
						t.Fatal(err)
					}
					if now == f.Duration {
						break
					}
				}
				if !s.Done() || s.Score.Kool != 99 || s.Score.Miss != 1 || s.Score.Total != 100 || s.Score.Points != tc.points || s.Score.Combo != tc.combo || s.Score.MaxCombo != tc.maxCombo || s.Score.Accuracy() != .99 {
					t.Fatalf("tick %v: %+v", tick, s.Score)
				}
				if err := s.Score.Validate(); err != nil {
					t.Fatal(err)
				}
				if want.Total == 0 {
					want, results = s.Score, s.Results
				} else if want != s.Score || !reflect.DeepEqual(results, s.Results) {
					t.Fatalf("MISS score or order depends on Update cadence: %v", tick)
				}
			}
			t.Logf("MISS at judgement %d: %d points, 99 KOOL / 1 MISS", tc.miss+1, want.Points)
			replayed, _, err := ReplayScore(c, f)
			if err != nil || replayed != want {
				t.Fatalf("verification changed MISS ordering: %+v %v", replayed, err)
			}
		})
	}
}

func TestHoldMissScore(t *testing.T) {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: 2 * time.Second, Kind: "hold"}, {At: 3 * time.Second, End: 3 * time.Second, Lane: 1, Kind: "tap"}}
	for _, tc := range []struct {
		name               string
		events             []Event
		kool, miss, points int
	}{
		{"untouched_head", nil, 1, 2, 43_000},
		{"head_in_miss_band", []Event{{At: 1110 * time.Millisecond, Key: 0, Down: true}, {At: 2 * time.Second, Key: 0}}, 1, 2, 43_000},
		{"early_head_in_miss_band", []Event{{At: 890 * time.Millisecond, Key: 0, Down: true}, {At: 2 * time.Second, Key: 0}}, 1, 2, 43_000},
		{"early_release", []Event{{At: time.Second, Key: 0, Down: true}, {At: 1500 * time.Millisecond, Key: 0}}, 2, 1, 409_666},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range tc.events {
				if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Input(3*time.Second, 1, true); err != nil {
				t.Fatal(err)
			}
			if err := s.Advance(4 * time.Second); err != nil {
				t.Fatal(err)
			}
			if !s.Done() || s.Score.Kool != tc.kool || s.Score.Miss != tc.miss || s.Score.Points != tc.points || s.Score.Total != 3 {
				t.Fatalf("hold MISS allowed tail score or incorrect recovery: %+v", s.Score)
			}
			if err := s.Score.Validate(); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %d points, %d KOOL / %d MISS", tc.name, s.Score.Points, s.Score.Kool, s.Score.Miss)
		})
	}
}

func TestMissedLongHeadCannotRecoverAtTail(t *testing.T) {
	for _, kind := range []string{"hold", "roll"} {
		t.Run(kind, func(t *testing.T) {
			c := chart.New(1)
			c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
			c.Notes = []chart.Note{{At: time.Second, End: 2 * time.Second, Kind: kind}}
			s, err := NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Input(1110*time.Millisecond, 0, true); err != nil {
				t.Fatal(err)
			}
			if s.Score.Miss != 2 || s.Score.Points != 0 || len(s.Results) != 2 || !s.Results[1].Tail {
				t.Fatalf("head MISS did not fail the tail immediately: %+v", s.Score)
			}
			before := s.Score
			for at := 1200 * time.Millisecond; at <= 2300*time.Millisecond; at += 100 * time.Millisecond {
				if err := s.Input(at, 0, false); err != nil {
					t.Fatal(err)
				}
				if err := s.Input(at+time.Nanosecond, 0, true); err != nil {
					t.Fatal(err)
				}
			}
			if s.Score != before || !s.Done() {
				t.Fatalf("holding or rolling revived a missed head: %+v", s.Score)
			}
		})
	}
}

func TestFailedLongNoteRemainsVisibleUntilEnd(t *testing.T) {
	for _, kind := range []string{"hold", "roll"} {
		for _, broken := range []bool{false, true} {
			c := chart.New(1)
			c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
			c.Notes = []chart.Note{{At: time.Second, End: 3 * time.Second, Kind: kind}}
			s, err := NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			if broken {
				if err := s.Input(time.Second, 0, true); err != nil {
					t.Fatal(err)
				}
				if !s.Active(0) {
					t.Fatal("successful head did not activate")
				}
				if kind == "hold" {
					if err := s.Input(1500*time.Millisecond, 0, false); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := s.Advance(2 * time.Second); err != nil {
				t.Fatal(err)
			}
			if !s.Visible(0) || !s.Failed(0) || s.Active(0) {
				t.Fatalf("%s broken=%v: missing failed note or still active", kind, broken)
			}
			score := s.Score
			if err := s.Input(2100*time.Millisecond, 0, false); err != nil {
				t.Fatal(err)
			}
			if err := s.Input(2200*time.Millisecond, 0, true); err != nil {
				t.Fatal(err)
			}
			if s.Active(0) || s.Score != score {
				t.Fatal("failed long note reactivated")
			}
			if err := s.Advance(3 * time.Second); err != nil {
				t.Fatal(err)
			}
			if s.Visible(0) || s.Score != score {
				t.Fatal("failed note outlived its tail or changed score")
			}
		}
	}
}

func TestHoldRequiresReleaseAndKeepsLegacyRule(t *testing.T) {
	for _, rules := range []string{Rules, StrictReleaseRules, LegacyRules} {
		c := chart.New(1)
		c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
		c.Notes = []chart.Note{{At: time.Second, End: 2 * time.Second, Kind: "hold"}}
		s, err := NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		s.Rules = rules
		if err := s.Input(time.Second, 0, true); err != nil {
			t.Fatal(err)
		}
		if err := s.Advance(2 * time.Second); err != nil {
			t.Fatal(err)
		}
		if rules != LegacyRules && len(s.Results) != 1 {
			t.Fatal("held tail auto-completed")
		}
		if err := s.Advance(2*time.Second + Window + time.Nanosecond); err != nil {
			t.Fatal(err)
		}
		want := Miss
		if rules == LegacyRules {
			want = Kool
		}
		if len(s.Results) != 2 || s.Results[1].Grade != want {
			t.Fatal(rules, s.Results)
		}
	}
	for _, delta := range []time.Duration{-45 * time.Millisecond, 0, 45 * time.Millisecond} {
		c := chart.New(1)
		c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
		c.Notes = []chart.Note{{At: time.Second, End: 2 * time.Second, Kind: "hold"}}
		s, err := NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Input(time.Second, 0, true); err != nil {
			t.Fatal(err)
		}
		if err := s.Input(2*time.Second+delta, 0, false); err != nil {
			t.Fatal(err)
		}
		if len(s.Results) != 2 || s.Results[1].Grade == Miss {
			t.Fatal("valid release missed", s.Results)
		}
	}
}

func TestHoldReleaseWindowIsLenient(t *testing.T) {
	for _, tc := range []struct {
		rules string
		delta time.Duration
		want  Grade
	}{
		{Rules, 33 * time.Millisecond, Kool},
		{Rules, -60 * time.Millisecond, Cool},
		{Rules, 120 * time.Millisecond, Good},
		{Rules, -120 * time.Millisecond, Good},
		{Rules, -140 * time.Millisecond, Miss},
		{StrictReleaseRules, 120 * time.Millisecond, Miss},
		{StrictReleaseRules, 33 * time.Millisecond, Cool},
	} {
		c := chart.New(1)
		c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
		c.Notes = []chart.Note{{At: time.Second, End: 2 * time.Second, Kind: "hold"}}
		s, err := NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		s.Rules = tc.rules
		if err := s.Input(time.Second, 0, true); err != nil {
			t.Fatal(err)
		}
		if err := s.Input(2*time.Second+tc.delta, 0, false); err != nil {
			t.Fatal(err)
		}
		if len(s.Results) != 2 || s.Results[1].Grade != tc.want {
			t.Fatal(tc.rules, tc.delta, s.Results)
		}
	}
}
