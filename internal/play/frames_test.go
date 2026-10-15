package play

import (
	"testing"
	"time"
)

func TestFrameWarningsRespectLimitAndStalls(t *testing.T) {
	for _, tt := range []struct {
		name        string
		fps, target float64
		gap         time.Duration
		want        int
	}{
		{"60 Hz steady", 60, 60, time.Second / 60, 0},
		{"240 Hz steady", 239, 240, time.Second / 240, 0},
		{"240 Hz mild drop", 210, 240, time.Second / 210, 1},
		{"240 Hz severe drop", 120, 240, time.Second / 120, 2},
		{"single missed frame", 240, 240, 8 * time.Millisecond, 1},
		{"single long stall", 240, 240, 40 * time.Millisecond, 2},
		{"unlimited steady", 1000, 0, time.Millisecond, 0},
		{"unlimited stall", 1000, 0, 5 * time.Millisecond, 2},
		{"unknown monitor low FPS", 25, 0, 40 * time.Millisecond, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameLevel(tt.fps, tt.target, tt.gap, tt.fps); got != tt.want {
				t.Fatalf("level = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFrameMeterRetainsStallWarningAndRecovers(t *testing.T) {
	f := frameMeter{refresh: 100}
	now := time.Unix(0, 0)
	f.sample(now, false)
	for i := 0; i < 50; i++ {
		now = now.Add(10 * time.Millisecond)
		f.sample(now, false)
	}
	if f.fps != 100 || f.text != "100  10.0ms" {
		t.Fatalf("steady sample: %v, %q", f.fps, f.text)
	}
	now = now.Add(40 * time.Millisecond)
	if level := f.sample(now, false); level != 2 {
		t.Fatal("instant stall was hidden by the average FPS", level)
	}
	for i := 0; i < 20; i++ {
		now = now.Add(10 * time.Millisecond)
		if f.sample(now, false) != 2 {
			t.Fatal("stall warning disappeared too soon")
		}
	}
	for i := 0; i < 100; i++ {
		now = now.Add(10 * time.Millisecond)
		f.sample(now, false)
	}
	if f.sample(now.Add(10*time.Millisecond), false) != 0 {
		t.Fatal("stall warning did not recover")
	}
}
