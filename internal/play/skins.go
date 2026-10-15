package play

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggosu/internal/settings"
)

// The other osu!lazer built-in mania skins, beside argon (argon.go):
//   - triangles: osu.Game.Rulesets.Mania/Skinning/Default and UI/Components (Default*).
//   - classic: Skinning/Legacy with the default legacy skin. Its textures (osu-resources, CC BY-NC) are
//     not copied; their sizes and sampled colours are redrawn in code.
//
// Sizes are lazer units at 720/768, as in argon.

var skinNames = []string{"argon", "triangles", "classic", "circles", "o2jam"}

// skinLine is the skin's default hit position: Stage.HIT_TARGET_POSITION, or the legacy
// DEFAULT_HIT_POSITION (480-402)*1.6 for classic.
func skinLine(skin string) float32 {
	if skin == "o2jam" {
		return 600
	}
	if skin == "classic" {
		return Height - 124.8*argonScale
	}
	return Height - argonHitTarget
}

var (
	white = color.NRGBA{255, 255, 255, 255}
	black = color.NRGBA{0, 0, 0, 255}
	// ManiaTrianglesSkinTransformer column colours.
	trianglesEven    = color.NRGBA{104, 224, 146, 255}
	trianglesOdd     = color.NRGBA{245, 126, 184, 255}
	trianglesSpecial = color.NRGBA{98, 201, 250, 255}
	// The brightest column of the default legacy mania-note1, -note2 and -noteS textures.
	classicWhite  = color.NRGBA{233, 232, 232, 255}
	classicPink   = color.NRGBA{249, 168, 210, 255}
	classicYellow = color.NRGBA{242, 232, 0, 255}
)

// skinColour is the column's accent. Triangles and classic alternate from the stage edges
// (LegacyManiaColumnElement.FallbackColumnIndex) with a third colour for the special column.
func skinColour(skin string, lanes, i int, scratch bool) color.NRGBA {
	special := scratch || argonSpecialColumn(lanes, i)
	edge := min(i, lanes-1-i)
	switch skin {
	case "o2jam":
		if special {
			return color.NRGBA{255, 215, 76, 255}
		}
		if edge%2 == 0 {
			return color.NRGBA{226, 237, 248, 255}
		}
		return color.NRGBA{100, 158, 255, 255}
	case "triangles":
		if special {
			return trianglesSpecial
		}
		if edge%2 == 0 {
			return trianglesOdd
		}
		return trianglesEven
	case "classic":
		if special {
			return classicYellow
		}
		if edge%2 == 0 {
			return classicWhite
		}
		return classicPink
	}
	if scratch {
		return argonSpecial
	}
	return argonColour(lanes, i)
}

// noteHeight: argon NOTE_HEIGHT, triangles DefaultNotePiece.NOTE_HEIGHT, classic the 512x164 note
// texture scaled to the column width.
func (p *view) noteHeight(width float32) float32 {
	return skinNoteHeight(p.skin, width) * p.noteScale()
}

func (p *view) noteScale() float32 {
	return float32((settings.Display{NoteThickness: p.noteThickness}).Thickness()) / 100
}

func skinNoteHeight(skin string, width float32) float32 {
	switch skin {
	case "o2jam":
		return 32
	case "circles":
		return width * 0.7
	case "triangles":
		return 12 * argonScale
	case "classic":
		return width * 164 / 512
	}
	return argonNoteHeight
}

const (
	trianglesNote uint8 = 10 + iota
	classicNote
	classicBody
	classicKey
	classicKeyDown
	classicLight
	circleNote
)

func renderSprite(kind uint8, w float64, c color.NRGBA) *ebiten.Image {
	switch kind {
	case circleNote:
		return renderCircle(w, c)
	case trianglesNote:
		return renderTriangles(w, c)
	case classicNote, classicBody:
		return renderClassicNote(kind, w, c)
	case classicKey, classicKeyDown:
		return renderClassicKey(kind, w)
	case classicLight:
		return renderClassicLight(w)
	}
	if kind >= o2Flare && kind < o2Flare+o2FlareFrames {
		return renderO2Flare(int(kind-o2Flare), w, c)
	}
	if kind >= o2Click && kind < o2Click+o2ClickFrames {
		return renderO2Click(int(kind-o2Click), w)
	}
	return renderArgon(kind, w, c)
}

