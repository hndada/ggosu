package play

import (
	"testing"
	"time"

	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestTimingText(t *testing.T) {
	for _, tc := range []struct {
		threshold string
		grades    []piano.Grade
	}{
		{"", []piano.Grade{piano.Cool, piano.Good}},
		{"kool", []piano.Grade{piano.Kool, piano.Cool, piano.Good}},
		{"cool", []piano.Grade{piano.Cool, piano.Good}},
		{"good", []piano.Grade{piano.Good}},
		{"off", nil},
	} {
		for _, grade := range []piano.Grade{piano.Kool, piano.Cool, piano.Good, piano.Miss} {
			for _, delta := range []time.Duration{-time.Millisecond, 0, time.Millisecond} {
				want := ""
				for _, shown := range tc.grades {
					if grade == shown && delta != 0 {
						want = "Late"
						if delta < 0 {
							want = "Early"
						}
					}
				}
				r := piano.Result{Grade: grade, Delta: delta}
				if got := timingText(r, tc.threshold); got != want {
					t.Errorf("threshold %q, %s, %v: %q, want %q", tc.threshold, grade, delta, got, want)
				}
			}
		}
	}
	if timingColor(-time.Millisecond) == timingColor(time.Millisecond) {
		t.Fatal("Early and Late have the same color")
	}
}

func TestTimingThresholdChangesWithoutNewJudgement(t *testing.T) {
	p := editView(t, 4)
	p.judgement = piano.Result{Grade: piano.Cool, Delta: -30 * time.Millisecond}
	before := p.session.Score
	for _, tc := range []struct{ threshold, want string }{{"good", ""}, {"off", ""}, {"kool", "Early"}, {"", "Early"}} {
		d := p.hudDisplay(settings.Display{})
		d.TimingThreshold = tc.threshold
		p.setHUD(d)
		if got := timingText(p.judgement, p.timingThreshold); got != tc.want {
			t.Fatal("stale timing threshold", tc.threshold, got)
		}
		if p.hudDisplay(d).TimingThreshold != p.timingThreshold || p.session.Score != before {
			t.Fatal("HUD save lost threshold or changed score")
		}
	}
}
