package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestHealthMotionDamageRecoveryAndPause(t *testing.T) {
	h := newHealthMotion(10000)
	h.update(10000, time.Second)
	h.update(8800, time.Second+10*time.Millisecond)
	if h.value <= 8800 || h.value >= 10000 || h.trail != 10000 || h.loss != h.at {
		t.Fatal("damage must ease with the old fill retained", h)
	}
	before := h
	h.update(8800, h.at)
	if h != before {
		t.Fatal("paused motion advanced", before, h)
	}
	h.update(8800, time.Second+170*time.Millisecond)
	if h.trail != 10000 {
		t.Fatal("damage trail drained before its delay", h)
	}
	h.update(8800, 2*time.Second)
	if h.trail >= 10000 || h.trail <= h.value {
		t.Fatal("damage trail must catch the live fill", h)
	}
	h.update(9600, 2*time.Second+20*time.Millisecond)
	if h.gain != h.at || h.value <= 8800 || h.value >= 9600 || h.trail < h.value {
		t.Fatal("recovery must rise without leaving a gap", h)
	}
	h.update(9600, h.at+20*time.Millisecond)
	if h.glow <= h.value || h.glow >= 9600 {
		t.Fatal("the glow must lead the rising fill", h)
	}
	h.update(0, 3*time.Second)
	if h.value != 0 || h.trail <= 0 {
		t.Fatal("failure must empty the live fill while retaining its trail", h)
	}
	h.update(10000, time.Second)
	if h.value != 10000 || h.trail != 10000 || healthPulse(h.at-h.loss, time.Second) != 0 {
		t.Fatal("rewind retained a stale effect", h)
	}
}

func TestHealthStageChangeDrainsFromFullToInheritedFill(t *testing.T) {
	p := editView(t, 4)
	p.session.Health = piano.NewHealth(3, 0)
	p.Update(viewState{Position: time.Second, Rate: 1, Scroll: 110})
	p.session.Health.Level, p.session.Health.Value = 2, 2800
	p.Update(viewState{Position: 2 * time.Second, Rate: 1, Scroll: 110})
	if p.health.level != 2 || p.health.value != 10000 || p.health.trail != 10000 || p.health.loss != p.health.at || p.health.gain > 0 {
		t.Fatal("stage change must start from a full bar as damage", p.health)
	}
	p.Update(viewState{Position: 2*time.Second + 50*time.Millisecond, Rate: 1, Scroll: 110})
	if p.health.value <= 7600 || p.health.value >= 10000 {
		t.Fatal("stage change must drain toward the inherited fill, slower than damage", p.health)
	}
	p.Update(viewState{Position: 4 * time.Second, Rate: 1, Scroll: 110})
	if p.health.level != 2 || p.health.value > 2801 {
		t.Fatal("stage change did not settle on the inherited fill", p.health)
	}
}

func TestHealthBeatThumpsOnEveryBeat(t *testing.T) {
	timing := []chart.Timing{{At: 100 * time.Millisecond, BPM: 120}}
	if got := healthBeat(timing, 600*time.Millisecond); got != 8 {
		t.Fatal("the fill must jump on the beat", got)
	}
	if got := healthBeat(timing, 650*time.Millisecond); got <= 0 || got >= 8 {
		t.Fatal("the bump must settle after the beat", got)
	}
	if got := healthBeat(timing, 400*time.Millisecond); got != 0 {
		t.Fatal("the fill must rest between beats", got)
	}
	if got := healthBeat(timing, 50*time.Millisecond); got != 0 {
		t.Fatal("before the first timing point the beat grid runs backwards", got)
	}
	if healthBeat(nil, time.Second) != 0 {
		t.Fatal("a chart without timing must not thump")
	}
}
