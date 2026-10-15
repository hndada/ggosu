package ui

import (
	"fmt"
	"image/color"
	"strings"

	"ggosu/internal/typeface"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	Small = iota
	Regular
	Label
	Large
	Combo
	CounterSmall
	CounterRegular
	CounterLabel
	CounterLarge
	CounterCombo
)

// Text holds the fonts used for rendering. Callers choose positions and colors.
type Text struct {
	Faces [10]font.Face
}

func NewText(data []byte) (Text, error) {
	var t Text
	var fonts [3]*opentype.Font
	for i, name := range []string{"gmarketmedium.ttf", "gmarketbold.ttf", "counter.ttf"} {
		b, err := typeface.Files.ReadFile("fonts/" + name)
		if err != nil {
			return t, fmt.Errorf("ui font %s: %w", name, err)
		}
		fonts[i], err = opentype.Parse(b)
		if err != nil {
			return t, fmt.Errorf("ui font %s: %w", name, err)
		}
	}
	var extra *opentype.Font
	if len(data) != 0 {
		var err error
		extra, err = opentype.Parse(data)
		if err != nil {
			return t, fmt.Errorf("ui fallback font: %w", err)
		}
	}
	sizes := [...]float64{14, 18, 22, 34, 56}
	for i := range t.Faces {
		size := sizes[i%len(sizes)]
		if i == CounterCombo {
			size = 96 // Counters are reduced to their ink height, never enlarged from a small glyph.
		}
		f := fonts[0]
		if i >= CounterSmall {
			f = fonts[2]
		} else if i >= Label {
			f = fonts[1]
		}
		opts := &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull}
		face, err := opentype.NewFace(f, opts)
		if err != nil {
			return t, err
		}
		t.Faces[i] = face
		if extra != nil && i < CounterSmall {
			alt, err := opentype.NewFace(extra, opts)
			if err != nil {
				return t, err
			}
			t.Faces[i] = fallback{Face: face, alt: alt}
		}
		text.CacheGlyphs(t.Faces[i], "0123456789.,%+-: /")
	}
	return t, nil
}

// CounterWidth measures the same fixed-width numerals used by the web result.
func (t Text) CounterWidth(s string, height float32) float32 {
	f := t.Faces[CounterCombo]
	b, _ := font.BoundString(f, "0")
	scale := float64(height) * 64 / float64(b.Max.Y-b.Min.Y)
	return float32(float64(font.MeasureString(f, s)) / 64 * scale)
}

// Counter draws at an ink height rather than a font size, preserving HUD anchors.
func (t Text) Counter(dst *ebiten.Image, s string, x, y, height float32, c color.NRGBA) {
	f := t.Faces[CounterCombo]
	b, _ := font.BoundString(f, "0")
	scale := float64(height) * 64 / float64(b.Max.Y-b.Min.Y)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y)-float64(b.Min.Y)/64*scale)
	op.ColorScale.ScaleWithColor(c)
	text.DrawWithOptions(dst, s, f, op)
}

func (t Text) Left(dst *ebiten.Image, s string, size int, x, y float32, c color.NRGBA) {
	text.Draw(dst, s, t.Faces[size], int(x), int(y), c)
}

func (t Text) Center(dst *ebiten.Image, s string, size int, x, y float32, c color.NRGBA) {
	w := font.MeasureString(t.Faces[size], s).Ceil()
	t.Left(dst, s, size, x-float32(w)/2, y, c)
}

// Posed draws s centered on x, scaled and rotated (radians) around its own middle; y is the baseline at scale 1.
func (t Text) Posed(dst *ebiten.Image, s string, size int, x, y float32, scale, angle float64, c color.NRGBA) {
	f := t.Faces[size]
	m := f.Metrics()
	cx, cy := float64(font.MeasureString(f, s).Ceil())/2, float64(m.Descent.Ceil()-m.Ascent.Ceil())/2
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-cx, -cy)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Rotate(angle)
	op.GeoM.Translate(float64(x), float64(y)+cy)
	op.ColorScale.ScaleWithColor(c)
	text.DrawWithOptions(dst, s, f, op)
}

func (t Text) Right(dst *ebiten.Image, s string, size int, x, y float32, c color.NRGBA) {
	w := font.MeasureString(t.Faces[size], s).Ceil()
	t.Left(dst, s, size, x-float32(w), y, c)
}

func Fit(s string, f font.Face, width int) string {
	s = strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
	if font.MeasureString(f, s).Ceil() <= width {
		return s
	}
	limit := fixed.I(width) - font.MeasureString(f, "...")
	var advance fixed.Int26_6
	var previous rune
	first := true
	for i, r := range s {
		if !first {
			advance += f.Kern(previous, r)
		}
		a, _ := f.GlyphAdvance(r)
		advance += a
		if advance > limit {
			return s[:i] + "..."
		}
		previous, first = r, false
	}
	return s
}

// Wrap keeps all words, splitting a word by rune only when it cannot fit a line.
func Wrap(s string, f font.Face, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" {
			if font.MeasureString(f, line+" "+word).Ceil() <= width {
				line += " " + word
				continue
			}
			lines = append(lines, line)
			line = ""
		}
		for _, r := range word {
			next := line + string(r)
			if line != "" && font.MeasureString(f, next).Ceil() > width {
				lines = append(lines, line)
				line = string(r)
			} else {
				line = next
			}
		}
	}
	return append(lines, line)
}
