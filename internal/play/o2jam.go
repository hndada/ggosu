package play

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// O2Jam's bar notes and metallic key bed, redrawn without client textures.
func drawO2Note(dst *ebiten.Image, x, y, w, h float32, c color.NRGBA, alpha float32) {
	a := float64(alpha)
	fill(dst, x+1, y, w-2, h, withAlpha(darken(c, 1), a), false)
	gradient(dst, x+2, y+1, w-4, h-2, withAlpha(mix(c, white, 0.65), a), withAlpha(c, a), false)
	fill(dst, x+2, y+1, w-4, 1, withAlpha(white, 0.9*a), false)
	fill(dst, x+2, y+h-2, w-4, 1, withAlpha(darken(c, 0.6), a), false)
}

func (p *view) drawO2Key(dst *ebiten.Image, k int, lane lane) {
	l := p.field.layout
	a, light := p.keyLight(k)
	h := min(float32(84), l.bottom-l.line-14)
	y := l.line + 8
	c := mix(darken(lane.color, 0.5), lane.color, light)
	fill(dst, lane.x, l.line, lane.width, l.bottom-l.line, color.NRGBA{14, 22, 36, 255}, false)
	gradient(dst, lane.x+2, y, lane.width-4, h, mix(c, white, 0.55), darken(c, 0.9), false)
	fill(dst, lane.x+3, y+2, lane.width-6, 2, mix(c, white, 0.8), false)
	fill(dst, lane.x+3, y+h-5, lane.width-6, 4, darken(c, 2), false)
	if a > 0 {
		glow(dst, lane.x+2, y, lane.width-4, h, lane.color, 12, 0.35*a)
	}
}

// O2Jam's EFFECT_LONGFLARE (open2jam resources.xml): 14 frames looped at 33.3 fps over the hit
// position while a long note is held. Redrawn in the lane colour and dimmer than the original.
const (
	o2Flare       uint8 = 30
	o2FlareFrames       = 14
	o2FlareFrame        = 30 * time.Millisecond
	o2FlareAlpha        = 0.6
)

func (p *view) drawO2Flare(dst *ebiten.Image, k int, lane lane) {
	age := p.state.Position - p.flashes[k]
	if p.state.Rate > 0 {
		age = time.Duration(float64(age) / p.state.Rate)
	}
	frame := uint8(max(age, 0)/o2FlareFrame) % o2FlareFrames
	img := p.sprites.get(o2Flare+frame, lane.width, lane.color)
	s := float32(img.Bounds().Dx())
	drawSprite(dst, img, lane.x+lane.width/2-s/2, p.field.layout.line-s/2, 1, o2FlareAlpha, true)
}

// renderO2Flare draws one frame, its size 2.4 lane widths: a soft oval glow, a few white cores and
// four-point sparkles around them, each frame placed from its own seed so the loop flickers.
func renderO2Flare(frame int, w float64, c color.NRGBA) *ebiten.Image {
	return ebiten.NewImageFromImage(o2FlareImage(frame, w, c))
}

func o2FlareImage(frame int, w float64, c color.NRGBA) *image.NRGBA {
	d := int(math.Ceil(w * 2.4))
	seed := uint32(frame)*977 + 13
	rnd := func() float64 {
		seed = seed*1103515245 + 12345
		return float64(seed>>8&0xffff) / 0xffff
	}
	acc := make([][3]float64, d*d)
	add := func(x, y int, col color.NRGBA, a float64) {
		if x < 0 || y < 0 || x >= d || y >= d || a <= 0 {
			return
		}
		px := &acc[y*d+x]
		px[0] += float64(col.R) / 255 * a
		px[1] += float64(col.G) / 255 * a
		px[2] += float64(col.B) / 255 * a
	}
	u := float64(d) / 128 // the original frame is 128 px
	cx, cy := float64(d)/2+(rnd()-.5)*6*u, float64(d)/2+(rnd()-.5)*6*u
	r := (30 + rnd()*10) * u
	for y := range d {
		for x := range d {
			t := math.Hypot((float64(x)+.5-cx)/0.8, float64(y)+.5-cy) / r
			if t < 1 {
				add(x, y, c, 0.7*(1-t)*(1-t))
			}
		}
	}
	for range 3 + int(rnd()*3) {
		bx, by, br := cx+(rnd()-.5)*24*u, cy+(rnd()-.5)*40*u, (3+rnd()*4)*u
		for y := int(by - br); y <= int(by+br); y++ {
			for x := int(bx - br); x <= int(bx+br); x++ {
				if t := math.Hypot(float64(x)+.5-bx, float64(y)+.5-by) / br; t < 1 {
					add(x, y, white, 1-t)
				}
			}
		}
	}
	spark := mix(c, white, 0.5)
	for range 14 {
		a, dist := rnd()*2*math.Pi, (10+rnd()*32)*u
		sx, sy, size := int(cx+math.Cos(a)*dist*0.8), int(cy+math.Sin(a)*dist), int((2+rnd()*4)*u)
		for i := -size; i <= size; i++ {
			fade := 0.8 * (1 - math.Abs(float64(i))/float64(size+1))
			add(sx+i, sy, spark, fade)
			if i != 0 {
				add(sx, sy+i, spark, fade)
			}
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, d, d))
	for i, px := range acc {
		a := min(max(px[0], px[1], px[2]), 1)
		if a == 0 {
			continue
		}
		ch := func(v float64) uint8 { return uint8(min(v/a, 1) * 255) }
		img.SetNRGBA(i%d, i/d, color.NRGBA{ch(px[0]), ch(px[1]), ch(px[2]), uint8(a * 255)})
	}
	return img
}

