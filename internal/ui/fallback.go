package ui

import (
	"image"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// fallback preserves title glyphs supplied by the platform's optional font.
// The bundled font still owns metrics and every glyph it supports.
type fallback struct {
	font.Face
	alt font.Face
}

func (f fallback) face(r rune) font.Face {
	if _, ok := f.Face.GlyphAdvance(r); ok {
		return f.Face
	}
	return f.alt
}

func (f fallback) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	return f.face(r).Glyph(dot, r)
}

func (f fallback) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	return f.face(r).GlyphBounds(r)
}

func (f fallback) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	return f.face(r).GlyphAdvance(r)
}

func (f fallback) Kern(a, b rune) fixed.Int26_6 {
	x, y := f.face(a), f.face(b)
	if x != y {
		return 0
	}
	return x.Kern(a, b)
}

func (f fallback) Close() error {
	err := f.Face.Close()
	other := f.alt.Close()
	if err != nil {
		return err
	}
	return other
}
