package play

import (
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/piano"
)

func (p *view) Press(lane int, at time.Duration) { p.flashes[lane] = at }

// keyState returns how long ago the lane's key went down and, once it is up, how long ago it came up.
func (p *view) keyState(k int) (down bool, pressAge, releaseAge time.Duration) {
	now := p.state.Position
	down = p.session.Held[k]
	press, release := p.flashes[k], p.released[k]
	if !down && release < press {
		// A tap that went down and up between two frames.
		release = press + 50*time.Millisecond
	}
	return down, now - press, now - release
}

// The column lights up in 50 ms, stays bright while held and fades only on release.
func (p *view) columnLight(k int) float64 {
	down, pressAge, releaseAge := p.keyState(k)
	if pressAge < 0 {
		return 0
	}
	if down {
		return tween(pressAge, 50*time.Millisecond, 0, 1)
	}
	// Start fading from the brightness reached when the key came up.
	at := tween(pressAge-max(releaseAge, 0), 50*time.Millisecond, 0, 1)
	return tween(releaseAge, 250*time.Millisecond, at, 0)
}

func (p *view) keyLight(k int) (alpha, white float64) {
	down, pressAge, releaseAge := p.keyState(k)
	if pressAge < 0 || !down && pressAge > time.Hour {
		return 0, 0
	}
	alpha = tween(pressAge, 70*time.Millisecond, 0, 1)
	if pressAge > 70*time.Millisecond {
		alpha = 1 - 0.2*min(float64(pressAge-70*time.Millisecond)/float64(500*time.Millisecond), 1)
	}
	white = tween(pressAge, 70*time.Millisecond, 0, 1)
	if down {
		return alpha, white
	}
	if releaseAge < 50*time.Millisecond {
		alpha = tween(releaseAge, 50*time.Millisecond, alpha, 0.3)
	} else {
		alpha = tween(releaseAge-50*time.Millisecond, 800*time.Millisecond, 0.3, 0)
	}
	return alpha, tween(releaseAge, 800*time.Millisecond, 1, 0)
}

func (p *view) drawField(dst *ebiten.Image) {
	l := p.field.layout
	p.drawStageLight(dst)
	p.notes.Clear()
	if p.masked() {
		for k, lane := range l.lanes {
			x := lane.x - l.left
			p.drawColumnLight(k, x, lane)
			p.drawHitTarget(x, lane)
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(l.left), float64(l.top))
		dst.DrawImage(p.notes, op)
		p.notes.Clear()
	}
	p.drawBarLines()
	var held uint64 // lanes holding a long note
	for k, lane := range l.lanes {
		x := lane.x - l.left
		if !p.masked() {
			p.drawColumnLight(k, x, lane)
			p.drawHitTarget(x, lane)
		}
		h := p.noteHeight(lane.width)
		w := p.field.windows[k]
		list := p.field.notes[k]
		// Descending source order lets nearby heads cover farther bodies without
		// allocating or sorting an index list for each frame.
		for j := w.last - 1; j >= w.first; j-- {
			v := list[j]
			if !p.session.Visible(v.note) {
				continue
			}
			n := p.session.Chart.Notes[v.note]
			y := p.field.y(v.head, p.state.Scroll) - l.top
			tail := p.field.y(v.tail, p.state.Scroll) - l.top
			c := lane.color
			if p.snap != nil {
				c = p.snap[v.note]
			}
			alpha := float32(1)
			switch n.Kind {
			case "mine":
				c = gradeColor(piano.Miss)
			case "lift":
				c = color.NRGBA{106, 210, 248, 255}
			case "fake":
				alpha = 90.0 / 255
			case "roll":
				c = color.NRGBA{208, 136, 241, 255}
			}
			if n.End > n.At {
				if p.session.Failed(v.note) {
					c, alpha = mix(c, muted, 0.85), 0.35
				}
				bottom := min(l.line-l.top, y) - h/2
				top := max(float32(0), tail-h/2)
				holding := p.session.Active(v.note) && p.session.Held[k] && p.state.Position < n.End
				if holding {
					held |= 1 << k
				}
				if bottom > top {
					p.drawBody(x, top, lane.width, bottom-top, c, alpha, holding)
					if holding {
						p.drawHoldGlow(x, top, lane.width, bottom-top, lane.color)
					}
				}
				p.drawTail(x, tail, lane.width, c, alpha)
				p.drawPiece(headPiece, x, min(y, l.line-l.top), lane.width, c, alpha)
			} else {
				p.drawPiece(notePiece, x, y, lane.width, c, alpha)
			}
			if n.Kind == "lift" || n.Kind == "mine" {
				rect(p.notes, x+lane.width*.4, y-h-4, lane.width*.2, 8, c)
			}
			if p.config.Debug {
				p.drawStrain(v.note, x+lane.width/2, min(y, l.line-l.top)-h-2)
			}
		}
		if a := tailOpacity(p.state.Position-p.tails[k].at, p.state.Rate); a > 0 && !p.masked() {
			p.drawTail(x, l.line-l.top, lane.width, lane.color, a)
		}
	}
	p.drawNotes(dst)
	if p.masked() {
		p.notes.Clear()
		for k, lane := range l.lanes {
			if a := tailOpacity(p.state.Position-p.tails[k].at, p.state.Rate); a > 0 {
				p.drawTail(lane.x-l.left, l.line-l.top, lane.width, lane.color, a)
			}
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(l.left), float64(l.top))
		dst.DrawImage(p.notes, op)
	}
	for k, lane := range l.lanes {
		p.drawKeys(dst, k, lane)
		if p.skin == "o2jam" && held&(1<<k) != 0 {
			p.drawO2Flare(dst, k, lane)
		}
	}
	for k, lane := range l.lanes {
		if age := p.state.Position - p.hits[k]; age >= 0 {
			p.drawExplosion(dst, lane, age)
		}
	}
}

