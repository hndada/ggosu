package chart

import (
	"testing"
	"time"
)

func TestBackgroundPicksEarliestImage(t *testing.T) {
	c := New(4)
	c.Assets["late"] = Asset{Path: "b.png", Kind: "image"}
	c.Assets["early"] = Asset{Path: "a.jpg", Kind: "image"}
	c.Assets["clip"] = Asset{Path: "v.mp4", Kind: "video"}
	c.Storyboard = []StoryEvent{
		{Kind: "background", Asset: "clip", At: -time.Second},
		{Kind: "background", Asset: "late", At: 2 * time.Second},
		{Kind: "background", Asset: "early", At: 0},
		{Kind: "sprite", Asset: "late"},
	}
	if got := background(c); got != "early" {
		t.Fatalf("background = %q, want early", got)
	}
	if got := background(New(4)); got != "" {
		t.Fatalf("no storyboard: %q", got)
	}
}