// Circles uses an inset disc with a white rim; heads and tails share its silhouette.
func renderCircle(w float64, c color.NRGBA) *ebiten.Image {
	h := w * 0.7
	img := image.NewNRGBA(image.Rect(0, 0, int(math.Ceil(w)), int(math.Ceil(h))))
	r := h / 2
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			d := math.Hypot(float64(x)+0.5-w/2, float64(y)+0.5-r)
			a := max(0, min(1, r-d))
			px := mix(c, white, max(0, min(1, (d-r+3)/2)))
			img.SetNRGBA(x, y, withAlpha(px, a))
		}
	}
	return ebiten.NewImageFromImage(img)
}

// Rounded lane-coloured notes retain a lighter upper edge for contrast.
func renderTriangles(w float64, accent color.NRGBA) *ebiten.Image {
	h := 12 * float64(argonScale)
	H := int(math.Ceil(h))
	img := image.NewNRGBA(image.Rect(0, 0, int(w), H))
	for iy := range H {
		for ix := range int(w) {
			px, py := float64(ix)+0.5, float64(iy)+0.5
			k := roundedCover(px, py, 0, 0, w, h, 5*float64(argonScale))
			c := over(color.NRGBA{}, lighten(accent, 0.25), k)
			if py >= h/2 {
				c = over(color.NRGBA{}, accent, k)
			}
			img.SetNRGBA(ix, iy, c)
		}
	}
	return ebiten.NewImageFromImage(img)
}

// classicShade follows the default legacy note texture across its width: dark edges rising to the
// brightest column just left of centre, then a step down and a fade to the right.
func classicShade(t float64) float64 {
	if t < 0.45 {
		return 0.67 + 0.33*t/0.45
	}
	if t < 0.5 {
		return 1 - 0.13*(t-0.45)/0.05
	}
	return 0.87 - 0.21*(t-0.5)/0.5
}

func shade(c color.NRGBA, f float64) color.NRGBA {
	g := func(v uint8) uint8 { return uint8(min(255, float64(v)*f)) }
	return color.NRGBA{g(c.R), g(c.G), g(c.B), c.A}
}

// mania-noteN: shaded body, a one-pixel darker rim, and a light line inside the top and bottom edges.
// mania-noteNL (hold body) is the same shading inset to 19%..86% of the width, without the rim.
func renderClassicNote(kind uint8, w float64, c color.NRGBA) *ebiten.Image {
	W, H := int(w), int(math.Ceil(w*164/512))
	if kind == classicBody {
		H = 1
	}
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	for iy := range H {
		for ix := range W {
			t := (float64(ix) + 0.5) / w
			px := shade(c, classicShade(t))
			switch {
			case kind == classicBody:
				if t < 0.19 || t > 0.86 {
					px = color.NRGBA{}
				}
			case ix == 0 || ix == W-1 || iy == 0 || iy == H-1:
				px = shade(c, 0.52)
			case iy == 1 || iy == H-2:
				px = mix(px, white, 0.6)
			}
			img.SetNRGBA(ix, iy, px)
		}
	}
	return ebiten.NewImageFromImage(img)
}

// mania-key1 / mania-key1D (100x214 at 2x): a lilac key, brightest a fifth of the way down, with a
// dark bottom edge. The pressed key is lighter.
func renderClassicKey(kind uint8, w float64) *ebiten.Image {
	W, H := int(w), int(math.Round(107*float64(argonScale)))
	top, peak, bottom := color.NRGBA{164, 150, 164, 255}, color.NRGBA{228, 197, 226, 255}, color.NRGBA{200, 179, 199, 255}
	if kind == classicKeyDown {
		top, peak, bottom = color.NRGBA{230, 220, 230, 255}, color.NRGBA{254, 248, 254, 255}, color.NRGBA{214, 201, 213, 255}
	}
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	r := 4 * float64(argonScale)
	for iy := range H {
		t := (float64(iy) + 0.5) / float64(H)
		c := mix(top, peak, min(t/0.19, 1))
		if t > 0.19 {
			c = mix(peak, bottom, (t-0.19)/0.81)
		}
		if iy == H-1 {
			c = black
		}
		for ix := range W {
			k := roundedCover(float64(ix)+0.5, float64(iy)+0.5, 0, -r, w, float64(H)+r, r)
			img.SetNRGBA(ix, iy, over(color.NRGBA{}, c, k))
		}
	}
	return ebiten.NewImageFromImage(img)
}

