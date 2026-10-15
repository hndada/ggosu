package settings

import (
	"math"
	"path/filepath"
	"testing"
)

func TestPlaybackAndFastScrollSurviveReload(t *testing.T) {
	c := Config{}
	p, err := c.Profile(4)
	if err != nil {
		t.Fatal(err)
	}
	p.Scroll, p.ScrollOffset = 35, 10
	c.SetProfile(4, p)
	c.Playback = &Playback{Health: 3, HealthFail: 2, Mode: "auto", Mod: "mirror+random", Visibility: "hidden", Rate: 1.75, Pitch: false}
	c.Display = &Display{Skin: "circles", Dim: 75, Background: true}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.PlaybackSettings() != *c.Playback || got.DisplaySettings() != *c.Display {
		t.Fatal("options lost on reload", got, err)
	}
	q, err := got.Profile(4)
	if err != nil || q.Scroll != 35 || q.ScrollOffset != 10 || q.Effective().Scroll != MaxScroll {
		t.Fatal("fast scroll lost on reload", q, err)
	}
	for _, speed := range []float64{.9, 40.1, math.NaN(), math.Inf(1)} {
		q.Scroll = speed
		if q.Validate(4) == nil {
			t.Fatal("accepted invalid speed", speed)
		}
	}
	if d := (Config{}).PlaybackSettings(); d.Rate != 1 || !d.Pitch || d.Mode != "live" || d.Health != 3 || d.HealthFail != 0 {
		t.Fatal("legacy defaults changed", d)
	}
}

func TestScoreServerDefaultsAndPersistence(t *testing.T) {
	if got := (Config{}).ScoreURL(); got != "http://127.0.0.1:8080" {
		t.Fatal(got)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, server := range []string{"https://example.com/gosu", ""} {
		c := Config{ScoreServer: &server}
		if err := Save(path, c); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.ScoreURL() != server {
			t.Fatal(got, err)
		}
	}
	for _, server := range []string{"file:///tmp", "https://user:password@example.com", "http://", "https://example.com?q=secret", "https://example.com#fragment"} {
		if err := ValidateScoreServer(server); err == nil {
			t.Fatal("accepted", server)
		}
	}
}
