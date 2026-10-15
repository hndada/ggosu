package play

import (
	"image"
	"testing"

	"ggosu/internal/settings"
)

func TestFitLetterbox(t *testing.T) {
	for _, c := range []struct {
		w, h int
		d    settings.Display
		want image.Rectangle
	}{
		{2560, 1440, settings.Display{}, image.Rect(0, 0, 2560, 1440)},
		{1920, 1200, settings.Display{}, image.Rect(0, 60, 1920, 1140)},
		{1920, 1080, settings.Display{LetterboxWidth: 50}, image.Rect(0, 0, 1920, 1080)}, // Off ignores the area.
		{1920, 1080, settings.Display{Letterbox: true}, image.Rect(192, 108, 1728, 972)},
		{1920, 1080, settings.Display{Letterbox: true, LetterboxWidth: 50, LetterboxHeight: 100, LetterboxX: -50}, image.Rect(0, 270, 960, 810)},
		{1920, 1080, settings.Display{Letterbox: true, LetterboxWidth: 50, LetterboxHeight: 50, LetterboxX: 50, LetterboxY: 50}, image.Rect(960, 540, 1920, 1080)},
	} {
		if got := fit(c.w, c.h, c.d); got != c.want {
			t.Errorf("fit(%d, %d, %+v) = %v, want %v", c.w, c.h, c.d, got, c.want)
		}
	}
}

func TestCursorMapsIntoFrame(t *testing.T) {
	g := &scene{area: image.Rect(192, 108, 1728, 972)}
	// ebiten.CursorPosition is zero before the game runs.
	if got := g.cursor(); got != image.Pt(-192*Width/1536, -108*Height/864) {
		t.Fatal(got)
	}
}