// mania-stage-light (100x500 at 2x): white whose alpha rises linearly from the top to the bottom.
func renderClassicLight(w float64) *ebiten.Image {
	W, H := int(w), int(math.Round(250*float64(argonScale)))
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	for iy := range H {
		c := withAlpha(white, (float64(iy)+0.5)/float64(H))
		for ix := range W {
			img.SetNRGBA(ix, iy, c)
		}
	}
	return ebiten.NewImageFromImage(img)
}

// lightingN: a white core, a pale yellow ring and a cyan halo, drawn additively.
var classicExplosion = func() *ebiten.Image {
	d := int(math.Round(170 * float64(argonScale)))
	img := image.NewNRGBA(image.Rect(0, 0, d, d))
	r := float64(d) / 2
	for iy := range d {
		for ix := range d {
			t := math.Hypot(float64(ix)+0.5-r, float64(iy)+0.5-r) / r
			var c color.NRGBA
			switch {
			case t < 0.22:
				c = white
			case t < 0.45:
				c = withAlpha(mix(white, color.NRGBA{255, 246, 200, 255}, (t-0.22)/0.23), 1-0.6*(t-0.22)/0.23)
			case t < 1:
				c = withAlpha(color.NRGBA{200, 250, 255, 255}, 0.4*(1-(t-0.45)/0.55))
			}
			img.SetNRGBA(ix, iy, c)
		}
	}
	return ebiten.NewImageFromImage(img)
}()

// hgradient fills a rectangle from the left colour to the right colour.
func hgradient(dst *ebiten.Image, x, y, w, h float32, left, right color.NRGBA, additive bool) {
	if w <= 0 || h <= 0 {
		return
	}
	v := func(px, py float32, c color.NRGBA) ebiten.Vertex {
		a := float32(c.A) / 255
		return ebiten.Vertex{DstX: px, DstY: py, SrcX: 1, SrcY: 1, ColorR: float32(c.R) / 255 * a, ColorG: float32(c.G) / 255 * a, ColorB: float32(c.B) / 255 * a, ColorA: a}
	}
	vs := []ebiten.Vertex{v(x, y, left), v(x+w, y, right), v(x, y+h, left), v(x+w, y+h, right)}
	op := &ebiten.DrawTrianglesOptions{}
	if additive {
		op.Blend = ebiten.BlendLighter
	}
	dst.DrawTriangles(vs, []uint16{0, 1, 2, 1, 3, 2}, whitePixel, op)
}

func drawSprite(dst, img *ebiten.Image, x, y, sy float32, alpha float32, additive bool) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1, float64(sy))
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleAlpha(alpha)
	if additive {
		op.Blend = ebiten.BlendLighter
	}
	dst.DrawImage(img, op)
}

// drawColumns is the static stage under the notes.
func (p *view) drawColumns(dst *ebiten.Image) {
	l := p.field.layout
	for i, lane := range l.lanes {
		switch p.skin {
		case "triangles":
			// DefaultColumnBackground: the accent darkened by 5.
			fill(dst, lane.x, l.top, lane.width, l.bottom-l.top, darken(lane.color, 5), false)
		case "classic":
			// LegacyStageBackground: black columns, a white line left of each and right of the last,
			// down to the hit position at 0.74 width.
			fill(dst, lane.x, l.top, lane.width, l.bottom-l.top, black, false)
		case "o2jam":
			fill(dst, lane.x, l.top, lane.width, l.bottom-l.top, color.NRGBA{8, 13, 23, 255}, false)
		default:
			fill(dst, lane.x, l.top, lane.width, l.line-l.top, withAlpha(darken(lane.color, 3), 0.8), false)
		}
		if p.laneLines && i > 0 {
			fill(dst, lane.x, l.top, 1, l.line-l.top, withAlpha(white, 0.4), false)
		}
	}
	if p.skin == "o2jam" {
		for _, x := range []float32{l.left - 8, l.left + l.width} {
			gradient(dst, x, l.top, 8, l.bottom-l.top, color.NRGBA{147, 167, 190, 255}, color.NRGBA{35, 51, 73, 255}, false)
		}
	}
}