// O2Jam's EFFECT_CLICK (open2jam resources.xml): 11 frames at 60 fps over the hit position after a
// hit, drawn in the original blue and white at 4.8 lane widths (the longflare's scale).
const (
	o2Click       uint8 = 44
	o2ClickFrames       = 11
	o2ClickFrame        = time.Second / 60
)

func (p *view) drawO2Click(dst *ebiten.Image, lane lane, age time.Duration) {
	frame := age / o2ClickFrame
	if frame >= o2ClickFrames {
		return
	}
	img := p.sprites.get(o2Click+uint8(frame), lane.width, white)
	s := float32(img.Bounds().Dx())
	drawSprite(dst, img, lane.x+lane.width/2-s/2, p.field.layout.line-s/2, 1, 1, true)
}

func renderO2Click(frame int, w float64) *ebiten.Image {
	return ebiten.NewImageFromImage(o2ClickImage(frame, w))
}

// o2ClickImage follows note_click5_3.png: a small dot, a star flash with rays and a horizontal
// streak (frames 1-3), a glowing ball throwing shards sideways (4-9) and a fading glow (10).
func o2ClickImage(frame int, w float64) *image.NRGBA {
	d := int(math.Ceil(w * 4.8))
	u := float64(d) / 256 // the original frame is 256 px
	c := float64(d) / 2
	acc := make([][3]float64, d*d)
	add := func(x, y int, col color.NRGBA, a float64) {
		if x < 0 || y < 0 || x >= d || y >= d || a <= 0 {
			return
		}
		px := &acc[y*d+x]
		px[0] += float64(col.R) / 255 * a
		px[1] += float64(col.G) / 255 * a
		px[2] += float64(col.B) / 255 * a
	}
	radial := func(r float64, shade func(t float64) (color.NRGBA, float64)) {
		for y := int(c - r); y <= int(c+r); y++ {
			for x := int(c - r); x <= int(c+r); x++ {
				if t := math.Hypot(float64(x)+.5-c, float64(y)+.5-c) / r; t < 1 {
					col, a := shade(t)
					add(x, y, col, a)
				}
			}
		}
	}
	core := color.NRGBA{255, 235, 255, 255}
	pale := color.NRGBA{215, 220, 255, 255}
	blue := color.NRGBA{130, 140, 255, 255}
	shard := color.NRGBA{170, 220, 255, 255}
	switch {
	case frame == 0:
		radial(16*u, func(t float64) (color.NRGBA, float64) { return mix(white, blue, t), 0.9 * (1 - t*t) })
	case frame <= 3:
		strength := []float64{0, 0.7, 1, 0.5}[frame]
		reach := []float64{0, 85, 125, 60}[frame] * u
		radial(reach, func(t float64) (color.NRGBA, float64) { return mix(pale, blue, t), 0.5 * strength * (1 - t) * (1 - t) })
		for i := range 16 {
			a := float64(i) * math.Pi / 8
			for r := 0.0; r < reach; r += 0.5 {
				add(int(c+math.Cos(a)*r), int(c+math.Sin(a)*r), blue, 0.3*strength*(1-r/reach))
			}
		}
		streak := []float64{0, 110, 125, 80}[frame] * u
		for x := int(c - streak); x <= int(c+streak); x++ {
			fade := 1 - math.Abs(float64(x)+.5-c)/streak
			for y := int(c - 2*u); y <= int(c+2*u); y++ {
				add(x, y, blue, strength*fade)
			}
		}
		radial([]float64{0, 16, 24, 20}[frame]*u, func(t float64) (color.NRGBA, float64) { return mix(white, core, t), 1 - t*t*t })
		if frame == 3 {
			radial(30*u, func(t float64) (color.NRGBA, float64) {
				return blue, 0.8 * max(0, 1-math.Abs(t-28.0/30)*15)
			})
		}
	case frame <= 9:
		j := float64(frame - 4)
		fade := 1 - j*0.08
		r := (30 + j*3) * u * 1.15
		radial(r, func(t float64) (color.NRGBA, float64) {
			if t < 0.35 {
				return mix(white, pale, t/0.35), fade
			}
			return mix(pale, blue, (t-0.35)/0.65), fade * (0.8 - 0.1*(t-0.35)/0.5) * min(1, (1-t)/0.15)
		})
		for i := range 9 {
			a := -math.Pi * float64(i) / 8
			dist := ((30+j*3)*u + 8*u + j*5*u) * []float64{0.85, 1}[i%2]
			sx, sy := c+math.Cos(a)*dist*1.6, c+math.Sin(a)*dist*0.55
			for y := int(sy - 2*u); y < int(sy+u); y++ {
				for x := int(sx - 3*u); x < int(sx+float64(3+i%3*3)*u); x++ {
					add(x, y, shard, 0.8*fade)
				}
			}
		}
	default:
		radial(55*u, func(t float64) (color.NRGBA, float64) {
			return mix(white, pale, min(t*2, 1)), 0.7 * (1 - t) * (1 - t)
		})
	}
	img := image.NewNRGBA(image.Rect(0, 0, d, d))
	for i, px := range acc {
		a := min(max(px[0], px[1], px[2]), 1)
		if a == 0 {
			continue
		}
		ch := func(v float64) uint8 { return uint8(min(v/a, 1) * 255) }
		img.SetNRGBA(i%d, i/d, color.NRGBA{ch(px[0]), ch(px[1]), ch(px[2]), uint8(a * 255)})
	}
	return img
}
