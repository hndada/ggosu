package play

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// osu!lazer's mania "argon" skin (osu.Game.Rulesets.Mania/Skinning/Argon), drawn at 720/768 of its
// 1024x768 reference size. Glows are approximated with a few faint additive layers.
const argonScale = float32(Height) / 768

var (
	argonNoteHeight = 42 * argonScale  // ArgonNotePiece.NOTE_HEIGHT
	argonRadius     = 3.4 * argonScale // ArgonNotePiece.CORNER_RADIUS
	argonHitTarget  = 110 * argonScale // Stage.HIT_TARGET_POSITION, measured from the bottom
	argonColumn     = 60 * argonScale  // ColumnWidth; the special (middle odd) column is twice as wide
)

const argonAccentRatio = 0.82 // ArgonNotePiece.NOTE_ACCENT_RATIO

// ManiaArgonSkinTransformer colours.
var (
	argonYellow  = color.NRGBA{255, 197, 40, 255}
	argonOrange  = color.NRGBA{252, 109, 1, 255}
	argonPink    = color.NRGBA{213, 35, 90, 255}
	argonPurple  = color.NRGBA{203, 60, 236, 255}
	argonCyan    = color.NRGBA{72, 198, 255, 255}
	argonGreen   = color.NRGBA{100, 192, 92, 255}
	argonSpecial = color.NRGBA{169, 106, 255, 255}
)

// argonLayouts is getColourForLayout for 1-10 columns.
var argonLayouts = [][]color.NRGBA{
	1:  {argonYellow},
	2:  {argonGreen, argonCyan},
	3:  {argonGreen, argonSpecial, argonCyan},
	4:  {argonYellow, argonOrange, argonPink, argonPurple},
	5:  {argonPink, argonOrange, argonYellow, argonGreen, argonCyan},
	6:  {argonPink, argonOrange, argonGreen, argonCyan, argonOrange, argonPink},
	7:  {argonPink, argonOrange, argonPink, argonSpecial, argonPink, argonOrange, argonPink},
	8:  {argonPurple, argonPink, argonOrange, argonGreen, argonCyan, argonOrange, argonPink, argonPurple},
	9:  {argonPurple, argonPink, argonOrange, argonYellow, argonSpecial, argonYellow, argonOrange, argonPink, argonPurple},
	10: {argonPurple, argonPink, argonOrange, argonYellow, argonGreen, argonCyan, argonYellow, argonOrange, argonPink, argonPurple},
}

func argonSpecialColumn(lanes, i int) bool { return lanes%2 == 1 && i == lanes/2 }

func argonColour(lanes, i int) color.NRGBA {
	if lanes < len(argonLayouts) {
		return argonLayouts[lanes][i]
	}
	if argonSpecialColumn(lanes, i) {
		return argonSpecial
	}
	return []color.NRGBA{argonYellow, argonOrange, argonPink, argonPurple, argonCyan, argonGreen}[i%6]
}

// osu!framework Color4.Darken / Lighten scale RGB by 1/(1+a) and (1+a).
func darken(c color.NRGBA, a float64) color.NRGBA {
	s := 1 / (1 + a)
	return color.NRGBA{uint8(float64(c.R) * s), uint8(float64(c.G) * s), uint8(float64(c.B) * s), c.A}
}

func lighten(c color.NRGBA, a float64) color.NRGBA {
	f := func(v uint8) uint8 { return uint8(min(255, float64(v)*(1+a))) }
	return color.NRGBA{f(c.R), f(c.G), f(c.B), c.A}
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}

func withAlpha(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(max(0, min(255, a*255)))
	return c
}

func outQuint(t float64) float64 { return 1 - math.Pow(1-min(max(t, 0), 1), 5) }

// tween runs from a to b over d starting at age 0 with OutQuint.
func tween(age, d time.Duration, a, b float64) float64 {
	if d <= 0 || age >= d {
		return b
	}
	return a + (b-a)*outQuint(float64(age)/float64(d))
}

var whitePixel = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
}()