// drawStageLight draws what sits under the notes but changes with input or reaches below the hit
// position, straight onto the frame: classic's stage light and hit target.
func (p *view) drawStageLight(dst *ebiten.Image) {
	if p.skin != "classic" {
		return
	}
	l := p.field.layout
	s := argonScale
	for k, lane := range l.lanes {
		// LegacyColumnBackground: mania-stage-light shows while the key is down and shrinks
		// downwards as it fades over 250 ms after release. Its foot is LightPosition (480-413)*1.6.
		if down, pressAge, releaseAge := p.keyState(k); pressAge >= 0 && (down || pressAge < time.Hour) {
			a := float32(1)
			if !down {
				a = float32(1 - min(float64(releaseAge)/float64(250*time.Millisecond), 1))
			}
			if a > 0 {
				img := p.sprites.get(classicLight, lane.width, white)
				h := float32(img.Bounds().Dy()) * a
				drawSprite(dst, img, lane.x, l.bottom-107.2*s-h, a, a, false)
			}
		}
	}
	// mania-stage-hint (52 px tall at 2x, scaled 0.9*1.6025) centred on the hit position: a pink haze
	// above a yellow line and a blue band below, then the 90% white judgement line.
	h := 26 * 0.9 * 1.6025 * s
	y := l.line - h/2
	gradient(dst, l.left, y+h*0.12, l.width, h*0.38, withAlpha(color.NRGBA{251, 190, 255, 255}, 0.02), withAlpha(color.NRGBA{251, 190, 255, 255}, 0.32), false)
	gradient(dst, l.left, l.line+1, l.width, h*0.48, withAlpha(color.NRGBA{198, 211, 251, 255}, 0.71), withAlpha(color.NRGBA{141, 193, 243, 255}, 0.5), false)
	fill(dst, l.left, y+h*0.92, l.width, 1, withAlpha(color.NRGBA{238, 234, 168, 255}, 0.89), false)
	fill(dst, l.left, l.line-1, l.width, 2, withAlpha(color.NRGBA{255, 239, 162, 255}, 1), false)
	fill(dst, l.left, l.line, l.width, 1, withAlpha(white, 0.9), false)
}

// drawColumnLight and drawHitTarget draw into the note surface (x relative to the field).
func (p *view) drawColumnLight(k int, x float32, lane lane) {
	if p.skin == "classic" {
		return
	}
	l := p.field.layout
	// Argon and DefaultColumnBackground: the lower half lights with the accent at 60%.
	if a := p.columnLight(k); a > 0 {
		half := (l.line - l.top) / 2
		gradient(p.notes, x, 0, lane.width, l.line-l.top, withAlpha(lane.color, 0), withAlpha(lane.color, 0.8*a), true)
		glow(p.notes, x+lane.width*0.4, half, lane.width*0.2, half, lane.color, 12, 0.3*a)
	}
}

func (p *view) drawHitTarget(x float32, lane lane) {
	l := p.field.layout
	line := l.line - l.top
	switch p.skin {
	case "o2jam":
		fill(p.notes, x, line-2, lane.width, 2, color.NRGBA{255, 91, 73, 255}, false)
	case "circles":
		r := skinNoteHeight(p.skin, lane.width) / 2
		vector.StrokeCircle(p.notes, x+lane.width/2, line-r, r-1, 2, withAlpha(white, 0.45), true)
	case "triangles":
		// DefaultHitTarget: a 60% black bar of note height and a 2-unit white line with an accent glow.
		h := 12 * argonScale
		fill(p.notes, x, line-h, lane.width, h, withAlpha(black, 0.6), false)
		glow(p.notes, x, line-2*argonScale, lane.width, 2*argonScale, withAlpha(lane.color, 1), 5*float64(argonScale), 0.5)
		fill(p.notes, x, line-2*argonScale, lane.width, 2*argonScale, white, false)
	case "classic":
	default:
		// ArgonHitTarget: a faint white band where notes land.
		h := argonNoteHeight * argonAccentRatio
		fill(p.notes, x, line-h, lane.width, h, withAlpha(white, 0.3), true)
	}
}

type piece uint8

const (
	notePiece piece = iota
	headPiece
	tailPiece
)

