//go:build windows

package play

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

// Opt-in silent captures use real inputs and the game's HUD renderer.
func TestTimingRender(t *testing.T) {
	dir := os.Getenv("GGOSU_TIMING_CAPTURE")
	if dir == "" {
		t.Skip("set GGOSU_TIMING_CAPTURE for native timing feedback captures")
	}
	mode := os.Getenv("GGOSU_TIMING_CASE")
	d := (settings.Config{}).DisplaySettings()
	delta := 30 * time.Millisecond
	if mode == "early" {
		delta = -delta
	}
	if mode == "left" || mode == "below" {
		d.JudgementPosition = mode
	}
	if mode == "focus" {
		d.HUD = "focus"
	}
	if mode == "off" || mode == "good" {
		d.TimingThreshold = mode
	}
	c := chart.New(4)
	c.Metadata.Title = "Timing feedback"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 4 * time.Second, End: 4 * time.Second, Lane: 1, Kind: "tap"}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Second + delta
	if err := s.Input(at, 0, true); err != nil {
		t.Fatal(err)
	}
	before := s.Score
	p, err := newView(c, s, nil, viewConfig{Keys: make([]string, 4), Mode: "live", Display: &d, Font: sceneFont()})
	if err != nil {
		t.Fatal(err)
	}
	p.Update(viewState{Position: at + 100*time.Millisecond, Rate: 1, Scroll: 110})
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := &renderGame{play: p, path: filepath.Join(dir, mode+".png")}
	ebiten.SetWindowSize(Width, Height)
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	if s.Score != before {
		t.Fatal("timing feedback changed score")
	}
}