// drawBarLines follows osu!lazer's ArgonBarLine: a white line across the stage whose bottom sits on the bar
// time, 1.7 units at 50% for a major bar and 1.2 units at 20% otherwise.
func (p *view) drawBarLines() {
	l := p.field.layout
	for _, b := range p.field.bars {
		y := p.field.y(b.position, p.state.Scroll) - l.top
		if y < 0 || y > l.line-l.top {
			continue
		}
		h, a := 1.2*argonScale, uint8(51)
		if b.major {
			h, a = 1.7*argonScale, 128
		}
		rect(p.notes, 0, y-h, l.width, h, color.NRGBA{255, 255, 255, a})
	}
}

type tailFade struct{ at time.Duration }

// Successful releases dissolve over 180 real milliseconds, independent of TPS.
func tailOpacity(age time.Duration, rate float64) float32 {
	if rate <= 0 || age < 0 {
		return 0
	}
	t := float64(age) / (rate * float64(180*time.Millisecond))
	if t >= 1 {
		return 0
	}
	return float32((1 - t) * (1 - t))
}

// The default key bed continues each lane below the hit line. Its flat face
// lights on press; small floating icons do not compete with incoming notes.
func (p *view) drawKeyArea(dst *ebiten.Image, k int, lane lane) {
	l := p.field.layout
	s := argonScale
	alpha, white := p.keyLight(k)
	if alpha > 0 {
		c := darken(lane.color, 0.2)
		if _, pressAge, _ := p.keyState(k); pressAge < 200*time.Millisecond {
			c = mix(lighten(lane.color, 0.8), c, outQuint(float64(pressAge)/float64(200*time.Millisecond)))
		}
		fill(dst, lane.x, l.line, lane.width, l.bottom-l.line, withAlpha(c, alpha), false)
	}
	lighting := mix(lane.color, color.NRGBA{255, 255, 255, 255}, 0.2)
	lineH := argonRadius * 2
	if white > 0 {
		glow(dst, lane.x, l.line, lane.width, lineH, lighting, 20*float64(s), 0.4*white)
	}
	gray := color.NRGBA{196, 196, 196, 255}
	fill(dst, lane.x, l.line, lane.width, lineH, mix(gray, color.NRGBA{255, 255, 255, 255}, white), false)
	// The key stretches to the bottom of the frame instead of stopping short of it.
	h := l.bottom - l.line - lineH
	if h <= 2 {
		return
	}
	x, y, w := lane.x+1, l.line+lineH, lane.width-2
	// A small downward travel reads as a key press while the judgement line stays fixed.
	y += float32(2 * white)
	h -= float32(2 * white)
	top := mix(darken(lane.color, 0.82), lane.color, 0.65*alpha)
	bottom := color.NRGBA{13, 16, 22, 255}
	gradient(dst, x, y, w, h, top, bottom, false)
	fill(dst, x, y, w, 2, withAlpha(mix(lane.color, color.NRGBA{255, 255, 255, 255}, white), 0.35+0.65*alpha), false)
	fill(dst, x, y+h-3, w, 3, color.NRGBA{5, 7, 11, 255}, false)
}
