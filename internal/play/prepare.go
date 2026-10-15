package play

import (
	"fmt"
	"time"

	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func (g *scene) prepare() error {
	if g.health < 0 || g.health > 3 || g.healthFail < 0 || g.healthFail > 3 || g.health > 0 && g.healthFail > g.health {
		return fmt.Errorf("invalid health level")
	}
	if len(g.profile.Keys) == 0 {
		var err error
		g.profile, err = (settings.Config{}).Profile(g.chart.Lanes)
		if err != nil {
			return err
		}
	}
	g.profile = g.profile.Normalized()
	if err := g.profile.Validate(g.chart.Lanes); err != nil {
		return err
	}
	g.scroll = g.profile.Scroll
	c, err := g.offsets.Apply(g.chart)
	if err != nil {
		return err
	}
	c, err = g.mods.Apply(c)
	if err != nil {
		return err
	}
	if g.sectionEnd != 0 {
		if c, err = c.Section(g.sectionStart, g.sectionEnd); err != nil {
			return err
		}
	}
	g.session, err = piano.NewSession(c)
	if err != nil {
		return err
	}
	g.guides, g.guide = guideTimes(c), 0
	if g.player != nil && g.player.Rules != "" {
		g.session.Rules = g.player.Rules
	}
	if g.player != nil && !g.autoplay {
		g.health = g.player.Health
	}
	g.session.Health = piano.NewHealth(g.health, g.healthFail)
	if g.player != nil && !g.autoplay {
		g.session.Health = g.player.HealthState()
	}
	g.session.SetO2Jam(g.mods)
	if g.session.O2Jam() {
		g.health = g.session.Health.Level
	}
	g.recording, err = piano.NewChartReplay(g.chart, g.session.Rules)
	if err != nil {
		return err
	}
	g.recording.Health = g.health
	g.recording.Player = g.playerName
	g.recording.HealthStages = g.session.Health.Start > 0
	g.recording.HealthFail = g.session.Health.FailLevel
	g.recording.HealthGauge = g.session.Health.Gauge
	g.recording.Offsets = g.offsets
	g.recording.Profile = &g.profile
	g.recording.Mods = g.mods
	// Loading already gave the player time to prepare. Keep only a short real-time
	// lead, scaled to the song timeline, and never skip negative audio or notes.
	lead := time.Duration(float64(250*time.Millisecond) * g.speed)
	g.preroll = lead
	for _, a := range c.Audio {
		g.preroll = max(g.preroll, -a.At)
	}
	for _, n := range c.Notes {
		g.preroll = max(g.preroll, -n.At+lead)
	}
	if g.sectionEnd != 0 {
		// beatoraja starts a practice section one second early and plays one second past its end.
		margin := time.Duration(float64(time.Second) * g.speed)
		g.preroll = min(g.preroll, margin-g.sectionStart)
		g.sectionStop = g.sectionEnd
		for _, n := range c.Notes {
			g.sectionStop = max(g.sectionStop, n.End)
		}
		g.sectionStop += margin
	}
	if g.autoplay {
		f, err := piano.Auto(c, piano.Rules)
		if err != nil {
			return err
		}
		// Bind input on the audio timeline to the original, unmodified chart.
		f.Health = g.health
		f.HealthStages, f.HealthFail, f.HealthGauge = g.recording.HealthStages, g.recording.HealthFail, g.recording.HealthGauge
		f.ChartHash, f.Offsets = g.recording.ChartHash, g.offsets
		f.Profile = &g.profile
		f.Mods = g.mods
		g.player, err = piano.NewPlayer(f)
		if err != nil {
			return err
		}
	}
	return nil
}
