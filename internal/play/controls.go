package play

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ggosu/internal/settings"
	"ggosu/internal/ui"
)

// retryHold follows lazer's default UIHoldActivationDelay for its quick retry overlay.
const retryHold = 200 * time.Millisecond

// holdRetry restarts once backquote or Ctrl+R has been held for retryHold, like
// osu!'s quick retry. Another retry needs the key released first.
func (g *scene) holdRetry(now time.Duration) (bool, error) {
	return g.retry(now, g.held(ebiten.KeyBackquote) || g.held(ebiten.KeyControl) && g.held(ebiten.KeyR))
}

func (g *scene) retry(now time.Duration, down bool) (bool, error) {
	if !down || g.networkEnabled() {
		g.retryHeld, g.retryLatched = false, false
		return false, nil
	}
	if g.retryLatched {
		return false, nil
	}
	if !g.retryHeld {
		g.retryHeld, g.retryAt = true, now
		return false, nil
	}
	if now-g.retryAt < retryHold {
		return false, nil
	}
	g.retryHeld, g.retryLatched = false, true
	return true, g.restart(now)
}

// retryShade darkens the screen while the retry key is held.
func (g *scene) retryShade() float64 {
	if !g.retryHeld || g.clock == nil {
		return 0
	}
	return min(1, max(0, float64(g.clock.Now()-g.retryAt)/float64(retryHold)))
}

func (g *scene) controls(now time.Duration, focused bool) error {
	if g.preview {
		g.editHUD(focused)
		return nil
	}
	if g.editing {
		if g.pressed(ebiten.KeyF9) {
			g.commitHUD()
			return nil
		}
		g.editHUD(focused)
		return nil
	}
	if retry, err := g.holdRetry(now); retry || err != nil {
		return err
	}
	if g.view != nil {
		if g.hudShortcut() {
			g.view.hideHUD = !g.view.hideHUD
		}
	}
	escape, pause := g.pressed(ebiten.KeyEscape), g.pressed(ebiten.KeyP)
	if escape && g.skipOutro(now) {
		return nil
	}
	// Past the last note Escape no longer pauses; trailing audio plays out.
	if escape && !g.paused && !g.networkEnabled() && g.afterNotes() {
		escape = false
	}
	if escape && g.networkEnabled() {
		g.stopRequested = true
	}
	if (escape || pause) && !g.networkEnabled() {
		if !g.paused || g.resumeAt != 0 {
			reason := "key-p"
			if escape {
				reason = "escape"
			}
			if err := g.pauseFor(now, reason); err != nil {
				return err
			}
		} else if focused {
			g.resumeFor(now, "keyboard")
		}
	}
	// Hovering selects only when the mouse moves, so it does not fight the arrow keys.
	point := g.cursor()
	moved := point != g.pauseCursor
	g.pauseCursor = point
	if g.view != nil && g.paused && g.resumeAt == 0 {
		buttons := ui.PauseButtons()
		if !ebiten.IsKeyPressed(ebiten.KeyAlt) && g.pressed(ebiten.KeyArrowUp) {
			g.pauseChoice = (g.pauseChoice + len(buttons) - 1) % len(buttons)
		}
		if !ebiten.IsKeyPressed(ebiten.KeyAlt) && g.pressed(ebiten.KeyArrowDown) {
			g.pauseChoice = (g.pauseChoice + 1) % len(buttons)
		}
		hovered := -1
		for i, b := range buttons {
			if point.In(b) {
				hovered = i
			}
		}
		if hovered >= 0 && moved {
			g.pauseChoice = hovered
		}
		choose := !ebiten.IsKeyPressed(ebiten.KeyAlt) && (g.pressed(ebiten.KeyEnter) || g.pressed(ebiten.KeyNumpadEnter))
		if hovered >= 0 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			g.pauseChoice, choose = hovered, true
		}
		if choose {
			switch g.pauseChoice {
			case 0:
				if focused {
					g.resumeFor(now, "pause-menu")
				}
			case 1:
				return g.restart(now)
			case 2:
				g.stopRequested = true
			}
		}
	}
	if g.pressed(ebiten.KeyF6) && g.mode == "replay" && g.view != nil {
		g.view.SetFocus(!g.view.Focus())
	}
	if g.pressed(ebiten.KeyF7) && g.mode == "replay" && !g.networkEnabled() {
		g.practice = true
		switch g.speed {
		case .5:
			g.speed = 1
		case 1:
			g.speed = 1.5
		case 1.5:
			g.speed = 2
		default:
			g.speed = .5
		}
		if !g.paused {
			if err := g.timeline.SetRate(now, g.speed); err != nil {
				return err
			}
		}
		if g.bank != nil {
			if err := g.bank.SetRate(g.speed); err != nil {
				return err
			}
		}
	}
	g.coverControls(now)
	faster, slower := g.pressed(ebiten.KeyPageUp), g.pressed(ebiten.KeyPageDown)
	if !g.held(ebiten.KeyShift) {
		faster = g.pressed(ebiten.KeyF3) || faster
		slower = g.pressed(ebiten.KeyF4) || slower
	}
	if g.held(ebiten.KeyControl) {
		faster = g.pressed(ebiten.KeyEqual) || g.pressed(ebiten.KeyNumpadAdd) || faster
		slower = g.pressed(ebiten.KeyMinus) || g.pressed(ebiten.KeyNumpadSubtract) || slower
	}
	if faster {
		g.scrollUntil = now + 1500*time.Millisecond
		if g.profile.ScrollMode == "exposure" {
			g.profile.ExposureMS = max(100, g.profile.ExposureMS-25)
			g.resetSpeedShare()
		} else {
			g.scroll = min(settings.MaxScroll, math.Round((g.scroll+g.scrollStep(ebiten.KeyPageUp))*10)/10)
			g.profile.Scroll = g.scroll
		}
	}
	if slower {
		g.scrollUntil = now + 1500*time.Millisecond
		if g.profile.ScrollMode == "exposure" {
			g.profile.ExposureMS = min(5000, g.profile.ExposureMS+25)
			g.resetSpeedShare()
		} else {
			g.scroll = max(settings.MinScroll, math.Round((g.scroll-g.scrollStep(ebiten.KeyPageDown))*10)/10)
			g.profile.Scroll = g.scroll
		}
	}
	return nil
}

// coverControls follows beatoraja's ControlInputProcessor with keyboard keys: Shift+F4 switches
// SUDDEN+ like a double START, Up/Down move the cover 1% and Ctrl+wheel 0.5% per notch.
// Plain wheel stays with the chat. A change shows in the scroll readout.
func (g *scene) coverControls(now time.Duration) {
	if g.view == nil || g.paused || ebiten.IsKeyPressed(ebiten.KeyAlt) {
		return
	}
	c := g.profile.LaneCover()
	before := c
	if g.coverShortcut() {
		c.SuddenOn = !c.SuddenOn
	}
	if g.pressed(ebiten.KeyArrowUp) {
		c.Adjust(-.01)
	}
	if g.pressed(ebiten.KeyArrowDown) {
		c.Adjust(.01)
	}
	if _, dy := ebiten.Wheel(); dy != 0 && ebiten.IsKeyPressed(ebiten.KeyControl) {
		c.Adjust(-dy * .005)
	}
	if c != before {
		g.profile.Cover = &c
		g.scrollUntil = now + 1500*time.Millisecond
	}
}

// resetSpeedShare makes a new exposure time count the lane below the current SUDDEN+.
func (g *scene) resetSpeedShare() {
	if g.view != nil {
		g.view.speedSudden = 1 - g.view.visibleShare()
	}
}
