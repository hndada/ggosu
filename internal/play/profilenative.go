//go:build !js

package play

import (
	"fmt"

	"ggosu/internal/settings"
)

// saveProfile keeps in-play controls even when no score record is retained.
func (g *scene) saveProfile() error {
	if g.settingsPath == "" || g.chart == nil || g.recording.Profile == nil || g.mode == "replay" || g.preview {
		return nil
	}
	c, recovered, err := settings.Recover(g.settingsPath)
	if err != nil {
		return fmt.Errorf("read scroll preferences: %w", err)
	}
	if recovered != nil {
		g.warnings = append(g.warnings, "스크롤 저장 전에 손상된 설정을 복구했습니다.")
	}
	p, err := c.Profile(g.chart.Lanes)
	if err != nil {
		return err
	}
	played := g.profile
	e := p.Effective()
	if e.Scroll == played.Scroll && e.ExposureMS == played.ExposureMS && p.LaneCover() == played.LaneCover() {
		return nil
	}
	// Play uses effective values; keep the stored key count's own offsets.
	if e.Scroll != played.Scroll {
		p.Scroll = min(settings.MaxScroll, max(settings.MinScroll, played.Scroll-p.ScrollOffset))
	}
	if e.ExposureMS != played.ExposureMS {
		p.ExposureMS = min(5000, max(100, played.ExposureMS-p.ExposureOffset))
	}
	cover := played.LaneCover()
	p.Cover = &cover
	if err := p.Validate(g.chart.Lanes); err != nil {
		return err
	}
	c.SetProfile(g.chart.Lanes, p)
	if err := settings.Save(g.settingsPath, c); err != nil {
		return fmt.Errorf("write scroll preferences: %w", err)
	}
	return nil
}
