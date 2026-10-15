package play

import (
	"testing"

	"ggosu/internal/chart"
	"ggosu/internal/settings"
)

func TestHiddenImagesNotLoaded(t *testing.T) {
	c := chart.New(4)
	c.Storyboard = []chart.StoryEvent{{Kind: "background", Asset: "bg"}, {Kind: "sprite", Asset: "sprite"}}
	g := scene{chart: c, display: settings.Display{Background: true}}
	w := g.imageAssets()
	if !w["bg"] || w["sprite"] {
		t.Fatal("loaded hidden storyboard")
	}
	g.display = settings.Display{Storyboard: true}
	w = g.imageAssets()
	if w["bg"] || !w["sprite"] {
		t.Fatal("loaded hidden background")
	}
}
