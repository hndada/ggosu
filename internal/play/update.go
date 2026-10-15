package play

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ggosu/internal/clock"
	"ggosu/internal/settings"
)

func (g *scene) Update() error {
	defer func(t time.Time) { zzUpdate(t, time.Since(t)) }(time.Now())
	if g.err != nil {
		return g.err
	}
	if err := g.windowControls(); err != nil {
		return err
	}
	g.volumeControls(g.clock.Now())
	g.trackCursor()
	if inpututil.IsKeyJustPressed(ebiten.KeyF12) {
		g.screenshot = true
	}
	g.updateToasts(g.clock.Now())
	if err := g.updateChat(); err != nil {
		return err
	}
	if g.finishing {
		if g.ended && g.pressed(ebiten.KeyEscape) {
			g.skipOutro(g.clock.Now())
		}
		if g.ended && g.clock.Now()-g.endedAt < endHold+endFade {
			return nil
		}
		done, err := g.networkFinished()
		if err != nil {
			return err
		}
		if !done {
			return nil
		}
		if g.capture != "" && !g.captured {
			return nil
		}
		return ebiten.Termination
	}
	if g.stopRequested || (g.limit > 0 && g.frames >= g.limit) {
		if g.networkEnabled() && g.started {
			g.completed, g.finishing = false, true
			return nil
		}
		return ebiten.Termination
	}
	now := g.clock.Now()
	if waiting, err := g.networkUpdate(now); err != nil || waiting {
		return err
	}
	if !g.started {
		g.started = true
		g.startAt = g.gameStart(now)
		g.timeline = clock.NewTimeline(g.startAt)
		if err := g.timeline.SetRate(g.startAt, g.speed); err != nil {
			return err
		}
		g.last = -g.preroll
		if g.hudEdit && g.view != nil {
			g.openEditor()
		}
	}
	focused := ebiten.IsFocused() || g.limit > 0 || g.runFor > 0
	if err := g.beforeUpdate(now); err != nil {
		return err
	}
	// Pause only when focus is lost; the new window may not own focus on its first frames.
	if !focused && g.focused && !g.networkEnabled() && !g.hudEdit {
		if err := g.pauseFor(now, "focus"); err != nil {
			return err
		}
	}
	if err := g.updateInput(now, focused); err != nil {
		return err
	}
	if err := g.controls(now, focused); err != nil {
		return err
	}
	if err := g.updatePauseLoop(now, focused); err != nil {
		return err
	}
	if g.stopRequested || g.finishing {
		return nil
	}
	if g.player != nil {
		var err error
		g.due, err = g.player.AppendDue(g.due[:0], g.position)
		if err != nil {
			return err
		}
		for _, e := range g.due {
			if err := g.input(e.At, int(e.Key), e.Down); err != nil {
				return err
			}
		}
	}
	if err := g.session.Advance(g.position); err != nil {
		return err
	}
	g.last = g.position
	if err := g.networkAdvance(); err != nil {
		return err
	}
	if err := g.playAudio(); err != nil {
		return err
	}
	if err := g.finish(now); err != nil {
		return err
	}
	if g.view != nil {
		s := viewState{Position: g.position, Rate: g.speed, Scroll: g.scroll * settings.PixelsPerScroll, Paused: g.paused, Countdown: g.countdown(now), PauseChoice: g.pauseChoice, Peek: g.paused && g.held(ebiten.KeyTab) && !g.held(ebiten.KeyShift), Retries: g.retries, Finished: g.completed, Aborted: g.aborted}
		// Chat exists only in a multiplayer room; solo play shows no drawer.
		g.networkState(&s)
		g.view.Update(s)
	}
	return nil
}

// osu!lazer Player: the finished frame stays for RESULTS_DISPLAY_DELAY, then the
// screen fades out over 250 ms before the results screen.
const endHold, endFade = 1000 * time.Millisecond, 250 * time.Millisecond

