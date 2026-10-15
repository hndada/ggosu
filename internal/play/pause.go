package play

import (
	"ggosu/internal/piano"
	"time"
)

const resumeDelay = 3 * time.Second

func (g *scene) logPause(at time.Duration, action, reason string) {
	g.recording.Pauses = append(g.recording.Pauses, piano.Pause{At: g.position, Elapsed: max(0, at-g.startAt), Action: action, Reason: reason, Rate: g.speed})
}

func (g *scene) pause(at time.Duration) error { return g.pauseFor(at, "manual") }

func (g *scene) pauseFor(at time.Duration, reason string) error {
	// Judgement is over: leave the remaining audio and result transition running.
	if g.outro() {
		return nil
	}
	if g.resumeAt != 0 {
		g.logPause(at, "resume-cancel", reason)
	}
	g.resumeAt = 0
	if g.paused {
		return nil
	}
	at = max(at, g.startAt)
	g.position = max(g.last, g.timeline.Position(at)-g.preroll)
	if err := g.timeline.SetRate(at, 0); err != nil {
		return err
	}
	g.last, g.paused, g.pauseChoice = g.position, true, 0
	g.logPause(at, "pause", reason)
	if g.bank != nil {
		g.bank.Pause(true)
	}
	return nil
}

func (g *scene) resume(at time.Duration) { g.resumeFor(at, "manual") }

func (g *scene) resumeFor(at time.Duration, reason string) {
	if g.paused && g.resumeAt == 0 {
		g.resumeAt = at + resumeDelay
		g.logPause(at, "resume-request", reason)
	}
}

func (g *scene) updateResume(now time.Duration, focused bool) error {
	if g.resumeAt == 0 {
		return nil
	}
	if !focused {
		g.logPause(now, "resume-cancel", "focus")
		g.resumeAt = 0
		return nil
	}
	if now < g.resumeAt {
		return nil
	}
	// Resume at this update rather than backdating a stalled/background frame.
	if err := g.timeline.SetRate(now, g.speed); err != nil {
		return err
	}
	g.logPause(now, "resume", "countdown-complete")
	g.resumeAt, g.inputSince, g.paused = 0, now, false
	if g.bank != nil {
		g.bank.Pause(false)
		return g.bank.Effect("resume", 1)
	}
	return nil
}

func (g *scene) countdown(now time.Duration) int {
	if g.resumeAt <= now {
		return 0
	}
	return int((g.resumeAt - now + time.Second - 1) / time.Second)
}