// gradient fills a rectangle from the top colour to the bottom colour; additive uses lighter blending.
func gradient(dst *ebiten.Image, x, y, w, h float32, top, bottom color.NRGBA, additive bool) {
	if w <= 0 || h <= 0 || top.A == 0 && bottom.A == 0 {
		return
	}
	v := func(px, py float32, c color.NRGBA) ebiten.Vertex {
		a := float32(c.A) / 255
		return ebiten.Vertex{DstX: px, DstY: py, SrcX: 1, SrcY: 1, ColorR: float32(c.R) / 255 * a, ColorG: float32(c.G) / 255 * a, ColorB: float32(c.B) / 255 * a, ColorA: a}
	}
	vs := []ebiten.Vertex{v(x, y, top), v(x+w, y, top), v(x, y+h, bottom), v(x+w, y+h, bottom)}
	op := &ebiten.DrawTrianglesOptions{}
	if additive {
		op.Blend = ebiten.BlendLighter
	}
	dst.DrawTriangles(vs, []uint16{0, 1, 2, 1, 3, 2}, whitePixel, op)
}

func fill(dst *ebiten.Image, x, y, w, h float32, c color.NRGBA, additive bool) {
	gradient(dst, x, y, w, h, c, c, additive)
}

// glow stacks faint additive rectangles around a box, standing in for an EdgeEffect glow.
func glow(dst *ebiten.Image, x, y, w, h float32, c color.NRGBA, radius, strength float64) {
	for i, step := range []float64{0.2, 0.45, 0.75, 1} {
		g := float32(radius * step)
		fill(dst, x-g, y-g, w+2*g, h+2*g, withAlpha(c, strength*[]float64{0.35, 0.22, 0.12, 0.06}[i]), true)
	}
}

// ring and disc match vector.StrokeCircle and FillCircle with anti-aliasing, as plain triangles:
// vector paths are re-rasterized on every draw, which per-lane hit effects cannot afford at 240 FPS.
func ring(dst *ebiten.Image, cx, cy, r, width float32, c color.NRGBA) {
	inner, outer := r-width/2, r+width/2
	if width < 1 { // A hairline keeps one pixel of coverage, scaled by its width.
		c, inner, outer = withAlpha(c, float64(c.A)/255*float64(width)), r, r
	}
	annulus(dst, cx, cy, []float32{max(0, inner-.5), max(0, inner+.5), outer - .5, outer + .5}, []float32{0, 1, 1, 0}, c)
}

func disc(dst *ebiten.Image, cx, cy, r float32, c color.NRGBA) {
	if r < .5 {
		c, r = withAlpha(c, float64(c.A)/255*float64(2*r)), .5
	}
	annulus(dst, cx, cy, []float32{0, r - .5, r + .5}, []float32{1, 1, 0}, c)
}

// annulus fills concentric bands between radii, with alpha interpolated between neighbouring radii.
func annulus(dst *ebiten.Image, cx, cy float32, radii, alphas []float32, c color.NRGBA) {
	if c.A == 0 || radii[len(radii)-1] <= 0 {
		return
	}
	n := min(128, max(16, int(math.Ceil(float64(2*math.Pi*radii[len(radii)-1]/3)))))
	a := float32(c.A) / 255
	vs := make([]ebiten.Vertex, 0, (n+1)*len(radii))
	for i := 0; i <= n; i++ {
		sin, cos := math.Sincos(2 * math.Pi * float64(i) / float64(n))
		for j, r := range radii {
			k := a * alphas[j]
			vs = append(vs, ebiten.Vertex{DstX: cx + r*float32(cos), DstY: cy + r*float32(sin), SrcX: 1, SrcY: 1,
				ColorR: float32(c.R) / 255 * k, ColorG: float32(c.G) / 255 * k, ColorB: float32(c.B) / 255 * k, ColorA: k})
		}
	}
	m := uint16(len(radii))
	is := make([]uint16, 0, n*int(m-1)*6)
	for i := uint16(0); i < uint16(n); i++ {
		for j := uint16(0); j+1 < m; j++ {
			p, q := i*m+j, (i+1)*m+j
			is = append(is, p, q, p+1, p+1, q, q+1)
		}
	}
	dst.DrawTriangles(vs, is, whitePixel, &ebiten.DrawTrianglesOptions{})
}

// Sprites are drawn per pixel once per colour and width, then reused every frame.
type sprite func(px, py float64) color.NRGBA

type argonSprites struct {
	cache map[argonKey]*ebiten.Image
}

type argonKey struct {
	kind  uint8
	width int
	c     color.NRGBA
}

const (
	argonNote uint8 = iota
	argonHead
	argonTail
)

