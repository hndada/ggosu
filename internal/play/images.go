//go:build !js

package play

import (
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/chart"
)

func (g *scene) loadImages() error {
	g.images = map[string]*ebiten.Image{}
	pixels := 0
	wanted := g.imageAssets()
	for id, a := range g.chart.Assets {
		if a.Kind != "image" || !wanted[id] {
			continue
		}
		_, _, f, err := chart.OpenImage(g.chart, id)
		if err != nil {
			g.warnings = append(g.warnings, err.Error())
			continue
		}
		img, err := readImage(f, 32<<20-pixels)
		if err != nil {
			g.warnings = append(g.warnings, id+": "+err.Error())
			continue
		}
		pixels += img.Bounds().Dx() * img.Bounds().Dy()
		g.images[id] = ebiten.NewImageFromImage(img)
	}
	return nil
}

func readImage(f io.ReadSeekCloser, budget int) (img image.Image, err error) {
	defer func() { err = errors.Join(err, f.Close()) }()
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 || config.Width*config.Height > budget {
		return nil, fmt.Errorf("image dimensions exceed memory budget")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err = image.Decode(f)
	return img, err
}
