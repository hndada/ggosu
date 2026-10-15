package play

import (
	"time"

	"ggosu/internal/clock"
)

// restart reuses decoded assets and keeps the selected chart and random seed.
func (g *scene) restart(now time.Duration) error {
	if g.networkEnabled() {
		return nil
	}
	if err := g.prepare(); err != nil {
		return err
	}
	if g.player != nil {
		g.player.Reset()
	}
	if g.bank != nil {
		g.bank.Close()
		g.bank.Pause(false)
		if err := g.prepareSectionMusic(); err != nil {
			return err
		}
	}
	g.retries++
	g.startAt, g.inputSince = now, now
	g.timeline = clock.NewTimeline(now)
	if err := g.timeline.SetRate(now, g.speed); err != nil {
		return err
	}
	g.position, g.last = -g.preroll, -g.preroll
	g.paused, g.completed, g.finishing, g.aborted = false, false, false, false
	g.resumeAt = 0
	g.startCue, g.finishCue = false, false
	g.lastCombo, g.comboBroken = 0, false
	g.ended, g.endedAt = false, 0
	if g.view != nil {
		return g.initView()
	}
	return nil
}

// prepareSectionMusic schedules a practice section's background audio so a start
// mid-song seeks long sounds instead of playing them from their beginning.
func (g *scene) prepareSectionMusic() error {
	if g.sectionEnd == 0 || g.bank == nil {
		return nil
	}
	return g.bank.PrepareMusic(g.session.Chart.Audio)
}
