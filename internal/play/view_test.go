package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestPresentationDoesNotChangeJudgement(t *testing.T) {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 1010 * time.Millisecond, End: 1010 * time.Millisecond, Lane: 1, Kind: "tap"}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for lane := 0; lane < 2; lane++ {
		if err := s.Input(1030*time.Millisecond, lane, true); err != nil {
			t.Fatal(err)
		}
	}
	before := s.Score
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	p := &view{session: s, field: newField(c, c, ""), flashes: make([]time.Duration, 2), lit: make([]time.Duration, 2), hits: make([]time.Duration, 2), held: make([]bool, 2), released: make([]time.Duration, 2), hud: hud{results: -1}}
	p.Update(viewState{Position: 1030 * time.Millisecond, Rate: 1, Scroll: 110})
	if p.hud.scoreShown != s.Score.Points {
		t.Fatal("score display did not update immediately")
	}
	if p.judgement.Grade != piano.Cool {
		t.Fatal("simultaneous chord depends on lane order")
	}
	p.Update(viewState{Position: 1030 * time.Millisecond, Rate: 2, Scroll: 300, Paused: true})
	if p.Phase() != Paused || p.meter.count != 2 || s.Score != before || len(s.Results) != 2 {
		t.Fatal("presentation generated a result or duplicate sample")
	}
	for _, n := range []int{3, 2, 1} {
		p.Update(viewState{Position: 1030 * time.Millisecond, Rate: 2, Scroll: 300, Paused: true, Countdown: n})
		if p.Phase() != Paused || p.meter.count != 2 || s.Score != before || len(s.Results) != 2 {
			t.Fatal("countdown changed judgement or timing samples")
		}
	}
	after, err := c.Digest()
	if err != nil || after != hash {
		t.Fatal("presentation changed source chart")
	}
}
