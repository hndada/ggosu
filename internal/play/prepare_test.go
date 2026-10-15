package play

import (
	"bytes"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestVisibilityPersistsInReplayAndRestartWithoutChangingScore(t *testing.T) {
	for _, visibility := range []string{"hidden", "fadein", "cover", "flashlight"} {
		g := pauseGame(t, 1)
		g.mods.Visibility, g.autoplay = visibility, true
		if err := g.prepare(); err != nil {
			t.Fatal(err)
		}
		if g.recording.Mods.Visibility != visibility || g.player == nil {
			t.Fatal("Auto or recording lost visibility")
		}
		var b bytes.Buffer
		f, err := piano.Auto(g.chart, piano.Rules)
		if err != nil {
			t.Fatal(err)
		}
		want, _, err := piano.ReplayScore(g.chart, f)
		if err != nil {
			t.Fatal(err)
		}
		f.Mods.Visibility = visibility
		if err := piano.WriteReplay(&b, f); err != nil {
			t.Fatal(err)
		}
		r, err := piano.ReadReplay(&b)
		if err != nil || r.Mods.Visibility != visibility {
			t.Fatal("replay lost visibility", err)
		}
		got, _, err := piano.ReplayScore(g.chart, r)
		if err != nil || got != want {
			t.Fatal("visibility changed judgement or score", err)
		}
		if err := g.restart(time.Second); err != nil || g.recording.Mods.Visibility != visibility {
			t.Fatal("restart lost visibility", err)
		}
	}
}

func TestStartLeadIsShortAtEveryRate(t *testing.T) {
	for _, rate := range []float64{0.5, 1, 2} {
		g := pauseGame(t, rate)
		g.chart.Metadata.LeadIn = 10 * time.Second
		if err := g.prepare(); err != nil {
			t.Fatal(err)
		}
		if got := time.Duration(float64(g.preroll) / rate); got != 250*time.Millisecond {
			t.Fatalf("rate %g: lead %v", rate, got)
		}
	}
}

func TestHealthPolicySurvivesPreparationAndOffIgnoresGameOver(t *testing.T) {
	for _, auto := range []bool{false, true} {
		for _, level := range []int{0, 3} {
			g := pauseGame(t, 1)
			g.health, g.healthFail, g.autoplay = level, 2, auto
			if err := g.prepare(); err != nil {
				t.Fatal(err)
			}
			if g.session.Health != piano.NewHealth(level, 2) {
				t.Fatal(g.session.Health)
			}
			if err := g.recording.Validate(); err != nil {
				t.Fatal("off retained an active game over policy", err)
			}
			if level == 3 && (!g.recording.HealthStages || g.recording.HealthFail != 2) {
				t.Fatal("recording lost the stage policy")
			}
		}
	}
}

func TestStartPreservesNegativeEvents(t *testing.T) {
	g := pauseGame(t, 2)
	g.chart.Timing[0].At = -5 * time.Second
	g.chart.Assets = map[string]chart.Asset{"audio": {Path: "silent.wav", Kind: "audio"}}
	g.chart.Audio = []chart.Audio{{At: -4 * time.Second, Sound: chart.Sound{Asset: "audio"}}}
	g.chart.Notes[0].At, g.chart.Notes[0].End = -2*time.Second, -2*time.Second
	hash, err := g.chart.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.prepare(); err != nil {
		t.Fatal(err)
	}
	if g.preroll != 4*time.Second {
		t.Fatalf("negative audio was skipped: %v", g.preroll)
	}
	g.chart.Audio = nil
	if err := g.prepare(); err != nil {
		t.Fatal(err)
	}
	if g.preroll != 2500*time.Millisecond {
		t.Fatalf("negative note lost its reaction time: %v", g.preroll)
	}
	g.chart.Audio = []chart.Audio{{At: -4 * time.Second, Sound: chart.Sound{Asset: "audio"}}}
	if after, err := g.chart.Digest(); err != nil || after != hash {
		t.Fatal("preparation changed source identity")
	}
}