// drawPiece draws a note, hold head or hold tail with its bottom edge at y.
func (p *view) drawPiece(kind piece, x, y, w float32, c color.NRGBA, alpha float32) {
	h := p.noteHeight(w)
	if kind == tailPiece {
		if p.skin == "triangles" {
			glow(p.notes, x, y-h, w, h, lighten(c, 1), 10*float64(argonScale), 0.2*float64(alpha))
		}
		drawSprite(p.notes, p.tailSprite(w, c), x, y-h, p.noteScale(), alpha, false)
		return
	}
	switch p.skin {
	case "o2jam":
		drawO2Note(p.notes, x, y-h, w, h, c, alpha)
	case "circles":
		drawSprite(p.notes, p.sprites.get(circleNote, w, c), x, y-h, p.noteScale(), alpha, false)
	case "triangles":
		// DefaultNotePiece for notes, heads and tails, with a 10-unit glow of the lightened accent.
		glow(p.notes, x, y-h, w, h, lighten(c, 1), 10*float64(argonScale), 0.2*float64(alpha))
		drawSprite(p.notes, p.sprites.get(trianglesNote, w, c), x, y-h, p.noteScale(), alpha, false)
	case "classic":
		drawSprite(p.notes, p.sprites.get(classicNote, w, c), x, y-h, p.noteScale(), alpha, false)
	default:
		sprite := [...]uint8{argonNote, argonHead}[kind]
		drawSprite(p.notes, p.sprites.get(sprite, w, c), x, y-h, p.noteScale(), alpha, false)
	}
}

// Hold tails are the skin's tail piece cut into an upward arrow (a pentagon) so they read
// apart from heads. Cached per skin, width and colour like the other sprites.
const tailSprites uint8 = 60

