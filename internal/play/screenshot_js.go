//go:build js

package play

import "github.com/hajimehoshi/ebiten/v2"

// The browser has its own screenshot tools.
func (g *scene) saveScreenshot(*ebiten.Image) {}

func (g *scene) revealScreenshot(string) {}
