//go:build !js

package play

import (
	"fmt"
	"image"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/screenshot"
)

// saveScreenshot writes the window as drawn this frame; encoding runs off the game loop.
// As in osu!lazer's ScreenshotManager the shutter sounds at once and a toast follows the save.
func (g *scene) saveScreenshot(dst *ebiten.Image) {
	if g.bank != nil {
		if err := g.bank.Effect("shutter", 1); err != nil {
			fmt.Fprintln(os.Stderr, "screenshot sound:", err)
		}
	}
	if g.shots == nil {
		g.shots = make(chan string, 8)
	}
	b := dst.Bounds()
	img := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	dst.ReadPixels(img.Pix)
	shots := g.shots
	go func() {
		path, err := screenshot.Save(img)
		if err != nil {
			fmt.Fprintln(os.Stderr, "screenshot:", err)
			return
		}
		select {
		case shots <- path:
		default: // More saves than toasts can queue; the file is still written.
		}
	}()
}

func (g *scene) revealScreenshot(path string) {
	if err := screenshot.Reveal(path); err != nil {
		fmt.Fprintln(os.Stderr, "screenshot:", err)
	}
}
