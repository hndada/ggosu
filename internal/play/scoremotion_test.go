package play

import (
	"testing"
	"time"

	"ggosu/internal/piano"
)

func TestScoreTweenRetargetsWithoutJumpAndSettlesExactly(t *testing.T) {
	var h hud
	h.updateScore(0, 0, false)
	h.updateScore(100000, time.Second, false)
	if h.scoreShown != 0 {
		t.Fatal("new target jumped", h.scoreShown)
	}
	h.updateScore(100000, time.Second+60*time.Millisecond, false)
	shown := h.scoreShown
	if shown <= 0 || shown >= 100000 {
		t.Fatal("score did not interpolate", shown)
	}
	h.updateScore(200000, time.Second+60*time.Millisecond, false)
	if h.scoreShown != shown {
		t.Fatal("retarget jumped", h.scoreShown, shown)
	}
	h.updateScore(200000, time.Second+120*time.Millisecond, false)
	if h.scoreShown <= shown || h.scoreShown >= 200000 {
		t.Fatal("new score did not continue counting", h.scoreShown)
	}
	h.updateScore(200000, time.Second+240*time.Millisecond, false)
	if h.scoreShown != 200000 || h.scoreText != "0200000" {
		t.Fatal("score did not settle", h.scoreShown, h.scoreText)
	}
}

func TestScoreTweenUsesTimeAndResetsOnRewindOrFinish(t *testing.T) {
	var sparse, frequent hud
	for _, h := range []*hud{&sparse, &frequent} {
		h.updateScore(0, 0, false)
		h.updateScore(1100000, time.Second, false)
	}
	for ms := 10; ms < 90; ms += 10 {
		frequent.updateScore(1100000, time.Second+time.Duration(ms)*time.Millisecond, false)
	}
	for _, h := range []*hud{&sparse, &frequent} {
		h.updateScore(1100000, time.Second+90*time.Millisecond, false)
	}
	if sparse.scoreShown != frequent.scoreShown {
		t.Fatal("animation depends on update count", sparse.scoreShown, frequent.scoreShown)
	}
	frequent.updateScore(1234, 0, false)
	if frequent.scoreShown != 1234 {
		t.Fatal("rewind kept old score", frequent.scoreShown)
	}
	frequent.updateScore(5000, time.Second, false)
	frequent.updateScore(5000, time.Second, true)
	if frequent.scoreShown != 5000 {
		t.Fatal("final frame kept an intermediate score", frequent.scoreShown)
	}
}

func TestScoreTweenLeavesSessionScoreAndAccuracyImmediate(t *testing.T) {
	p := &view{session: &piano.Session{}, hud: hud{results: -1}}
	p.hud.update(p)
	p.session.Score = piano.Score{Points: 550000, Kool: 1, Total: 2, Possible: 2, Combo: 1}
	want := p.session.Score
	p.state.Position = time.Second
	p.hud.update(p)
	if p.hud.scoreShown != 0 || p.hud.score != want || p.hud.comboText != "1" || p.hud.counts[0] != "1" {
		t.Fatal("score motion delayed the other feedback", p.hud.scoreShown, p.hud.score)
	}
	p.state.Position += 60 * time.Millisecond
	p.hud.update(p)
	if p.hud.scoreShown <= 0 || p.hud.scoreShown >= want.Points || p.session.Score != want {
		t.Fatal("animation changed gameplay score", p.hud.scoreShown, p.session.Score)
	}
}
