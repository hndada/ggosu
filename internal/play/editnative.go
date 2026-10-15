//go:build !js

package play

import (
	"fmt"

	"ggosu/internal/settings"
)

func (g *scene) persistHUD(hud settings.Display) error {
	if g.settingsPath == "" {
		return nil
	}
	c, recovered, err := settings.Recover(g.settingsPath)
	if err != nil {
		return fmt.Errorf("read preferences: %w", err)
	}
	if recovered != nil {
		g.warnings = append(g.warnings, "HUD 저장 전에 손상된 설정을 복구했습니다.")
	}
	d := c.DisplaySettings()
	d.Layout, d.HUD, d.TimeDisplay = hud.Layout, hud.HUD, hud.TimeDisplay
	d.ComboPosition, d.JudgementPosition, d.Skin = hud.ComboPosition, hud.JudgementPosition, hud.Skin
	d.NoteThickness = hud.NoteThickness
	c.Display = &d
	if err := settings.Save(g.settingsPath, c); err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	return nil
}