func (g *scene) outro() bool {
	return !g.editing && !g.hudEdit && !g.preview && !g.replayIncomplete && g.session.Done() && g.position >= max(g.chart.Duration(), g.session.Chart.Duration())
}

// afterNotes reports a play whose last note has ended and been judged, even
// while trailing audio or storyboard still runs.
func (g *scene) afterNotes() bool {
	if g.editing || g.hudEdit || g.preview || g.replayIncomplete || !g.session.Judged() {
		return false
	}
	var end time.Duration
	for _, n := range g.session.Chart.Notes {
		end = max(end, n.At, n.End)
	}
	return g.position >= end
}

// skipOutro keeps a fully judged play complete and starts its fade immediately.
// Backdating the hold leaves the existing real-time transition deadline intact.
func (g *scene) skipOutro(now time.Duration) bool {
	if !g.outro() {
		return false
	}
	if g.ended && now-g.endedAt >= endHold {
		return true // Repeated Escape presses must not restart an active fade.
	}
	if g.bank != nil {
		g.bank.Close()
	}
	g.paused, g.resumeAt = false, 0
	g.completed, g.finishing, g.ended = true, true, true
	g.endedAt = now - endHold
	if g.view != nil {
		s := g.view.state
		s.Position, s.Paused, s.Finished, s.Countdown = g.position, false, true, 0
		g.view.Update(s)
	}
	return true
}

// endShade is how dark the end fade has made the screen, 0..1.
func (g *scene) endShade() float32 {
	if !g.ended {
		return 0
	}
	return float32(min(1, max(0, float64(g.clock.Now()-g.endedAt-endHold)/float64(endFade))))
}

func (g *scene) finish(now time.Duration) error {
	// A practice section plays again from its start until the player leaves.
	if g.sectionEnd != 0 && !g.paused && (g.position >= g.sectionStop || g.session.Health.Failed) {
		// -duration counts from the first start, not from each loop.
		start := g.startAt
		if err := g.restart(now); err != nil {
			return err
		}
		g.startAt = start
	}
	if g.abortDue() {
		g.aborted = true
	}
	if (g.session.Health.Failed || g.aborted) && !g.hudEdit {
		g.completed, g.finishing, g.ended = false, true, true
		g.endedAt = now
		if g.bank != nil {
			g.bank.Close()
		}
		return nil
	}
	if !g.paused && g.session.Done() && g.position >= max(g.chart.Duration(), g.session.Chart.Duration()) && (g.bank == nil || !g.bank.Playing()) {
		if !g.finishCue {
			g.finishCue = true
			if g.bank != nil {
				if err := g.bank.Effect("finish", 1); err != nil {
					return err
				}
			}
		}
		if g.bank == nil || !g.bank.Playing() {
			g.completed = true
			if g.hudEdit {
				return g.loop(now)
			}
			if (g.limit == 0 && g.runFor == 0 || g.networkEnabled()) && g.networkFinishReady() {
				g.finishing = true
				if !g.ended {
					g.ended, g.endedAt = true, now
				}
			}
		}
	}
	if !g.paused && g.replayIncomplete && g.position >= g.replayEnd {
		g.completed = false
		g.finishing = true
	}
	if g.runFor > 0 && now-g.startAt >= g.runFor {
		g.finishing = true
	}
	return nil
}

// abortDue is ONGEKI's バトル中断設定 on the ghost target: a live play ends, like a
// quit without a record, once even all remaining KOOLs cannot reach the target.
func (g *scene) abortDue() bool {
	return g.display.Abort && g.mode == "live" && !g.hudEdit && g.sectionEnd == 0 && !g.networkEnabled() &&
		g.session.Score.MaxPoints() < g.display.TargetScore()
}

// The HUD editor plays the chart again until it is closed, keeping the draft.
func (g *scene) loop(now time.Duration) error {
	g.display = g.view.hudDisplay(g.display)
	// -duration counts from the first start, not from each loop.
	start := g.startAt
	if err := g.restart(now); err != nil {
		return err
	}
	g.startAt = start
	g.view.editing = true
	return nil
}