func (p *view) tailSprite(w float32, c color.NRGBA) *ebiten.Image {
	kind := tailSprites
	switch p.skin {
	case "o2jam":
		kind++
	case "circles":
		kind += 2
	case "triangles":
		kind += 3
	case "classic":
		kind += 4
	}
	if p.sprites.cache == nil {
		p.sprites.cache = map[argonKey]*ebiten.Image{}
	}
	k := argonKey{kind, int(math.Round(float64(w))), c}
	if img, ok := p.sprites.cache[k]; ok {
		return img
	}
	h := skinNoteHeight(p.skin, w)
	img := ebiten.NewImage(int(math.Ceil(float64(w))), int(math.Ceil(float64(h))))
	switch p.skin {
	case "o2jam":
		drawO2Note(img, 0, 0, w, h, c, 1)
	case "circles":
		drawSprite(img, p.sprites.get(circleNote, w, c), 0, 0, 1, 1, false)
	case "triangles":
		drawSprite(img, p.sprites.get(trianglesNote, w, c), 0, 0, 1, 1, false)
	case "classic":
		drawSprite(img, p.sprites.get(classicNote, w, c), 0, 0, 1, 1, false)
	default:
		drawSprite(img, p.sprites.get(argonTail, w, c), 0, 0, 1, 1, false)
	}
	// Cut the two top corners away: the apex is the top centre, the shoulders half way down.
	shoulder := h * 0.5
	v := func(x, y float32) ebiten.Vertex {
		return ebiten.Vertex{DstX: x, DstY: y, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	vs := []ebiten.Vertex{v(-1, -1), v(w/2, -1), v(-1, shoulder), v(w+1, -1), v(w+1, shoulder)}
	op := &ebiten.DrawTrianglesOptions{Blend: ebiten.BlendDestinationOut, AntiAlias: true}
	img.DrawTriangles(vs, []uint16{0, 1, 2, 1, 3, 4}, whitePixel, op)
	p.sprites.cache[k] = img
	return img
}

// The terminal cap has a tapering trail above it; failed notes keep the same
// silhouette with their desaturated colour and alpha.
func (p *view) drawTail(x, y, w float32, c color.NRGBA, alpha float32) {
	h := p.noteHeight(w)
	trail := float32(40)
	gradient(p.notes, x+w*0.2, y-h-trail, w*0.6, trail, withAlpha(c, 0), withAlpha(c, 0.6*float64(alpha)), false)
	p.drawPiece(tailPiece, x, y, w, c, alpha)
}

// drawBody draws a hold body between top and bottom; holding pulses it while the key is held.
func (p *view) drawBody(x, top, w, h float32, c color.NRGBA, alpha float32, holding bool) {
	a := float64(alpha)
	switch p.skin {
	case "o2jam":
		if holding {
			c = mix(c, white, 0.4)
		}
		fill(p.notes, x+w*0.2, top, w*0.6, h, withAlpha(c, 0.55*a), false)
		fill(p.notes, x+w*0.2, top, 2, h, withAlpha(c, a), true)
		fill(p.notes, x+w*0.8-2, top, 2, h, withAlpha(c, a), true)
	case "circles":
		if holding {
			c = mix(c, white, 0.35)
		}
		fill(p.notes, x+w*0.35, top, w*0.3, h, withAlpha(c, 0.65*a), false)
	case "triangles":
		// DefaultBodyPiece, additive: the accent at 70%, and over it the accent at 50% with a
		// soft-edged hole down the middle. A held note stays brighter without flashing.
		fill(p.notes, x, top, w, h, withAlpha(c, 0.7*a), true)
		over := withAlpha(c, 0.5*a)
		if holding {
			over = withAlpha(lighten(c, 0.2), 0.5*a)
		}
		hgradient(p.notes, x, top, w/2, h, over, withAlpha(c, 0), true)
		hgradient(p.notes, x+w/2, top, w/2, h, withAlpha(c, 0), over, true)
	case "classic":
		// mania-noteNL stretched along the hold.
		drawSprite(p.notes, p.sprites.get(classicBody, w, c), x, top, h, alpha, false)
	default:
		// ArgonHoldBodyPiece: the lane colour darkened, steadily brighter while held.
		fill(p.notes, x, top, w, h, withAlpha(darken(c, 0.6), a), false)
		if holding {
			fill(p.notes, x, top, w, h, withAlpha(lighten(c, 0.2), 0.3), true)
		}
	}
}

func (p *view) drawHoldGlow(x, top, w, h float32, c color.NRGBA) {
	if p.skin == "o2jam" { // its longflare (drawO2Flare) marks the hold instead
		return
	}
	glow(p.notes, x+w*0.15, top, w*0.7, h, c, 14, 0.28)
	fill(p.notes, x+w*0.12, top, 2, h, withAlpha(lighten(c, 0.5), 0.56), true)
	fill(p.notes, x+w*0.88-2, top, 2, h, withAlpha(lighten(c, 0.5), 0.56), true)
	line := p.field.layout.line - p.field.layout.top
	glow(p.notes, x, line-p.noteHeight(w), w, p.noteHeight(w), c, 28, 0.8)
}

func (p *view) drawKeys(dst *ebiten.Image, k int, lane lane) {
	switch p.skin {
	case "o2jam":
		p.drawO2Key(dst, k, lane)
	case "circles":
		l := p.field.layout
		alpha, light := p.keyLight(k)
		r := lane.width * 0.2
		cx, cy := lane.x+lane.width/2, l.line+r+12*argonScale
		disc(dst, cx, cy, r, withAlpha(lane.color, 0.2+0.8*alpha))
		ring(dst, cx, cy, r, 2, mix(lane.color, white, light))
	case "triangles":
		p.drawTrianglesKey(dst, k, lane)
	case "classic":
		// LegacyKeyArea: mania-keyN while up, mania-keyND while down, back 80 ms after release.
		down, pressAge, releaseAge := p.keyState(k)
		kind := classicKey
		if pressAge >= 0 && (down || pressAge < time.Hour && releaseAge < 80*time.Millisecond) {
			kind = classicKeyDown
		}
		img := p.sprites.get(kind, lane.width, white)
		drawSprite(dst, img, lane.x, p.field.layout.bottom-float32(img.Bounds().Dy()), 1, 1, false)
	default:
		p.drawKeyArea(dst, k, lane)
	}
}

// DefaultKeyArea: under the hit position a black gradient at 50% and a 10-unit key outline 20 units
// down that grows to 1.4 in 50 ms on press, settles at 1.3, and returns in 125 ms on release.
func (p *view) drawTrianglesKey(dst *ebiten.Image, k int, lane lane) {
	l := p.field.layout
	s := argonScale
	gradient(dst, lane.x, l.line, lane.width, 110*s, withAlpha(black, 0), withAlpha(black, 0.5), false)
	scale := 1.0
	if down, pressAge, releaseAge := p.keyState(k); pressAge >= 0 && (down || pressAge < time.Hour) {
		scale = tween(pressAge, 50*time.Millisecond, 1, 1.4)
		if pressAge > 50*time.Millisecond {
			scale = tween(pressAge-50*time.Millisecond, 250*time.Millisecond, 1.4, 1.3)
		}
		if !down {
			scale = tween(releaseAge, 125*time.Millisecond, min(scale, 1.3), 1)
		}
	}
	size := 10 * s * float32(scale)
	cx, cy := lane.x+lane.width/2, l.line+20*s
	glow(dst, cx-size/2, cy-size/2, size, size, lane.color, 5*float64(s), 0.5)
	var path vector.Path
	r := 3 * s * float32(scale)
	x0, y0, x1, y1 := cx-size/2+s, cy-size/2+s, cx+size/2-s, cy+size/2-s
	path.MoveTo(x0+r, y0)
	path.ArcTo(x1, y0, x1, y1, r)
	path.ArcTo(x1, y1, x0, y1, r)
	path.ArcTo(x0, y1, x0, y0, r)
	path.ArcTo(x0, y0, x1, y0, r)
	path.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(mix(lane.color, white, 0.25))
	vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: 2 * s}, op)
}

