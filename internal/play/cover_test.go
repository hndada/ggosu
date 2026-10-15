package play

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/settings"
)

// The cover jumps on the first frame, then halves its distance every 25 ms of real time.
func TestCoverMotionDamps(t *testing.T) {
	var m coverMotion
	c := settings.Cover{Sudden: .2, SuddenOn: true}
	m.update(c, 0, 1)
	if m.sudden != .2 || m.topAlpha != 1 || !m.visible() {
		t.Fatalf("first frame %+v", m)
	}
	c.Sudden = .4
	m.update(c, 50*time.Millisecond, 2) // 25 ms of real time at 2x
	if math.Abs(m.sudden-.3) > 1e-9 {
		t.Fatalf("half time %+v", m)
	}
	c.SuddenOn = false
	m.update(c, time.Hour, 1)
	if m.visible() {
		t.Fatalf("switched off %+v", m)
	}
	m.update(settings.Cover{Hidden: .5, HiddenOn: true}, 0, 1) // a restart rewinds
	top, bottom := m.bounds(400)
	if m.hidden != .5 || top[0] != 0 || bottom[0] != 200 || bottom[1] != 100 || bottom[2] != 1 {
		t.Fatalf("rewind %+v %v %v", m, top, bottom)
	}
}

func TestCoverText(t *testing.T) {
	if got := coverText(settings.Cover{Sudden: .205, SuddenOn: true, Lift: .1, LiftOn: true, Hidden: .3}); got != "SUDDEN+ 20.5%  LIFT 10%" {
		t.Fatalf("%q", got)
	}
}
