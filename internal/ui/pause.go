package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const Width, Height = 1280, 720

// Buttons use the menu screens' theme (decisions 28/30, osu!lazer ShearedButton in scenes/theme.css):
// sheared by 0.2 with the label upright, a 2 px border lighter at the bottom, and hover
// brightening by a 10% white layer over 150 ms OutQuint.
const (
	pauseLeft, pauseWidth = 510, 260
	pauseTop, pauseHeight = 310, 48
	pauseStep             = 64
	pauseShear            = 0.2
	pauseBorder           = 2
	hoverDuration         = 150 * time.Millisecond
	fadeDuration          = 200 * time.Millisecond
)

// theme.css :root colours.
var (
	themeText        = color.NRGBA{0xec, 0xec, 0xec, 255}
	themeDim         = color.NRGBA{0x8a, 0x8a, 0x8a, 255}
	themeButton      = color.NRGBA{0x39, 0x44, 0x46, 255}
	themeButtonLight = color.NRGBA{0x5c, 0x6c, 0x70, 255}
	themeAccent      = color.NRGBA{0x00, 0xad, 0xd8, 255}
)

func pauseRow(i int) image.Rectangle {
	y := pauseTop + i*pauseStep
	return image.Rect(pauseLeft, y, pauseLeft+pauseWidth, y+pauseHeight)
}

func ResumeBounds() image.Rectangle  { return pauseRow(0) }
func RestartBounds() image.Rectangle { return pauseRow(1) }
func ReturnBounds() image.Rectangle  { return pauseRow(2) }

// PauseButtons lists the pause buttons in display order; arrow keys move through it.
func PauseButtons() []image.Rectangle {
	return []image.Rectangle{ResumeBounds(), RestartBounds(), ReturnBounds()}
}

// PauseMenu keeps the overlay's real-time motion; game time stands still while paused.
// The zero value is a closed menu: the next DrawPause fades it in.
type PauseMenu struct {
	opened   time.Time
	selected int
	hover    [3]motion
	since    [3]time.Time
}

// motion is one OutQuint tween in real time.
type motion struct {
	from, to float64
	d        time.Duration
}

func (m motion) at(age time.Duration) float64 {
	if m.d <= 0 || age >= m.d {
		return m.to
	}
	t := float64(age) / float64(m.d)
	return m.from + (m.to-m.from)*(1-math.Pow(1-t, 5))
}

// Close resets the menu so its next appearance fades in again.
func (m *PauseMenu) Close() { *m = PauseMenu{} }

// DrawPause draws only the overlay. The mode handles its buttons and timing.
// selected is the index into PauseButtons chosen by keyboard or mouse; info is the line under the buttons.
func DrawPause(dst *ebiten.Image, t Text, m *PauseMenu, countdown, selected int, info string) {
	now := time.Now()
	if countdown > 0 {
		m.Close()
		vector.FillRect(dst, 0, 0, Width, Height, color.NRGBA{4, 7, 14, 178}, false)
		t.Center(dst, fmt.Sprint(countdown), Combo, Width/2, 340, themeText)
		t.Center(dst, "Resuming - Esc to stay paused", Regular, Width/2, 382, themeDim)
		return
	}
	if m.opened.IsZero() {
		m.opened, m.selected = now, -1
	}
	if selected != m.selected {
		for i := range m.hover {
			to := 0.0
			if i == selected {
				to = .1
			} else if i != m.selected {
				continue
			}
			m.hover[i] = motion{m.hover[i].at(now.Sub(m.since[i])), to, hoverDuration}
			m.since[i] = now
		}
		m.selected = selected
	}
	// Fades in over osu!lazer GameplayMenuOverlay's 200 ms, but OutQuint so it shows from the first frame.
	fade := motion{0, 1, fadeDuration}.at(now.Sub(m.opened))
	alpha := func(c color.NRGBA, a float64) color.NRGBA {
		c.A = uint8(float64(c.A) * a * fade)
		return c
	}

	// Dim on the first paused frame; only the menu follows the fade-in.
	vector.FillRect(dst, 0, 0, Width, Height, color.NRGBA{A: 191}, false)
	t.Center(dst, "paused", Large, Width/2, float32(pauseTop-36), alpha(themeAccent, 1))
	for i, label := range []string{"Continue", "Retry", "Quit"} {
		r := pauseRow(i)
		l, rt := float32(r.Min.X), float32(r.Max.X)
		top, bottom := float32(r.Min.Y), float32(r.Max.Y)
		cy := (top + bottom) / 2
		sheared(dst, l, rt, cy, top, bottom, alpha(themeButton, 1))
		sheared(dst, l, rt, cy, bottom-pauseBorder, bottom, alpha(themeButtonLight, 1))
		if h := m.hover[i].at(now.Sub(m.since[i])); h > 0 {
			sheared(dst, l, rt, cy, top, bottom, alpha(color.NRGBA{255, 255, 255, 255}, h))
		}
		t.Center(dst, label, Regular, float32(r.Min.X+r.Dx()/2), top+30, alpha(themeText, 1))
	}
	t.Center(dst, info, Small, Width/2, float32(pauseRow(2).Max.Y+48), alpha(themeDim, 1))
}

var pausePixel = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
}()

// sheared fills the band from top to bottom of a button spanning l..r around its middle cy,
// leaning like skewX(-11.31deg): the top runs ahead of the bottom.
func sheared(dst *ebiten.Image, l, r, cy, top, bottom float32, c color.NRGBA) {
	v := func(x, y float32) ebiten.Vertex {
		return ebiten.Vertex{DstX: x + pauseShear*(cy-y), DstY: y, SrcX: 1, SrcY: 1,
			ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(c.A) / 255}
	}
	vs := []ebiten.Vertex{v(l, top), v(r, top), v(l, bottom), v(r, bottom)}
	dst.DrawTriangles(vs, []uint16{0, 1, 2, 1, 3, 2}, pausePixel, &ebiten.DrawTrianglesOptions{AntiAlias: true})
}
