package play

import (
	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/settings"
)

func setFrameRate(d settings.Display) {
	ebiten.SetTPS(d.TickRate())
	mode := ebiten.FPSModeVsyncOn
	if d.FrameLimit == "unlimited" {
		mode = ebiten.FPSModeVsyncOffMaximum
	}
	ebiten.SetFPSMode(mode)
}
