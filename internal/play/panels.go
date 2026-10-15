package play

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image/color"
)

var panelFill = color.NRGBA{12, 20, 31, 236}
var panelLine = color.NRGBA{57, 77, 98, 255}
var tileFill = color.NRGBA{24, 36, 50, 255}

// Panels and digits are drawn at the game's logical resolution, on both native and web.
func panel(dst *ebiten.Image, x, y, w, h float32) { framed(dst, x, y, w, h, panelLine) }

// framed is a panel whose 1 px border carries a colour, e.g. a hovered button.
func framed(dst *ebiten.Image, x, y, w, h float32, line color.NRGBA) {
	rect(dst, x, y, w, h, line)
	rect(dst, x+1, y+1, w-2, h-2, panelFill)
}

func gauge(dst *ebiten.Image, x, y, w, h, value float32, c color.NRGBA) {
	value = min(1, max(0, value))
	rect(dst, x, y, w, h, tileFill)
	rect(dst, x, y, w*value, h, c)
}
