package play

import (
	"fmt"
	"math"
	"strings"
	"time"

	"ggosu/internal/settings"
)

// coverHalfTime and coverGradient follow osu!lazer's PlayfieldCoveringWrapper: the shown cover
// eases toward its setting with Interpolation.DampContinuously (25 ms half time), and a
// gradient a quarter of the field high softens its edge.
const (
	coverHalfTime = 25 * time.Millisecond
	coverGradient = .25
)

// coverMotion is the drawn SUDDEN+ and HIDDEN+: their fractions of the lane above the
// (lifted) judgement line and the gradient strength, which fades out with a cover switched off.
type coverMotion struct {
	sudden, hidden        float64
	topAlpha, bottomAlpha float64
	at                    time.Duration
	started               bool
}

func damp(current, target float64, elapsed time.Duration) float64 {
	return target + (current-target)*math.Pow(.5, float64(elapsed)/float64(coverHalfTime))
}

// update moves toward c over real time; the first call and any rewind (restart) jump there.
func (m *coverMotion) update(c settings.Cover, at time.Duration, rate float64) {
	sudden, hidden, _ := c.Amounts()
	top, bottom := 0.0, 0.0
	if sudden > 0 {
		top = 1
	}
	if hidden > 0 {
		bottom = 1
	}
	elapsed := time.Duration(float64(at-m.at) / max(rate, .01))
	if !m.started || elapsed < 0 {
		m.sudden, m.hidden, m.topAlpha, m.bottomAlpha = sudden, hidden, top, bottom
	} else {
		m.sudden, m.hidden = damp(m.sudden, sudden, elapsed), damp(m.hidden, hidden, elapsed)
		m.topAlpha, m.bottomAlpha = damp(m.topAlpha, top, elapsed), damp(m.bottomAlpha, bottom, elapsed)
	}
	m.at, m.started = at, true
}

func (m coverMotion) visible() bool {
	return m.sudden > 1e-4 || m.hidden > 1e-4 || m.topAlpha > 1e-3 || m.bottomAlpha > 1e-3
}

// bounds gives the mask sides for a field h pixels from the top to the judgement line:
// SUDDEN+ covers from the top, HIDDEN+ up from the line, as beatoraja's LaneRenderer.
func (m coverMotion) bounds(h float32) (top, bottom []float32) {
	fade := h * coverGradient
	top = []float32{h * float32(m.sudden), fade, float32(m.topAlpha)}
	bottom = []float32{h * float32(1-m.hidden), fade, float32(m.bottomAlpha)}
	return
}

// liftLine raises the judgement line by LIFT's share of the lane, as beatoraja's LaneRenderer.
// The HUD editor places the unlifted hit position, so it sees no lift.
func (p *view) liftLine(line float32) float32 {
	p.lifted = p.lift()
	return line - (line-p.field.layout.top)*float32(p.lifted)
}

func (p *view) lift() float64 {
	if p.editing || p.config.Profile == nil {
		return 0
	}
	_, _, lift := p.config.Profile.LaneCover().Amounts()
	return lift
}

// visibleShare is the part of the lane below SUDDEN+, which beatoraja counts as the green
// number; HIDDEN+ and LIFT do not change it.
func (p *view) visibleShare() float64 {
	if p.config.Profile == nil {
		return 1
	}
	sudden, _, _ := p.config.Profile.LaneCover().Amounts()
	return 1 - sudden
}

// coverText lists the covers in use above the scroll readout, in percent of the lane.
func coverText(c settings.Cover) string {
	var parts []string
	for _, part := range []struct {
		name string
		on   bool
		v    float64
	}{{"SUDDEN+", c.SuddenOn, c.Sudden}, {"HIDDEN+", c.HiddenOn, c.Hidden}, {"LIFT", c.LiftOn, c.Lift}} {
		if part.on {
			parts = append(parts, fmt.Sprintf("%s %g%%", part.name, math.Round(part.v*1000)/10))
		}
	}
	return strings.Join(parts, "  ")
}
