//go:build !js

package play

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/settings"
)

func (g *scene) readVolumeKeys(now time.Duration) {
	active := ebiten.IsFocused() && ebiten.IsKeyPressed(ebiten.KeyAlt)
	for i, key := range [...]ebiten.Key{ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyArrowLeft, ebiten.KeyArrowRight} {
		down := active && ebiten.IsKeyPressed(key)
		if down && !g.volume.held[i] {
			g.volumeKey(key, now)
			g.volume.repeatAt[i] = now + 300*time.Millisecond
		} else if down && now >= g.volume.repeatAt[i] {
			g.volumeKey(key, now)
			g.volume.repeatAt[i] = now + 50*time.Millisecond
		}
		g.volume.held[i] = down
	}
}

func (g *scene) persistAudio() error {
	if g.settingsPath == "" {
		return nil
	}
	c, recovered, err := settings.Recover(g.settingsPath)
	if err != nil {
		return fmt.Errorf("read preferences: %w", err)
	}
	if recovered != nil {
		g.warnings = append(g.warnings, "볼륨 저장 전에 손상된 설정을 복구했습니다.")
	}
	a := g.audio
	c.Audio = &a
	if err := settings.Save(g.settingsPath, c); err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	return nil
}
