package storyboard

import (
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggosu/internal/chart"
	"ggosu/internal/settings"
)

type Background struct {
	images      map[string]*ebiten.Image
	backgrounds []chart.StoryEvent
	sprites     [4][]chart.StoryEvent
	warnings    []string
	dim         int
}

func NewBackground(c *chart.Chart, images map[string]*ebiten.Image, d settings.Display) Background {
	b := Background{images: images, warnings: c.Warnings, dim: d.Dim}
	for _, s := range c.Storyboard {
		if s.Kind == "background" && d.Background {
			b.backgrounds = append(b.backgrounds, s)
		}
		if s.Kind == "sprite" && d.Storyboard {
			for i, l := range []string{"Background", "Pass", "Foreground", "Overlay"} {
				if s.Layer == l {
					b.sprites[i] = append(b.sprites[i], s)
				}
			}
		}
	}
	return b
}

func (b Background) Draw(dst *ebiten.Image, at time.Duration) {
	width, height := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	dst.Fill(color.NRGBA{13, 18, 28, 255})
	for i := len(b.backgrounds) - 1; i >= 0; i-- {
		s := b.backgrounds[i]
		if s.At > at {
			continue
		}
		if img := b.images[s.Asset]; img != nil {
			size := img.Bounds().Size()
			scale := max(width/float64(size.X), height/float64(size.Y))
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(scale, scale)
			op.GeoM.Translate((width-float64(size.X)*scale)/2+s.X, (height-float64(size.Y)*scale)/2+s.Y)
			dst.DrawImage(img, op)
		}
		break
	}
	// Storyboards use a centred 640x480 canvas, uniformly fitted to the target.
	scale := min(width/640, height/480)
	for _, layer := range b.sprites {
		for _, s := range layer {
			state := SpriteAt(s, at)
			img := b.images[s.Asset]
			if !state.Visible || img == nil {
				continue
			}
			size := img.Bounds().Size()
			ox, oy := .5, .5
			switch s.Origin {
			case "TopLeft":
				ox, oy = 0, 0
			case "TopCentre":
				ox, oy = .5, 0
			case "TopRight":
				ox, oy = 1, 0
			case "CentreLeft":
				ox, oy = 0, .5
			case "CentreRight":
				ox, oy = 1, .5
			case "BottomLeft":
				ox, oy = 0, 1
			case "BottomCentre":
				ox, oy = .5, 1
			case "BottomRight":
				ox, oy = 1, 1
			}
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(-float64(size.X)*ox, -float64(size.Y)*oy)
			op.GeoM.Scale(state.ScaleX*scale, state.ScaleY*scale)
			op.GeoM.Rotate(state.Rotation)
			op.GeoM.Translate(state.X*scale+(width-640*scale)/2, state.Y*scale+(height-480*scale)/2)
			op.ColorScale.Scale(float32(state.R), float32(state.G), float32(state.B), float32(state.Alpha))
			dst.DrawImage(img, op)
		}
	}
	vector.FillRect(dst, 0, 0, float32(width), float32(height), color.NRGBA{0, 0, 0, uint8(b.dim * 255 / 100)}, false)
}
