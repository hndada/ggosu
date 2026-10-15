package play

import (
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Volume uses the real clock so it also works while paused or waiting online.
type volume struct {
	repeatAt [4]time.Duration
	held     [4]bool
	selected int
	until    time.Duration
	saveAt   time.Duration
	dirty    bool
	err      string
}

var volumeNames = [...]string{"Master", "Music", "Keys", "Effects"}

func (g *scene) volumeKey(key ebiten.Key, now time.Duration) {
	v := &g.volume
	if now >= v.until {
		v.selected = 0
	}
	v.until = now + time.Second
	switch key {
	case ebiten.KeyArrowLeft:
		v.selected = (v.selected + len(volumeNames) - 1) % len(volumeNames)
	case ebiten.KeyArrowRight:
		v.selected = (v.selected + 1) % len(volumeNames)
	case ebiten.KeyArrowUp, ebiten.KeyArrowDown:
		values := [...]*int{&g.audio.Master, &g.audio.Music, &g.audio.Keys, &g.audio.Effects}
		value := values[v.selected]
		step := 5
		if key == ebiten.KeyArrowDown {
			step = -step
		}
		next := max(0, min(100, *value+step))
		if next == *value {
			return
		}
		*value = next
		if g.bank != nil {
			g.bank.SetAudio(g.audio)
		}
		v.dirty, v.saveAt = true, now+500*time.Millisecond
	}
}

func (g *scene) volumeControls(now time.Duration) {
	g.readVolumeKeys(now)
	if g.volume.dirty && now >= g.volume.saveAt {
		if err := g.saveVolume(); err != nil {
			g.volume.err = "Volume save failed"
			g.warnings = append(g.warnings, err.Error())
			g.volume.until = now + 3*time.Second
		}
	}
}

func (g *scene) saveVolume() error {
	if !g.volume.dirty {
		return nil
	}
	// A failed save stays visible; the next adjustment can retry it.
	g.volume.dirty = false
	if err := g.persistAudio(); err != nil {
		return fmt.Errorf("save volume: %w", err)
	}
	g.volume.err = ""
	return nil
}

func (g *scene) drawVolume(dst *ebiten.Image) {
	if g.view == nil || g.clock == nil || g.clock.Now() >= g.volume.until {
		return
	}
	const x, y, w = float32(18), float32(220), float32(238)
	panel(dst, x, y, w, 225)
	h := &g.view.hud
	h.Left(dst, "VOLUME", label, x+16, y+30, color.NRGBA{255, 255, 255, 255})
	values := [...]int{g.audio.Master, g.audio.Music, g.audio.Keys, g.audio.Effects}
	for i, name := range volumeNames {
		at := y + 62 + float32(i)*34
		c := color.NRGBA{163, 173, 186, 255}
		if i == g.volume.selected {
			c = color.NRGBA{110, 211, 240, 255}
		}
		h.Left(dst, name, regular, x+16, at, c)
		h.Right(dst, fmt.Sprintf("%d%%", values[i]), regular, x+w-16, at, c)
	}
	h.Left(dst, "Alt + arrows", small, x+16, y+199, color.NRGBA{163, 173, 186, 255})
	if g.volume.err != "" {
		h.Left(dst, g.volume.err, small, x+16, y+217, color.NRGBA{255, 150, 130, 255})
	}
}
