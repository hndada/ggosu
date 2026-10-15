package piano

import (
	"reflect"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func fixture() *chart.Chart {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Lane: 0, Kind: "tap"}, {At: time.Second, End: 2 * time.Second, Lane: 1, Kind: "hold"}, {At: 3 * time.Second, End: 3 * time.Second, Lane: 2, Kind: "mine"}, {At: 4 * time.Second, End: 5 * time.Second, Lane: 3, Kind: "roll"}, {At: 6 * time.Second, End: 6 * time.Second, Lane: 0, Kind: "lift"}}
	return c
}
func TestReplayFrameIndependence(t *testing.T) {
	c := fixture()
	f, err := Auto(c, Rules)
	if err != nil {
		t.Fatal(err)
	}
	var want Score
	var results []Result
	for _, tick := range []time.Duration{time.Millisecond, 17 * time.Millisecond, 250 * time.Millisecond, 7 * time.Second} {
		s, err := NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		p, err := NewPlayer(f)
		if err != nil {
			t.Fatal(err)
		}
		for at := time.Duration(0); ; at += tick {
			at = min(at, f.Duration)
			events, err := p.AppendDue(nil, at)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range events {
				if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Advance(at); err != nil {
				t.Fatal(err)
			}
			if at == f.Duration {
				break
			}
		}
		if !s.Done() || s.Score.Miss != 0 || s.Score.Total != 7 {
			t.Fatalf("tick %v: %+v", tick, s.Score)
		}
		if want.Total == 0 {
			want = s.Score
			results = s.Results
		} else if s.Score != want || !reflect.DeepEqual(results, s.Results) {
			t.Fatalf("frame-dependent result: tick %v", tick)
		}
	}
}
func TestEarlyHoldReleaseAndRepeat(t *testing.T) {
	c := fixture()
	s, err := NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct {
		at   time.Duration
		down bool
	}{{time.Second, true}, {time.Second, true}, {1500 * time.Millisecond, false}} {
		if err := s.Input(e.at, 1, e.down); err != nil {
			t.Fatal(err)
		}
	}
	if s.Score.Kool != 1 || s.Score.Miss != 2 {
		t.Fatalf("repeat or early release: %+v", s.Score)
	}
	if err := s.Advance(7 * time.Second); err != nil {
		t.Fatal(err)
	}
	score := s.Score
	if err := s.Advance(8 * time.Second); err != nil {
		t.Fatal(err)
	}
	if s.Score != score {
		t.Fatal("miss counted twice")
	}
}
func TestMineAndBackwards(t *testing.T) {
	s, err := NewSession(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Input(2900*time.Millisecond, 2, true); err != nil {
		t.Fatal(err)
	}
	miss := s.Score.Miss
	if err := s.Advance(3100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if s.Score.Miss != miss {
		t.Fatal("mine counted twice")
	}
	if s.Advance(time.Second) == nil {
		t.Fatal("accepted reverse time")
	}
}

func TestMissOrderAcrossFrames(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 500 * time.Millisecond, End: 2 * time.Second, Lane: 3, Kind: "hold"}, {At: 3 * time.Second, End: 3 * time.Second, Lane: 0, Kind: "tap"}}
	var want Score
	var results []Result
	for _, tick := range []time.Duration{time.Millisecond, 250 * time.Millisecond, 4 * time.Second} {
		s, err := NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Input(500*time.Millisecond, 3, true); err != nil {
			t.Fatal(err)
		}
		for at := 500*time.Millisecond + tick; at < 4*time.Second; at += tick {
			if err := s.Advance(at); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Advance(4 * time.Second); err != nil {
			t.Fatal(err)
		}
		if want.Total == 0 {
			want = s.Score
			results = s.Results
		} else if want != s.Score || !reflect.DeepEqual(results, s.Results) {
			t.Fatalf("frame-dependent missed-note order: %v %+v", tick, s.Score)
		}
	}
}

func TestHitErrorAveragesHitsAndSkipsMisses(t *testing.T) {
	s := &Session{Results: []Result{{Delta: 30 * time.Millisecond, Grade: Kool}, {Delta: -10 * time.Millisecond, Grade: Good}, {Delta: 135 * time.Millisecond, Grade: Miss}}}
	if e := s.HitError(); e == nil || e.Count != 2 || e.MeanMS != 10 || e.Fast != 1 || e.Slow != 0 {
		t.Fatalf("hit error %+v", e)
	}
	s.Results = append(s.Results, Result{Delta: 40 * time.Millisecond, Grade: Cool})
	if e := s.HitError(); e.Fast != 1 || e.Slow != 1 {
		t.Fatalf("fast/slow excludes kool %+v", e)
	}
	if (&Session{}).HitError() != nil {
		t.Fatal("empty session has a hit error")
	}
}
