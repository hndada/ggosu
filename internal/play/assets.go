package play

import (
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
)

func (g *scene) imageAssets() map[string]bool {
	wanted := make(map[string]bool)
	for _, s := range g.chart.Storyboard {
		if s.Kind == "background" && g.display.Background || s.Kind == "sprite" && g.display.Storyboard {
			wanted[s.Asset] = true
		}
	}
	return wanted
}
