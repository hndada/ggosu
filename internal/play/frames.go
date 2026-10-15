package play

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// frameMeter uses wall time, so pauses and playback rates cannot hide stalls.
type frameMeter struct {
	last, sampled, checked time.Time
	warningUntil           time.Time
	count                  int
	fps, refresh           float64
	warning                int
	limitText, text        string
}

func frameLevel(fps, target float64, gap time.Duration, typical float64) int {
	if target <= 0 {
		target = 60 // No numeric cap exists in unlimited mode or on unknown displays.
	}
	budget := 1 / target
	if typical > 0 {
		budget = min(budget, 1/typical)
	}
	switch {
	case fps > 0 && fps < target*.75 || gap.Seconds() > budget*2.5:
		return 2
	case fps > 0 && fps < target*.95 || gap.Seconds() > budget*1.5:
		return 1
	default:
		return 0
	}
}

func frameColour(level int) color.NRGBA {
	return [...]color.NRGBA{{116, 220, 151, 255}, {242, 193, 95, 255}, {245, 112, 125, 255}}[level]
}

func (f *frameMeter) sample(now time.Time, unlimited bool) int {
	if f.last.IsZero() {
		f.last, f.sampled = now, now
		f.text = "--  --ms"
		return 0
	}
	gap := now.Sub(f.last)
	f.last = now
	f.count++
	if elapsed := now.Sub(f.sampled); elapsed >= 250*time.Millisecond {
		f.fps = float64(f.count) / elapsed.Seconds()
		f.count, f.sampled = 0, now
		f.text = fmt.Sprintf("%.0f  %.1fms", f.fps, 1000/f.fps)
	}
	target := f.refresh
	if unlimited {
		target = 60
	}
	level := frameLevel(f.fps, target, gap, f.fps)
	if level > 0 && (level >= f.warning || !now.Before(f.warningUntil)) {
		f.warning, f.warningUntil = level, now.Add(500*time.Millisecond)
	}
	if now.Before(f.warningUntil) {
		level = max(level, f.warning)
	}
	return level
}

func (p *view) drawFrames(dst *ebiten.Image) {
	f := &p.frames
	now := time.Now()
	unlimited := p.config.Display != nil && p.config.Display.FrameLimit == "unlimited"
	if f.checked.IsZero() || now.Sub(f.checked) >= time.Second {
		f.checked = now
		f.refresh = monitorHz()
		f.limitText = ""
		if !unlimited && f.refresh > 0 {
			f.limitText = fmt.Sprintf("/%.0f", f.refresh)
		}
	}
	c := frameColour(f.sample(now, unlimited))
	// A compact, always-visible counter, including focus HUD and pause/outro.
	// One line: fps, refresh cap (absent when unlimited), frame time.
	text := f.text
	if i := strings.IndexByte(text, ' '); i > 0 && f.limitText != "" {
		text = text[:i] + f.limitText + text[i:]
	}
	p.hud.Right(dst, text, small, Width-12, Height-14, c)
}
