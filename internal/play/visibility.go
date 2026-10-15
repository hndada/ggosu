package play

import "github.com/hajimehoshi/ebiten/v2"

// Visibility masks the note layer, including each pixel of a long note. Keys,
// the hit line, input feedback and HUD stay readable under every effect.
const visibilityShader = `//kage:unit pixels
package main

// Each side is (edge, fade, strength): nothing shows past the edge, and the
// fade ramps to full alpha. Strength below 1 lets a fading lane cover show through.
var Top vec3
var Bottom vec3

func side(d float, s vec3) float {
	if d < 0 {
		return 0
	}
	if s.y <= 0 {
		return 1
	}
	return 1 - s.z*(1-clamp(d/s.y, 0, 1))
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	y := srcPos.y - imageSrc0Origin().y
	return imageSrc0At(srcPos) * side(y-Top.x, Top) * side(Bottom.x-y, Bottom)
}`

// Hidden and Fade In grow from 160 to 400 reference pixels with combo, as in
// osu!mania. All distances scale with the current top-to-hit-line area, so
// skins and HUD hit-position edits do not move the mask outside the lanes.
func visibilityBounds(kind string, height float32, combo int) (low, high, fade float32) {
	low, high = -height, height*2
	coverage := min(float32(400), 160+float32(max(combo, 0))*.5) / 768
	switch kind {
	case "hidden":
		high, fade = height*(1-coverage), height*50/768
	case "fadein":
		low, fade = height*coverage, height*50/768
	case "cover":
		low = height / 2
	case "flashlight":
		half := height * 50 / 768
		low, high = height/2-half, height/2+half
	}
	return
}

func (p *view) drawNotes(dst *ebiten.Image) {
	l := p.field.layout
	if p.visibilityShader == nil || !p.masked() {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(l.left), float64(l.top))
		dst.DrawImage(p.notes, op)
		return
	}
	h := l.line - l.top
	top, bottom := []float32{-h, 0, 1}, []float32{h * 2, 0, 1}
	if p.config.Visibility != "" {
		low, high, fade := visibilityBounds(p.config.Visibility, h, p.session.Score.Combo)
		top, bottom = []float32{low, fade, 1}, []float32{high, fade, 1}
	}
	// The lane cover and a visibility mod on the same side: the one hiding more wins.
	ct, cb := p.cover.bounds(h)
	if ct[0]+ct[1]*ct[2] > top[0]+top[1]*top[2] {
		top = ct
	}
	if cb[0]-cb[1]*cb[2] < bottom[0]-bottom[1]*bottom[2] {
		bottom = cb
	}
	op := &ebiten.DrawRectShaderOptions{}
	op.GeoM.Translate(float64(l.left), float64(l.top))
	op.Images[0] = p.notes
	op.Uniforms = map[string]any{"Top": top, "Bottom": bottom}
	w, hh := p.notes.Bounds().Dx(), p.notes.Bounds().Dy()
	dst.DrawRectShader(w, hh, p.visibilityShader, op)
}

// masked reports whether notes are drawn through the visibility mask this frame.
func (p *view) masked() bool { return p.config.Visibility != "" || p.cover.visible() }
