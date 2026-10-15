package storyboard

import (
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestSeekState(t *testing.T) {
	s := chart.StoryEvent{Kind: "sprite", At: 0, End: 2 * time.Second, X: 320, Y: 240, Commands: []chart.Command{{Kind: "M", End: 2 * time.Second, Values: []float64{0, 0, 640, 480}}, {Kind: "F", End: 2 * time.Second, Values: []float64{0, 1}}}}
	x := SpriteAt(s, time.Second)
	if x.X != 320 || x.Y != 240 || x.Alpha != .5 || !x.Visible {
		t.Fatalf("bad state: %+v", x)
	}
	SpriteAt(s, 2*time.Second)
	if again := SpriteAt(s, time.Second); again != x {
		t.Fatal("seek depends on past calls")
	}
	if SpriteAt(s, 3*time.Second).Visible {
		t.Fatal("expired sprite visible")
	}
}