// drawExplosion draws a hit effect age after the hit.
func (p *view) drawExplosion(dst *ebiten.Image, lane lane, age time.Duration) {
	l := p.field.layout
	s := argonScale
	if p.state.Rate > 0 {
		age = time.Duration(float64(age) / p.state.Rate)
	}
	if age >= 0 && age < 280*time.Millisecond {
		t := float32(age) / float32(280*time.Millisecond)
		cx, cy := lane.x+lane.width/2, l.line-p.noteHeight(lane.width)/2
		c := withAlpha(mix(lane.color, white, 0.35), float64((1-t)*(1-t)))
		ring(dst, cx, cy, lane.width*(0.2+0.55*t), 3*(1-t), c)
		for i := 0; i < 6; i++ {
			a := float64(i)*math.Pi/3 - math.Pi/2
			r := lane.width * (0.25 + 0.6*t)
			disc(dst, cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a)), 2.5*(1-t), c)
		}
	}
	switch p.skin {
	case "circles":
		if age >= 200*time.Millisecond {
			return
		}
		t := float32(age) / float32(200*time.Millisecond)
		r := p.noteHeight(lane.width) / 2
		ring(dst, lane.x+lane.width/2, l.line-r, r*(1+t), 3*(1-t), withAlpha(lane.color, float64(1-t)))
	case "triangles":
		// DefaultHitExplosion over 200 ms (PoolableHitExplosion.DURATION), centred half a note above
		// the line and squashed to 60%: a faint glow widening fivefold and a bright one growing to 1.4.
		if age >= 200*time.Millisecond {
			return
		}
		t := float64(age) / float64(200*time.Millisecond)
		a := 1 - (1 - (1-t)*(1-t))
		cx, cy := lane.x+lane.width/2, l.line-6*s
		h := 12 * s * 0.6
		faint := lane.width * 0.8 * float32(tween(age, 200*time.Millisecond, 1, 5))
		glow(dst, cx-faint/2, cy-h/2, faint, h, mix(lane.color, white, 0.1), 60*float64(s), 0.3*a)
		main := lane.width * float32(tween(age, 200*time.Millisecond, 1, 1.4))
		glow(dst, cx-main/2, cy-h/2, main, h, mix(lane.color, white, 0.6), 30*float64(s), a)
		fill(dst, cx-main/2, cy-h/2, main, h, withAlpha(mix(lane.color, white, 0.6), a), true)
	case "classic":
		// LegacyHitExplosion: lightingN fades in over 80 ms and out over 120 ms.
		if age >= 200*time.Millisecond {
			return
		}
		a := float32(age) / float32(80*time.Millisecond)
		if age > 80*time.Millisecond {
			a = 1 - float32(age-80*time.Millisecond)/float32(120*time.Millisecond)
		}
		d := float32(classicExplosion.Bounds().Dx())
		drawSprite(dst, classicExplosion, lane.x+lane.width/2-d/2, l.line-d/2, 1, a, true)
	case "o2jam":
		p.drawO2Click(dst, lane, age)
	default:
		// ArgonHitExplosion: a near-white box with a lane-coloured glow, gone in 200 ms.
		if age >= 200*time.Millisecond {
			return
		}
		t := float64(age) / float64(200*time.Millisecond)
		a := 1 - (1 - (1-t)*(1-t)) // Easing.Out
		bh := argonNoteHeight * argonAccentRatio
		glow(dst, lane.x, l.line-bh, lane.width, bh, lane.color, 60*float64(s), a)
		fill(dst, lane.x, l.line-bh, lane.width, bh, withAlpha(mix(lane.color, white, 0.8), a), true)
	}
}