func (s *argonSprites) get(kind uint8, width float32, c color.NRGBA) *ebiten.Image {
	if s.cache == nil {
		s.cache = map[argonKey]*ebiten.Image{}
	}
	k := argonKey{kind, int(math.Round(float64(width))), c}
	if img, ok := s.cache[k]; ok {
		return img
	}
	img := renderSprite(kind, float64(k.width), c)
	s.cache[k] = img
	return img
}

// coverage of a rounded rectangle at a pixel centre, with one pixel of antialiasing.
func roundedCover(px, py, x, y, w, h, r float64) float64 {
	cx := max(x+r-px, 0, px-(x+w-r))
	cy := max(y+r-py, 0, py-(y+h-r))
	d := math.Hypot(cx, cy) - r
	if cx == 0 || cy == 0 {
		d = max(x-px, px-(x+w), y-py, py-(y+h))
	}
	return min(max(0.5-d, 0), 1)
}

func segmentDistance(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := min(max(((px-ax)*dx+(py-ay)*dy)/(dx*dx+dy*dy), 0), 1)
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// over composites src (straight alpha, coverage k) onto dst.
func over(dst, src color.NRGBA, k float64) color.NRGBA {
	a := float64(src.A) / 255 * k
	if a <= 0 {
		return dst
	}
	da := float64(dst.A) / 255
	oa := a + da*(1-a)
	f := func(s, d uint8) uint8 { return uint8((float64(s)*a + float64(d)*da*(1-a)) / oa) }
	return color.NRGBA{f(src.R, dst.R), f(src.G, dst.G), f(src.B, dst.B), uint8(oa * 255)}
}

func add(dst, src color.NRGBA, k float64) color.NRGBA {
	a := float64(src.A) / 255 * k
	f := func(s, d uint8) uint8 { return uint8(min(255, float64(d)+float64(s)*a)) }
	return color.NRGBA{f(src.R, dst.R), f(src.G, dst.G), f(src.B, dst.B), dst.A}
}

func renderArgon(kind uint8, w float64, accent color.NRGBA) *ebiten.Image {
	h := float64(argonNoteHeight)
	r := float64(argonRadius)
	H := int(math.Ceil(h))
	img := image.NewNRGBA(image.Rect(0, 0, int(w), H))
	top := h * (1 - argonAccentRatio)
	for iy := range H {
		for ix := range int(w) {
			px, py := float64(ix)+0.5, float64(iy)+0.5
			var c color.NRGBA
			// Shadow: the whole piece fades from clear to black, downwards.
			shadowH := h
			if kind == argonTail {
				shadowH = h * 0.9
			}
			c = over(c, color.NRGBA{0, 0, 0, uint8(255 * min(py/shadowH, 1))}, roundedCover(px, py, 0, 0, w, shadowH, r))
			k := roundedCover(px, py, 0, top, w, h-top, r)
			t := (py - top) / (h - top)
			switch kind {
			case argonTail:
				c = over(c, darken(accent, 0.6), k)
				if t < 0.5 {
					c = add(c, withAlpha(accent, 0.4*(1-t/0.5)), k)
				}
			default:
				c = over(c, mix(lighten(accent, 0.1), accent, t), k)
				// White pill along the bottom edge.
				c = over(c, color.NRGBA{255, 255, 255, 255}, roundedCover(px, py, 0, h-2*r, w, 2*r, r))
				cx, cy := w/2, h/2
				if kind == argonNote {
					// FontAwesome AngleDown, 20 px, squashed to 0.7 height, 4 px below centre.
					s := 20 * float64(argonScale)
					hw, hh := s*0.36, s*0.7*0.22
					y0 := cy + 4*float64(argonScale)
					d := min(segmentDistance(px, py, cx-hw, y0-hh, cx, y0+hh), segmentDistance(px, py, cx, y0+hh, cx+hw, y0-hh))
					c = over(c, color.NRGBA{255, 255, 255, 255}, min(max(2.2-d, 0), 1))
				} else {
					// Hold heads carry a 20x5 pill 2 px below centre.
					pw, ph := 20*float64(argonScale), 5*float64(argonScale)
					c = over(c, color.NRGBA{255, 255, 255, 255}, roundedCover(px, py, cx-pw/2, cy+2*float64(argonScale)-ph/2, pw, ph, ph/2))
				}
			}
			img.SetNRGBA(ix, iy, c)
		}
	}
	return ebiten.NewImageFromImage(img)
}
